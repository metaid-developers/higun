# Dogecoin 适配器实现细节

本文档详细记录了 `utxo-indexer` 中 Dogecoin (DOGE) 适配器的特殊处理逻辑。由于 Dogecoin 基于早期的 Bitcoin 代码库并引入了 AuxPoW (辅助工作量证明)，因此在区块解析和交易处理上与标准的 Bitcoin 有显著差异。

## 1. AuxPoW (Auxiliary Proof of Work) 处理

Dogecoin 使用 AuxPoW 允许矿工在挖掘 Litecoin 等其他 Scrypt 币的同时挖掘 Dogecoin。这导致 Dogecoin 的区块结构中包含额外的 AuxPoW 数据。

### 问题描述

标准的 Bitcoin 区块结构是：
`[Block Header] + [Tx Count] + [Transactions]`

而 Dogecoin 的 AuxPoW 区块结构是：
`[Block Header] + [AuxPoW Data (Optional)] + [Tx Count] + [Transactions]`

如果直接使用标准的 `msgBlock.Deserialize()` 方法，解析器会尝试紧接着区块头读取交易数量。如果存在 AuxPoW 数据，解析器会将 AuxPoW 数据的一部分误认为是交易数量，导致解析失败（通常报错 "unexpected EOF" 或内存分配错误）。

### 解决方案

我们需要手动解析区块，并在读取交易列表之前检查并跳过 AuxPoW 数据。

#### 检测 AuxPoW

AuxPoW 的存在通过区块头版本号（Version）的高位来标识。通常使用 `(1 << 8)` 即 `0x100` 作为标志位。

```go
// 检查是否是 AuxPoW
// Dogecoin AuxPoW 版本位通常是 (1 << 8) = 256
isAuxPow := (msgBlock.Header.Version & (1 << 8)) != 0
```

#### 跳过 AuxPoW 数据

如果检测到 AuxPoW 标志，我们需要读取并丢弃 AuxPoW 数据，以便指针移动到正确的位置（交易数量）。AuxPoW 数据结构如下：

1.  **Coinbase Transaction**: 父块的 Coinbase 交易。
2.  **Block Hash**: 父块哈希。
3.  **Merkle Branch**: Merkle 分支路径。
4.  **Index**: 索引。
5.  **Chain Merkle Branch**: 链 Merkle 分支。
6.  **Chain Index**: 链索引。
7.  **Parent Block Header**: 父块头 (80 字节)。

我们在 `blockchain/adapter_doge.go` 中实现了 `readDogeAuxPow` 函数来处理这部分数据。

```go
// readDogeAuxPow 读取并跳过 AuxPoW 数据
func readDogeAuxPow(r io.Reader) error {
    // 1. CTransaction tx;
    msgTx := &wire.MsgTx{}
    if err := msgTx.Deserialize(r); err != nil {
        return fmt.Errorf("failed to read auxpow tx: %v", err)
    }

    // ... 读取其他字段 ...
    
    // 7. CBlockHeader parentBlockHeader;
    parentHeader := make([]byte, 80)
    if _, err := io.ReadFull(r, parentHeader); err != nil {
        return fmt.Errorf("failed to read parentBlockHeader: %v", err)
    }

    return nil
}
```

## 2. 非 SegWit (隔离见证) 支持

Dogecoin 目前不支持 SegWit (隔离见证)。

### 交易解析

在解析交易时，必须使用 `DeserializeNoWitness` 方法，或者使用 `BtcDecode` 并指定不支持 Witness 的协议版本。如果使用默认的 `Deserialize`，解析器可能会尝试读取不存在的 Witness 数据，或者将其他数据误读为 Witness 数据。

```go
tx := &wire.MsgTx{}
// 使用 DeserializeNoWitness 而不是 Deserialize
if err := tx.DeserializeNoWitness(reader); err != nil {
    return nil, fmt.Errorf("failed to deserialize tx %d: %w", i, err)
}
```

## 3. 完整的区块解析流程

在 `DOGEAdapter.GetBlock` 方法中，我们采用了以下自定义解析流程：

1.  **获取原始区块数据**: 通过 RPC `getblock` 获取十六进制数据。
2.  **解析区块头**: 使用 `msgBlock.Header.Deserialize(reader)`。
3.  **处理 AuxPoW**:
    *   检查 `msgBlock.Header.Version`。
    *   如果包含 AuxPoW 标志，调用 `readDogeAuxPow(reader)` 跳过数据。
4.  **读取交易数量**: 使用 `wire.ReadVarInt(reader, 0)`。
5.  **解析交易列表**:
    *   循环 `txCount` 次。
    *   对每个交易使用 `tx.DeserializeNoWitness(reader)`。

## 4. 网络参数配置

Dogecoin 的网络参数与 Bitcoin 不同，我们在 `blockchain/adapter_doge.go` 中定义了 `DogeMainNetParams` 等配置。

关键差异包括：
*   **PubKeyHashAddrID**: `0x1e` (地址以 'D' 开头)
*   **ScriptHashAddrID**: `0x16` (地址以 '9' 或 'A' 开头)
*   **PrivateKeyID**: `0x9e`
*   **Bech32HRPSegwit**: "dc" (虽然不支持 SegWit，但保留了定义)

```go
var DogeMainNetParams = chaincfg.Params{
    Name:             "dogecoin-mainnet",
    Net:              wire.MainNet,
    PubKeyHashAddrID: 0x1e, // 'D' addresses
    ScriptHashAddrID: 0x16, // '9' or 'A' addresses
    PrivateKeyID:     0x9e, // WIF private keys
    // ...
}
```

## 总结

实现 Dogecoin 适配器的核心在于正确处理 AuxPoW 数据结构和禁用 SegWit 解析逻辑。通过手动控制解析流程，我们能够兼容 Dogecoin 的特殊区块格式。
