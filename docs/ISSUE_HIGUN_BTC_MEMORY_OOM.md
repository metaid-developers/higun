# Issue: higun_btc production OOM and suspected memory leak

## Summary

`higun_btc` on production host `8.217.251.101` repeatedly reaches its Docker cgroup memory limit and is killed by the kernel OOM killer. This still happens after the container memory limit was raised from 6GiB to 10GiB and then to 12GiB. A two-hour production experiment disabling mempool/ZMQ did not prevent another OOM, so mempool/ZMQ is not the sole root cause.

Current evidence points to abnormal memory growth or unbounded cache/allocation behavior inside the main BTC `utxo_indexer` runtime, especially the block indexing path and Pebble storage/cache interaction. This issue should be handled as a code-level memory investigation, not only an ops capacity issue.

## Production context

- Host: `8.217.251.101`
- Container: `higun_btc`
- Process name in kernel OOM logs: `utxo_indexer`
- Chain/network: BTC mainnet
- API ports: `8066` primary API, `8085` compatibility API
- Current Docker memory limit observed during incident: `12884901888` bytes, about 12GiB
- Host memory observed during incident window: about 15GiB RAM plus 4GiB swap
- Docker restart policy: `always`
- Side monitoring path on host: `/date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv`
- Side monitoring scripts on host:
  - `/usr/local/bin/higun_btc_mem_watch.sh`
  - `/usr/local/bin/higun_btc_mem_report.sh`
  - `higun-btc-mem-watch.timer`

Observed production config values during the investigation:

```yaml
chain: btc
network: mainnet
shard_count: 6
batch_size: 5000
tx_concurrency: 8
workers: 2
cpu_cores: 2
memory_gb: 4
high_perf: false
api_port: 8066
mem_utxo_max_count: 9000
max_tx_per_batch: 10000
zmq_address:
  - tcp://172.31.168.214:28333
```

Startup logs showed the effective auto configuration as:

```text
Using configuration: CPU=2, Memory=4GB, Shards=6, BatchSize=500, Workers=2
```

Note the config has `batch_size: 5000`, but `config.AutoConfigure` currently derives the effective `BatchSize` from `memory_gb/high_perf`, and in balanced mode logs `BatchSize=500`.

## Why this is not just low machine capacity

The workload is a BTC node/indexer workload where normal per-block input size should be bounded. The service still grows until a 12GiB cgroup kill even when public traffic is not high. Raising the Docker limit delayed but did not materially solve the issue.

The most important production observations:

1. Repeated OOM kills happen at about the container memory ceiling.
2. The killed process is consistently `utxo_indexer`.
3. OOM kill logs show very high anonymous RSS, not only file cache.
4. Disabling mempool/ZMQ did not stop OOM.
5. The process restarts and resumes indexing, then memory grows again.

This pattern is compatible with:

- A Go heap/object retention leak.
- Native/off-heap memory retained by Pebble or cgo-backed/native paths.
- Unbounded or duplicated Pebble cache/memtable allocation across stores and shards.
- Large transient per-block/per-height allocations that are retained longer than expected.
- A runtime memory limit mismatch where Go and Pebble are not aware of Docker cgroup pressure.

## OOM evidence

After memory had been raised to 12GiB, kernel logs showed repeated cgroup OOM kills:

| Time (CST) | Killed PID | Kernel process name | anon RSS | Notes |
| --- | ---: | --- | ---: | --- |
| 2026-06-09 03:23:24 | 31478 | `utxo_indexer` | `12203340kB` | cgroup OOM |
| 2026-06-09 04:09:29 | 43914 | `utxo_indexer` | `12233980kB` | cgroup OOM |
| 2026-06-09 04:57:39 | 49178 | `utxo_indexer` | `12232900kB` | cgroup OOM |
| 2026-06-09 06:49:42 | 54711 | `utxo_indexer` | `12181564kB` | cgroup OOM |
| 2026-06-09 08:42:16 | 66991 | `utxo_indexer` | `12183100kB` | cgroup OOM |
| 2026-06-09 10:46:33 | 82412 | `utxo_indexer` | `12186868kB` | cgroup OOM while mempool/ZMQ was disabled |

The 10:46 kill is especially important because it happened while `zmq_address` was deliberately set to `[]`.

## Side-monitor observations

The side monitor separates process lifetimes by PID. Reported PID generations included:

```text
31478|0
43914|1
49178|2
54711|3
66991|4
82412|0
94526|1
```

During the mempool/ZMQ-disable experiment:

- Config was changed to `zmq_address: []`.
- Logs confirmed:

```text
Initializing mempool manager, ZMQ address: [], network: mainnet
Mempool disabled: no zmq_address configured
```

- The container still OOMed at `2026-06-09 10:46:33 CST`.
- Kernel killed pid `82412`.
- OOM values included:
  - `anon-rss=12186868kB`
  - `file-rss=9600kB`
  - memcg under the Docker cgroup

Side monitor trend after disabling mempool/ZMQ:

