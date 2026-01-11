# Chain Adapter 实施指南

## 🎯 这才是你真正需要的架构!

通过 **适配器模式** 实现链的完全解耦:
- ✅ 核心代码(索引器、存储、API)完全不知道链类型
- ✅ 每个链实现自己的适配器(连接、解析、内存池)
- ✅ 添加新链只需实现适配器接口
- ✅ 所有功能自动复用

## 📐 架构图

```
核心层(链无关)
├── Indexer  ──┐
├── Storage  ──┤
└── API      ──┤
               │
            调用接口
               │
          ┌────▼────┐
          │ Adapter │ 接口
          │Interface│
          └────┬────┘
               │
    ┌──────────┼──────────┐
    │          │          │
┌───▼──┐  ┌───▼──┐  ┌───▼──┐
│ BTC  │  │ MVC  │  │ DOGE │
│Adapter│  │Adapter│  │Adapter│
└──────┘  └──────┘  └──────┘
```

## 🔧 实施步骤

### 第一步: 创建适配器接口 (5分钟)

**文件**: `blockchain/adapter.go`

```go
type ChainAdapter interface {
    // 连接管理
    Connect() error
    Shutdown()
    GetChainName() string
    
    // 区块数据(核心)
    GetBlockCount() (int, error)
    GetBlockHash(height int64) (string, error)
    GetBlock(height int64) (*indexer.Block, error)  // 返回统一格式
    
    // 交易和内存池
    GetTransaction(txid string) (*indexer.Transaction, error)
    GetRawMempool() ([]string, error)
}
```

参考: `docs/adapter_interface.go.example`

### 第二步: 创建适配器工厂 (2分钟)

**文件**: `blockchain/factory.go`

```go
func NewChainAdapter(cfg *config.Config) (ChainAdapter, error) {
    switch cfg.Chain {
    case "btc":
        return NewBTCAdapter(cfg)
    case "mvc":
        return NewMVCAdapter(cfg)
    case "doge":
        return NewDOGEAdapter(cfg)
    default:
        return nil, fmt.Errorf("unsupported chain: %s", cfg.Chain)
    }
}
```

参考: `docs/adapter_factory.go.example`

### 第三步: 实现 BTC 适配器 (30分钟)

**文件**: `blockchain/adapter_btc.go`

核心方法:
1. `GetBlock()` - 获取并解析 BTC 区块
2. `convertBTCTxToIndexerTx()` - 转换为统一格式
3. `extractAddress()` - 提取 BTC 地址

参考: `docs/adapter_btc.go.example`

### 第四步: 实现 MVC 适配器 (30分钟)

**文件**: `blockchain/adapter_mvc.go`

核心差异:
- 使用 `bsvwire.MsgBlock` 解析
- 使用 `GetNewHash2()` 计算交易哈希
- 使用 `EncodeAddress()` 编码地址

参考: `docs/adapter_mvc.go.example`

### 第五步: 改造 main.go (15分钟)

```go
func main() {
    cfg, params := initConfig()
    
    // 创建适配器(自动选择)
    adapter, err := blockchain.NewChainAdapter(cfg)
    if err != nil {
        log.Fatal(err)
    }
    defer adapter.Shutdown()
    
    // 连接
    adapter.Connect()
    log.Printf("Connected to %s", adapter.GetChainName())
    
    // 创建索引器(不关心链类型)
    idx := indexer.NewUTXOIndexer(...)
    
    // 同步区块(使用适配器)
    go syncBlocks(adapter, idx, ...)
}

func syncBlocks(adapter ChainAdapter, idx *indexer.UTXOIndexer, ...) {
    for {
        height := ...
        // 通过适配器获取(已转换为统一格式)
        block, _ := adapter.GetBlock(height)
        // 索引器处理(完全不关心链类型)
        idx.IndexBlock(block)
    }
}
```

参考: `docs/main_with_adapter.go.example`

## 🚀 使用示例

### 启动 BTC 实例
```yaml
# config_btc.yaml
chain: "btc"
data_dir: "/data/higun/btc"
# ...
```

```bash
./utxo_indexer --config config_btc.yaml
# 输出: Connected to btc
```

### 启动 MVC 实例
```yaml
# config_mvc.yaml
chain: "mvc"
data_dir: "/data/higun/mvc"
# ...
```

```bash
./utxo_indexer --config config_mvc.yaml
# 输出: Connected to mvc
```

## ✨ 添加新链(DOGE 示例)

### 步骤 1: 实现适配器 (1小时)

