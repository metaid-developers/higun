# CheckUtxo API 文档

## 接口说明

检测UTXO是否被花费的接口，可以批量查询UTXO的状态信息。

## 请求方式

```
POST /utxo/check
```

## 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| outPoints | []string | 是 | UTXO列表，格式为 "txhash:index" |

### 请求示例

```json
{
  "outPoints": [
    "abc123def456:0",
    "abc123def456:1",
    "xyz789abc012:0"
  ]
}
```

## 响应参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| code | int | 响应码，2000表示成功 |
| msg | string | 响应消息 |
| data | map[string]UtxoInfo | UTXO信息映射，key为outPoint |

### UtxoInfo 结构

| 字段名 | 类型 | 说明 |
|--------|------|------|
| isExist | bool | UTXO是否存在 |
| height | int | 区块高度（未实现） |
| date | int64 | 时间戳（秒） |
| value | int64 | 金额（satoshi） |
| txConfirm | bool | 交易是否确认 |
| where | string | UTXO位置：block（已确认）或 mempool（未确认） |
| address | string | 地址 |
| spendStatus | string | 花费状态：unspent（未花费）或 spend（已花费） |
| spendInfo | UtxoSpendInfo | 花费信息 |

### UtxoSpendInfo 结构

| 字段名 | 类型 | 说明 |
|--------|------|------|
| spendTx | string | 花费交易的hash |
| height | int | 花费区块高度（未实现） |
| date | int64 | 花费时间戳（秒） |
| where | string | 花费位置：block（已确认）或 mempool（未确认） |
| address | string | 地址 |

### 响应示例

#### 成功响应

```json
{
  "code": 2000,
  "msg": "ok",
  "data": {
    "abc123def456:0": {
      "isExist": true,
      "height": 0,
      "date": 1704614400,
      "value": 100000,
      "txConfirm": true,
      "where": "block",
      "address": "bc1q...",
      "spendStatus": "unspent",
      "spendInfo": {
        "spendTx": "",
        "height": 0,
        "date": 0,
        "where": "",
        "address": ""
      }
    },
    "abc123def456:1": {
      "isExist": true,
      "height": 0,
      "date": 1704614400,
      "value": 50000,
      "txConfirm": true,
      "where": "block",
      "address": "bc1q...",
      "spendStatus": "spend",
      "spendInfo": {
        "spendTx": "xyz789abc012...",
        "height": 0,
        "date": 1704700800,
        "where": "block",
        "address": "bc1q..."
      }
    },
    "xyz789abc012:0": {
      "isExist": true,
      "height": 0,
      "date": 1704787200,
      "value": 200000,
      "txConfirm": false,
      "where": "mempool",
      "address": "bc1q...",
      "spendStatus": "unspent",
      "spendInfo": {
        "spendTx": "",
        "height": 0,
        "date": 0,
        "where": "",
        "address": ""
      }
    }
  }
}
```

#### 错误响应

```json
{
  "code": -2001,
  "msg": "request parameter error"
}
```

## 错误码说明

| 错误码 | 说明 |
|--------|------|
| 2000 | 成功 |
| -2001 | 请求参数错误 |

## 使用示例

### cURL

```bash
curl -X POST http://localhost:8080/utxo/check \
  -H "Content-Type: application/json" \
  -d '{
    "outPoints": [
      "abc123def456:0",
      "abc123def456:1"
    ]
  }'
```

### JavaScript (fetch)

```javascript
fetch('http://localhost:8080/utxo/check', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    outPoints: [
      'abc123def456:0',
      'abc123def456:1'
    ]
  })
})
  .then(response => response.json())
  .then(data => console.log(data))
  .catch(error => console.error('Error:', error));
```

### Python (requests)

```python
import requests

url = 'http://localhost:8080/utxo/check'
data = {
    'outPoints': [
        'abc123def456:0',
        'abc123def456:1'
    ]
}

response = requests.post(url, json=data)
print(response.json())
```

## 注意事项

1. outPoints格式必须为 "txhash:index"，其中index为数字
2. 接口会同时查询区块链数据和mempool数据
3. 如果UTXO不存在（isExist为false），其他字段可能为空值
4. spendStatus为"spend"时，spendInfo包含花费详情
5. where字段区分UTXO是在区块链（block）还是在内存池（mempool）中

## 性能说明

- 支持批量查询，建议单次请求不超过100个UTXO
- 查询涉及数据库访问和内存池检索，响应时间取决于数据量
- 对于大量UTXO查询，建议分批请求
