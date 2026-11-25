# Vex — Cross-Agent Project Instructions

**Vex** is a desktop-first, platform-agnostic, local-first knowledge Rolodex, configurable pattern decoder, and MCP hub built with **Go + Wails v3** and **Angular 22** (Standalone Components + Signals + Zoneless).

## Core Architecture & Guardrails
- **Acyclic Go Packages**: `internal/models → internal/indexer → internal/search → internal/plugins → internal/mcp → main`.
- **UI & Feel Stability**: Preserve Light/Dark CSS tokens (`frontend/src/styles.css`), full-width stacked card geometry (no vertical accordion tree lines, L0→L1→L2 alternating inset surfaces), and interaction feel unless UI changes are explicitly requested.
- **Platform-Agnostic UI**: Never render OS-specific window chrome (traffic lights, Windows buttons) or OS-specific key glyphs (`⌘`, `⌥`) in UI components.
- **Zero Hardcoded Sample Data**: Never hardcode demo/sample cards in Go or `frontend/src/`. Optional starter packs live in `internal/indexer/starter_modules/*.json`.
- **macOS Main-Thread Safety**: Wrap Wails window/tray/menu mutations inside GlobalShortcut or background callbacks with `application.InvokeAsync(func() { ... })`.
- **Clean `--mcp-stdio`**: Never write non-JSON-RPC bytes to stdout.
- **Sandbox-Safe Tests**: Never bind loopback TCP ports in unit tests; use `http.RoundTripper`, `client.NewInProcessClient`, and `httptest.NewRecorder`.

## Standard Project & Feature Rhythm (Mandatory)
Every new project, feature, or multi-step chat MUST follow `.agent/skills/after-action-review/SKILL.md` in this exact order:

1. **Plan & Scope First (Before Writing Code)**:
   - Decompose work into **Phases → Stories** (`X.1`, `X.2`, …) with clear intent, file scope, and acceptance criteria.
   - Output the **Phase Model, Context & Reasoning Table** (recommended model, reasoning level, task intensity, context footprint, and justification for choosing a strong vs. lighter model).
   - Confirm git isolation: work on a local `feat/<topic>` branch (or a `git worktree` for parallel/high-risk phases). Keep `main` clean.
   - **Pause** for user approval and optional model switching before starting implementation.
2. **Execute & Verify**:
   - Implement stories cleanly and run the Verification Commands below.
   - Run the full **Documentation Drift Sweep** (`.agent/skills/doc-drift-sweep/SKILL.md`).
3. **Walkthrough & After-Action Review (AAR)**:
   - Deliver the structured **AAR** (gate results, changes by story with `file://` links, **what went well**, **what went poorly**, **skill/instruction add/improve/remove recommendations**, and **context/model handoff**: compact vs. new chat + next model).
   - Provide a concrete **User Verification Walkthrough** and **pause for user approval** before any merge.
4. **Post-Approval Merge & Hygiene**:
   - Only after user approval: merge the worktree back into the feature branch (if used), merge the feature branch into `main` (never push unless explicitly asked), and list stale worktrees/branches recommended for pruning.

## Verification Commands
- **macOS (Homebrew paths for sandboxed agents)**:
  - Frontend: `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
  - Go tests: `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
  - macOS binary: `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .`
- **Windows (`bin\windows\vex.exe`)**:
  - PowerShell: `cd frontend; deno task typecheck; deno task test; deno task build; cd ..; go test -race -count=1 ./...; go build -o bin\windows\vex.exe .`

## Canonical References
- Overview & build/run: [README.md](README.md)
- System architecture & diagrams: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- Card layout blueprints: [docs/TEMPLATE_BLUEPRINTS.md](docs/TEMPLATE_BLUEPRINTS.md)
- Roadmap & open items: [TODO.md](TODO.md)
- LLM codebase index: [llms.txt](llms.txt)
- Workflow rules & skills: [.agent/rules/vex-workflow.md](.agent/rules/vex-workflow.md), [.agent/skills/](.agent/skills/)