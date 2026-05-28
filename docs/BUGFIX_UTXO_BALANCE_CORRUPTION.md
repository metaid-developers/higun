# HIGUN UTXO 余额数据损坏修复

## 问题

多位 MVC 用户反馈余额显示不正确，且会出现 `[-25] Missing inputs` 或 `[-26]258: txn-mempool-conflict` 错误，导致无法转账。即使重索引（`/blocks/reindex`）后问题依然存在。

具体案例：
- `1AxUdSkVdDyDreYSYVoDRFeyS1pvQdvcJx` — UTXO 假阳性（0.875 MVC 显示未花费但实际已花费）
- `1P6q4EijHrUT2wMXTGHUorgK4fC3mhigf1` — 同上（7.06 MVC 假阳性）
- `161jhzUUAo9MbjfEKtnCqf6x1yBx4kMQBn` — 余额虚高 750 MVC，真实余额仅 18 MVC
- `12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ` — 余额虚高 521 MVC，真实余额约 440 MVC

## 根因

### 第一层：旧 Merger 不幂等（已通过 Fix 1 修复）

**首次部署（2026-02-28）** 时使用的 Pebble 默认 Merger 是简单字节拼接。HIGUN 每次重启后可能重处理部分区块，`batch.Merge()` 会追加重复/损坏的 segment 到已有值后面。形成：

```
addressStore value: valid,corrupted,valid,corrupted,valid
```

### 第二层：reindex 无法清除已有损坏（本次需修复的核心问题）

即使更新了自定义 Merger（Fix 1，2026-05-02 部署），reindex 时仍然使用 `BulkMergeMapConcurrent`（内部调用 `batch.Merge`）。Merge 只能**追加**数据，无法**删除**已有的损坏条目。

```
reindex 前: valid,corrupted,corrupted
reindex 后: valid,corrupted,corrupted,valid  ← 烂数据还在
```

`GetUTXOs` 解析时，损坏条目被 `len(incomes) < 3` 或 `ParseInt` 静默跳过，导致大量合法 UTXO 不返回。但余额指数（balanceStore）中的数值是正确的，所以余额显示不变——实际能用的 UTXO 却很有限。

### 第三层：自定义 Merger 不能重排 segment

Fix 1 的自定义 Merger 用 `sort.Strings()` 做确定性输出。这个对 `addressStore` / `spendStore` 问题不大，但 `utxoStore` 的 value 是按交易输出下标排列的：

```
txid -> output0,output1,output2
```

`QueryUTXODetails(txid:index)` 依赖列表位置找 output index。如果 Merger 把输出 segment 按字典序重排，`index=0/1/2` 会指向错误 output，进一步造成 spendStore 记错地址/金额，余额和可用 UTXO 都会错。

### 影响范围

所有在 2026-02-28（HIGUN 首次启动，高度 162,274）到 2026-05-02（Fix 1 部署）之间被写入过 addressStore / spendStore 的地址，都可能存在损坏条目。**重索引无法修复的原因是代码中缺少 reindex 模式的对等支持。**

## 代码修复

### 修改 1：`storage/pebble.go` — Merger 去重但保留首次出现顺序

`dedupMerger.Finish()` 不再对 segment 排序，而是按旧值到新值的扫描顺序保留首次出现的 segment。这样既能去重，又不会破坏 `utxoStore` 的交易输出下标语义。

### 修改 2：`indexer/utxo.go` — reindex 不再按块删除地址历史

函数签名变更：
```go
// 旧
func (i *UTXOIndexer) IndexBlock(block *Block, allBlock *Block, updateHeight bool, blockTimeStr string) (...)

// 新
func (i *UTXOIndexer) IndexBlock(block *Block, allBlock *Block, updateHeight bool, blockTimeStr string, reindex bool) (...)
```

注意：不能在每个区块里删除该区块涉及地址的整条历史记录。否则同一地址在后续区块再次出现时，会把前面已经重建好的 UTXO 也删掉。`reindex=true` 现在用于避免历史重索引误清 mempool，并在写入每个区块的 txid 前先覆盖旧的 `utxoStore` txid value。

### 修改 3：`indexer/utxo.go` — 新增一次性历史重建 reset

新增：

```go
func (i *UTXOIndexer) ResetConfirmedHistoryForReindex() error
```

