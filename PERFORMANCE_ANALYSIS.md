# BTC索引性能瓶颈分析报告

## 问题概述
连接局域网的BTC节点，索引速度仅 **1个区块/秒**，性能明显偏低。

## 主要性能瓶颈

### 1. 🔴 **RPC网络请求 - 最严重瓶颈**

#### 问题位置
[blockchain/adapter_btc.go](blockchain/adapter_btc.go#L109-L125)

```go
func (a *BTCAdapter) GetBlock(height int64) (*indexer.Block, error) {
    // 1. 获取区块哈希 - RPC调用1
    hashStr, err := a.GetBlockHash(height)
    
    // 2. 获取原始区块数据 - RPC调用2
    resp, err := a.rpcClient.RawRequest("getblock", ...)
}
```

**影响分析：**
- 每个区块需要 **2次RPC调用**（先获取hash，再获取区块）
- 局域网RTT假设为1-5ms，每个区块至少增加 **2-10ms延迟**
- 如果区块处理时间本身是50ms，网络延迟占比20%

**优化方案：**
```go
// 改进：使用批量RPC请求预取多个区块
func (a *BTCAdapter) GetBlockBatch(heights []int64) ([]*indexer.Block, error) {
    // 批量获取10-50个区块的hash
    // 批量获取区块数据
    // 减少网络往返次数
}
```

---

### 2. 🟡 **磁盘同步频率过高**

#### 问题位置
[indexer/utxo.go](indexer/utxo.go#L161-L168)

```go
func (i *UTXOIndexer) IndexBlock(...) {
    if !block.IsPartialBlock && updateHeight {
        // 每个区块都进行3次Sync操作
        go func() {
            i.utxoStore.Sync()     // 磁盘同步1
            i.addressStore.Sync()  // 磁盘同步2
            i.spendStore.Sync()    // 磁盘同步3
        }()
        i.metaStore.Sync()  // 磁盘同步4 (立即执行)
    }
}
```

**影响分析：**
- 每个区块触发 **4次磁盘fsync操作**
- 即使使用异步，metaStore仍是同步fsync
- 假设每次fsync耗时5ms，每个区块增加 **20ms延迟**

[storage/pebble.go](storage/pebble.go#L930-L936) 批量提交已优化：
```go
// 批量提交时使用 NoSync，最后才 Sync
if err := batch.Commit(pebble.NoSync); err != nil {
    // ...
}
```

**但最终每个区块的Sync过于频繁！**

**优化方案：**
```go
// 改进：每N个区块才fsync一次（如每10个区块）
if block.Height % 10 == 0 {
    i.utxoStore.Sync()
    i.addressStore.Sync()
    i.spendStore.Sync()
}
// metaStore每个区块仍需sync，确保高度持久化
```

---

### 3. 🟡 **数据库写入批次大小**

#### 当前配置
[config.yaml](config.yaml#L11-L12)
```yaml
workers: 4
batch_size: 20000
```

[storage/pebble.go](storage/pebble.go#L926-L928)
```go
// 批量提交阈值
if count >= 5000 || batch.Len() >= maxBatchSize {
    batch.Commit(pebble.NoSync)
}
```

**影响分析：**
- batch_size=20000 用于交易数据分组
- 实际写入批次=5000条
- 如果区块有2000笔交易，会进行多次小批量提交

**优化方案：**
```yaml
# 增大批次减少提交次数
batch_size: 50000
```

```go
// 增大写入批次
if count >= 10000 || batch.Len() >= maxBatchSize {
    batch.Commit(pebble.NoSync)
}
```

---

### 4. 🟢 **并发配置偏低**

#### 当前配置
[config.yaml](config.yaml#L11)
```yaml
workers: 4  # 并发workers
```

**影响分析：**
- workers=4 控制数据库写入并发度
- 对于4核CPU，可以适度提高
- 但局域网环境，网络IO是瓶颈，提高workers效果有限

**优化方案：**
```yaml
workers: 8  # 对于IO密集型任务可以超配
```

---

### 5. 🟢 **区块文件归档开销**

#### 问题位置
[indexer/utxo.go](indexer/utxo.go#L140-L142)
```go
// 存储utxo归档文件
SaveBlockFile("utxo", allBlock, true)
```

[config.yaml](config.yaml#L1-L50) 中未看到 `block_files_enabled` 配置

**影响分析：**
- 如果启用了区块文件归档，每个区块都会额外写文件
- 增加磁盘IO开销

**优化方案：**
```yaml
# 在索引阶段禁用归档，索引完成后再归档
block_files_enabled: false
```

---

## 性能优化优先级排序

| 优先级 | 优化项 | 预期提升 | 实施难度 |
|--------|--------|----------|----------|
| ⭐⭐⭐ | RPC批量请求 | 30-50% | 中 |
| ⭐⭐⭐ | 减少fsync频率 | 20-30% | 低 |
| ⭐⭐ | 增大批次大小 | 10-15% | 低 |
| ⭐ | 提高workers | 5-10% | 低 |
| ⭐ | 禁用归档 | 5-10% | 低 |

---

## 快速优化建议（立即可实施）

### 1. 修改配置文件 [config.yaml](config.yaml)
```yaml
workers: 8              # 从4提升到8
batch_size: 50000       # 从20000提升到50000
block_files_enabled: false  # 禁用归档加速索引
```

### 2. 修改存储层 [storage/pebble.go](storage/pebble.go#L926)
```go
// 从5000提升到10000
if count >= 10000 || batch.Len() >= maxBatchSize {
```

### 3. 修改索引器 [indexer/utxo.go](indexer/utxo.go#L161)
```go
// 每10个区块才同步一次
if !block.IsPartialBlock && updateHeight {
    if block.Height % 10 == 0 {
        go func() {
            i.utxoStore.Sync()
            i.addressStore.Sync()
            i.spendStore.Sync()
        }()
    }
    // metaStore仍每次同步确保高度正确
    i.metaStore.Sync()
}
```

---

## 中长期优化方案

### 1. RPC批量预取（最大收益）
```go
// 新增批量获取接口
func (c *Client) ProcessBlockBatch(idx *indexer.UTXOIndexer, startHeight, endHeight int) error {
    // 预取10-50个区块
    batch := min(50, endHeight - startHeight + 1)
    
    // 异步预取下一批
    go func() {
        for h := startHeight; h < startHeight + batch; h++ {
            blockCache[h] = c.adapter.GetBlock(h)
        }
    }()
    
    // 处理当前批
    for h := startHeight; h < min(startHeight + batch, endHeight); h++ {
        block := <-blockCache[h]
        idx.IndexBlock(block, ...)
    }
}
```

### 2. 使用getblockheader + getblocktxn优化
避免重复获取hash，改用：
```bash
# 直接通过高度获取
getblockhash 12345
getblock <hash> 0  # 获取原始数据
```

改为：
```bash
# 使用getblockheader更轻量
getblockheader <hash> true
```

---

## 性能监控建议

在 [blockchain/client.go](blockchain/client.go#L291) 添加详细计时：
```go
func (c *Client) ProcessBlock(...) {
    t1 := time.Now()
    allBlock, err := c.adapter.GetBlock(int64(height))
    log.Printf("GetBlock: %.2fs", time.Since(t1).Seconds())
    
    t2 := time.Now()
    idx.IndexBlock(...)
    log.Printf("IndexBlock: %.2fs", time.Since(t2).Seconds())
}
```

---

## 预期优化效果

**当前性能：** 1区块/秒 = 1000ms/区块

**优化后预期：**
- 快速优化（配置调整）: **2-3区块/秒** (减少30-50%)
- 加上RPC批量预取: **5-8区块/秒** (减少80-85%)
- 极限优化: **10+区块/秒** (需要更深度改造)

---

## 立即行动清单

- [ ] 修改 config.yaml 参数
- [ ] 减少fsync频率（每10个区块）
- [ ] 增大批量提交阈值
- [ ] 添加性能监控日志
- [ ] 测试并记录优化效果
- [ ] （中期）实现RPC批量预取

---

**建议先进行快速优化，观察效果后再决定是否进行深度重构。**
