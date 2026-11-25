# Vex — Phased Implementation Roadmap & TODOs

> **Design North Star**: *Less is more.* Desktop-first, platform-agnostic, local-first knowledge Rolodex with optional deletable modules, configurable pattern matching, 9 pure generic full-width stacked nestable layouts (Light/Dark mode), and live MCP authoring + agent skill sharing. Keep the Go package graph acyclic (`internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → `internal/mcp` → `main`), keep `stdout` 100% clean in `--mcp-stdio` mode, and wrap all macOS Wails window/tray/menu mutations in `application.InvokeAsync`.

---

## Standard Phase-End Verification & Documentation Gate (Required After Every Phase)
1. Run the 8-point Documentation Drift Sweep (`.agent/skills/doc-drift-sweep/SKILL.md`) and update `README.md`, `docs/ARCHITECTURE.md`, `docs/TEMPLATE_BLUEPRINTS.md`, `TODO.md`, `llms.txt`, `AGENTS.md`, `.github/copilot-instructions.md`, and `.agent/skills/*` to prevent drift.
2. **Frontend (macOS Homebrew Deno)**: `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
3. **Go Tests**: `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
4. **macOS Binary**: `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .`
5. **Windows Binary (`bin\windows\vex.exe`)**: `cd frontend; deno task typecheck; deno task test; deno task build; cd ..; go test -race -count=1 ./...; go build -o bin\windows\vex.exe .` *(verified on Windows hosts)*

---

## Completed Phases (v0.1 – v0.2)

- [x] **Phase 1 — Vex Identity & Core Architecture**: Rebased app identity to `Vex`, `~/.vex` default data dir, rotating JSONL logging + Prometheus `/metrics`, and no-login update checker.
- [x] **Phase 2 — Search, Unit Converter & Composable Overlays**: SQLite FTS5 search, instant unit/3D cargo dimensional weight converter, and `EntityKey` cross-card synergy.
- [x] **Phase 3 — Viewport-Aware Anchoring & Cross-Platform Tray**: Pure-Go multi-monitor & DPI work-area anchoring (`window_anchor.go`), Wails v3 system tray, and Launch-at-Login (`internal/autostart`).
- [x] **Phase 4 — Local-First Privacy Controls**: Local-first data isolation and safe URL/clipboard execution guardrails.
- [x] **Phase 5 — Declarative Pattern Matching & MCP Bridge**: RE2 pattern decoding with checksum verification, external MCP `ClientManager`, and read-only CLI auth probing (`gh` / `az`).
- [x] **Phase 6 — Desktop-First Rolodex Refactor & Angular 19 Foundation (`feat/rolodex-angular-refactor`)**: Stripped CLI TUI, removed hardcoded sample data and edition split, made all modules optional/deletable JSON files in `~/.vex/modules/`, and made `PatternEngine` 100% loadable/configurable.
- [x] **Phase 7 — Full-Width Stacked Nested Card System, Light/Dark Mode & MCP Agent Skill Sharing (`feat/rolodex-angular-refactor`)**: Removed vertical accordion tree lines in favor of 100% full-width `gap: 8px` L0→L1→L2 inset stacking, added Light/Dark tokens, and added MCP Agent Skill Sharing (`list_skills`, `get_skill`, `search://skills`, `skill://{name}`).
- [x] **Phase 8 — Angular 22 Migration, Zoneless Runtime, Vitest Suite & Cross-Agent Workflow (`feat/angular-22-migration`)**: Upgraded to Angular `22.3.0` + TypeScript `6.0.3`, removed `zone.js`, added offline Vitest 5 + `happy-dom` test harness, and formalized Phase→Story AAR workflow.
- [x] **Phase 9 — Search Precision, SQLite Schema v2 & Anti-Sprawl Ranking (`feat/search-precision`)**: Implemented 4-tier exact/contextual/prefix/guarded-fuzzy ranking, value-only `SearchableBody()` indexing, dynamic relevance cliff, and active query cap (`limit = 10`).
- [x] **Phase 10A — Simplified Vex Core, 9 Pure Generic Layouts, Strict Validator & SQLite v3 Flat-on-Disk (`feat/phase-10`)**:
  - [x] Removed embedded AI RAG & vector pipeline (`internal/ai/`, `internal/secrets/`, `card_embeddings`, `ai_prompt`, `rag_context_prompt`), keeping `internal/mcp/` intact.
  - [x] Replaced all 16 legacy templates/aliases with the 9 pure generic layouts (`CardLayoutType`, JSON field `"layout"`: `group`, `link`, `action-list`, `facts`, `segments`, `steps`, `convert`, `table`, `snippet`), strict `additionalProperties: false` validator, MCP `validate_module` tool (20 tools total), and CLI `vex validate <path.json>`.
  - [x] Upgraded SQLite atomically (`PRAGMA user_version = 3`) with `parent_entity_key`, `entity_links` dynamic 2-level cross-module child hydration, `card_recents` (`RecordCardUsed` / `ListRecentCards`), and layout-aware `SearchableBody()` whitelist.
  - [x] Migrated all 5 starter modules (`air-carriers.json`, `ocean-carriers.json`, `containers.json`, `logistics-dictionary.json`, `reference-patterns.json`) to strict flat-on-disk 9-layout schemas with zero echo chips.
- [x] **Phase 10B — Angular 22 9 Pure Layouts, Pinned+Recent, Keyboard Flow & Dialog Polish (`feat/phase-10`)**:
  - [x] Replaced all 9 legacy Angular template components with the 9 pure `BaseCardTemplate<T>` components (`GroupCardComponent`, `LinkCardComponent`, `ActionListCardComponent`, `FactsCardComponent`, `SegmentsCardComponent`, `StepsCardComponent`, `ConvertCardComponent`, `TableCardComponent`, `SnippetCardComponent`) strictly registered in `CardTemplateRegistry`.
  - [x] Upgraded `RolodexCardShellComponent` to omit chevrons on empty accordions, apply active `--chip-blue-bg`/`--chip-blue-border` pin highlights on all Level 0 cards (including `link`), render live IANA clock chips across all 9 layouts, and record usage via `VexBridgeService.recordCardUsed(id)` on expand/open/copy.
  - [x] Replaced browse-all empty state in `AppComponent` with **Pinned** (max 3) + **Recent** (last 6 via `listRecentCards()`) + subtle total indexed count (`listCollections()`), and added full Level 0 keyboard navigation (`ArrowDown`/`ArrowUp`, `Enter` primary action, two-stage `Escape`).
  - [x] Polished `ModulesDialogComponent` (full-width 3-row cards, separated Starter Catalog with `[Install]` only and zero checkboxes, modal heading focus hygiene, live MCP `● Listening`/`○ Off` status pill + `[Copy Client JSON]`) and `PatternsDialogComponent` (top Live Test bar with rendered `segments` card preview, per-rule Enabled switch, example + linked entity pills + `Live GET`/`Live MCP` badges, collapsible `Edit JSON` disclosure with inline validator errors).
  - [x] Expanded offline Vitest suite to **33 passing tests across 7 spec files** (`deno task typecheck && deno task test && deno task build`).
- [x] **Phase 10C — Declarative Pattern `liveHttp` GET Enrichment & GitHub/ADO MCP Query Flow (`feat/phase-10`)**:
  - [x] Added `PatternLiveHTTP` & `PatternLiveResultSpec` to `models.PatternRule` with strict zero-secret validation (`ValidatePatternLiveHTTP`, rejecting inline tokens, auth headers, non-HTTP(S) schemes, and embedded userinfo) and updated Draft 2020-12 `CardJSONSchema()`.
  - [x] Implemented injectable `search.LiveHTTPExecutor` + concrete `mcp.LiveHTTPClient` (`internal/mcp/live_http.go`): GET-only HTTP/HTTPS validation, safe `url.PathEscape`/`url.QueryEscape` capture interpolation, in-memory TTL response cache, cached hardcoded-arg `gh_cli`/`az_devops` & `env:VAR_NAME` auth, JSON dot-path projection (including dotted ADO keys and array indices) into nested `facts`/`table`/`steps`/`link` cards, manual `Fetch Live Data` hydration (`AppService.FetchPatternLiveHTTP`), and muted `Live lookup unavailable` fallback chip.
  - [x] Wired `/gh <query>` and `/ado <query>` external MCP queries (`internal/mcp/query_bridge.go`, `search.LiveMCPQueryProvider`) mapping repos/PRs/issues and ADO work items strictly onto generic `link`, `facts`, `action-list`, and `table` layouts, added MCP connect/test/disconnect RPCs on `AppService`, surfaced `gh`/`az` CLI auth status + 1-click GitHub/ADO MCP presets in `ModulesDialogComponent`, and added disabled-by-default `ado-work-item` & `github-issue-pr` starter patterns to `reference-patterns.json`.
  - [x] Expanded offline Vitest suite to **35 passing tests across 7 spec files** and verified full Go `-race` + macOS binary build gates.

---

## Upcoming Backlog
- [ ] **Live Weather Chip Hydration on `group` Location Cards**: Wire optional client-side caching over `AppService.FetchWeather(lat, lon)` when `data.meta.lat`/`lon` is present.
- [ ] **Persisting Light/Dark Theme Choice in `config.json` (`UserPrefs`)**: Add `theme: "system" | "light" | "dark"` to Go `UserPrefs` so Wails native window styling matches at startup.
- [ ] **Distribution & Self-Update Flow**: Host `manifest.json` and wire automated binary download + SHA-256 verification (`updater.VerifyFileSHA256`).
- [ ] **AppService Modularization**: Split `app_service.go` into focused files (`config.go`, `weather.go`, `modules_rpc.go`, `patterns_rpc.go`).
