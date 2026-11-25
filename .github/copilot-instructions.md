# GitHub Copilot Instructions — Vex

Follow the project-wide engineering, testing, and documentation contract in [AGENTS.md](../AGENTS.md) and [.agent/rules/vex-workflow.md](../.agent/rules/vex-workflow.md).

## Quick Orientation
- **Codebase Index**: Read [llms.txt](../llms.txt) for key file paths, Go backend packages, Angular UI shell files, and MCP server contracts.
- **Architecture & Templates**: Consult [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md), [docs/TEMPLATE_BLUEPRINTS.md](../docs/TEMPLATE_BLUEPRINTS.md), and the skills under `.agent/skills/` (`vex-architecture`, `vex-search-ranking`, `vex-rolodex-ui`, `vex-frontend-testing`, `vex-template-authoring`, `doc-drift-sweep`).
- **Non-Negotiable Invariants**:
  - Keep Go packages strictly acyclic (`models → indexer → search → plugins → mcp → main`).
  - Keep Angular UI platform-agnostic, full-width stacked (no accordion tree lines), and free of hardcoded sample data.
  - Keep `--mcp-stdio` `stdout` strictly JSON-RPC only, and wrap macOS Wails UI callbacks in `application.InvokeAsync`.
  - Update tests and run the `doc-drift-sweep` checklist whenever schemas, templates, MCP tools, or build toolchains change, keeping macOS and Windows documentation aligned.
