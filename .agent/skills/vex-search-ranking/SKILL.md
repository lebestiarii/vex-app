---
name: vex-search-ranking
description: Reference guide for Vex's v3 SQLite search schema (entity_links + card_recents), layout-aware SearchableBody whitelist indexing, 4-tier exact/contextual/prefix/guarded-fuzzy ranking pipeline, dynamic 2-level child hydration, and dynamic relevance cliff. Use whenever modifying internal/indexer/store.go, internal/models/card.go (SearchableBody/ChipsText), or internal/search/service.go ranking logic.
---

# Vex — Search Precision & SQLite Schema v3 Guide

## 1. SQLite Schema v3 (`internal/indexer/store.go`)

- **`PRAGMA user_version = 3`**: `OpenStore` runs `migrate()` automatically. Legacy `v0/v1/v2` `~/.vex/index.db` databases are upgraded atomically in place (renaming `cards.template` → `cards.layout`, adding `parent_entity_key`, dropping deprecated `card_embeddings`, creating `entity_links` and `card_recents`, rebuilding 9-column `cards_fts`, and backfilling metadata + `entity_links`).
- **Indexed Columns on `cards`**:
  - `layout` *(one of the 9 strict `CardLayoutType` values)*
  - `entity_key` *(lowercased `card.EntityKey`, e.g. `loc:ams`)* — `idx_cards_entity_key`
  - `entity_code` *(lowercased suffix after `:`, e.g. `ams`, or full key)* — `idx_cards_entity_code`
  - `parent_entity_key` *(lowercased `card.ParentEntityKey` for flat-on-disk nesting)* — `idx_cards_parent_entity_key`
  - `chips_text` *(space-joined `GlanceChip.Label` values via `card.ChipsText()`)*
  - `related_keys_json` *(JSON array of `card.RelatedKeys`)*
  - `updated_at` — `idx_cards_updated_at` (`DESC`)
- **`entity_links` Table**:
  - `entity_links(source_card_id TEXT, target_entity_key TEXT, relation_type TEXT)` indexed on `(target_entity_key, relation_type)` with `FOREIGN KEY (source_card_id) REFERENCES cards(id) ON DELETE CASCADE`.
  - Populated from `parentEntityKey` (`relation_type = 'child_of'`) and `relatedKeys` (`relation_type = 'related_to'`).
  - `hydrateChildrenLocked` dynamically assembles up to **2 levels** (`L0 → L1 → L2`) in at most 2 batched SQL queries (zero N+1, cycle-safe) for `SearchHybrid`, `FindCardsByEntityKeys`, `GetCard`, `FindRelatedCards`, and `ListRecentCards`.
- **`card_recents` Table**:
  - `card_recents(card_id TEXT PRIMARY KEY, last_used_at INTEGER NOT NULL)` indexed on `last_used_at DESC`.
  - Managed via `RecordCardUsed(ctx, cardID)` and `ListRecentCards(ctx, limit)`.
- **9-Column `cards_fts` (`porter unicode61`)**:
  - Columns: `id UNINDEXED, collection UNINDEXED, entity_key, title, chips, modifiers, tags, subtitle, body`
  - BM25 weights: `bm25(cards_fts, 0.0, 0.0, 14.0, 12.0, 10.0, 9.0, 9.0, 6.0, 2.0)`
- **Single-Connection Cursor Rule (`SetMaxOpenConns(1)`)**: Always read all rows into memory and call `rows.Close()` **before** issuing follow-up `UPDATE`, `INSERT`, or `SELECT ... WHERE id IN (...)` queries on the same connection or transaction.

## 2. Layout-Aware Whitelist `SearchableBody()` (`internal/models/card.go`)

`SearchableBody()` uses a strict layout-aware switch over the 9 generic layouts — extracting ONLY human-readable searchable text (never URLs, tones, UI flags, `monoColumns`, `group` meta, or JSON keys):
- `group` / `link`: `description`, non-URL `actions`, and `children` only (`data.meta` and `data.url` excluded).
- `action-list`: `rows[].badge.label`, `primary`, `subtitle`, `actions[].label`, and `actions[].value` only when `type == "copy"` and non-URL.
- `facts`: `summary`, `facts[].label/value`, `views[].label` + `views[].facts[].label/value` (non-URL values).
- `segments`: `segments[].value/role/meaning` and `explanation` (never `tone`).
- `steps`: `steps[].label/caption`, `summary`, `bullets[]`.
- `convert`: `input`, `output`, `equivalents[].label/value`.
- `table`: `columns[]` and non-URL `rows[][]` cells (never `monoColumns` or `sourceUrl`).
- `snippet`: `language`, `body`, `vars[].label`.

## 3. 4-Tier Ranking & Score Bands (`SearchHybrid` + `search.Service`)

| Band | Layer / Tier | `MatchSource` | Semantics |
| :--- | :--- | :--- | :--- |
| `150.0` | Service: Converter | `calculator` | Instant unit conversions & 3D cargo dim-weight (`TryConvertQuery`, `layout: "convert"`) |
| `145.0` / `140.0` | Service: Pattern Engine | `pattern` / `pattern-enrichment` | Regex + checksum pattern decode (`layout: "segments"`) & linked entity enrichment |
| `135.0` | Service: External MCP | `mcp-live` | Explicit `/gh` or `/ado` external MCP queries mapped onto generic `link`/`facts`/`action-list`/`table` layouts |
| `120.0` / `40.0` | Service: Plugins | `plugin` | Explicit `/modifier` plugin (`120.0`) or ambient plugin (`40.0`) |
| `88.0 – 100.0` | Store Tier 1: Exact | `exact` | B-tree match on `id`/`entity_key`/`entity_code`/`title`, plus exact `modifier`/`tag`/chip segment/`(CODE)` verified unstemmed |
| `52.0 – 78.0` | Store Tier 2b: Neighbors | `contextual` | Up to **4** 1-hop `related_to` neighbors of Tier 1 entity hits (direct `child_of` children hydrate inside the parent instead of duplicating as siblings) |
| `45.0 – 75.0` | Store Tier 2a: Whole-Token FTS | `fts` | Un-wildcarded FTS5 BM25 (`"tok"`, no `*`) |
| `18.0 – 38.0` | Store Tier 3: Header Prefix | `fts` | Prefix FTS5 (`"tok"*`) matching title/subtitle/entity_key/chips/tags/modifiers |
| `16.0 – 24.0` | Store Tier 3: Body Prefix | `fts` | Prefix FTS5 matching `body` only (kept below the `0.25 * topScore` Tier 1 cliff) |
| `5.0 – 14.0` | Store Tier 4: Guarded Fuzzy | `fuzzy` | Runs **only** when Tiers 1–3 return `< 3` hits and query has `>= 3` runes; searches metadata columns only (**never `lower(card_json) LIKE`**) |

### Critical Query Guardrails
- **Never scan `lower(card_json)`** in SQL `LIKE` queries; `searchFuzzyLocked` stays restricted to `title`, `subtitle`, `entity_key`, `chips_text`, and `tags_json` with `ESCAPE '\'`.
- **Parenthesize multi-token FTS5 column filters**: Always wrap multi-token queries in `{entity_key title chips modifiers tags} : (...)`.
- **Dynamic Relevance Cliff**: Prune results below `0.25 * topScore` when `topScore >= 80.0`, and below `0.30 * topScore` when `topScore >= 45.0`.
- **Flat Child Deduplication**: In active search results, omit standalone flat child cards already hydrated inside a returned parent; in unfiltered empty-query browse, omit flat child cards whose parent exists in the index.