它会在全量重索引开始前一次性清空派生索引：

- `addressStore`
- `spendStore`
- `address_balance`
- `balance_rank`
- 内存 UTXO cache

并把 balance index 标记为 not ready。`utxoStore` 不做全库清空，避免在生产磁盘空间紧张时重写 87GB UTXO 库；重索引到每个区块时，会对当前区块涉及的 txid 先 delete 再 merge，用干净 value 替换旧值。

清空派生索引使用 Pebble shard 级 `DeleteRange`，不逐条读取 value，避免大地址历史 value 在 reset 阶段触发 OOM。

### 修改 4：`blockchain/client.go` — `ProcessBlock` 加 `reindex` 参数

```go
// 旧
func (c *Client) ProcessBlock(idx *indexer.UTXOIndexer, height int, updateHeight bool, currentHeight int) error

// 新
func (c *Client) ProcessBlock(idx *indexer.UTXOIndexer, height int, updateHeight bool, currentHeight int, reindex bool) error
```

函数内部，两处 `idx.IndexBlock(...)` 调用透传 `reindex` 参数。

普通同步调用处（line ~340）传 `false`：
```go
c.ProcessBlock(idx, height, true, currentHeight, false)
```

### 修改 5：`api/server.go` — reindex 支持显式 reset

```go
// 旧
s.bcClient.ProcessBlock(s.indexer, height, false, height)

// 新
s.bcClient.ProcessBlock(s.indexer, height, false, height, true)
```

当请求带 `reset=true` 时，handler 会先执行 `ResetConfirmedHistoryForReindex()`，再开始按高度重索引。

reindex 期间会占用一个全局 reindex 状态位，普通区块同步循环会暂停写入，避免 reset/rebuild 和实时同步并发导致 `balanceStore` 增量重复。

## 部署步骤

### 1. 编译

```bash
# 在服务器上，用 Docker golang 镜像编译
docker run --rm -v /path/to/higun_src:/src -w /src \
  -e CGO_ENABLED=1 -e GOOS=linux -e GOARCH=amd64 \
  golang:1.24 go build -o /src/utxo_indexer ./main.go
```

### 2. 部署（停机 ~30 秒）

```bash
# 备份旧二进制
cp /mvc/higun_mvc/utxo_indexer /mvc/higun_mvc/utxo_indexer.bak.$(date +%Y%m%d_%H%M%S)

# 停机替换
docker stop higun_new
cp /tmp/higun_build/utxo_indexer /mvc/higun_mvc/utxo_indexer
docker start higun_new
```

### 3. 全量重索引（在线，~2-3 小时）

```bash
# 从 HIGUN 首次启动高度开始，全量重索引，并先重置地址/花费/余额派生索引
curl -s 'http://127.0.0.1:8085/blocks/reindex?start=162274&end=<当前高度>&reset=true'
```

`reset=true` 只应用于这种全量历史重建。普通小范围 reindex 不要带这个参数，否则会清掉范围外地址/花费/余额历史。`utxoStore` 会在 reindex 过程中按 txid 逐块覆盖，不会先全库清空。

### 4. 验证

```bash
# 抽查之前的问题地址
curl -s http://127.0.0.1:8085/balance?address=161jhzUUAo9MbjfEKtnCqf6x1yBx4kMQBn
curl -s http://127.0.0.1:8085/balance?address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ
```

## 回滚

```bash
docker stop higun_new
cp /mvc/higun_mvc/utxo_indexer.bak.XXXXXXXX /mvc/higun_mvc/utxo_indexer
docker start higun_new
```

## 修改文件清单

| 文件 | 改动 |
|------|------|
| `storage/pebble.go` | Merger 去重保序；新增 shard 级 `Clear()`，避免 reset 读取大 value |
| `indexer/utxo.go` | `IndexBlock` 加 `reindex` 参数；新增一次性 reset 和 reindex 状态位；reindex 模式避免误清 mempool |
| `indexer/balance_index.go` | `clearStore()` 改用 Pebble range delete |
| `blockchain/client.go` | `ProcessBlock` 加 `reindex` 参数 + 透传；reindex 期间暂停普通同步 |
| `api/server.go` | reindex handler 支持 `reset=true`，并拒绝并发 reindex |
