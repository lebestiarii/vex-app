---
name: vex-architecture
description: Reference guide for the Vex Wails v3 + Go + Angular 22 Full-Width Stacked Card Rolodex codebase, covering the Homebrew Go & Deno toolchains, sandbox-safe testing conventions, acyclic package boundaries, optional deletable modules, configurable pattern matching, and MCP schemas & skill sharing.
---

# Vex Architecture & Conventions

## Toolchain & Platform-Specific Build Paths

### macOS (`bin/macos/vex`)
- **Go Binary**: `/opt/homebrew/Cellar/go/1.27.4/bin/go` (Go 1.27.4 darwin/arm64).
- **Frontend Toolchain (Deno + Angular 22 + TypeScript 6.0)**: `/opt/homebrew/Cellar/deno/2.9.8/bin/deno` (`frontend/scripts/build-aot.mjs` runs Angular's official `ngc` AOT compiler + Angular Ivy Linker + dynamically discovered `esbuild@*` offline; standard `frontend/angular.json` also supports `ng serve` / `ng build` under Node.js `^22.22.3 || ^24.15.0 || >=26.0.0`).
  - **TypeScript 6.0 Note**: `frontend/tsconfig.app.json` must keep `"rootDir": "./src"` explicitly set so `ngc` emit (`TS5011`) roots `out-tsc/app/main.js` cleanly.
- **Full macOS Regression & Build Gate** (runs 100% offline inside the sandbox):
  ```bash
  cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build
  /opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...
  /opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .
  ```

### Windows (`bin\windows\vex.exe`)
*(Native Windows binary execution and WebView2 rendering are verified on Windows 10/11 hosts.)*
- **Prerequisites**: Go `1.27+`, built-in Microsoft Edge WebView2, and Deno `2.x` (or Node.js `22.22+` / Angular CLI `22`).
- **Build & Test (PowerShell)**:
  ```powershell
  cd frontend; deno task typecheck; deno task test; deno task build; cd ..
  go test -race -count=1 ./...
  go build -o bin\windows\vex.exe .
  ```

- **HTTP & MCP Mocking Rules**: Never bind loopback TCP ports (`httptest.NewServer`) in unit tests when avoidable — inject an in-memory `http.RoundTripper`, `client.NewInProcessClient(hub.MCPServer())`, and `httptest.NewRecorder()`.

## Acyclic Package Dependency Graph

Keep imports strictly acyclic in this order:
- Leaf stdlib/OS packages: `internal/logging` · `internal/updater` · `internal/secrets` · `internal/autostart`
- Core pipeline: `internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → (`internal/ai`, `internal/mcp`) → `main`

`internal/search` exposes interfaces (`DynamicPluginProvider`, `EmbedderFunc`, `LivePatternExecutor`) so `plugins`, `mcp`, and `ai` plug into `search.Service` without circular imports.

## Build Identity (`buildinfo.go`)

Unified single-version application (no community/enterprise split):
- `main.AppName` (default `"Vex"`)
- `main.AppVersion` (default `"0.2.0"`)
- `main.UpdateFeedURL` (default `""`)
- Optional 16×16 tray icon override at `assets/tray_icon.png` (embedded via `assets/*`; falls back to `AppName`).
- Default data directory is `~/.vex` (`data/`, `modules/`, `patterns/`, `plugins/`, `skills/`, `logs/`, `config.json`, `index.db`).

## Core Schemas

### 1. `models.SearchCard`, `models.CardNode` & `models.ModuleManifest` (`internal/models/card.go`)
- **5 Canonical Generic Templates (`CardTemplateType`)**:
  - `location` (header-only; NO addresses/phones; expands directly into full-width nested children)
  - `server-matrix` (3 stoplight chips `DEV`/`QA`/`PROD`; `[Copy]`, `[SSH]`, `[↗]` buttons)
  - `website-link` (Multi-Chip Mode for 2+ links; Single-Button Mode for 1 link omits chevron and shows top-right `↗`)
  - `pattern-breakdown` (3-column Blue/Amber/Green segment tile grid + plain-English explanation + nested entities)
  - `conversion` (`Input ⇄ Output` strip + 1-click copy + equivalent chips)
- **Extended & Alias Templates**: `container-blueprint`, `dictionary-guide`, `kv-grid`, `shortcut`, `knowledge`, `command`, `metric-card`, plus backwards-compatible aliases (`branch-gateway` → `location`, `env-bookmarks` → `website-link`, `reference-decoder` → `pattern-breakdown`, `branch-contact`).
- **Action Types**: `open_url`, `open_pwa`, `launch_app`, `copy`, `http`, `ai_prompt`.
- **Recursive Nesting & Header-Strip Fields**:
  - `Chips` (`[]GlanceChip{Label, Tone, Dot}`): Typed Row-2 header chips (`neutral` | `blue` | `amber` | `green` | `red` | `mono`, optional stoplight `dot`: `green` | `amber` | `red`).
  - `Children` (`[]SearchCard`): Recursive nested child cards projected up to depth 2 via `SearchCard.ToCardNode(depth)`.
  - `DefaultExpanded` (`bool`) & `Pinnable` (`*bool`, true for Level 0, false for nested children).
  - `EntityKey` (e.g. `"loc:AMS"`, `"air-carrier:176"`, `"ocean-carrier:MAEU"`, `"guide:hbl"`): Canonical cross-module linking key.
- **Validation & Schema**: Always call `models.ValidateAndNormalizeCard(&card, defaultCollection)` before indexing or writing to disk. `models.CardJSONSchema()` exports the canonical schema for `SearchCard`, `GlanceChip`, `PatternSegment`, `ModuleManifest`, `PatternRule`, and `ETLMappingRule`.

### 2. Hybrid Indexer, Optional Starter Modules & Live Watcher (`internal/indexer/`)
- **Store (`store.go`)**: Embedded CGo-free SQLite (`modernc.org/sqlite`, `SetMaxOpenConns(1)`, `PRAGMA user_version = 2`) with indexed `entity_key`/`entity_code`/`chips_text`/`related_keys_json` columns, 9-column `cards_fts`, `card_embeddings`, 4-tier exact/contextual/prefix/guarded-fuzzy ranking + dynamic relevance cliff, and hybrid vector scoring (see `.agent/skills/vex-search-ranking/SKILL.md`).
- **Optional Deletable Modules (`modules.go`, `starter_modules/*.json`)**:
  - **Zero forced hardcoded modules**: `LoadAllModules(modulesDir)` reads ONLY `~/.vex/modules/*.json`. Deleting a module (`DeleteModule` or removing the file on disk) removes it permanently.
  - **Optional Embedded Starter Catalog (`//go:embed starter_modules/*.json`)**: 6 real/generic optional packs (`air-carriers`, `ocean-carriers`, `containers`, `logistics-dictionary`, `reference-patterns`, `team-templates` — zero sample/dummy data) exposed via `ListStarterModules` / `InstallStarterModules` for onboarding or Module Manager installation.
  - `CompileActiveModules` merges base cards, overlay sections, and overlay `ChildCards` under `source_file = "module://compiled"`.
- **Watcher (`watcher.go`)**: Debounced `fsnotify` watcher monitoring `dataDir`, `modulesDir` (`SetModulesWatcher`), and `patternsDir` (`SetPatternsWatcher`) for live hot-reindexing.
- **ETL (`etl.go`)**: `SampleRawData`, `TransformWithRule`, `SaveCollectionFile`, `UpsertCardToDisk`, `BatchUpsertCardsToDisk`, and clean `EnsureDataDir` (no hardcoded demo/test data).

### 3. Query Parser, Instant Unit Converter & Configurable Pattern Engine (`internal/search/`)
- `TryConvertQuery(rawText)` (`converter.go`): Zero-latency unit converter and 3D cargo volume + IATA 1:167 air dimensional weight calculator emitting `template: "conversion"` (`data.input`, `data.output`, `data.equivalents[]`).
- `PatternEngine` (`patterns.go`, `models.PatternRule`, `indexer.LoadActivePatternRules`):
  - **Zero hardcoded pattern rules** — all rules load from enabled modules (`ModuleManifest.Patterns`) and `~/.vex/patterns/*.json`.
  - Defaults to `template: "pattern-breakdown"` with interpolated 3-column `segments[]` (`PatternSegmentSpec`) and `explanationTemplate`.
  - Supports `enabled`, `priority`, `normalize`, checksums (`iata-mod7`, `iso6346`, `luhn`), SQLite `dataLookups`, recursive `linkRules` (`targetEntityKey`, `targetCollection`, `targetTag`, `autoExpand`, `nestChildren`, `copyChildrenOnly`, `attachUnder`, `maxDepth`), optional `liveMcp` lookup (`1500ms` timeout), and `TestRule` dry-run testing.

### 4. Declarative Plugins, Action Runner & AI RAG (`internal/plugins/`, `internal/ai/`)
- `plugins.Runner` executes safe actions (`open_url`, `open_pwa`, `launch_app`, `copy`, `http`, `ai_prompt`) without arbitrary shell execution.
- `ai.Service` supports `ollama`, `openai`, `anthropic`, and `gemini`, enforcing `localOnly` stripping on cloud LLM calls and keeping API keys in `internal/secrets` (macOS Keychain / Windows Credential Manager).

### 5. Tri-Modal MCP Hub, Agent Skill Sharing & Observability (`internal/mcp/`, `internal/logging/`)
- **Domain MCP Server (`server.go` — `HubServer`, `mcp-go v1.6.0`)**:
  - Transports: `--mcp-stdio` (`WithWorkerPoolSize(1)`) and Streamable HTTP (`127.0.0.1:17380/mcp`, `/health`, `/metrics`, protected by `loopbackGuardMiddleware`).
  - **Resources & Templates (7)**: `search://collections`, `schema://search-card`, `card://{collection}/{id}`, `search://modules`, `search://patterns`, `search://skills`, `skill://{name}`.
  - **Tools (19)**: `search_knowledge` (returns both `card` and projected `cardNode`), `get_card_schema`, `preview_raw_data_sample`, `transform_and_import_data`, `upsert_card`, `batch_upsert_cards`, `list_plugins`, `execute_card_action`, `get_related_cards`, `upsert_pattern_rule`, `list_pattern_rules`, `delete_pattern_rule`, `list_modules`, `get_module`, `upsert_module`, `delete_module`, `install_starter_module`, `list_skills`, `get_skill`.
  - **Prompt (1)**: `rag_context_prompt`.
  - **Agent Skill Sharing**: `ServerDeps.SkillsDirs` scans `~/.vex/skills/` and workspace `.agent/skills/` for `<name>/SKILL.md` files so external AI agents can load Vex UI and template-authoring guidelines directly over MCP.
- **External MCP Client (`client.go`)** & read-only CLI auth probe (`auth_probe.go`: `ProbeCLIAuth`, `RecommendedMCPPresets`).

### 6. Wails v3 Desktop Shell & Angular 22 Full-Width Stacked Card UI (`main.go`, `app_service.go`, `frontend/`)
- **Default Widget Frame**: `480×680` (`MinWidth: 420, MinHeight: 480`) hosting a `420px–520px` (default `460px`, `max-height: 680px`) internal-scrolling Rolodex widget with Light & Dark modes (`[data-theme="light"]` / `[data-theme="dark"]`).
- **Frontend ↔ Go Bridge**: `VexBridgeService` (`frontend/src/app/services/vex-bridge.service.ts`) dynamically imports `/wails/runtime.js`, calls `rt.Call.ByName("main.AppService.<Method>", ...args)`, and returns clean empty state (zero hardcoded demo data) when opened outside Wails.
- **macOS Main-Thread Rule**: Every Wails UI/window/menu call inside `GlobalShortcut` or background callbacks MUST be wrapped in `application.InvokeAsync(func() { ... })`.
- See `.agent/skills/vex-rolodex-ui/SKILL.md` and `.agent/skills/vex-template-authoring/SKILL.md` for Light/Dark design tokens, full-width stacked nesting rules, and generic template contracts.
