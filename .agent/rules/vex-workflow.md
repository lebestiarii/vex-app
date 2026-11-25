# Vex — Antigravity Workspace Workflow & Guardrails

Always follow this rule alongside `AGENTS.md` and `.agent/skills/`. Every new feature, refactor, or multi-step chat MUST follow the exact Planning → Execution → Walkthrough → AAR → Merge rhythm defined in `.agent/skills/after-action-review/SKILL.md`.

## 1. Architecture & Codebase Guardrails
- **Acyclic Go Packages:** `internal/models → internal/indexer → internal/search → internal/plugins → internal/mcp → main`. Never introduce circular imports; expose interfaces from `internal/search` when wiring higher layers.
- **UI & Feel Stability:** Preserve Light/Dark CSS tokens (`frontend/src/styles.css`), full-width stacked card geometry (zero vertical accordion tree lines, L0→L1→L2 alternating inset surfaces), and interaction feel unless UI changes are explicitly requested.
- **Platform-Agnostic UI:** Never render OS-specific window chrome (traffic lights, Windows buttons) or OS-specific key glyphs (`⌘`, `⌥`, `Win`) in UI components.
- **Zero Hardcoded Sample Data:** Never bake demo/sample cards into Go or `frontend/src/`. Optional starter packs live exclusively in `internal/indexer/starter_modules/*.json` and user modules in `~/.vex/modules/*.json`.
- **macOS Main-Thread Safety:** Wrap all Wails window, tray, and menu mutations inside GlobalShortcut or background callbacks with `application.InvokeAsync(func() { ... })`.
- **Clean `--mcp-stdio`:** Never write non-JSON-RPC bytes or logs to `stdout`.
- **Sandbox-Safe Tests:** Never bind loopback TCP ports in unit tests; use `http.RoundTripper`, `client.NewInProcessClient`, and `httptest.NewRecorder`.

## 2. Branching, Worktree & Merge Discipline
- **Isolate All Work:** Develop on a local `feat/<topic>` branch. Keep `main` clean at all times.
- **Worktrees for Parallel or High-Risk Phases:** Use `git worktree` when isolating risky refactors or running parallel phases while keeping the primary checkout runnable.
- **No Unapproved Merges or Pushes:** Never merge a worktree into a feature branch, never merge a feature branch into `main`, and never run `git push` until the user explicitly approves the Phase-End Verification Walkthrough.
- **Post-Approval Cleanup:** Once approved, merge worktree → `feat/<topic>` (if applicable), merge `feat/<topic>` → `main`, and list stale worktrees (`git worktree list`) and merged branches (`git branch --merged main`) for pruning.

## 3. Mandatory Phase Rhythm (Delegates to `after-action-review` Skill)
1. **Pre-Phase Plan & Pause:** Before editing files, break work into **Phases → Stories (`X.1`, `X.2`, …)** with intent, file scope, and acceptance criteria, output the **AI Model, Context & Reasoning Plan** table (with intensity and strong vs. lighter model justification), confirm branch/worktree isolation, and **pause** for approval/model switching.
2. **Phase-End Verification Gate:** Run all relevant toolchain checks below and execute the **8-point Documentation, Diagram & Skill Drift Sweep** (`.agent/skills/doc-drift-sweep/SKILL.md`).
3. **Structured AAR & User Walkthrough:** Deliver all 7 required sections from `.agent/skills/after-action-review/SKILL.md` (gate/drift results, changes by story with `file://` links, what went well, what went poorly, skill/instruction add/improve/remove recommendations, compact vs. fresh-chat + next-model handoff, and a step-by-step **User Verification Walkthrough**). **Pause for user approval** before merging.

## 4. Build & Verification Toolchains
### macOS (Homebrew paths for sandboxed agents)
```bash
# 1. Frontend typecheck, offline Vitest suite & AOT/esbuild bundle
cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build

# 2. Go race-checked test suite
/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...

# 3. macOS desktop binary
/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/macos/vex .
```

### Windows (`bin\windows\vex.exe` — PowerShell)
```powershell
cd frontend; deno task typecheck; deno task test; deno task build; cd ..
go test -race -count=1 ./...
go build -o bin\windows\vex.exe .
```
*(Node.js 22.22+ `npm run ng:test` / `npm run ng:build` is also supported on Windows hosts.)*