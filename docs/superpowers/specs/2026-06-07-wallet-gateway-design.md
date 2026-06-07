# Wallet Gateway Design

## Problem

Higun already provides the core chain indexer APIs for address balance and UTXO
queries, but application-facing wallet APIs are currently reached through
separate services such as `metalet-service` and `asset-base-service`. This makes
the production path hard to reason about:

- new applications need to understand Metalet gateway paths instead of a single
  Higun wallet surface;
- BTC, MVC, and DOGE wallet data are routed through different service layers;
- old compatibility fields are mixed with raw chain-indexer behavior;
- self-hosted application developers cannot simply run Higun and integrate one
  wallet API.

The goal is to simplify the application contract while keeping Higun core
focused on chain-indexed facts.

## Goal

Add a Wallet Gateway module to Higun that gives new applications one stable
wallet API for BTC, MVC, and DOGE balance and UTXO reads.

The first version must:

- expose clean application-facing endpoints under `/wallet/v1`;
- support BTC, MVC, and DOGE;
- return a new standard response format by default;
- optionally return Metalet-compatible response shapes for migration support;
- use integer satoshi values as the canonical amount representation;
- include mempool UTXOs by default;
- keep existing Metalet services untouched for current consumers;
- support one gateway aggregating multiple chain-specific Higun core endpoints.

## Non-Goals

The first version does not migrate every Metalet feature into Higun.

Out of scope:

- replacing existing `metalet-service` production routes;
- moving BRC20, runes, inscriptions, pins, MRC20, FT, or NFT aggregation into the
  Wallet Gateway;
- changing existing Higun core `/balance` and `/utxos` response schemas;
- requiring all BTC/MVC/DOGE core indexers to run in the same process;
- exposing raw Higun host/IP details to new applications;
- implementing account management, signing, key custody, or wallet storage.

## Architecture

The Wallet Gateway is an application-facing layer inside the Higun repository.
It is separate from Higun core indexing logic.

```text
Applications
  -> /wallet/v1/{chain}/...
      -> Wallet Gateway
          -> BTC Higun Core endpoint
          -> MVC Higun Core endpoint
          -> DOGE Higun Core endpoint
```

Higun core remains responsible for raw indexed chain facts:

- confirmed balance;
- confirmed UTXO set;
- mempool income and spend overlays;
- UTXO history;
- UTXO validation;
- transaction broadcast.

The Wallet Gateway is responsible for wallet-facing semantics:

- chain routing;
- request validation;
- response normalization;
- safe balance calculation;
- UTXO filtering and sorting;
- application-oriented error envelopes;
- optional Metalet-compatible response formats;
- CORS, rate limiting, and observability hooks when enabled by deployment.

## Deployment Model

One Wallet Gateway instance can aggregate all three supported chains.

Configuration provides a core endpoint per chain:

```yaml
wallet:
  enabled: true
  chains:
    btc:
      enabled: true
      core_url: "http://127.0.0.1:8066"
    mvc:
      enabled: true
      core_url: "http://172.31.165.127:8085"
    doge:
      enabled: true
      core_url: "http://8.217.14.206:8066"
```

Self-hosted deployments can point every enabled chain at local Higun instances.
Production deployments can point each chain at its existing Higun core host.

If a chain is disabled or has no configured `core_url`, Wallet Gateway returns a
chain-unavailable error for that chain instead of falling through to another
service.

## Endpoint Contract

### Balance

```text
GET /wallet/v1/{chain}/address/{address}/balance
```

Supported chains:

- `btc`
- `mvc`
- `doge`

Query parameters:

- `format`: optional. Default `standard`. Supported values are `standard` and
  `metalet`.

