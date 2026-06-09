# Higun BTC OOM Stabilization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make production `higun_btc` on `8.217.251.101` run under its Docker memory limit without repeated cgroup OOM kills, with enough diagnostics to prove the fixed memory behavior.

**Architecture:** Add a small diagnostics package first, then make memory budgeting cgroup-aware before changing indexing behavior. Bound Pebble memory explicitly across the stores opened by `main.go`, add stage-level memory checkpoints around BTC block processing, and only then reduce the allocation-heavy UTXO spend-query path. Production rollout uses `make linux` from `/Users/tusm/Documents/MetaID_Projects/servers/higun.md`, backup-first binary replacement, and a 24-hour soak with pprof/profile artifacts.

**Tech Stack:** Go 1.24, Gin, Pebble, `runtime`, `runtime/metrics`, `runtime/pprof`, Linux cgroup v1/v2 files, Docker, `make linux`, Go test

---

## Context And Constraints

Primary evidence:

```text
docs/ISSUE_HIGUN_BTC_MEMORY_OOM.md
```

Important current production facts from that document:

```yaml
host: 8.217.251.101
container: higun_btc
process: utxo_indexer
chain: btc
network: mainnet
api_port: 8066
compat_port: 8085
docker_memory_limit_bytes: 12884901888
workers: 2
tx_concurrency: 8
batch_size: 5000
max_tx_per_batch: 10000
memory_gb: 4
mem_utxo_max_count: 9000
```

Compile/deploy note:

```text
/Users/tusm/Documents/MetaID_Projects/servers/higun.md
```

The compile command from that file is:

```bash
make linux
```

The deploy example in that file targets MVC on `47.239.239.128`, so BTC rollout must first verify the live `8.217.251.101` path and container mounts before replacing anything.

Local validation notes:

```bash
CGO_ENABLED=0 go test ./config ./storage ./indexer
```

Use `CGO_ENABLED=0` for local package validation on this Mac when normal cgo tests fail with SDK noise. Use `make linux` before production upload because the documented deployment path builds Linux amd64 with cgo enabled.

## File Structure

Create:

- `diagnostics/memory.go`: runtime/proc/cgroup snapshot structs and readers.
- `diagnostics/memory_test.go`: unit tests for cgroup v1/v2/proc parsing.
- `diagnostics/server.go`: localhost-only debug server with pprof/expvar and profile dumps.
- `diagnostics/server_test.go`: unit tests for loopback binding and disabled behavior.
- `diagnostics/checkpoint.go`: stage checkpoint logging helper.
- `diagnostics/checkpoint_test.go`: checkpoint formatting tests.
- `config/memory_budget_test.go`: cgroup-aware budget and Go memory limit tests.
- `storage/pebble_budget_test.go`: Pebble budget tests across main stores and shards.
- `docs/higun-btc-oom-runbook.md`: rollout, soak, rollback, and acceptance runbook.

Modify:

- `config/config.go`: add diagnostics config and memory budget config.
- `config/auto.go`: compute effective memory from cgroup/config and expose Pebble budget fields.
- `config.yaml`: document disabled-by-default diagnostics and conservative memory defaults.
- `main.go`: start diagnostics server, apply Go memory limit before stores open, use main-store count for Pebble budget, and emit startup budget logs.
- `storage/pebble.go`: use `IndexerParams` for cache/memtable sizing and expose store budget helpers.
- `blockchain/client.go`: add memory checkpoints around fetch, per-batch index, and height update.
- `blockchain/adapter_btc.go`: add BTC fetch/decode/convert checkpoints and release large local buffers before returning.
- `indexer/utxo.go`: add checkpoints around income, spend, commit, memory-cache eviction, and GC.
- `indexer/utxo_spend_query_test.go`: add regression coverage for spend-query results after reducing transient allocations.
- `storage/pebble_query_test.go`: add tests for `QueryUTXODetails` parse-on-read behavior.
- `docs/ISSUE_HIGUN_BTC_MEMORY_OOM.md`: append final root-cause and verification notes after production soak.

## Task Graph

Use this sequence:

1. Task 1: diagnostics configuration and memory snapshot readers.
2. Task 2: safe pprof/debug server and high-water profile dumps.
3. Task 3: cgroup-aware runtime memory budget and startup logs.
4. Task 4: bounded Pebble memory across all main stores.
5. Task 5: per-height memory checkpoints in BTC indexing.
6. Task 6: reduce `QueryUTXODetails` transient allocations.
7. Task 7: BTC block-fetch/index peak cleanup.
8. Task 8: runbook, production rollout, and 24-hour soak verification.

Run tasks sequentially. Tasks 3-7 touch shared startup/indexing/storage paths, so avoid parallel edits unless a reviewer has already merged the previous task.

---

### Task 1: Diagnostics Config And Memory Snapshot Readers

**Files:**
- Modify: `config/config.go`
- Modify: `config.yaml`
- Create: `diagnostics/memory.go`
- Create: `diagnostics/memory_test.go`

- [ ] **Step 1: Write failing config tests**

Create `diagnostics/memory_test.go` with the parsing tests first:

```go
package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCgroupV2Memory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "memory.current"), []byte("12345\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory.max"), []byte("98765\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory.events"), []byte("low 0\nhigh 2\nmax 3\noom 4\noom_kill 5\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := readCgroupMemory(root)
	if err != nil {
		t.Fatalf("readCgroupMemory: %v", err)
	}
	if got.Version != 2 || got.CurrentBytes != 12345 || got.LimitBytes != 98765 || got.OOMKillCount != 5 || got.FailCount != 3 {
		t.Fatalf("unexpected v2 cgroup snapshot: %+v", got)
	}
}

func TestReadCgroupV1Memory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "memory.usage_in_bytes"), []byte("111\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory.limit_in_bytes"), []byte("222\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory.failcnt"), []byte("7\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory.stat"), []byte("cache 33\nrss 44\nmapped_file 55\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := readCgroupMemory(root)
	if err != nil {
		t.Fatalf("readCgroupMemory: %v", err)
	}
	if got.Version != 1 || got.CurrentBytes != 111 || got.LimitBytes != 222 || got.FailCount != 7 || got.FileBytes != 33 || got.AnonBytes != 44 {
		t.Fatalf("unexpected v1 cgroup snapshot: %+v", got)
	}
}

func TestReadProcStatusRSS(t *testing.T) {
	status := []byte("Name:\tutxo_indexer\nVmHWM:\t  2048 kB\nVmRSS:\t  1024 kB\n")
	got := parseProcStatus(status)
	if got.RSSBytes != 1024*1024 || got.HighWaterBytes != 2048*1024 {
		t.Fatalf("unexpected proc snapshot: %+v", got)
	}
}
```

Run:

```bash
go test ./diagnostics
```

Expected: FAIL because package `diagnostics` and the parsing functions do not exist.

- [ ] **Step 2: Add diagnostics config fields**

Modify `config/config.go`:

```go
type DiagnosticsConfig struct {
	Enabled                    bool    `yaml:"enabled"`
	Bind                       string  `yaml:"bind"`
	PprofEnabled               bool    `yaml:"pprof_enabled"`
	ExpvarEnabled              bool    `yaml:"expvar_enabled"`
	SampleIntervalSeconds      int     `yaml:"sample_interval_seconds"`
	ProfileDir                 string  `yaml:"profile_dir"`
	HighWaterProfilePercents   []int   `yaml:"high_water_profile_percents"`
	MemoryLogEveryNBlocks      int     `yaml:"memory_log_every_n_blocks"`
	MemoryCheckpointMinPercent float64 `yaml:"memory_checkpoint_min_percent"`
}

type MemoryBudgetConfig struct {
	UseCgroupLimit          bool    `yaml:"use_cgroup_limit"`
	GoMemoryLimitPercent    float64 `yaml:"go_memory_limit_percent"`
	PebbleCachePercent      float64 `yaml:"pebble_cache_percent"`
	PebbleMemTablePercent   float64 `yaml:"pebble_memtable_percent"`
	ReservePercent          float64 `yaml:"reserve_percent"`
	MainStoreCount          int     `yaml:"main_store_count"`
	CgroupRoot              string  `yaml:"cgroup_root"`
	ForceCgroupLimitBytes   int64   `yaml:"force_cgroup_limit_bytes"`
}

type Config struct {
	// existing fields...
	Diagnostics  DiagnosticsConfig `yaml:"diagnostics"`
	MemoryBudget MemoryBudgetConfig `yaml:"memory_budget"`
}
```

