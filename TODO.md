# Vex — Phased Implementation Roadmap & TODOs

> **Design North Star**: *Less is more.* Desktop-first, platform-agnostic, local-first knowledge Rolodex with optional deletable modules, configurable pattern matching, full-width stacked nestable templates (Light/Dark mode), and live MCP authoring + agent skill sharing. Keep the Go package graph acyclic (`internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → `internal/ai` / `internal/mcp` → `main`), keep `stdout` 100% clean in `--mcp-stdio` mode, and wrap all macOS Wails window/tray/menu mutations in `application.InvokeAsync`.

---

## Standard Phase-End Verification & Documentation Gate (Required After Every Phase)
1. Run the 6-point Documentation Drift Sweep (`.agent/skills/doc-drift-sweep/SKILL.md`) and update `README.md`, `docs/ARCHITECTURE.md`, `docs/TEMPLATE_BLUEPRINTS.md`, `TODO.md`, `llms.txt`, `AGENTS.md`, `.github/copilot-instructions.md`, and `.agent/skills/*` to prevent drift.
2. **Frontend (macOS Homebrew Deno)**: `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
3. **Go Tests**: `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
4. **macOS Binary**: `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .`
5. **Windows Binary (`bin\windows\vex.exe`)**: `cd frontend; deno task typecheck; deno task test; deno task build; cd ..; go test -race -count=1 ./...; go build -o bin\windows\vex.exe .` *(verified on Windows hosts)*

---

## Completed Phases (v0.1 – v0.2)

- [x] **Phase 1 — Vex Identity & Core Architecture**: Rebased app identity to `Vex`, `~/.vex` default data dir, rotating JSONL logging + Prometheus `/metrics`, and no-login update checker.
- [x] **Phase 2 — Search, Unit Converter & Composable Overlays**: Hybrid SQLite FTS5 + Vector RRF (`k=60`), instant unit/3D cargo dimensional weight converter, and `EntityKey` cross-card synergy.
- [x] **Phase 3 — Viewport-Aware Anchoring & Cross-Platform Tray**: Pure-Go multi-monitor & DPI work-area anchoring (`window_anchor.go`), Wails v3 system tray, and Launch-at-Login (`internal/autostart`).
- [x] **Phase 4 — Local-First Privacy & OS Secret Store**: `localOnly` cloud-RAG filter (`internal/ai`) and native macOS Keychain / Windows Credential Manager secret storage (`internal/secrets`).
- [x] **Phase 5 — Declarative Pattern Matching & MCP Bridge**: RE2 pattern decoding with checksum verification, external MCP `ClientManager`, and read-only CLI auth probing (`gh` / `az`).
- [x] **Phase 6 — Desktop-First Rolodex Refactor & Angular 19 Foundation (`feat/rolodex-angular-refactor`)**:
  - [x] Stripped all CLI TUI code (`internal/tui`, `--tui`) and removed all OS-specific UI buttons/glyphs.
  - [x] Removed hardcoded test data (`testdata/`, `logistics_seed.go`, `SeedDefaultCollections`) and the `community`/`enterprise` split (`Edition`, `modules-enterprise/`).
  - [x] Made **all modules optional, deletable JSON files** in `~/.vex/modules/` with an optional embedded 6-pack starter catalog installable at onboarding or via Modules dialog + live `fsnotify` watching.
  - [x] Upgraded `PatternEngine` to be 100% loadable/configurable as a module with `enabled`, `priority`, `normalize`, `checksum` (`iata-mod7`, `iso6346`, `luhn`), live SQLite `dataLookups`, recursive cross-card `linkRules`, and `TestRule`.
  - [x] Built 6 rich optional starter modules: **Air Carrier IATA Prefix Index**, **Ocean Carrier SCAC Index**, **ISO Container Sizes & Blueprints**, **Logistics Dictionary**, **Reference Number Decoders**, and **Generic Team Templates**.
- [x] **Phase 7 — Full-Width Stacked Nested Card System, Light/Dark Mode & MCP Agent Skill Sharing (`feat/rolodex-angular-refactor`)**:
  - [x] Removed all vertical left-rail accordion tree lines; migrated `RolodexCardShellComponent` to **100% full-width `gap: 8px` vertical nesting** with depth communicated via alternating inset surfaces (L0 `#ffffff`/`#18181b`, L1 `#f8f9fb`/`#202024`, L2 `#ffffff`/`#18181b`) inside a `420px–520px` (default `460px`, `max-height: 680px`) widget frame.
  - [x] Implemented full **Light & Dark mode** (`[data-theme="light"]` / `[data-theme="dark"]`, `<meta name="color-scheme" content="light dark">`, OS preference listener + header toggle) across all components, modals, toasts, and theme-adaptive container blueprint SVGs.
  - [x] Implemented the **5 canonical generic nestable templates**:
    - `LocationCardComponent` (`location`, strictly header-only — no addresses or phones).
    - `ServerMatrixCardComponent` (`server-matrix` with `[Copy]`, `[SSH]`, and `[↗]` buttons).
    - `WebsiteLinkCardComponent` (`website-link` supporting Multi-Chip Mode + Single-Button Mode with no chevron and top-right `↗`).
    - `PatternBreakdownCardComponent` (`pattern-breakdown` with 3-column Blue/Amber/Green segment tile grid + plain-English explanation + nested entities).
    - `ConversionCardComponent` (`conversion` `Input ⇄ Output` strip + copy + equivalents, wired directly to `internal/search/converter.go`).
  - [x] Expanded MCP Hub to **19 tools** and **7 resources/templates** by adding **Agent Skill Sharing** (`list_skills`, `get_skill`, `search://skills`, `skill://{name}`) and returning projected `cardNode` trees alongside raw `card` records in `search_knowledge`.
  - [x] Completed full Documentation Drift Sweep across `README.md`, `docs/ARCHITECTURE.md`, `TODO.md`, and `.agent/skills/*`.
- [x] **Phase 8 — Angular 22 Migration, Zoneless Runtime, Vitest Suite & Cross-Agent Workflow (`feat/angular-22-migration`)**:
  - [x] Formalized Phase→Story decomposition, model handovers, and AAR workflow across `.agent/rules/vex-workflow.md`, `AGENTS.md`, and `.github/copilot-instructions.md`.
  - [x] Upgraded Angular `19.2` → `22.3.0` and TypeScript `5.8` → `6.0.3` (`rootDir: "./src"` in `tsconfig.app.json`, dynamic `esbuild@*` discovery in `build-aot.mjs`).
  - [x] Transitioned to Angular 22 default **Zoneless** runtime (removed `zone.js`, `-34kb` bundle size), modernized `RolodexCardShellComponent` (`linkedSignal`) and `PatternsDialogComponent` (signal-bound `ngModel`), and added `DestroyRef` cleanup in `RolodexStateService`.
  - [x] Added offline **Vitest 5.1.1 + `happy-dom`** frontend test harness (`deno task test` / `ng test`) with 19 passing unit & component tests.
  - [x] Completed 6-point Documentation Drift Sweep, Windows PowerShell build/run parity, and added `.agent/skills/vex-frontend-testing/SKILL.md`.

---

## Clarifications & Open Questions (Review Needed)

1. **`air-carriers.json` & `ocean-carriers.json` Tracking Links (`env-bookmarks` alias vs `website-link`)**:
   - Currently, carrier tracking child cards in `internal/indexer/starter_modules/air-carriers.json` and `ocean-carriers.json` still use `"template": "env-bookmarks"`. `RolodexCardShellComponent` aliases `env-bookmarks` → `website-link` (so single tracking links already render cleanly in Single-Button Mode with top-right `↗`).
   - **Clarification**: Should we keep `"env-bookmarks"` as a backwards-compatible alias and bulk-migrate those two starter modules to `"template": "website-link"` with top-level `data.url`, or keep `bookmarks[]` structure?
2. **Live Weather Chip Hydration on `location` Cards**:
   - `AppService.GetLiveWeather(lat, lon)` already exists in Go (Open-Meteo, no API key required).
   - **Clarification**: Should `LocationCardComponent` / `RolodexCardShellComponent` automatically hydrate a temperature chip via `VexBridgeService.getLiveWeather()` when a user's `location` card provides `data.lat`/`data.lon` (with a 15-minute client cache and graceful offline fallback)?
3. **Deprecation Scope for `branch-contact` Template**:
   - `LocationCardComponent` strictly excludes street addresses, phone numbers, and contact rows. `BranchContactCardComponent` remains registered in `CardTemplateRegistry` purely as an opt-in non-location template (e.g., for user-authored On-Call / Escalation Desk cards).
   - **Clarification**: Should `branch-contact` remain available for opt-in non-location cards, or be removed entirely in favor of `kv-grid`?
4. **Persisting Light/Dark Theme Choice in `config.json` (`UserPrefs`)**:
   - Currently the Light/Dark toggle persists in `localStorage` and tracks `prefers-color-scheme: dark` live.
   - **Clarification**: Should we also add a `theme: "system" | "light" | "dark"` field to Go `UserPrefs` (`~/.vex/config.json`) so Wails native window chrome/vibrancy can match the selected theme at startup?

---

## Upcoming Backlog
- [x] **Complete Windows Build Template (`bin\windows\vex.exe`)**: Added Windows PowerShell prerequisites (Go 1.27+, WebView2, Deno 2.x / Node.js 22.22+), test/build commands, and run instructions across `README.md`, `.agent/rules/vex-workflow.md`, `.agent/skills/vex-architecture/SKILL.md`, `AGENTS.md`, and `TODO.md` *(native Windows execution verified on Windows hosts)*.
- [ ] **Visual Module & Card Authoring Studio**: Add a form-based builder inside the Modules dialog to create `location`, `server-matrix`, `website-link`, `pattern-breakdown`, and `conversion` cards without writing raw JSON.
- [ ] **AI RAG Assistant Panel**: Wire an optional collapsible `?` RAG drawer in Angular backed by `AppService.AskRAG` & `GetAIConfig`/`SetAIConfig`.
- [ ] **Distribution & Self-Update Flow**: Host `manifest.json` and wire automated binary download + SHA-256 verification (`updater.VerifyFileSHA256`).
- [ ] **AppService Modularization**: Split `app_service.go` into focused files (`config.go`, `weather.go`, `modules_rpc.go`, `patterns_rpc.go`).
