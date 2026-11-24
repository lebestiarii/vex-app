# Vex

**Vex** is a desktop-first, platform-agnostic, local-first knowledge Rolodex, configurable pattern decoder, and MCP hub built with **Go + Wails v3** and **Angular 19 (Standalone Components + Signals)**.

> **Documentation**: See **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** for system architecture, **[docs/TEMPLATE_BLUEPRINTS.md](docs/TEMPLATE_BLUEPRINTS.md)** for layout blueprints & data examples of every card template, and **[TODO.md](TODO.md)** for the phased roadmap and open questions.

Press your global shortcut (`Ctrl+Shift+B` by default) or click the tray icon to open the **Vex Full-Width Stacked Card Rolodex** (Light & Dark mode) — search gateways, air carrier prefixes, ocean SCACs, ISO containers, unit conversions, server matrices, bookmark portals, and live-decoded reference numbers with full-width nested cards up to 2 levels deep.

---

## Key Capabilities

- **Full-Width Stacked Nested Card System (Angular 19, Light/Dark Mode, Platform-Agnostic)**:
  - Responsive `420px–520px` (default `460px`) desktop widget frame with clean internal scrolling (`max-height: 680px`), Light & Dark themes (`[data-theme="light"]` / `[data-theme="dark"]` synced with OS preference + manual toggle), and zero OS-specific window buttons or key symbols.
  - **Zero accordion tree lines**: Nested child cards span **100% of their parent's inner width** stacked vertically (`gap: 8px`) with depth communicated via subtle alternating inset surfaces (L0 → L1 → L2).
  - **Unified Header-Strip Pattern**: `[Icon/Flag]` + `15px` semibold title + L0-only Pin (`3` max with blue-tinted pill, shake & toast feedback) + expand chevron + semantic/stoplight glance chips (`● DEV` green, `● QA` amber, `● PROD` red, blue codes, monospace clocks/temps).
  - **5 Canonical Generic Nestable Templates** (plus `container-blueprint`, `dictionary-guide`, `kv-grid`):
    1. `location` — Strictly header-only container (UNLOC·IATA, live IANA clock, currency, temp; no addresses/phones) expanding directly into full-width nested child cards.
    2. `server-matrix` — Stoplight `● DEV` / `● QA` / `● PROD` rows with monospace hostnames and inline `[Copy]`, `[SSH]`, and `[↗]` buttons.
    3. `website-link` — Multi-Chip Mode (2+ links) expanding to launch/copy rows, and Single-Button Mode (1 link) omitting the chevron so the whole card clicks to open with a top-right `↗`.
    4. `pattern-breakdown` — 3-column equal-width Blue/Amber/Green segment tile grid (`Value` + `ROLE` + `Meaning`) + plain-English explanation + full-width nested entity cards.
    5. `conversion` — Compact `Input ⇄ Output` strip with 1-click Copy and equivalent chips.
- **100% Optional, Deletable Modules (`~/.vex/modules/`)**:
  - Nothing is forced from code and no sample/demo data is baked into the frontend or backend. All 6 packaged optional starter modules (**Air Carrier IATA Prefix Index**, **Ocean Carrier SCAC Index**, **ISO Container Sizes & Blueprints**, **Logistics Dictionary**, **Reference Number Decoders**, and **Generic Team Templates**) are optional packs installable at onboarding or via the **Modules** dialog.
  - Every installed module lives as a real JSON file in `~/.vex/modules/<id>.json` that can be toggled, edited, updated via MCP, or deleted permanently (with live `fsnotify` hot-reload).
- **Loadable, Configurable Pattern Matching (`~/.vex/patterns/` & Module Patterns)**:
  - Zero hardcoded rules in Go. Supports input normalizers (`trim`, `uppercase`, `strip-spaces`, `strip-dashes`), checksum validators (`iata-mod7`, `iso6346`, `luhn`), 3-column `segments[]` + `explanationTemplate`, live SQLite `dataLookups` across modules, recursive cross-card `linkRules`, and live dry-run testing in the **Patterns** dialog.
