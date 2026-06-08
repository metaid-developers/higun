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
