# Vex

**Vex** is a desktop-first, platform-agnostic, local-first knowledge Rolodex, configurable pattern decoder, and MCP hub built with **Go + Wails v3** and **Angular 22 (Standalone Components + Signals + Zoneless)**.

> **Documentation**: See **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** for system architecture, **[docs/TEMPLATE_BLUEPRINTS.md](docs/TEMPLATE_BLUEPRINTS.md)** for layout blueprints & JSON examples of all 9 pure layouts, and **[TODO.md](TODO.md)** for the phased roadmap.

Press your global shortcut (`Ctrl+Shift+B` by default) or click the tray icon to open the **Vex Full-Width Stacked Card Rolodex** (Light & Dark mode) — search gateways, air carrier prefixes, ocean SCACs, ISO containers, unit conversions, action lists, tables, snippets, and live-decoded reference numbers with full-width nested cards up to 2 levels deep.

---

## Key Capabilities

- **Full-Width Stacked Nested Card System (Angular 22, Zoneless, Light/Dark Mode, Platform-Agnostic)**:
  - Responsive `420px–520px` (default `460px`) desktop widget frame with clean internal scrolling (`max-height: 680px`), Light & Dark themes (`[data-theme="light"]` / `[data-theme="dark"]` synced with OS preference + manual toggle), and zero OS-specific window buttons or key symbols.
  - **Zero accordion tree lines & zero empty accordions**: Nested child cards span **100% of their parent's inner width** stacked vertically (`gap: 8px`) via alternating inset surfaces (L0 → L1 → L2). Expand chevrons appear only when a card has renderable body content or children.
  - **Pinned + Recent Empty State & Keyboard Flow**: Empty search displays **Pinned** (max 3 with active blue pill highlight) and **Recent** (last 6 used cards from SQLite `card_recents`) plus subtle total indexed card count. Navigate Level 0 cards with `ArrowDown`/`ArrowUp`, trigger primary actions with `Enter` (open `link`, copy `convert`, or toggle expand), and press `Escape` once to clear query or twice to hide the window.
  - **9 Pure Generic Nestable Layouts** (strict zero-alias `layout` field):
    1. `group` — Header-only container expanding directly into full-width nested child cards.
    2. `link` — Single-Button clickable launch card with top-right `↗` and no chevron.
    3. `action-list` — Compact rows with optional stoplight badge, primary text (optional mono), subtitle, and 1–3 `copy` / `open_url` buttons.
    4. `facts` — Optional summary, optional segmented `views[]` switcher (e.g. Metric / Imperial), and read-only `Label / Value` grid (`[Copy]` rendered only when `copyable: true`).
    5. `segments` — 2–4 equal-width Blue/Amber/Green click-to-copy token tiles + plain-English explanation + full-width nested entity cards.
    6. `steps` — N-step visual timeline + summary + optional bullets.
    7. `convert` — Compact `Input ⇄ Output` strip with 1-click `[Copy]` and clickable equivalent pills.
    8. `table` — Sticky header (2–4 cols, max 8 visible rows), optional mono columns, `[Copy CSV]` and `[Copy MD]` footer buttons.
    9. `snippet` — Optional compact inline `{{var}}` inputs live-interpolated into a monospace block + 1-click `[Copy]`.
- **100% Optional, Deletable Modules (`~/.vex/modules/`)**:
  - Zero hardcoded sample data in runtime code. All 5 packaged optional starter modules (**Air Carrier IATA Prefix Index**, **Ocean Carrier SCAC Index**, **ISO Container Sizes & Blueprints**, **Logistics Dictionary**, and **Reference Number Decoders**) live as flat-on-disk 9-layout JSON packs installable at onboarding or via the **Modules** dialog.
  - Every installed module lives as an editable JSON file in `~/.vex/modules/<id>.json` that can be toggled, exported, updated via MCP, or deleted permanently (with live `fsnotify` hot-reload).
- **Loadable, Configurable Pattern Matching (`~/.vex/patterns/` & Module Patterns)**:
  - Zero hardcoded rules in Go. Supports input normalizers (`trim`, `uppercase`, `strip-spaces`, `strip-dashes`), checksum validators (`iata-mod7`, `iso6346`, `luhn`), 2–4 column `segments[]` + `explanationTemplate`, live SQLite `dataLookups` across modules, recursive cross-card `linkRules`, per-rule Enabled toggles, and top-of-modal live testing with rendered card previews in the **Patterns** dialog.
  - **Declarative Read-Only `liveHttp` GET Enrichment**: Optional `liveHttp` config on any `PatternRule` (`urlTemplate` with safe `PathEscape`/`QueryEscape`, `trigger: "auto"|"manual"`, bounded `timeoutMs ≤ 5000`, in-memory `cacheTtlSec`, `auth: "none"|"gh_cli"|"az_devops"|"env:VAR_NAME"` with zero inline secrets, and `resultCard` JSON dot-path projection into nested `facts`, `table`, `steps`, or `link` cards). Manual rules render an on-demand **Fetch Live Data** button; offline/timeout errors preserve the local `segments` decode with a muted `Live lookup unavailable` chip.
  - Includes optional disabled-by-default starter patterns in `reference-patterns.json` for Azure DevOps work items (`AB#12345` via `liveMcp`) and GitHub issues/PRs (`owner/repo#123` via `liveHttp` + `gh_cli`).
