# 多链支持改造方案

## 概述
支持多个 UTXO 链(BTC, MVC, DOGE 等),通过配置文件 `chain` 字段标识链类型,用于选择正确的交易/地址解析器。每个实例独立运行一条链,使用独立的数据目录。

## 改造原则
1. **最小化代码修改** - 复用现有代码结构,只添加解析器选择逻辑
2. **配置标识链类型** - `chain` 字段仅用于选择解析器,不影响数据目录
3. **实例独立运行** - 每个实例独立配置,无需在目录中区分链名称
4. **扩展性优先** - 添加新链只需实现解析逻辑,其他功能自动复用

## 改造步骤

### 第一步: 配置文件增强

#### 修改 `config.yaml`
```yaml
# 新增: 明确指定链类型
chain: "btc"  # 可选: btc, mvc, ltc 等

# 现有配置保持不变
network: "regtest"
block_info_indexer: true
# ... 其他配置
```

#### 修改 `config/config.go`
需要添加:
1. `Chain` 字段到 `Config` 结构体
2. 链类型常量定义
3. 链参数获取方法 `GetChainParams()`
4. 配置验证方法

### 第二步: 链抽象层

#### 在 `blockchain/client.go` 中
当前代码已经有基础:
- `c.cfg.RPC.Chain` 用于区分链类型
- `convertMvcTxToIndexerTx` 和 `convertBtcTxToIndexerTx` 方法
- `GetBlockMsg` 方法处理不同链的区块

需要优化:
1. 统一链处理接口
2. 清晰的链类型判断逻辑
3. 链特定的序列化/反序列化

### 第三步: 实例独立配置

#### 数据目录独立配置
```go
// 在 config.go 中
func (c *Config) GetChainDataDir() string {
    // 直接返回配置的数据目录,不添加链名称
    return c.DataDir
}
```

#### 每个实例使用独立目录
```
# BTC 实例配置
data_dir: /data/higun/btc_instance

# MVC 实例配置(可在不同服务器)
data_dir: /data/higun/mvc_instance

# DOGE 实例配置
data_dir: /data/higun/doge_instance
```

### 第四步: 链特定逻辑处理

#### 交易哈希计算
- BTC: 标准双 SHA256
- MVC: 可能有不同的哈希算法(已有 GetNewHash2)

#### 地址格式
- BTC: 使用 btcutil
- MVC: 使用 bsvutil (已实现)

#### 脚本解析
- 使用 `GetAddressFromScript` 统一接口,根据 chainName 处理

## 代码修改清单

### 必须修改的文件

1. **config/config.go**
   - 添加 `Chain` 字段
   - 添加链类型常量
   - 添加 `ValidateChain()` 方法
   - 修改 `GetDataDir()` 返回链特定路径

2. **blockchain/client.go**
   - 已有链区分逻辑,无需大改
   - 确保所有链相关判断使用 `c.cfg.RPC.Chain`

3. **blockchain/getblock.go** (如果存在)
   - 查看是否需要链特定的区块获取逻辑

### 可选优化的文件

1. **mempool/transaction.go**
   - 统一使用 `config.GlobalConfig.RPC.Chain`
   - 考虑传入 chain 参数而不是全局变量

2. **indexer/*.go**
   - 确保索引器不依赖特定链逻辑

## 配置示例

### BTC 配置
```yaml
chain: "btc"
network: "mainnet"
data_dir: "/data/higun/btc"
rpc:
  chain: "btc"  # 保持一致
  host: "127.0.0.1"
  port: "8332"
  user: "btc"
  password: "btc"
```

### MVC 配置
```yaml
chain: "mvc"
network: "mainnet"
data_dir: "/data/higun/mvc"
rpc:
  chain: "mvc"  # 保持一致
  host: "127.0.0.1"
  port: "9882"
  user: "mvc"
  password: "mvc"
```

## 测试计划

1. **BTC 链测试**
   - 区块同步
   - 交易索引
   - UTXO 查询

2. **MVC 链测试**
   - 区块同步
   - 交易索引
   - UTXO 查询

3. **切换测试**
   - 停止 BTC 实例
   - 修改配置为 MVC
   - 启动并验证独立运行

## 风险评估

### 低风险
- 配置文件修改
- 数据目录隔离
- 链类型判断逻辑

### 中风险
- 交易哈希算法差异
- 地址格式转换
- 区块解析差异

### 高风险
- 共享全局变量 (`GlobalConfig`, `GlobalNetwork`)
- RPC 客户端兼容性
- 内存池处理逻辑

## 实施建议

### 阶段一: 配置和数据隔离 (1天)
- 修改配置结构
- 实现数据目录隔离
- 添加配置验证

### 阶段二: 代码适配 (2天)
- 确认链判断逻辑
- 测试 BTC 和 MVC 分别运行
- 修复兼容性问题

### 阶段三: 测试和优化 (2天)
- 完整功能测试
- 性能测试
- 文档更新

## 后续扩展

### 支持新链的步骤(以 DOGE 为例)
1. **添加链标识** - 在 config.go 常量中添加 `ChainDOGE = "doge"`
2. **实现交易解析** - 在 blockchain/client.go 添加 `convertDogeTxToIndexerTx()` 方法
3. **实现地址解析** - 在 blockchain/util.go 的 `GetAddressFromScript()` 中添加 DOGE 分支
4. **创建配置文件** - 复制现有配置,修改 `chain: "doge"` 和 RPC 参数
5. **启动测试** - `./utxo_indexer --config config_doge.yaml`

**就这么简单!** 所有索引、查询、API 逻辑自动复用,无需任何修改。

### 可能需要扩展的点
- 不同的共识算法
- 不同的脚本类型
- 不同的签名算法
- 特殊的交易类型

## 总结

这个方案的核心优势:
1. **改动最小** - 主要是配置和数据隔离
2. **清晰明确** - 通过配置明确链类型
3. **易于测试** - 每个链独立运行
4. **便于扩展** - 添加新链只需少量代码

主要修改集中在:
- 配置文件和配置加载
- 数据目录隔离
- 确保链判断逻辑一致性

现有代码已经有较好的链区分基础,主要是规范化和完善。