- **Built-In MCP Hub & Agent Skill Sharing**:
  - Runs as a **Domain MCP Server** (`--mcp-stdio` for Claude Desktop, Cursor, Copilot CLI; Streamable HTTP at `http://127.0.0.1:17380/mcp` for VS Code Copilot) exposing **19 tools**, **7 resources/templates**, and **1 prompt** so AI agents can query Vex data, transform raw CSV/JSON files into cards, enrich/create modules & pattern rules, and read shared agent skills (`list_skills`, `get_skill`, `search://skills`, `skill://{name}`).
  - Also supports connecting to external MCP servers (`ClientManager`) and read-only `gh` / `az` CLI session checks.

---

## Build & Run

### Prerequisites
- **Go `1.27+`** (pure-Go SQLite via `modernc.org/sqlite`; no C SQLite compiler required).
  - **macOS**: Xcode Command Line Tools (`xcode-select --install`) for Cocoa/WKWebView.
  - **Windows**: Built-in WebView2 (Windows 10/11).
- **Frontend (only needed if editing UI code)**: **Deno `2.x`** *or* **Node.js `20+` / Angular CLI**.
  - Pre-built `frontend/dist` assets are embedded via `//go:embed all:frontend/dist`.

### Build Frontend & Native Desktop Binaries

#### macOS (`bin/macos/vex`)
```bash
# 1. Typecheck & build Angular 19 AOT + Ivy-linked frontend -> frontend/dist/
#    With Deno:
cd frontend && deno install --node-modules-dir=auto && deno task typecheck && deno task build && cd ..
#    Or with Node.js / Angular CLI:
#    cd frontend && npm install && npm run ng:build && cd ..

# 2. Run full Go test suite with race detector (100% offline)
go test -race -count=1 ./...

# 3. Build macOS desktop binary (with WKWebView transparency tag)
go build -tags private_mac_apis -o bin/macos/vex .
```

#### Windows (`bin/windows/vex.exe`)
<!-- TODO: Complete Windows build template (toolchain paths, WebView2 setup, build command for bin/windows/vex.exe, and test verification). -->
*(Section intentionally left blank — complete this template on Windows.)*

### Run & Preview
```bash
# Launch macOS desktop app (default data directory ~/.vex)
./bin/macos/vex

# Launch with a clean isolated data directory (opens starter module onboarding)
./bin/macos/vex --data-dir /tmp/vex-fresh

# Run headless MCP stdio server (for Claude Desktop, Cursor, Copilot CLI)
./bin/macos/vex --mcp-stdio

# Override HTTP MCP bind address (or pass 'off' to disable)
./bin/macos/vex --mcp-addr 127.0.0.1:17380

# Override global shortcut accelerator
./bin/macos/vex --hotkey Ctrl+Shift+B
```

---

## Security Posture
- **100% Local-First & `localOnly` Cloud RAG Guardrail**: All cards, modules, patterns, and SQLite FTS5 indexes live locally in `~/.vex`. Items marked `localOnly: true` are automatically stripped from cloud LLM calls (`openai`, `anthropic`, `gemini`) while remaining searchable locally and via Ollama.
- **Native OS Credential Store (`internal/secrets`)**: AI API keys are stored in macOS Keychain or Windows Credential Manager — never persisted in plaintext `config.json`.
- **Loopback-Only MCP HTTP + CSRF/DNS-Rebinding Guard**: `127.0.0.1:17380` rejects non-loopback `Host` and cross-origin `Origin` headers.
- **Safe Action Execution**: `open_url` / `open_pwa` strictly validate `http://` / `https://` URLs and reject shell metacharacters; arbitrary shell execution is disabled.

---

## Roadmap
See [TODO.md](TODO.md) for the phased Vex implementation history, open clarifications, and upcoming backlog.
