# HIGUN BTC 陈旧余额（stale tracked balance）事故 runbook

适用主机：`8.217.251.101`（内网 `172.31.165.190`），容器 `higun_btc`，API `8066`，钱包兼容入口 `8085`（`btc-api-compat-proxy` → `127.0.0.1:8066`）。

## 症状

- 用户钱包余额与链上不符：已花掉的钱仍显示（虚高），或新收到的钱不显示（偏低）。
- `/balance?address=X` 与 `/utxos?address=X` 求和（含粉尘口径）不一致；`/utxos`、`/utxos/history` 正常，说明 income/spend 历史库正确。
- 交叉验证链上真值：BTC 节点 RPC `scantxoutset`（节点 `172.31.168.214:8332`，凭据见服务器 `/date/higun_btc/config.yaml`）。

## 根因

confirmed-balance index 从未 bootstrap（meta `balance_index_ready="0"`），且 `sync_touched_balance_rows` 未开启（`6a5da9e` 起默认 false）。此时 `GetBalance` 对存在 `Tracked` 缓存行的地址直接返回缓存值；缓存行无人刷新，地址再有新交易后即冻结在旧值。任何「查询过、之后又有新交易」的地址都可能中招。

历史事故：

- 2026-08-17：地址 `1PutNR27YL7p...` 余额偏低（漏一笔 470,244 sat 收入）。止血方式为清空 address_balance 全部 tracked 行。
- 2026-10-20：同一地址余额虚高（491,812 sat 已花掉仍显示，真值 22,366 sat 粉尘）——即 8·17 清空后 tracked 行重新积累、再次冻结的复发。另有 `152pNTzx...`（tracked 30,814 vs 真值 47,667）。

## 修复（已随 main 落地）

`efe7b81`（fix: ignore stale tracked balance rows when row sync disabled）：当 index 未 ready 且 touched 同步关闭时，`GetBalance` 不再读写 tracked 行，一律按 income/spend 历史实时重算。陈旧行留在 store 中但永不被返回。

### 部署记录

| 日期 | commit | sha256 | 备份 | 备注 |
|---|---|---|---|---|
| 2026-06-11 | 6a5da9e | abccbd1d… | `utxo_indexer.bak.20260410_*` 等 | 引入 stale 行问题（touched 同步默认关闭）；CGO，服务器构建 |
| 2026-10-20 | main 7aa3ddb | b5bb48b5… | `utxo_indexer.bak.20261020.6a5da9e` + `rollback-20261020-123451/` | 部署 efe7b81 修复；CGO，服务器构建（`build-src-7aa3ddb`，go1.24.3） |

服务器构建方式（与既有流程一致）：

```bash
# 本地导出源码
git archive --format=tar.gz -o /tmp/higun-<rev>.tar.gz main
scp /tmp/higun-<rev>.tar.gz www@8.217.251.101:/tmp/
# 服务器（sudo，密码见 servers/8.217.251.101.md）
mkdir -p /date/higun_btc/build-src-<rev> && tar xzf /tmp/higun-<rev>.tar.gz -C /date/higun_btc/build-src-<rev>
cd /date/higun_btc/build-src-<rev>
export PATH=/date/higun_btc/build-tools/go1.24.3/bin:$PATH
export GOCACHE=/date/higun_btc/build-cache/go-build GOPATH=/date/higun_btc/build-cache/gopath HOME=/root
go build -o /tmp/utxo_indexer.<rev> .          # CGO 默认开，与生产一致
go build -o /tmp/check_balance_index.<rev> ./tools/check_balance_index
```

部署（原子替换 + 备份 + 重启）与验证流程见 `docs/higun-btc-oom-runbook.md` 的 Backup / Deploy / Immediate Post-Restart Checks 各节，照用即可；停机窗口仅容器重启的数十秒。

### 部署后验证（2026-10-20 实测）

- `check_balance_index`（修复前，停机窗口内跑）：450 行 tracked 全部无维护；`1PutNR...` tracked 491,812/40 vs 历史 22,366/40；`152pNT...` tracked 30,814/6 vs 历史 47,667/7。
- 部署后 `/balance`：`1PutNR...` = 22,366 sat（unsafe 20,748），`152pNT...` = 47,667 sat（unsafe 1,638），与节点 `scantxoutset` 完全一致（22,366 + 47,667 = 70,033）。
- 公网端到端 `www.metalet.space/wallet-api/v3/address/btc-balance` 同步恢复正确。
- 启动日志含 `[BalanceIndex] touched balance row sync disabled by config`（预期的安全模式）。

## 遗留与根治

当前形态（index 未 ready + 同步关闭）余额永远正确，但每次 `/balance` 都做历史扫描（无缓存），且 `/rich-list` 不可用。根治需一次性全量重建并置 ready，之后逐块增量维护：

```bash
# 必须停容器全程运行；支持进度续跑（max-addresses 分批）
docker stop higun_btc
cd /date/higun_btc
/tmp/bootstrap_balance_index.<rev> -data-dir data -chain btc -network mainnet \
  -shards 6 -cpu 2 -memory 4 -commit-batch-size 500 -sleep-ms 50
docker start higun_btc
```

2 vCPU/7.4G 机器上预计数小时到 1-2 天（income 431G 顺序扫 + 对 669G spend store 的逐地址点查），期间钱包 BTC 余额/UTXO API 不可用，需单独排期低峰窗口或先迁移到更大规格机器。完成后 `balance_index_ready="1"`，`/balance` 走增量行，`/rich-list` 恢复。
