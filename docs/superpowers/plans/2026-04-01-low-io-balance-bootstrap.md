# Low IO Balance Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the full persistent `address -> confirmed_balance` table on the isolated test server with a resumable, low-memory bootstrap that does not depend on `income/spend` history blobs.

**Architecture:** Replace the current bootstrap algorithm with a two-phase pipeline. Phase 1 scans the current `utxo` Pebble store sequentially, accumulates bounded in-memory address deltas, and incrementally merges them into `address_balance`. Phase 2 scans `address_balance` sequentially to rebuild `balance_rank`. Persist phase/shard/key progress in `meta`, keep `balance_index_ready=0` until both phases finish, and run the tool on the candidate data directory only with low-priority scheduling.

**Tech Stack:** Go, Pebble, Go test, SSH deployment to isolated ECS test instance

---

### Task 1: Replace bootstrap test fixtures with UTXO-based expectations

**Files:**
- Modify: `tools/bootstrap_balance_index/main_test.go`

- [ ] **Step 1: Write a failing test showing a full run builds `address_balance` and `balance_rank` from seeded `utxo` rows without relying on `income/spend`**
- [ ] **Step 2: Run `go test ./tools/bootstrap_balance_index -run TestRunBuildsConfirmedBalanceIndexes -count=1` and verify it fails for the expected bootstrap-path reason**
- [ ] **Step 3: Write a failing resume test that seeds `utxo` rows, limits the first run, and expects progress to resume from the saved UTXO shard/key**
- [ ] **Step 4: Run `go test ./tools/bootstrap_balance_index -run TestRunResumesBootstrapFromSavedProgress -count=1` and verify it fails for the expected reason**

### Task 2: Implement resumable UTXO-to-balance bootstrap

**Files:**
- Modify: `tools/bootstrap_balance_index/main.go`
- Modify: `tools/bootstrap_balance_index/main_test.go`

- [ ] **Step 1: Write a failing unit test for parsing one UTXO value into per-address balance deltas, skipping invalid or `errAddress` outputs**
- [ ] **Step 2: Run the targeted unit test and verify it fails**
- [ ] **Step 3: Add a sequential UTXO shard scanner that resumes from `current_shard + last_key`, accumulates bounded address deltas, and flushes them into `address_balance`**
- [ ] **Step 4: Persist bootstrap progress after each committed flush while keeping `balance_index_ready=0`**
- [ ] **Step 5: Run the targeted bootstrap tests and verify they pass**

### Task 3: Merge deltas safely into `address_balance`

**Files:**
- Modify: `tools/bootstrap_balance_index/main.go`
- Modify: `tools/bootstrap_balance_index/main_test.go`

- [ ] **Step 1: Write a failing test that multiple UTXO records for the same address merge into one `address_balance` row across flush boundaries**
- [ ] **Step 2: Run the targeted test and verify it fails**
- [ ] **Step 3: Implement bounded delta flushing using existing rows from `address_balance`, deleting zero rows and preserving resumability**
- [ ] **Step 4: Re-run the targeted merge test and the full tool package tests and verify they pass**

### Task 4: Rebuild `balance_rank` only after the balance table is complete

**Files:**
- Modify: `tools/bootstrap_balance_index/main.go`
- Modify: `tools/bootstrap_balance_index/main_test.go`

- [ ] **Step 1: Write a failing test that the tool rebuilds `balance_rank` from `address_balance` in sorted order and sets `balance_index_ready=1` only after the rank phase finishes**
- [ ] **Step 2: Run the targeted test and verify it fails**
- [ ] **Step 3: Add a second resumable phase that scans `address_balance`, writes `balance_rank`, and marks bootstrap `Done=true` only at the end**
- [ ] **Step 4: Re-run the targeted test and then `go test ./tools/bootstrap_balance_index ./tools/check_balance_index -count=1`**

### Task 5: Deploy and run on the isolated test server

**Files:**
- Modify: `docs/superpowers/plans/2026-04-01-low-io-balance-bootstrap.md`

- [ ] **Step 1: Build the updated Linux bootstrap and checker binaries locally**
- [ ] **Step 2: Upload them to `/data/higun_candidate/run` on `47.245.138.101`**
- [ ] **Step 3: Start bootstrap with `ionice -c3`, `nice -n 19`, conservative batch settings, and the isolated data dir `/data/higun_candidate/data`**
- [ ] **Step 4: Poll progress and verify sample addresses with `check_balance_index` while watching memory, IO, and row counts**
- [ ] **Step 5: Continue until the test instance reports the full table complete, then summarize runtime and production rollout implications**
