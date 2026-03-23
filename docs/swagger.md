# HiGun API 文档

## 概述

HiGun 是一个基于 MetaID 的 UTXO 索引器，支持 FT（同质化代币）和 NFT（非同质化代币）的链上数据查询。

## 基础信息

- **Base URL**: `http://localhost:8080` (默认端口)
- **数据格式**: JSON
- **响应包装**: 所有响应都会包裹在统一格式中

## 通用响应格式

### 成功响应
```json
{
  "success": true,
  "code": 200,
  "data": { ... },
  "time": 1700000000000,
  "cost": 10
}
```

### 错误响应
```json
{
  "success": false,
  "code": 400,
  "error": "error message",
  "time": 1700000000000,
  "cost": 5
}
```

---

# FT 接口

## 余额查询

### 获取 FT 余额

**接口**: `GET /ft/balance`

查询指定地址的 FT 余额

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | FT 代码哈希 |
| genesis | string | 否 | FT 创世 ID |

**示例请求**:
```
GET /ft/balance?address=1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa
```

**响应示例**:
```json
{
  "success": true,
  "code": 200,
  "data": {
    "balances": [
      {
        "codeHash": "aabbccdd...",
        "genesis": "11223344...",
        "balance": "1000",
        "satoshi": "100000000"
      }
    ]
  },
  "time": 1700000000000,
  "cost": 15
}
```

---

### 获取 FT UTXO 列表

**接口**: `GET /ft/utxos`

查询指定地址的 FT UTXO 列表

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | FT 代码哈希 |
| genesis | string | 否 | FT 创世 ID |

**示例请求**:
```
GET /ft/utxos?address=1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa&genesis=11223344
```

---

### 获取唯一 FT UTXO

**接口**: `GET /ft/unique/utxos`

查询指定地址的唯一 FT UTXO（用于同质化代币追踪）

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | FT 代码哈希 |
| genesis | string | 否 | FT 创世 ID |

---

### 获取 FT 摘要

**接口**: `GET /ft/summary`

获取 FT 整体概览信息

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 否 | FT 代码哈希 |
| genesis | string | 否 | FT 创世 ID |

---

### 获取 FT Genesis 信息

**接口**: `GET /ft/genesis`

获取 FT 创世信息

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 否 | FT 代码哈希 |
| genesis | string | 否 | FT 创世 ID |

**响应示例**:
```json
{
  "success": true,
  "code": 200,
  "data": {
    "codeHash": "aabbccdd...",
    "genesis": "11223344...",
    "name": "MyToken",
    "symbol": "MTK",
    "decimal": 8,
    "totalSupply": "1000000000"
  },
  "time": 1700000000000,
  "cost": 10
}
```

---

### 获取 FT 供应量

**接口**: `GET /ft/supply`

查询 FT 总供应量

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 是 | FT 代码哈希 |
| genesis | string | 是 | FT 创世 ID |

---

### 获取 FT 持有者列表

**接口**: `GET /ft/owners`

获取 FT 的所有持有者

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 是 | FT 代码哈希 |
| genesis | string | 是 | FT 创世 ID |

---

### 获取地址 FT 历史

**接口**: `GET /ft/address/history`

获取指定地址的 FT 交易历史

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | FT 代码哈希 |
| genesis | string | 否 | FT 创世 ID |

---

### 获取 Genesis FT 历史

**接口**: `GET /ft/genesis/history`

获取指定 FT 的交易历史

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 是 | FT 代码哈希 |
| genesis | string | 是 | FT 创世 ID |

---

# Mempool 接口

### 获取 Mempool FT UTXO

**接口**: `GET /ft/mempool/utxos`

获取内存池中待确认的 FT UTXO

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | FT 代码哈希 |
| genesis | string | 否 | FT 创世 ID |

---

### 启动 Mempool 监控

**接口**: `GET /ft/mempool/start`

启动 ZMQ 和 Mempool 监控

**参数**: 无

**响应示例**:
```json
{
  "success": true,
  "message": "Mempool started successfully",
  "status": "running"
}
```

---

### 重建 Mempool

**接口**: `GET /ft/mempool/rebuild`

重建 Mempool 数据

**参数**: 无

---

### 重新索引区块

**接口**: `GET /ft/blocks/reindex`

重新索引指定范围的区块

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| start | integer | 是 | 起始区块高度 |
| end | integer | 是 | 结束区块高度 |

**示例请求**:
```
GET /ft/blocks/reindex?start=100&end=200
```

---

# NFT 接口

### 获取地址 NFT UTXO