Default response:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "address": "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ",
    "confirmedSatoshi": 135758,
    "unconfirmedSatoshi": 0,
    "mempoolIncomeSatoshi": 0,
    "mempoolSpendSatoshi": 0,
    "unsafeSatoshi": 134862,
    "safeSatoshi": 896,
    "utxoCount": 1040,
    "confirmed": "0.00135758",
    "unconfirmed": "0",
    "mempoolIncome": "0",
    "mempoolSpend": "0",
    "unsafe": "0.00134862",
    "safe": "0.00000896"
  }
}
```

Amount rules:

- integer satoshi fields are canonical;
- decimal fields are strings for display and compatibility;
- the gateway must not expose float amount fields in the standard response;
- decimal conversion uses 8 decimal places for BTC, MVC, and DOGE in the first
  version.

Safe balance rule:

```text
safeSatoshi = confirmedSatoshi + mempoolIncomeSatoshi - mempoolSpendSatoshi - unsafeSatoshi
```

If the result is negative:

```text
safeSatoshi = 0
```

`safeSatoshi` is a wallet usability estimate, not a separate chain consensus
fact.

### UTXOs

```text
GET /wallet/v1/{chain}/address/{address}/utxos
```

Query parameters:

- `confirmedOnly`: optional boolean. Default `false`.
- `sort`: optional. Default `desc`. Supported values are `desc` and `asc`.
- `format`: optional. Default `standard`. Supported values are `standard` and
  `metalet`.

Default behavior:

- return confirmed UTXOs and mempool UTXOs;
- use `confirmedOnly=true` to exclude mempool UTXOs;
- sort by amount descending unless `sort=asc` is provided.

Default response:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "chain": "btc",
    "address": "12ghVWG1yAgNjzXj4mr3qK9DgyornMUikZ",
    "confirmedOnly": false,
    "sort": "desc",
    "total": 1,
    "utxos": [
      {
        "txid": "example-txid",
        "vout": 0,
        "outpoint": "example-txid:0",
        "satoshi": 1000,
        "amount": "0.00001000",
        "confirmed": true,
        "mempool": false,
        "height": 952729
      }
    ]
  }
}
```

UTXO response rules:

- `satoshi` is canonical;
- `amount` is a decimal string;
- `confirmed=false` means the UTXO is from mempool data;
- `mempool=true` is included for direct filtering by applications;
- `height` may be `-1` or omitted when the core endpoint cannot provide a
  confirmed height.

## Metalet Compatibility Format

`format=metalet` is for migration and comparison, not the preferred format for
new applications.

For BTC balance, compatibility should preserve the existing Metalet v3 envelope
as closely as possible:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "balance": 0.00135758,
    "block": {
      "incomeFee": 0.00135758,
      "spendFee": 0
    },
    "mempool": {
      "incomeFee": 0,
      "spendFee": 0
    },
    "pendingBalance": 0,
    "safeBalance": 0.00000896,
    "unSafeBalance": 0.00134862
  }
}
```

Compatibility responses may contain float fields because they match historical
Metalet clients. Standard responses must not.

For MVC and DOGE balance, compatibility should preserve the current v4
`balance-info` shape:

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "address": "example-address",
    "confirmed": 0,
    "unconfirmed": 0,
    "utxoCount": 0
  }
}
```

For UTXO compatibility, the gateway should keep the historical field names used
by Metalet where practical, while deriving values from the normalized Wallet
Gateway UTXO model.

## Core Endpoint Mapping

The first implementation reads from existing Higun core APIs.

Balance source:

```text
GET {core_url}/balance?address={address}
```

UTXO source:

```text
GET {core_url}/utxos?address={address}
```

Optional mempool source when a core implementation separates mempool UTXOs:

```text
GET {core_url}/mempool/utxos?address={address}
```

The gateway should normalize different core deployments into one internal model:

```text
WalletBalance
  chain
  address
  confirmedSatoshi
  mempoolIncomeSatoshi
  mempoolSpendSatoshi
  unsafeSatoshi
  confirmedUtxoCount
  mempoolUtxoCount

WalletUTXO
  chain
  address
  txid
  vout
  satoshi
  confirmed
  mempool
  height
```

If a core endpoint already merges confirmed and mempool UTXOs, the gateway must
avoid duplicating mempool entries. Deduplication key is `txid:vout`.

## Error Contract

Standard errors use the same top-level envelope as successful standard
responses:

```json
{
  "code": -4001,
  "message": "unsupported chain",
  "data": null
}
```

Recommended codes:

