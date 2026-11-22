# Vex — Phased Implementation Roadmap & TODOs

> **Design North Star**: *Less is more.* Fast, calm, local-first search launcher and composable knowledge hub. No UI taglines, catchphrases, or marketing copy. Keep the Go package graph acyclic (`internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → `internal/ai` / `internal/mcp` / `internal/tui` → `main`), keep `stdout` 100% clean in `--mcp-stdio` mode, and wrap all macOS Wails window/tray/menu mutations in `application.InvokeAsync`.

---

## Execution & Model Routing Plan

| Phase | Scope | Assigned Model |
| :--- | :--- | :--- |
| **Phase 1** | Vex Rebase, Build-Time Editions & Doc Drift Sweep | **Gemini 4 Hydrogen** (Medium) |
| **Phase 2** | Hero Card + Compact Runner-Ups & Pinned `⌘1–⌘5` Categories | **Gemini 4 Neon** (Medium-High) |
| **Phase 3** | Viewport-Aware Upward Flip & Windows DPI/WebView2 Anchoring | **Gemini 4 Neon** (High) |
| **Phase 4** | Minimal Enterprise Guardrails (`localOnly` RAG & OS Secret Store) | **Gemini 4 Hydrogen** (Medium) |
| **Phase 5** | Lightweight Interactive TUI (`--tui` via Bubble Tea) | **Gemini 4 Argon** (High) |

### Standard Phase-End Verification Gate (Required for Every Phase)
1. `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
2. `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
3. `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/vex .`

---

## Phase 1: Vex Rebase, Build-Time Editions & Doc Drift Sweep
**Owner Model**: Gemini 4 Hydrogen (Medium reasoning) · **Branch**: `feat/vex-rebase-editions`

### Tasks
- [x] **Rebase Identity to Vex (Repo-Wide)**:
  - Update user-facing strings, window titles, tray labels/tooltips, CLI help text, and MCP server metadata from `Barycenter` / `PulseLaunch` / `search-widget` to **Vex** (`main.go`, `app_service.go`, `internal/mcp/server.go`, `frontend/index.html`, `frontend/src/`).
  - Rename binary output target to `bin/vex` (`bin/vex.exe` on Windows), log filename to `vex.jsonl`, structured log `service` field to `"vex"`, and Prometheus metric prefix to `vex_*` (`internal/logging/logger.go`).
  - Default data directory to `~/.vex` in `InitBackend` (`app_service.go`).
- [x] **Zero-Fork Build-Time Editions (`community` vs. `enterprise`)**:
  - Introduce package-level build variables in `main` (e.g., `buildinfo.go` or `app_service.go`) injectable via Go `-ldflags`:
    - `AppName` (default `"Vex"`)
    - `Edition` (default `"community"`)
    - `AppVersion` (default `"0.1.0"`)
    - `UpdateFeedURL` (default `""`, used as fallback when `config.json` has no override)
  - Support an optional build-time tray icon override (16×16 flag/edition icon embedded via build tag or optional asset override) while keeping the core repository 100% generic with zero hardcoded company hostnames or proprietary data.
  - Expose `GetAppInfo()` on `AppService` (`name`, `edition`, `version`) so the frontend renders the active edition name cleanly.
- [x] **"Less Is More" UI Copy Sweep**:
  - Audit `frontend/src/components/OnboardingWizard.tsx`, `frontend/src/App.tsx` (`? Help` popover & empty state), and `frontend/src/components/SettingsModal.tsx` to strip any catchphrases, taglines, or marketing copy.
- [x] **Rules, Skills & 6-Point Documentation Drift Sweep**:
  - Rename `.agent/rules/barycenter-workflow.md` → `.agent/rules/vex-workflow.md` and `.agent/skills/pulselaunch-architecture/` → `.agent/skills/vex-architecture/`.
  - Update `README.md`, `docs/ARCHITECTURE.md`, `.agent/skills/doc-drift-sweep/SKILL.md`, and `TODO.md` and run the full 6-point Documentation Drift Sweep.

### Acceptance Criteria
- Building with default flags produces `bin/vex` titled `Vex` writing `~/.vex/logs/vex.jsonl`; building with `-ldflags "-X main.Edition=enterprise -X main.UpdateFeedURL=..."` updates edition, `GetAppInfo()`, and default update feed with zero code changes.
- All data, logs, and config default cleanly to `~/.vex`.
- Zero legacy user-facing strings or UI taglines remain; verification gate passes.

---

## Phase 2: Hero Card + Compact Runner-Ups & Pinned `⌘1–⌘5` Categories
**Owner Model**: Gemini 4 Neon (Medium-High reasoning) · **Branch**: `feat/hero-runnerups-ux`

### Tasks
- [x] **User-Configurable `PinnedCategories` (`⌘1–⌘5`)**:
  - Add `PinnedCategories []string` (`json:"pinnedCategories,omitempty"`) to `UserPrefs` (`app_service.go` & `frontend/src/types/models.ts`) with sensible defaults when empty.
  - Render the oval glass search bar + up to 4 pinned category modifier pills (`⌘1–⌘5` shortcuts; optional overflow trigger for remaining collections) in `frontend/src/App.tsx`.
  - Allow pinning/unpinning and reordering categories in `frontend/src/components/SettingsModal.tsx`.
- [x] **Hero Card + Compact Runner-Ups Layout (`frontend/src/`)**:
  - Refine the search results surface inside the macOS 27 Siri Intelligence glass aesthetic (`frontend/src/App.tsx`, `frontend/src/components/CardRenderer.tsx`, `frontend/src/index.css`):
    - **Primary Hero Card**: Feature-rich top result with single-tab module perspectives (auto-selecting the matching module tab on hit, e.g. *Global Tech* when querying a host), structured template fields, action buttons, and a subtle provenance footer.
    - **Compact Runner-Ups**: Soft divider followed by 2–3 compact horizontal rows, each surfacing up to 3 backend-computed `GlancePills` (`card.glancePills`).
- [x] **Keyboard Navigation & Staged `Esc`**:
  - `↑` / `↓` navigates cleanly between the hero card and runner-up rows.
  - `Enter` on a runner-up promotes/swaps it into the Primary Hero Card slot; `Enter` on the focused hero executes its primary action.
  - Staged `Esc`: closes open popovers/modals → resets promoted runner-up or active category scope → clears query → hides launcher window.

### Acceptance Criteria
- Querying a multi-module entity (e.g., `AMS` or a host in a module overlay) renders 1 Primary Hero Card (with the matching perspective tab auto-selected and provenance footer visible) plus 2–3 compact runner-up rows with `GlancePills` below a subtle divider.
- `⌘1–⌘5` activates the user's configured `PinnedCategories`, persisting across restarts in `config.json`.
- Full keyboard loop (`↑`/`↓` + `Enter` promote + staged `Esc`) works without mouse input; verification gate passes.

---

## Phase 3: Viewport-Aware Upward Flip & Windows DPI/WebView2 Anchoring
**Owner Model**: Gemini 4 Neon (High reasoning) · **Branch**: `feat/upward-flip-anchoring`

### Tasks
- [x] **Pure-Go Display & Anchor Geometry Helper (`window_anchor.go`)**:
  - Implement testable work-area placement logic that inspects the active display's work area (`x, y, w, h`), cursor/tray spawn point, and launcher dimensions.
  - Detect when Vex spawns near the bottom-right system tray or within the bottom 25–30% of the active display's work area, returning exact `(x, y)` coordinates clamped inside the work area plus `AnchorDirection` (`"down"` or `"up"`).
  - Add unit tests covering single display, multi-monitor negative/offset coordinates, macOS menu bar vs. Windows taskbar, and 100%/150%/200% DPI scaling (`main_test.go` or `window_anchor_test.go`).
- [x] **Wails v3 Anchoring & Layout Reversal (`main.go`, `app_service.go`, `frontend/src/`)**:
  - Update `toggleLauncher` in `main.go` (inside `application.InvokeAsync`) to compute spawn position + `AnchorDirection`, position the window before `Show()`, and notify the frontend via a Wails event + `GetAnchorDirection()` RPC.
  - In `frontend/src/App.tsx` and `frontend/src/index.css`, when `anchor === 'up'`, lock the oval search bar to the bottom of the window and reverse the flex container (`flex-col-reverse`) so runner-ups and the Primary Hero Card expand upward smoothly with zero bottom-edge bounce or clipping.
- [x] **Windows WebView2 Glass & Tray Polish**:
  - Verify `BackgroundTypeTransparent` + `BackgroundColour: RGBA(0,0,0,0)` configuration and document/guard any Windows-specific WebView2 backdrop or DPI coordinate nuances (using physical vs. logical pixels consistently).

### Acceptance Criteria
- Spawning near the top/center opens normally (`anchor: "down"`, search bar on top); spawning from the bottom-right tray or bottom 25–30% of the screen flips cleanly (`anchor: "up"`, search bar locked to bottom, results expanding upward) with zero clipping or visual bounce.
- All macOS Wails window/tray calls remain wrapped in `application.InvokeAsync`.
- Pure-Go multi-monitor + DPI geometry tests and full verification gate pass.

---

## Phase 4: Minimal Enterprise Guardrails (`localOnly` RAG Filter & OS Secret Store)
**Owner Model**: Gemini 4 Hydrogen (Medium reasoning) · **Branch**: `feat/enterprise-guardrails`

### Tasks
- [x] **`localOnly` Flag on Modules & Provenance (`internal/models/`, `internal/indexer/`, `internal/ai/`)**:
  - Add `LocalOnly bool` (`json:"localOnly,omitempty"`) to `models.ModuleManifest` and `models.Provenance` in `internal/models/card.go`, and update `models.CardJSONSchema()`.
  - Propagate `LocalOnly` from a module manifest to its compiled `SearchCard`s and `CardSection`s in `internal/indexer/modules.go`.
  - Enforce `LocalOnly` in `internal/ai/service.go`: when `cfg.Provider != ProviderOllama` (i.e., cloud providers `openai`, `anthropic`, `gemini`), strip `localOnly` cards and `localOnly` sections before building RAG prompts and citations. Keep `localOnly` items 100% searchable locally and eligible for local Ollama RAG.
  - Surface a subtle `Local Only` badge in `CardRenderer.tsx` / `SettingsModal.tsx` where appropriate.
- [x] **Native OS Credential Store for AI API Keys (`internal/secrets/` + `app_service.go`)**:
  - Add a minimal, acyclic `internal/secrets` package abstracting OS secret storage (`Store` interface with `Get`, `Set`, `Delete`) backed by macOS Keychain (`security` / native keyring) on Darwin and Windows Credential Manager on Windows, plus an in-memory fake for sandbox-safe unit tests.
  - Move `ai.Config.APIKey` persistence out of plaintext `config.json` into the OS secret store.
  - On startup, automatically migrate any legacy plaintext `ai.apiKey` found in `config.json` into the OS secret store and rewrite `config.json` without the secret.
  - Never log or echo raw API keys in `vex.jsonl` or `GetConfig` diagnostics.

### Acceptance Criteria
- Unit tests verify that `localOnly` cards and overlay sections are excluded from OpenAI/Anthropic/Gemini RAG payloads while included in local search and Ollama RAG payloads.
- Saving an AI API key stores it in the OS credential store and leaves `apiKey` absent/redacted in `config.json`; legacy plaintext keys migrate automatically on load.
- Verification gate passes with `-race`.

---

## Phase 5: Lightweight Interactive TUI (`--tui` via Bubble Tea)
**Owner Model**: Gemini 4 Argon (High reasoning) · **Branch**: `feat/tui-bubbletea`

### Tasks
- [x] **Wire `--tui` Entrypoint & `internal/tui` Package**:
  - Add `github.com/charmbracelet/bubbletea` (and minimal styling via `lipgloss` if helpful, keeping dependencies lean) to `go.mod`.
  - Add `--tui` flag to `main.go` launching `internal/tui` directly against `backend.Search`, `backend.Store`, `backend.Runner`, and `backend.Config()` with zero duplicated search/conversion/module logic and zero impact on `--mcp-stdio` stdout cleanliness.
- [x] **Terminal Parity with Hero + Runner-Ups UX**:
  - Live search input bar + `1`–`4` category filter shortcuts (backed by `UserPrefs.PinnedCategories`).
  - **Primary Hero Card** pane showing title, subtitle, structured fields, provenance footer, and perspective tab switching (`Tab` / `Shift+Tab`) with auto-selected matching module tab.
  - **Compact Runner-Ups** list (2–3 rows with `GlancePills`) beneath a subtle divider; `↑`/`↓` + `Enter` promotes a runner-up into the hero slot; `Enter` on the focused hero executes its primary action (`copy` / `open_url` via `internal/plugins/runner.go`).
  - Instant offline unit & cargo volumetric math (`internal/search/converter.go`) rendered automatically at the top.
  - Staged `Esc` (clear scope/query → quit) and `Ctrl+C` clean exit.
- [x] **Tests & Final Drift Sweep**:
  - Add deterministic unit tests in `internal/tui/` covering model `Update` / `View` state transitions (typing query, `1–4` category toggle, `Tab` perspective switch, runner-up promotion, unit converter hit, staged `Esc`).
  - Run the 6-point Documentation Drift Sweep (`README.md`, `docs/ARCHITECTURE.md`, `TODO.md`, skills) so `--tui` and `internal/tui` are accurately documented.

### Acceptance Criteria
- `./bin/vex --tui` launches an interactive terminal UI with instant search, `1–4` category filters, hero perspective tabs (`Tab`/`Shift+Tab`), compact runner-ups, volumetric math, and action execution.
- `./bin/vex --mcp-stdio` continues to emit zero non-JSON-RPC bytes on `stdout`.
- All unit tests and the full verification gate pass.

---

## Phase 6: Enterprise Auth (`gh`/`az` MCP Delegation) & Pattern Recognition Engine
**Owner Model**: Gemini 4 Neon (High reasoning) · **ADR**: [docs/ADR-001-enterprise-auth-and-patterns.md](docs/ADR-001-enterprise-auth-and-patterns.md)

### Tasks
- [x] **Security Feasibility Review & MCP-First Enterprise Auth (`internal/mcp/auth_probe.go`, `internal/mcp/client.go`)**:
  - Documented threat model and architecture decision in `docs/ADR-001-enterprise-auth-and-patterns.md`: rejected raw `gh auth token` / `az account get-access-token` extraction in favor of isolated official GitHub & Azure DevOps MCP servers + mandatory `LocalOnly: true` cloud-RAG isolation.
  - Added `LocalOnly` propagation on `ExternalServerConfig` / `buildVirtualCards`, `ExecutePatternMCP` on `ClientManager`, read-only `ProbeCLIAuth` (`gh auth status` / `az account show`), and opt-in `EnterpriseMCPPresets()`.
- [x] **Declarative Pattern Recognition, Reference Decoding & Live Entity Enrichment (`internal/models/pattern.go`, `internal/indexer/patterns_loader.go`, `internal/search/patterns.go`)**:
  - Added `models.PatternRule` & `PatternMCPLookup` (exported in `models.CardJSONSchema()` and loadable from `~/.vex/patterns/*.json` or `ModuleManifest.Patterns`).
  - Added built-in generic standards (`iata-awb` Mod-7, `iso-container` ISO 6346 Mod-11, `ado-work-item`, `github-issue`), cross-card `EntityKey` enrichment (`Store.FindCardsByEntityKeys`), and live MCP template card unpacking via `LivePatternExecutor` (1500 ms timeout, graceful offline fallback).
  - Exposed `ListPatternRules()`, `ProbeEnterpriseCLIAuth()`, and `GetEnterpriseMCPPresets()` on `AppService`.
- [x] **Unit Tests & Handoff Doc Sync**:
  - Added `internal/search/patterns_test.go` (IATA Mod-7, ISO 6346, custom 10-digit `7031234567` House/Master Bill → `loc:SEA` cross-enrichment, live ADO MCP template card + offline fallback) and `internal/mcp/auth_probe_test.go`.
  - Synced `README.md`, `docs/ARCHITECTURE.md`, `docs/ADR-001-enterprise-auth-and-patterns.md`, `.agent/skills/vex-architecture/SKILL.md`, and `TODO.md`.

---

## Deferred Backlog (Out of Scope for Vex v1 Phases 1–6)
Kept here for post-v1 tracking so the phases above stay tight and free of scope creep:
- [ ] **Distribution & Updates**: Host `manifest.json` on an internal URL/network share; automated binary download + SHA-256 verification (`updater.VerifyFileSHA256`) + staged self-update.
- [ ] **Observability Wiring**: Connect Prometheus / Grafana Alloy scraping (`127.0.0.1:17380/metrics`), Loki log shipping (`vex.jsonl`), and starter Grafana dashboard JSON.
- [ ] **Real Team Data & Shared Modules**: Replace sample `global-tech` / `github-catalog` data with live CMDB / `gh repo list` exports; add `sharedModulesDir` auto-loading; curated department module packs; optional Accounting, DGR/IMDG, and Carrier/AWB modules.
- [ ] **Enterprise Settings UI Polish**: Surface `ProbeEnterpriseCLIAuth` status badges, one-click `EnterpriseMCPPresets` setup, and a custom `~/.vex/patterns/` rule viewer in `SettingsModal.tsx`.
- [ ] **Architecture & Authoring**: Slim down `app_service.go` (`config.go`, `weather.go`), Visual Module Authoring Studio, and future Angular / Vanilla JS UI layer validation.
