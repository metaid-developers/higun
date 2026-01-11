# 多链适配器架构 - 使用说明

## 概述

本项目已重构为支持多条 UTXO 链的架构，通过**适配器模式**实现一套代码适配不同链。

## 核心设计

### 1. 架构图

```
配置文件 (config.yaml)
    ↓
工厂方法 (NewChainAdapter)
    ↓
具体适配器 (BTCAdapter / MVCAdapter / DOGEAdapter)
    ↓
统一接口 (ChainAdapter)
    ↓
业务逻辑 (Indexer / Storage / API)
```

### 2. 关键组件

- **ChainAdapter 接口** (`blockchain/adapter.go`): 定义所有链必须实现的方法
- **工厂方法** (`blockchain/factory.go`): 根据配置自动选择适配器
- **具体适配器**:
  - `blockchain/adapter_btc.go` - BTC 链适配器
  - `blockchain/adapter_mvc.go` - MVC 链适配器
  - `blockchain/adapter_doge.go` - DOGE 链适配器 ✅

### 3. 接口定义

```go
type ChainAdapter interface {
    Connect() error
    Shutdown()
    GetChainName() string
    GetChainParams() *chaincfg.Params
    GetBlockCount() (int, error)
    GetBlockHash(height int64) (string, error)
    GetBlock(height int64) (*indexer.Block, error)  // 核心方法
    GetTransaction(txid string) (*indexer.Transaction, error)
    GetRawMempool() ([]string, error)
    FindReorgHeight() (int, int)
}
```

## 使用方法

### 1. 配置文件

在 `config.yaml` 中设置链类型：

```yaml
chain: "btc"  # 支持: btc, mvc, doge

rpc:
  chain: "btc"  # 必须与上面一致
  host: "127.0.0.1"
  port: "18443"
  user: "rpc_user"
  password: "rpc_password"
```

### 2. 运行不同链

#### BTC 链
```bash
# 使用默认配置 (config.yaml)
./utxo_indexer

# 或使用环境变量
export CHAIN=btc
export RPC_HOST=127.0.0.1
export RPC_PORT=18443
./utxo_indexer
```

#### MVC 链
```bash
# 使用 MVC 配置文件
./utxo_indexer -config config_mvc_example.yaml

# 或使用环境变量
export CHAIN=mvc
export RPC_HOST=127.0.0.1
export RPC_PORT=9882
./utxo_indexer
```

#### DOGE 链
```bash
# 使用 DOGE 配置文件
./utxo_indexer -config config_doge_example.yaml

# 或使用环境变量
export CHAIN=doge
export RPC_HOST=127.0.0.1
export RPC_PORT=22555
./utxo_indexer
```

### 3. 验证运行

启动后会看到类似日志：

```
Starting UTXO Indexer...
Initializing blockchain adapter: chain=btc
✓ Blockchain adapter initialized successfully: btc
✓ Connected to BTC node successfully
Starting block synchronization...
```

## 配置示例

### BTC 配置 (config.yaml)
```yaml
chain: "btc"
network: "regtest"
data_dir: "/data/higun/btc"
api_port: "3001"
rpc:
  chain: "btc"
  host: "127.0.0.1"
  port: "18443"
  user: "test"
  password: "test"
```

### MVC 配置 (config_mvc_example.yaml)
```yaml
chain: "mvc"
network: "mainnet"
data_dir: "/data/higun/mvc"
api_port: "3002"
rpc:
  chain: "mvc"
  host: "127.0.0.1"
  port: "9882"
  user: "mvc_rpc_user"
  password: "mvc_rpc_password"
```

## 开发指南

### 添加新链支持

1. 创建新的适配器文件 `blockchain/adapter_xxx.go`
2. 实现 `ChainAdapter` 接口的所有方法
3. 在 `blockchain/factory.go` 中添加 case 分支
4. 在 `config/config.go` 中添加新的链常量

示例：
```go
// blockchain/adapter_doge.go
type DOGEAdapter struct {
    rpcClient *rpcclient.Client
    cfg       *config.Config
    params    *chaincfg.Params
}

func NewDOGEAdapter(cfg *config.Config) (*DOGEAdapter, error) {
    // 实现 DOGE 特定的初始化逻辑
}

func (a *DOGEAdapter) GetBlock(height int64) (*indexer.Block, error) {
    // 实现 DOGE 特定的区块获取逻辑
}
// ... 实现其他接口方法
```

```go
// blockchain/factory.go
case config.ChainDOGE:
    return NewDOGEAdapter(cfg)
```

### 核心差异处理

不同链的主要差异由适配器封装：

| 差异点 | BTC | MVC | DOGE |
|--------|-----|-----|------|
| Wire 包 | `btcd/wire` | `bsvd/wire` | `btcd/wire` |
| 交易 ID | `tx.TxHash().String()` | `GetNewHash2(tx)` | `tx.TxHash().String()` |
| 地址编码 | `addr.String()` | `addr.EncodeAddress()` | `addr.EncodeAddress()` |
| 区块结构 | `wire.MsgBlock` | `bsvwire.MsgBlock` | `wire.MsgBlock` |
| 地址前缀 | '1', '3', 'bc1' | '1' | 'D', '9', 'A' |
| 地址参数 | 标准 BTC | 标准 BTC | 0x1e, 0x16 |

这些差异在各自的适配器中处理，业务逻辑无需关心。

## 测试

### 单元测试
```bash
# 测试 BTC 适配器
go test ./blockchain -run TestBTCAdapter

# 测试 MVC 适配器
go test ./blockchain -run TestMVCAdapter
```

### 集成测试
```bash
# 使用 regtest 网络测试
./scripts/test_btc_adapter.sh
./scripts/test_mvc_adapter.sh
```

## 架构优势

1. **代码复用**: 核心逻辑（索引、存储、API）完全通用
2. **易于扩展**: 新增链只需实现适配器接口
3. **类型安全**: 编译时检查接口实现
4. **配置驱动**: 无需重新编译即可切换链
5. **向后兼容**: 保留了原有 Client 结构的兼容模式

## 常见问题

### Q: 如何验证适配器是否正确实现？
A: 编译时会自动检查接口实现。也可以运行：
```go
var _ ChainAdapter = (*BTCAdapter)(nil)  // 编译时验证
```

### Q: 可以同时运行多条链吗？
A: 可以。每条链使用独立的配置文件和数据目录，运行不同实例即可：
```bash
./utxo_indexer -config config_btc.yaml &
./utxo_indexer -config config_mvc.yaml &
```

### Q: 如何切换链？
A: 修改 `config.yaml` 中的 `chain` 字段，或使用环境变量 `CHAIN`。

### Q: 旧代码还能用吗？
A: 可以。如果 `adapter` 为 `nil`，会自动回退到旧的实现模式。

## 相关文档

- [适配器架构设计](docs/CHAIN_ADAPTER_DESIGN.md)
- [实现指南](docs/ADAPTER_IMPLEMENTATION_GUIDE.md)
- [配置示例](docs/chain_config_examples.md)
- [接口定义](blockchain/adapter.go)

## 版本历史

- **v2.0.0** (2024-01) - 引入适配器架构，支持 BTC/MVC
- **v1.0.0** - 初始版本，仅支持 BTC

---

如有问题，请查看文档或提交 issue。
