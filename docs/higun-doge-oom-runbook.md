# HIGUN DOGE OOM Rollout Runbook

This runbook covers the guarded rollout of the OOM stabilization branch (main `e89a772`) to the DOGE `higun_doge` indexer. It is the DOGE equivalent of `docs/higun-btc-oom-runbook.md`. It is written for production host `8.217.14.206`, container `higun_doge`, program directory `/doge/higun`, API port `8066`, and diagnostics bind `127.0.0.1:6060`.

Do not mark the incident fixed from local tests alone. The production acceptance gate is a 24 hour soak with mempool/ZMQ enabled, the 12h cron restart suspended, no cgroup OOM, stable restart count, responsive API, bounded memory behavior, and RSS peak below 6 GiB (25% headroom under the 8 GiB cgroup limit).

## Background

The deployed `higun_doge` binary was built 2026-06-09 01:33 CST, hours before the entire OOM stabilization series landed on main. It runs with no `debug.SetMemoryLimit`, no cgroup-aware budget, no bounded Pebble budgets, no diagnostics server, and with the continuous `SyncBaseCount` scan (20 minute full-shard iterator scans over the address and UTXO stores) always enabled. Three cgroup OOM kills were recorded: 2026-08-29, 2026-09-07, 2026-09-18 (all `CONSTRAINT_MEMCG`, task `utxo_indexer`, anon RSS ~8.1 GB).

The fix version is main `e89a772`, the same commit running on MVC production since 2026-09-02. It includes:

- cgroup-aware memory budget + `debug.SetMemoryLimit` (43b9f63, config/auto.go, main.go)
- bounded Pebble cache/memtable budgets across all stores (b7d4356, daf42a9, storage/pebble.go)
- per-height memory checkpoints (b7725cd, blockchain/client.go, indexer/utxo.go, diagnostics/checkpoint.go)
- reduced spend-query allocations (11f49df, storage/pebble.go)
- release of per-block archive data after indexing (a209da1, blockchain/client.go)
- safe localhost-only diagnostics server with high-water heap/goroutine dumps (00e47ff, e696d64, daf42a9, diagnostics/)
- `SyncBaseCount` disabled by default (f91dc1b); touched-balance-row sync disabled by default with stale-row read fix (6a5da9e, efe7b81)
- mempool rebuild fixes including a ZMQ context leak fix (964e07e, fc68f33, f5fdbbe, 349c559)

All of these except the BTC adapter internals of b7725cd/a209da1 are chain-agnostic and apply to DOGE directly. The DOGE adapter only allocates its per-block archive maps empty (they are never populated), so the BTC-specific adapter part is not needed on DOGE.

## Local Verification

Run from the repository root before building or uploading an artifact (last run 2026-09-24 on e89a772: all pass):

```bash
export GOTOOLCHAIN=go1.24.0
git status --short
go test ./config ./diagnostics
CGO_ENABLED=0 go test ./storage ./indexer
CGO_ENABLED=0 go test ./blockchain -run 'TestConvertToIndexerBlockDoesNotPopulateArchiveMapsWhenDisabled|Test.*BTC|Test.*TxDetail' -count=1
```

Build the production artifact (same command as the MVC SOP; `make linux` does not work on macOS):

```bash
cd /Users/tusm/Documents/MetaID_Projects/higun
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=go1.24.0 \
  go build -ldflags="-checklinkname=0" -o /tmp/utxo_indexer_e89a772 .
shasum -a 256 /tmp/utxo_indexer_e89a772
```

Expected artifact: ~38 MB, static ELF x86-64. Built twice from a clean tree at e89a772 on 2026-09-24, both builds produced:

```text
sha256: 4953dd7d648497ca7e79a545f00ea515dfb1f3659b67be4907c7c399bde698a9
```

Verify the uploaded file on the server against this hash before promotion. If a rebuild produces a different hash, the build environment differed (toolchain, dirty tree, flags) — do not deploy until the difference is explained.

## Required Config Changes

