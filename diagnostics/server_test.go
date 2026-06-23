package diagnostics

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/metaid/utxo_indexer/config"
)

func TestValidateDebugBindRejectsNonLoopback(t *testing.T) {
	if err := validateDebugBind("0.0.0.0:6060"); err == nil {
		t.Fatal("validateDebugBind() error = nil, want non-loopback rejection")
	}
}

func TestValidateDebugBindAcceptsLoopbackIP(t *testing.T) {
	if err := validateDebugBind("127.0.0.1:6060"); err != nil {
		t.Fatalf("validateDebugBind() error = %v, want nil", err)
	}
}

func TestValidateDebugBindAcceptsLocalhost(t *testing.T) {
	if err := validateDebugBind("localhost:6060"); err != nil {
		t.Fatalf("validateDebugBind() error = %v, want nil", err)
	}
}

func TestStartServerDisabledReturnsNilStopAndNilError(t *testing.T) {
	stop, err := StartServer(config.DiagnosticsConfig{Enabled: false}, "")
	if err != nil {
		t.Fatalf("StartServer() error = %v, want nil", err)
	}
	if stop != nil {
		t.Fatal("StartServer() stop != nil, want nil when disabled")
	}
}

func TestStartServerOnLoopbackFreePort(t *testing.T) {
	addr := reserveLoopbackAddress(t)
	profileDir := t.TempDir()

	stop, err := StartServer(config.DiagnosticsConfig{
		Enabled:                    true,
		Bind:                       addr,
		PprofEnabled:               true,
		ExpvarEnabled:              true,
		SampleIntervalSeconds:      1,
		ProfileDir:                 profileDir,
		HighWaterProfilePercents:   []int{95},
		MemoryLogEveryNBlocks:      1,
		MemoryCheckpointMinPercent: 0,
	}, "")
	if err != nil {
		t.Fatalf("StartServer() error = %v, want nil", err)
	}
	if stop == nil {
		t.Fatal("StartServer() stop = nil, want stop function")
	}
	defer stop()

	client := &http.Client{Timeout: time.Second}
	var resp *http.Response
	for i := 0; i < 10; i++ {
		resp, err = client.Get("http://" + addr + "/debug/memory")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /debug/memory error = %v, want nil", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/memory status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var snapshot Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode /debug/memory response error = %v", err)
	}
	if snapshot.Timestamp.IsZero() {
		t.Fatal("snapshot timestamp is zero, want captured snapshot")
	}
}

func TestStartServerCapturesSnapshotBeforeServingMemory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mkfifo is not supported on windows")
	}

	addr := reserveLoopbackAddress(t)
	cgroupRoot := t.TempDir()
	writeTestFile(t, cgroupRoot, "memory.max", "100\n")
	currentPath := filepath.Join(cgroupRoot, "memory.current")
	if err := syscall.Mkfifo(currentPath, 0o600); err != nil {
		t.Fatalf("mkfifo memory.current: %v", err)
	}

	wrote := make(chan error, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		file, err := os.OpenFile(currentPath, os.O_WRONLY, 0)
		if err != nil {
			wrote <- err
			return
		}
		_, err = file.WriteString("95\n")
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		wrote <- err
	}()

	stop, err := StartServer(config.DiagnosticsConfig{
		Enabled:               true,
		Bind:                  addr,
		SampleIntervalSeconds: 3600,
		ProfileDir:            t.TempDir(),
	}, cgroupRoot)
	if err != nil {
		t.Fatalf("StartServer() error = %v, want nil", err)
	}
	if stop == nil {
		t.Fatal("StartServer() stop = nil, want stop function")
	}
	defer stop()

	select {
	case err := <-wrote:
		if err != nil {
			t.Fatalf("write delayed cgroup sample: %v", err)
		}
	default:
		t.Fatal("StartServer returned before the first memory sample completed")
	}

	snapshot := getMemorySnapshot(t, addr)
	if snapshot.Timestamp.IsZero() {
		t.Fatal("snapshot timestamp is zero immediately after server start")
	}
	if snapshot.Cgroup.CurrentBytes != 95 || snapshot.Cgroup.LimitBytes != 100 {
		t.Fatalf("snapshot cgroup = current:%d limit:%d, want current:95 limit:100", snapshot.Cgroup.CurrentBytes, snapshot.Cgroup.LimitBytes)
	}
}

