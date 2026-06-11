package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCgroupMemoryV2(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "memory.current", "12345\n")
	writeTestFile(t, root, "memory.max", "98765\n")
	writeTestFile(t, root, "memory.events", "low 0\nhigh 3\nmax 7\noom 2\noom_kill 1\n")
	writeTestFile(t, root, "memory.stat", "anon 555\nfile 666\nkernel_stack 7\n")

	snapshot, err := readCgroupMemory(root)
	if err != nil {
		t.Fatalf("readCgroupMemory() error = %v", err)
	}

	if snapshot.Version != 2 {
		t.Fatalf("Version = %d, want 2", snapshot.Version)
	}
	if snapshot.CurrentBytes != 12345 {
		t.Fatalf("CurrentBytes = %d, want 12345", snapshot.CurrentBytes)
	}
	if snapshot.LimitBytes != 98765 {
		t.Fatalf("LimitBytes = %d, want 98765", snapshot.LimitBytes)
	}
	if snapshot.FailCount != 7 {
		t.Fatalf("FailCount = %d, want 7", snapshot.FailCount)
	}
	if snapshot.OOMKillCount != 1 {
		t.Fatalf("OOMKillCount = %d, want 1", snapshot.OOMKillCount)
	}
	if snapshot.FileBytes != 666 {
		t.Fatalf("FileBytes = %d, want 666", snapshot.FileBytes)
	}
	if snapshot.AnonBytes != 555 {
		t.Fatalf("AnonBytes = %d, want 555", snapshot.AnonBytes)
	}
}

func TestReadCgroupMemoryV2UnlimitedMax(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "memory.current", "12345\n")
	writeTestFile(t, root, "memory.max", "max\n")

	snapshot, err := readCgroupMemory(root)
	if err != nil {
		t.Fatalf("readCgroupMemory() error = %v", err)
	}

	if snapshot.Version != 2 {
		t.Fatalf("Version = %d, want 2", snapshot.Version)
	}
	if snapshot.LimitBytes != 0 {
		t.Fatalf("LimitBytes = %d, want 0 for unlimited v2 max", snapshot.LimitBytes)
	}
}

func TestReadCgroupMemoryV1(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "memory.usage_in_bytes", "22222\n")
	writeTestFile(t, root, "memory.limit_in_bytes", "33333\n")
	writeTestFile(t, root, "memory.failcnt", "4\n")
	writeTestFile(t, root, "memory.stat", "cache 555\nrss 666\nmapped_file 7\n")

	snapshot, err := readCgroupMemory(root)
	if err != nil {
		t.Fatalf("readCgroupMemory() error = %v", err)
	}

	if snapshot.Version != 1 {
		t.Fatalf("Version = %d, want 1", snapshot.Version)
	}
	if snapshot.CurrentBytes != 22222 {
		t.Fatalf("CurrentBytes = %d, want 22222", snapshot.CurrentBytes)
	}
	if snapshot.LimitBytes != 33333 {
		t.Fatalf("LimitBytes = %d, want 33333", snapshot.LimitBytes)
	}
	if snapshot.FailCount != 4 {
		t.Fatalf("FailCount = %d, want 4", snapshot.FailCount)
	}
	if snapshot.FileBytes != 555 {
		t.Fatalf("FileBytes = %d, want 555", snapshot.FileBytes)
	}
	if snapshot.AnonBytes != 666 {
		t.Fatalf("AnonBytes = %d, want 666", snapshot.AnonBytes)
	}
}

