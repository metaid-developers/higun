package diagnostics

import "fmt"

const bytesPerMiB = 1024 * 1024

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
	runtime := c.Snapshot.Runtime
	proc := c.Snapshot.Proc
	cgroup := c.Snapshot.Cgroup

	heapInuseBytes := uint64(0)
	if runtime.HeapSysBytes > runtime.HeapIdleBytes {
		heapInuseBytes = runtime.HeapSysBytes - runtime.HeapIdleBytes
	}

	cgroupPct := 0.0
	if cgroup.LimitBytes > 0 {
		cgroupPct = float64(cgroup.CurrentBytes) * 100 / float64(cgroup.LimitBytes)
	}

	return fmt.Sprintf(
		"[MemoryCheckpoint] height=%d phase=%s tx=%d inputs=%d outputs=%d elapsed_ms=%d heap_alloc_mb=%d heap_inuse_mb=%d rss_mb=%d hwm_mb=%d cgroup_mb=%d cgroup_limit_mb=%d cgroup_pct=%.1f cgroup_file_mb=%d cgroup_anon_mb=%d cgroup_fail=%d gc=%d goroutines=%d",
		c.Height,
		c.Phase,
		c.TxCount,
		c.InputCount,
		c.OutputCount,
		c.ElapsedMS,
		runtime.HeapAllocBytes/bytesPerMiB,
		heapInuseBytes/bytesPerMiB,
		proc.RSSBytes/bytesPerMiB,
		proc.HighWaterBytes/bytesPerMiB,
		cgroup.CurrentBytes/bytesPerMiB,
		cgroup.LimitBytes/bytesPerMiB,
		cgroupPct,
		cgroup.FileBytes/bytesPerMiB,
		cgroup.AnonBytes/bytesPerMiB,
		cgroup.FailCount,
		runtime.NumGC,
		runtime.NumGoroutine,
	)
}
