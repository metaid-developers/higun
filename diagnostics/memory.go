package diagnostics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"
)

type RuntimeSnapshot struct {
	HeapAllocBytes     uint64
	HeapSysBytes       uint64
	HeapIdleBytes      uint64
	HeapReleasedBytes  uint64
	HeapObjects        uint64
	NumGoroutine       int
	NumGC              uint32
	GOGC               int
	GoMemoryLimitBytes int64
}

type CgroupSnapshot struct {
	Version      int
	CurrentBytes uint64
	LimitBytes   uint64
	FileBytes    uint64
	AnonBytes    uint64
	FailCount    uint64
	OOMKillCount uint64
}

type ProcSnapshot struct {
	RSSBytes       uint64
	HighWaterBytes uint64
}

type Snapshot struct {
	Runtime   RuntimeSnapshot
	Cgroup    CgroupSnapshot
	Proc      ProcSnapshot
	Timestamp time.Time
}

func CaptureSnapshot(cgroupRoot string) Snapshot {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	snapshot := Snapshot{
		Runtime: RuntimeSnapshot{
			HeapAllocBytes:     stats.HeapAlloc,
			HeapSysBytes:       stats.HeapSys,
			HeapIdleBytes:      stats.HeapIdle,
			HeapReleasedBytes:  stats.HeapReleased,
			HeapObjects:        stats.HeapObjects,
			NumGoroutine:       runtime.NumGoroutine(),
			NumGC:              stats.NumGC,
			GOGC:               currentGOGC(),
			GoMemoryLimitBytes: debug.SetMemoryLimit(-1),
		},
		Timestamp: time.Now(),
	}

	if cgroupRoot != "" {
		if cgroup, err := readCgroupMemory(cgroupRoot); err == nil {
			snapshot.Cgroup = cgroup
		}
	}
	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		snapshot.Proc = parseProcStatus(data)
	}

	return snapshot
}

func readCgroupMemory(root string) (CgroupSnapshot, error) {
	return readCgroupMemoryWithProc(root, "/proc/self/cgroup")
}

func readCgroupMemoryWithProc(root, procCgroupPath string) (CgroupSnapshot, error) {
	var firstErr error
	for _, candidate := range cgroupMemoryCandidates(root, procCgroupPath) {
		snapshot, err := readCgroupMemoryDir(candidate)
		if err == nil {
			return snapshot, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return CgroupSnapshot{}, firstErr
	}
	return CgroupSnapshot{}, fmt.Errorf("cgroup memory usage not found under %s", root)
}

func cgroupMemoryCandidates(root, procCgroupPath string) []string {
	if root == "" {
		root = "/sys/fs/cgroup"
	}

	var candidates []string
	addCandidate := func(path string) {
		if path == "" {
			return
		}
		for _, existing := range candidates {
			if existing == path {
				return
			}
		}
		candidates = append(candidates, path)
	}

	addCandidate(root)
	addCandidate(filepath.Join(root, "memory"))

	if data, err := os.ReadFile(procCgroupPath); err == nil {
		v1MemoryPath, v2Path := parseProcSelfCgroup(data)
		if v1MemoryPath != "" {
			rel := strings.TrimPrefix(v1MemoryPath, "/")
			if rel == "" {
				addCandidate(filepath.Join(root, "memory"))
			} else {
				addCandidate(filepath.Join(root, "memory", rel))
				addCandidate(filepath.Join(root, rel))
			}
		}
		if v2Path != "" {
			rel := strings.TrimPrefix(v2Path, "/")
			if rel == "" {
				addCandidate(root)
			} else {
				addCandidate(filepath.Join(root, rel))
			}
		}
	}

	return candidates
}

func parseProcSelfCgroup(data []byte) (v1MemoryPath string, v2Path string) {
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		controllers := strings.Split(parts[1], ",")
		if parts[0] == "0" && parts[1] == "" {
			v2Path = parts[2]
			continue
		}
		for _, controller := range controllers {
			if controller == "memory" {
				v1MemoryPath = parts[2]
				break
			}
		}
	}
	return v1MemoryPath, v2Path
}