The current `/doge/higun/config.yaml` has `memory_gb: 2` and no `diagnostics`/`memory_budget` sections. With the new binary, `config.ChooseMemoryBudget` takes `min(memory_gb, cgroup_limit)` when `use_cgroup_limit` is true (the default). Leaving `memory_gb: 2` would cap the process at 2 GiB effective and set a 1.3 GiB Go soft limit, causing heavy GC churn. `memory_gb` must be raised to match the container limit.

Apply this diff to `/doge/higun/config.yaml`:

```yaml
memory_gb: 8        # was 2; must be >= the 8 GiB cgroup limit so the cgroup value rules

diagnostics:
  enabled: true
  bind: "127.0.0.1:6060"
  pprof_enabled: true
  expvar_enabled: true
  sample_interval_seconds: 30
  profile_dir: "data/diagnostics"
  high_water_profile_percents: [70, 80, 90]
  memory_log_every_n_blocks: 1
  memory_checkpoint_min_percent: 0

# optional; these are the built-in defaults and are correct for an 8 GiB container
memory_budget:
  use_cgroup_limit: true
  go_memory_limit_percent: 65
  pebble_cache_percent: 12
  pebble_memtable_percent: 8
  reserve_percent: 15
  main_store_count: 5
```

With these values on the 8 GiB cgroup the process gets: Go soft memory limit ~5.2 GiB, total Pebble block-cache budget ~1.0 GiB split across 5 main stores, memtable budget ~655 MiB split across stores and shards, max 1 concurrent compaction per store.

Keep `mem_utxo_max_count: 100000` unchanged for the soak (one variable at a time). The `(~4.2GB)` comment on that line is stale — it describes the repo example value 14000000 (~300 bytes/entry). 100000 entries cost ~20-30 MB.

`profile_dir: "data/diagnostics"` resolves inside the container at `/app/data/diagnostics`, which is mounted from `/doge/higun/data/diagnostics` on the host, so high-water dumps survive restarts.

Do not set `sync_base_count_enabled` or `sync_touched_balance_rows`; their defaults (false) are what we want.

## Production Preflight

```bash
ssh root@8.217.14.206 '
set -eu
date -Is
docker inspect higun_doge --format "id={{.Id}} restart={{.RestartCount}} oom={{.State.OOMKilled}} status={{.State.Status}} started={{.State.StartedAt}} mem={{.HostConfig.Memory}}"
docker stats higun_doge --no-stream --format "mem={{.MemUsage}} perc={{.MemPerc}} cpu={{.CPUPerc}}"
docker exec higun_doge sh -lc "grep -E \"^(VmRSS|VmHWM):\" /proc/1/status || true"
curl -fsS --max-time 5 http://127.0.0.1:8066/cleanedHeight/get || true
sha256sum /doge/higun/utxo_indexer
'
```

Expected current binary sha256: `c28eec1f3663cb2f95386fedc90b80abc226592087f2429ab1e93d7674e99f2f`.

## Backup

```bash
ssh root@8.217.14.206 '
set -eu
ts=$(date +%Y%m%d-%H%M%S)
backup=/doge/higun/rollback-$ts
mkdir -p "$backup"
docker inspect higun_doge > "$backup/docker-inspect.json"
docker logs --tail 5000 higun_doge > "$backup/docker-tail.log" 2>&1 || true
cp -a /doge/higun/utxo_indexer /doge/higun/config.yaml "$backup/"
sha256sum "$backup"/* > "$backup/SHA256SUMS" || true
echo "$backup"
'
```

## Deploy

```bash
scp /tmp/utxo_indexer_e89a772 root@8.217.14.206:/doge/higun/utxo_indexer.new
ssh root@8.217.14.206 '
set -eu
cd /doge/higun
chmod +x utxo_indexer.new
sha256sum utxo_indexer.new   # must equal 4953dd7d648497ca7e79a545f00ea515dfb1f3659b67be4907c7c399bde698a9
cp -p config.yaml config.yaml.bak
# apply the config diff from the section above, then:
mv utxo_indexer utxo_indexer.prev
mv utxo_indexer.new utxo_indexer
docker restart higun_doge
'
```

## Suspend The 12h Cron Restart For The Soak

