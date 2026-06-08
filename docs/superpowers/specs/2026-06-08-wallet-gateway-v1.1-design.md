# Wallet Gateway v1.1 Design

## Problem

Wallet Gateway v1 gives applications a clean Higun entry point for BTC, MVC,
and DOGE balance and UTXO reads:

```text
GET /wallet/v1/{chain}/address/{address}/balance
GET /wallet/v1/{chain}/address/{address}/utxos
```

Those two endpoints are enough for read-only balance display, but they are not
enough for new applications to stop depending on Metalet-style wallet services.
Most wallet-facing flows also need transaction broadcast, transaction detail,
transaction confirmation state, address transaction history, and fee-rate
guidance.

The v1.1 goal is to absorb those base-chain wallet capabilities into Higun's
Wallet Gateway so new applications can integrate one public Higun wallet API
instead of routing through Metalet as an additional gateway layer.

## Goals

Wallet Gateway v1.1 must:

- keep the existing v1 balance and UTXO endpoints unchanged;
- add transaction lifecycle endpoints under the same `/wallet/v1` namespace;
- support BTC, MVC, and DOGE for every v1.1 endpoint;
- include mempool/unconfirmed transaction data by default where the data model
  supports it;
- expose fee rates as a public Higun endpoint so downstream applications do not
  configure their own values;
- source fee rates from Higun configuration in the first implementation;
- keep the fee-rate API stable so a later dynamic estimator can replace the
  config source without changing downstream clients;
- keep existing Metalet services and routes untouched for old applications.

## Non-Goals

Wallet Gateway v1.1 does not move every Metalet feature into Higun.

Out of scope:

- BRC20, runes, inscriptions, pins, MRC20, FT, NFT, or other asset aggregation;
- prices, market data, portfolio summaries, or fiat exchange rates;
- DApp list, app-base, version upgrade, or application metadata endpoints;
- account management, signing, private key custody, mnemonic storage, or wallet
  identity storage;
- dynamic fee estimation from mempool/network conditions;
- forcing old Metalet consumers to migrate immediately;
- changing the public v1 balance or UTXO response schemas.

## Endpoint Summary

Wallet Gateway v1.1 adds these endpoints:

```text
POST /wallet/v1/{chain}/tx/broadcast
GET  /wallet/v1/{chain}/tx/{txid}
GET  /wallet/v1/{chain}/address/{address}/history
GET  /wallet/v1/{chain}/fee-rate
```

Supported `{chain}` values:

- `btc`
- `mvc`
- `doge`

All standard responses use the existing Wallet Gateway envelope:

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

The v1.1 endpoints are standard-format endpoints. `format=metalet` remains a v1
compatibility feature for balance and UTXO migration; it is not required for the
new transaction and fee-rate endpoints.

## Broadcast Transaction

```text
POST /wallet/v1/{chain}/tx/broadcast
```

Request body:

```json
{
  "rawTx": "0200000001..."
}
```

Response:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "txid": "example-txid",
    "accepted": true
  }
}
```

Rules:

- `rawTx` is required and must be a non-empty hex string;
- the gateway does not sign, mutate, or store transactions;
- the response returns the transaction id accepted by the configured chain core;
- the gateway must not log the raw transaction body;
- validation failures return `-4004`;
- chain node rejection returns `-5004` with a public message of
  `broadcast rejected`;
- upstream transport or malformed upstream responses use the existing core
  error codes.

Initial core mapping:

```text
POST {core_url}/btc/broadcast
```

Current Higun core exposes the legacy path above and returns:

```json
{
  "code": 2000,
  "msg": "example-txid"
}
```

Wallet Gateway v1.1 must normalize that legacy success shape into the standard
Wallet Gateway response. The chain-specific gateway path is the public
contract; the internal core broadcast path can later move to a generic
`POST /tx/broadcast` route without changing downstream applications.

To avoid hard-coding the legacy core path forever, chain config may provide an
optional `broadcast_path`. If it is omitted, the first implementation uses
`/btc/broadcast` for compatibility with the current core route.

## Transaction Detail And Confirmation Status

```text
GET /wallet/v1/{chain}/tx/{txid}
```

Response:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "txid": "example-txid",
    "confirmed": true,
    "mempool": false,
    "confirmations": 12,
    "height": 952729,
    "blockHash": "example-block-hash",
    "blockTime": 1717833600,
    "inputs": [
      {
        "txid": "previous-txid",
        "vout": 0,
        "address": "example-address",
        "satoshi": 1000,
        "amount": "0.00001000"
      }
    ],
    "outputs": [
      {
        "vout": 0,
        "address": "example-address",
        "satoshi": 900,
        "amount": "0.00000900"
      }
    ],
    "feeSatoshi": 100,
    "fee": "0.00000100",
    "size": 225,
    "vsize": 225
  }
}
```

