---
name: doc-drift-sweep
description: Pre-release and pre-merge documentation drift checklist for Vex. Verifies that README.md, docs/ARCHITECTURE.md, TODO.md, SKILL.md, workflow rules, and live MCP/JSON schemas accurately reflect the codebase without unnecessary prose churn.
---

# Vex — Documentation Drift Sweep

Run this targeted 6-point audit before merging major phases to `main` or cutting a release. **Rule**: Only fix genuine mismatches between code and documentation — never invent problems or rewrite prose just to create diff churn.

## 1. Package & Architecture Diagram Sync
- List directories under `internal/` and verify every package appears accurately in:
  - `.agent/skills/vex-architecture/SKILL.md` (Acyclic Package Dependency Graph)
  - `docs/ARCHITECTURE.md` (Mermaid architecture diagram & feature sections)

## 2. Live MCP Contracts & JSON Schema Sync
- Compare `models.SearchCard`, `models.CardSection`, `models.Provenance`, and `models.ModuleManifest` in `internal/models/card.go` against `models.CardJSONSchema()` (`schema://search-card`).
- Count registered MCP tools (`AddTool(...)`) and resources in `internal/mcp/server.go` and confirm the count & names match `SKILL.md` and `docs/ARCHITECTURE.md`.

## 3. CLI Flags, Environment & Config Sync
- Compare `flag.*` definitions in `main.go` and fields in `RuntimeConfig` / `UserPrefs` (`app_service.go`) against:
  - `README.md` (Run commands & options)
  - `.env.example` (build/testing environment profile variables)
  - `SKILL.md` (User Preferences section)

## 4. Build Commands & Dependency Prerequisites
- Confirm Go & frontend build commands (`-tags private_mac_apis` on macOS, Deno & Node/npm commands, `go test -race -count=1 ./...`, `-o bin/vex .`) match across `README.md`, `SKILL.md`, and `.agent/rules/vex-workflow.md`.
- Confirm `AppVersion` in `buildinfo.go` is accurate for the current release state.

## 5. TODO.md & Repo Hygiene
- Check `TODO.md` and remove or check off any items that have already been implemented.
- Verify `.gitignore` excludes local secrets (`.env`, `.env.*` except `.env.example`), build artifacts (`bin/`, `*.exe`, `*.app/`), and local logs/DBs (`*.db`, `*.jsonl`, `*.log`).

## 6. Report Findings Concisely
- Summarize only real drift found and fixed (or confirm **"Zero drift detected"**), with clickable `file://` links to any updated files.