In `LoadConfig`, add conservative defaults:

```go
Diagnostics: DiagnosticsConfig{
	Enabled:                    false,
	Bind:                       "127.0.0.1:6060",
	PprofEnabled:               true,
	ExpvarEnabled:              true,
	SampleIntervalSeconds:      30,
	ProfileDir:                 "data/diagnostics",
	HighWaterProfilePercents:   []int{70, 80, 90},
	MemoryLogEveryNBlocks:      1,
	MemoryCheckpointMinPercent: 0,
},
MemoryBudget: MemoryBudgetConfig{
	UseCgroupLimit:        true,
	GoMemoryLimitPercent:  65,
	PebbleCachePercent:    12,
	PebbleMemTablePercent: 8,
	ReservePercent:        15,
	MainStoreCount:        5,
	CgroupRoot:            "/sys/fs/cgroup",
},
```

- [ ] **Step 3: Add disabled-by-default config example**

Append this block to `config.yaml`:

```yaml
diagnostics:
  enabled: false
  bind: "127.0.0.1:6060"
  pprof_enabled: true
  expvar_enabled: true
  sample_interval_seconds: 30
  profile_dir: "data/diagnostics"
  high_water_profile_percents: [70, 80, 90]
  memory_log_every_n_blocks: 1
  memory_checkpoint_min_percent: 0

memory_budget:
  use_cgroup_limit: true
  go_memory_limit_percent: 65
  pebble_cache_percent: 12
  pebble_memtable_percent: 8
  reserve_percent: 15
  main_store_count: 5
  cgroup_root: "/sys/fs/cgroup"
  force_cgroup_limit_bytes: 0
```

- [ ] **Step 4: Implement snapshot readers**

Create `diagnostics/memory.go`:

```go
package diagnostics

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"
)

type RuntimeSnapshot struct {
	Time              time.Time `json:"time"`
	AllocBytes        uint64    `json:"allocBytes"`
	HeapAllocBytes    uint64    `json:"heapAllocBytes"`
	HeapInuseBytes    uint64    `json:"heapInuseBytes"`
	HeapIdleBytes     uint64    `json:"heapIdleBytes"`
	HeapReleasedBytes uint64    `json:"heapReleasedBytes"`
	SysBytes          uint64    `json:"sysBytes"`
	NumGC             uint32    `json:"numGC"`
	NumGoroutine      int       `json:"numGoroutine"`
	GoMemoryLimit     uint64    `json:"goMemoryLimit"`
}

type ProcSnapshot struct {
	RSSBytes       uint64 `json:"rssBytes"`
	HighWaterBytes uint64 `json:"highWaterBytes"`
}

type CgroupSnapshot struct {
	Version      int               `json:"version"`
	CurrentBytes uint64           `json:"currentBytes"`
	LimitBytes   uint64           `json:"limitBytes"`
	FileBytes    uint64           `json:"fileBytes"`
	AnonBytes    uint64           `json:"anonBytes"`
	FailCount    uint64           `json:"failCount"`
	OOMCount     uint64           `json:"oomCount"`
	OOMKillCount uint64           `json:"oomKillCount"`
	Stats        map[string]uint64 `json:"stats,omitempty"`
}

type Snapshot struct {
	Runtime RuntimeSnapshot `json:"runtime"`
	Proc    ProcSnapshot    `json:"proc"`
	Cgroup  CgroupSnapshot  `json:"cgroup"`
}

func Capture(cgroupRoot string) Snapshot {
	return Snapshot{
		Runtime: readRuntime(),
		Proc:    readProcSelfStatus(),
		Cgroup:  readCgroupBestEffort(cgroupRoot),
	}
}

func readRuntime() RuntimeSnapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return RuntimeSnapshot{
		Time:              time.Now(),
		AllocBytes:        mem.Alloc,
		HeapAllocBytes:    mem.HeapAlloc,
		HeapInuseBytes:    mem.HeapInuse,
		HeapIdleBytes:     mem.HeapIdle,
		HeapReleasedBytes: mem.HeapReleased,
		SysBytes:          mem.Sys,
		NumGC:             mem.NumGC,
		NumGoroutine:      runtime.NumGoroutine(),
		GoMemoryLimit:     readGoMemoryLimit(),
	}
}

func readGoMemoryLimit() uint64 {
	samples := []metrics.Sample{{Name: "/gc/gomemlimit:bytes"}}
	metrics.Read(samples)
	if len(samples) == 1 && samples[0].Value.Kind() == metrics.KindUint64 {
		return samples[0].Value.Uint64()
	}
	return 0
}

func readProcSelfStatus() ProcSnapshot {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return ProcSnapshot{}
	}
	return parseProcStatus(data)
}

func parseProcStatus(data []byte) ProcSnapshot {
	var snap ProcSnapshot
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "VmRSS:"):
			snap.RSSBytes = parseStatusKB(line)
		case strings.HasPrefix(line, "VmHWM:"):
			snap.HighWaterBytes = parseStatusKB(line)
		}
	}
	return snap
}

func parseStatusKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	kb, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return kb * 1024
}

func readCgroupBestEffort(root string) CgroupSnapshot {
	if root == "" {
		root = "/sys/fs/cgroup"
	}
	snap, err := readCgroupMemory(root)
	if err != nil {
		return CgroupSnapshot{}
	}
	return snap
}

func readCgroupMemory(root string) (CgroupSnapshot, error) {
	if _, err := os.Stat(filepath.Join(root, "memory.current")); err == nil {
		return readCgroupV2(root)
	}
	if _, err := os.Stat(filepath.Join(root, "memory.usage_in_bytes")); err == nil {
		return readCgroupV1(root)
	}
	return CgroupSnapshot{}, errors.New("no supported cgroup memory files found")
}

func readCgroupV2(root string) (CgroupSnapshot, error) {
	current, err := readUintFile(filepath.Join(root, "memory.current"))
	if err != nil {
		return CgroupSnapshot{}, err
	}
	limit, err := readLimitFile(filepath.Join(root, "memory.max"))
	if err != nil {
		return CgroupSnapshot{}, err
	}
	events := readKeyValueFile(filepath.Join(root, "memory.events"))
	stat := readKeyValueFile(filepath.Join(root, "memory.stat"))
	return CgroupSnapshot{
		Version:      2,
		CurrentBytes: current,
		LimitBytes:   limit,
		FileBytes:    stat["file"],
		AnonBytes:    stat["anon"],
		FailCount:    events["max"],
		OOMCount:     events["oom"],
		OOMKillCount: events["oom_kill"],
		Stats:        stat,
	}, nil
}

func readCgroupV1(root string) (CgroupSnapshot, error) {
	current, err := readUintFile(filepath.Join(root, "memory.usage_in_bytes"))
	if err != nil {
		return CgroupSnapshot{}, err
	}
	limit, err := readLimitFile(filepath.Join(root, "memory.limit_in_bytes"))
	if err != nil {
		return CgroupSnapshot{}, err
	}
	fail := readOptionalUint(filepath.Join(root, "memory.failcnt"))
	stat := readKeyValueFile(filepath.Join(root, "memory.stat"))
	return CgroupSnapshot{
		Version:      1,
		CurrentBytes: current,
		LimitBytes:   limit,
		FileBytes:    stat["cache"],
		AnonBytes:    stat["rss"],
		FailCount:    fail,
		Stats:        stat,
	}, nil
}

func readLimitFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	text := strings.TrimSpace(string(data))
	if text == "max" {
		return 0, nil
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	if value > 1<<60 {
		return 0, nil
	}
	return value, nil
}

func readUintFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return value, nil
}

func readOptionalUint(path string) uint64 {
	value, err := readUintFile(path)
	if err != nil {
		return 0
	}
	return value
}

func readKeyValueFile(path string) map[string]uint64 {
	result := make(map[string]uint64)
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			result[fields[0]] = value
		}
	}
	return result
}
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./config ./diagnostics
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add config/config.go config.yaml diagnostics/memory.go diagnostics/memory_test.go
git commit -m "feat: add higun memory diagnostics snapshots"
```

