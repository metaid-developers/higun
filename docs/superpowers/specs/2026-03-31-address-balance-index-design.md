# Address Balance Index Design

**Problem**

The current MVC native balance path does not store confirmed balances directly. `/balance` recomputes confirmed balance per request from `income/addressStore` and `spendStore`, while `/rich-list` periodically performs a full scan over all addresses and recomputes balances from raw income/spend history. This is correct but wasteful:

- `/balance` latency scales with per-address history size instead of a point lookup.
- `/rich-list` requires a full-address scan and can take hours on production data.
- The current 4-hour refresh is a repeated full rebuild, not incremental maintenance.

**Goal**

Add persistent confirmed balance indexes so that:

- confirmed balance queries are direct Pebble lookups;
- rich-list reads from a maintained ranking index instead of rescanning `income` and `spend`;
- per-block updates are incremental;
- reorg handling reverses the same incremental deltas;
- full scans remain available only as rebuild/bootstrap tools.

## Existing Foundations

The current architecture already has the required source-of-truth data and update boundaries:

- `processIncome` in `indexer/utxo.go` sees every confirmed output as `address + amount`.
- `processSpend` already resolves each spent outpoint back to its owning address through `utxoStore`.
- `IndexBlock` already has the per-block transaction boundary where balance deltas should be committed.
- `HandleReorg` and block archive files already provide the boundary needed to reverse per-block writes.

This means the new balance index is an incremental projection layered on top of the existing UTXO/income/spend stores. It does not replace them.

## Proposed Stores

Add two new Pebble-backed stores.

### 1. `address_balance`

Key:

- `address`

Value:

- JSON or compact string containing:
  - `confirmed_balance_satoshi`
  - `confirmed_utxo_count`

Purpose:

- fast confirmed balance reads for `/balance`
- bootstrap source for repair and diagnostics
- canonical per-address confirmed state

### 2. `balance_rank`

Key:

- `reverse(balance_satoshi)` + `":"` + `address`

Value:

- empty payload or compact duplicate of balance metadata

Purpose:

- ordered scan for `/rich-list`
- direct top-N reads without scanning all addresses

`reverse(balance_satoshi)` means the encoded key sorts higher balances first in Pebble lexicographic order. A fixed-width zero-padded descending encoding is sufficient.

## Update Model

### Confirmed Outputs

When processing block outputs:

- add output amount to `address_balance.confirmed_balance_satoshi`
- increment `address_balance.confirmed_utxo_count`
- update the corresponding `balance_rank` entry

### Confirmed Inputs

When processing block inputs:

- resolve each spent outpoint to `address + amount`
- subtract amount from `address_balance.confirmed_balance_satoshi`
- decrement `address_balance.confirmed_utxo_count`
- update the corresponding `balance_rank` entry

### Important Constraint

`processSpend` currently resolves only the owning address for each spent outpoint. The new design requires a new `utxoStore` query helper that resolves both address and amount from an outpoint so spend-side balance deltas are exact.

## Block Commit Semantics

Balance updates should be applied inside the normal confirmed block indexing flow, after income and spend data for the block have been derived but before the block is considered fully indexed.

For each block:

1. accumulate per-address deltas in memory
2. read current `address_balance` entries for touched addresses
3. update `address_balance`
4. remove old `balance_rank` entries for touched addresses
5. insert new `balance_rank` entries
6. sync with the same durability policy already used for the main stores

This preserves existing indexing semantics while avoiding per-output random writes.

## Reorg Handling

The balance indexes must be reorg-safe.

Recommended rule:

- on reorg rollback, load archived block data and reverse the balance deltas implied by that block

Reverse rules:

- archived `IncomeData` becomes `-amount`, `-utxo_count`
- archived `SpendData` must be translated back into `+amount`, `+utxo_count`

Because spend rollback needs original spent amounts, the reorg path must either:

- derive them again from archived UTXO information; or
- persist enough spent amount information in the block archive / rollback helper data.

The implementation should also audit the current reorg deletion path before relying on it for the new balance projection.

## Bootstrap and Rebuild

Production already contains full `income` and `spend` history but no balance index. The first deployment therefore needs a one-time bootstrap.

Bootstrap behavior:

- if `address_balance` is missing and indexed addresses already exist, start a background bootstrap scan
- build `address_balance` and `balance_rank` from the existing `income/addressStore` and `spendStore`
- expose index-not-ready status until bootstrap completes

After bootstrap:

- stop using periodic full rich-list rescans as the primary mechanism
- keep a rebuild path for repair, verification, and disaster recovery only

## API Behavior

### `/balance`

New behavior:

- confirmed balance and confirmed UTXO count come from `address_balance`
- mempool income/spend overlay is still computed dynamically
- response format stays unchanged

This preserves current API compatibility while making confirmed reads O(1) point lookups.

### `/rich-list`

New behavior:

- read top N directly from `balance_rank`
- keep `limit` capped at 100
- return `503` while bootstrap/rebuild has not produced the index yet

This turns rich-list from a repeated batch compute into a cheap ordered read.

## Storage Impact

The new indexes add extra space, but much less than the raw history stores:

- one row per address in `address_balance`
- one row per positive-balance address in `balance_rank`

At current production scale, this should be far smaller than the existing `income` and `spend` stores. The tradeoff is favorable because it replaces repeated full scans with direct reads and bounded per-block updates.

## Failure and Recovery

If balance index writes fail during block indexing:

- fail the block indexing step
- do not advance `last_indexed_height`
- rely on normal restart/replay behavior

If the index is corrupted or deleted:

- block indexing can keep running only if balance index writes are restored quickly; otherwise queries depending on the index should return not-ready
- a rebuild command or startup bootstrap should reconstruct the projection from canonical history stores

## Recommended Rollout

1. add the stores and bootstrap path
2. keep the current rich-list full scan only as a rebuild tool
3. switch `/balance` to the balance store plus mempool overlay
4. switch `/rich-list` to the ranking index
5. verify production bootstrap once
6. remove the 4-hour rich-list full rebuild from the hot path

## Out of Scope

- changing the external `/balance` response schema
- mempool-native rich-list ordering
- FT/NFT balance indexing
- introducing a non-Pebble database