func TestReadCgroupMemoryV1NestedMemoryController(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "memory"), "memory.usage_in_bytes", "22222\n")
	writeTestFile(t, filepath.Join(root, "memory"), "memory.limit_in_bytes", "33333\n")
	writeTestFile(t, filepath.Join(root, "memory"), "memory.failcnt", "4\n")
	writeTestFile(t, filepath.Join(root, "memory"), "memory.stat", "cache 555\nrss 666\n")

	snapshot, err := readCgroupMemory(root)
	if err != nil {
		t.Fatalf("readCgroupMemory() error = %v", err)
	}

	if snapshot.Version != 1 {
		t.Fatalf("Version = %d, want 1", snapshot.Version)
	}
	if snapshot.CurrentBytes != 22222 {
		t.Fatalf("CurrentBytes = %d, want 22222", snapshot.CurrentBytes)
	}
	if snapshot.LimitBytes != 33333 {
		t.Fatalf("LimitBytes = %d, want 33333", snapshot.LimitBytes)
	}
	if snapshot.FailCount != 4 {
		t.Fatalf("FailCount = %d, want 4", snapshot.FailCount)
	}
	if snapshot.FileBytes != 555 {
		t.Fatalf("FileBytes = %d, want 555", snapshot.FileBytes)
	}
	if snapshot.AnonBytes != 666 {
		t.Fatalf("AnonBytes = %d, want 666", snapshot.AnonBytes)
	}
}

func TestReadCgroupMemoryV1DockerControllerPathFromProcCgroup(t *testing.T) {
	root := t.TempDir()
	procCgroup := filepath.Join(root, "proc-self-cgroup")
	writeTestFile(t, root, "proc-self-cgroup", "7:memory:/docker/abc123\n")
	controllerDir := filepath.Join(root, "memory", "docker", "abc123")
	writeTestFile(t, controllerDir, "memory.usage_in_bytes", "44444\n")
	writeTestFile(t, controllerDir, "memory.limit_in_bytes", "55555\n")
	writeTestFile(t, controllerDir, "memory.failcnt", "6\n")
	writeTestFile(t, controllerDir, "memory.stat", "cache 777\nrss 888\n")

	snapshot, err := readCgroupMemoryWithProc(root, procCgroup)
	if err != nil {
		t.Fatalf("readCgroupMemoryWithProc() error = %v", err)
	}

	if snapshot.Version != 1 {
		t.Fatalf("Version = %d, want 1", snapshot.Version)
	}
	if snapshot.CurrentBytes != 44444 {
		t.Fatalf("CurrentBytes = %d, want 44444", snapshot.CurrentBytes)
	}
	if snapshot.LimitBytes != 55555 {
		t.Fatalf("LimitBytes = %d, want 55555", snapshot.LimitBytes)
	}
	if snapshot.FailCount != 6 {
		t.Fatalf("FailCount = %d, want 6", snapshot.FailCount)
	}
	if snapshot.FileBytes != 777 {
		t.Fatalf("FileBytes = %d, want 777", snapshot.FileBytes)
	}
	if snapshot.AnonBytes != 888 {
		t.Fatalf("AnonBytes = %d, want 888", snapshot.AnonBytes)
	}
}

func TestReadCgroupMemoryV1UnlimitedLimit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "memory.usage_in_bytes", "22222\n")
	writeTestFile(t, root, "memory.limit_in_bytes", "1152921504606846977\n")
	writeTestFile(t, root, "memory.failcnt", "0\n")

	snapshot, err := readCgroupMemory(root)
	if err != nil {
		t.Fatalf("readCgroupMemory() error = %v", err)
	}

	if snapshot.Version != 1 {
		t.Fatalf("Version = %d, want 1", snapshot.Version)
	}
	if snapshot.LimitBytes != 0 {
		t.Fatalf("LimitBytes = %d, want 0 for unlimited v1 sentinel", snapshot.LimitBytes)
	}
}

func TestReadCgroupMemoryMissingUsageFilesReturnsError(t *testing.T) {
	root := t.TempDir()

	if _, err := readCgroupMemory(root); err == nil {
		t.Fatal("readCgroupMemory() error = nil, want error")
	}
}

func TestReadCgroupMemoryV2MissingMaxReturnsV2Error(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "memory.current", "12345\n")

	_, err := readCgroupMemory(root)
	if err == nil {
		t.Fatal("readCgroupMemory() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "memory.max") {
		t.Fatalf("readCgroupMemory() error = %v, want memory.max error", err)
	}
}

