# Wallet Gateway Smoke Checks

Use these checks before publishing a Wallet Gateway configuration.

## Prerequisites

- `wallet.enabled: true` must be set.
- At least one `wallet.chains.<chain>.enabled: true` entry must have a valid absolute `core_url`, or the matching `WALLET_<CHAIN>_CORE_URL` environment variable must be set.
- The local gateway URL examples assume API port `3001`; adjust them if `api_port` differs.
- The `BTC_CORE`, `MVC_CORE`, and `DOGE_CORE` variables below must match the enabled gateway config.

## Upstream Core Health

Set the core URLs used by this gateway:

```bash
BTC_CORE='http://127.0.0.1:8066'
MVC_CORE='http://127.0.0.1:8085'
DOGE_CORE='http://127.0.0.1:<doge-core-port>'
```

Replace `DOGE_CORE` with the DOGE Higun core URL from the enabled gateway config.

Check each configured core before checking the gateway:

```bash
curl -sS "$BTC_CORE/cleanedHeight/get"
curl -sS "$BTC_CORE/balance?address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ"
curl -sS "$BTC_CORE/utxos?address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ"

curl -sS "$MVC_CORE/cleanedHeight/get"
curl -sS "$MVC_CORE/balance?address=1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK"
curl -sS "$MVC_CORE/utxos?address=1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK"

curl -sS "$DOGE_CORE/cleanedHeight/get"
curl -sS "$DOGE_CORE/balance?address=DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L"
curl -sS "$DOGE_CORE/utxos?address=DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L"
```

The Wallet Gateway first version expects each configured core `/utxos` response to be the merged confirmed plus mempool UTXO source. If a core only exposes mempool data through `/mempool/utxos`, fix or wrap that core before enabling the gateway for that chain.

## Local Standard Responses

BTC balance:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/balance'
```

BTC UTXOs, default includes mempool:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos'
```

BTC UTXOs, confirmed only:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos?confirmedOnly=true'
```

MVC balance:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/balance'
```

MVC UTXOs, default includes mempool:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/utxos'
```

MVC UTXOs, confirmed only:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/utxos?confirmedOnly=true'
```

DOGE balance:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/balance'
```

DOGE UTXOs, default includes mempool:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/utxos'
```

DOGE UTXOs, confirmed only:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/utxos?confirmedOnly=true'
```

For smoke validation, verify HTTP status `200` and response `.code == 0` for gateway responses. Concise patterns are:

```bash
curl -sS -w '\nHTTP %{http_code}\n' 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/balance'
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/balance' | jq '.code, .message'
```

## Metalet Compatibility Responses

BTC balance compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/balance?format=metalet'
```

BTC UTXO compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos?format=metalet'
curl -sS 'http://127.0.0.1:3001/wallet/v1/btc/address/12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ/utxos?format=metalet&confirmedOnly=true'
```

MVC balance compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/balance?format=metalet'
```

MVC UTXO compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/utxos?format=metalet'
curl -sS 'http://127.0.0.1:3001/wallet/v1/mvc/address/1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK/utxos?format=metalet&confirmedOnly=true'
```

DOGE balance compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/balance?format=metalet'
```

DOGE UTXO compatibility:

```bash
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/utxos?format=metalet'
curl -sS 'http://127.0.0.1:3001/wallet/v1/doge/address/DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L/utxos?format=metalet&confirmedOnly=true'
```

## Comparison Targets

Compare sampled values against existing services before publishing the gateway:

```bash
curl -sS 'https://www.metalet.space/wallet-api/v3/address/btc-balance?net=livenet&address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ'
curl -sS 'http://8.217.251.101:8066/balance?address=12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ'
curl -sS 'https://www.metalet.space/wallet-api/v4/mvc/address/balance-info?net=livenet&address=1D4qt4KHvCbEQLGZvQSsPbsuHKAKnUxLjK'
curl -sS 'https://www.metalet.space/wallet-api/v4/doge/address/balance-info?net=livenet&address=DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L'
```

Expected checks:

- standard responses use satoshi integer fields and decimal strings;
- standard responses do not expose float amount fields;
- `format=metalet` responses preserve old balance field names;
- UTXO default responses include mempool entries when the configured core `/utxos` reports them;
- `confirmedOnly=true` excludes mempool entries;
- existing Metalet endpoints are not changed by enabling Wallet Gateway.

