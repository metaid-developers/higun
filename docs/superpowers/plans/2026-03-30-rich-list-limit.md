# Rich List Limit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/rich-list` serve confirmed-only SPACE top-N queries through `limit`, capped at `100`, with explicit cache-not-ready behavior.

**Architecture:** Keep the existing route and cached ranking scan. Tighten the API parameter contract in the handler and make the indexer surface a dedicated cache-not-ready error when cached data is missing or invalid.

**Tech Stack:** Go, Gin, Pebble, Go test

---

### Task 1: Add failing rich-list behavior tests

**Files:**
- Create: `indexer/query_rich_list_test.go`
- Create: `api/server_rich_list_test.go`

- [ ] **Step 1: Write indexer tests for missing and invalid cache**
- [ ] **Step 2: Run the targeted indexer tests and verify they fail**
- [ ] **Step 3: Write API tests for `limit` alias and cache-not-ready status**
- [ ] **Step 4: Run the targeted API tests and verify they fail**

### Task 2: Implement rich-list cache readiness rules

**Files:**
- Modify: `indexer/query.go`

- [ ] **Step 1: Add a sentinel error for rich-list cache-not-ready**
- [ ] **Step 2: Treat missing cache as not ready when indexed address metadata is non-zero**
- [ ] **Step 3: Treat empty cached payload as not ready when indexed address metadata is non-zero**
- [ ] **Step 4: Run targeted indexer tests and verify they pass**

### Task 3: Implement `/rich-list` limit contract

**Files:**
- Modify: `api/server.go`

- [ ] **Step 1: Parse `limit` as the primary size parameter**
- [ ] **Step 2: Cap `limit` and `page_size` at `100`**
- [ ] **Step 3: Map cache-not-ready errors to HTTP `503`**
- [ ] **Step 4: Run targeted API tests and verify they pass**

### Task 4: Final verification

**Files:**
- Modify: `docs/superpowers/specs/2026-03-30-rich-list-limit-design.md`
- Modify: `docs/superpowers/plans/2026-03-30-rich-list-limit.md`

- [ ] **Step 1: Run all rich-list targeted tests together**
- [ ] **Step 2: Run `go test ./api ./indexer` if feasible**
- [ ] **Step 3: Summarize any residual risk if broader tests are not clean**