Required normalized fields:

- `chain`
- `txid`
- `confirmed`
- `mempool`
- `confirmations`
- `inputs`
- `outputs`

Fields that should be present when the core can determine them:

- `height`
- `blockHash`
- `blockTime`
- `feeSatoshi`
- `fee`
- `size`
- `vsize`

Confirmation rules:

- confirmed transactions return `confirmed=true`, `mempool=false`, and
  `confirmations >= 1`;
- mempool transactions return `confirmed=false`, `mempool=true`, and
  `confirmations=0`;
- if a txid cannot be found in the indexed chain data or node mempool, return
  `-4041 transaction not found`;
- if the configured core can decode the transaction but cannot determine the
  required confirmation status, return `-5002 invalid upstream response` rather
  than guessing.

Core requirement:

Current Higun adapters already have transaction decode capabilities, but the
public core API does not yet expose a complete transaction detail and
confirmation-status route. The v1.1 implementation must add or adapt a core
route, recommended as:

```text
GET {core_url}/tx/{txid}
```

That core route must provide enough data for the gateway to populate the
required normalized fields above. The gateway remains the application-facing
contract; the core route remains an internal Higun-to-Higun mapping.

## Address Transaction History

```text
GET /wallet/v1/{chain}/address/{address}/history
```

Query parameters:

- `page`: optional integer. Default `1`.
- `limit`: optional integer. Default `20`, maximum `100`.
- `confirmedOnly`: optional boolean. Default `false`.
- `sort`: optional. Default `desc`. Supported values are `desc` and `asc`.

Default behavior:

- include confirmed and mempool history;
- sort newest first;
- apply `confirmedOnly` filtering before pagination;
- apply sorting before pagination;
- use `confirmedOnly=true` only when the caller explicitly wants to hide
  mempool/unconfirmed items.

