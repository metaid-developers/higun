# AGENTS.md

This file provides guidance to AI coding agents working in this repository.

## Project Overview and Modules

- HIGUN (Hyper Indexer of General UTXO Network) is a high-performance Go indexer for BTC, MVC, and DOGE UTXO networks.
- It tracks confirmed and mempool UTXOs, address histories, spend records, balances/rich lists, block info, and MetaContract FT/NFT assets.
- Storage is Pebble-based and sharded; production behavior is sensitive to RPC latency, memory, batch size, worker count, and reorg handling.
- `main.go` is the standard UTXO runtime; `apps/ft-main` and `apps/nft-main` are dedicated MetaContract runtimes.
- `config/` defines YAML config, chain selection, RPC settings, resource tuning, and wallet gateway options.
- `blockchain/` contains BTC/MVC/DOGE adapters, block/tx fetchers, reorg/backfill handling, and FT/NFT clients.
- `indexer/` contains core UTXO indexing, balance/rich-list indexes, history, reorg cleanup, and contract indexers under `indexer/contract/`.
- `storage/` wraps Pebble stores, metadata, backup helpers, and query primitives.
- `mempool/` handles ZMQ subscriptions, unconfirmed UTXO tracking, and FT/NFT mempool verification.
- `api/` exposes Gin HTTP APIs, dashboard templates, FT/NFT controllers, wallet gateway endpoints, and response formats.
- `wallet/` implements gateway client logic, normalized responses, fee-rate lookup, and broadcast helpers.
- `explorer/blockindexer/` handles MVC block information indexing and explorer-facing APIs.
- `deploy/` contains Dockerfiles, compose files, and deployment scripts; `tools/` and `docs/` hold repair CLIs, diagnostics, smoke checks, Swagger, and runbooks.

## Commit and Merge Rules

- If you notice unfamiliar or unrelated file changes, continue working and stay focused on your own scoped edits unless the user asks you to inspect them.
- For each completed round that modifies existing code/docs or adds new code/docs, automatically stage and commit only the files you changed and understand.
- For deletion changes, wait until the user explicitly says "commit" before staging and committing those deletions.
- Prefer small, frequent commits. Commit each independent, verifiable unit of work as soon as it is complete.
- For every modification or newly added feature, create one commit.
- For every commit, use the `metabot-post-buzz` skill with the Lisa Hahn identity (slug: `lisa-hahn`) to post a detailed development-journal entry on-chain describing the change.
- Use commit messages in the format `<type>: <short description>`, where `<type>` is one of `feat`, `fix`, `refactor`, `docs`, or `chore`.
- Before committing, make sure the relevant local tests or verification steps pass for your changes.
- When merging completed work into `main`, use `git merge --no-ff` to preserve the feature merge point.

## Behavioral Guidelines

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them — don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it — don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

### 5. No Guessing, No Drive-By Fixes

**Verify boundaries before acting. Don't fix bugs you didn't create.**

- Never guess. When writing a plan or code, if anything is unclear or any scope boundary is ambiguous, either read the relevant code or discuss with the user — keep going until every boundary is clear.
- Don't opportunistically fix pre-existing bugs that fall outside the current task. Surface them to the user and let them decide; never silently change behavior you weren't asked to change.
