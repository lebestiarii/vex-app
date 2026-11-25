---
name: vex-architecture
description: Reference guide for the Vex Wails v3 + Go + Angular 22 Full-Width Stacked Card Rolodex codebase, covering the Homebrew Go & Deno toolchains, sandbox-safe testing conventions, acyclic package boundaries, strict 9-layout schema & validator, SQLite v3 flat-on-disk composition, optional deletable modules, configurable pattern matching, and MCP hub & skill sharing.
---

# Vex Architecture & Conventions

## Toolchain & Platform-Specific Build Paths

### macOS (`bin/macos/vex`)
- **Go Binary**: `/opt/homebrew/Cellar/go/1.27.4/bin/go` (Go 1.27.4 darwin/arm64).
  - Always pass `-tags private_mac_apis` so Wails v3 WKWebView transparency (`MacBackdropTransparent`) renders cleanly:
    ```bash
    /opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .
    ```
  - CLI validation command: `./bin/macos/vex validate <module.json> [...]`
- **Frontend Toolchain (Deno + Angular 22 + TypeScript 6.0)**: `/opt/homebrew/Cellar/deno/2.9.8/bin/deno` (`frontend/scripts/build-aot.mjs` runs Angular `ngc` AOT + Angular Ivy Linker + dynamically discovered `esbuild@*` offline; standard `frontend/angular.json` also supports `ng serve` / `ng build` under Node.js `^22.22.3 || ^24.15.0 || >=26.0.0`).
  - **TypeScript 6.0 Note**: `frontend/tsconfig.app.json` must keep `"rootDir": "./src"` explicitly set.
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
- Leaf stdlib/OS packages: `internal/logging` · `internal/updater` · `internal/autostart`
- Core pipeline: `internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → `internal/mcp` → `main`

`internal/search` exposes interfaces (`DynamicPluginProvider`, `LivePatternExecutor`) so `plugins` and `mcp` plug into `search.Service` without circular imports.

## Build Identity (`buildinfo.go`)

Unified single-version application:
- `main.AppName` (default `"Vex"`)
- `main.AppVersion` (default `"0.2.0"`)
- `main.UpdateFeedURL` (default `""`)
- Optional 16×16 tray icon override at `assets/tray_icon.png` (embedded via `assets/*`; falls back to `AppName`).
- Default data directory is `~/.vex` (`data/`, `modules/`, `patterns/`, `plugins/`, `skills/`, `logs/`, `config.json`, `index.db`).

## Core Schemas

### 1. Strict Zero-Alias 9-Layout Schema & Validator (`internal/models/card.go`, `internal/models/layouts.go`)
- **9 Pure Generic Layouts (`CardLayoutType`, JSON field `"layout"`, zero legacy aliases)**:
  - `group` (header-only container; optional `data.meta` with primitive string/number metadata only)
  - `link` (`{ url }` single-button direct launch)
  - `action-list` (`{ rows: [{ badge?, primary, mono?, subtitle?, actions: [{ label, type: "copy"|"open_url", value, tone?, dot? }] }] }`)
  - `facts` (`{ summary?, facts?: [{ label, value, mono?, copyable? }], views?: [{ label, facts }] }`)
  - `segments` (`{ segments: [{ value, role, meaning, tone }], explanation }`)
  - `steps` (`{ steps: [{ label, caption? }], activeStep?, summary, bullets? }`)
  - `convert` (`{ input, output, equivalents? }`)
  - `table` (`{ columns, rows, monoColumns?, sourceUrl? }`, 2–4 columns)
  - `snippet` (`{ language?, body, vars?: [{ key, label, defaultValue? }] }`)
- **Action Types**: `open_url`, `open_pwa`, `launch_app`, `copy`, `http` (`ai_prompt` and `shell` are rejected).
- **Flat-on-Disk Composition & Compiler-as-Referee Validation**:
  - `EntityKey` + `ParentEntityKey` compose flat cards across separate modules at query time via SQLite `entity_links` (max resolved depth: 2).
  - `ValidateAndNormalizeCard`, `ValidateModuleManifest`, and `ValidateModuleJSON` enforce `additionalProperties: false` per layout, max 4 chips, zero duplicate chips, zero exact-token title-to-chip echo (preserving distinct secondary codes like `UNFE` + `UNFU`), `group` zero body fields, `table` 2–4 columns, and depth ≤ 2.
  - `CardJSONSchema()` exports strict Draft 2020-12 JSON Schemas for all 9 layouts, `SearchCard`, `ModuleManifest`, `PatternRule`, and `ETLMappingRule`.

### 2. SQLite v3 Indexer, Optional Starter Modules & Live Watcher (`internal/indexer/`)
- **Store (`store.go`)**: Embedded CGo-free SQLite (`modernc.org/sqlite`, `SetMaxOpenConns(1)`, `PRAGMA user_version = 3`) with `cards` (`layout`, `entity_key`, `entity_code`, `parent_entity_key`, `chips_text`, `related_keys_json`), 9-column `cards_fts`, `entity_links(source_card_id, target_entity_key, relation_type)`, and `card_recents(card_id, last_used_at)` (`RecordCardUsed` / `ListRecentCards`). See `.agent/skills/vex-search-ranking/SKILL.md`.
- **Optional Deletable Modules (`modules.go`, `starter_modules/*.json`)**:
  - **Zero forced hardcoded modules**: `LoadAllModules(modulesDir)` reads ONLY `~/.vex/modules/*.json`.
  - **5 Optional Embedded Starter Packs (`//go:embed starter_modules/*.json`)**: `air-carriers`, `ocean-carriers`, `containers`, `logistics-dictionary`, `reference-patterns` — all flat-on-disk strict 9-layout manifests passing `ValidateModuleJSON`.
- **Watcher (`watcher.go`)**: Debounced `fsnotify` watcher monitoring `dataDir`, `modulesDir`, and `patternsDir`.
- **ETL (`etl.go`)**: `SampleRawData`, `TransformWithRule` (emitting strict 9-layout payloads), `SaveCollectionFile`, `UpsertCardToDisk`, `BatchUpsertCardsToDisk`.

### 3. Query Parser, Instant Unit Converter & Configurable Pattern Engine (`internal/search/`)
- `TryConvertQuery(rawText)` (`converter.go`): Zero-latency unit converter and 3D cargo volume + IATA 1:167 air dimensional weight calculator emitting `layout: "convert"`.
- `PatternEngine` (`patterns.go`, `models.PatternRule`, `indexer.LoadActivePatternRules`):
  - **Zero hardcoded pattern rules** — loads from enabled modules and `~/.vex/patterns/*.json`.
  - Defaults to `layout: "segments"` with interpolated breakdown tiles and explanation, supports `fact.<Label>` / `meta.<key>` `dataLookups`, recursive `linkRules` (consuming `entity_links`-hydrated children), optional `liveMcp` (attached as a nested `facts` child card), and `TestRule`.

### 4. Declarative Plugins & Action Runner (`internal/plugins/`)
- `plugins.Runner` executes safe actions (`open_url`, `open_pwa`, `launch_app`, `copy`, `http`) without arbitrary shell execution.

### 5. Tri-Modal MCP Hub, External MCP Clients & Skill Sharing (`internal/mcp/`, `internal/logging/`)
- **Domain MCP Server (`server.go` — `HubServer`, `mcp-go v1.6.0`)**:
  - Transports: `--mcp-stdio` (`WithWorkerPoolSize(1)`) and Streamable HTTP (`127.0.0.1:17380/mcp`, `/health`, `/metrics`, protected by `loopbackGuardMiddleware`).
  - **Resources & Templates (7)**: `search://collections`, `schema://search-card`, `card://{collection}/{id}`, `search://modules`, `search://patterns`, `search://skills`, `skill://{name}`.
  - **Tools (20)**: `search_knowledge`, `get_card_schema`, `preview_raw_data_sample`, `transform_and_import_data`, `upsert_card`, `batch_upsert_cards`, `list_plugins`, `execute_card_action`, `get_related_cards`, `upsert_pattern_rule`, `list_pattern_rules`, `delete_pattern_rule`, `list_modules`, `get_module`, `validate_module`, `upsert_module`, `delete_module`, `install_starter_module`, `list_skills`, `get_skill`.
- **External MCP Client (`client.go`)** & read-only CLI auth probe (`auth_probe.go`: `ProbeCLIAuth`, `RecommendedMCPPresets`).

### 6. Wails v3 Desktop Shell & Angular 22 UI (`main.go`, `app_service.go`, `frontend/`)
- **Default Widget Frame**: `480×680` (`MinWidth: 420, MinHeight: 480`) hosting a `420px–520px` (default `460px`, `max-height: 680px`) internal-scrolling Rolodex widget with Light & Dark modes.
- **Wails Recents RPC**: `AppService.RecordCardUsed(cardID)` and `AppService.ListRecentCards(limit)`.
- **macOS Main-Thread Rule**: Every Wails UI/window/menu call inside `GlobalShortcut` or background callbacks MUST be wrapped in `application.InvokeAsync(func() { ... })`.
