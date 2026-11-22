---
name: vex-architecture
description: Reference guide for the Vex Wails v3 + Go + React codebase, covering the Homebrew Go toolchain, sandbox-safe testing conventions, package boundaries, build-time Editions, and canonical SearchCard, ETLMappingRule, PluginManifest, and ParsedQuery schemas.
---

# Vex Architecture & Conventions

## Toolchain & Regression Commands

- **Go Binary**: `/opt/homebrew/Cellar/go/1.27.4/bin/go` (Go 1.27.4 darwin/arm64, installed via Homebrew Cellar).
- **Frontend Toolchain (Deno)**: `/opt/homebrew/Cellar/deno/2.9.8/bin/deno` (`node`/`npm` are not installed; use Deno tasks in `frontend/`).
- **Full Regression & Build Gate** (runs 100% offline inside the sandbox):
  ```bash
  cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build
  /opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...
  /opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/vex .
  ```
- **HTTP & MCP Mocking Rules**: Never bind loopback TCP ports (`httptest.NewServer`) in unit tests when avoidable — the sandbox blocks local loopback connects (`operation not permitted`). Instead:
  - Inject an in-memory `http.RoundTripper` on `http.Client.Transport` for HTTP/LLM tests.
  - Use `client.NewInProcessClient(hub.MCPServer())` and `httptest.NewRecorder()` for full MCP JSON-RPC and Streamable HTTP integration tests.

## Acyclic Package Dependency Graph

