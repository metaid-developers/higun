# Higun Wallet Gateway API 接入文档

本文档面向下游应用开发。新的应用可以直接对接 Higun Wallet Gateway，不需要理解或依赖 Metalet 的转发层。

对应 OpenAPI 文件：

- `docs/wallet-gateway-openapi.yaml`

## 基础信息

Base URL 由部署环境决定，例如：

```text
http://127.0.0.1:3001
https://higun.example.com
```

当前支持三条基础链：

```text
btc
mvc
doge
```

所有标准接口都使用统一响应包：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

标准成功码是 `0`。错误时 `data` 为 `null`，例如：

```json
{
  "code": -5001,
  "message": "core unavailable",
  "data": null
}
```

## 接口列表

```text
GET  /wallet/v1/{chain}/address/{address}/balance
GET  /wallet/v1/{chain}/address/{address}/utxos
GET  /wallet/v1/{chain}/fee-rate
POST /wallet/v1/{chain}/tx/broadcast
GET  /wallet/v1/{chain}/tx/{txid}
GET  /wallet/v1/{chain}/address/{address}/history
```

`{chain}` 必须是 `btc`、`mvc` 或 `doge`。

## 余额查询

```text
GET /wallet/v1/{chain}/address/{address}/balance
```

示例：

```bash
curl -sS "$BASE_URL/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/balance"
```

响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "address": "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ",
    "confirmedSatoshi": 100000,
    "unconfirmedSatoshi": 5000,
    "mempoolIncomeSatoshi": 5000,
    "mempoolSpendSatoshi": 0,
    "unsafeSatoshi": 0,
    "safeSatoshi": 105000,
    "utxoCount": 3,
    "confirmed": "0.00100000",
    "unconfirmed": "0.00005000",
    "mempoolIncome": "0.00005000",
    "mempoolSpend": "0",
    "unsafe": "0",
    "safe": "0.00105000"
  }
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `confirmedSatoshi` | 已确认余额，单位 satoshi |
| `unconfirmedSatoshi` | 未确认净额，等于 mempool income - mempool spend，可能为负数 |
| `mempoolIncomeSatoshi` | 内存池收入金额 |
| `mempoolSpendSatoshi` | 内存池支出金额 |
| `unsafeSatoshi` | 当前链核心认为不安全的金额 |
| `safeSatoshi` | 应用可优先使用的安全余额 |
| `utxoCount` | 地址底层 UTXO 数量统计；不等同于 `/utxos` 返回的可花费 UTXO 数量 |
| `confirmed` 等字符串字段 | 对应 satoshi 字段的 8 位小数字符串 |

## UTXO 查询

```text
GET /wallet/v1/{chain}/address/{address}/utxos
```

这个接口返回应用构造交易时可优先使用的 spendable UTXO 集合，不保证返回地址下所有 UTXO。

BTC 链会保留小额资产载体保护：`<=1000 sat` 的 UTXO 可能承载 MetaID PIN、Ordinals 等资产，默认不会作为可花费 UTXO 返回，避免应用误花用户资产载体。

查询参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `confirmedOnly` | `false` | 默认包含 mempool/unconfirmed UTXO；传 `true` 时只返回已确认 UTXO |
| `sort` | `desc` | `desc` 或 `asc`，按 UTXO 金额排序 |

示例：

```bash
curl -sS "$BASE_URL/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos"
curl -sS "$BASE_URL/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos?confirmedOnly=true"
```