func TestStartServerStopReleasesListenerForRebind(t *testing.T) {
	addr := reserveLoopbackAddress(t)
	cfg := config.DiagnosticsConfig{
		Enabled:               true,
		Bind:                  addr,
		SampleIntervalSeconds: 1,
		ProfileDir:            t.TempDir(),
	}

	stop, err := StartServer(cfg, "")
	if err != nil {
		t.Fatalf("first StartServer() error = %v, want nil", err)
	}
	if stop == nil {
		t.Fatal("first StartServer() stop = nil, want stop function")
	}
	stop()

	stop, err = StartServer(cfg, "")
	if err != nil {
		t.Fatalf("second StartServer() error = %v, want nil after stop", err)
	}
	if stop == nil {
		t.Fatal("second StartServer() stop = nil, want stop function")
	}
	stop()
}

func TestDumpHighWaterProfilesBelowThresholdDoesNothing(t *testing.T) {
	profileDir := t.TempDir()
	dumped := make(map[int]bool)

	dumpHighWaterProfiles(config.DiagnosticsConfig{
		ProfileDir:               profileDir,
		HighWaterProfilePercents: []int{95},
	}, Snapshot{
		Timestamp: time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC),
		Cgroup: CgroupSnapshot{
			CurrentBytes: 94,
			LimitBytes:   100,
		},
	}, dumped)

	matches, err := filepath.Glob(filepath.Join(profileDir, "*"))
	if err != nil {
		t.Fatalf("glob profile dir: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("profile files = %v, want none below threshold", matches)
	}
}

func TestDumpHighWaterProfilesCreatesArtifactsAtThreshold(t *testing.T) {
	profileDir := t.TempDir()
	dumped := make(map[int]bool)
	mark := 95
	timestamp := time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC)

	dumpHighWaterProfiles(config.DiagnosticsConfig{
		ProfileDir:               profileDir,
		HighWaterProfilePercents: []int{mark},
	}, Snapshot{
		Timestamp: timestamp,
		Cgroup: CgroupSnapshot{
			CurrentBytes: 95,
			LimitBytes:   100,
		},
	}, dumped)

	if files := globProfileFiles(t, profileDir, fmt.Sprintf("heap-%d-*.pprof", mark)); len(files) != 1 {
		t.Fatalf("heap profiles = %v, want 1", files)
	}
	if files := globProfileFiles(t, profileDir, fmt.Sprintf("goroutine-%d-*.txt", mark)); len(files) != 1 {
		t.Fatalf("goroutine profiles = %v, want 1", files)
	}
}

func TestDumpHighWaterProfilesDoesNotRepeatAfterPartialFailure(t *testing.T) {
	profileDir := t.TempDir()
	dumped := make(map[int]bool)
	mark := 95
	firstTimestamp := time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC)
	firstGoroutinePath := filepath.Join(profileDir, fmt.Sprintf("goroutine-%d-%s.txt", mark, firstTimestamp.UTC().Format("20060102T150405Z")))
	if err := os.Mkdir(firstGoroutinePath, 0o755); err != nil {
		t.Fatalf("create blocking goroutine profile path: %v", err)
	}

	cfg := config.DiagnosticsConfig{
		ProfileDir:               profileDir,
		HighWaterProfilePercents: []int{mark},
	}
	snapshot := Snapshot{
		Timestamp: firstTimestamp,
		Cgroup: CgroupSnapshot{
			CurrentBytes: 95,
			LimitBytes:   100,
		},
	}
	dumpHighWaterProfiles(cfg, snapshot, dumped)
	if files := globProfileFiles(t, profileDir, fmt.Sprintf("heap-%d-*.pprof", mark)); len(files) != 1 {
		t.Fatalf("heap profiles after first attempt = %v, want 1", files)
	}

	snapshot.Timestamp = firstTimestamp.Add(time.Second)
	dumpHighWaterProfiles(cfg, snapshot, dumped)
	if files := globProfileFiles(t, profileDir, fmt.Sprintf("heap-%d-*.pprof", mark)); len(files) != 1 {
		t.Fatalf("heap profiles after repeated attempt = %v, want 1 because mark was already attempted", files)
	}
}

func reserveLoopbackAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listener: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserve listener: %v", err)
	}
	return addr
}

func getMemorySnapshot(t *testing.T, addr string) Snapshot {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	var lastErr error
	for i := 0; i < 10; i++ {
		resp, err := client.Get("http://" + addr + "/debug/memory")
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /debug/memory status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var snapshot Snapshot
		if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
			t.Fatalf("decode /debug/memory response error = %v", err)
		}
		return snapshot
	}
	t.Fatalf("GET /debug/memory error = %v, want nil", lastErr)
	return Snapshot{}
}

func globProfileFiles(t *testing.T, profileDir, pattern string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(profileDir, pattern))
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	return matches
}
