# Address Balance Index Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace on-demand confirmed balance recomputation and periodic rich-list full rescans with persistent Pebble-backed confirmed balance and ranking indexes.

**Architecture:** Add two new projection stores, `address_balance` and `balance_rank`, maintained incrementally during block indexing and reversed during reorg handling. Keep a one-time bootstrap/rebuild path for existing production data, then switch `/balance` and `/rich-list` to the new indexes while preserving mempool overlay behavior.

**Tech Stack:** Go, Pebble, Gin, Go test

---

### Task 1: Add failing tests for the new balance index contract

**Files:**
- Create: `indexer/balance_index_test.go`
- Modify: `api/server_rich_list_test.go`
- Modify: `indexer/query_rich_list_test.go`

- [ ] **Step 1: Write an indexer test for point-looked-up confirmed balances**
- [ ] **Step 2: Run the targeted test and verify it fails because no balance index exists yet**
- [ ] **Step 3: Write an indexer test for rich-list reads from a ranking index**
- [ ] **Step 4: Run the targeted rich-list test and verify it fails**
- [ ] **Step 5: Extend API tests so `/balance` and `/rich-list` reflect index-not-ready behavior**
- [ ] **Step 6: Run `go test ./api ./indexer -run 'Test(GetBalance|GetRichList|BalanceIndex)'` and verify failures are the expected missing-feature failures**

### Task 2: Add Pebble store types and typed balance helpers

**Files:**
- Modify: `storage/pebble.go`
- Modify: `main.go`
- Modify: `indexer/utxo.go`

- [ ] **Step 1: Add new store directory/type constants for `address_balance` and `balance_rank`**
- [ ] **Step 2: Extend `initDb` and `closeDb` so the new stores are opened and closed with the MVC indexer**
- [ ] **Step 3: Extend `UTXOIndexer` construction to carry the new stores**
- [ ] **Step 4: Add typed helpers for reading and writing balance rows and rank keys**
- [ ] **Step 5: Run targeted storage/indexer tests and verify the new helpers compile and pass**

### Task 3: Add UTXO lookup support for spend-side amount deltas

**Files:**
- Modify: `storage/pebble.go`
- Modify: `indexer/utxo.go`
- Test: `indexer/balance_index_test.go`

- [ ] **Step 1: Write a failing test for querying spent outpoints as `address + amount`**
- [ ] **Step 2: Run the targeted test and verify it fails**
- [ ] **Step 3: Add a new `utxoStore` helper that resolves outpoints to both address and amount**
- [ ] **Step 4: Update spend-side indexing code to use the richer lookup data**
- [ ] **Step 5: Re-run the targeted test and verify it passes**

### Task 4: Maintain balance and rank indexes incrementally during block indexing

**Files:**
- Modify: `indexer/utxo.go`
- Test: `indexer/balance_index_test.go`

- [ ] **Step 1: Write a failing test that indexing block outputs increments confirmed balance and UTXO count**
- [ ] **Step 2: Run the targeted test and verify it fails**
- [ ] **Step 3: Write a failing test that indexing block inputs decrements confirmed balance and UTXO count**
- [ ] **Step 4: Run the targeted test and verify it fails**
- [ ] **Step 5: Add per-block in-memory address deltas for income and spend**
- [ ] **Step 6: Apply those deltas to `address_balance` and `balance_rank` before block height advancement**
- [ ] **Step 7: Re-run the targeted indexing tests and verify they pass**

### Task 5: Bootstrap the balance projection from existing history stores

**Files:**
- Modify: `indexer/query.go`
- Modify: `indexer/utxo.go`
- Modify: `main.go`
- Test: `indexer/balance_index_test.go`

- [ ] **Step 1: Write a failing test for one-time bootstrap when the balance index is missing**
- [ ] **Step 2: Run the targeted bootstrap test and verify it fails**
- [ ] **Step 3: Add a background bootstrap path that rebuilds `address_balance` and `balance_rank` from `income/addressStore` and `spendStore`**
- [ ] **Step 4: Make bootstrap publish not-ready state until the rank index is durable**
- [ ] **Step 5: Re-run the targeted bootstrap test and verify it passes**

### Task 6: Reverse balance deltas during reorg

**Files:**
- Modify: `indexer/reorg.go`
- Possibly modify: `storage/pebble.go`
- Test: `indexer/balance_index_test.go`

- [ ] **Step 1: Write a failing reorg test covering both income rollback and spend rollback**
- [ ] **Step 2: Run the targeted reorg test and verify it fails**
- [ ] **Step 3: Audit and correct the existing spend rollback path before layering balance rollback on top**
- [ ] **Step 4: Reverse `address_balance` and `balance_rank` updates using archived block data**
- [ ] **Step 5: Re-run the targeted reorg test and verify it passes**

### Task 7: Switch `/balance` and `/rich-list` to the new indexes

**Files:**
- Modify: `indexer/query.go`
- Modify: `api/server.go`
- Modify: `api/server_rich_list_test.go`
- Test: `indexer/query_rich_list_test.go`
- Test: `indexer/balance_index_test.go`

- [ ] **Step 1: Write a failing test showing `/balance` confirmed fields come from `address_balance` while mempool overlay remains dynamic**
- [ ] **Step 2: Run the targeted test and verify it fails**
- [ ] **Step 3: Switch `GetBalance` confirmed reads to `address_balance`**
- [ ] **Step 4: Switch `GetRichList` reads to `balance_rank` and retain `503` when bootstrap is incomplete**
- [ ] **Step 5: Re-run targeted query and API tests and verify they pass**

### Task 8: Retire 4-hour full scans as the hot path

**Files:**
- Modify: `indexer/query.go`
- Modify: `main.go`
- Test: `indexer/balance_index_test.go`

- [ ] **Step 1: Write a failing test or assertion for bootstrap-only rebuild behavior**
- [ ] **Step 2: Run the targeted test and verify it fails**
- [ ] **Step 3: Replace periodic rich-list rebuild scheduling with bootstrap/rebuild-only behavior**
- [ ] **Step 4: Preserve a manual or internal rebuild path for repair scenarios**
- [ ] **Step 5: Re-run targeted tests and verify they pass**

### Task 9: Final verification

**Files:**
- Modify: `docs/superpowers/specs/2026-03-31-address-balance-index-design.md`
- Modify: `docs/superpowers/plans/2026-03-31-address-balance-index.md`

- [ ] **Step 1: Run `SDKROOT=$(xcrun --sdk macosx --show-sdk-path) GOPROXY=https://goproxy.cn,direct go test ./api ./indexer` locally**
- [ ] **Step 2: If cross-platform validation is required, run the same package tests in a Linux builder container**
- [ ] **Step 3: Summarize bootstrap expectations for production rollout**
- [ ] **Step 4: Document any residual reorg or rebuild risks that remain after tests pass**