响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "address": "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ",
    "confirmedOnly": false,
    "sort": "desc",
    "total": 2,
    "utxos": [
      {
        "txid": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "vout": 0,
        "outpoint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:0",
        "satoshi": 100000,
        "amount": "0.00100000",
        "confirmed": true,
        "mempool": false,
        "height": 840000
      },
      {
        "txid": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "vout": 1,
        "outpoint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb:1",
        "satoshi": 5000,
        "amount": "0.00005000",
        "confirmed": false,
        "mempool": true,
        "height": -1
      }
    ]
  }
}
```

应用侧建议：

- 默认使用不带 `confirmedOnly` 的结果，这样应用可以使用未确认 UTXO，避免用户刚转入或刚操作后应用不可用。
- 不要把 balance 里的 `utxoCount` 当成 `/utxos` 可花费列表数量；尤其 BTC 上小额资产载体 UTXO 会被保护性过滤。
- 只有在业务明确要求“必须确认后才能使用”时，才加 `confirmedOnly=true`。

## 费率查询

```text
GET /wallet/v1/{chain}/fee-rate
```

示例：

```bash
curl -sS "$BASE_URL/wallet/v1/btc/fee-rate"
```

响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "source": "config",
    "unit": "sat_per_byte",
    "slow": 1,
    "normal": 3,
    "fast": 5,
    "default": "normal"
  }
}
```

说明：

- 当前 `source` 固定为 `config`，表示来自 Higun 配置。
- 当前三条链 `unit` 都是 `sat_per_byte`。
- 下游应用不需要自己配置费率，直接读取该接口即可。
- 后续如果 Higun 改成动态估算，路径和字段名保持稳定，`source` 可能变为其他值。

## 广播交易

```text
POST /wallet/v1/{chain}/tx/broadcast
```

请求体：

```json
{
  "rawTx": "0200000001..."
}
```

示例：

```bash
curl -sS -X POST "$BASE_URL/wallet/v1/btc/tx/broadcast" \
  -H 'Content-Type: application/json' \
  -d '{"rawTx":"0200000001..."}'
```

成功响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "txid": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    "accepted": true
  }
}
```

节点拒绝交易时：

```json
{
  "code": -5004,
  "message": "broadcast rejected",
  "data": null
}
```

规则：

- `rawTx` 必须是已签名交易 hex。
- Higun 不负责签名，不修改交易，也不保存交易。
- Higun 不记录 rawTx 请求体。
- `rawTx` 最大长度为 1,000,000 字符。

## 交易详情和确认状态

```text
GET /wallet/v1/{chain}/tx/{txid}
```

示例：

```bash
curl -sS "$BASE_URL/wallet/v1/btc/tx/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
```

响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "txid": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    "confirmed": true,
    "mempool": false,
    "confirmations": 12,
    "height": 840000,
    "blockHash": "0000000000000000000000000000000000000000000000000000000000000000",
    "blockTime": 1717833600,
    "inputs": [
      {
        "txid": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "vout": 0,
        "address": "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ",
        "satoshi": 100000,
        "amount": "0.00100000"
      }
    ],
    "outputs": [
      {
        "vout": 0,
        "address": "1BoatSLRHtKNngkdXEeobR76b53LETtpyT",
        "satoshi": 90000,
        "amount": "0.00090000"
      }
    ],
    "feeSatoshi": 10000,
    "fee": "0.00010000",
    "size": 225,
    "vsize": 225
  }
}
```

确认状态规则：

| 状态 | 字段表现 |
| --- | --- |
| 已确认 | `confirmed=true`、`mempool=false`、`confirmations >= 1` |
| 未确认 | `confirmed=false`、`mempool=true`、`confirmations = 0` |
| 查不到 | `code=-4041`、`message=transaction not found` |

MVC 注意事项：

- MVC history 对外返回 public txid。
- Higun 内部会处理 MVC public txid 和节点 txid 的 alias 解析。
- 下游只需要保存和传递接口返回的 `txid`，不需要理解 alias 细节。

## 地址交易历史

```text
GET /wallet/v1/{chain}/address/{address}/history
```

查询参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `page` | `1` | 页码，小于 1 或非法值会按 1 处理 |
| `limit` | `20` | 每页数量，最大 100 |
| `confirmedOnly` | `false` | 默认包含 mempool/unconfirmed 历史 |
| `sort` | `desc` | `desc` 或 `asc` |

示例：

```bash
curl -sS "$BASE_URL/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/history?page=1&limit=20"
curl -sS "$BASE_URL/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/history?page=1&limit=20&confirmedOnly=true"
```

