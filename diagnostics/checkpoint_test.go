package diagnostics

import (
	"strings"
	"testing"
)

func TestFormatCheckpointIncludesMemoryFields(t *testing.T) {
	snapshot := Snapshot{
		Runtime: RuntimeSnapshot{
			HeapAllocBytes: 10 << 20,
			HeapSysBytes:   25 << 20,
			HeapIdleBytes:  5 << 20,
			NumGC:          3,
			NumGoroutine:   12,
		},
		Proc: ProcSnapshot{
			RSSBytes:       30 << 20,
			HighWaterBytes: 40 << 20,
		},
		Cgroup: CgroupSnapshot{
			CurrentBytes: 50 << 20,
			LimitBytes:   100 << 20,
			FileBytes:    5 << 20,
			AnonBytes:    45 << 20,
			FailCount:    2,
		},
	}

	line := FormatCheckpoint(Checkpoint{
		Height:      952908,
		Phase:       "after_spend",
		TxCount:     3000,
		InputCount:  9000,
		OutputCount: 6000,
		ElapsedMS:   1234,
		Snapshot:    snapshot,
	})

	for _, want := range []string{
		"[MemoryCheckpoint]",
		"height=952908",
		"phase=after_spend",
		"tx=3000",
		"inputs=9000",
		"outputs=6000",
		"elapsed_ms=1234",
		"heap_alloc_mb=10",
		"heap_inuse_mb=20",
		"rss_mb=30",
		"hwm_mb=40",
		"cgroup_mb=50",
		"cgroup_limit_mb=100",
		"cgroup_pct=50.0",
		"cgroup_file_mb=5",
		"cgroup_anon_mb=45",
		"cgroup_fail=2",
		"gc=3",
		"goroutines=12",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("FormatCheckpoint() missing %q in %q", want, line)
		}
	}
}

func TestFormatCheckpointHandlesZeroCgroupLimit(t *testing.T) {
	line := FormatCheckpoint(Checkpoint{
		Snapshot: Snapshot{
			Cgroup: CgroupSnapshot{
				CurrentBytes: 50 << 20,
				LimitBytes:   0,
			},
		},
	})

	if !strings.Contains(line, "cgroup_pct=0.0") {
		t.Fatalf("FormatCheckpoint() should report zero cgroup percent for no limit, got %q", line)
	}
}

func TestFormatCheckpointClampsHeapInuseWhenIdleExceedsSys(t *testing.T) {
	line := FormatCheckpoint(Checkpoint{
		Snapshot: Snapshot{
			Runtime: RuntimeSnapshot{
				HeapSysBytes:  5 << 20,
				HeapIdleBytes: 10 << 20,
			},
		},
	})

	if !strings.Contains(line, "heap_inuse_mb=0") {
		t.Fatalf("FormatCheckpoint() should clamp heap in-use to zero, got %q", line)
	}
}
