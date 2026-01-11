# Chain Adapter 架构设计

## 🎯 核心思想

通过 **适配器模式** 实现链的完全解耦。每种链实现自己的适配器,包括:
- RPC 连接
- 区块获取
- 交易解析
- 地址解析
- 内存池处理

核心代码(索引器、存储、API)完全不关心具体是什么链。

## 📐 架构设计

```
┌─────────────────────────────────────────────────┐
│           Core Indexer (链无关)                  │
│  - UTXO 索引逻辑                                 │
│  - 数据存储逻辑                                  │
│  - API 服务                                      │
└────────────────┬────────────────────────────────┘
                 │
                 │ 调用 ChainAdapter 接口
                 │
         ┌───────▼───────┐
         │ ChainAdapter  │ (接口)
         │   Interface   │
         └───────┬───────┘
                 │
    ┌────────────┼────────────┐
    │            │            │
┌───▼────┐  ┌───▼────┐  ┌───▼────┐
│  BTC   │  │  MVC   │  │  DOGE  │
│Adapter │  │Adapter │  │Adapter │
└────────┘  └────────┘  └────────┘
```

## 📝 接口定义

### 1. ChainAdapter 接口

```go
// blockchain/adapter.go

package blockchain

import (
    "github.com/btcsuite/btcd/chaincfg"
    "github.com/metaid/utxo_indexer/indexer"
)

// ChainAdapter 链适配器接口
type ChainAdapter interface {
    // 初始化连接
    Connect() error
    
    // 获取最新区块高度
    GetBlockCount() (int, error)
    
    // 获取指定高度的区块哈希
    GetBlockHash(height int64) (string, error)
    
    // 获取区块数据并解析为标准格式
    GetBlock(height int64) (*indexer.Block, error)
    
    // 获取区块头信息
    GetBlockHeader(hash string) (*BlockHeader, error)
    
    // 获取内存池交易列表
    GetRawMempool() ([]string, error)
    
    // 获取单笔交易
    GetTransaction(txid string) (*indexer.Transaction, error)
    
    // 获取链参数(用于地址验证等)
    GetChainParams() *chaincfg.Params
    
    // 关闭连接
    Shutdown()
    
    // 获取链名称
    GetChainName() string
}

// BlockHeader 区块头信息
type BlockHeader struct {
    Hash              string
    Height            int64
    PreviousBlockHash string
    NextBlockHash     string
    Timestamp         int64
    Confirmations     int64
}
```

### 2. BTC 适配器实现