**接口**: `GET /nft/address/utxos`

获取指定地址的 NFT UTXO 列表

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | NFT 代码哈希 |
| genesis | string | 否 | NFT 创世 ID |
| cursor | integer | 否 | 分页游标（默认: 0） |
| size | integer | 否 | 每页数量（默认: 10） |

**示例请求**:
```
GET /nft/address/utxos?address=1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa&cursor=0&size=10
```

**响应示例**:
```json
{
  "success": true,
  "code": 200,
  "data": {
    "address": "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
    "utxos": [
      {
        "codeHash": "aabbccdd...",
        "genesis": "11223344...",
        "tokenId": "1",
        "txid": "abc123...",
        "index": "0",
        "metaId": "meta123..."
      }
    ],
    "total": 100,
    "cursor": 0,
    "nextCursor": 10,
    "size": 10
  },
  "time": 1700000000000,
  "cost": 20
}
```

---

### 获取 Genesis NFT UTXO

**接口**: `GET /nft/genesis/utxos`

获取指定 NFT Genesis 的 UTXO 列表

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 否 | NFT 代码哈希 |
| genesis | string | 否 | NFT 创世 ID |
| cursor | integer | 否 | 分页游标（默认: 0） |
| size | integer | 否 | 每页数量（默认: 10） |

---

### 获取地址出售中 NFT UTXO

**接口**: `GET /nft/address/sell-utxos`

获取指定地址正在出售的 NFT UTXO

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | NFT 代码哈希 |
| genesis | string | 否 | NFT 创世 ID |
| cursor | integer | 否 | 分页游标 |
| size | integer | 否 | 每页数量 |

---

### 获取 Genesis 出售中 NFT UTXO

**接口**: `GET /nft/genesis/sell-utxos`

获取指定 NFT Genesis 中正在出售的 UTXO

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 否 | NFT 代码哈希 |
| genesis | string | 否 | NFT 创世 ID |
| cursor | integer | 否 | 分页游标 |
| size | integer | 否 | 每页数量 |

---

### 获取地址 NFT UTXO 数量

**接口**: `GET /nft/address/utxo-count`

获取指定地址的 NFT UTXO 数量

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |
| codeHash | string | 否 | NFT 代码哈希 |
| genesis | string | 否 | NFT 创世 ID |

---

### 获取地址 NFT 摘要

**接口**: `GET /nft/address/summary`

获取指定地址的 NFT 持有摘要

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| address | string | 是 | 钱包地址 |

---

### 获取 NFT 摘要

**接口**: `GET /nft/summary`

获取 NFT 整体概览

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 否 | NFT 代码哈希 |
| genesis | string | 否 | NFT 创世 ID |

---

### 获取 NFT Genesis 信息

**接口**: `GET /nft/genesis`

获取 NFT Genesis 详情

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 否 | NFT 代码哈希 |
| genesis | string | 否 | NFT 创世 ID |

**响应示例**:
```json
{
  "success": true,
  "code": 200,
  "data": {
    "codeHash": "aabbccdd...",
    "genesis": "11223344...",
    "name": "MyNFT",
    "symbol": "MNFT",
    "totalSupply": "1000"
  },
  "time": 1700000000000,
  "cost": 12
}
```

---

### 获取 NFT 持有者列表

**接口**: `GET /nft/owners`

获取 NFT 的所有持有者

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| codeHash | string | 是 | NFT 代码哈希 |
| genesis | string | 是 | NFT 创世 ID |

---

# NFT Mempool 接口

### 启动 NFT Mempool 监控

**接口**: `GET /nft/mempool/start`

启动 NFT Mempool 监控

**参数**: 无

---

### 重建 NFT Mempool

**接口**: `GET /nft/mempool/rebuild`

重建 NFT Mempool 数据

**参数**: 无

---

### 重新索引 NFT 区块

**接口**: `GET /nft/blocks/reindex`

重新索引指定范围的 NFT 区块

**参数**:

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| start | integer | 是 | 起始区块高度 |
| end | integer | 是 | 结束区块高度 |

---

# 内部数据库查询接口

以下接口为内部数据库查询接口，用于调试和数据验证。

## FT 数据库查询

