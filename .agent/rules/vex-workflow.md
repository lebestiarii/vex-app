# Vex Workspace Workflow & Engineering Rules

## 1. Toolchain & Sandbox Execution
- **Go Toolchain**: Always invoke `/opt/homebrew/Cellar/go/1.27.4/bin/go`.
  - When building the macOS desktop binary, always pass `-tags private_mac_apis` so Wails v3 WKWebView transparency (`MacBackdropTransparent`) renders without an opaque white backdrop:
    ```bash
    /opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/vex .
    ```
  - For build-time Edition overrides (`community` vs. `enterprise`), pass `-ldflags`:
    ```bash
    /opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -ldflags "-X main.Edition=enterprise" -o bin/vex .
    ```
- **Frontend Toolchain**: Always invoke `/opt/homebrew/Cellar/deno/2.9.8/bin/deno` inside `frontend/` (`deno task typecheck` and `deno task build`).
- **Sandbox-Safe Tests**: Never bind loopback TCP ports in unit tests; use injected `http.RoundTripper`, `client.NewInProcessClient`, and `httptest.NewRecorder`.

## 2. Git Branching Discipline
- Keep `main` clean and verified.
- Develop each new phase on a dedicated local feature branch (`feat/<topic>`).
- Merge back to `main` (`git merge --no-ff`) and delete the feature branch only when the phase is verified or requested by the user.
- Never push to a remote repository unless explicitly asked.

## 3. Architecture & UX Guardrails
- **Acyclic Go Package Graph**: `internal/models` → `internal/indexer` → `internal/search` → `internal/plugins` → (`internal/ai`, `internal/mcp`, `internal/tui`) → `main`.
- **Framework-Agnostic Backend Intelligence**: Keep module compilation, glance-pill generation, provenance tracking, unit/dimensional conversions, and GitHub taxonomy parsing in Go so any frontend layer (React, Angular, or Vanilla JS/CSS) or `--tui` can consume ready-to-render JSON/structs.
- **macOS Main-Thread Safety**: Always wrap Wails window/tray/menu mutations inside `GlobalShortcut` or background callbacks with `application.InvokeAsync(func() { ... })`.
- **Clean `--mcp-stdio`**: Never write non-JSON-RPC bytes to `stdout`.
- **Less Is More UI**: No UI taglines, catchphrases, or marketing copy.

## 4. Mandatory Phase-End Verification & Status Report
At the end of every phase:
1. Run the full verification gate:
   - `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
   - `/opt/homebrew/Cellar/go/1.27.4/bin/go test -race -count=1 ./...`
   - `/opt/homebrew/Cellar/go/1.27.4/bin/go build -tags private_mac_apis -o bin/vex .`
2. Output a structured Status Report covering:
   - **Testing & Build Gate Results**
   - **What Was Done**
   - **What Went Well**
   - **What Was Added / Modified** (with clickable `file://` links)
   - **Skill & Instruction File Recommendations**
   - **Context & Model Recommendations**
