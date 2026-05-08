# BTC Reorg Recovery Runbook

## Scope

This document records the BTC production recovery performed on `2026-04-12` to `2026-04-13` for:

- `higun_btc` on `8.217.251.101`
- compat BTC API on `8.217.251.101:8085`
- `metalet-service` on `47.76.58.120`

It is intended as the runbook to follow if BTC reorg-related dirty data appears again.

## Incident Summary

The production issue was not a single bug. It was a stack of three failures:

1. A BTC reorg left dirty data in the `utxo / income / spend` indexes inside `higun_btc`.
2. During recovery, the local mempool databases became invalid, so the service could still return confirmed balances but could not correctly track unconfirmed spends and change outputs.
3. `metalet-service` had recently changed its BTC v3 compatibility path to prefer a newer gRPC address API whose UTXO semantics did not match what existing Metalet wallet clients expected.

This combination produced symptoms such as:

- BTC transfer error: `Transaction already in block chain`
- BTC transfer error: `[-25] bad-txns-inputs-missingorspent`
- BTC transfer error: `[-26] txn-mempool-conflict`
- balances missing or drifting after self-transfer
- first transfer succeeds, second transfer fails before confirmation

## Final Resolved State

As of `2026-04-13`:

- `higun_btc :8066` is the authoritative internal BTC balance and UTXO service.
- `8.217.251.101:8085` points to `:8066` and serves as the compatibility BTC API.
- `metalet-service` BTC v3 reads prefer the compat path backed by `251:8085`.
- mempool tracking is active again, so consecutive transfers can work without waiting for confirmation.
- there is no separate long-running `full repair` process still active.

## Root Causes

### 1. Reorg dirty data in main BTC index

The BTC index on `251` had become inconsistent after reorg handling. The required recovery was to bring the main index back to a consistent height and make `:8066` trustworthy again.

### 2. Mempool database corruption

The mempool manager initially failed to start. The key log signature was:

```text
Failed to create mempool income database at data/mempool_income
... MANIFEST-000001: no such file or directory
```

This meant `mempool_income` had an invalid Pebble state and the service was effectively running without mempool semantics.

When mempool is missing:

- confirmed UTXOs may still look correct
- unconfirmed spends are not excluded
- unconfirmed change is not added back
- second transfer can reuse already-spent inputs

### 3. BTC v3 compatibility path in `metalet-service`

`metalet-service` BTC v3 APIs had been changed to prefer a newer address gRPC path first. Existing Metalet wallet clients depend on the older BTC UTXO semantics. That mismatch caused wrong UTXO selection even after the main index was mostly fixed.

The fix was to restore BTC v3 read behavior so that:

- `v3/address/btc-utxo`
- `v3/address/btc-balance`

prefer the internal compat path backed by `251:8085`, with gRPC only used as fallback.

## Recovery Architecture

### Production components

- Main BTC indexer and API:
  - host: `8.217.251.101`
  - service: `higun_btc`
  - internal API: `http://127.0.0.1:8066`

- BTC compat proxy:
  - host: `8.217.251.101`
  - external/internal compat API: `http://8.217.251.101:8085`

- legacy readonly balance service:
  - host: `8.217.251.101`
  - port: `18067`
  - note: emergency-only, not preferred for normal production reads

- wallet API:
  - host: `47.76.58.120`
  - service: `metalet-service`
  - public URL: `https://www.metalet.space/wallet-api/v3/...`

## Recovery Order

Always follow this order.

### Step 1. Verify whether the main BTC index is healthy

On `251`, confirm:

- `higun_btc` container is up
- `:8066` responds
- current indexed height is at or near node height
- balances and UTXOs are returned from internal data, not third-party fallback

Useful checks:

```bash
docker ps
docker logs --tail 200 higun_btc
curl 'http://127.0.0.1:8066/balance?address=<btc_address>'
curl 'http://127.0.0.1:8066/utxos?address=<btc_address>'
curl 'http://127.0.0.1:8066/cleanedHeight/get'
```

If `:8066` itself is bad, fix this first. Do not start by changing wallet clients.

### Step 2. Verify mempool is actually enabled

Look for these successful startup markers:

```text
Mempool manager initialized successfully
Mempool manager started via API, listening for new transactions...
Mempool core started successfully
Fetched <N> mempool transactions from node
Mempool data initialization complete
```

If logs show `mempoolMgr is nil: true` or `Failed to create mempool income database`, mempool is broken.

### Step 3. If mempool DB is broken, rebuild it

The mempool Pebble stores are derived data and can be rebuilt.

Paths:

- `/date/higun_btc/data/mempool_income`
- `/date/higun_btc/data/mempool_spend`

Observed recovery method:

1. stop `higun_btc`
2. delete and recreate `mempool_income` and `mempool_spend`
3. start `higun_btc`
4. verify mempool re-initialization in logs

Typical sequence:

```bash
docker stop -t 20 higun_btc
# rebuild /date/higun_btc/data/mempool_income
# rebuild /date/higun_btc/data/mempool_spend
docker start higun_btc
docker logs -f higun_btc
```

After restart, do not trust the service until logs confirm mempool initialization completed.

### Step 4. Restore the compat BTC path

Ensure the compat proxy on `251:8085` points to the repaired main API `:8066`, not to the old readonly path.

Desired state:

- `251:8085 -> 127.0.0.1:8066`

Not preferred for production steady state:

- `251:8085 -> 127.0.0.1:18067`

Reason:

