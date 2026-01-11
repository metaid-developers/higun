# 多链支持代码修改清单

## 修改概览

### 必须修改的文件 (共 3 个)

1. ✅ **config/config.go** - 配置管理增强
2. ✅ **config.yaml** - 配置文件模板
3. ⚠️ **blockchain/client.go** - 已基本支持,需确认

### 可选修改的文件 (优化建议)

4. **main.go** - 添加启动日志
5. **mempool/transaction.go** - 减少全局变量依赖

---

## 详细修改说明

### 1. config/config.go (必须修改)

#### 需要添加的内容:

```go
// 在文件开头添加常量
const (
	ChainBTC = "btc"
	ChainMVC = "mvc"
)

// 在 Config 结构体中添加字段
type Config struct {
	Chain string `yaml:"chain"` // 新增字段,放在第一个
	// ... 其他现有字段
}

// 添加新方法
func (c *Config) ValidateChain() error {
	if c.Chain == "" {
		return fmt.Errorf("chain field is required")
	}
	supportedChains := map[string]bool{
		ChainBTC: true,
		ChainMVC: true,
	}
	if !supportedChains[c.Chain] {
		return fmt.Errorf("unsupported chain: %s", c.Chain)
	}
	if c.Chain != c.RPC.Chain {
		return fmt.Errorf("chain mismatch: config.chain=%s but rpc.chain=%s", 
			c.Chain, c.RPC.Chain)
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
	if strings.Contains(c.DataDir, "/"+c.Chain+"/") || 
	   strings.HasSuffix(c.DataDir, "/"+c.Chain) {
		return c.DataDir
	}
	return fmt.Sprintf("%s/%s", strings.TrimSuffix(c.DataDir, "/"), c.Chain)
}
```

#### 在 LoadConfig 函数中添加:

```go
func LoadConfig(path string) (*Config, error) {
	// ... 现有代码 ...
	
	cfg := &Config{
		Chain: ChainBTC, // 添加默认值
		// ... 其他现有默认值 ...
		RPC: RPCConfig{
			Chain: ChainBTC, // 添加默认值
			// ... 其他现有默认值 ...
		},
	}
	
	// ... 加载配置文件的代码 ...
	
	// 环境变量支持 (添加 CHAIN 支持)
	if chain := os.Getenv("CHAIN"); chain != "" {
		cfg.Chain = chain
	}
	
	// 验证链配置 (新增)
	if err := cfg.ValidateChain(); err != nil {
		return nil, fmt.Errorf("chain configuration validation failed: %w", err)
	}
	
	// 使用链特定目录 (修改)
	dataDir := cfg.GetChainDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	cfg.DataDir = dataDir
	
	fmt.Printf("Initialized for chain: %s, network: %s\n", cfg.GetChainName(), cfg.Network)
	fmt.Printf("Data directory: %s\n", dataDir)
	
	// ... 其余代码 ...
}
```

---

### 2. config.yaml (必须修改)

在文件开头添加 chain 字段:

```yaml
# 链类型标识 (新增)
chain: "btc"  # 可选: btc, mvc

# UTXO Indexer Configuration
network: "regtest"
block_info_indexer: true
# ... 其他现有配置保持不变 ...
```

**重要**: 确保 `rpc.chain` 与顶层 `chain` 字段一致

---

### 3. blockchain/client.go (检查确认)

**当前状态**: 代码已经通过 `c.cfg.RPC.Chain` 区分不同链

**需要确认的点**:

✅ `GetBlockMsg` 方法是否正确处理不同链  
✅ `convertMvcTxToIndexerTx` 和 `convertBtcTxToIndexerTx` 是否完整  
✅ `GetAddressFromScript` 是否正确处理不同链的地址格式

**可能需要的改进** (如果 GetBlockMsg 不存在):

```go
// 添加统一的区块消息获取方法
func (c *Client) GetBlockMsg(chainName string, height int64) (
	msgBlockInterface interface{}, 
	txCount int, 
	expectedInTxCount int, 
	expectedOutTxCount int, 
	err error) {
	
	hash, err := c.GetBlockHash(height)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	
	// 获取原始区块数据
	resp, err := c.rpcClient.RawRequest("getblock", []json.RawMessage{
		json.RawMessage(fmt.Sprintf("\"%s\"", hash.String())),
		json.RawMessage("0"),
	})
	if err != nil {
		return nil, 0, 0, 0, err
	}
	
	var blockHex string
	if err := json.Unmarshal(resp, &blockHex); err != nil {
		return nil, 0, 0, 0, err
	}
	
	blockBytes, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	
	// 根据链类型反序列化
	if chainName == "mvc" {
		msgBlock := &bsvwire.MsgBlock{}
		if err := msgBlock.Deserialize(bytes.NewReader(blockBytes)); err != nil {
			return nil, 0, 0, 0, err
		}
		txCount = len(msgBlock.Transactions)
		// 计算预期的输入输出数量
		for _, tx := range msgBlock.Transactions {
			expectedInTxCount += len(tx.TxIn)
			expectedOutTxCount += len(tx.TxOut)
		}
		return msgBlock, txCount, expectedInTxCount, expectedOutTxCount, nil
	} else {
		msgBlock := &wire.MsgBlock{}
		if err := msgBlock.Deserialize(bytes.NewReader(blockBytes)); err != nil {
			return nil, 0, 0, 0, err
		}
		txCount = len(msgBlock.Transactions)
		for _, tx := range msgBlock.Transactions {
			expectedInTxCount += len(tx.TxIn)
			expectedOutTxCount += len(tx.TxOut)
		}
		return msgBlock, txCount, expectedInTxCount, expectedOutTxCount, nil
	}
}
```

