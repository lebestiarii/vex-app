# Vex — Cross-Agent Project Instructions

**Vex** is a desktop-first, platform-agnostic, local-first knowledge Rolodex, configurable pattern decoder, and MCP hub built with **Go + Wails v3** and **Angular 22** (Standalone Components + Signals + Zoneless).

## Core Architecture & Guardrails
- **Acyclic Go Packages**: `internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → (`internal/ai`, `internal/mcp`) → `main`.
- **UI & Feel Stability**: Preserve Light/Dark CSS tokens (`frontend/src/styles.css`), full-width stacked card geometry (no vertical accordion tree lines, L0→L1→L2 alternating inset surfaces), and interaction feel unless UI changes are explicitly requested.
- **Platform-Agnostic UI**: Never render OS-specific window chrome (traffic lights, Windows buttons) or OS-specific key glyphs (`⌘`, `⌥`) in UI components.
- **Zero Hardcoded Sample Data**: Never hardcode demo/sample cards in Go or `frontend/src/`. Optional starter packs live in `internal/indexer/starter_modules/*.json`.
- **macOS Main-Thread Safety**: Wrap Wails window/tray/menu mutations inside `GlobalShortcut` or background callbacks with `application.InvokeAsync(func() { ... })`.
- **Clean `--mcp-stdio`**: Never write non-JSON-RPC bytes to `stdout`.
- **Sandbox-Safe Tests**: Never bind loopback TCP ports in unit tests; use `http.RoundTripper`, `client.NewInProcessClient`, and `httptest.NewRecorder`.

## Workflow: Phases, Stories & After-Action Reviews
1. Break multi-step work into **Phases**, each decomposed into **Stories** (`X.1`, `X.2`, …) with clear intent, file scope, and acceptance criteria.
2. Assign a recommended Gemini model and reasoning level per phase; pause after scoping or completing a phase so the user can switch models.
3. Develop on a local `feat/<topic>` branch; keep `main` clean and never push unless explicitly asked.
4. Run the verification gate and documentation drift sweep (`.agent/skills/doc-drift-sweep/SKILL.md`) at the end of every phase, and deliver a structured **After-Action Review (AAR)** (gate results, changes by story, what went well, skill update/new-skill suggestions, and next-phase model recommendation).

## Verification Commands
- **macOS (Homebrew paths for sandboxed agents)**:
  - Frontend typecheck, unit tests & build: `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
  - Go tests: `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
  - macOS binary: `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .`
- **Windows (`bin\windows\vex.exe`)**:
  - PowerShell: `cd frontend; deno task typecheck; deno task test; deno task build; cd ..; go test -race -count=1 ./...; go build -o bin\windows\vex.exe .` (or `npm run ng:test` / `ng:build` with Node.js `22.22+`; native Windows execution verified on Windows hosts).

## Canonical References (Single Source of Truth)
- Overview & build/run: [README.md](README.md)
- System architecture & MCP hub: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- Card template blueprints: [docs/TEMPLATE_BLUEPRINTS.md](docs/TEMPLATE_BLUEPRINTS.md)
- Roadmap & open items: [TODO.md](TODO.md)
- LLM codebase index: [llms.txt](llms.txt)
- Antigravity workflow rules & skills: [.agent/rules/vex-workflow.md](.agent/rules/vex-workflow.md), [.agent/skills/](.agent/skills/)
