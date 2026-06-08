# AGENTS.md

This file guides AI coding agents working in this repository.

## Project Overview

- HIGUN (Hyper Indexer of General UTXO Network) is a high-performance Go indexer for BTC, MVC, and DOGE UTXO networks.
- It indexes confirmed and mempool UTXOs, address history, spend records, balances/rich lists, block info, and MetaContract FT/NFT assets.
- Storage is Pebble-based and sharded; production tuning is sensitive to RPC latency, memory, batch size, worker count, and chain reorgs.
- Main runtime is `main.go`; dedicated FT/NFT runtimes live in `apps/ft-main` and `apps/nft-main`.

## Module Map

- `config/`: YAML config, chain selection, RPC settings, resource tuning, wallet gateway config.
- `blockchain/`: BTC/MVC/DOGE adapters, block/tx fetchers, reorg/backfill handling, FT/NFT clients.
- `indexer/`: core UTXO indexing, balance/rich-list indexes, history, reorg cleanup, contract indexers under `indexer/contract/`.
- `storage/`: Pebble stores, metadata, backup helpers, query primitives.
- `mempool/`: ZMQ subscription, unconfirmed UTXO tracking, FT/NFT mempool verification.
- `api/`: Gin HTTP APIs, dashboard templates, FT/NFT controllers, wallet gateway endpoints and response formats.
- `wallet/`: wallet gateway client logic, normalized responses, fee-rate/broadcast helpers.
- `explorer/blockindexer/`: MVC block information indexing and explorer-facing APIs.
- `deploy/`: Dockerfiles, compose files, and deployment scripts for standard, FT, and NFT services.
- `tools/` and `docs/`: repair/bootstrap/diagnostic CLIs plus design notes, smoke checks, Swagger, and runbooks.

## Development Rules

- Start by reading the relevant module and existing tests; prefer repo patterns over new abstractions.
- Keep changes surgical. Do not refactor unrelated code, data paths, deployment scripts, or production configs without explicit scope.
- For chain logic, verify BTC/MVC/DOGE differences instead of assuming one adapter's behavior applies to another.
- For storage/indexing changes, consider reorg safety, idempotency, restart behavior, and Pebble write/read amplification.
- For API/wallet changes, preserve existing response compatibility unless the user explicitly asks to change it.
- Before claiming done, run focused verification: `go test ./...` when code behavior changes; for docs-only edits, run line-count and diff checks.

## Commit and Merge Rules

- If unfamiliar changes exist, leave them untouched and stage only files you changed and understand.
- Commit each independent verified docs/code change with `<type>: <short description>` using `feat`, `fix`, `refactor`, `docs`, or `chore`.
- Do not stage deletion changes unless the user explicitly says `commit`; use `git merge --no-ff` for completed merges into `main`.
- For every commit, post a detailed development journal with `metabot-post-buzz` using Lisa Hahn (`lisa-hahn`).