---

### 4. main.go (可选优化)

在 `initConfig` 函数后添加日志:

```go
func initConfig() (cfg *config.Config, params config.IndexerParams) {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	// 新增: 输出链信息
	log.Printf("=================================")
	log.Printf("Chain: %s", cfg.GetChainName())
	log.Printf("Network: %s", cfg.Network)
	log.Printf("Data Directory: %s", cfg.DataDir)
	log.Printf("API Port: %s", cfg.APIPort)
	log.Printf("=================================")
	
	// ... 其余代码保持不变 ...
}
```

---

### 5. mempool/transaction.go (可选优化)

**当前问题**: 使用全局变量 `config.GlobalConfig.RPC.Chain`

**优化建议**: 将 chain 作为参数传递

```go
// 修改前
if config.GlobalConfig.RPC.Chain == "mvc" {
	// ...
}

// 修改后 (推荐但不必须)
type MempoolManager struct {
	// ... 现有字段 ...
	chainName string // 添加链名称字段
}

func NewMempoolManager(dataDir string, utxoStore *storage.PebbleStore, 
	chainCfg *chaincfg.Params, zmqAddresses []string) *MempoolManager {
	return &MempoolManager{
		// ... 现有初始化 ...
		chainName: config.GlobalConfig.GetChainName(), // 保存链名称
	}
}

// 然后在方法中使用 m.chainName 替代 config.GlobalConfig.RPC.Chain
```

---

## 修改步骤

### 步骤 1: 备份 (5分钟)
```bash
cd /home/momo/projects/metaid/higun
cp config/config.go config/config.go.bak
cp config.yaml config.yaml.bak
```

### 步骤 2: 修改 config/config.go (20分钟)
- 添加常量定义
- 添加 Chain 字段到 Config 结构体
- 添加 ValidateChain, GetChainName, GetChainDataDir 方法
- 修改 LoadConfig 函数

### 步骤 3: 修改 config.yaml (2分钟)
- 在文件开头添加 `chain: "btc"` 或 `chain: "mvc"`
- 确认 rpc.chain 与 chain 一致

### 步骤 4: 测试编译 (5分钟)
```bash
go build -o utxo_indexer_new
```

### 步骤 5: 验证配置 (10分钟)
```bash
# 测试 BTC 配置
cp config.yaml config_btc_test.yaml
# 修改 chain: "btc"
./utxo_indexer_new --config config_btc_test.yaml

# 测试 MVC 配置  
cp config.yaml config_mvc_test.yaml
# 修改 chain: "mvc"
./utxo_indexer_new --config config_mvc_test.yaml
```

### 步骤 6: 验证数据隔离 (10分钟)
```bash
# 检查数据目录是否正确创建
ls -la /home/momo/data/higun/test/btc/
ls -la /home/momo/data/higun/test/mvc/

# 确认日志中显示正确的链信息
```

---

## 测试检查清单

- [ ] 配置文件正确加载
- [ ] Chain 字段验证正常工作
- [ ] Chain 和 RPC.Chain 不一致时报错
- [ ] 数据目录按链隔离
- [ ] BTC 链能正常同步区块
- [ ] MVC 链能正常同步区块
- [ ] 不同配置可以同时运行多个实例
- [ ] API 端口不冲突
- [ ] 日志输出正确的链信息

---

## 回滚方案

如果出现问题,快速回滚:

```bash
cd /home/momo/projects/metaid/higun
cp config/config.go.bak config/config.go
cp config.yaml.bak config.yaml
go build -o utxo_indexer
```

---

## 常见问题

### Q1: 配置验证失败怎么办?
**A**: 检查 `chain` 和 `rpc.chain` 是否一致

### Q2: 数据目录创建失败?
**A**: 检查磁盘空间和目录权限

### Q3: 多个实例端口冲突?
**A**: 确保每个配置文件的 `api_port` 不同

### Q4: 现有数据如何迁移?
**A**: 
```bash
# 假设当前数据在 /home/momo/data/higun/test
# 迁移到链特定目录
mkdir -p /home/momo/data/higun/test/btc
mv /home/momo/data/higun/test/utxo /home/momo/data/higun/test/btc/
mv /home/momo/data/higun/test/income /home/momo/data/higun/test/btc/
mv /home/momo/data/higun/test/spend /home/momo/data/higun/test/btc/
```

---

## 总结

### 最小修改方案
如果只想快速支持多链,**只需修改 2 个文件**:
1. `config/config.go` - 添加 Chain 字段和验证
2. `config.yaml` - 添加 chain 字段

### 推荐修改方案
为了更好的可维护性,建议完成清单中的所有修改。

### 预计工作量
- 核心修改: 1-2 小时
- 测试验证: 2-3 小时
- 文档更新: 1 小时
- **总计: 4-6 小时**