The 12h restart (`/etc/cron.d/higun-doge-daily-restart`) masks OOM evidence (it resets `State.OOMKilled` and `RestartCount`) and itself creates the peak window (catch-up indexing + warmup + mempool rebuild right after each restart). Suspend it for the 24h soak and only re-enable (or remove) it after acceptance:

```bash
ssh root@8.217.14.206 'sed -i "s|^\([^#].*higun_doge.*\)|# soak-suspend \1|" /etc/cron.d/higun-doge-daily-restart && crontab -l >/dev/null 2>&1 || true; cat /etc/cron.d/higun-doge-daily-restart'
```

## Immediate Post-Restart Checks

Run immediately, then again after 5 and 15 minutes:

```bash
ssh root@8.217.14.206 '
set -eu
docker inspect higun_doge --format "restart={{.RestartCount}} oom={{.State.OOMKilled}} status={{.State.Status}} started={{.State.StartedAt}}"
docker logs --tail 150 higun_doge
docker stats higun_doge --no-stream --format "mem={{.MemUsage}} perc={{.MemPerc}} cpu={{.CPUPerc}}"
curl -fsS --max-time 5 http://127.0.0.1:8066/cleanedHeight/get
curl -fsS --max-time 5 http://127.0.0.1:6060/debug/memory
'
```

The startup logs must show:

- `[MemoryBudget] Applied Go memory limit: ... chosen=~5.5e9 cgroup=8589934592` (chosen ≈ 65% of 8 GiB)
- one `[PebbleBudget] store=...` line per store with bounded `cache_per_store` / `memtable_per_shard`
- `[MemoryCheckpoint] height=... phase=after_block_fetch|after_income|after_spend|after_balance_index|after_height_update` lines once block sync resumes

## Soak Schedule

- T+15 minutes: service responsive, height advancing, diagnostics reachable locally, no immediate OOM.
- T+1 hour: restart count unchanged, cgroup memory below 85% or transient spike explained by checkpoint logs.
- T+6 hours: memory trend is not monotonically climbing to the cgroup ceiling.
- T+24 hours: final acceptance check with mempool/ZMQ enabled and cron restart still suspended.

Acceptance gates (all must pass):

1. 24h with mempool/ZMQ enabled, no cron restart, no cgroup OOM (`docker events` shows no `die` with exitCode 137; `docker inspect` shows `OOMKilled=false`, unchanged `RestartCount`).
2. Process RSS peak < 6 GiB over the soak (from the mem-watch TSV; 25% headroom under the 8 GiB limit).
3. `/cleanedHeight/get` advancing with upstream; API 8066 responsive.
4. Memory checkpoints show cgroup usage releasing after peaks, not ratcheting upward.
5. If any high-water profile dumped, preserve it in the evidence bundle (this is expected data, not a failure).

## Side Monitor (DOGE equivalent of higun_btc_mem_watch)

Install once on the host:

```bash
ssh root@8.217.14.206 'cat > /usr/local/bin/higun_doge_mem_watch.sh <<"EOF"
#!/usr/bin/env bash
set -u
out_dir=/doge/higun/monitoring
mkdir -p "$out_dir"
out=$out_dir/higun_doge_mem_watch-$(date +%Y%m%d).tsv
latest=$out_dir/higun_doge_mem_watch-latest.tsv
ts=$(date -Is)
id=$(docker inspect -f "{{.Id}}" higun_doge 2>/dev/null || echo "")
pid=$(docker inspect -f "{{.State.Pid}}" higun_doge 2>/dev/null || echo 0)
rss_kb=0; hwm_kb=0
if [ "${pid:-0}" != "0" ]; then
  rss_kb=$(awk "/^VmRSS:/{print \$2}" /proc/$pid/status 2>/dev/null || echo 0)
  hwm_kb=$(awk "/^VmHWM:/{print \$2}" /proc/$pid/status 2>/dev/null || echo 0)
fi
cgcur=""; cgmax=""
for base in "/sys/fs/cgroup/system.slice/docker-$id.scope" "/sys/fs/cgroup/system.slice/docker-$id.scope/memory"; do
  if [ -r "$base/memory.current" ]; then cgcur=$(cat "$base/memory.current"); cgmax=$(cat "$base/memory.max"); break; fi
  if [ -r "$base/memory.usage_in_bytes" ]; then cgcur=$(cat "$base/memory.usage_in_bytes"); cgmax=$(cat "$base/memory.limit_in_bytes"); break; fi
done
line="$ts	pid=$pid	rss_kb=$rss_kb	hwm_kb=$hwm_kb	cgroup_current=${cgcur:-na}	cgroup_limit=${cgmax:-na}"
echo "$line" >> "$out"
cp "$out" "$latest"
EOF
chmod +x /usr/local/bin/higun_doge_mem_watch.sh
echo "* * * * * root /usr/local/bin/higun_doge_mem_watch.sh" > /etc/cron.d/higun-doge-mem-watch
'
```

