# 多链支持改造 - 实施总结

## 🎯 核心思想

**配置文件中的 `chain` 字段仅用于标识链类型,选择正确的解析器。**

- ✅ 每个实例独立运行一条链
- ✅ 数据目录在配置文件中独立指定,无需按链名称区分
- ✅ 添加新链只需实现该链的交易/地址解析代码
- ✅ 所有其他功能(索引、查询、API)完全复用

## 📝 需要修改的内容

### 1. config/config.go (添加约 40 行)

```go
// 添加常量
const (
    ChainBTC = "btc"
    ChainMVC = "mvc"
)

// Config 添加字段
type Config struct {
    Chain string `yaml:"chain"` // 用于选择解析器
    // ... 其他现有字段
}

// 添加方法
func (c *Config) ValidateChain() error { ... }
func (c *Config) GetChainName() string { ... }
func (c *Config) GetChainDataDir() string {
    return c.DataDir  // 直接返回,不添加子目录
}

// LoadConfig 中添加验证
if err := cfg.ValidateChain(); err != nil {
    return nil, err
}
```

### 2. config.yaml (添加 1 行)

```yaml
chain: "btc"  # 或 "mvc", 用于选择解析器
network: "regtest"
data_dir: "/home/momo/data/higun/instance1"  # 独立目录
# ... 其他配置
```

### 3. blockchain/client.go (已支持,无需修改)

现有代码已经通过 `c.cfg.RPC.Chain` 区分不同链:
- ✅ `GetBlockMsg` 方法根据 chainName 处理
- ✅ `convertMvcTxToIndexerTx` 方法
- ✅ `convertBtcTxToIndexerTx` 方法

## 🚀 使用方式

### 单个实例
```bash
# 启动 BTC 实例
./utxo_indexer --config config_btc.yaml

# 启动 MVC 实例  
./utxo_indexer --config config_mvc.yaml
```

### 配置示例

**BTC 配置 (config_btc.yaml)**
```yaml
chain: "btc"
network: "mainnet"
data_dir: "/data/higun/btc_mainnet"
api_port: "3001"
rpc:
  chain: "btc"
  host: "127.0.0.1"
  port: "8332"
```

**MVC 配置 (config_mvc.yaml)**
```yaml
chain: "mvc"
network: "mainnet"
data_dir: "/data/higun/mvc_mainnet"
api_port: "3002"
rpc:
  chain: "mvc"
  host: "127.0.0.1"
  port: "9882"
```

## ✨ 扩展新链

### 示例: 添加 DOGE 支持

#### 步骤 1: 添加常量 (config/config.go)
```go
const ChainDOGE = "doge"
supportedChains := map[string]bool{
    ChainBTC:  true,
    ChainMVC:  true,
    ChainDOGE: true,
}
```

#### 步骤 2: 添加解析器 (blockchain/client.go)
```go
// 在 GetBlockMsg 中添加
if chainName == "doge" {
    msgBlock := &wire.MsgBlock{}
    // DOGE 特定处理
}

// 添加转换方法
func (c *Client) convertDogeTxToIndexerTx(tx *wire.MsgTx) *indexer.Transaction {
    // DOGE 交易转换逻辑
}
```

#### 步骤 3: 添加地址解析 (blockchain/util.go)
```go
func GetAddressFromScript(..., chainName string) string {
    if chainName == "doge" {
        // DOGE 地址格式处理
    }
}
```

#### 步骤 4: 创建配置文件
```yaml
chain: "doge"
network: "mainnet"
data_dir: "/data/higun/doge_mainnet"
api_port: "3003"
rpc:
  chain: "doge"
  host: "127.0.0.1"
  port: "22555"
```

#### 步骤 5: 启动
```bash
./utxo_indexer --config config_doge.yaml
```

**完成!** 所有索引、查询、API 功能自动可用。

## 💡 关键点

1. **`chain` 字段的作用**: 仅用于选择解析器,不影响数据目录结构
2. **数据目录**: 在配置文件中独立指定,按实例划分,不按链名称划分
3. **扩展性**: 添加新链只需实现解析逻辑(通常 < 100 行代码)
4. **复用性**: 所有核心功能(索引、存储、查询、API)完全复用
5. **独立性**: 每个实例独立运行,可部署在不同服务器

## ⏱️ 工作量

- **核心修改**: 30 分钟(config.go)
- **测试验证**: 30 分钟
- **添加新链**: 1-2 小时(含测试)
- **总计**: 约 1 小时即可完成基础改造

## 📚 详细文档

- **快速开始**: `docs/QUICK_START.md`
- **完整方案**: `README_MULTI_CHAIN.md`
- **设计文档**: `MULTI_CHAIN_REFACTOR_PLAN.md`
- **代码示例**: `docs/config_refactored.go.example`

---

**方案优势**: 最小改动 + 最大复用 + 最强扩展性
