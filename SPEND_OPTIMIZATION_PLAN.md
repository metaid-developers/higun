# Spend性能优化方案

## 🔥 问题确认

根据性能日志分析，**Spend处理占用了90-95%的索引时间**：

```
Height 670165: TOTAL=4.952s
├─ GetBlock: 0.097s (2%)
├─ Income:   0.139s (3%)
└─ Spend:    4.677s (94%)  ← 瓶颈！
```

**原因：** 每个输入都需要查询数据库，导致大量随机读操作。

---

## 💡 优化方案（按优先级排序）

### 方案1：增大batch_size和workers ⭐⭐⭐（立即可用）

**当前配置：**
```yaml
workers: 4
batch_size: 20000
```

**建议配置：**
```yaml
workers: 16        # 增加到16个并发worker
batch_size: 50000  # 增大批次减少分批次数
```

**预期效果：** Spend时间减少30-40%

**实施方法：**
```bash
# 修改 config.yaml
vim config.yaml
```

---

### 方案2：优化QueryUTXOAddresses2的并发度 ⭐⭐⭐（中等难度）

**问题代码：** [storage/pebble.go#L1327](storage/pebble.go#L1327)
```go
concurrency := runtime.NumCPU() * 2  // 当前并发度太低
```

**优化方案：** 增加并发度，减少锁竞争

#### 实施代码：

修改 `storage/pebble.go` 中的 `QueryUTXOAddresses2` 方法：

```go
func (s *PebbleStore) QueryUTXOAddresses2(outpoints *[]string) (map[string][]string, error) {
	if len(*outpoints) == 0 {
		return make(map[string][]string), nil
	}

	// 增大并发度
	concurrency := runtime.NumCPU() * 4  // 从2改为4
	jobsCh := make(chan string, len(*outpoints))

	// 使用分片map减少锁竞争
	numShards := 32
	type shardMap struct {
		mu   sync.Mutex
		data map[string][]string
	}
	shards := make([]shardMap, numShards)
	for i := range shards {
		shards[i].data = make(map[string][]string)
	}

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range jobsCh {
				// Parse key: txid:index
				colonIdx := strings.LastIndexByte(key, ':')
				if colonIdx == -1 {
					continue
				}
				txid := key[:colonIdx]
				indexStr := key[colonIdx+1:]

				// Get from DB
				db := s.getShard(txid)
				value, closer, err := db.Get([]byte(txid))
				if err != nil {
					continue
				}

				// Parse address directly from bytes to avoid allocation
				address := extractAddressFromValue(value, indexStr)
				closer.Close()

				if address != "" {
					// 使用分片减少锁竞争
					shardIdx := xxhash.Sum64String(address) % uint64(numShards)
					shards[shardIdx].mu.Lock()
					shards[shardIdx].data[address] = append(shards[shardIdx].data[address], key)
					shards[shardIdx].mu.Unlock()
				}
			}
		}()
	}

	// Send jobs
	for _, op := range *outpoints {
		jobsCh <- op
	}
	close(jobsCh)

	wg.Wait()

	// 合并结果
	results := make(map[string][]string)
	for i := range shards {
		for k, v := range shards[i].data {
			results[k] = v
		}
	}

	return results, nil
}
```

**预期效果：** Spend时间减少20-30%

---

### 方案3：使用内存缓存UTXO ⭐⭐⭐⭐（最大收益，高难度）

**核心思路：** 将热点UTXO缓存在内存中，避免重复查询数据库

#### 实施方案：

1. **在IndexBlock开始时预加载所有需要的UTXO**
2. **使用LRU缓存存储最近的UTXO**
3. **批量查询而非逐个查询**

**伪代码：**
```go
type UTXOCache struct {
    cache *lru.Cache
}

func (i *UTXOIndexer) IndexBlock(block *Block, ...) {
    // 1. 收集所有需要的UTXO
    requiredUTXOs := collectAllInputs(block)
    
    // 2. 批量预加载（一次查询）
    utxoMap := i.utxoStore.BatchGet(requiredUTXOs)
    
    // 3. 处理Income（写入新UTXO）
    i.indexIncome(block, allBlock, blockTimeStr)
    
    // 4. 处理Spend（使用预加载的数据）
    i.processSpendWithCache(block, utxoMap, blockTimeStr)
}
```

**预期效果：** Spend时间减少70-80%，从4s降至0.8-1.2s

**实施难度：** 需要重构processSpend逻辑

---

### 方案4：数据库层面优化 ⭐⭐（辅助优化）

#### 4.1 调整Pebble参数

修改 `storage/pebble.go` 中的数据库配置：

```go
dbOptions := &pebble.Options{
    // 增大Block Cache
    Cache: pebble.NewCache(4 << 30), // 从2GB增加到4GB
    
    // 增大MemTable
    MemTableSize: 128 << 20, // 从64MB增加到128MB
    
    // 增大读缓冲
    MaxOpenFiles: 10000,
    
    // L0优化
    L0CompactionThreshold: 8,
    L0StopWritesThreshold: 24,
}
```

#### 4.2 使用BloomFilter加速查询

```go
dbOptions.Levels[0].FilterPolicy = bloom.FilterPolicy(10)
```

**预期效果：** Spend时间减少10-15%

---

## 📊 综合优化方案（推荐）

### 第一阶段：快速优化（1小时内完成）

1. ✅ 修改config.yaml
   ```yaml
   workers: 16
   batch_size: 50000
   ```

2. ✅ 修改QueryUTXOAddresses2并发度
   - 将 `concurrency` 从 `NumCPU*2` 改为 `NumCPU*4`
   - 使用分片map减少锁竞争

3. ✅ 增大Pebble Cache
   - Cache从2GB增加到4GB

**预期效果：** 从4-5s/区块 → **2-2.5s/区块**（提升50%+）

### 第二阶段：深度优化（需要1-2天）

4. ✅ 实现UTXO批量预加载
5. ✅ 添加LRU缓存
6. ✅ 重构processSpend逻辑

**预期效果：** 从2-2.5s/区块 → **0.5-0.8s/区块**（提升80%+）

---

## 🚀 立即行动清单

### Step 1: 修改配置（2分钟）
```bash
cd /srv/dev_project/metaid/higun
vim config.yaml

# 修改：
# workers: 4 → workers: 16
# batch_size: 20000 → batch_size: 50000
```

### Step 2: 优化代码（30分钟）
- 修改 `storage/pebble.go` 中的并发度
- 增大Cache大小
- 使用分片map

### Step 3: 重新编译测试
```bash
make linux
scp ./utxo_indexer metaid-btc-utxo:/date/higun_btc
```

### Step 4: 观察性能提升
```bash
# 查看新的性能日志
docker logs -f <container> | grep "\[Perf"
```

---

## 🎯 预期最终效果

| 阶段 | 当前速度 | 优化后速度 | 提升幅度 |
|------|----------|------------|----------|
| 当前 | 1区块/4-5s | - | - |
| 第一阶段 | 1区块/4-5s | **1区块/2-2.5s** | **50%+** |
| 第二阶段 | 1区块/2-2.5s | **1区块/0.5-0.8s** | **80%+** |

**最终目标：** 从 **1区块/秒** 提升到 **4-8区块/秒** 🚀

---

**建议：** 先实施第一阶段的快速优化，验证效果后再考虑第二阶段的深度优化。