After the new binary is up, `/debug/memory` provides the same data in-process; a 1-minute cron TSV remains useful as out-of-process evidence that survives an OOM kill.

## Capturing Profiles On Request

With diagnostics enabled, from the host:

```bash
# live JSON snapshot (heap, goroutines, cgroup current/limit, RSS/HWM)
curl -fsS http://127.0.0.1:6060/debug/memory

# heap profile (in-use), for go tool pprof
curl -fsS http://127.0.0.1:6060/debug/pprof/heap > heap-$(date +%Y%m%d-%H%M%S).pprof

# goroutine dump (text)
curl -fsS "http://127.0.0.1:6060/debug/pprof/goroutine?debug=2" > goroutine-$(date +%Y%m%d-%H%M%S).txt

# 30s CPU profile if indexing stalls
curl -fsS http://127.0.0.1:6060/debug/pprof/profile?seconds=30 > cpu-$(date +%Y%m%d-%H%M%S).pprof
```

High-water dumps at 70/80/90% of the cgroup limit are automatic and land in `/doge/higun/data/diagnostics/heap-{70,80,90}-*.pprof` and `goroutine-{70,80,90}-*.txt`. Copy them off the host into the evidence bundle before any restart.

The old binary exposes no pprof endpoint; live profiling only becomes possible after this deploy.

## Rollback

Rollback if the container OOMs again, the API fails after restart, diagnostics binds to anything other than localhost, or memory crosses the cgroup ceiling without releasing.

```bash
ssh root@8.217.14.206 '
set -eu
cd /doge/higun
test -x utxo_indexer.prev
mv utxo_indexer utxo_indexer.failed.$(date +%Y%m%d-%H%M%S)
mv utxo_indexer.prev utxo_indexer
cp -p config.yaml.bak config.yaml   # only if config was changed
docker restart higun_doge
docker inspect higun_doge --format "restart={{.RestartCount}} oom={{.State.OOMKilled}} status={{.State.Status}} started={{.State.StartedAt}}"
'
```

Also re-enable the 12h cron restart if the rollout is aborted (uncomment the line in `/etc/cron.d/higun-doge-daily-restart`).

## Evidence Template

```text
branch: main
commit: e89a772
artifact_sha256: 4953dd7d648497ca7e79a545f00ea515dfb1f3659b67be4907c7c399bde698a9
host: 8.217.14.206
container: higun_doge
docker_memory_limit_bytes: 8589934592
mempool_zmq_enabled: yes
diagnostics_bind: 127.0.0.1:6060
backup_dir:
cron_restart_suspended: yes/no

preflight_restart_count:
preflight_oom_killed:
preflight_cleaned_height:
preflight_binary_sha256: c28eec1f3663cb2f95386fedc90b80abc226592087f2429ab1e93d7674e99f2f

post_restart_started_at:
post_restart_go_memory_limit:
post_restart_cleaned_height:

t+15m_mem:
t+1h_mem:
t+6h_mem:
t+24h_mem:
t+24h_rss_peak_gib:
t+24h_restart_count:
t+24h_oom_killed:

profile_artifacts:
root_cause_evidence:
rollback_needed: yes/no
```