- PID `82412|0`: height `952908 -> 952915`; process RSS `899432 -> 1049428 kB` in sampled points, but observed max RSS reached `6963372 kB`, HWM reached `10649664 kB`, cgroup max reached about `12884897792` bytes, cache max reached about `11680473088` bytes, failcnt increased sharply, then OOM.
- PID `94526|1`: after restart and before restoring ZMQ, process RSS moved from about `197612 -> 1065572 kB`, max RSS reached `4185888 kB`, cgroup max again approached the 12GiB limit.

Interpretation: the side monitor already suggests pressure may alternate between process RSS and cgroup cache/failcnt. The code fix should instrument both, because the root cause may be Go heap/object retention, Pebble native/cache usage, or cgroup file-cache pressure caused by storage access patterns.

## pprof/debug endpoint gap

During the incident, these endpoints returned 404:

```text
/debug/pprof/
/debug/pprof/heap
/debug/vars
```

This prevented live heap/goroutine/profile capture before the OOM. The first required code change should be safe production profiling and runtime metrics guarded behind config or localhost/admin-only access.

## Relevant code areas

This is not a final root-cause claim; it is the initial map for developers to investigate.

### Runtime resource auto configuration

- `config/auto.go`
  - `AutoConfigure` derives `WorkerCount`, `BatchSize`, Pebble cache size, memtable size, WAL size, and estimated memory usage from configured `memory_gb`, `cpu_cores`, `high_perf`, and `shard_count`.
  - It does not appear to read Docker cgroup memory limits.
  - It does not appear to set Go runtime memory limits such as `debug.SetMemoryLimit` or expect `GOMEMLIMIT`.
  - It divides cache/memtable by shard count for one store, but the runtime opens several stores.

### Pebble store creation

- `storage/pebble.go`
  - `NewPebbleStore` opens multiple Pebble shards.
  - Runtime opens UTXO, income, spend, address balance, balance rank, meta, and mempool-related stores.
  - Developers should verify whether each store/shard gets independent cache/memtable resources, and whether the total memory budget across all stores can exceed the intended process budget.
  - Developers should check Pebble cache object ownership and close behavior.

### Startup/indexing flow

- `main.go`
  - `initDb` opens the storage stores and initializes the blockchain client and mempool manager.
  - `WarmupMemoryUTXO(lastHeightInt)` runs at startup when a last height exists.
  - `bcClient.SyncBlocks(...)` runs continuously in a goroutine.
  - `newMempoolManagerIfConfigured` correctly returns nil when ZMQ is disabled; the production test confirmed this path.

### Main BTC indexing path

- `blockchain/client.go`
  - `SyncBlocks`
  - `ProcessBlock`
- `indexer/utxo.go`
  - `WarmupMemoryUTXO`
  - `IndexBlock`

These functions should get memory checkpoints around fetch/decode/process/index/commit/height-update stages.

### Mempool path

- `mempool/`
- `main.go` mempool manager setup and delayed start

Mempool/ZMQ is not cleared. The production experiment only shows it is not the only root cause. It may still add pressure after the main leak/retention issue is fixed.

## Requested development tasks

### 1. Add safe production diagnostics

Add config-gated diagnostics that can be enabled on production without exposing secrets:

- `net/http/pprof` on localhost or an admin-only port.
- `expvar` or structured `/debug/vars` style metrics.
- Periodic runtime memory logs:
  - `runtime.MemStats.Alloc`
  - `runtime.MemStats.HeapAlloc`
  - `runtime.MemStats.HeapInuse`
  - `runtime.MemStats.HeapIdle`
  - `runtime.MemStats.HeapReleased`
  - `runtime.MemStats.Sys`
  - `runtime.MemStats.NumGC`
  - goroutine count
  - `debug.SetMemoryLimit` effective value if used
- Process/cgroup logs:
  - process RSS/HWM from `/proc/self/status`
  - cgroup current/max/failcnt/events where available
  - Pebble cache/memtable metrics if exposed
- Automatic heap/goroutine profile dump when memory crosses configurable high-water marks, for example 70%, 80%, and 90% of cgroup memory.
- Stage markers around block processing so a profile can be tied to a height and phase.

Acceptance for this task: before changing memory behavior, we can capture heap, goroutine, and runtime/cgroup metrics from a running production or staging container.

### 2. Make memory budgeting cgroup-aware

The runtime should compute memory budget from the actual Docker cgroup limit when running in a container. Configured `memory_gb` should be treated as a cap or hint, not blindly as the true machine memory.

Recommended direction:

- Detect cgroup v1/v2 memory limits at startup.
- Compute a conservative application memory budget from the smaller of configured memory and cgroup limit.
- Set `debug.SetMemoryLimit` or document and enforce `GOMEMLIMIT`, leaving headroom for Pebble native memory, file cache, stacks, and the OS.
- Emit startup logs showing:
  - detected host memory
  - detected cgroup memory limit
  - configured `memory_gb`
  - chosen Go memory limit
  - chosen Pebble/store budget
  - chosen worker/batch settings