响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "address": "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ",
    "page": 1,
    "limit": 20,
    "confirmedOnly": false,
    "sort": "desc",
    "total": 1,
    "items": [
      {
        "txid": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
        "direction": "income",
        "incomeSatoshi": 100000,
        "spendSatoshi": 0,
        "netSatoshi": 100000,
        "income": "0.00100000",
        "spend": "0",
        "net": "0.00100000",
        "confirmed": false,
        "mempool": true,
        "confirmations": 0,
        "height": null,
        "timestamp": 1717833600,
        "time": "2026-06-08 12:00:00"
      }
    ]
  }
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `direction` | `income`、`spend` 或 `mixed` |
| `incomeSatoshi` | 该地址在该交易中的收入金额 |
| `spendSatoshi` | 该地址在该交易中的支出金额 |
| `netSatoshi` | `incomeSatoshi - spendSatoshi`，可能为负数 |
| `income`、`spend`、`net` | 对应金额的 8 位小数字符串 |
| `confirmed` | 是否已确认 |
| `mempool` | 是否是 mempool/unconfirmed 记录 |
| `confirmations` | 确认数，mempool 记录为 `0` |
| `height` | 区块高度，mempool 记录为 `null` |
| `timestamp` | Unix 秒级时间戳 |
| `time` | 展示用时间字符串 |

## 错误码

| code | HTTP | message | 说明 |
| --- | --- | --- | --- |
| `0` | `200` | `success` | 成功 |
| `-4001` | `404` | `unsupported chain` | 不支持的 `{chain}` |
| `-4002` | `400` | `address is required` | 地址为空 |
| `-4003` | `400` | `invalid query parameter` 或具体参数错误 | 查询参数非法，例如 `sort` 或 `confirmedOnly` |
| `-4004` | `400` | `invalid raw transaction` | 广播请求 rawTx 非法 |
| `-4041` | `404` | `transaction not found` | 交易不存在或当前节点/索引还查不到 |
| `-5001` | `502/503` | `core unavailable` | 对应链 core 未配置、不可用或返回非 2xx |
| `-5002` | `502` | `invalid upstream response` | core 响应格式不符合网关要求 |
| `-5003` | `500` | `internal wallet error` | 网关内部错误 |
| `-5004` | `502` | `broadcast rejected` | 节点拒绝广播交易 |
| `-5005` | `500` | `fee rate unavailable` | 费率配置运行时不可用 |

## Metalet 兼容模式

余额和 UTXO 接口支持：

```text
format=standard
format=metalet
```

默认是 `standard`。新下游应用应该使用 `standard`，不要使用 `format=metalet`。

`format=metalet` 只是给旧迁移路径兼容：

```bash
curl -sS "$BASE_URL/wallet/v1/btc/address/$ADDRESS/balance?format=metalet"
curl -sS "$BASE_URL/wallet/v1/btc/address/$ADDRESS/utxos?format=metalet"
```

新接口 `fee-rate`、`tx/broadcast`、`tx/{txid}`、`history` 不需要也不支持 Metalet 格式。

## 推荐接入流程

1. 启动时读取 `GET /wallet/v1/{chain}/fee-rate`，用于构造交易时选择费率。
2. 展示余额时调用 `GET /wallet/v1/{chain}/address/{address}/balance`。
3. 构造交易前调用 `GET /wallet/v1/{chain}/address/{address}/utxos`，默认包含未确认 UTXO，并返回可花费 UTXO 集。
4. 广播已签名交易调用 `POST /wallet/v1/{chain}/tx/broadcast`。
5. 广播后用 `GET /wallet/v1/{chain}/tx/{txid}` 查询确认状态。
6. 展示流水或交易记录时调用 `GET /wallet/v1/{chain}/address/{address}/history`。

## Smoke Check

上线前可以参考：

- `docs/wallet-gateway-smoke-checks.md`

最小验证示例：

```bash
BASE_URL="http://127.0.0.1:3001"
CHAIN="btc"
ADDRESS="replace-with-funded-address"

curl -sS "$BASE_URL/wallet/v1/$CHAIN/fee-rate" | jq .
curl -sS "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/balance" | jq .
curl -sS "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/utxos" | jq .
curl -sS "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/history?page=1&limit=20" | jq .
```
