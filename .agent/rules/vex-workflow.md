# Vex Workspace Workflow & Engineering Rules

## 1. Platform-Specific Build Toolchains & Binary Paths

### macOS (`bin/macos/vex`)
- **Go Toolchain**: Always invoke `/opt/homebrew/Cellar/go/1.27.4/bin/go`.
  - Always pass `-tags private_mac_apis` so Wails v3 WKWebView transparency (`MacBackdropTransparent`) renders cleanly:
    ```bash
    /opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .
    ```
  - Optional build-time metadata overrides (`AppName`, `AppVersion`, `UpdateFeedURL`) can be passed via `-ldflags` (single unified app — no edition split).
- **Frontend Toolchain (Angular 19 + Deno)**: Always invoke `/opt/homebrew/Cellar/deno/2.9.8/bin/deno` inside `frontend/`:
  - `deno task typecheck` (Angular `ngc` strict AOT check)
  - `deno task build` (`scripts/build-aot.mjs` → `ngc` AOT + Angular Ivy Linker + `esbuild` bundle into `frontend/dist/`)
  - Standard `angular.json` is preserved for `ng serve` / `ng build` under Node.js.
- **Sandbox-Safe Tests**: Never bind loopback TCP ports in unit tests; use injected `http.RoundTripper`, `client.NewInProcessClient`, and `httptest.NewRecorder`.

### Windows (`bin/windows/vex.exe`)
<!-- TODO: Complete Windows build instructions template (Go/Deno/Node binary paths, WebView2 prerequisites, build command outputting bin/windows/vex.exe, and verification steps). -->

- **Binary Path**: `bin/windows/vex.exe`
- *(Section intentionally left blank — complete this template on Windows)*

---

## 2. Git Branching Discipline
- Keep `main` clean and verified.
- Develop each new phase on a dedicated local feature branch (`feat/<topic>`).
- Merge back to `main` (`git merge --no-ff`) and delete the feature branch only when the phase is verified or requested by the user.
- Never push to a remote repository unless explicitly asked.

## 3. Architecture & UX Guardrails
- **Acyclic Go Package Graph**: `internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → (`internal/ai`, `internal/mcp`) → `main`.
- **Desktop-First & Platform-Agnostic UI**: No CLI TUI code; never render OS-specific window buttons (traffic lights, Windows chrome) or OS-specific key symbols (`⌘`, `⌥`) in UI components.
- **Zero Hardcoded Sample Data & Optional Deletable Modules**: Never hardcode demo/sample cards in Go or Angular (`frontend/src/`). Packaged optional starter packs live in `internal/indexer/starter_modules/*.json` and are installed into `~/.vex/modules/` only when selected.
- **Generic Templates First**: Keep the 5 canonical Angular templates (`location`, `server-matrix`, `website-link`, `pattern-breakdown`, `conversion`) and extended templates (`container-blueprint`, `dictionary-guide`, `kv-grid`, `branch-contact`) content-agnostic and reusable across domains. Follow `.agent/skills/vex-rolodex-ui/SKILL.md`, `.agent/skills/vex-template-authoring/SKILL.md`, and `docs/TEMPLATE_BLUEPRINTS.md`.
- **macOS Main-Thread Safety**: Always wrap Wails window/tray/menu mutations inside `GlobalShortcut` or background callbacks with `application.InvokeAsync(func() { ... })`.
- **Clean `--mcp-stdio`**: Never write non-JSON-RPC bytes to `stdout`.
- **Less Is More UI**: No UI taglines, catchphrases, or marketing copy.

## 4. Mandatory Phase-End Verification, Documentation Sync & Status Report
At the end of **every** phase:
1. **Update Documentation to Prevent Drift**: Run the `doc-drift-sweep` checklist (`.agent/skills/doc-drift-sweep/SKILL.md`) and update `README.md`, `docs/ARCHITECTURE.md`, `docs/TEMPLATE_BLUEPRINTS.md`, `TODO.md`, `.env.example`, `.gitignore`, and workspace skills (`vex-architecture`, `vex-rolodex-ui`, `vex-template-authoring`) whenever code, schemas, MCP tools, or templates change.
2. **Run the Full Verification Gate**:
   - **Frontend**: `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
   - **Go Tests**: `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
   - **macOS Binary**: `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .`
   - **Windows Binary (`bin/windows/vex.exe`)**: *(TODO: Complete Windows verification template)*
3. **Output a Structured Status Report** covering:
   - **Testing & Build Gate Results**
   - **What Was Done**
   - **What Went Well**
   - **What Was Added / Modified** (with clickable `file://` links)
   - **Documentation & Skill Sync Summary**
   - **Context & Model Recommendations**
