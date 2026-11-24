---
name: doc-drift-sweep
description: Post-phase, pre-release, and pre-merge documentation drift checklist for Vex. Verifies that README.md, docs/ARCHITECTURE.md, TODO.md, workspace skills, workflow rules, and live MCP/JSON schemas accurately reflect the codebase without unnecessary prose churn.
---

# Vex — Documentation Drift Sweep

Run this targeted 6-point audit **at the end of every implementation phase** and before merging to `main` or cutting a release. **Rule**: Fix every genuine mismatch between code and documentation — never leave stale stack, flag, count, or schema claims behind.

## 1. Package, Frontend Stack & Cross-Agent Sync
- List directories under `internal/` and verify every package matches:
  - `.agent/skills/vex-architecture/SKILL.md` (Acyclic Package Dependency Graph)
  - `.agent/rules/vex-workflow.md` (Architecture Guardrails)
  - `AGENTS.md` and `.github/copilot-instructions.md` (Cross-agent Gemini + Copilot parity)
  - `llms.txt` (LLM codebase index)
  - `docs/ARCHITECTURE.md` (Mermaid diagram & feature sections)
- Verify frontend framework, template registry (`CardTemplateRegistry`), test harness (`vitest.config.mts` / `scripts/test-aot.mjs`), and build pipeline (`ngc` + `esbuild` / `angular.json`) match `.agent/skills/vex-rolodex-ui/SKILL.md`, `.agent/skills/vex-frontend-testing/SKILL.md`, and `.agent/skills/vex-template-authoring/SKILL.md`.

## 2. Live MCP Contracts & JSON Schema Sync
- Compare `models.SearchCard`, `models.CardNode`, `models.GlanceChip`, `models.ModuleManifest`, and `models.PatternRule` (`internal/models/`) against `models.CardJSONSchema()` (`schema://search-card`).
- Count registered MCP tools (`AddTool(...)`) and resources (`AddResource` / `AddResourceTemplate`) in `internal/mcp/server.go` and confirm the counts and names match `SKILL.md`, `README.md`, and `docs/ARCHITECTURE.md`.

## 3. CLI Flags, Environment & Config Sync
- Compare `flag.*` definitions in `main.go` and fields in `RuntimeConfig` / `UserPrefs` (`app_service.go`) against:
  - `README.md` (Run commands & flags)
  - `.env.example` (build/testing environment variables)
  - `.agent/skills/vex-architecture/SKILL.md`

## 4. Build Commands & Dependency Prerequisites
- Confirm Go & frontend commands (`-tags private_mac_apis -o bin/macos/vex .` on macOS, `go build -o bin\windows\vex.exe .` on Windows, Deno `deno task typecheck && deno task test && deno task build`, `go test -race -count=1 ./...`) match across `README.md`, `AGENTS.md`, `SKILL.md`, `TODO.md`, and `.agent/rules/vex-workflow.md`.
- Confirm `AppVersion` in `buildinfo.go` matches docs.

## 5. TODO.md & Repo Hygiene
- Check `TODO.md`: mark completed phase items `[x]`, document the latest phase cleanly, and prune obsolete backlog items.
- Verify `.gitignore` excludes local secrets (`.env`, `.env.*` except `.env.example`), build artifacts (`bin/`, `*.exe`, `*.app/`), frontend build caches (`frontend/node_modules/`, `frontend/.angular/`, `frontend/out-tsc/`), and local DBs/logs (`*.db`, `*.jsonl`, `*.log`).

## 6. Report Findings Concisely
- Summarize real drift found and fixed (or confirm **"Zero drift detected"**), with clickable `file://` links to all updated files.