```go
// blockchain/adapter_btc.go

package blockchain

import (
    "fmt"
    "github.com/btcsuite/btcd/chaincfg"
    "github.com/btcsuite/btcd/rpcclient"
    "github.com/metaid/utxo_indexer/config"
    "github.com/metaid/utxo_indexer/indexer"
)

type BTCAdapter struct {
    rpcClient *rpcclient.Client
    cfg       *config.Config
    params    *chaincfg.Params
}

func NewBTCAdapter(cfg *config.Config) (*BTCAdapter, error) {
    connCfg := &rpcclient.ConnConfig{
        Host:         fmt.Sprintf("%s:%s", cfg.RPC.Host, cfg.RPC.Port),
        User:         cfg.RPC.User,
        Pass:         cfg.RPC.Password,
        HTTPPostMode: true,
        DisableTLS:   true,
    }
    
    client, err := rpcclient.New(connCfg, nil)
    if err != nil {
        return nil, err
    }
    
    params, _ := cfg.GetChainParams()
    
    return &BTCAdapter{
        rpcClient: client,
        cfg:       cfg,
        params:    params,
    }, nil
}

func (a *BTCAdapter) Connect() error {
    // BTC 连接验证
    _, err := a.rpcClient.GetBlockCount()
    return err
}

func (a *BTCAdapter) GetBlockCount() (int, error) {
    count, err := a.rpcClient.GetBlockCount()
    return int(count), err
}

func (a *BTCAdapter) GetBlockHash(height int64) (string, error) {
    hash, err := a.rpcClient.GetBlockHash(height)
    if err != nil {
        return "", err
    }
    return hash.String(), nil
}

func (a *BTCAdapter) GetBlock(height int64) (*indexer.Block, error) {
    // 1. 获取区块哈希
    hashStr, err := a.GetBlockHash(height)
    if err != nil {
        return nil, err
    }
    
    // 2. 获取原始区块数据
    blockHex, err := a.getRawBlock(hashStr)
    if err != nil {
        return nil, err
    }
    
    // 3. 解析为 BTC 区块
    msgBlock, err := a.parseBlockHex(blockHex)
    if err != nil {
        return nil, err
    }
    
    // 4. 转换为标准索引器格式
    return a.convertToIndexerBlock(msgBlock, height, hashStr)
}

func (a *BTCAdapter) GetTransaction(txid string) (*indexer.Transaction, error) {
    // BTC 交易获取和解析
    txHash, _ := chainhash.NewHashFromStr(txid)
    tx, err := a.rpcClient.GetRawTransaction(txHash)
    if err != nil {
        return nil, err
    }
    return a.convertBTCTxToIndexerTx(tx.MsgTx()), nil
}

func (a *BTCAdapter) GetRawMempool() ([]string, error) {
    hashes, err := a.rpcClient.GetRawMempool()
    if err != nil {
        return nil, err
    }
    txids := make([]string, len(hashes))
    for i, hash := range hashes {
        txids[i] = hash.String()
    }
    return txids, nil
}

func (a *BTCAdapter) GetChainParams() *chaincfg.Params {
    return a.params
}

func (a *BTCAdapter) Shutdown() {
    a.rpcClient.Shutdown()
}

func (a *BTCAdapter) GetChainName() string {
    return "btc"
}

// BTC 特定的私有方法
func (a *BTCAdapter) getRawBlock(hash string) (string, error) {
    // 实现获取原始区块数据
}

func (a *BTCAdapter) parseBlockHex(blockHex string) (*wire.MsgBlock, error) {
    // 实现解析区块
}

func (a *BTCAdapter) convertToIndexerBlock(msgBlock *wire.MsgBlock, height int64, hash string) (*indexer.Block, error) {
    // 转换为标准格式
    block := &indexer.Block{
        Height:       int(height),
        BlockHash:    hash,
        Transactions: make([]*indexer.Transaction, len(msgBlock.Transactions)),
    }
    
    for i, tx := range msgBlock.Transactions {
        block.Transactions[i] = a.convertBTCTxToIndexerTx(tx)
    }
    
    return block, nil
}

func (a *BTCAdapter) convertBTCTxToIndexerTx(tx *wire.MsgTx) *indexer.Transaction {
    // BTC 交易转换逻辑
    inputs := make([]*indexer.Input, len(tx.TxIn))
    for i, in := range tx.TxIn {
        inputs[i] = &indexer.Input{
            TxPoint: fmt.Sprintf("%s:%d", in.PreviousOutPoint.Hash, in.PreviousOutPoint.Index),
        }
    }
    
    outputs := make([]*indexer.Output, len(tx.TxOut))
    for i, out := range tx.TxOut {
        address := a.extractAddress(out.PkScript)
        outputs[i] = &indexer.Output{
            Address: address,
            Amount:  fmt.Sprintf("%d", out.Value),
        }
    }
    
    return &indexer.Transaction{
        ID:      tx.TxHash().String(),
        Inputs:  inputs,
        Outputs: outputs,
    }
}

func (a *BTCAdapter) extractAddress(pkScript []byte) string {
    // BTC 地址提取
    _, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, a.params)
    if err == nil && len(addrs) > 0 {
        return addrs[0].String()
    }
    return "errAddress"
}
```

### 3. MVC 适配器实现

```go
// blockchain/adapter_mvc.go

package blockchain

import (
    "github.com/bitcoinsv/bsvd/wire"
    "github.com/metaid/utxo_indexer/indexer"
)

type MVCAdapter struct {
    rpcClient *rpcclient.Client
    cfg       *config.Config
    params    *chaincfg.Params
}

func NewMVCAdapter(cfg *config.Config) (*MVCAdapter, error) {
    // MVC 特定的连接配置
    // ...
}

func (a *MVCAdapter) GetBlock(height int64) (*indexer.Block, error) {
    // MVC 特定的区块获取逻辑
    // 使用 bsvwire 包解析
}

func (a *MVCAdapter) convertMVCTxToIndexerTx(tx *wire.MsgTx) *indexer.Transaction {
    // MVC 交易转换逻辑
    // 处理 MVC 特定的交易哈希算法
    txid, _ := GetNewHash2(tx)
    // ...
}

func (a *MVCAdapter) extractAddress(pkScript []byte) string {
    // MVC 地址格式处理
    _, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, a.params)
    if err == nil && len(addrs) > 0 {
        return addrs[0].EncodeAddress() // MVC 使用不同的编码
    }
    return "errAddress"
}

// 实现其他接口方法...
```

### 4. DOGE 适配器实现(扩展示例)

```go
// blockchain/adapter_doge.go

package blockchain

type DOGEAdapter struct {
    rpcClient *rpcclient.Client
    cfg       *config.Config
    params    *chaincfg.Params
}

func NewDOGEAdapter(cfg *config.Config) (*DOGEAdapter, error) {
    // DOGE 特定实现
}

// 实现 ChainAdapter 接口的所有方法
```

## 🔧 适配器工厂

```go
// blockchain/factory.go

package blockchain

import (
    "fmt"
    "github.com/metaid/utxo_indexer/config"
)

// NewChainAdapter 根据配置创建对应的链适配器
func NewChainAdapter(cfg *config.Config) (ChainAdapter, error) {
    switch cfg.Chain {
    case config.ChainBTC:
        return NewBTCAdapter(cfg)
    case config.ChainMVC:
        return NewMVCAdapter(cfg)
    case config.ChainDOGE:
        return NewDOGEAdapter(cfg)
    default:
        return nil, fmt.Errorf("unsupported chain: %s", cfg.Chain)
    }
}
```

