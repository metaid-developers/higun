# Rich List Limit Design

## Goal

Make the existing `GET /rich-list` endpoint usable as a confirmed-only SPACE top-N query by:
- supporting `limit` as the primary request parameter
- capping returned size at `100`
- returning an explicit service error when the cached rich list is not ready or is obviously invalid

## Scope

In scope:
- keep using the existing `/rich-list` route
- keep using the existing cached rich-list scan
- count only confirmed on-chain balances
- default to first-page top-N behavior

Out of scope:
- new endpoint paths
- mempool-aware ranking
- FT or NFT ranking
- on-demand full-scan API

## Request Contract

`GET /rich-list`

Supported query parameters:
- `limit`: alias of `page_size`, default `100`, min `1`, max `100`
- `page`: optional, default `1`
- `page_size`: kept for backward compatibility, but also capped at `100`

If both `limit` and `page_size` are provided, `limit` wins.

## Response Contract

Success keeps the current JSON envelope:

```json
{
  "total": 100,
  "page": 1,
  "page_size": 100,
  "list": []
}
```

Failure behavior changes:
- when the cache key is missing, return `503`
- when the cache payload exists but is empty while indexed address count is greater than zero, return `503`

## Implementation Notes

- Reuse `indexer.GetRichList`.
- Introduce a sentinel rich-list cache-not-ready error so the API layer can map it to `503`.
- Detect obviously invalid empty cache by checking `total_address_count` in metadata.
- Do not change the confirmed-only scan logic in `runRichListScan`.

## Testing

- indexer test for missing cache with indexed addresses
- indexer test for invalid empty cache with indexed addresses
- api test for `limit` alias and max cap
- api test for `503` when cache is unavailable
