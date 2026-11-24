---
name: vex-template-authoring
description: Authoring guide for creating generic, reusable Vex full-width stacked card templates, optional JSON modules (~/.vex/modules/), configurable PatternRules (~/.vex/patterns/ or ModuleManifest.patterns), and sharing agent skills via live MCP tools.
---

# Vex — Generic Template, Optional Module & Pattern Authoring Guide

Use this skill whenever creating new card modules, transforming raw data into Vex cards, updating starter packs, or authoring pattern-matching and cross-card linking rules via JSON or MCP. For full ASCII layout blueprints, slot-to-field tables, and multi-domain JSON examples of every template, see `docs/TEMPLATE_BLUEPRINTS.md`.

## 1. Golden Rule: Use the 5 Canonical Generic Template Classes

Every card renders inside the full-width stacked card shell (zero left-rail tree lines, alternating inset surfaces for L0/L1/L2, Light & Dark mode). Prefer the **5 canonical generic templates** so any team or LLM can transform data into consistent, nestable forms:

| Template (`template`) | Layout Contract | Example Content Reuse Across Domains |
| :--- | :--- | :--- |
| **`location`** | **Header-only** container (`[UNLOC · IATA]` blue mono, live `[HH:MM]` clock, `[CUR]` green, `[Temp]` neutral). **NO street addresses or phones.** Expands directly into full-width nested child cards. | Gateways (`Amsterdam Gateway (AMS)`), Regional Offices, Warehouses, Cloud Regions (`us-west-2`), Data Centers. |
| **`server-matrix`** | Always 3 stoplight chips (`● DEV`, `● QA`, `● PROD`) + compact monospace rows with inline `[Copy]`, `[SSH]`, and `[↗]` buttons. | EDI Servers, Database Hosts, Kubernetes Clusters, API Endpoints, Bastion Hosts, Brokers. |
| **`website-link`** | **Multi-Chip Mode (2+ links)**: expandable header (`● QA`, `● PROD`, `N links`) revealing launch/copy rows. **Single-Button Mode (1 link)**: omits chevron; whole card clicks to open with top-right `↗`. | Customs & Port Links, CBP ACE Portal, Carrier Tracking, Dashboards, Admin Portals, Runbooks. |
| **`pattern-breakdown`** | `[Type]` blue, `[Valid ✓]` green, `[Origin]` neutral chips + **3-column Blue/Amber/Green segment tile grid** (`value`, `role`, `meaning`) + 1–2 sentence `explanation` + full-width nested entity cards. | HBLs (`7031234567`), IATA MAWBs (`176-12345675`), ISO 6346 Containers, Ticket IDs, Invoice/PO Numbers. |
| **`conversion`** | Compact `Input ⇄ Output` strip with 1-click Copy + secondary equivalent chips. | Weight/Volume/Temp/Currency conversions, IATA Dimensional Weight, Data size conversions. |

Additional optional templates supported by the registry: `container-blueprint` (theme-adaptive SVG schematic + Metric/Imperial toggle + capacity boxes), `dictionary-guide` (N-step process timeline + summary), and `kv-grid` (generic click-to-copy key-value metadata grid for indexes like Air Carriers & Ocean SCACs).

## 2. Canonical Generic JSON Data Contracts

### A. `location` (Header-Only Container — NO Addresses/Phones)
```json
{
  "id": "loc-ams",
  "entityKey": "loc:AMS",
  "title": "Amsterdam Gateway (AMS)",
  "collection": "gateways",
  "template": "location",
  "icon": "🇳🇱",
  "chips": [
    { "label": "NLAMS · AMS", "tone": "blue" },
    { "label": "08:09", "tone": "mono" },
    { "label": "EUR €", "tone": "green" },
    { "label": "14°C", "tone": "mono" }
  ],
  "data": { "ianaTimezone": "Europe/Amsterdam", "unlocode": "NLAMS", "iata": "AMS" },
  "children": [ "...server-matrix and website-link child cards..." ]
}
```

### B. `server-matrix` (`[Copy]`, `[SSH]`, `[↗]`)
Supports either `servers` or generic `rows`:
```json
{
  "id": "ams-edi-servers",
  "title": "AMS EDI Servers",
  "collection": "gateways",
  "template": "server-matrix",
  "icon": "🖥️",
  "chips": [
    { "label": "DEV", "tone": "green", "dot": "green" },
    { "label": "QA", "tone": "amber", "dot": "amber" },
    { "label": "PROD", "tone": "red", "dot": "red" }
  ],
  "data": {
    "rows": [
      { "badge": "DEV", "tone": "green", "value": "edi-ams-dev.internal", "sshCommand": "ssh ops@edi-ams-dev.internal", "url": "https://edi-ams-dev.internal" },
      { "badge": "QA", "tone": "amber", "value": "edi-ams-qa.internal", "sshCommand": "ssh ops@edi-ams-qa.internal", "url": "https://edi-ams-qa.internal" },
      { "badge": "PROD", "tone": "red", "value": "edi-ams-prd.internal", "sshCommand": "ssh ops@edi-ams-prd.internal", "url": "https://edi-ams-prd.internal" }
    ]
  }
}
```