---

### Task 2: Safe Debug Server And High-Water Profile Dumps

**Files:**
- Create: `diagnostics/server.go`
- Create: `diagnostics/server_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing debug server tests**

Create `diagnostics/server_test.go`:

```go
package diagnostics

import (
	"net"
	"testing"
	"time"

	"github.com/metaid/utxo_indexer/config"
)

func TestValidateBindRejectsNonLoopback(t *testing.T) {
	err := validateDebugBind("0.0.0.0:6060")
	if err == nil {
		t.Fatal("expected non-loopback bind to be rejected")
	}
}

func TestValidateBindAcceptsLoopback(t *testing.T) {
	if err := validateDebugBind("127.0.0.1:6060"); err != nil {
		t.Fatalf("validateDebugBind loopback: %v", err)
	}
	if err := validateDebugBind("localhost:6060"); err != nil {
		t.Fatalf("validateDebugBind localhost: %v", err)
	}
}

func TestStartDisabledDoesNothing(t *testing.T) {
	stop, err := StartServer(config.DiagnosticsConfig{Enabled: false}, "")
	if err != nil {
		t.Fatalf("StartServer disabled: %v", err)
	}
	if stop != nil {
		t.Fatal("disabled StartServer returned non-nil stop")
	}
}

func TestStartServerOnLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	stop, err := StartServer(config.DiagnosticsConfig{
		Enabled:                    true,
		Bind:                       addr,
		PprofEnabled:               true,
		ExpvarEnabled:              true,
		SampleIntervalSeconds:      1,
		HighWaterProfilePercents:   []int{95},
		MemoryLogEveryNBlocks:      1,
		MemoryCheckpointMinPercent: 0,
	}, t.TempDir())
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	defer stop()
	time.Sleep(20 * time.Millisecond)
}
```

Run:

```bash
go test ./diagnostics
```

Expected: FAIL because `StartServer` and `validateDebugBind` do not exist.

- [ ] **Step 2: Implement localhost-only server**

Create `diagnostics/server.go`:

```go
package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	runtimepprof "runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/metaid/utxo_indexer/config"
)

var latestSnapshot atomicSnapshot

type atomicSnapshot struct {
	mu   sync.RWMutex
	data Snapshot
}

func (s *atomicSnapshot) Store(snapshot Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = snapshot
}

func (s *atomicSnapshot) Load() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

func StartServer(cfg config.DiagnosticsConfig, cgroupRoot string) (func(), error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := validateDebugBind(cfg.Bind); err != nil {
		return nil, err
	}
	if cfg.SampleIntervalSeconds <= 0 {
		cfg.SampleIntervalSeconds = 30
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/memory", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(latestSnapshot.Load())
	})
	if cfg.ExpvarEnabled {
		mux.Handle("/debug/vars", expvar.Handler())
	}
	if cfg.PprofEnabled {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	}

	server := &http.Server{Addr: cfg.Bind, Handler: mux}
	ctx, cancel := context.WithCancel(context.Background())
	go sampleLoop(ctx, cfg, cgroupRoot)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[Diagnostics] debug server stopped with error: %v", err)
		}
	}()

	return func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}, nil
}

func validateDebugBind(bind string) error {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return fmt.Errorf("invalid diagnostics bind %q: %w", bind, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("diagnostics bind must be loopback-only, got %q", bind)
	}
	return nil
}

