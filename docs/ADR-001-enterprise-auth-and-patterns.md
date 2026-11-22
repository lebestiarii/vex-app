# ADR-001: Enterprise Auth (`gh` / `az`), MCP Delegation & Declarative Pattern Recognition

- **Status**: Accepted & Implemented
- **Audience**: Engineers, Security Reviewers, and AI Coding Agents (Handoff Reference)

---

## 1. Enterprise Connections (`gh auth` / `az login` for GitHub & Azure DevOps)

### Decision
**Do NOT extract, scrape, or cache raw OAuth/bearer tokens from `gh auth token` or `az account get-access-token` inside Vex.**

Instead, Vex uses a **two-layer MCP-first architecture**:
1. **Isolated MCP Server Delegation (`internal/mcp/client.go`, `EnterpriseMCPPresets()`)**:
   - Connect Vex's built-in MCP client to the official external MCP servers (`github-mcp-server` for GitHub; `@azure-devops/mcp` for Azure DevOps) over `stdio` or `http`.
   - Authentication stays inside the external MCP server process or the user's native OS broker — Vex never holds GitHub PATs or Entra ID access tokens in memory or on disk.
   - All enterprise MCP presets default to **`Enabled: false`** (explicit opt-in) and **`LocalOnly: true`** (every indexed tool/resource card and every live pattern response is automatically excluded from external cloud LLM RAG prompts in `internal/ai/service.go`, while remaining 100% searchable locally and via local Ollama).
2. **Read-Only CLI Auth Probe (`internal/mcp/auth_probe.go`, `ProbeCLIAuth`)**:
   - Runs strictly read-only identity diagnostics (`gh auth status`, `az account show --output json`, 2.5 s timeout) to report whether the user's workstation already has a signed-in `gh` or `az` session ready for MCP delegation.
   - Enforced by unit test (`TestProbeCLIAuthReadOnlyAndSafe`): never invokes `token` or `get-access-token` subcommands.

### Risk Assessment Summary

| Approach | Risk Level | Rationale |
| :--- | :--- | :--- |
| Parsing `gh auth token` / `az account get-access-token` into Vex | **High (Rejected)** | Broad CLI OAuth scopes (`repo`, `write:org`, subscription-wide Azure tokens), bearer-token leakage risk in logs/memory, and potential corporate Conditional Access / DLP friction. |
| Official GitHub & Azure DevOps MCP Servers + `localOnly: true` | **Low (Accepted)** | Process-isolated credentials, least-privilege MCP tool surface, automatic `localOnly: true` cloud-LLM isolation, and zero new auth code to maintain in Vex. |

---

## 2. Declarative Pattern Recognition, Reference Decoding & Live Enrichment

### Design Intent
Users frequently paste opaque identifiers — shipment references (`7031234567`), Air Waybills (`176-12345675`), ISO 6346 containers (`MSCU123456...`), Azure DevOps work items (`AB#48219`), or GitHub issues (`org/repo#42`) — expecting instant structured context without remembering which portal or module to open.

To keep Vex's core repository **100% generic** (zero hardcoded company branch codes or proprietary hostnames), pattern recognition is driven by `models.PatternRule` (`internal/models/pattern.go`) and evaluated inside `search.Service.Search` (`internal/search/patterns.go`), which means **Desktop GUI, `--tui`, and MCP clients all get identical decoding and enrichment with zero duplicated logic**.

### Built-In Public Standards (`search.BuiltInPatternRules()`)
1. **`iata-awb`**: 11-digit IATA Air Waybill (`176-12345675`) with airline prefix lookup and real **Mod-7 check-digit validation**.
2. **`iso-container`**: 11-character ISO 6346 intermodal container (`MSCU1234567`) with BIC owner prefix lookup and official **ISO 6346 weighted Mod-11 check-digit validation**.
3. **`ado-work-item`**: `AB#12345` / `ADO-12345` → decodes offline and queries connected `azure-devops` MCP server (`wit_get_work_item`, 1500 ms timeout), unpacking JSON fields into a `kv-grid` template card tagged `localOnly: true`. Degrades gracefully offline if disconnected.
4. **`github-issue`**: `owner/repo#42` → decodes offline and queries connected `github` MCP server (`get_issue`), tagged `localOnly: true`.

### Custom Team / Company Rules (`~/.vex/patterns/*.json`, `upsert_pattern_rule` MCP Tool & Hot-Reload)
Custom rules are loaded ahead of built-ins via `indexer.LoadActivePatternRules(patternsDir, modulesDir, enabledModuleIDs)`.
- **Live MCP Authoring**: External AI assistants (GitHub Copilot CLI, VS Code Copilot, Claude Desktop) can inspect `schema://search-card#/definitions/PatternRule` and call the **`upsert_pattern_rule`** MCP tool to validate the regex, write `~/.vex/patterns/<id>.json` atomically, and hot-reload `PatternEngine` immediately in memory.
- **Filesystem Hot-Reload**: Editing or dropping any `.json` file directly into `~/.vex/patterns/` is automatically detected by `indexer.Watcher` (`fsnotify`) and reloaded in memory with zero restarts.

**Drop-in Example (`~/.vex/patterns/shipment-refs.json`)** — decodes a 10-digit House/Master Bill like `7031234567` (`7` = Ocean Import, `03` = Seattle `SEA`) and cross-links `loc:SEA` so Vex automatically enriches the decoded card with Seattle branch details and surfaces `port-ussea` right below it:

```json
{
  "id": "house-bill-10",
  "name": "10-Digit Shipment Reference",
  "category": "logistics",
  "regex": "^(?P<mode>[0-9])(?P<branch>[0-9]{2})(?P<seq>[0-9]{7})$",
  "lookups": {
    "mode": {
      "7": "Ocean Import",
      "8": "Ocean Export"
    },
    "branch": {
      "03": "Seattle (SEA)",
      "44": "Amsterdam (AMS)"
    },
    "branch_entity": {
      "03": "loc:SEA",
      "44": "loc:AMS"
    }
  },
  "titleTemplate": "{{lookup.mode}} · {{match}}",
  "subtitleTemplate": "Origin Branch: {{lookup.branch}}",
  "collection": "ocean-freight",
  "template": "kv-grid",
  "fields": {
    "shipment_type": "{{lookup.mode}}",
    "origin_branch": "{{lookup.branch}}",
    "sequence": "{{seq}}"
  },
  "glancePills": ["{{lookup.mode}}", "{{lookup.branch}}"],
  "linkEntityKeys": ["{{lookup.branch_entity}}"],
  "localOnly": true
}
```

### How Cross-Card Enrichment Works
- Any lookup table named `<capture>_<suffix>` (e.g. `branch_entity`) automatically reads capture group `<capture>` (`branch`), allowing one regex capture to drive both human labels and canonical `EntityKey` links (`loc:SEA`).
- `PatternEngine.Match` calls `store.FindCardsByEntityKeys(ctx, resolvedKeys)` (`internal/indexer/store.go`), attaches a summary `CardSection` onto the decoded Hero Card, and returns the linked cards as `MatchSource: "pattern-enrichment"` runner-ups right beneath the Hero Card.
