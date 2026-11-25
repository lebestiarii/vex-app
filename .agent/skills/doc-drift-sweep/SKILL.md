---
name: doc-drift-sweep
description: Post-phase, pre-release, and pre-merge documentation, diagram, folder-structure, and skill-usefulness audit for Vex. Ensures docs, Mermaid/ASCII diagrams, schemas, and agent skills accurately match code reality and task intent.
---

# Vex — Documentation, Diagram & Skill Drift Sweep

Run this targeted **8-point audit** at the end of every implementation phase and before merging to `main` or cutting a release.

**Rule:** Fix every genuine mismatch between code and documentation—never leave stale diagrams, phantom folders, outdated counts, or unhelpful skills behind. Avoid unnecessary prose rewrites where facts are already accurate.

---

## 1. Architecture, Diagrams & Intent Accuracy
- **Mermaid Diagrams (`docs/ARCHITECTURE.md`)**: Trace every node and edge in the Mermaid flowchart against real packages and call paths. Remove deleted subsystems, add new packages/hooks, and verify arrow directions match actual runtime data flow (including acyclic Go boundaries).
- **ASCII Blueprints (`docs/TEMPLATE_BLUEPRINTS.md`)**: Verify every visual blueprint, chip slot, button label, and layout contract matches the live Angular template components (`frontend/src/app/templates/`).
- **Intent Alignment**: Confirm high-level capability descriptions in `README.md` and `docs/ARCHITECTURE.md` still match what the code *actually* does (no "planned" features described as shipped, and no shipped features missing).

## 2. Full-Repo Folder Structure & Stack Sync
- Inspect the real directory tree across the whole repo (`internal/`, `frontend/src/app/`, `.agent/`, `docs/`, `scripts/`)—not just `internal/`.
- Verify package lists, folder trees, frontend framework/version, template registry (`CardTemplateRegistry`), test harness (`vitest.config.mts`), and build pipeline (`ngc + esbuild` / `angular.json`) match across:
  - `AGENTS.md` and `.github/copilot-instructions.md` (Gemini + Copilot parity)
  - `.agent/rules/vex-workflow.md`
  - `.agent/skills/vex-architecture/SKILL.md`
  - `.agent/skills/vex-rolodex-ui/SKILL.md`, `vex-frontend-testing/SKILL.md`, `vex-template-authoring/SKILL.md`
  - `llms.txt` and `docs/ARCHITECTURE.md`

## 3. Skill Usefulness, Overlap & Intent Audit
Audit every file in `.agent/skills/*/SKILL.md` and `.agent/rules/*.md` against four questions:
1. **Task Accuracy:** Does each skill's description and instructions accurately match the task it is meant to perform today?
2. **Stale Contracts:** Does it reference removed templates, old field aliases, deprecated commands, or old schema versions?
3. **Overlap / Bloat:** Do two skills duplicate the same rules (causing conflicting guidance or wasted context tokens)?
4. **Net Usefulness:** Is the skill actively preventing errors or enforcing repeatable quality? Flag any low-value or redundant skill for **improvement, consolidation, or removal** in the AAR.

## 4. Live MCP Contracts & JSON Schema Sync
- Compare Go models (`internal/models/`) against `models.CardJSONSchema()` (`schema://search-card`) and ensure zero drift in required fields, enums, or layout names.
- Count registered MCP tools (`AddTool`), resources (`AddResource` / `AddResourceTemplate`), and prompts in `internal/mcp/server.go` and confirm counts and names match `README.md`, `docs/ARCHITECTURE.md`, `llms.txt`, and `vex-architecture/SKILL.md`.

## 5. CLI Flags, Environment & Config Sync
- Compare `flag.*` definitions in `main.go` and fields in `RuntimeConfig` / `UserPrefs` (`app_service.go`) against `README.md`, `.env.example`, and `vex-architecture/SKILL.md`.

## 6. Build Commands & Dependency Prerequisites
- Confirm macOS and Windows Go + Deno/Frontend commands and pinned toolchain versions match across `README.md`, `AGENTS.md`, `TODO.md`, `.agent/rules/vex-workflow.md`, and testing skills.
- Confirm `AppVersion` in `buildinfo.go` matches docs.

## 7. `TODO.md` & Repo/Git Hygiene
- Mark completed phase items `[x]` in `TODO.md`, record the latest phase concisely, and prune obsolete or superseded backlog items.
- Verify `.gitignore` excludes local secrets (`.env`, `.env.*` except `.env.example`), build artifacts (`bin/`, `*.exe`, `*.app/`), frontend caches (`frontend/node_modules/`, `frontend/.angular/`, `frontend/out-tsc/`), and local DBs/logs (`*.db`, `*.jsonl`, `*.log`).
- Check `git worktree list` and `git branch --merged main` so stale worktrees/branches are surfaced in the AAR.

## 8. Report Findings Concisely
- Summarize real drift found and fixed (grouped by Diagrams/Architecture, Folder Structure, Skills, Contracts/Docs), or confirm **"Zero drift detected"**, with clickable `file://` links to all updated files.