func readCgroupMemoryDir(root string) (CgroupSnapshot, error) {
	if currentData, currentErr := os.ReadFile(filepath.Join(root, "memory.current")); currentErr == nil {
		maxData, err := os.ReadFile(filepath.Join(root, "memory.max"))
		if err != nil {
			return CgroupSnapshot{}, fmt.Errorf("read cgroup v2 memory.max: %w", err)
		}
		return readCgroupV2(currentData, maxData, root)
	}

	usageData, usageErr := os.ReadFile(filepath.Join(root, "memory.usage_in_bytes"))
	if usageErr != nil {
		return CgroupSnapshot{}, fmt.Errorf("cgroup memory usage not found under %s", root)
	}
	limitData, err := os.ReadFile(filepath.Join(root, "memory.limit_in_bytes"))
	if err != nil {
		return CgroupSnapshot{}, fmt.Errorf("read cgroup v1 memory.limit_in_bytes: %w", err)
	}
	failData, err := os.ReadFile(filepath.Join(root, "memory.failcnt"))
	if err != nil {
		return CgroupSnapshot{}, fmt.Errorf("read cgroup v1 memory.failcnt: %w", err)
	}

	current, err := parseFirstUint(usageData)
	if err != nil {
		return CgroupSnapshot{}, fmt.Errorf("parse cgroup v1 memory.usage_in_bytes: %w", err)
	}
	limit, err := parseFirstUint(limitData)
	if err != nil {
		return CgroupSnapshot{}, fmt.Errorf("parse cgroup v1 memory.limit_in_bytes: %w", err)
	}
	if limit > 1<<60 {
		limit = 0
	}
	failCount, err := parseFirstUint(failData)
	if err != nil {
		return CgroupSnapshot{}, fmt.Errorf("parse cgroup v1 memory.failcnt: %w", err)
	}

	snapshot := CgroupSnapshot{
		Version:      1,
		CurrentBytes: current,
		LimitBytes:   limit,
		FailCount:    failCount,
	}
	if statData, err := os.ReadFile(filepath.Join(root, "memory.stat")); err == nil {
		values := parseKeyedUintFields(statData)
		snapshot.FileBytes = values["cache"]
		snapshot.AnonBytes = values["rss"]
	}
	return snapshot, nil
}

func readCgroupV2(currentData, maxData []byte, root string) (CgroupSnapshot, error) {
	current, err := parseFirstUint(currentData)
	if err != nil {
		return CgroupSnapshot{}, fmt.Errorf("parse cgroup v2 memory.current: %w", err)
	}
	limit, err := parseCgroupV2Limit(maxData)
	if err != nil {
		return CgroupSnapshot{}, fmt.Errorf("parse cgroup v2 memory.max: %w", err)
	}

	snapshot := CgroupSnapshot{
		Version:      2,
		CurrentBytes: current,
		LimitBytes:   limit,
	}
	if eventsData, err := os.ReadFile(filepath.Join(root, "memory.events")); err == nil {
		values := parseKeyedUintFields(eventsData)
		snapshot.FailCount = values["max"]
		snapshot.OOMKillCount = values["oom_kill"]
	}
	if statData, err := os.ReadFile(filepath.Join(root, "memory.stat")); err == nil {
		values := parseKeyedUintFields(statData)
		snapshot.FileBytes = values["file"]
		snapshot.AnonBytes = values["anon"]
	}
	return snapshot, nil
}

func parseProcStatus(data []byte) ProcSnapshot {
	var snapshot ProcSnapshot
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "VmRSS:":
			snapshot.RSSBytes = value * 1024
		case "VmHWM:":
			snapshot.HighWaterBytes = value * 1024
		}
	}
	return snapshot
}

func parseCgroupV2Limit(data []byte) (uint64, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty value")
	}
	if fields[0] == "max" {
		return 0, nil
	}
	return strconv.ParseUint(fields[0], 10, 64)
}

func parseFirstUint(data []byte) (uint64, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty value")
	}
	return strconv.ParseUint(fields[0], 10, 64)
}

func parseKeyedUintFields(data []byte) map[string]uint64 {
	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[fields[0]] = value
	}
	return values
}

func currentGOGC() int {
	samples := []metrics.Sample{{Name: "/gc/gogc:percent"}}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return int(samples[0].Value.Uint64())
}
