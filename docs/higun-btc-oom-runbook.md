# HIGUN BTC OOM Rollout Runbook

This runbook covers the guarded rollout for the BTC `higun_btc` OOM stabilization branch. It is written for production host `8.217.251.101`, container `higun_btc`, API port `8066`, compatibility API port `8085`, and diagnostics bind `127.0.0.1:6060`.

Do not mark the incident fixed from local tests alone. The production acceptance gate is a 24 hour soak with mempool/ZMQ enabled, no cgroup OOM, stable restart count, responsive APIs, and bounded memory behavior.

## Local Verification

Run these from the repository root before building or uploading an artifact:

```bash
export GOTOOLCHAIN=go1.24.3
git status --short
go test ./config ./diagnostics
CGO_ENABLED=0 go test ./storage ./indexer
CGO_ENABLED=0 go test ./blockchain -run 'TestConvertToIndexerBlockDoesNotPopulateArchiveMapsWhenDisabled|Test.*BTC|Test.*TxDetail' -count=1
make linux
```

The current `linux` make target runs `CGO_ENABLED=1 go build` without `-o`; depending on the build host, the output artifact may be named `higun`. Production kernel OOM logs identify the running process as `utxo_indexer`, so promote the artifact explicitly during deployment instead of assuming the local filename is the production filename.

On macOS, `make linux` requires a working Linux cgo cross compiler. If that toolchain is unavailable, use this only as a local code-level smoke build and build the production artifact on Linux:

```bash
export GOTOOLCHAIN=go1.24.3
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/higun-linux-smoke .
file /tmp/higun-linux-smoke
```

```bash
test -x ./utxo_indexer || test -x ./higun
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum ./utxo_indexer ./higun 2>/dev/null || true
else
  shasum -a 256 ./utxo_indexer ./higun 2>/dev/null || true
fi
```

## Production Preflight

Use a read-only pass first. Capture the current container generation, restart count, OOM status, memory limit, mounts, API status, and monitor trend before changing anything.

```bash
ssh root@8.217.251.101 '
set -eu
date -Is
docker inspect higun_btc --format "id={{.Id}} restart={{.RestartCount}} oom={{.State.OOMKilled}} status={{.State.Status}} started={{.State.StartedAt}} mem={{.HostConfig.Memory}}"
docker stats higun_btc --no-stream --format "mem={{.MemUsage}} perc={{.MemPerc}} cpu={{.CPUPerc}}"
docker exec higun_btc sh -lc "grep -E \"^(VmRSS|VmHWM):\" /proc/1/status || true"
docker exec higun_btc sh -lc "grep -R \"diagnostics:\" -n /app/config.yaml /config/config.yaml 2>/dev/null || true"
curl -fsS --max-time 5 http://127.0.0.1:8066/cleanedHeight/get || true
curl -sS -o /tmp/higun_btc_8085.out -w "compat_8085_http=%{http_code}\n" --max-time 5 http://127.0.0.1:8085/ || true
test -f /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv && tail -20 /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv || true
'
```

If diagnostics are enabled, verify they are bound to localhost only:

```bash
ssh root@8.217.251.101 '
curl -fsS --max-time 5 http://127.0.0.1:6060/debug/vars >/tmp/higun_debug_vars.json
curl -fsS --max-time 5 http://127.0.0.1:6060/debug/pprof/ >/tmp/higun_pprof_index.html
ss -ltnp | grep -E ":(6060|8066|8085)\b" || true
'
```

## Backup

Create a timestamped rollback bundle before replacing the binary or config.

```bash
ssh root@8.217.251.101 '
set -eu
ts=$(date +%Y%m%d-%H%M%S)
backup=/date/higun_btc/rollback-$ts
mkdir -p "$backup"
docker inspect higun_btc > "$backup/docker-inspect.json"
docker logs --tail 5000 higun_btc > "$backup/docker-tail.log" 2>&1 || true
for p in /date/higun_btc/utxo_indexer /date/higun_btc/config.yaml /date/higun_btc/docker-compose.yml; do
  test -e "$p" && cp -a "$p" "$backup/"
done
if command -v sha256sum >/dev/null 2>&1; then
  find "$backup" -maxdepth 1 -type f -exec sha256sum {} \; > "$backup/SHA256SUMS"
fi
echo "$backup"
'
```

Keep the printed backup directory. It is required for rollback.

## Deploy

Upload the locally built artifact to a `.new` path, then promote it inside the production directory. Adjust the local artifact path if `make linux` produced `./higun`.

```bash
ARTIFACT=./utxo_indexer
test -x "$ARTIFACT" || ARTIFACT=./higun
test -x "$ARTIFACT"

scp "$ARTIFACT" root@8.217.251.101:/date/higun_btc/utxo_indexer.new
ssh root@8.217.251.101 '
set -eu
cd /date/higun_btc
chmod +x utxo_indexer.new
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum utxo_indexer utxo_indexer.new 2>/dev/null || true
fi
mv utxo_indexer utxo_indexer.prev
mv utxo_indexer.new utxo_indexer
docker restart higun_btc
'
```

