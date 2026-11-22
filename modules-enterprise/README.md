# External Enterprise Modules (`modules-enterprise/`)

Place external, organization-specific `models.ModuleManifest` JSON packs here to keep proprietary branch directories, internal hostnames, or enterprise knowledge completely separated from Vex's generic open/personal codebase.

> **Git Isolation:** `modules-enterprise/*.json` is listed in `.gitignore` so proprietary module JSON files are never committed into the core Vex repository. Every enterprise module should also set `"defaultEnabled": false` and `"localOnly": true` (on the manifest, every card, and every `provenance` block) so enterprise data is automatically excluded from external cloud LLM RAG calls (`openai`, `anthropic`, `gemini`) while remaining 100% searchable locally and via local Ollama.

## Quick 1-Line Install / Enterprise Vex Bundling

### Option A — Drop directly into a user's `~/.vex/modules/` directory (zero code changes)
```bash
mkdir -p ~/.vex/modules && cp modules-enterprise/<module-id>.json ~/.vex/modules/
```
Restart Vex (or open **Settings → Modules**) and toggle the module **ON**. If the pack replaces the 23 built-in gateways (`loc:<IATA>`), toggle **Geographies & Global Gateways** (`geographies`) **OFF** so built-in `air-freight`, `ocean-freight`, `customs`, `transcon`, and `global-tech` overlays attach directly to the enterprise location cards.

### Option B — Import via Desktop UI or Go RPC (`AppService.ImportModuleJSON`)
1. Open **Settings → Modules → Import Module JSON** and select `modules-enterprise/<module-id>.json`.
2. Or programmatically call `AppService.ImportModuleJSON(rawJSON)` — Vex validates every card via `models.ValidateAndNormalizeCard`, saves `<module-id>.json` into `~/.vex/modules/`, and enables it immediately.

### Option C — Bundle into an Enterprise Vex Distribution Package
Copy `modules-enterprise/*.json` into your Vex enterprise installer/distribution payload's seeded `modules/` directory (copied to `~/.vex/modules/` on first launch or via MDM/Intune/Jamf deployment). No Go or frontend code modifications are required — `indexer.LoadAllModules(modulesDir)` discovers all `*.json` manifests automatically at startup.