| 接口 | 说明 | 参数 |
|------|------|------|
| `GET /db/ft/utxo` | 根据交易ID查询 FT UTXO | tx |
| `GET /db/ft/income` | 根据地址查询 FT 收入 | address, codeHash, genesis |
| `GET /db/ft/income/valid` | 查询有效 FT 收入 | address, codeHash, genesis |
| `GET /db/ft/spend` | 根据地址查询 FT 支出 | address, codeHash, genesis |
| `GET /db/ft/unique/income` | 查询唯一 FT 收入 | codeHash, genesis |
| `GET /db/ft/unique/spend` | 查询唯一 FT 支出 | codeHash, genesis |
| `GET /db/ft/all/income` | 查询所有 FT 收入 | page, page_size |
| `GET /db/ft/all/spend` | 查询所有 FT 支出 | page, page_size |
| `GET /db/ft/address/income` | 查询地址 FT 收入详情 | address, codeHash, genesis |
| `GET /db/ft/address/spend` | 查询地址 FT 支出详情 | address, codeHash, genesis |
| `GET /db/ft/info` | 查询 FT 信息 | codeHash, genesis |
| `GET /db/ft/genesis` | 查询所有 FT Genesis | - |
| `GET /db/ft/genesis/output` | 查询 FT Genesis Output | - |
| `GET /db/ft/genesis/utxo` | 查询 FT Genesis UTXO | - |
| `GET /db/ft/used/income` | 查询已使用的 FT 收入 | - |
| `GET /db/ft/uncheck/outpoint` | 查询未确认 FT Outpoint | - |
| `GET /db/ft/supply/list` | 查询 FT 供应列表 | - |
| `GET /db/ft/burn/list` | 查询 FT 销毁列表 | - |
| `GET /db/ft/invalid/outpoint` | 查询无效 FT Outpoint | outpoint |

## NFT 数据库查询

| 接口 | 说明 | 参数 |
|------|------|------|
| `GET /db/nft/utxo` | 根据交易ID查询 NFT UTXO | tx |
| `GET /db/nft/utxo/all` | 查询所有 NFT UTXO | page, page_size |
| `GET /db/nft/address/income` | 查询地址 NFT 收入 | address, codeHash, genesis |
| `GET /db/nft/address/income/valid` | 查询有效 NFT 收入 | address, codeHash, genesis |
| `GET /db/nft/address/spend` | 查询地址 NFT 支出 | address, codeHash, genesis |
| `GET /db/nft/codehash-genesis/income` | 查询 NFT CodeHash-Genesis 收入 | codeHash, genesis |
| `GET /db/nft/codehash-genesis/spend` | 查询 NFT CodeHash-Genesis 支出 | codeHash, genesis |
| `GET /db/nft/info` | 查询 NFT 信息 | - |
| `GET /db/nft/genesis` | 查询所有 NFT Genesis | - |
| `GET /db/nft/genesis/output` | 查询 NFT Genesis Output | - |
| `GET /db/nft/uncheck/outpoint` | 查询未确认 NFT Outpoint | - |
| `GET /db/nft/used/income` | 查询已使用的 NFT 收入 | - |
| `GET /db/nft/invalid/outpoint` | 查询无效 NFT Outpoint | outpoint |

## Mempool 数据库查询

### FT Mempool 查询

| 接口 | 说明 | 参数 |
|------|------|------|
| `GET /db/ft/mempool/verify/tx` | 查询 Mempool 验证交易 | txId, page, page_size |
| `GET /db/ft/mempool/uncheck/utxo` | 查询 Mempool 未确认 UTXO | outpoint, page, page_size |
| `GET /db/ft/mempool/spend` | 查询 Mempool 地址 FT 支出 | - |
| `GET /db/ft/mempool/unique/spend` | 查询 Mempool 唯一 FT 支出 | - |
| `GET /db/ft/mempool/address/income` | 查询 Mempool 地址 FT 收入 | - |
| `GET /db/ft/mempool/address/income/valid` | 查询 Mempool 有效地址 FT 收入 | - |

### NFT Mempool 查询

| 接口 | 说明 | 参数 |
|------|------|------|
| `GET /db/nft/mempool/spend` | 查询 Mempool NFT 支出 | - |
| `GET /db/nft/mempool/address/income` | 查询 Mempool 地址 NFT 收入 | - |
| `GET /db/nft/mempool/address/income/valid` | 查询 Mempool 有效地址 NFT 收入 | - |

---

# 错误码说明

| 错误码 | 说明 |
|--------|------|
| 200 | 成功 |
| 400 | 请求参数错误 |
| 500 | 服务器内部错误 |

---

# 注意事项

1. 所有时间戳为毫秒级 Unix 时间戳
2. `cost` 字段表示请求处理耗时（毫秒）
3. 分页查询时，`nextCursor` 为 -1 表示没有更多数据
4. 内部数据库接口 (`/db/*`) 主要用于调试，请谨慎使用