- `0`: success
- `-4001`: unsupported chain
- `-4002`: invalid address or missing address
- `-4003`: invalid query parameter
- `-5001`: configured chain core is unavailable
- `-5002`: upstream core returned an invalid response
- `-5003`: wallet gateway internal error

HTTP status behavior:

- request validation failures should use `400`;
- unsupported or disabled chains should use `404` or `503` based on whether the
  chain is unknown or configured-but-unavailable;
- upstream failures should use `502` or `503`;
- successful application-level responses use `200`.

## Package Boundary

Recommended package layout:

```text
wallet/
  config.go
  server.go
  handlers.go
  client.go
  model.go
  normalize.go
  metalet_compat.go
  errors.go
```

Responsibilities:

- `server.go`: route registration under `/wallet/v1`;
- `handlers.go`: HTTP request parsing and response writing;
- `client.go`: per-chain Higun core HTTP client;
- `model.go`: internal wallet balance and UTXO models;
- `normalize.go`: core response normalization, amount conversion, sorting, and
  filtering;
- `metalet_compat.go`: compatibility response rendering only;
- `errors.go`: stable error codes and response helpers.

The package must not import Metalet service code. Historical Metalet behavior is
implemented as a compatibility contract, not by coupling Higun to the old
service.

## Observability

The gateway should log enough context to debug routing without leaking private
keys or signed transactions.

Log fields:

- chain;
- route;
- address hash or truncated address;
- upstream core URL host;
- upstream latency;
- status code;
- error code.

Metrics should be added when the existing project metric pattern is identified:

- request count by chain and endpoint;
- upstream latency by chain;
- upstream error count by chain;
- invalid request count;
- compatibility-format request count.

## Rollout Plan

1. Add the Wallet Gateway package and configuration.
2. Register `/wallet/v1` routes when wallet gateway is enabled.
3. Implement standard balance for BTC, MVC, and DOGE.
4. Implement standard UTXO list for BTC, MVC, and DOGE.
5. Add `format=metalet` compatibility rendering.
6. Validate against current public Metalet BTC/MVC/DOGE balance and UTXO
   endpoints for sampled addresses.
7. Publish the new Wallet Gateway endpoint for new applications.
8. Keep existing Metalet services serving old applications.
9. Migrate old applications only after compatibility and traffic observations
   are complete.

## Testing

Unit tests:

- safe balance calculation, including negative-result clamp to zero;
- decimal string conversion from satoshi values;
- UTXO sorting by amount ascending and descending;
- `confirmedOnly=true` filtering;
- deduplication by `txid:vout`;
- unsupported chain and invalid query parameter errors;
- Metalet compatibility rendering for BTC balance;
- Metalet compatibility rendering for MVC/DOGE balance-info.

Handler tests:

- `GET /wallet/v1/btc/address/{address}/balance`;
- `GET /wallet/v1/mvc/address/{address}/balance`;
- `GET /wallet/v1/doge/address/{address}/balance`;
- `GET /wallet/v1/{chain}/address/{address}/utxos`;
- `format=metalet`;
- disabled chain behavior;
- upstream timeout behavior.

Integration checks:

- compare BTC standard balance against `higun_btc /balance`;
- compare BTC compatibility balance against current Metalet v3 for sampled
  addresses;
- compare MVC compatibility balance against current Metalet v4 `balance-info`;
- compare DOGE compatibility balance against current Metalet v4 `balance-info`;
- confirm default UTXO responses include mempool entries when the core endpoint
  reports them.

## First-Version Acceptance Criteria

The first version is complete when:

- `/wallet/v1/btc/address/{address}/balance` returns normalized standard data;
- `/wallet/v1/mvc/address/{address}/balance` returns normalized standard data;
- `/wallet/v1/doge/address/{address}/balance` returns normalized standard data;
- `/wallet/v1/{chain}/address/{address}/utxos` works for all three chains;
- UTXO responses include mempool data by default;
- `confirmedOnly=true` filters out mempool UTXOs;
- standard responses contain no float amount fields;
- `format=metalet` is available for balance and UTXO responses;
- old Metalet routes remain untouched;
- the gateway can route BTC, MVC, and DOGE to different configured core URLs.