```go
// blockchain/adapter_doge.go

type DOGEAdapter struct {
    rpcClient *rpcclient.Client
    cfg       *config.Config
}

func NewDOGEAdapter(cfg *config.Config) (*DOGEAdapter, error) {
    // DOGE RPC 连接
}

func (a *DOGEAdapter) GetBlock(height int64) (*indexer.Block, error) {
    // 1. 获取 DOGE 原始区块
    // 2. 使用 DOGE wire 包解析
    // 3. 转换为统一格式
}

func (a *DOGEAdapter) convertDOGETxToIndexerTx(tx *wire.MsgTx) *indexer.Transaction {
    // DOGE 交易转换
    // 处理 DOGE 特定的地址格式、金额单位等
}
```

### 步骤 2: 注册到工厂 (1分钟)

```go
// blockchain/factory.go
func NewChainAdapter(cfg *config.Config) (ChainAdapter, error) {
    switch cfg.Chain {
    case "btc":
        return NewBTCAdapter(cfg)
    case "mvc":
        return NewMVCAdapter(cfg)
    case "doge":
        return NewDOGEAdapter(cfg)  // 添加这一行
    }
}
```

### 步骤 3: 创建配置 (1分钟)

```yaml
chain: "doge"
data_dir: "/data/higun/doge"
rpc:
  chain: "doge"
  host: "127.0.0.1"
  port: "22555"
```

### 步骤 4: 启动 (完成!)

```bash
./utxo_indexer --config config_doge.yaml
# 所有功能自动可用!
```

## 💡 核心优势

### 1. 完全解耦
```go
// 索引器完全不知道链类型
func (idx *UTXOIndexer) IndexBlock(block *Block) error {
    // 处理统一格式的区块
    // BTC? MVC? DOGE? 不关心!
}
```

### 2. 易于测试
```go
// 创建 Mock 适配器
type MockAdapter struct {}
func (m *MockAdapter) GetBlock(h int64) (*Block, error) {
    return &Block{Height: int(h)}, nil
}

// 测试索引器
idx := NewUTXOIndexer(...)
idx.IndexBlock(mockAdapter.GetBlock(100))
```

### 3. 灵活替换
```go
// 可以为同一个链创建多个适配器实现
type BTCHttpAdapter struct {}    // HTTP RPC
type BTCGrpcAdapter struct {}    // gRPC
type BTCRestAdapter struct {}    // REST API

// 运行时选择
adapter := createAdapter(cfg.AdapterType)
```

## 📁 文件结构

```
blockchain/
├── adapter.go           # 接口定义 ⭐
├── factory.go           # 工厂方法 ⭐
├── adapter_btc.go       # BTC 实现 ⭐
├── adapter_mvc.go       # MVC 实现 ⭐
├── adapter_doge.go      # DOGE 实现(新增)
└── util.go              # 通用工具

main.go                  # 使用适配器 ⭐

indexer/                 # 完全不改
storage/                 # 完全不改
api/                     # 完全不改
```

## ⚠️ 关键点

1. **接口是契约** - 所有适配器必须实现相同接口
2. **转换在适配器内** - 每个适配器负责转换为 `indexer.Block` 格式
3. **核心代码链无关** - 索引器、存储、API 完全不关心链类型
4. **工厂创建适配器** - 根据配置自动选择
5. **数据格式统一** - 所有适配器返回相同的 `indexer.Block` 结构

## 📊 工作量评估

| 任务 | 时间 | 优先级 |
|-----|------|--------|
| 定义接口 | 15分钟 | P0 |
| 创建工厂 | 5分钟 | P0 |
| BTC 适配器 | 30分钟 | P0 |
| MVC 适配器 | 30分钟 | P0 |
| 改造 main.go | 20分钟 | P0 |
| 测试验证 | 30分钟 | P0 |
| **总计** | **~2小时** | |

添加新链(如 DOGE):
| 任务 | 时间 |
|-----|------|
| 实现适配器 | 1小时 |
| 注册工厂 | 1分钟 |
| 创建配置 | 1分钟 |
| **总计** | **~1小时** |

## 📚 参考文档

- **接口定义**: `docs/adapter_interface.go.example`
- **工厂方法**: `docs/adapter_factory.go.example`
- **BTC 实现**: `docs/adapter_btc.go.example`
- **MVC 实现**: `docs/adapter_mvc.go.example`
- **主程序**: `docs/main_with_adapter.go.example`
- **架构设计**: `docs/CHAIN_ADAPTER_DESIGN.md`

## ✅ 实施检查清单

- [ ] 创建 `blockchain/adapter.go` 接口
- [ ] 创建 `blockchain/factory.go` 工厂
- [ ] 实现 `blockchain/adapter_btc.go`
- [ ] 实现 `blockchain/adapter_mvc.go`
- [ ] 改造 `main.go` 使用适配器
- [ ] 删除或重构 `blockchain/client.go` 中的链特定代码
- [ ] 测试 BTC 链
- [ ] 测试 MVC 链
- [ ] 验证核心代码链无关

---

**这才是真正可扩展的架构!** 🎉

添加新链 = 实现适配器 + 注册工厂 + 完成!
