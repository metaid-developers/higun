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

### 影响范围

所有在 2026-02-28（HIGUN 首次启动，高度 162,274）到 2026-05-02（Fix 1 部署）之间被写入过 addressStore / spendStore 的地址，都可能存在损坏条目。**重索引无法修复的原因是代码中缺少 reindex 模式的对等支持。**

## 代码修复

### 修改 1：`storage/pebble.go` — 新增 `DeleteKey` 方法

```go
// 在 PebbleStore 结构体后添加
func (s *PebbleStore) DeleteKey(key string) error {
    db := s.getShard(key)
    return db.Delete([]byte(key), pebble.Sync)
}
```

与此前已部署的 `DeleteMapKeys` 方法配合使用（该方法已在 Fix 1 时添加）。

### 修改 2：`indexer/utxo.go` — `IndexBlock` 加 `reindex` 参数，reindex 时先删后写

函数签名变更：
```go
// 旧
func (i *UTXOIndexer) IndexBlock(block *Block, allBlock *Block, updateHeight bool, blockTimeStr string) (...)

// 新
func (i *UTXOIndexer) IndexBlock(block *Block, allBlock *Block, updateHeight bool, blockTimeStr string, reindex bool) (...)
```

在 `IndexBlock` 入口处（`var balanceDeltas` 之前），添加 reindex 预处理逻辑：

```go
// 重索引时，先删除该块涉及的所有地址在 addressStore + spendStore 中的旧数据
// 确保后续 Merge 写入的是全新干净数据，不会被历史损坏条目污染
if reindex {
    addressSet := make(map[string]struct{})
    for _, tx := range block.Transactions {
        for _, out := range tx.Outputs {
            if out.Address != "" && out.Address != "errAddress" {
                addressSet[out.Address] = struct{}{}
            }
        }
    }
    if len(addressSet) > 0 {
        for addr := range addressSet {
            i.addressStore.DeleteKey(addr)
            i.spendStore.DeleteKey(addr)
        }
    }
}
```

### 修改 3：`blockchain/client.go` — `ProcessBlock` 加 `reindex` 参数

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

### 修改 4：`api/server.go` — reindex handler 传 `true`

```go
// 旧
s.bcClient.ProcessBlock(s.indexer, height, false, height)

// 新
s.bcClient.ProcessBlock(s.indexer, height, false, height, true)
```

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
# 从 HIGUN 首次启动高度开始，全量重索引
curl -s 'http://127.0.0.1:8085/blocks/reindex?start=162274&end=<当前高度>'
```

这次 reindex 会先删除每个区块涉及地址的旧数据，再用新 Merger 写入干净数据。

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
| `storage/pebble.go` | 新增 `DeleteKey` 方法（1 行） |
| `indexer/utxo.go` | `IndexBlock` 加 `reindex` 参数 + reindex 清理逻辑（~20 行） |
| `blockchain/client.go` | `ProcessBlock` 加 `reindex` 参数 + 透传 + 普通调用加 `false`（4 处） |
| `api/server.go` | reindex handler 传 `true`（1 处） |