- **High-Precision 4-Tier Search & Indexed SQLite v3 (`internal/indexer/store.go`)**:
  - Atomic v3 schema with `entity_links` dynamic 2-level cross-module nesting and `card_recents` usage tracking. Ranks exact entity/code/tag/chip matches decisively first, followed by 1-hop contextual neighbors and whole-token FTS5 BM25, then prefix FTS5 and guarded metadata-only fallback — with a strict layout-aware `SearchableBody()` whitelist, dynamic relevance cliff, and a focused `10`-card active query limit.
- **Built-In MCP Hub, GitHub/ADO Query Flow & Agent Skill Sharing**:
  - Runs as a **Domain MCP Server** (`--mcp-stdio` for Claude Desktop, Cursor, Copilot CLI; Streamable HTTP at `http://127.0.0.1:17380/mcp` for VS Code Copilot) exposing **20 tools** (including strict `validate_module`) and **7 resources/templates** (including shared agent skills: `list_skills`, `get_skill`, `search://skills`, `skill://{name}`).
  - Connects to external MCP servers (`ClientManager`) with 1-click **GitHub MCP** and **Azure DevOps MCP** presets + live `gh`/`az` CLI auth status inside the **Modules & MCP** dialog, mapping `/gh <query>` and `/ado <query>` results directly onto Vex's generic `link`, `facts`, `action-list`, and `table` layouts.

---

## Build & Run

### Prerequisites
- **Go `1.27+`** (pure-Go SQLite via `modernc.org/sqlite`; no C SQLite compiler required).
  - **macOS**: Xcode Command Line Tools (`xcode-select --install`) for Cocoa/WKWebView.
  - **Windows**: Built-in WebView2 (Windows 10/11).
- **Frontend (only needed if editing UI code)**: **Deno `2.x`** *or* **Node.js `22.22+` (`24+` LTS) / Angular CLI `22`**.
  - Pre-built `frontend/dist` assets are embedded via `//go:embed all:frontend/dist`.

### Build Frontend & Native Desktop Binaries

#### macOS (`bin/macos/vex`)
```bash
# 1. Typecheck, test (Vitest + happy-dom), & build Angular 22 Zoneless AOT frontend -> frontend/dist/
#    With Deno:
cd frontend && deno install --node-modules-dir=auto && deno task typecheck && deno task test && deno task build && cd ..
#    Or with Node.js / Angular CLI:
#    cd frontend && npm install && npm run ng:test && npm run ng:build && cd ..

# 2. Run full Go test suite with race detector (100% offline)
go test -race -count=1 ./...

# 3. Build macOS desktop binary (with WKWebView transparency tag)
go build -tags private_mac_apis -o bin/macos/vex .
```

#### Windows (`bin\windows\vex.exe`)
*(Native Windows binary execution and WebView2 rendering are verified on Windows 10/11 hosts.)*
```powershell
# 1. Typecheck, test, & build Angular 22 Zoneless AOT frontend -> frontend/dist/
#    With Deno:
cd frontend; deno install --node-modules-dir=auto; deno task typecheck; deno task test; deno task build; cd ..
#    Or with Node.js 22.22+ / Angular CLI:
#    cd frontend; npm install; npm run ng:test; npm run ng:build; cd ..

# 2. Run full Go test suite with race detector (100% offline)
go test -race -count=1 ./...

# 3. Build Windows desktop binary (uses built-in Microsoft Edge WebView2)
go build -o bin\windows\vex.exe .
```

### Run & Validate
```bash
# Launch desktop app (default data directory ~/.vex on macOS, %USERPROFILE%\.vex on Windows)
./bin/macos/vex        # macOS
# .\bin\windows\vex.exe  # Windows (PowerShell)

# Launch with a clean isolated data directory (opens starter module onboarding)
./bin/macos/vex --data-dir /tmp/vex-fresh

# Validate one or more module JSON manifests against the strict 9-layout schema
./bin/macos/vex validate path/to/module.json

# Run headless MCP stdio server (for Claude Desktop, Cursor, Copilot CLI)
./bin/macos/vex --mcp-stdio

# Override HTTP MCP bind address (or pass 'off' to disable)
./bin/macos/vex --mcp-addr 127.0.0.1:17380

# Override global shortcut accelerator
./bin/macos/vex --hotkey Ctrl+Shift+B
```

---

## Security Posture
- **100% Local-First**: All cards, modules, patterns, logs, and SQLite FTS5 indexes live locally in `~/.vex` with zero embedded cloud AI calls.
- **Loopback-Only MCP HTTP + CSRF/DNS-Rebinding Guard**: `127.0.0.1:17380` rejects non-loopback `Host` and cross-origin `Origin` headers.
- **Safe Action & Read-Only `liveHttp` Execution**: `open_url` / `open_pwa` and `liveHttp` strictly validate `http://` / `https://` URLs and reject shell metacharacters, embedded URL credentials, inline tokens, and auth headers in JSON; `liveHttp` is strictly GET-only with cached OS CLI (`gh`/`az`) or `env:VAR_NAME` auth; arbitrary shell execution is disabled.

---

## Roadmap
See [TODO.md](TODO.md) for the phased Vex implementation history and upcoming backlog.