Keep imports strictly acyclic in this order:
`internal/logging` (stdlib only) · `internal/updater` (stdlib only) · `internal/secrets` (stdlib/OS only) · `internal/autostart` (stdlib/OS only)
`internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → (`internal/ai`, `internal/mcp`, `internal/tui`) → `main`

- `internal/search` exposes interfaces (`DynamicPluginProvider`, `EmbedderFunc`) so `plugins`, `mcp`, and `ai` plug into `search.Service` without circular imports.

## Build-Time Editions (`buildinfo.go`)

- `main.AppName` (default `"Vex"`, overridable via `-ldflags` if needed)
- `main.Edition` (default `"community"`)
- `main.AppVersion` (default `"0.1.0"`)
- `main.UpdateFeedURL` (default `""`, fallback when `config.json` has no `updateFeedUrl`)
- Optional 16×16 tray icon override at `assets/tray_icon.png` (embedded via `assets/*`; falls back to text label `AppName`).
- Default data directory is `~/.vex`.

## Core Schemas

### 1. `models.SearchCard` & Composable Modules (`internal/models/card.go`)
- **Templates (`CardTemplateType`)**: `shortcut`, `knowledge`, `command`, `kv-grid`, `metric-card`.
- **Action Types**: `open_url`, `open_pwa`, `launch_app`, `copy`, `http`, `ai_prompt`.
- **Composable Fields on `SearchCard`**:
  - `EntityKey` (e.g., `"loc:AMS"`, `"uld:AKE"`): Canonical cross-module entity identifier.
  - `ModuleID`: Originating module ID.
  - `Sections` (`[]CardSection{ModuleID, ModuleName, Title, Badge, Notes, Fields, Actions, Provenance}`): Attributed, isolated module perspectives merged onto a base card (indexed into `SearchableBody()` and executable via `Runner.Execute`).
  - `GlancePills` (`[]string`): Up to 3 high-signal summary badges auto-computed by `models.ComputeGlancePills(card)` for the small card row.
  - `Provenance` (`*Provenance{SourceName, SourceURL, VerifiedAt, Authority, LocalOnly}`): Source attribution & freshness metadata surfaced in the UI footer and injected into AI RAG citations (`formatCardForRAG`).
  - `LocalOnly` (`bool`): Present on `ModuleManifest`, `ModuleStatus`, `SearchCard`, `CardSection`, and `Provenance`. Propagated by `CompileActiveModules` and enforced by `filterCardForProvider` in `internal/ai/service.go` (excluded from cloud LLM RAG/embeddings while retained for local search and local `ollama`).
  - `RelatedKeys` (`[]string`): Cross-card synergy keys used by `Store.FindRelatedCards(ctx, cardID, limit)`.
- **Module Schema (`ModuleManifest` & `CardOverlay`)**:
  - `ModuleManifest{ID, Name, Category, Description, Icon, Version, DefaultEnabled, Cards, Overlays}`.
  - `CardOverlay{TargetEntityKey, TargetCardID, FallbackTitle, FallbackCol, Section, AddTags, AddModifiers, AddRelatedKeys}`: Enriches an active base card matching `TargetEntityKey`/`TargetCardID`, or automatically promotes to a standalone card when the base module is disabled.
- **Validation**: Always call `models.ValidateAndNormalizeCard(&card, defaultCollection)` before indexing or writing to disk.
- **Schema Export**: `models.CardJSONSchema()` returns the formal JSON Schema (used by MCP `schema://search-card` and AI ETL schema inference).

### 2. Hybrid Indexer, Modules & ETL (`internal/indexer/`)
- **Store (`store.go`)**: Embedded CGo-free SQLite (`modernc.org/sqlite`, `SetMaxOpenConns(1)`) with `cards`, `cards_fts` (FTS5 weighted BM25), and `card_embeddings` (normalized `float32` blobs + Cosine Similarity) merged via Reciprocal Rank Fusion (RRF, `k=60`).
  - `ReplaceSourceFile(ctx, sourceFile, cards)` atomically replaces all cards for a given `source_file` (e.g., `"module://compiled"`) across collections without touching user-created JSON files in `data/`.
  - `FindRelatedCards(ctx, cardID, limit)` scores cross-card synergy via `EntityKey`, `RelatedKeys`, and domain tags.
- **Composable Module Compiler (`modules.go` & `github_catalog.go`)**:
  - `BuiltInModules()` provides 7 toggleable modules (`geographies`, `air-freight`, `ocean-freight`, `customs`, `transcon`, `global-tech`, `github-catalog`).
  - `ParseRepoTaxonomy(repoName)` & `BuildGitHubCatalogModule(repos)` parse `<prefix>-<portfolio>-<team>-<product>-<component>` repositories into individual repo cards + Portfolio/Team roll-up directory cards (importable via `AppService.ImportGitHubReposJSON`).
  - `LoadAllModules(modulesDir)` merges built-in modules with shared custom `.json` modules in `<baseDir>/modules/*.json`.
  - `CompileActiveModules(all, enabledIDs)` and `SyncModules(ctx, store, modulesDir, enabledIDs)` compile enabled base cards + overlays into SQLite under `source_file = "module://compiled"`.
- **ETL (`etl.go`)**:
  - `SampleRawData(raw, filenameHint)` — strips UTF-8 BOM, auto-detects `.csv`/`.tsv`/`.json`/`.jsonl` and nested JSON array `DetectedRootPath`, returns fields + up to 5 sample rows.
  - `TransformWithRule(raw, rule)` — applies `models.ETLMappingRule` with `{{Field}}` interpolation and dot-path lookup.
  - `SaveCollectionFile` / `UpsertCardToDisk` / `BatchUpsertCardsToDisk` — atomic `.tmp` + rename writes to `<dataDir>/<collection>.json`.
- **Watcher (`watcher.go`)**: Debounced `fsnotify` watcher keeping SQLite synchronized with JSON files on disk.

### 3. Query Parser, Instant Unit Converter, Pattern Engine & Search (`internal/search/`)
- `TryConvertQuery(rawText)` (`converter.go`): Zero-latency (<0.1ms) unit converter (`1588 kg to lbs`, `-30c to f`, `4.3 cbm to ft3`) and 3D cargo volume + IATA 1:167 air dimensional weight calculator (`120x80x100 cm`, `2 x 120x80x110 cm`), prepended as synthetic card `builtin-unit-convert`.
- `PatternEngine` (`patterns.go`, `models.PatternRule`, `indexer.LoadActivePatternRules`): Declarative RE2 reference-number decoder & cross-card enricher loaded from `~/.vex/patterns/*.json`, `ModuleManifest.Patterns`, and built-in public standards (`iata-awb` with Mod-7, `iso-container` with ISO 6346 Mod-11, `ado-work-item`, `github-issue`). Supports `LinkEntityKeys` (`Store.FindCardsByEntityKeys`) and optional `LiveMCP` tool lookup via the acyclic `LivePatternExecutor` interface (`internal/mcp.ClientManager.ExecutePatternMCP`, 1500ms timeout, `LocalOnly: true`).
- Enterprise CLI Auth Probe (`internal/mcp/auth_probe.go`): `ProbeCLIAuth` runs read-only identity checks (`gh auth status`, `az account show`) without ever requesting tokens; `EnterpriseMCPPresets()` provides opt-in `LocalOnly: true` presets for official GitHub & Azure DevOps MCP servers (see `docs/ADR-001-enterprise-auth-and-patterns.md`).
- `Parse(raw)` supports:
  - `/modifier` (first single-segment `/token`)
  - `@collection` (e.g., `@devops`, `@mcp:github`)
  - `#tag` (repeatable, deduplicated)
  - `type:<template>` / `tpl:<template>`
  - `key:value` and `key:"quoted value"` (URLs containing `://` are ignored)
  - `? question` (`ModeAI`, still supports `@collection` and `#tag` scoping)

### 4. Declarative Plugins & Action Runner (`internal/plugins/`)
- Manifests live at `<pluginsDir>/<id>/plugin.json` (`PluginManifest` with `Modifiers`, `StaticCards`, and optional `DynamicSource` of type `http`).
- `InterpolateTemplate(tmpl, card, parsed, extraArgs)` resolves `{{query}}`, `{{raw}}`, `{{arg1}}`..`{{argN}}`, `{{param}}`, `{{data.key}}`, and `{{card.title}}`.
- `Runner.Execute(...)` runs actions cross-platform and parses JSON stdout/HTTP responses into `ActionResult.Data`.

### 5. Multi-Provider AI RAG & Schema Mapper (`internal/ai/service.go`)
- `Config` (`provider`: `ollama` | `openai` | `anthropic` | `gemini`, `baseUrl`, `apiKey`, `chatModel`, `embeddingModel`, `temperature`, `maxContextCards`), live-updatable via `GetConfig()` / `SetConfig(cfg)`.
- `Embed(ctx, text)` directly satisfies `search.EmbedderFunc`; `SyncMissingEmbeddings(ctx)` embeds cards whose `ContentHash()` is missing/stale.
- `AskRAG(ctx, RAGRequest{Query, Collection, Tags, Limit}, onChunk)` retrieves top-K hybrid cards, formats numbered `[1]..[N]` citations, streams chunks via `onChunk`, and returns `RAGResponse{Answer, Sources, Model, Provider, DurationMs}`.
- `GenerateETLMapping(ctx, sample, userHint)` & `GenerateETLMappingWithPreview(ctx, rawBytes, filenameHint, userHint)` infer a validated `models.ETLMappingRule` (stripping markdown fences and inheriting `DetectedRootPath`) plus preview `SearchCard`s.

### 6. Tri-Modal MCP Hub & Observability (`internal/mcp/`, `internal/logging/`)
- **Domain MCP Server (`server.go` — `HubServer`, `mcp-go v1.6.0`)**:
  - Transports: `ServeStdio()` (`--mcp-stdio`) and `StartHTTP("127.0.0.1:17380")` / `HTTPHandler()` (Streamable HTTP at `/mcp`, `/health`, and Prometheus `/metrics`, protected by `loopbackGuardMiddleware` against DNS rebinding & CSRF).
  - Resources & Templates: `search://collections`, `schema://search-card`, `card://{collection}/{id}`.
  - Tools (10): `search_knowledge`, `get_card_schema`, `preview_raw_data_sample`, `transform_and_import_data`, `upsert_card`, `batch_upsert_cards`, `list_plugins`, `execute_card_action`, `get_related_cards`, `upsert_pattern_rule` (validates regex, writes `~/.vex/patterns/<id>.json`, hot-reloads `PatternEngine`).
  - Prompt: `rag_context_prompt(topic, collection, limit)`.
- **Local Structured Logging (`internal/logging/logger.go`)**:
  - Zero-dependency `log/slog` JSONL writer at `<baseDir>/logs/vex.jsonl` (`0o600`, 5 MB rotation, 3 backups) + atomic Prometheus counters (`vex_*`) exposed at `/metrics`. Never writes to `stdout`.
- **No-Login Update Checker (`internal/updater/updater.go`)**:
  - Fetches a `manifest.json` from an HTTP/HTTPS URL or shared network/local file path (no login required, 3s timeout), compares semver against `AppVersion`, and supports SHA-256 verification.
- **External MCP Client Bridge (`client.go` — `ClientManager`)**:
  - Transports: `stdio`, `sse`, `http` (StreamableHTTP), and `inprocess` (`AttachInProcess`).
  - Indexes discovered remote tools & resources into SQLite as virtual `SearchCard`s under collection `mcp-<id>` (`sourceFile = "mcp://<id>"`), queryable via `@mcp:<id>` and `/mcp`, with `CallTool`, `ReadResource`, and `Disconnect`.
- **Headless `--mcp-stdio` Safety**: Always pass `server.WithWorkerPoolSize(1)` to `server.ServeStdio` so rapid piped JSON-RPC batches finish cleanly before EOF, and wrap live-search `Embed` calls with a `1500ms` timeout context so offline embedders never stall search.

### 7. Wails v3.1.0 Desktop Shell, Secret Store, TUI & UX Conventions (`main.go`, `app_service.go`, `window_anchor.go`, `internal/secrets/`, `internal/tui/`)
- **Frontend ↔ Go Bridge**: Wails v3 does **not** expose `window.wails.Call.ByName` by default. Import the Wails-served ES module `import('/wails/runtime.js')` dynamically and call `rt.Call.ByName("main.AppService.<Method>", ...args)` and `rt.Events.On(name, cb)` (falling back gracefully in standalone Vite dev mode).
- **Transparent Floating Windows & Viewport-Aware Upward Flip**:
  - Set `BackgroundType: application.BackgroundTypeTransparent`, `BackgroundColour: application.NewRGBA(0, 0, 0, 0)`, `Mac: application.MacWindow{Backdrop: application.MacBackdropTransparent, DisableShadow: true}`, and `Windows: application.WindowsWindow{DisableFramelessWindowDecorations: true, HiddenOnTaskbar: true}`, and build with **`-tags private_mac_apis`** on macOS.
  - `window_anchor.go` computes work-area placement and `AnchorDirection` (`"down"` or `"up"` when spawning from the bottom-right tray or bottom 25–30% of the active display's work area), exposed via `GetAnchorDirection()` and the `vex:anchor` event so `frontend/src/App.tsx` flips to `flex-col-reverse`.
- **macOS Main-Thread Rule**: `app.GlobalShortcut.Register(...)` dispatches callbacks on a **background goroutine**. Every Wails UI/window/menu call (`launcherWin.IsVisible/Center/Show/Focus/Hide`, `menuItem.SetLabel`) MUST be wrapped in `application.InvokeAsync(func() { ... })`. Never call `app.Show()` directly off the main thread.
- **Native OS Secret Store (`internal/secrets/`)**:
  - `Store` interface (`Get`, `Set`, `Delete`) backed by macOS Keychain (`/usr/bin/security` in `store_darwin.go`), pure-Go Windows Credential Manager (`advapi32.dll` in `store_windows.go`), and `MemoryStore` for sandbox-safe unit tests (`InitBackendWithSecrets`).
  - `ai.Config.APIKey` is stored under `service="vex"`, `account="ai_api_key"` and scrubbed (`scrubConfigSecrets`) before writing `config.json`, with automatic startup migration of legacy plaintext keys.
- **Interactive Bubble Tea TUI (`internal/tui/`, `--tui`)**:
  - Invoked via `./bin/vex --tui`, directly reusing `search.Service`, `indexer.Store`, and `plugins.Runner` with `1–4` (`Alt+1–5`) category filters, Primary Hero Card + `Tab`/`Shift+Tab` perspective tabs, compact runner-ups (`↑`/`↓` + `Enter` promote/run), unit/volumetric math, and staged `Esc`.
- **User Preferences (`RuntimeConfig.Prefs` in `config.json`)**:
  - `UserPrefs{OnboardingCompleted, PowerUserMode, AIEnabled, LaunchAtLogin, LayoutMode ("compact"|"split"), SelectedGoals, EnabledModules, PinnedCategories}` exposed via `GetUserPrefs()` / `SetUserPrefs(prefs)`, plus `GetLaunchAtLogin()` / `SetLaunchAtLogin(bool)` (backed by `internal/autostart` macOS LaunchAgent / Windows `HKCU\...\Run` and synced with the tray checkbox), `GetAppInfo()`, `GetAnchorDirection()`, `ListModules()`, `SetEnabledModules(ids)`, `ImportModuleJSON(raw)`, `ExportModuleJSON(id)`, and `GetRelatedCards(cardID, limit)`.
  - On first run (`!cfg.Prefs.OnboardingCompleted`), set `Hidden: false` on `launcherWin` so the 3-step `OnboardingWizard` opens automatically.

## Phase Protocol & Workflow Loop
1. **Git Branching**: Work on a dedicated feature branch (`feat/<topic>`), never push to a remote unless explicitly asked, and commit + merge back to `main` when the user approves a phase.
2. **Verification Gate**: At the end of every phase, run:
   - `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
   - `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
   - `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/vex .`
3. **Mandatory Phase-End Status Report**: Always report Testing Gate results, **What Was Done**, **What Went Well**, **What Was Added / Modified** (with clickable `file://` links), **Skill / Rule Recommendations**, and **Context & Model Recommendations**.