Acceptance for this task: in a 12GiB Docker cgroup, the service does not configure itself as if it owns unbounded host memory, and logs make the chosen budget auditable.

### 3. Bound total Pebble memory across all stores and shards

Audit Pebble options across all open stores. The goal is to cap total memory, not just per-shard or per-store memory.

Recommended direction:

- Create one shared Pebble cache where appropriate, or explicitly divide cache budgets across all stores and shards.
- Ensure memtable/write-buffer budgets are part of the total budget.
- Consider lower cache/memtable defaults for BTC production until the leak is fixed.
- Expose Pebble metrics in runtime logs.
- Verify all stores and batches are closed promptly.

Acceptance for this task: total configured Pebble cache/memtable memory across UTXO/income/spend/balance/rank/meta/mempool stores is explicitly bounded and visible in logs.

### 4. Add per-height memory checkpoints

Add low-overhead logs around each block height:

- before block fetch
- after block fetch/decode
- after input processing
- after output indexing
- after Pebble batch commit
- after height update
- after any cache cleanup

Each checkpoint should include height, tx count, input/output count when available, process RSS, Go heap, cgroup memory current, and elapsed time.

Acceptance for this task: one can identify whether memory rises during fetch, decode, index, commit, cache warmup, mempool merge, or cleanup.

### 5. Investigate and fix retained allocations

Use pprof and checkpoints to identify retained object owners. Likely areas to inspect:

- Large slices/maps retained beyond one block or batch.
- UTXO/address/spend aggregation maps not cleared after commit.
- `WarmupMemoryUTXO` cache behavior and `mem_utxo_max_count` enforcement.
- API or query caches holding references to large objects.
- Mempool merge structures retained in confirmed indexer paths.
- Pebble iterators, batches, closers, snapshots, or readers not closed.
- `append` patterns that keep large backing arrays alive.

Acceptance for this task: heap profile after several hours shows stable retained memory; top retained object owners are understood and documented.

### 6. Add soak/regression tests

Create a reproducible memory regression test that can run outside production:

- Run BTC mainnet indexing from a recent DB snapshot or a controlled block range near the observed production height.
- Enable the same production-like config: 2 CPU, 6 shards, effective balanced mode, 12GiB cgroup limit, mempool/ZMQ enabled for the final test.
- Record memory every minute and per height.
- Keep a profile artifact when memory crosses thresholds.

Acceptance for this task: developers can reproduce or disprove the growth pattern in staging before deploying to production.

## Production acceptance criteria

A fix should not be considered done until all criteria below pass.

### Observability

- `/debug/pprof` or equivalent is available in a safe, restricted way.
- Runtime logs include Go heap, RSS, cgroup memory, cgroup failcnt/events, goroutine count, and current block height.
- Logs include effective memory budget and Pebble budget at startup.
- High-water profile dumps are available before OOM.

### Stability

- With Docker memory limit at 12GiB and mempool/ZMQ enabled, `higun_btc` runs for at least 24 hours without cgroup OOM.
- `docker inspect` reports `OOMKilled=false` after the soak.
- `RestartCount` does not increase during the soak.
- `/cleanedHeight/get` continues to advance or remains consistent with upstream height.
- API port `8066` and compatibility port `8085` stay responsive.

### Memory behavior

- If the process is caught up and traffic is low, memory should reach a steady state instead of monotonically growing until cgroup OOM.
- cgroup memory should remain below 85% of the limit in steady state, or documented transient spikes must release within a bounded period.
- `proc_rss` and Go heap should not show unbounded monotonic growth across block heights.
- If `proc_rss` is stable but cgroup cache/failcnt remains high, the fix must explain and bound Pebble/file-cache pressure.

### Root-cause evidence

- The final PR should include before/after pprof or metrics evidence.
- The fix should state whether the root cause was:
  - Go heap/object retention,
  - Pebble/native/cache behavior,
  - cgroup-aware memory limit mismatch,
  - per-block transient allocation pressure,
  - mempool/ZMQ pressure,
  - or a combination.

### Regression protection

- Add tests or a repeatable benchmark/soak command that exercises the fixed path.
- Document the expected memory ceiling and test environment.
- CI or release checklist should include the memory regression check for BTC indexer changes touching indexing/storage/mempool.

## Current workaround status

- Raising memory to 12GiB is only a temporary mitigation. It did not solve OOM.
- Disabling mempool/ZMQ is not an acceptable long-term workaround and did not prevent OOM in the production experiment.
- Moving services off the host may reduce pressure, but current evidence still requires a Higun-side fix because `utxo_indexer` itself repeatedly grows to the cgroup ceiling.

## Definition of done

The expected deliverable is not only a code patch. It should include:

1. A code change that adds safe diagnostics.
2. A code change that bounds or fixes the identified memory growth.
3. A documented explanation of the root cause.
4. A staging or production-like soak result.
5. Production verification on `8.217.251.101` with mempool/ZMQ restored and no OOM for at least 24 hours.
