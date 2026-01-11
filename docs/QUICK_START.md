# 多链支持快速实施指南

## 🎯 目标
让 UTXO Indexer 支持多条链(BTC, MVC, DOGE等),通过配置文件中的 `chain` 字段标识链类型,用于选择正确的交易/地址解析器。每个实例独立运行一条链,数据目录无需区分链名称。

## ⚡ 最小改动方案 (推荐)

### 改动文件列表
1. ✏️ `config/config.go` - 添加 ~50 行代码
2. ✏️ `config.yaml` - 添加 1 行配置
3. 🔍 `blockchain/client.go` - 确认现有代码(无需修改)

### 核心改动点

#### 1️⃣ 修改 config/config.go

**A. 在开头添加常量** (第 13 行后):
```go
const (
	ChainBTC = "btc"
	ChainMVC = "mvc"
)
```

**B. 在 Config 结构体添加字段** (第 24 行,第一个字段):
```go
type Config struct {
	Chain                   string    `yaml:"chain"` // 新增
	Network                 string    `yaml:"network"`
	// ... 其余字段保持不变
```

**C. 添加三个新方法** (在 GetChainParams 方法后):
```go
func (c *Config) ValidateChain() error {
	if c.Chain == "" {
		return fmt.Errorf("chain field is required")
	}
	supportedChains := map[string]bool{ChainBTC: true, ChainMVC: true}
	if !supportedChains[c.Chain] {
		return fmt.Errorf("unsupported chain: %s, supported: btc, mvc", c.Chain)
	}
	if c.Chain != c.RPC.Chain {
		return fmt.Errorf("chain mismatch: config.chain=%s but rpc.chain=%s", c.Chain, c.RPC.Chain)
	}
	return nil
}

func (c *Config) GetChainName() string {
	if c.Chain != "" {
		return c.Chain
	}
	if c.RPC.Chain != "" {
		return c.RPC.Chain
	}
	return ChainBTC
}

func (c *Config) GetChainDataDir() string {
	// 直接返回配置的数据目录,不添加链名称子目录
	// 每个实例独立运行,通过配置文件区分链类型即可
	return c.DataDir
}
```

**D. 修改 LoadConfig 函数**:

