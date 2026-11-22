# Vex

**Vex** is a fast, local-first, cross-platform (macOS & Windows) desktop search launcher, composable knowledge hub, and tri-modal MCP + AI RAG server built with **Go + Wails v3** and **React + TypeScript + Tailwind CSS**.

> **Deep-Dive Documentation**: See **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** for the full system architecture diagram and design decisions, **[docs/ADR-001-enterprise-auth-and-patterns.md](docs/ADR-001-enterprise-auth-and-patterns.md)** for the Enterprise Auth (`gh`/`az` MCP) & Pattern Recognition design record, and **[TODO.md](TODO.md)** for the phased roadmap.

Press your global hotkey (`Ctrl+Shift+B` by default, configurable in Settings) to open a calm, two-piece floating glass search bar — get instant answers, unit/dimensional weight conversions, live port/airport clocks & weather, and role-tailored knowledge modules.

---

## Key Capabilities

- **Two-Piece Floating UI & Viewport-Aware Upward Flip**:
  - Oval glass search bar + user-configurable `⌘1`–`⌘5` pinned category pills (`PinnedCategories`), **1 Primary Hero Card** with single-tab module perspectives, and **2–3 compact horizontal runner-up rows** with `GlancePills` and keyboard promotion (`↑`/`↓` + `Enter`, staged `Esc`).
  - Automatically flips the layout upward (`flex-col-reverse`, search bar anchored at the bottom) when spawned from the bottom-right system tray or bottom 25–30% of the active display's work area.
  - Minimal 3-step **Onboarding** + contextual `? Help` popover.
- **Interactive Terminal UI (`--tui`)**:
  - Run `./bin/vex --tui` for full terminal parity via Bubble Tea: live search input, `1`–`4` (`Alt+1`–`Alt+5`) category filters, Primary Hero Card with `Tab`/`Shift+Tab` perspective tabs, compact runner-ups with `Enter` promotion/action execution, and instant unit/cargo volumetric math.
- **Composable, Toggleable Modules & Card Overlays**:
  - Enable only the modules relevant to your role in **Settings → Modules** (or share/import portable `.json` module packs).
  - Modules contribute standalone cards **and** enrich shared entities (`entityKey`, e.g. `loc:AMS`) with isolated Perspective Tabs (`Overview`, `Air`, `Ocean`, `Customs`, `Transcon`, `Global Tech`, `GitHub Catalog`) — and promote overlays to standalone cards if the base module is turned off.
  - Includes a lightweight **GitHub Engineering Catalog** (`gt-<portfolio>-<team>-<product>-<component>`) that parses repo metadata into repo cards + Portfolio/Team directory roll-ups.
- **Instant Unit, Cargo Dimensional Weight & Reference Pattern Decoder**:
  - Type `1588 kg to lbs`, `-30c to f`, `4.3 cbm to ft3`, or `120x80x100 cm` for zero-latency conversions and IATA `1:167` air volumetric weight + Ocean LCL `W/M` calculations.
  - Paste an IATA Air Waybill (`176-12345675`, Mod-7 verified), ISO 6346 container (`MSCU123456...`), Azure DevOps Work Item (`AB#48219`), GitHub issue (`owner/repo#42`), or drop custom JSON rules (e.g., 10-digit House/Master Bills) into `~/.vex/patterns/` to decode references and cross-enrich linked branch/carrier cards automatically.
- **Source Provenance & Tri-Modal MCP + AI RAG**:
  - Every card/section carries `Provenance` (`sourceName`, `sourceUrl`, `verifiedAt`, `authority`) surfaced in the UI footer and injected into AI RAG & MCP context.
  - Runs as a **Domain MCP Server** (`--mcp-stdio` for Claude Desktop, Cursor, and GitHub Copilot CLI; Streamable HTTP at `http://127.0.0.1:17380/mcp` for VS Code Copilot) and **External MCP Client**.
- **Zero-Fork Build-Time Editions (`community` / `enterprise`)**:
  - Override `main.AppName`, `main.Edition`, `main.AppVersion`, `main.UpdateFeedURL`, and `assets/tray_icon.png` at build time via `-ldflags`.

---

## Build & Run

### Prerequisites & Dependencies
- **Go `1.27+`** (`go.mod` / `go.sum` installs all Go dependencies automatically; SQLite uses pure-Go `modernc.org/sqlite`, so no C SQLite compiler is required).
  - **macOS**: Xcode Command Line Tools (`xcode-select --install`) for Cocoa/WKWebView.
  - **Windows**: Built-in WebView2 (pre-installed on Windows 10/11).
- **Frontend (only needed if editing UI code)**: **Deno `2.x`** *or* **Node.js `20+` / npm**.
  - *Tip*: Pre-built `frontend/dist` assets are committed and embedded via `//go:embed`, so you can run `go build` immediately after `git clone`.

### Build Frontend & Native Binary
```bash
# 1. (Optional — only if modifying frontend/src) Install deps, typecheck & build UI:
#    With Deno:
cd frontend && deno install && deno task typecheck && deno task build && cd ..
#    Or with Node/npm:
#    cd frontend && npm install && npm run typecheck && npm run build && cd ..

# 2. Run full Go test suite with race detector (100% offline, no external DB)
go test -race -count=1 ./...

# 3. Build macOS binary (with WKWebView transparency tag)
go build -tags private_mac_apis -o bin/vex .

# Optional build-time Edition override (e.g. enterprise)
# go build -tags private_mac_apis -ldflags "-X main.Edition=enterprise" -o bin/vex .

# (Windows build)
# go build -o bin/vex.exe .
```

### Run
```bash
# Default data directory (~/.vex)
./bin/vex

# Fresh isolated demo directory (triggers first-run onboarding)
./bin/vex --data-dir /tmp/vex-demo

# Interactive Bubble Tea Terminal UI (Hero Card + Runner-Ups in the terminal)
./bin/vex --tui

# Headless MCP stdio server (for Claude Desktop, Cursor, Copilot CLI)
./bin/vex --mcp-stdio
```

---

## Security Posture
- **100% Local-First & `localOnly` Cloud RAG Guardrail**: All cards, SQLite FTS5 indexes, and embeddings stay on disk in `~/.vex`; modules, cards, or overlay sections marked `localOnly: true` are automatically stripped from external cloud LLM payloads (`openai`, `anthropic`, `gemini`) while remaining 100% searchable locally and via local Ollama.
- **Native OS Credential Store (`internal/secrets`)**: AI API keys are persisted in macOS Keychain or Windows Credential Manager (never stored in plaintext `config.json`, with automatic startup migration and scrubbing of legacy keys).
- **Loopback-Only MCP HTTP + CSRF/DNS-Rebinding Guard**: The HTTP MCP endpoint binds to `127.0.0.1:17380` and rejects non-loopback `Host` or cross-origin `Origin` headers.
- **Safe Action Execution**: `open_url` / `open_pwa` strictly validate `http://` / `https://` URLs and reject shell metacharacters; arbitrary shell command execution is disabled across cards, plugins, and MCP.

---

## Roadmap
See [TODO.md](TODO.md) for the phased Vex implementation roadmap and deferred backlog.