If config changes are required to enable diagnostics, keep the bind at `127.0.0.1:6060` and restart only after the backup bundle exists.

```yaml
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
memory_budget:
  use_cgroup_limit: true
  go_memory_limit_percent: 65
  pebble_cache_percent: 12
  pebble_memtable_percent: 8
  reserve_percent: 15
  main_store_count: 5
```

Restore mempool/ZMQ for the final acceptance soak. The previous incident experiment showed disabling ZMQ did not prevent OOM, so a disabled mempool is not a valid final state.

## Immediate Post-Restart Checks

Run these checks immediately, then again after 5 and 15 minutes.

```bash
ssh root@8.217.251.101 '
set -eu
docker inspect higun_btc --format "restart={{.RestartCount}} oom={{.State.OOMKilled}} status={{.State.Status}} started={{.State.StartedAt}} mem={{.HostConfig.Memory}}"
docker logs --tail 120 higun_btc
docker stats higun_btc --no-stream --format "mem={{.MemUsage}} perc={{.MemPerc}} cpu={{.CPUPerc}}"
curl -fsS --max-time 5 http://127.0.0.1:8066/cleanedHeight/get
curl -sS -o /tmp/higun_btc_8085.out -w "compat_8085_http=%{http_code}\n" --max-time 5 http://127.0.0.1:8085/ || true
curl -fsS --max-time 5 http://127.0.0.1:6060/debug/vars >/tmp/higun_debug_vars.json || true
test -f /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv && tail -20 /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv || true
'
```

The logs should show the effective memory budget, cgroup limit, Go memory limit, total Pebble cache/memtable budgets, and per-height memory checkpoints.

## Soak Schedule

Record evidence at these intervals:

- T+15 minutes: service responsive, height visible, no immediate OOM, diagnostics reachable locally.
- T+1 hour: restart count unchanged, cgroup memory below 85% or transient spike explained by logs.
- T+6 hours: memory trend is not monotonically climbing to the cgroup ceiling.
- T+24 hours: final acceptance check with mempool/ZMQ enabled.

Use the existing side monitor plus container/runtime data:

```bash
ssh root@8.217.251.101 '
set -eu
date -Is
docker inspect higun_btc --format "restart={{.RestartCount}} oom={{.State.OOMKilled}} status={{.State.Status}} started={{.State.StartedAt}} mem={{.HostConfig.Memory}}"
docker stats higun_btc --no-stream --format "mem={{.MemUsage}} perc={{.MemPerc}} cpu={{.CPUPerc}}"
curl -fsS --max-time 5 http://127.0.0.1:8066/cleanedHeight/get || true
curl -fsS --max-time 5 http://127.0.0.1:6060/debug/vars >/tmp/higun_debug_vars.json || true
test -f /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv && tail -60 /date/higun_btc/monitoring/higun_btc_mem_watch-latest.tsv || true
'
```

If a high-water profile is dumped, preserve the profile directory path and copy the profile artifacts into the final evidence bundle.

## Rollback

Rollback if the container OOMs again, APIs fail after restart, diagnostics expose anything other than localhost, or memory crosses the cgroup ceiling trend without releasing.

```bash
ssh root@8.217.251.101 '
set -eu
cd /date/higun_btc
test -x utxo_indexer.prev
mv utxo_indexer utxo_indexer.failed.$(date +%Y%m%d-%H%M%S)
mv utxo_indexer.prev utxo_indexer
docker restart higun_btc
docker inspect higun_btc --format "restart={{.RestartCount}} oom={{.State.OOMKilled}} status={{.State.Status}} started={{.State.StartedAt}}"
'
```

If config was changed, restore it from the timestamped rollback bundle and restart the container again.

## Evidence Template

Fill this out before declaring the production gate complete.

```text
branch:
commit:
artifact_sha256:
host: 8.217.251.101
container: higun_btc
docker_memory_limit_bytes:
mempool_zmq_enabled: yes/no
diagnostics_bind:
backup_dir:

preflight_restart_count:
preflight_oom_killed:
preflight_cleaned_height:

post_restart_started_at:
post_restart_restart_count:
post_restart_oom_killed:
post_restart_cleaned_height:

t+15m_mem:
t+15m_cleaned_height:
t+15m_notes:

t+1h_mem:
t+1h_cleaned_height:
t+1h_restart_count:
t+1h_notes:

t+6h_mem:
t+6h_cleaned_height:
t+6h_restart_count:
t+6h_notes:

t+24h_mem:
t+24h_cleaned_height:
t+24h_restart_count:
t+24h_oom_killed:
t+24h_notes:

profile_artifacts:
root_cause_evidence:
rollback_needed: yes/no
```