- `18067` was too slow for large addresses
- `8066` is the repaired authoritative service

### Step 5. Restore `metalet-service` BTC v3 compatibility behavior

The wallet-facing APIs are:

- `GET /wallet-api/v3/address/btc-utxo`
- `GET /wallet-api/v3/address/btc-balance`
- `POST /wallet-api/v3/tx/broadcast`

The recovery on `2026-04-13` changed `metalet-service` so BTC v3 reads prefer the internal compat path again.

Relevant file:

- [wallet_service_v3.go](/Users/tusm/Documents/MetaID_Projects/metalet-service/service/wallet_service/wallet_service_v3.go)

Required behavior:

- `btc-utxo` should prefer `own_service` first
- `btc-balance` should prefer `own_service` first
- gRPC should remain fallback only

Reason:

- older wallet clients depend on the older BTC UTXO semantics
- this path now resolves against the repaired internal indexer on `251:8085`

## Validation Checklist

Use real wallet behavior, not only unit checks.

### Core service checks on `251`

Expected:

- `:8066/balance` returns `confirmed_balance`, `mempool_income`, `mempool_spend`
- `:8066/utxos` returns correct confirmed and unconfirmed spendable set
- mempool fields change when there is an unconfirmed transaction

Example:

```bash
curl 'http://127.0.0.1:8066/balance?address=<btc_address>'
curl 'http://127.0.0.1:8066/utxos?address=<btc_address>'
curl 'http://127.0.0.1:8066/mempool/utxos?address=<btc_address>'
```

### Compat API checks on `251:8085`

Expected:

- `address/btc-utxo` matches the repaired `:8066` behavior
- `address/btc-balance` matches the repaired `:8066` behavior

Example:

```bash
curl 'http://8.217.251.101:8085/address/btc-utxo?address=<btc_address>&unconfirmed=1&order=desc'
curl 'http://8.217.251.101:8085/address/btc-balance?address=<btc_address>'
```

### Wallet API checks on `metalet-service`

Expected:

- production wallet API returns the same UTXO semantics as `251:8085`
- broadcast endpoint returns structured BTC RPC errors for invalid raw tx

Example:

```bash
curl 'https://www.metalet.space/wallet-api/v3/address/btc-utxo?net=livenet&address=<btc_address>&unconfirmed=1&order=desc'
curl 'https://www.metalet.space/wallet-api/v3/address/btc-balance?net=livenet&address=<btc_address>'
curl -X POST 'https://www.metalet.space/wallet-api/v3/tx/broadcast' \
  -H 'content-type: application/json' \
  --data '{"chain":"btc","net":"livenet","rawTx":"00"}'
```

### Real user-flow checks

These are required before declaring recovery complete:

1. transfer BTC once
2. immediately transfer BTC again before confirmation
3. self-transfer BTC and verify balance does not drift incorrectly
4. confirm wallet can still broadcast
5. confirm wallet balance remains visible during mempool activity

If first transfer succeeds but second fails with `missingorspent`, suspect mempool tracking or wallet-facing UTXO semantics first.

## What Not To Do

- Do not rely on third-party BTC balance providers as the normal production path.
- Do not mix BTC and MVC compatibility logic just because the address format is shared.
- Do not assume `confirmed balance works` means the wallet is fixed.
- Do not validate only with one broadcast; always test consecutive transfers.

## Known Residual Issue

There is still a separate background stability issue unrelated to the main wallet recovery:

- `SyncBaseCount` / Pebble iterator panic during shutdown or background counting

Observed stack area:

- `storage.(*PebbleStore).IncrementalKeyCount`
- `indexer.SyncBaseCount`
- `main.go`

This did not block the BTC recovery completed on `2026-04-13`, but it should be fixed separately as a stability hardening task.

## Fast Triage Decision Tree

If BTC wallet behavior is broken again, use this order:

1. Is `higun_btc` up and is `:8066` current?
2. Does `:8066/balance` return nonzero `mempool_income` or `mempool_spend` when expected?
3. Did mempool manager initialize successfully?
4. Is `251:8085` pointing to `:8066`?
5. Does `metalet-service v3/address/btc-utxo` match `251:8085`?
6. Can the wallet do two consecutive transfers without waiting for confirmation?

## Files and Services Involved

Main repo:

- [main.go](/Users/tusm/Documents/MetaID_Projects/higun/main.go)
- [api/server.go](/Users/tusm/Documents/MetaID_Projects/higun/api/server.go)
- [mempool/mempool.go](/Users/tusm/Documents/MetaID_Projects/higun/mempool/mempool.go)
- [mempool/transaction.go](/Users/tusm/Documents/MetaID_Projects/higun/mempool/transaction.go)
- [tools/api_compat_proxy/main.go](/Users/tusm/Documents/MetaID_Projects/higun/tools/api_compat_proxy/main.go)

Wallet API repo:

- [wallet_service_v3.go](/Users/tusm/Documents/MetaID_Projects/metalet-service/service/wallet_service/wallet_service_v3.go)
- [tx_service.go](/Users/tusm/Documents/MetaID_Projects/metalet-service/service/tx_service/tx_service.go)
- [own_service.go](/Users/tusm/Documents/MetaID_Projects/metalet-service/service/own_service/own_service.go)

## Status After This Runbook

This runbook reflects the recovery that restored:

- correct BTC balances
- correct BTC UTXO selection
- working BTC broadcast
- working consecutive BTC transfers

If reorg-related symptoms reappear, start from this document rather than debugging from scratch.
