# GitHub Copilot Instructions — Vex

**Vex** is a desktop-first, platform-agnostic, local-first knowledge Rolodex, configurable pattern decoder, and MCP hub built with **Go + Wails v3** and **Angular 22** (Standalone Components + Signals + Zoneless).

This file is aligned with `AGENTS.md` and `.agent/rules/vex-workflow.md`. Consult workspace skills under `.agent/skills/` before making changes in their domain.

## Core Guardrails
- **Acyclic Go Packages:** `internal/models → internal/indexer → internal/search → internal/plugins → internal/mcp → main`.
- **UI Invariants:** Preserve Light/Dark tokens (`frontend/src/styles.css`), full-width stacked nested cards (no vertical tree lines, L0→L1→L2 inset surfaces), and platform-agnostic controls (no OS traffic lights, Windows buttons, or `⌘`/`⌥` glyphs).
- **Zero Hardcoded Sample Data:** Keep demo/sample data out of Go and `frontend/src/`. Optional packs live in `internal/indexer/starter_modules/*.json`.
- **macOS Main-Thread Safety:** Wrap Wails window/tray/menu mutations in callbacks with `application.InvokeAsync(func() { ... })`.
- **Clean `--mcp-stdio`:** Never print non-JSON-RPC output to `stdout`.
- **Sandbox-Safe Tests:** Never bind loopback TCP ports in unit tests; use `http.RoundTripper`, `client.NewInProcessClient`, and `httptest.NewRecorder`.

## Required Workflow Rhythm
Follow `.agent/skills/after-action-review/SKILL.md` for every multi-step task:
1. **Plan First:** Break work into **Phases → Stories (`X.1`, `X.2`, …)** with a **Model, Context & Reasoning Plan** table justifying strong vs. lighter models, confirm `feat/<topic>` branch (or `git worktree`), and **pause** for approval before coding.
2. **Verify & Sweep:** Run the verification commands below and the 8-point audit in `.agent/skills/doc-drift-sweep/SKILL.md` (covering diagrams, full-repo folder structure, schemas, and skill usefulness).
3. **AAR & Walkthrough:** Output gate/drift results, changes by story (`file://` links), what went well, what went poorly, skill add/improve/remove recommendations, compact vs. new-chat + next-model guidance, and a **User Verification Walkthrough**.
4. **Merge Only After Approval:** Once approved by the user, merge worktree → feature branch (if used) → `main` (never `git push` unless asked), and suggest pruning stale worktrees/branches.

## Verification Commands
- **macOS:**
  - `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
  - `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
  - `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .`
- **Windows (PowerShell):**
  - `cd frontend; deno task typecheck; deno task test; deno task build; cd ..; go test -race -count=1 ./...; go build -o bin\windows\vex.exe .`

## Workspace Skills Reference
- Workflow, Planning & AAR: `.agent/skills/after-action-review/SKILL.md`
- Git Branching & Worktree Flow: `.agent/skills/git-branch-and-worktree-flow/SKILL.md`
- Docs, Diagrams & Skill Drift Audit: `.agent/skills/doc-drift-sweep/SKILL.md`
- Architecture & MCP Hub: `.agent/skills/vex-architecture/SKILL.md`
- Angular 22 UI & Tokens: `.agent/skills/vex-rolodex-ui/SKILL.md`
- Offline Vitest Suite: `.agent/skills/vex-frontend-testing/SKILL.md`
- Search & SQLite Ranking: `.agent/skills/vex-search-ranking/SKILL.md`
- Templates, Modules & Patterns: `.agent/skills/vex-template-authoring/SKILL.md`