func TestReadCgroupMemoryMalformedMandatoryFieldsReturnErrors(t *testing.T) {
	t.Run("v2 current", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "memory.current", "bad\n")
		writeTestFile(t, root, "memory.max", "98765\n")

		if _, err := readCgroupMemory(root); err == nil {
			t.Fatal("readCgroupMemory() error = nil, want error")
		}
	})

	t.Run("v2 max", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "memory.current", "12345\n")
		writeTestFile(t, root, "memory.max", "bad\n")

		if _, err := readCgroupMemory(root); err == nil {
			t.Fatal("readCgroupMemory() error = nil, want error")
		}
	})

	t.Run("v1 usage", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "memory.usage_in_bytes", "bad\n")
		writeTestFile(t, root, "memory.limit_in_bytes", "33333\n")
		writeTestFile(t, root, "memory.failcnt", "4\n")

		if _, err := readCgroupMemory(root); err == nil {
			t.Fatal("readCgroupMemory() error = nil, want error")
		}
	})

	t.Run("v1 limit", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "memory.usage_in_bytes", "22222\n")
		writeTestFile(t, root, "memory.limit_in_bytes", "bad\n")
		writeTestFile(t, root, "memory.failcnt", "4\n")

		if _, err := readCgroupMemory(root); err == nil {
			t.Fatal("readCgroupMemory() error = nil, want error")
		}
	})

	t.Run("v1 failcnt", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "memory.usage_in_bytes", "22222\n")
		writeTestFile(t, root, "memory.limit_in_bytes", "33333\n")
		writeTestFile(t, root, "memory.failcnt", "bad\n")

		if _, err := readCgroupMemory(root); err == nil {
			t.Fatal("readCgroupMemory() error = nil, want error")
		}
	})
}

func TestReadCgroupMemoryMalformedOptionalFieldsUseZeroDefaults(t *testing.T) {
	t.Run("v2 events and stat", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "memory.current", "12345\n")
		writeTestFile(t, root, "memory.max", "98765\n")
		writeTestFile(t, root, "memory.events", "max bad\noom_kill bad\n")
		writeTestFile(t, root, "memory.stat", "file bad\nanon bad\n")

		snapshot, err := readCgroupMemory(root)
		if err != nil {
			t.Fatalf("readCgroupMemory() error = %v", err)
		}
		if snapshot.FailCount != 0 || snapshot.OOMKillCount != 0 || snapshot.FileBytes != 0 || snapshot.AnonBytes != 0 {
			t.Fatalf("optional fields = fail:%d oom_kill:%d file:%d anon:%d, want all zero", snapshot.FailCount, snapshot.OOMKillCount, snapshot.FileBytes, snapshot.AnonBytes)
		}
	})

	t.Run("v1 stat", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, root, "memory.usage_in_bytes", "22222\n")
		writeTestFile(t, root, "memory.limit_in_bytes", "33333\n")
		writeTestFile(t, root, "memory.failcnt", "4\n")
		writeTestFile(t, root, "memory.stat", "cache bad\nrss bad\n")

		snapshot, err := readCgroupMemory(root)
		if err != nil {
			t.Fatalf("readCgroupMemory() error = %v", err)
		}
		if snapshot.FileBytes != 0 || snapshot.AnonBytes != 0 {
			t.Fatalf("optional fields = file:%d anon:%d, want zero", snapshot.FileBytes, snapshot.AnonBytes)
		}
	})
}

func TestParseProcStatus(t *testing.T) {
	snapshot := parseProcStatus([]byte("Name:\tutxo_indexer\nVmHWM:\t  2048 kB\nVmRSS:\t  1024 kB\n"))

	if snapshot.RSSBytes != 1024*1024 {
		t.Fatalf("RSSBytes = %d, want %d", snapshot.RSSBytes, 1024*1024)
	}
	if snapshot.HighWaterBytes != 2048*1024 {
		t.Fatalf("HighWaterBytes = %d, want %d", snapshot.HighWaterBytes, 2048*1024)
	}
}

func TestParseProcStatusMalformedValuesUseZeroDefaults(t *testing.T) {
	snapshot := parseProcStatus([]byte("VmHWM:\t bad kB\nVmRSS:\t nope kB\n"))

	if snapshot.RSSBytes != 0 {
		t.Fatalf("RSSBytes = %d, want 0", snapshot.RSSBytes)
	}
	if snapshot.HighWaterBytes != 0 {
		t.Fatalf("HighWaterBytes = %d, want 0", snapshot.HighWaterBytes)
	}
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
