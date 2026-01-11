# 性能监控日志说明

## 已添加的性能监控点

### 1. RPC层监控 (blockchain/adapter_btc.go)
```
[Perf-RPC] Height XXX: GetHash=X.XXXs, GetRawBlock=X.XXXs, Deserialize=X.XXXs, Convert=X.XXXs
```

**监控内容：**
- `GetHash`: 获取区块哈希的RPC调用耗时
- `GetRawBlock`: 获取原始区块数据的RPC调用耗时
- `Deserialize`: 反序列化区块数据的耗时
- `Convert`: 转换为索引器格式的耗时

### 2. 区块处理层监控 (blockchain/client.go)
```
[Perf] Height XXX: GetBlock took X.XXXs
[Perf] Height XXX: IndexBlock batch took X.XXXs
[Perf] Height XXX TOTAL: X.XXXs (GetBlock: X.XXXs, IndexBlock: processing included), TxCount: XXX
```

**监控内容：**
- `GetBlock took`: 获取区块的总耗时（包含RPC + 解析 + 转换）
- `IndexBlock batch took`: 索引区块批次的耗时
- `TOTAL`: 处理一个区块的总耗时

### 3. 索引层监控 (indexer/utxo.go)
```
[Perf-Index] Height XXX: Income=X.XXXs, SaveUtxo=X.XXXs, Spend=X.XXXs, SaveSpend=X.XXXs, Sync=X.XXXs (InCnt=XXX, OutCnt=XXX, Addr=XXX)
```

**监控内容：**
- `Income`: 索引输出（UTXO创建）的耗时
- `SaveUtxo`: 保存UTXO归档文件的耗时
- `Spend`: 处理输入（UTXO花费）的耗时
- `SaveSpend`: 保存Spend归档文件的耗时
- `Sync`: 数据库同步（fsync）的耗时
- `InCnt/OutCnt/Addr`: 输入数、输出数、地址数统计

## 如何分析性能瓶颈

### 运行程序并查看日志
```bash
./higun | grep -E "\[Perf"
```

### 典型输出示例
```
[Perf-RPC] Height 12345: GetHash=0.005s, GetRawBlock=0.015s, Deserialize=0.003s, Convert=0.002s
[Perf] Height 12345: GetBlock took 0.025s
[Perf] Height 12345: IndexBlock batch took 0.150s
[Perf-Index] Height 12345: Income=0.080s, SaveUtxo=0.010s, Spend=0.045s, SaveSpend=0.005s, Sync=0.010s (InCnt=2500, OutCnt=2600, Addr=1200)
[Perf] Height 12345 TOTAL: 0.175s (GetBlock: 0.025s, IndexBlock: processing included), TxCount: 2500
```

### 分析方法

#### 1. 如果 GetRawBlock 耗时最长（>50%）
**瓶颈：** 网络延迟或节点性能
**优化方向：**
- 实现批量RPC预取
- 检查网络质量
- 升级到更快的节点

#### 2. 如果 Income 或 Spend 耗时最长
**瓶颈：** 数据库写入
**优化方向：**
- 增大 batch_size
- 增加 workers 数量
- 减少 fsync 频率

#### 3. 如果 Sync 耗时很长
**瓶颈：** 磁盘同步
**优化方向：**
- 减少 Sync 频率（每N个区块）
- 使用更快的磁盘（SSD/NVMe）
- 调整 Pebble 参数

#### 4. 如果 SaveUtxo/SaveSpend 耗时较长
**瓶颈：** 归档文件写入
**优化方向：**
- 临时禁用归档：`block_files_enabled: false`
- 异步写入归档
- 使用单独的磁盘

## 性能基准参考

### 正常性能指标（局域网BTC节点）
- GetHash: 1-5ms
- GetRawBlock: 5-20ms（取决于区块大小）
- Deserialize: 1-10ms（取决于区块大小）
- Convert: 1-5ms
- Income: 50-200ms（取决于交易数）
- Spend: 30-150ms（取决于输入数）
- Sync: 5-20ms

### 异常指标警告
- GetRawBlock > 100ms：网络问题或节点负载高
- Income/Spend > 500ms：数据库写入慢，需要优化
- Sync > 50ms：磁盘IO瓶颈
- TOTAL > 1000ms：整体性能不足，需要全面优化

## 快速诊断命令

### 统计平均耗时
```bash
./higun | grep "\[Perf\] Height.*TOTAL" | awk '{print $7}' | sed 's/s//' | awk '{sum+=$1; count++} END {print "Average:", sum/count, "s"}'
```

### 找出最慢的区块
```bash
./higun | grep "\[Perf\] Height.*TOTAL" | sort -t':' -k2 -rn | head -10
```

### 分析RPC耗时占比
```bash
./higun | grep "\[Perf-RPC\]" | awk -F'GetRawBlock=' '{print $2}' | cut -d',' -f1 | sed 's/s//' | awk '{sum+=$1; count++} END {print "Average GetRawBlock:", sum/count, "s"}'
```

---

**提示：** 运行索引几分钟后，查看日志输出，就能清楚看到性能瓶颈在哪里！