在 cfg := &Config{ 中添加 (第 65 行左右):
```go
cfg := &Config{
	Chain:   ChainBTC, // 新增
	Network: "testnet",
	// ... 其他保持不变
	RPC: RPCConfig{
		Chain: ChainBTC, // 新增
		Host:  "localhost",
		Port:  "8332",
	},
```

在读取环境变量部分添加 (第 100 行左右):
```go
if chain := os.Getenv("CHAIN"); chain != "" {
	cfg.Chain = chain
}
```

在 Ensure data dir exists 之前添加 (第 135 行左右):
```go
// 验证链配置
if err := cfg.ValidateChain(); err != nil {
	return nil, fmt.Errorf("chain validation failed: %w", err)
}

// 输出链信息
fmt.Printf("Chain: %s, Network: %s, Data Dir: %s\n", 
	cfg.GetChainName(), cfg.Network, cfg.DataDir)

// Ensure data dir exists
if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
	return nil, fmt.Errorf("failed to create data directory: %w", err)
}
```

#### 2️⃣ 修改 config.yaml

在文件开头第 2 行添加:
```yaml
# UTXO Indexer Configuration
chain: "btc"  # 链类型: btc 或 mvc
network: "regtest"
```

#### 3️⃣ 检查 blockchain/client.go

确认以下代码存在(无需修改):
- ✅ 第 485 行: `chainName := c.cfg.RPC.Chain`
- ✅ 第 490 行: `GetBlockMsg` 方法根据 chainName 处理
- ✅ 第 1025 行: `convertMvcTxToIndexerTx` 方法
- ✅ 第 1048 行: `convertBtcTxToIndexerTx` 方法

---

## 🚀 实施步骤

### 步骤 1: 备份
```bash
cd /home/momo/projects/metaid/higun
cp config/config.go config/config.go.bak
cp config.yaml config.yaml.bak
```

### 步骤 2: 应用代码修改
参考上面的详细修改点,修改 `config/config.go`

### 步骤 3: 修改配置文件
在 `config.yaml` 开头添加 `chain: "btc"`

### 步骤 4: 编译测试
```bash
go build -o utxo_indexer_test
```

### 步骤 5: 验证 BTC 配置
```bash
# 复制配置
cp config.yaml config_btc.yaml

# 确保配置正确
cat config_btc.yaml | grep -A 5 "^chain:"

# 测试运行
./utxo_indexer_test --config config_btc.yaml
```

### 步骤 6: 验证 MVC 配置
```bash
# 复制配置
cp config.yaml config_mvc.yaml

# 修改链类型
sed -i 's/chain: "btc"/chain: "mvc"/' config_mvc.yaml
sed -i 's/  chain: "btc"/  chain: "mvc"/' config_mvc.yaml

# 修改数据目录和端口
sed -i 's/data_dir: "\/home\/momo\/data\/higun\/test"/data_dir: "\/home\/momo\/data\/higun\/mvc"/' config_mvc.yaml
sed -i 's/api_port: "3001"/api_port: "3002"/' config_mvc.yaml

# 测试运行
./utxo_indexer_test --config config_mvc.yaml
```

---

## ✅ 验证清单

启动程序后检查日志:

```
Chain: btc, Network: regtest, Data Dir: /home/momo/data/higun/btc_instance
```

检查文件系统:
```bash
# BTC 实例数据
ls -la /home/momo/data/higun/btc_instance/utxo/
ls -la /home/momo/data/higun/btc_instance/income/
ls -la /home/momo/data/higun/btc_instance/spend/

# MVC 实例数据(在另一个服务器或目录)
ls -la /home/momo/data/higun/mvc_instance/utxo/
ls -la /home/momo/data/higun/mvc_instance/income/
ls -la /home/momo/data/higun/mvc_instance/spend/
```

---

## 📋 配置模板

### BTC 主网配置
```yaml
chain: "btc"  # 用于选择BTC交易/地址解析器
network: "mainnet"
data_dir: "/data/higun/instance1"  # 实例独立数据目录
api_port: "3001"
rpc:
  chain: "btc"
  host: "127.0.0.1"
  port: "8332"
  user: "bitcoin"
  password: "password"
```

### MVC 主网配置
```yaml
chain: "mvc"  # 用于选择MVC交易/地址解析器
network: "mainnet"
data_dir: "/data/higun/instance2"  # 实例独立数据目录
api_port: "3002"
rpc:
  chain: "mvc"
  host: "127.0.0.1"
  port: "9882"
  user: "mvc"
  password: "password"
```

### DOGE 主网配置(扩展示例)
```yaml
chain: "doge"  # 用于选择DOGE交易/地址解析器
network: "mainnet"
data_dir: "/data/higun/instance3"  # 实例独立数据目录
api_port: "3003"
rpc:
  chain: "doge"
  host: "127.0.0.1"
  port: "22555"
  user: "doge"
  password: "password"
```

---

## ⚠️ 注意事项

### 1. 配置一致性
❌ 错误示例:
```yaml
chain: "btc"
rpc:
  chain: "mvc"  # 不一致!
```

✅ 正确示例:
```yaml
chain: "btc"
rpc:
  chain: "btc"  # 一致
```

### 2. 数据目录独立
每个实例使用独立的数据目录(可以在不同服务器):
- 实例1(BTC): `/data/higun/instance1/`
- 实例2(MVC): `/data/higun/instance2/`
- 实例3(DOGE): `/data/higun/instance3/`

### 3. 端口不冲突
同时运行多个实例时,确保端口不同:
- BTC: `api_port: "3001"`
- MVC: `api_port: "3002"`

### 4. 现有数据无需迁移
现有数据直接使用,只需在配置文件中添加 `chain` 字段标识链类型。

例如,现有 BTC 数据在 `/home/momo/data/higun/test`,配置文件设置:
```yaml
chain: "btc"
data_dir: "/home/momo/data/higun/test"
```

---

## 🐛 故障排查

### 问题 1: "chain field is required"
**原因**: 配置文件缺少 `chain` 字段  
**解决**: 在 config.yaml 开头添加 `chain: "btc"` 或 `chain: "mvc"`

### 问题 2: "chain mismatch"
**原因**: `chain` 和 `rpc.chain` 不一致  
**解决**: 确保两个字段值相同

### 问题 3: "unsupported chain"
**原因**: 链类型不支持  
**解决**: 只能使用 "btc" 或 "mvc"

### 问题 4: 数据目录权限错误
**原因**: 没有权限创建目录  
**解决**: 
```bash
sudo mkdir -p /data/higun/btc
sudo chown -R $USER:$USER /data/higun
```

### 问题 5: 端口已被占用
**原因**: API 端口冲突  
**解决**: 修改 `api_port` 为其他值

---

## 📊 工作量评估

| 任务 | 预计时间 |
|-----|---------|
| 修改 config.go | 30 分钟 |
| 修改 config.yaml | 2 分钟 |
| 编译测试 | 5 分钟 |
| 功能验证 | 30 分钟 |
| 创建配置模板 | 15 分钟 |
| **总计** | **~1.5 小时** |

---

## 🎉 完成后效果

### 单实例运行
```bash
# 运行 BTC 索引器
./utxo_indexer --config config_btc.yaml

# 或运行 MVC 索引器  
./utxo_indexer --config config_mvc.yaml
```

### 多实例运行
```bash
# 终端 1: BTC
./utxo_indexer --config config_btc_mainnet.yaml

# 终端 2: MVC
./utxo_indexer --config config_mvc_mainnet.yaml

# 终端 3: BTC Testnet
./utxo_indexer --config config_btc_testnet.yaml
```

### 数据目录结构(每个实例独立)
```
# 服务器A - BTC实例
/data/higun/btc_mainnet/
├── utxo/
├── income/
└── spend/

# 服务器B - MVC实例  
/data/higun/mvc_mainnet/
├── utxo/
├── income/
└── spend/

# 服务器C - DOGE实例
/data/higun/doge_mainnet/
├── utxo/
├── income/
└── spend/
```

---

## 📚 相关文档

- 详细设计方案: `MULTI_CHAIN_REFACTOR_PLAN.md`
- 配置示例: `docs/chain_config_examples.md`
- 代码修改清单: `docs/code_modification_checklist.md`
- 重构代码示例: `docs/config_refactored.go.example`

---

## 💡 扩展新链支持

### 示例: 添加 DOGE 链支持

**核心思想**: 只需添加 DOGE 特定的交易/地址解析代码,其他逻辑完全复用

#### 步骤 1: 添加链标识 (config/config.go)
```go
const (
	ChainBTC  = "btc"
	ChainMVC  = "mvc"
	ChainDOGE = "doge"  // 新增
)

// 在 ValidateChain 中添加
supportedChains := map[string]bool{
	ChainBTC:  true,
	ChainMVC:  true,
	ChainDOGE: true,  // 新增
}
```

#### 步骤 2: 添加 DOGE 交易解析器 (blockchain/client.go)
```go
// 在 GetBlockMsg 方法中添加 DOGE 分支
if chainName == "doge" {
	msgBlock := &wire.MsgBlock{}  // 使用 DOGE 的 wire 包
	if err := msgBlock.Deserialize(bytes.NewReader(blockBytes)); err != nil {
		return nil, 0, 0, 0, err
	}
	// ... 处理 DOGE 特定逻辑
	return msgBlock, txCount, expectedInTxCount, expectedOutTxCount, nil
}

// 添加 DOGE 交易转换方法
func (c *Client) convertDogeTxToIndexerTx(tx *wire.MsgTx) *indexer.Transaction {
	// DOGE 特定的交易处理
	// 地址格式、金额单位等
}
```

#### 步骤 3: 添加 DOGE 地址解析 (blockchain/util.go)
```go
// 在 GetAddressFromScript 中添加 DOGE 处理
if chainName == "doge" {
	// DOGE 特定的地址解析
	address = addrs[0].EncodeAddress()  // 使用 DOGE 地址格式
	return
}
```

#### 步骤 4: 创建配置文件
```yaml
# config_doge.yaml
chain: "doge"
network: "mainnet"
data_dir: "/data/higun/doge_instance"
api_port: "3003"
rpc:
  chain: "doge"
  host: "127.0.0.1"
  port: "22555"
  user: "doge"
  password: "password"
```

#### 步骤 5: 启动 DOGE 实例
```bash
./utxo_indexer --config config_doge.yaml
```

**就这么简单!** 所有的索引、查询、API 逻辑都自动复用,无需修改。

---

**预计总实施时间: 1.5 - 2 小时**  
**风险等级: 低**  
**向后兼容性: 完全兼容**
