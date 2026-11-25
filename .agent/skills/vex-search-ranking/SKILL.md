---
name: vex-search-ranking
description: Reference guide for Vex's v2 SQLite search schema, value-only SearchableBody indexing, 4-tier exact/contextual/prefix/guarded-fuzzy ranking pipeline, dynamic relevance cliff, and hybrid vector RRF scoring. Use whenever modifying internal/indexer/store.go, internal/models/card.go (SearchableBody/ChipsText), or internal/search/service.go ranking logic.
---

# Vex — Search Precision & SQLite Schema v2 Guide

## 1. SQLite Schema v2 (`internal/indexer/store.go`)

- **`PRAGMA user_version = 2`**: `OpenStore` runs `migrate()` automatically. Legacy `v0/v1` `~/.vex/index.db` databases are upgraded atomically in place (`ALTER TABLE cards ADD COLUMN ...` + `cards_fts` rebuild + row backfill) without dropping `card_embeddings`.
- **Indexed Columns on `cards`**:
  - `entity_key` *(lowercased `card.EntityKey`, e.g. `loc:ams`)* — indexed by `idx_cards_entity_key`
  - `entity_code` *(lowercased suffix after `:`, e.g. `ams`, or full key)* — indexed by `idx_cards_entity_code`
  - `chips_text` *(space-joined `GlanceChip.Label` values via `card.ChipsText()`)*
  - `related_keys_json` *(JSON array of `card.RelatedKeys`)*
  - `updated_at` — indexed by `idx_cards_updated_at` (`DESC`)
- **9-Column `cards_fts` (`porter unicode61`)**:
  - Columns: `id UNINDEXED, collection UNINDEXED, entity_key, title, chips, modifiers, tags, subtitle, body`
  - BM25 weights: `bm25(cards_fts, 0.0, 0.0, 14.0, 12.0, 10.0, 9.0, 9.0, 6.0, 2.0)`
- **Single-Connection Cursor Rule (`SetMaxOpenConns(1)`)**: Always read all rows into memory and call `rows.Close()` **before** issuing follow-up `UPDATE`, `INSERT`, or `SELECT ... WHERE id IN (...)` queries on the same connection or transaction.

## 2. Value-Only `SearchableBody()` (`internal/models/card.go`)

- `SearchableBody()` uses `writeIndexableValue` to extract only human-readable string/numeric leaf values from `Data`, `Sections.Fields`, `Children`, and non-URL `Actions`.
- **Never index structural boilerplate**: `skippedDataKeys` excludes `contract`, `url`, `devUrl`, `qaUrl`, `prodUrl`, `trackingUrl`, `mapUrl`, `sshCommand`, `mode`, `aspect`, `tone`, `dot`, `icon`, `ianaTimezone` (and snake_case equivalents), as well as boolean flags and Go `fmt.Sprintf("%v", map)` dumps.
- `ChipsText()` feeds the dedicated `chips` FTS5 column and `cards.chips_text`.

## 3. 4-Tier Ranking & Score Bands (`SearchHybrid` + `search.Service`)

| Band | Layer / Tier | `MatchSource` | Semantics |
| :--- | :--- | :--- | :--- |
| `150.0` | Service: Converter | `calculator` | Instant unit conversions & 3D cargo dim-weight (`TryConvertQuery`) |
| `145.0` / `140.0` | Service: Pattern Engine | `pattern` / `pattern-enrichment` | Regex + checksum pattern decode & linked entity enrichment |
| `120.0` / `40.0` | Service: Plugins | `plugin` | Explicit `/modifier` plugin (`120.0`) or ambient plugin (`40.0`) |
| `88.0 – 100.0` | Store Tier 1: Exact | `exact` | B-tree match on `id`/`entity_key`/`entity_code`/`title`, plus exact `modifier`/`tag`/chip segment/`(CODE)` verified unstemmed |
| `52.0 – 78.0` | Store Tier 2b: Neighbors | `contextual` | Up to **4** 1-hop `RelatedKeys`/`EntityKey` neighbors of Tier 1 entity hits (respects active filters) |
| `45.0 – 75.0` | Store Tier 2a: Whole-Token FTS | `fts` | Un-wildcarded FTS5 BM25 (`"tok"`, no `*`) |
| `18.0 – 38.0` | Store Tier 3: Header Prefix | `fts` | Prefix FTS5 (`"tok"*`) matching title/subtitle/entity_key/chips/tags/modifiers |
| `16.0 – 24.0` | Store Tier 3: Body Prefix | `fts` | Prefix FTS5 matching `body` only (kept below the `0.25 * topScore` Tier 1 cliff) |
| `5.0 – 14.0` | Store Tier 4: Guarded Fuzzy | `fuzzy` | Runs **only** when Tiers 1–3 return `< 3` hits and query has `>= 3` runes; searches metadata columns only (**never `lower(card_json) LIKE`**) |

### Critical Query Guardrails
- **Never scan `lower(card_json)`** in SQL `LIKE` queries; `searchFuzzyLocked` must stay restricted to `title`, `subtitle`, `entity_key`, `chips_text`, and `tags_json` with `ESCAPE '\'`.
- **Parenthesize multi-token FTS5 column filters**: Always wrap multi-token queries in `{entity_key title chips modifiers tags} : (...)` so SQLite FTS5 applies the column filter across all tokens.
- **Dynamic Relevance Cliff**: For non-empty lexical queries (`len(QueryEmbedding) == 0`), prune results below `0.25 * topScore` when `topScore >= 80.0`, and below `0.30 * topScore` when `topScore >= 45.0`. Bypass the cliff when `len(QueryEmbedding) > 0` so semantic/hybrid RAG retrieval stays intact.
- **Interactive UI Limit**: `AppComponent` requests `limit = 10` on active search queries and `limit = 24` when browsing with an empty query.