func sampleLoop(ctx context.Context, cfg config.DiagnosticsConfig, cgroupRoot string) {
	ticker := time.NewTicker(time.Duration(cfg.SampleIntervalSeconds) * time.Second)
	defer ticker.Stop()
	dumped := make(map[int]bool)
	for {
		snapshot := Capture(cgroupRoot)
		latestSnapshot.Store(snapshot)
		maybeDumpProfiles(snapshot, cfg, dumped)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func maybeDumpProfiles(snapshot Snapshot, cfg config.DiagnosticsConfig, dumped map[int]bool) {
	if snapshot.Cgroup.LimitBytes == 0 || snapshot.Cgroup.CurrentBytes == 0 || cfg.ProfileDir == "" {
		return
	}
	percent := int(float64(snapshot.Cgroup.CurrentBytes) * 100 / float64(snapshot.Cgroup.LimitBytes))
	for _, mark := range cfg.HighWaterProfilePercents {
		if mark <= 0 || dumped[mark] || percent < mark {
			continue
		}
		if err := os.MkdirAll(cfg.ProfileDir, 0755); err != nil {
			log.Printf("[Diagnostics] create profile dir: %v", err)
			return
		}
		stamp := time.Now().Format("20060102-150405")
		heapPath := filepath.Join(cfg.ProfileDir, fmt.Sprintf("heap-%d-%s.pprof", mark, stamp))
		goroutinePath := filepath.Join(cfg.ProfileDir, fmt.Sprintf("goroutine-%d-%s.txt", mark, stamp))
		writeHeapProfile(heapPath)
		writeGoroutineProfile(goroutinePath)
		dumped[mark] = true
		log.Printf("[Diagnostics] dumped high-water profiles at %d%% cgroup memory: heap=%s goroutine=%s", mark, heapPath, goroutinePath)
	}
}

func writeHeapProfile(path string) {
	file, err := os.Create(path)
	if err != nil {
		log.Printf("[Diagnostics] create heap profile: %v", err)
		return
	}
	defer file.Close()
	if err := runtimepprof.WriteHeapProfile(file); err != nil {
		log.Printf("[Diagnostics] write heap profile: %v", err)
	}
}

func writeGoroutineProfile(path string) {
	file, err := os.Create(path)
	if err != nil {
		log.Printf("[Diagnostics] create goroutine profile: %v", err)
		return
	}
	defer file.Close()
	if profile := runtimepprof.Lookup("goroutine"); profile != nil {
		_ = profile.WriteTo(file, 2)
	}
}

func IsDiagnosticsPath(path string) bool {
	return strings.HasPrefix(path, "/debug/")
}
```

- [ ] **Step 3: Wire server startup and shutdown**

Modify `main.go` after `cfg, params := initConfig()` and before `initDb`:

```go
stopDiagnostics, err := diagnostics.StartServer(cfg.Diagnostics, cfg.MemoryBudget.CgroupRoot)
if err != nil {
	log.Fatalf("Failed to start diagnostics server: %v", err)
}
if stopDiagnostics != nil {
	defer stopDiagnostics()
	log.Printf("[Diagnostics] enabled at http://%s/debug/memory", cfg.Diagnostics.Bind)
}
```

Add import:

```go
github.com/metaid/utxo_indexer/diagnostics
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./diagnostics
CGO_ENABLED=0 go test ./config ./storage ./indexer
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add diagnostics/server.go diagnostics/server_test.go main.go
git commit -m "feat: expose safe higun diagnostics server"
```

---

### Task 3: Cgroup-Aware Runtime Memory Budget

**Files:**
- Modify: `config/auto.go`
- Create: `config/memory_budget_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing memory-budget tests**

Create `config/memory_budget_test.go`:

```go
package config

import "testing"

func TestChooseEffectiveMemoryUsesSmallerCgroupLimit(t *testing.T) {
	cfg := MemoryBudgetConfig{
		UseCgroupLimit:        true,
		ForceCgroupLimitBytes: 12 << 30,
		GoMemoryLimitPercent:  65,
		ReservePercent:        15,
	}
	got := ChooseMemoryBudget(32, cfg)
	if got.EffectiveMemoryBytes != 12<<30 {
		t.Fatalf("EffectiveMemoryBytes = %d, want 12GiB", got.EffectiveMemoryBytes)
	}
	wantGo := int64(float64(12<<30) * 0.65)
	if got.GoMemoryLimitBytes != wantGo {
		t.Fatalf("GoMemoryLimitBytes = %d, want %d", got.GoMemoryLimitBytes, wantGo)
	}
}

func TestChooseEffectiveMemoryFallsBackToConfiguredMemory(t *testing.T) {
	cfg := MemoryBudgetConfig{
		UseCgroupLimit:       true,
		GoMemoryLimitPercent: 65,
		ReservePercent:       15,
	}
	got := ChooseMemoryBudget(4, cfg)
	if got.EffectiveMemoryBytes != 4<<30 {
		t.Fatalf("EffectiveMemoryBytes = %d, want 4GiB", got.EffectiveMemoryBytes)
	}
}

func TestAutoConfigureKeepsExplicitBatchAndWorkerSettingsOutsideParams(t *testing.T) {
	params := AutoConfigure(SystemResources{
		CPUCores:   2,
		MemoryGB:   4,
		HighPerf:   false,
		ShardCount: 6,
	})
	if params.WorkerCount != 2 {
		t.Fatalf("WorkerCount = %d, want 2", params.WorkerCount)
	}
	if params.MaxBatchSizeMB != 4 {
		t.Fatalf("MaxBatchSizeMB = %d, want balanced 4", params.MaxBatchSizeMB)
	}
}
```

Run:

```bash
go test ./config
```

Expected: FAIL because `ChooseMemoryBudget` and the new budget fields do not exist.

- [ ] **Step 2: Add budget model and Go memory limit application**

Modify `config/auto.go`:

```go
import (
	"log"
	"runtime"
	"runtime/debug"
)

type MemoryBudget struct {
	ConfiguredMemoryBytes int64
	CgroupLimitBytes     int64
	EffectiveMemoryBytes int64
	GoMemoryLimitBytes   int64
	ReserveBytes         int64
	PebbleCacheBytes     int64
	PebbleMemTableBytes  int64
}

func ChooseMemoryBudget(memoryGB int, cfg MemoryBudgetConfig) MemoryBudget {
	configured := int64(memoryGB)
	if configured <= 0 {
		configured = 4
	}
	configuredBytes := configured << 30
	cgroupLimit := cfg.ForceCgroupLimitBytes
	effective := configuredBytes
	if cfg.UseCgroupLimit && cgroupLimit > 0 && cgroupLimit < effective {
		effective = cgroupLimit
	}
	goPercent := cfg.GoMemoryLimitPercent
	if goPercent <= 0 {
		goPercent = 65
	}
	reservePercent := cfg.ReservePercent
	if reservePercent <= 0 {
		reservePercent = 15
	}
	cachePercent := cfg.PebbleCachePercent
	if cachePercent <= 0 {
		cachePercent = 12
	}
	memTablePercent := cfg.PebbleMemTablePercent
	if memTablePercent <= 0 {
		memTablePercent = 8
	}
	return MemoryBudget{
		ConfiguredMemoryBytes: configuredBytes,
		CgroupLimitBytes:     cgroupLimit,
		EffectiveMemoryBytes: effective,
		GoMemoryLimitBytes:   int64(float64(effective) * goPercent / 100),
		ReserveBytes:         int64(float64(effective) * reservePercent / 100),
		PebbleCacheBytes:     int64(float64(effective) * cachePercent / 100),
		PebbleMemTableBytes:  int64(float64(effective) * memTablePercent / 100),
	}
}

func ApplyGoMemoryLimit(budget MemoryBudget) {
	if budget.GoMemoryLimitBytes <= 0 {
		return
	}
	previous := debug.SetMemoryLimit(budget.GoMemoryLimitBytes)
	log.Printf("[MemoryBudget] Go memory limit set: previous=%d chosen=%d effective=%d configured=%d cgroup=%d reserve=%d",
		previous,
		budget.GoMemoryLimitBytes,
		budget.EffectiveMemoryBytes,
		budget.ConfiguredMemoryBytes,
		budget.CgroupLimitBytes,
		budget.ReserveBytes,
	)
}
```

- [ ] **Step 3: Detect cgroup limit before choosing budget**

Add this helper in `config/auto.go`:

```go
func DetectCgroupLimitBytes(root string) int64 {
	if root == "" {
		root = "/sys/fs/cgroup"
	}
	limit := readCgroupLimitFile(filepath.Join(root, "memory.max"))
	if limit > 0 {
		return limit
	}
	return readCgroupLimitFile(filepath.Join(root, "memory.limit_in_bytes"))
}
```

Use small unexported `readCgroupLimitFile` with the same semantics as Task 1: `max` and huge v1 sentinel values return 0.

- [ ] **Step 4: Apply budget in startup**

Modify `main.go` in `initConfig` after loading `cfg` and before `AutoConfigure`:

```go
if cfg.MemoryBudget.ForceCgroupLimitBytes <= 0 && cfg.MemoryBudget.UseCgroupLimit {
	cfg.MemoryBudget.ForceCgroupLimitBytes = config.DetectCgroupLimitBytes(cfg.MemoryBudget.CgroupRoot)
}
budget := config.ChooseMemoryBudget(cfg.MemoryGB, cfg.MemoryBudget)
config.ApplyGoMemoryLimit(budget)
params = config.AutoConfigure(config.SystemResources{
	CPUCores:   cfg.CPUCores,
	MemoryGB:   int(maxInt64(1, budget.EffectiveMemoryBytes>>30)),
	HighPerf:   cfg.HighPerf,
	ShardCount: cfg.ShardCount,
})
params.MemoryBudget = budget
```

Add `MemoryBudget MemoryBudget` to `IndexerParams`.

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./config
CGO_ENABLED=0 go test ./config ./storage ./indexer
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add config/auto.go config/memory_budget_test.go main.go
git commit -m "feat: make higun memory budget cgroup aware"
```

---

### Task 4: Bound Pebble Memory Across Main Stores

**Files:**
- Modify: `config/auto.go`
- Modify: `storage/pebble.go`
- Create: `storage/pebble_budget_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing Pebble budget tests**

Create `storage/pebble_budget_test.go`:

```go
package storage

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

func TestPebbleBudgetDividesAcrossStoresAndShards(t *testing.T) {
	params := config.IndexerParams{
		MemoryBudget: config.MemoryBudget{
			PebbleCacheBytes:    120 << 20,
			PebbleMemTableBytes: 240 << 20,
		},
		PebbleMainStoreCount: 5,
	}
	got := ComputePebbleOpenOptions(params, 6)
	if got.CacheSizeBytes != 24<<20 {
		t.Fatalf("CacheSizeBytes = %d, want 24MiB per store", got.CacheSizeBytes)
	}
	if got.MemTableSizeBytes != 8<<20 {
		t.Fatalf("MemTableSizeBytes = %d, want 8MiB per store shard", got.MemTableSizeBytes)
	}
}

func TestPebbleBudgetHasSafeMinimums(t *testing.T) {
	params := config.IndexerParams{
		MemoryBudget: config.MemoryBudget{
			PebbleCacheBytes:    8 << 20,
			PebbleMemTableBytes: 8 << 20,
		},
		PebbleMainStoreCount: 5,
	}
	got := ComputePebbleOpenOptions(params, 6)
	if got.CacheSizeBytes < 4<<20 {
		t.Fatalf("CacheSizeBytes = %d, want at least 4MiB", got.CacheSizeBytes)
	}
	if got.MemTableSizeBytes < 4<<20 {
		t.Fatalf("MemTableSizeBytes = %d, want at least 4MiB", got.MemTableSizeBytes)
	}
}
```

Run:

```bash
go test ./storage
```

Expected: FAIL because `ComputePebbleOpenOptions` and `PebbleMainStoreCount` do not exist.

- [ ] **Step 2: Add explicit Pebble budget fields**

Modify `config.IndexerParams` in `config/auto.go`:

```go
MemoryBudget         MemoryBudget
PebbleMainStoreCount int
```

Set the value in `main.go` before stores open:

```go
if cfg.MemoryBudget.MainStoreCount <= 0 {
	cfg.MemoryBudget.MainStoreCount = 5
}
params.PebbleMainStoreCount = cfg.MemoryBudget.MainStoreCount
```

Use `5` for current main runtime stores:

```text
utxo
income
spend
address_balance
balance_rank
```

`meta` remains small and separate.

- [ ] **Step 3: Implement budget helper**

Modify `storage/pebble.go`:

```go
const (
	minPebbleCacheBytes    int64  = 4 << 20
	minPebbleMemTableBytes uint64 = 4 << 20
)

func ComputePebbleOpenOptions(params config.IndexerParams, shardCount int) PebbleOpenOptions {
	storeCount := params.PebbleMainStoreCount
	if storeCount <= 0 {
		storeCount = 1
	}
	if shardCount <= 0 {
		shardCount = 1
	}

	cacheBytes := params.MemoryBudget.PebbleCacheBytes / int64(storeCount)
	if cacheBytes < minPebbleCacheBytes {
		cacheBytes = minPebbleCacheBytes
	}

	memTableBytes := params.MemoryBudget.PebbleMemTableBytes / int64(storeCount*shardCount)
	if memTableBytes < int64(minPebbleMemTableBytes) {
		memTableBytes = int64(minPebbleMemTableBytes)
	}

	return PebbleOpenOptions{
		CacheSizeBytes:              cacheBytes,
		MemTableSizeBytes:           uint64(memTableBytes),
		MemTableStopWritesThreshold: defaultPebbleMemTableStopWritesThreshold,
		MaxConcurrentCompactions:    defaultPebbleMaxConcurrentCompactions,
		MaxOpenFiles:                defaultPebbleMaxOpenFiles,
	}
}
```

- [ ] **Step 4: Use budget helper for all normal store opens**

Modify `NewPebbleStore`:

```go
func NewPebbleStore(params config.IndexerParams, dataDir string, storeType StoreType, shardCount int) (*PebbleStore, error) {
	return NewPebbleStoreWithOptions(params, dataDir, storeType, shardCount, ComputePebbleOpenOptions(params, shardCount))
}
```

Modify `newPebbleDBOptions` to use `params` when explicit options are absent:

```go
func newPebbleDBOptions(params config.IndexerParams, openOpts PebbleOpenOptions) (*pebble.Options, *pebble.Cache) {
	if openOpts.CacheSizeBytes <= 0 || openOpts.MemTableSizeBytes == 0 {
		budgeted := ComputePebbleOpenOptions(params, 1)
		if openOpts.CacheSizeBytes <= 0 {
			openOpts.CacheSizeBytes = budgeted.CacheSizeBytes
		}
		if openOpts.MemTableSizeBytes == 0 {
			openOpts.MemTableSizeBytes = budgeted.MemTableSizeBytes
		}
	}
	// keep existing option construction
}
```

Then avoid double-dividing by passing computed options from `NewPebbleStore`.

- [ ] **Step 5: Emit auditable startup logs**

In `NewPebbleStoreWithOptions`, before opening shards:

```go
log.Printf("[PebbleBudget] store=%s shards=%d cache_per_store=%d memtable_per_shard=%d stop_writes=%d max_batch=%d",
	storeType.String(),
	shardCount,
	openOpts.CacheSizeBytes,
	openOpts.MemTableSizeBytes,
	openOpts.MemTableStopWritesThreshold,
	maxBatchSize,
)
```

Add a `String()` method for `StoreType` with names used in `main.go`.

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./storage
CGO_ENABLED=0 go test ./config ./storage ./indexer
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add config/auto.go storage/pebble.go storage/pebble_budget_test.go main.go
git commit -m "fix: bound pebble memory for higun btc"
```

---

### Task 5: Per-Height Memory Checkpoints

**Files:**
- Create: `diagnostics/checkpoint.go`
- Create: `diagnostics/checkpoint_test.go`
- Modify: `blockchain/client.go`
- Modify: `blockchain/adapter_btc.go`
- Modify: `indexer/utxo.go`

- [ ] **Step 1: Write failing checkpoint tests**

Create `diagnostics/checkpoint_test.go`:

```go
package diagnostics

import (
	"strings"
	"testing"
)

func TestCheckpointLineContainsHeightPhaseAndMemory(t *testing.T) {
	snapshot := Snapshot{
		Runtime: RuntimeSnapshot{AllocBytes: 10 << 20, HeapInuseBytes: 20 << 20, NumGC: 3, NumGoroutine: 12},
		Proc:    ProcSnapshot{RSSBytes: 30 << 20, HighWaterBytes: 40 << 20},
		Cgroup:  CgroupSnapshot{CurrentBytes: 50 << 20, LimitBytes: 100 << 20, FileBytes: 5 << 20, AnonBytes: 45 << 20, FailCount: 2},
	}
	line := FormatCheckpoint(Checkpoint{
		Height:     952908,
		Phase:      "after_spend",
		TxCount:    3000,
		InputCount: 9000,
		OutputCount: 6000,
		ElapsedMS:  1234,
		Snapshot:   snapshot,
	})
	for _, want := range []string{"height=952908", "phase=after_spend", "tx=3000", "inputs=9000", "outputs=6000", "heap_alloc_mb=10", "rss_mb=30", "cgroup_pct=50.0"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q missing %q", line, want)
		}
	}
}
```

Run:

```bash
go test ./diagnostics
```

Expected: FAIL because `Checkpoint` and `FormatCheckpoint` do not exist.

- [ ] **Step 2: Implement checkpoint formatter**

Create `diagnostics/checkpoint.go`:

```go
package diagnostics

import "fmt"

type Checkpoint struct {
	Height      int
	Phase       string
	TxCount     int
	InputCount  int
	OutputCount int
	ElapsedMS   int64
	Snapshot    Snapshot
}

func FormatCheckpoint(c Checkpoint) string {
	cgroupPct := 0.0
	if c.Snapshot.Cgroup.LimitBytes > 0 {
		cgroupPct = float64(c.Snapshot.Cgroup.CurrentBytes) * 100 / float64(c.Snapshot.Cgroup.LimitBytes)
	}
	return fmt.Sprintf(
		"[MemoryCheckpoint] height=%d phase=%s tx=%d inputs=%d outputs=%d elapsed_ms=%d heap_alloc_mb=%d heap_inuse_mb=%d rss_mb=%d hwm_mb=%d cgroup_mb=%d cgroup_limit_mb=%d cgroup_pct=%.1f cgroup_file_mb=%d cgroup_anon_mb=%d cgroup_fail=%d gc=%d goroutines=%d",
		c.Height,
		c.Phase,
		c.TxCount,
		c.InputCount,
		c.OutputCount,
		c.ElapsedMS,
		c.Snapshot.Runtime.HeapAllocBytes>>20,
		c.Snapshot.Runtime.HeapInuseBytes>>20,
		c.Snapshot.Proc.RSSBytes>>20,
		c.Snapshot.Proc.HighWaterBytes>>20,
		c.Snapshot.Cgroup.CurrentBytes>>20,
		c.Snapshot.Cgroup.LimitBytes>>20,
		cgroupPct,
		c.Snapshot.Cgroup.FileBytes>>20,
		c.Snapshot.Cgroup.AnonBytes>>20,
		c.Snapshot.Cgroup.FailCount,
		c.Snapshot.Runtime.NumGC,
		c.Snapshot.Runtime.NumGoroutine,
	)
}
```

- [ ] **Step 3: Add low-overhead checkpoint calls**

Add import where needed:

```go
"github.com/metaid/utxo_indexer/diagnostics"
```

In `blockchain/adapter_btc.go`, add log calls:

```go
log.Println(diagnostics.FormatCheckpoint(diagnostics.Checkpoint{
	Height:    int(height),
	Phase:     "btc_after_verbose1",
	TxCount:   len(verbose1.Tx),
	ElapsedMS: time.Since(t2).Milliseconds(),
	Snapshot:  diagnostics.Capture(config.GlobalConfig.MemoryBudget.CgroupRoot),
}))
```

After raw block decode:

```go
log.Println(diagnostics.FormatCheckpoint(diagnostics.Checkpoint{
	Height:    int(height),
	Phase:     "btc_after_decode",
	TxCount:   len(msgBlock.Transactions),
	ElapsedMS: time.Since(t4).Milliseconds(),
	Snapshot:  diagnostics.Capture(config.GlobalConfig.MemoryBudget.CgroupRoot),
}))
```

In `blockchain/client.go`, add around the adapter `GetBlock` and after each `IndexBlock` call:

```go
fetchStart := time.Now()
allBlock, err := c.adapter.GetBlock(int64(height))
log.Println(diagnostics.FormatCheckpoint(diagnostics.Checkpoint{
	Height:    height,
	Phase:     "after_block_fetch",
	TxCount:   len(allBlock.Transactions),
	ElapsedMS: time.Since(fetchStart).Milliseconds(),
	Snapshot:  diagnostics.Capture(config.GlobalConfig.MemoryBudget.CgroupRoot),
}))
```

In `indexer/utxo.go`, add after income, spend, balance update, and height commit:

```go
log.Println(diagnostics.FormatCheckpoint(diagnostics.Checkpoint{
	Height:      block.Height,
	Phase:       "after_income",
	TxCount:     len(block.Transactions),
	OutputCount: outCnt,
	ElapsedMS:   incomeTime.Milliseconds(),
	Snapshot:    diagnostics.Capture(config.GlobalConfig.MemoryBudget.CgroupRoot),
}))
```

Use phase names:

```text
after_income
after_spend
after_balance_index
after_height_update
after_gc
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./diagnostics
CGO_ENABLED=0 go test ./config ./storage ./indexer
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add diagnostics/checkpoint.go diagnostics/checkpoint_test.go blockchain/client.go blockchain/adapter_btc.go indexer/utxo.go
git commit -m "feat: add btc indexing memory checkpoints"
```

---

### Task 6: Reduce `QueryUTXODetails` Transient Allocations

**Files:**
- Modify: `storage/pebble.go`
- Create: `storage/pebble_query_test.go`
- Create or modify: `indexer/utxo_spend_query_test.go`

- [ ] **Step 1: Write failing behavior test for parse-on-read details**

Create `storage/pebble_query_test.go`:

```go
package storage

import (
	"testing"

	"github.com/metaid/utxo_indexer/config"
)

func TestQueryUTXODetailsReturnsOnlyRequestedOutpoints(t *testing.T) {
	store, err := NewPebbleStore(config.IndexerParams{}, t.TempDir(), StoreTypeUTXO, 2)
	if err != nil {
		t.Fatalf("NewPebbleStore: %v", err)
	}
	defer store.Close()

	if err := store.Put([]byte("tx1"), []byte(",addr0@100@1@0,addr1@200@1@1,addr2@300@1@2")); err != nil {
		t.Fatalf("Put tx1: %v", err)
	}
	if err := store.Put([]byte("tx2"), []byte(",addrA@400@1@0")); err != nil {
		t.Fatalf("Put tx2: %v", err)
	}

	outpoints := []string{"tx1:1", "tx2:0", "missing:0"}
	got, err := store.QueryUTXODetails(&outpoints)
	if err != nil {
		t.Fatalf("QueryUTXODetails: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %+v", len(got), got)
	}
	if got["tx1:1"].Address != "addr1" || got["tx1:1"].Amount != 200 {
		t.Fatalf("tx1:1 = %+v", got["tx1:1"])
	}
	if got["tx2:0"].Address != "addrA" || got["tx2:0"].Amount != 400 {
		t.Fatalf("tx2:0 = %+v", got["tx2:0"])
	}
}
```

Run:

```bash
go test ./storage -run TestQueryUTXODetailsReturnsOnlyRequestedOutpoints -count=1
```

Expected before refactor: PASS. Keep this as a guard before changing internals.

- [ ] **Step 2: Write allocation benchmark**

In `storage/pebble_query_test.go`, add:

```go
func BenchmarkQueryUTXODetailsRepeatedOutputs(b *testing.B) {
	store, err := NewPebbleStore(config.IndexerParams{}, b.TempDir(), StoreTypeUTXO, 2)
	if err != nil {
		b.Fatalf("NewPebbleStore: %v", err)
	}
	defer store.Close()

	value := ",addr0@100@1@0,addr1@200@1@1,addr2@300@1@2,addr3@400@1@3"
	for i := 0; i < 1000; i++ {
		key := []byte("tx" + strconv.Itoa(i))
		if err := store.Put(key, []byte(value)); err != nil {
			b.Fatalf("Put: %v", err)
		}
	}
	outpoints := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		outpoints = append(outpoints, "tx"+strconv.Itoa(i)+":2")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		got, err := store.QueryUTXODetails(&outpoints)
		if err != nil {
			b.Fatal(err)
		}
		if len(got) != 1000 {
			b.Fatalf("len(got) = %d", len(got))
		}
	}
}
```

Add `strconv` import.

Run:

```bash
go test ./storage -run '^$' -bench BenchmarkQueryUTXODetailsRepeatedOutputs -benchmem -count=1
```

Expected: records current allocation baseline.

- [ ] **Step 3: Replace cache-copy implementation with parse-on-read**

In `storage/pebble.go`, replace the current `QueryUTXODetails` implementation with:

```go
func (s *PebbleStore) QueryUTXODetails(outpoints *[]string) (map[string]UTXODetail, error) {
	if outpoints == nil || len(*outpoints) == 0 {
		return map[string]UTXODetail{}, nil
	}

	type requestedOutput struct {
		indexStr string
		fullKey  string
	}
	requestsByTxID := make(map[string][]requestedOutput, len(*outpoints))
	for _, op := range *outpoints {
		colonIdx := strings.LastIndexByte(op, ':')
		if colonIdx <= 0 || colonIdx == len(op)-1 {
			continue
		}
		txid := op[:colonIdx]
		requestsByTxID[txid] = append(requestsByTxID[txid], requestedOutput{
			indexStr: op[colonIdx+1:],
			fullKey:  op,
		})
	}
	if len(requestsByTxID) == 0 {
		return map[string]UTXODetail{}, nil
	}

	type job struct {
		txid     string
		requests []requestedOutput
	}
	type result struct {
		key    string
		detail UTXODetail
	}

	concurrency := runtime.NumCPU()
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 16 {
		concurrency = 16
	}

	jobs := make(chan job, concurrency*2)
	results := make(chan result, concurrency*8)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				db := s.getShard(item.txid)
				value, closer, err := db.Get([]byte(item.txid))
				if err != nil {
					if err == pebble.ErrNotFound {
						continue
					}
					s.dbGetErrors.Add(1)
					select {
					case errCh <- err:
					default:
					}
					continue
				}
				for _, req := range item.requests {
					detail, ok := extractUTXODetailFromValue(value, req.indexStr)
					if ok && detail.Address != "" {
						results <- result{key: req.fullKey, detail: detail}
					}
				}
				closer.Close()
			}
		}()
	}

	go func() {
		for txid, requests := range requestsByTxID {
			jobs <- job{txid: txid, requests: requests}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	final := make(map[string]UTXODetail, len(*outpoints))
	for item := range results {
		final[item.key] = item.detail
	}

	select {
	case err := <-errCh:
		if len(final) == 0 {
			return nil, err
		}
	default:
	}
	return final, nil
}
```

Important constraints:

- Do not keep `value` after `closer.Close()`.
- Do not copy complete Pebble values into a temporary global cache.
- Keep missing outpoints non-fatal, matching current caller behavior.
- Keep returned map keyed by full `txid:index`.

- [ ] **Step 4: Run behavior and benchmark**

Run:

```bash
go test ./storage -run TestQueryUTXODetailsReturnsOnlyRequestedOutpoints -count=1
go test ./storage -run '^$' -bench BenchmarkQueryUTXODetailsRepeatedOutputs -benchmem -count=1
CGO_ENABLED=0 go test ./storage ./indexer
```

Expected:

- Behavior test PASS.
- Benchmark allocation count and bytes should be lower than or no worse than baseline.
- Package tests PASS.

- [ ] **Step 5: Commit**

```bash
git add storage/pebble.go storage/pebble_query_test.go indexer/utxo_spend_query_test.go
git commit -m "fix: reduce higun utxo spend query allocations"
```

---

### Task 7: BTC Block-Fetch And Index Peak Cleanup

**Files:**
- Modify: `blockchain/adapter_btc.go`
- Modify: `blockchain/client.go`
- Modify: `indexer/utxo.go`

- [ ] **Step 1: Add cleanup regression test for BTC adapter conversion**

Create `blockchain/adapter_btc_memory_test.go`:

```go
package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

func TestConvertToIndexerBlockDoesNotPopulateArchiveMapsWhenDisabled(t *testing.T) {
	adapter := &BTCAdapter{params: &chaincfg.MainNetParams}
	block := wire.NewMsgBlock(&wire.BlockHeader{})
	block.AddTransaction(wire.NewMsgTx(2))

	got, err := adapter.convertToIndexerBlock(block, 1, "hash", 123)
	if err != nil {
		t.Fatalf("convertToIndexerBlock: %v", err)
	}
	if len(got.Transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(got.Transactions))
	}
	if got.UtxoData != nil || got.IncomeData != nil || got.SpendData != nil {
		t.Fatalf("archive maps should be nil for BTC normal indexing: %+v", got)
	}
}
```

Run:

```bash
go test ./blockchain -run TestConvertToIndexerBlockDoesNotPopulateArchiveMapsWhenDisabled -count=1
```

Expected: FAIL because archive maps are currently allocated unconditionally.

- [ ] **Step 2: Stop allocating unused archive maps when block files are disabled**

In `blockchain/adapter_btc.go`, replace `allBlock` creation in `getBlockByTxHashes` and `convertToIndexerBlock`:

```go
allBlock := &indexer.Block{
	Height:    height,
	BlockHash: blockHash,
	BlockTime: blockTime,
}
if config.GlobalConfig != nil && config.GlobalConfig.BlockFilesEnabled {
	allBlock.UtxoData = make(map[string][]string)
	allBlock.IncomeData = make(map[string][]string)
	allBlock.SpendData = make(map[string][]string)
}
```

Use the correct hash/time variables in each function:

```go
BlockHash: verbose1.Hash
BlockTime: verbose1.Time
```

for `getBlockByTxHashes`.

- [ ] **Step 3: Release raw block buffers before return**

In `BTCAdapter.GetBlock`, after successful conversion:

```go
blockHex = ""
blockBytes = nil
msgBlock.Transactions = nil
msgBlock = nil
runtime.GC()
```

Only force GC when the block size is near the large-block threshold or memory pressure is high:

```go
if verbose1.Size > threshold/2 {
	runtime.GC()
}
```

- [ ] **Step 4: Release adapter full-block references after indexing**

In `blockchain/client.go`, after indexing all parts and before logging:

```go
txCount := len(allBlock.Transactions)
allBlock.Transactions = nil
if allBlock.UtxoData != nil {
	allBlock.UtxoData = nil
}
if allBlock.IncomeData != nil {
	allBlock.IncomeData = nil
}
if allBlock.SpendData != nil {
	allBlock.SpendData = nil
}
```

Keep `BlockHash`, `BlockTime`, and `txCount` in local variables before clearing.

- [ ] **Step 5: Guard `SaveBlockFile` against nil archive maps**

In `indexer/utxo.go`, update `SaveBlockFile`:

```go
if allBlock == nil || !config.GlobalConfig.BlockFilesEnabled {
	return
}
if fileType == "utxo" && allBlock.UtxoData == nil {
	return
}
if fileType == "spend" && allBlock.SpendData == nil {
	return
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
CGO_ENABLED=0 go test ./blockchain ./indexer ./storage
```

Expected: PASS. If `./blockchain` hangs on existing long-running tests, run the new targeted test plus related BTC tests:

```bash
CGO_ENABLED=0 go test ./blockchain -run 'TestConvertToIndexerBlockDoesNotPopulateArchiveMapsWhenDisabled|Test.*BTC|Test.*TxDetail' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add blockchain/adapter_btc.go blockchain/client.go blockchain/adapter_btc_memory_test.go indexer/utxo.go
git commit -m "fix: reduce btc block indexing peak memory"
```

---

### Task 8: Runbook, Production Soak, And Final Evidence

**Files:**
- Create: `docs/higun-btc-oom-runbook.md`
- Modify: `docs/ISSUE_HIGUN_BTC_MEMORY_OOM.md`

- [ ] **Step 1: Create rollout runbook**

Create `docs/higun-btc-oom-runbook.md`:

````markdown
# Higun BTC OOM Rollout Runbook

## Goal

Deploy the Higun BTC memory fix to `8.217.251.101` and prove `higun_btc` can run at least 24 hours without Docker cgroup OOM.

## Build

From `/Users/tusm/Documents/MetaID_Projects/higun`:

```bash
make linux
test -x utxo_indexer
sha256sum utxo_indexer
```

## Preflight On 8.217.251.101

```bash
ssh root@8.217.251.101 '
set -euo pipefail
docker inspect higun_btc --format "status={{.State.Status}} oom={{.State.OOMKilled}} restart={{.RestartCount}} memory={{.HostConfig.Memory}} swap={{.HostConfig.MemorySwap}}"
docker stats --no-stream --format "{{.Name}} {{.MemUsage}} {{.MemPerc}} {{.CPUPerc}}" higun_btc
docker inspect higun_btc --format "{{json .Mounts}}"
curl -fsS http://127.0.0.1:8066/cleanedHeight/get
curl -sS -o /tmp/higun_btc_8085.out -w "%{http_code}\n" http://127.0.0.1:8085/address/btc-balance
tail -n 20 /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv || true
'
```

If SSH authentication is not available, stop and get access before deployment.

## Backup

```bash
ssh root@8.217.251.101 '
set -euo pipefail
TS=$(date +%Y%m%d_%H%M%S)
BK=/date/higun_btc/backups/oom-fix-$TS
mkdir -p "$BK"
docker inspect higun_btc > "$BK/higun_btc.inspect.before.json"
docker logs --tail 1000 higun_btc > "$BK/higun_btc.logs.before.txt" 2>&1 || true
cp -a /date/higun_btc/config.yaml "$BK/config.yaml.before"
cp -a /date/higun_btc/utxo_indexer "$BK/utxo_indexer.before"
sha256sum /date/higun_btc/utxo_indexer > "$BK/utxo_indexer.before.sha256"
echo "$BK"
'
```

## Upload And Restart

```bash
scp utxo_indexer root@8.217.251.101:/date/higun_btc/utxo_indexer.new
ssh root@8.217.251.101 '
set -euo pipefail
cd /date/higun_btc
chmod +x utxo_indexer.new
sha256sum utxo_indexer utxo_indexer.new
mv utxo_indexer "utxo_indexer.pre-oom-fix.$(date +%Y%m%d_%H%M%S)"
mv utxo_indexer.new utxo_indexer
docker restart higun_btc
sleep 10
docker inspect higun_btc --format "status={{.State.Status}} oom={{.State.OOMKilled}} restart={{.RestartCount}} started={{.State.StartedAt}}"
curl -fsS http://127.0.0.1:8066/cleanedHeight/get
curl -sS http://127.0.0.1:6060/debug/memory | head -c 1000
'
```

## Soak Checks

Run after 15 minutes, 1 hour, 6 hours, and 24 hours:

```bash
ssh root@8.217.251.101 '
set -euo pipefail
date
docker inspect higun_btc --format "status={{.State.Status}} oom={{.State.OOMKilled}} restart={{.RestartCount}} memory={{.HostConfig.Memory}}"
docker stats --no-stream --format "{{.Name}} {{.MemUsage}} {{.MemPerc}} {{.CPUPerc}}" higun_btc
curl -fsS http://127.0.0.1:8066/cleanedHeight/get
curl -fsS http://127.0.0.1:6060/debug/memory > /tmp/higun_btc_debug_memory.json
tail -n 30 /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv || true
journalctl -k --since "24 hours ago" --no-pager | grep -Ei "higun_btc|utxo_indexer|oom|killed process" | tail -n 50 || true
find /date/higun_btc/diagnostics -type f -maxdepth 1 -mtime -1 -print 2>/dev/null || true
'
```

## Acceptance

- `docker inspect higun_btc` reports `OOMKilled=false`.
- `RestartCount` does not increase during the soak.
- `/cleanedHeight/get` remains responsive.
- `8066` and `8085` remain responsive.
- cgroup memory reaches a steady state or stays below 85 percent of limit.
- If cgroup cache remains high, `/debug/memory` separates file/cache pressure from anonymous RSS.
- If profiles are dumped, heap/goroutine artifacts are copied into the issue record.

## Rollback

```bash
ssh root@8.217.251.101 '
set -euo pipefail
cd /date/higun_btc
LATEST_BACKUP=$(ls -td /date/higun_btc/backups/oom-fix-* | head -n 1)
cp -a "$LATEST_BACKUP/utxo_indexer.before" /date/higun_btc/utxo_indexer
chmod +x /date/higun_btc/utxo_indexer
docker restart higun_btc
sleep 10
docker inspect higun_btc --format "status={{.State.Status}} oom={{.State.OOMKilled}} restart={{.RestartCount}}"
curl -fsS http://127.0.0.1:8066/cleanedHeight/get
'
```
````

- [ ] **Step 2: Build and local verification**

Run:

```bash
go test ./config ./diagnostics
CGO_ENABLED=0 go test ./storage ./indexer
CGO_ENABLED=0 go test ./blockchain -run 'TestConvertToIndexerBlockDoesNotPopulateArchiveMapsWhenDisabled|Test.*BTC|Test.*TxDetail' -count=1
make linux
test -x utxo_indexer
```

Expected: PASS and Linux binary produced.

- [ ] **Step 3: Production preflight**

Run the preflight section from `docs/higun-btc-oom-runbook.md`.

Expected:

- Existing binary and config backed up.
- Current Docker memory limit, restart count, and OOMKilled state recorded.
- Current `/cleanedHeight/get` recorded.

- [ ] **Step 4: Production deploy**

Run the upload and restart section from `docs/higun-btc-oom-runbook.md`.

Expected:

- `higun_btc` restarts once.
- `OOMKilled=false` immediately after restart.
- `/cleanedHeight/get` returns HTTP 200.
- `http://127.0.0.1:6060/debug/memory` returns JSON.

- [ ] **Step 5: Soak and evidence capture**

Run soak checks at:

```text
T+15m
T+1h
T+6h
T+24h
```

Expected:

- No new kernel OOM for `utxo_indexer`.
- `RestartCount` unchanged after deployment restart.
- Memory trend stops monotonically climbing to cgroup limit.
- pprof artifacts exist if high-water marks were crossed.

- [ ] **Step 6: Update issue document**

Generate the verification section from collected evidence:

```bash
cat >> docs/ISSUE_HIGUN_BTC_MEMORY_OOM.md <<'EOF'

## Fix Verification

- Build command: `make linux`
- Deployment host: `8.217.251.101`
- Container: `higun_btc`
- Docker memory limit bytes: `12884901888`
- Soak requirement: `24h without cgroup OOM`
- Acceptance source files:
  - `/tmp/higun_btc_debug_memory.json`
  - `/date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv`
  - `journalctl -k --since "24 hours ago"`
- Root-cause classification must be one of:
  - Go heap/object retention
  - Pebble/native/cache behavior
  - cgroup-aware memory limit mismatch
  - per-block/per-height transient allocation pressure
  - mempool/ZMQ pressure
  - combination

| Metric | Before | After |
| --- | ---: | ---: |
| Docker memory limit bytes | 12884901888 | 12884901888 |
| Peak cgroup current bytes | 12884897792 | from `/tmp/higun_btc_debug_memory.json` |
| Peak process RSS bytes | 10649664kB HWM from incident note | from `/tmp/higun_btc_debug_memory.json` |
| Go heap alloc bytes | unavailable before fix | from `/tmp/higun_btc_debug_memory.json` |
| cgroup file/cache bytes | 11680473088 max from incident note | from `/tmp/higun_btc_debug_memory.json` |
| OOM kill count delta | incident baseline | 0 during soak |

Do not claim a final root cause until the "After" column is replaced with numeric values from the soak evidence.
EOF
```

- [ ] **Step 7: Commit runbook and verification notes**

Before production verification:

```bash
git add docs/higun-btc-oom-runbook.md
git commit -m "docs: add higun btc oom rollout runbook"
```

After production verification:

```bash
git add docs/ISSUE_HIGUN_BTC_MEMORY_OOM.md
git commit -m "docs: record higun btc oom verification"
```

---

## Self-Review

Spec coverage:

- Production diagnostics: Tasks 1, 2, and 5.
- cgroup-aware memory budget: Task 3.
- Pebble memory bounding: Task 4.
- retained/transient allocation investigation and reduction: Tasks 5, 6, and 7.
- production-like soak and acceptance: Task 8.
- compile reference from `/Users/tusm/Documents/MetaID_Projects/servers/higun.md`: Task 8 uses `make linux`.

Placeholder scan:

- No unresolved implementation placeholders remain.
- Production-specific unknowns are handled by concrete preflight commands that stop if SSH/path access is unavailable.

Type consistency:

- `config.DiagnosticsConfig`, `config.MemoryBudgetConfig`, and `config.MemoryBudget` are defined before use.
- `diagnostics.Snapshot`, `diagnostics.Checkpoint`, and `diagnostics.FormatCheckpoint` are defined before instrumentation tasks.
- `storage.ComputePebbleOpenOptions` returns existing `PebbleOpenOptions`.

Risk controls:

- The first production-affecting change is observability, not behavior.
- Runtime memory limits are logged and auditable.
- Pebble budgets are explicit before query/index allocation changes.
- Rollback restores the exact previous binary from backup.
