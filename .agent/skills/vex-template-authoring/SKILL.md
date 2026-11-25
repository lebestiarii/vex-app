---
name: vex-template-authoring
description: Strict, zero-alias authoring and deterministic data-mapping guide for Vex's 9 generic card layouts, flat-on-disk JSON modules (~/.vex/modules/), PatternRules with liveHttp/liveMcp (~/.vex/patterns/), and MCP validation workflows.
---

# Vex — Generic 9-Layout Authoring & Deterministic Data Mapping

Use this skill whenever creating or updating Vex modules, transforming raw CSV/TSV/JSON/CMDB sources into cards, or authoring `PatternRule` decoders (including `liveHttp` GET lookups and `liveMcp` calls).

## 1. The 4 Informational Data Types (Strict Form Rules)
Classify every field before mapping it:
1. **Glance Identifiers** (short codes like `AMS`, `EK`, `176`, currency, live IANA clock, short status/region): Place in `chips[]` (max 4). **NEVER** create a copy button for short, easily typed identifiers.
2. **High-Friction Values** (hostnames, SSH commands, full tracking/AWB/PO IDs, emails, UUIDs, converted outputs, multi-line queries): Place in `action-list`, `convert`, `snippet`, or `facts` with `"copyable": true`.
3. **Launchables** (URLs, portals, repos, tickets): 1 URL = `link` (Single-Button, no chevron). 2+ URLs = `action-list`.
4. **Comprehension Widgets** (ID breakdowns, timelines, comparison matrices): Use `segments`, `steps`, or `table`.

### Non-Negotiable Form Invariants (Enforced by `validate_module`)
- **Zero Header-to-Body Echo:** Never repeat an identical token across `title`, `chips[]`, or `facts[]` (e.g., if title is `Bahri Logistics (BAXU)` and chip 1 is `SCAC BAXU`, do not add a second `BAXU` chip). Distinct secondary codes (e.g., `UNFE` + `UNFU`) are allowed.
- **Read-Only by Default, Copyable by Exception:** In `facts`, `copyable` defaults to `false`. Set `copyable: true` ONLY for high-friction values (≥8 chars, symbols, emails, URLs, hostnames, full IDs).
- **No Empty Accordions:** Never wrap a single URL or header-only facts in an expandable card. Use `link` for 1 URL, or `group` only when child cards attach via `parentEntityKey`.
- **Zero Aliases:** Legacy names (`location`, `server-matrix`, `website-link`, `pattern-breakdown`, `conversion`, `kv-grid`, `container-blueprint`, `dictionary-guide`, `branch-contact`, `env-bookmarks`, `branch-gateway`, `reference-decoder`, `shortcut`, `knowledge`, `command`, `metric-card`) and alias fields (`servers`, `qaUrl`, `prodUrl`, `timezone`) are rejected.

## 2. The 9 Generic Layouts (Exact Zero-Alias Contracts)
All cards are stored **flat** in `ModuleManifest.cards[]`. Use `"parentEntityKey": "<key>"` to nest a card under any parent across modules (max resolved depth: 2).

| Layout (`layout`) | Choose When the User Wants To... | Strict `data` Contract |
| :--- | :--- | :--- |
| `group` | Drill into stacked child cards under one entity | `{ meta?: { ianaTimezone?, lat?, lon?, [k]: string\|number } }` *(Header-only; zero body rendered)* |
| `link` | Click once to open 1 URL (`↗`, no chevron) | `{ url: string }` |
| `action-list` | Pick among env hosts, multi-env portals, commands, or contacts | `{ rows: [{ badge?: { label, tone, dot? }, primary, mono?, subtitle?, actions: [{ label, type: "copy"\|"open_url", value, tone?, dot? }] }] }` |
| `facts` | Scan structured key/value specs (with optional unit/variant switcher) | `{ summary?, facts?: [{ label, value, mono?, copyable? }], views?: [{ label, facts: [...] }] }` |
| `segments` | Decode parts of a structured ID / code | `{ segments: [{ value, role, meaning, tone: "blue"\|"amber"\|"green" }], explanation }` |
| `steps` | Follow a numbered lifecycle, SOP, or checklist | `{ steps: [{ label, caption? }], activeStep?: number, summary, bullets?: string[] }` |
| `convert` | View & copy an `Input ⇄ Output` calculation | `{ input, output, equivalents?: [{ label, value }] }` |
| `table` | Compare 2–4 columns across ≤8 rows & export CSV/MD | `{ columns: string[], rows: string[][], monoColumns?: number[], sourceUrl? }` |
| `snippet` | Fill inline `{{vars}}` and copy a multi-line query/template | `{ language?, body: string, vars?: [{ key, label, defaultValue? }] }` |

### Smart Chips & Canonical `entityKey` Conventions
- **Smart Clock Chip:** Any chip whose `label` is a valid IANA timezone (e.g., `"Europe/Amsterdam"`) renders a live 24h clock automatically on any layout.
- **Stoplight Normalization:** `dev/sandbox` → `DEV` (`green`), `qa/uat/stage` → `QA` (`amber`), `prod/prd/live` → `PROD` (`red`).
- **Canonical `entityKey`:** `loc:<CODE>`, `svc:<slug>`, `repo:<org>/<name>`, `ado:<project>/<id>`, `air-carrier:<3-digit>`, `ocean-carrier:<SCAC>`, `equipment:<code >`, `guide:<slug>`.

## 3. Pattern Rules (`PatternRule`) & Live GET Enrichment (`liveHttp`)
- Store in `~/.vex/patterns/<id>.json` or `ModuleManifest.patterns[]`.
- Emits `layout: "segments"` by default with named regex capture groups, optional `checksum` (`iata-mod7`, `iso6346`, `luhn`), local SQLite `dataLookups`, and `linkRules`.
- Optional **`liveHttp`** (strictly read-only `GET`, `trigger: "auto"|"manual"` default `"manual"`, `timeoutMs` default `1500` / max `5000`, `cacheTtlSec` default `60`, optional non-auth `headers`): safely URL-escapes `{{0}}` / `{{match}}` / named captures (`PathEscape`/`QueryEscape`) into `urlTemplate`, authenticates strictly via `"none" | "gh_cli" | "az_devops" | "env:VAR_NAME"` (inline tokens and auth headers are rejected), and projects JSON dot-paths (`{{json.path}}`, supporting dotted keys and array indices) via `resultCard` into a nested `facts`, `table`, `steps`, or `link` child card. On timeout/error, local `segments` stay intact with a muted `Live lookup unavailable` chip.
- Optional **`liveMcp`**: delegates live lookup to a configured external MCP server (e.g., GitHub or Azure DevOps).

## 4. Deterministic Authoring Workflow (MCP)
1. **Load Schema First:** Call `get_card_schema` (`schema://search-card`).
2. **Bulk Sources (>10 rows) = ETL Rule:** Call `preview_raw_data_sample`, author an `ETLMappingRule` targeting one of the 9 layouts, and run `transform_and_import_data`. Never hand-type 50+ cards.
3. **Validate Until Zero Errors:** Call `validate_module(manifest)` (or `vex validate <file.json>`). Fix every field error before saving.
4. **Publish & Verify:** Call `upsert_module(manifest)` (or `upsert_pattern_rule`), then verify resolved `cardNode` trees and nesting depth (≤ 2) via `search_knowledge`.