## Log Checks

While running the curl commands, check the active Higun process log source for wallet upstream lines. Replace `higun.log` with the active log source, such as `journalctl -u <service>` or `docker logs <container>`.

```bash
rg 'wallet upstream request' higun.log
journalctl -u <service> | rg 'wallet upstream request'
docker logs <container> 2>&1 | rg 'wallet upstream request'
```

Expected log properties:

- includes upstream `host`, `path`, HTTP `status`, and `duration_ms`;
- logs a truncated address such as `12ghVW...nMUikZ`;
- does not log a full wallet address;
- failed upstream calls include an `error` field.

## Wallet Gateway v1.1 Smoke Checks

Set these variables before running checks:

```bash
BASE_URL="http://127.0.0.1:3001"
CHAIN="btc"
ADDRESS="replace-with-funded-address"
TXID="replace-with-known-transaction-id"
```

Fee-rate check:

```bash
curl -sS "$BASE_URL/wallet/v1/$CHAIN/fee-rate" | jq -e '
  .code == 0 and
  .data.source == "config" and
  .data.unit == "sat_per_byte" and
  (.data.slow > 0) and
  (.data.normal > 0) and
  (.data.fast > 0)
'
```

Expected:

- `code` is `0`;
- `data.source` is `config`;
- `data.unit` is `sat_per_byte`;
- `data.slow`, `data.normal`, and `data.fast` are positive integers.

History checks:

```bash
curl -sS "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/history?page=1&limit=20" | jq -e '
  .code == 0 and
  .data.page == 1 and
  .data.limit == 20 and
  (.data.items | type == "array") and
  all(.data.items[]?; has("mempool") and has("timestamp") and has("net") and has("netSatoshi"))
'

curl -sS "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/history?page=1&limit=20&confirmedOnly=true" | jq -e '
  .code == 0 and
  .data.confirmedOnly == true and
  all(.data.items[]?; .mempool != true)
'

curl -sS "$BASE_URL/wallet/v1/$CHAIN/address/$ADDRESS/history?page=1&limit=20" | jq -r '
  .data.items[]? | [.txid, .incomeSatoshi, .spendSatoshi, .netSatoshi, .net, .timestamp, .mempool] | @tsv
'
```

Expected:

- default history may include mempool items;
- `confirmedOnly=true` excludes items where `mempool` is `true`;
- `timestamp` is numeric when the core has timestamp data;
- `netSatoshi` and `net` represent the exact signed delta from `incomeSatoshi - spendSatoshi`.

Transaction detail check:

```bash
curl -sS "$BASE_URL/wallet/v1/$CHAIN/tx/$TXID" | jq -e --arg txid "$TXID" '
  .code == 0 and
  .data.txid == $txid and
  (.data | has("confirmed") and has("mempool") and has("confirmations")) and
  (if .data.confirmed then .data.confirmations >= 1 elif .data.mempool then .data.confirmations == 0 else true end)
'
```

Expected:

- `code` is `0`;
- `data.txid` equals `$TXID`;
- `data.confirmed`, `data.mempool`, and `data.confirmations` are present;
- confirmed transactions have `confirmations >= 1`;
- mempool transactions have `confirmations = 0`.

Broadcast accepted check:

```bash
SIGNED_RAW_TX="replace-with-signed-raw-transaction-hex"

curl -sS -X POST "$BASE_URL/wallet/v1/$CHAIN/tx/broadcast" \
  -H 'Content-Type: application/json' \
  -d "{\"rawTx\":\"$SIGNED_RAW_TX\"}" | jq -e '
    .code == 0 and
    .data.accepted == true and
    (.data.txid | type == "string" and length > 0)
  '
```

Expected on accepted transaction:

- `code` is `0`;
- `data.accepted` is `true`;
- `data.txid` is present.

Broadcast rejected check:

```bash
REJECTED_RAW_TX="replace-with-valid-hex-that-core-will-reject"

curl -sS -X POST "$BASE_URL/wallet/v1/$CHAIN/tx/broadcast" \
  -H 'Content-Type: application/json' \
  -d "{\"rawTx\":\"$REJECTED_RAW_TX\"}" | jq -e '
    .code == -5004 and
    .message == "broadcast rejected"
  '
```

Expected on rejected transaction:

- `code` is `-5004`;
- `message` is `broadcast rejected`.
