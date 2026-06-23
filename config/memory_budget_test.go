package config

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func TestChooseMemoryBudgetUsesSmallerCgroupLimit(t *testing.T) {
	cgroupLimit := int64(12 << 30)

	got := ChooseMemoryBudget(32, MemoryBudgetConfig{
		UseCgroupLimit:        true,
		GoMemoryLimitPercent:  65,
		ReservePercent:        15,
		ForceCgroupLimitBytes: cgroupLimit,
	})

	if got.EffectiveMemoryBytes != cgroupLimit {
		t.Fatalf("EffectiveMemoryBytes = %d, want %d", got.EffectiveMemoryBytes, cgroupLimit)
	}

	wantGoLimit := int64(float64(cgroupLimit) * 0.65)
	if got.GoMemoryLimitBytes != wantGoLimit {
		t.Fatalf("GoMemoryLimitBytes = %d, want %d", got.GoMemoryLimitBytes, wantGoLimit)
	}
}

func TestChooseMemoryBudgetFallsBackToConfiguredMemory(t *testing.T) {
	const configuredLimit int64 = 4 << 30

	got := ChooseMemoryBudget(4, MemoryBudgetConfig{
		UseCgroupLimit:       true,
		GoMemoryLimitPercent: 65,
		ReservePercent:       15,
	})

	if got.EffectiveMemoryBytes != configuredLimit {
		t.Fatalf("EffectiveMemoryBytes = %d, want %d", got.EffectiveMemoryBytes, configuredLimit)
	}
}

func TestAutoConfigureBalancedSmallHostWorkerAndBatch(t *testing.T) {
	got := AutoConfigure(SystemResources{
		CPUCores:   2,
		MemoryGB:   4,
		HighPerf:   false,
		ShardCount: 6,
	})

	if got.WorkerCount != 2 {
		t.Fatalf("WorkerCount = %d, want 2", got.WorkerCount)
	}
	if got.MaxBatchSizeMB != 4 {
		t.Fatalf("MaxBatchSizeMB = %d, want 4", got.MaxBatchSizeMB)
	}
}

func TestDetectCgroupLimitBytesReadsV2MemoryMax(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "memory.max"), []byte("12345\n"), 0644); err != nil {
		t.Fatalf("write memory.max: %v", err)
	}

	if got := DetectCgroupLimitBytes(root); got != 12345 {
		t.Fatalf("DetectCgroupLimitBytes = %d, want 12345", got)
	}
}

func TestDetectCgroupLimitBytesMapsV2MaxToZero(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "memory.max"), []byte("max\n"), 0644); err != nil {
		t.Fatalf("write memory.max: %v", err)
	}

	if got := DetectCgroupLimitBytes(root); got != 0 {
		t.Fatalf("DetectCgroupLimitBytes = %d, want 0", got)
	}
}

func TestDetectCgroupLimitBytesReadsV1WhenV2MaxIsUnlimited(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "memory.max"), []byte("max\n"), 0644); err != nil {
		t.Fatalf("write memory.max: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "memory.limit_in_bytes"), []byte("34567\n"), 0644); err != nil {
		t.Fatalf("write memory.limit_in_bytes: %v", err)
	}

	if got := DetectCgroupLimitBytes(root); got != 34567 {
		t.Fatalf("DetectCgroupLimitBytes = %d, want 34567", got)
	}
}

func TestDetectCgroupLimitBytesReadsFlatV1MemoryLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "memory.limit_in_bytes"), []byte("23456\n"), 0644); err != nil {
		t.Fatalf("write memory.limit_in_bytes: %v", err)
	}

	if got := DetectCgroupLimitBytes(root); got != 23456 {
		t.Fatalf("DetectCgroupLimitBytes = %d, want 23456", got)
	}
}

func TestDetectCgroupLimitBytesReadsV1MemoryLimit(t *testing.T) {
	root := t.TempDir()
	memoryDir := filepath.Join(root, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		t.Fatalf("mkdir memory cgroup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "memory.limit_in_bytes"), []byte("54321\n"), 0644); err != nil {
		t.Fatalf("write memory.limit_in_bytes: %v", err)
	}

	if got := DetectCgroupLimitBytes(root); got != 54321 {
		t.Fatalf("DetectCgroupLimitBytes = %d, want 54321", got)
	}
}

func TestDetectCgroupLimitBytesMapsV1HugeSentinelToZero(t *testing.T) {
	root := t.TempDir()
	memoryDir := filepath.Join(root, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		t.Fatalf("mkdir memory cgroup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "memory.limit_in_bytes"), []byte("9223372036854771712\n"), 0644); err != nil {
		t.Fatalf("write memory.limit_in_bytes: %v", err)
	}

	if got := DetectCgroupLimitBytes(root); got != 0 {
		t.Fatalf("DetectCgroupLimitBytes = %d, want 0", got)
	}
}

func TestDetectCgroupLimitBytesInvalidValuesReturnZero(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "empty", content: ""},
		{name: "malformed", content: "not-a-number\n"},
		{name: "zero", content: "0\n"},
		{name: "negative", content: "-1\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "memory.max"), []byte(tt.content), 0644); err != nil {
				t.Fatalf("write memory.max: %v", err)
			}

			if got := DetectCgroupLimitBytes(root); got != 0 {
				t.Fatalf("DetectCgroupLimitBytes = %d, want 0", got)
			}
		})
	}
}

func TestDetectCgroupLimitBytesMissingFilesReturnZero(t *testing.T) {
	if got := DetectCgroupLimitBytes(t.TempDir()); got != 0 {
		t.Fatalf("DetectCgroupLimitBytes = %d, want 0", got)
	}
}

func TestApplyGoMemoryLimitSetsAndRestoresRuntimeLimit(t *testing.T) {
	previous := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		debug.SetMemoryLimit(previous)
	})

	const want int64 = 128 << 20
	ApplyGoMemoryLimit(MemoryBudget{GoMemoryLimitBytes: want})

	if got := debug.SetMemoryLimit(-1); got != want {
		t.Fatalf("debug.SetMemoryLimit(-1) = %d, want %d", got, want)
	}
}