## 📦 核心代码改造

### 修改 main.go

```go
// main.go

func main() {
    cfg, params := initConfig()
    
    // 创建链适配器(自动选择)
    chainAdapter, err := blockchain.NewChainAdapter(cfg)
    if err != nil {
        log.Fatalf("Failed to create chain adapter: %v", err)
    }
    defer chainAdapter.Shutdown()
    
    // 连接测试
    if err := chainAdapter.Connect(); err != nil {
        log.Fatalf("Failed to connect to chain: %v", err)
    }
    
    log.Printf("Connected to %s chain", chainAdapter.GetChainName())
    
    // 初始化索引器(不关心具体链类型)
    idx := indexer.NewUTXOIndexer(params, utxoStore, addressStore, metaStore, spendStore)
    
    // 区块同步(使用适配器)
    go SyncBlocks(chainAdapter, idx, checkInterval, stopCh)
    
    // ... 其他逻辑
}
```

### 修改区块同步逻辑

```go
// blockchain/sync.go

func SyncBlocks(adapter ChainAdapter, idx *indexer.UTXOIndexer, 
    checkInterval time.Duration, stopCh <-chan struct{}) error {
    
    for {
        select {
        case <-stopCh:
            return nil
        default:
        }
        
        // 获取当前高度(通过适配器)
        currentHeight, err := adapter.GetBlockCount()
        if err != nil {
            return err
        }
        
        lastHeight, _ := idx.GetLastIndexedHeight()
        
        // 同步新区块
        for height := lastHeight + 1; height <= currentHeight; height++ {
            // 通过适配器获取区块(已解析为统一格式)
            block, err := adapter.GetBlock(int64(height))
            if err != nil {
                return err
            }
            
            // 索引器处理(完全不关心链类型)
            if err := idx.IndexBlock(block); err != nil {
                return err
            }
        }
        
        time.Sleep(checkInterval)
    }
}
```

### 内存池适配器

```go
// mempool/adapter.go

type MempoolAdapter interface {
    // 连接到 ZMQ
    Connect(zmqAddresses []string) error
    
    // 获取内存池交易
    GetMempoolTxs() ([]string, error)
    
    // 订阅新交易
    SubscribeNewTx(handler func(tx *indexer.Transaction))
    
    // 关闭
    Close()
}

// BTC 内存池适配器
type BTCMempoolAdapter struct {
    chainAdapter ChainAdapter
    // ...
}

// MVC 内存池适配器  
type MVCMempoolAdapter struct {
    chainAdapter ChainAdapter
    // ...
}
```

## 🎯 优势

### 1. 完全解耦
- 核心代码不知道具体链类型
- 添加新链无需修改核心逻辑

### 2. 易于测试
```go
// 可以创建 Mock 适配器用于测试
type MockAdapter struct {}
func (m *MockAdapter) GetBlock(height int64) (*indexer.Block, error) {
    // 返回测试数据
}
```

### 3. 易于扩展
添加新链只需:
1. 实现 `ChainAdapter` 接口
2. 在 factory 中注册
3. 创建配置文件

### 4. 灵活性
- 同一个链可以有多个适配器实现(HTTP RPC、gRPC 等)
- 可以动态切换适配器

## 📁 文件结构

```
blockchain/
├── adapter.go          # 接口定义
├── factory.go          # 适配器工厂
├── adapter_btc.go      # BTC 实现
├── adapter_mvc.go      # MVC 实现
├── adapter_doge.go     # DOGE 实现
├── sync.go             # 通用同步逻辑(使用适配器)
└── util.go             # 通用工具函数

mempool/
├── adapter.go          # 内存池适配器接口
├── adapter_btc.go      # BTC 内存池
└── adapter_mvc.go      # MVC 内存池
```

## 🚀 使用示例

### 启动 BTC 实例
```yaml
# config_btc.yaml
chain: "btc"
# ...
```

```bash
./utxo_indexer --config config_btc.yaml
# 自动使用 BTCAdapter
```

### 启动 MVC 实例  
```yaml
# config_mvc.yaml
chain: "mvc"
# ...
```

```bash
./utxo_indexer --config config_mvc.yaml
# 自动使用 MVCAdapter
```

### 添加 DOGE 支持
1. 创建 `adapter_doge.go` 实现接口
2. 在 `factory.go` 添加 case
3. 完成!

## 💡 关键点

1. **接口是核心** - 所有链必须实现相同接口
2. **转换在适配器内** - 每个适配器负责转换为标准格式
3. **核心代码链无关** - 索引器、存储、API 完全不关心链类型
4. **工厂模式创建** - 根据配置自动选择适配器

这才是真正的可扩展架构! 🎉