Response:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "address": "example-address",
    "page": 1,
    "limit": 20,
    "confirmedOnly": false,
    "sort": "desc",
    "total": 1,
    "items": [
      {
        "txid": "example-txid",
        "direction": "income",
        "incomeSatoshi": 1000,
        "spendSatoshi": 0,
        "netSatoshi": 1000,
        "income": "0.00001000",
        "spend": "0",
        "net": "0.00001000",
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

History item rules:

- `direction` is one of `income`, `spend`, or `mixed`;
- `incomeSatoshi` and `spendSatoshi` are unsigned integer amounts;
- `netSatoshi = incomeSatoshi - spendSatoshi` and may be negative;
- decimal amount fields are strings;
- `confirmed=false` and `mempool=true` identify unconfirmed items;
- confirmed history should return `confirmations >= 1` when height is known;
- mempool history returns `confirmations=0` and `height=null`;
- `timestamp` is Unix seconds and is the canonical time field;
- `time` is a display string for compatibility and debugging only.

Initial core mapping:

```text
GET {core_url}/utxos/history?address={address}&page={page}&limit={limit}
```

Current Higun core already returns address-level transaction history from this
route. Wallet Gateway v1.1 normalizes the current `tx_id`, `income`, `spend`,
`type`, `is_mempool`, and `time` fields into the public response above.

If the current core route cannot provide numeric timestamps or confirmation
counts, the v1.1 implementation should extend the core response additively. The
gateway should not fabricate confirmations from incomplete data.

If the current core route cannot apply `confirmedOnly` and `sort` before
pagination, the v1.1 implementation should extend the core route additively so
the public gateway pagination is stable.

## Fee Rate

```text
GET /wallet/v1/{chain}/fee-rate
```

Response:

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

Rules:

- this endpoint is public and intended for downstream applications;
- downstream applications should not configure fee rates themselves;
- the first implementation reads fee rates from Higun config;
- `source` is `config` in v1.1;
- `unit` is `sat_per_byte` for BTC, MVC, and DOGE in v1.1;
- `slow`, `normal`, and `fast` are positive integer rates;
- `default` must be one of `slow`, `normal`, or `fast`;
- an enabled chain must have an effective fee-rate config from YAML, environment
  overrides, or application defaults;
- if no valid effective fee-rate config can be built for an enabled chain,
  gateway startup must fail;
- `-5005 fee rate unavailable` is reserved for unexpected runtime lookup
  failures after startup;
- a later dynamic estimator may change `source` to a value such as `estimator`
  without changing the endpoint path or response field names.

Recommended config shape:

```yaml
wallet:
  enabled: true
  timeout_seconds: 10
  chains:
    btc:
      enabled: true
      core_url: "http://127.0.0.1:8066"
      broadcast_path: "/btc/broadcast"
      fee_rate:
        unit: sat_per_byte
        slow: 1
        normal: 3
        fast: 5
        default: normal
    mvc:
      enabled: true
      core_url: "http://127.0.0.1:8067"
      fee_rate:
        unit: sat_per_byte
        slow: 1
        normal: 2
        fast: 3
        default: normal
    doge:
      enabled: true
      core_url: "http://127.0.0.1:8068"
      fee_rate:
        unit: sat_per_byte
        slow: 1
        normal: 2
        fast: 5
        default: normal
```

The config is an internal Higun deployment concern. It is not a downstream
application requirement.

## Error Contract Additions

Existing v1 error codes remain valid:

- `0`: success
- `-4001`: unsupported chain
- `-4002`: invalid address or missing address
- `-4003`: invalid query parameter
- `-5001`: configured chain core is unavailable
- `-5002`: upstream core returned an invalid response
- `-5003`: wallet gateway internal error

Wallet Gateway v1.1 adds:

- `-4004`: invalid raw transaction
- `-4041`: transaction not found
- `-5004`: broadcast rejected
- `-5005`: fee rate unavailable

HTTP status behavior:

- request validation failures use `400`;
- unknown chains use `404`;
- configured but unavailable chains use `503`;
- transaction not found uses `404`;
- upstream transport failures use `502` or `503`;
- broadcast rejection uses `502`;
- successful application-level responses use `200`.

Public error messages should be stable and safe. Internal upstream errors may be
logged in redacted form but should not be returned to downstream applications
unless they are already safe and intentionally exposed.

## Package Boundary

v1.1 should extend the existing `wallet` package instead of adding a separate
gateway stack.

Expected package responsibilities:

- `config.go`: map app config to wallet config, including fee-rate config and
  optional broadcast path;
- `server.go`: register new `/wallet/v1` routes;
- `handlers.go`: parse transaction, history, and fee-rate requests;
- `client.go`: call configured core endpoints and normalize transport errors;
- `model.go`: add wallet transaction, history, and fee-rate internal models;
- `normalize.go`: normalize core transaction/history shapes and amount strings;
- `response.go`: render standard response envelopes;
- `errors.go`: add v1.1 error codes.

The package must not import Metalet service code. Metalet remains a historical
compatibility target, not a runtime dependency.

## Security And Logging

Wallet Gateway v1.1 handles signed raw transactions but not private keys.

Requirements:

- never log `rawTx`;
- never return raw upstream errors that include raw transactions;
- log only chain, route, txid when known, a redacted address, upstream host,
  status code, wallet error code, and latency;
- keep raw transaction body size bounded;
- use upstream HTTP timeouts;
- keep broadcast rate limiting as a deployment-level or middleware concern if
  the existing project pattern already supports it.

## Acceptance Criteria

Wallet Gateway v1.1 is complete when:

- `POST /wallet/v1/btc/tx/broadcast` works through the configured BTC core;
- `POST /wallet/v1/mvc/tx/broadcast` works through the configured MVC core;
- `POST /wallet/v1/doge/tx/broadcast` works through the configured DOGE core;
- broadcast success normalizes legacy core `code=2000,msg=txid` responses;
- raw transaction bodies are not logged;
- `GET /wallet/v1/{chain}/tx/{txid}` returns transaction detail and required
  confirmation status fields for BTC, MVC, and DOGE;
- missing txids return `-4041`;
- `GET /wallet/v1/{chain}/address/{address}/history` returns paginated history
  for BTC, MVC, and DOGE;
- history includes mempool/unconfirmed items by default;
- `confirmedOnly=true` filters out mempool history items;
- `GET /wallet/v1/{chain}/fee-rate` returns config-backed rates for BTC, MVC,
  and DOGE;
- downstream applications do not need fee-rate config;
- old v1 balance and UTXO endpoints still pass their existing tests;
- old Metalet services and routes remain untouched.

## Testing Strategy

Unit tests:

- fee-rate config validation and default rendering;
- invalid fee-rate config behavior;
- broadcast request validation;
- raw transaction redaction in logs;
- legacy broadcast response normalization;
- transaction-detail response rendering;
- missing transaction error mapping;
- history pagination defaults and bounds;
- history `confirmedOnly=true` filtering;
- history amount and direction normalization;
- v1 error behavior remains unchanged.

Handler tests:

- `POST /wallet/v1/{chain}/tx/broadcast`;
- `GET /wallet/v1/{chain}/tx/{txid}`;
- `GET /wallet/v1/{chain}/address/{address}/history`;
- `GET /wallet/v1/{chain}/fee-rate`;
- unsupported chain behavior for every new endpoint;
- upstream timeout behavior for every upstream-backed endpoint.

Integration checks:

- compare wallet broadcast against a configured chain node or local test node;
- compare wallet transaction detail against the corresponding core transaction
  route;
- compare wallet history against core `/utxos/history` for sampled addresses;
- verify history includes mempool items when the core reports mempool history;
- verify fee-rate endpoint returns configured values without any downstream
  client-side config.
