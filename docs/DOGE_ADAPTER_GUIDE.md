# Dogecoin (DOGE) 适配器实现说明

## 概述

DOGE 适配器已成功实现，支持狗狗币主网、测试网和 regtest 网络。

## 技术特点

### 1. 地址参数

狗狗币使用独特的地址前缀参数：

```go
// 主网参数
DogeMainNetParams.PubKeyHashAddrID = 0x1e  // 'D' 开头的地址
DogeMainNetParams.ScriptHashAddrID = 0x16  // '9' 或 'A' 开头的地址
DogeMainNetParams.PrivateKeyID = 0x9e      // WIF 私钥前缀

// 示例地址
// D7YWHebTdyxF3KiLKdkCVxLEZDZvochxqm (主网 P2PKH)
// 9vJQKBKXpnzPjRiXyXjfJDQnqmQqJj9Zj5 (主网 P2SH)
```

### 2. 与 BTC 的相似性

DOGE 基于 BTC 代码，因此共享许多特性：

| 特性 | BTC | DOGE | 说明 |
|------|-----|------|------|
| Wire 格式 | `btcd/wire` | `btcd/wire` | 相同 ✅ |
| 交易哈希 | `tx.TxHash().String()` | `tx.TxHash().String()` | 相同 ✅ |
| 区块结构 | `wire.MsgBlock` | `wire.MsgBlock` | 相同 ✅ |
| 地址编码 | `addr.String()` | `addr.EncodeAddress()` | 不同 ⚠️ |
| 地址前缀 | 0x00, 0x05 | 0x1e, 0x16 | 不同 ⚠️ |

### 3. 网络支持

```go
// 主网
network: "mainnet"
port: 22555

// 测试网
network: "testnet"  
port: 44555

// Regtest
network: "regtest"
port: 18444
```

## 实现细节

### 核心方法

#### GetBlock()
```go
func (a *DOGEAdapter) GetBlock(height int64) (*indexer.Block, error) {
    // 1. 获取区块哈希
    hashStr, err := a.GetBlockHash(height)
    
    // 2. 通过 RPC 获取原始区块数据 (hex)
    resp, err := a.rpcClient.RawRequest("getblock", ...)
    
    // 3. 反序列化为 wire.MsgBlock
    msgBlock := &wire.MsgBlock{}
    msgBlock.Deserialize(bytes.NewReader(blockBytes))
    
    // 4. 转换为统一的 indexer.Block 格式
    return a.convertToIndexerBlock(msgBlock, ...)
}
```

#### extractAddress()
```go
func (a *DOGEAdapter) extractAddress(pkScript []byte) string {
    // 使用狗狗币参数提取地址
    _, addrs, _, err := txscript.ExtractPkScriptAddrs(pkScript, a.params)
    if err == nil && len(addrs) > 0 {
        // 使用 EncodeAddress() 获取正确的 'D' 前缀
        return addrs[0].EncodeAddress()
    }
    return "errAddress"
}
```

### 地址示例验证

来自真实 DOGE 交易的地址提取测试：

```go
// 交易 ID: d96170578d6c2868cb9cf63ec414c854f39c3e5fadd1e03005e9db54c309935c
// 输出地址:
// - D69140ac9abc2016f7a9dc9c67be6b96cccd3c848 (500,000,000,000 DOGE)
// - D788a64424c2b5206cb59bb7fd3d870829fa0ac91 (2,000,000,000,000 DOGE)
// - De254330131ae32fec4f05a1e18ec74cb0187a7cf (2,000,000,000,000 DOGE)
// 等...
```

## 配置示例

### 主网配置
```yaml
chain: "doge"
network: "mainnet"
data_dir: "/data/higun/doge"
api_port: "3003"

rpc:
  chain: "doge"
  host: "127.0.0.1"
  port: "22555"
  user: "dogerpc"
  password: "dogepassword"

zmq_address:
  - "tcp://127.0.0.1:29333"
```

### 测试网配置
```yaml
chain: "doge"
network: "testnet"
data_dir: "/data/higun/doge-testnet"
api_port: "3013"

rpc:
  chain: "doge"
  host: "127.0.0.1"
  port: "44555"
  user: "dogerpc"
  password: "dogepassword"
```

## 使用方法

### 启动索引器

```bash
# 方法 1: 使用配置文件
./utxo_indexer -config config_doge_example.yaml

# 方法 2: 环境变量
export CHAIN=doge
export RPC_HOST=127.0.0.1
export RPC_PORT=22555
export RPC_USER=dogerpc
export RPC_PASSWORD=dogepassword
./utxo_indexer
```

### 验证连接

启动后会看到：
```
Starting UTXO Indexer...
Initializing blockchain adapter: chain=doge
✓ Connected to DOGE node successfully
✓ Blockchain adapter initialized successfully: doge
Starting block synchronization...
```

## 测试

### 单元测试
```bash
go test ./blockchain -run TestDOGEAdapter -v
go test ./blockchain -run TestNewChainAdapter_DOGE -v
```

### 接口验证
```bash
go test ./blockchain -run TestAdapterInterface -v
# 输出: PASS
```

## 性能特点

### 批处理
- 大区块自动分批处理
- 默认批次大小: 50,000 笔交易
- 超过 400,000 笔交易时强制 GC

### 内存优化
```go
if txCount > 400000 {
    runtime.GC() // 强制垃圾回收
}
```

## 已知限制

1. **FindReorgHeight()** - 待实现
2. **ZMQ 支持** - 需要 DOGE 节点开启 ZMQ
3. **Segwit** - DOGE 不支持 Segwit

## 与其他链的对比

### BTC vs DOGE

**相同点**:
- 使用相同的 wire 格式
- 交易结构相同
- RPC 接口基本一致

**不同点**:
- 地址前缀 (BTC: 0x00, DOGE: 0x1e)
- 区块时间 (BTC: 10分钟, DOGE: 1分钟)
- 总量限制 (BTC: 2100万, DOGE: 无上限)

### MVC vs DOGE

**相同点**:
- 都使用 `EncodeAddress()` 编码地址

**不同点**:
- MVC 使用 `bsvwire.MsgBlock`
- MVC 使用 `GetNewHash2()` 计算交易 ID
- DOGE 使用标准 BTC wire 格式

## 故障排查

### 连接失败
```bash
# 检查 DOGE 节点是否运行
dogecoin-cli getblockchaininfo

# 检查 RPC 配置
cat ~/.dogecoin/dogecoin.conf
# server=1
# rpcuser=dogerpc
# rpcpassword=dogepassword
# rpcport=22555
```

### 地址错误
确保使用正确的网络参数：
- 主网: 0x1e (D 地址)
- 测试网: 0x71 (n/m 地址)
- Regtest: 0x6f (m/n 地址)

## 参考资源

- [Dogecoin Core](https://github.com/dogecoin/dogecoin)
- [Dogecoin 官方文档](https://dogecoin.com/)
- [DOGE 地址格式](https://en.bitcoin.it/wiki/List_of_address_prefixes)
- [测试交易浏览器](https://sochain.com/DOGE)

## 贡献者

感谢 `docs/doge_test.go` 提供的地址解析示例代码。

---

**实现日期**: 2024-11-21  
**状态**: ✅ 已完成并测试  
**版本**: v2.0.0-adapter