### C. `website-link` (Multi-Chip Mode & Single-Button Mode)
**Multi-Chip Mode (2+ links):**
```json
{
  "id": "ams-customs-links",
  "title": "AMS Customs & Port Links",
  "collection": "gateways",
  "template": "website-link",
  "icon": "🔗",
  "chips": [
    { "label": "QA", "tone": "amber", "dot": "amber" },
    { "label": "PROD", "tone": "red", "dot": "red" },
    { "label": "2 links", "tone": "neutral" }
  ],
  "data": {
    "bookmarks": [
      {
        "name": "Dutch Customs DMS Portal",
        "subtitle": "Import/export declarations",
        "links": [
          { "label": "QA", "tone": "amber", "url": "https://dms-qa.douane.nl" },
          { "label": "PROD", "tone": "red", "url": "https://dms.douane.nl" }
        ]
      }
    ]
  }
}
```
**Single-Button Mode (1 link — omits chevron, entire card is a clickable button with top-right `↗`):**
```json
{
  "id": "sea-cbp-ace-portal",
  "title": "CBP ACE Portal",
  "collection": "gateways",
  "template": "website-link",
  "icon": "🌐",
  "chips": [
    { "label": "PROD", "tone": "red", "dot": "red" },
    { "label": "Single Link", "tone": "neutral" }
  ],
  "data": {
    "url": "https://ace.cbp.dhs.gov"
  }
}
```

### D. `pattern-breakdown` (3-Column Segment Tile Grid + Explanation + Nested Entities)
Authored inside `PatternRule` (`ModuleManifest.patterns` or `~/.vex/patterns/<id>.json`):
```json
{
  "id": "hbl-ocean-import",
  "name": "House Bill of Lading (HBL)",
  "priority": 100,
  "normalize": ["trim", "uppercase", "strip-spaces"],
  "regex": "^(?P<mode>[6789])(?P<branch>\\d{2})(?P<seq>\\d{7})$",
  "titleTemplate": "House Bill · {{0}}",
  "template": "pattern-breakdown",
  "icon": "🧩",
  "defaultExpanded": true,
  "chips": [
    { "label": "HBL", "tone": "blue" },
    { "label": "Valid ✓", "tone": "green" },
    { "label": "Origin: {{lookup.branch}}", "tone": "neutral" }
  ],
  "segments": [
    { "value": "{{mode}}", "role": "MODE", "meaning": "{{lookup.mode}}", "tone": "blue" },
    { "value": "{{branch}}", "role": "BRANCH", "meaning": "{{lookup.branchLabel}}", "tone": "amber" },
    { "value": "{{seq}}", "role": "SEQUENCE", "meaning": "Shipment #", "tone": "green" }
  ],
  "explanationTemplate": "Forwarder-issued House Bill of Lading linking an individual shipment to a master consolidation.",
  "linkRules": [
    { "targetEntityKey": "loc:{{lookup.branch}}", "autoExpand": true, "nestChildren": true, "maxDepth": 2 }
  ]
}
```

### E. `conversion` (Compact `Input ⇄ Output` Strip)
Emitted automatically by `TryConvertQuery` or usable in static cards:
```json
{
  "id": "conv-weight",
  "title": "Weight Conversion",
  "collection": "conversions",
  "template": "conversion",
  "icon": "⚖️",
  "defaultExpanded": true,
  "data": {
    "input": "100 kg",
    "output": "220.46 lb",
    "equivalents": [
      { "label": "Metric Ton", "value": "0.10 t" },
      { "label": "Ounces", "value": "3,527.40 oz" }
    ]
  }
}
```

## 3. Optional Module Packaging Rules

- **All modules are optional and deletable**: Packaged starter packs live in `internal/indexer/starter_modules/*.json` and are only installed into `~/.vex/modules/<id>.json` when chosen by the user at onboarding, via the **Modules** dialog, or via MCP (`install_starter_module`).
- **Canonical `entityKey` Naming**:
  - Locations / Gateways: `loc:<CODE>` (e.g. `loc:AMS`, `loc:SEA`)
  - Air Carriers: `air-carrier:<3-digit-prefix>` (e.g. `air-carrier:176`)
  - Ocean Carriers: `ocean-carrier:<SCAC>` (e.g. `ocean-carrier:MAEU`)
  - Equipment / Containers: `container:<SHORTCODE>` (e.g. `container:40HC`)
  - Guides / Dictionary: `guide:<slug>` (e.g. `guide:hbl`, `guide:mawb`)
  - Reusable Templates: `template:<slug>`

## 4. MCP Workflows for LLMs & Agent Skill Sharing

MCP clients (Claude Desktop, Cursor, Copilot, VS Code) connect via `http://127.0.0.1:17380/mcp` or `vex --mcp-stdio`:

1. **Discover & Share Agent Skills**:
   - Call `list_skills` (or read `search://skills`) to list installed Vex authoring/UI/architecture skills from `~/.vex/skills/` and workspace `.agent/skills/`.
   - Call `get_skill(name)` (or read `skill://{name}`) to load this skill or `vex-rolodex-ui` directly into any external agent's context!
2. **Inspect Schemas & Live Data**:
   - Call `get_card_schema` (or read `schema://search-card`) for full JSON schemas of `SearchCard`, `CardNode`, `GlanceChip`, `PatternSegment`, `ModuleManifest`, `PatternRule`, and `ETLMappingRule`.
   - Call `search_knowledge(query)` to retrieve both raw `SearchCard` records and projected `CardNode` trees, or `get_related_cards(entity_key)` for cross-module graph lookups.
3. **Transform Raw Data & Enrich Cards**:
   - Call `preview_raw_data_sample` → `transform_and_import_data` to turn CSV/TSV/JSON files into Vex cards.
   - Call `upsert_card` / `batch_upsert_cards` to enrich cards or push custom forms.
4. **Create & Update Modules & Patterns Live**:
   - Call `list_modules` / `get_module` / `upsert_module` / `delete_module` / `install_starter_module` to manage `~/.vex/modules/*.json` with instant SQLite re-indexing.
   - Call `list_pattern_rules` / `upsert_pattern_rule` / `delete_pattern_rule` to manage `~/.vex/patterns/*.json` live.
