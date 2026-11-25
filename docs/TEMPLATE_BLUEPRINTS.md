# Vex — 9 Pure Generic Card Layout Blueprints & Data Examples

This reference documents the **visual layout blueprint**, **strict zero-alias `data` contract** (`additionalProperties: false`), and **concrete JSON examples** for all **9 pure generic layouts** registered in Vex (`CardTemplateRegistry`). Validate any module manifest with `validate_module` (MCP) or `vex validate <module.json>` (CLI) before publishing.

---

## 1. Universal Card Shell & Nesting Rules

Every card—regardless of layout—is rendered by `RolodexCardShellComponent` inside a responsive `420px–520px` (default `460px`, `max-height: 680px`) internal-scrolling frame with **Light & Dark mode** (`[data-theme="light"]` / `[data-theme="dark"]`).

```text
╔══════════════════════════════════════════════════════════════╗  ← Level 0 Card
║  [Icon]  Title (15px semibold)                    [📌]  [▼]  ║    Light: #ffffff / border #e4e7ec / r=12px / p=12px
║  [Chip 1]  [Chip 2]  [Chip 3]  [Chip 4]                      ║    Dark:  #18181b / border #27272a
╟──────────────────────────────────────────────────────────────╢  ← 1px divider (ONLY when expanded)
║  ┌─ Layout Leaf Body (optional, via NgComponentOutlet) ───┐  ║
║  └────────────────────────────────────────────────────────┘  ║
║  ┌────────────────────────────────────────────────────────┐  ║  ← Level 1 Child (100% width, NO left rail, NO pin)
║  │ [Icon] Nested Title (14px semibold)                [▼] │  ║    Light: #f8f9fb / border #e4e7ec / r=10px / p=10px
║  │ [Chip 1]  [Chip 2]  [Chip 3]                           │  ║    Dark:  #202024 / border #2e2e33
║  ├────────────────────────────────────────────────────────┤  ║
║  │ ┌────────────────────────────────────────────────────┐ │  ║  ← Level 2 Child (100% width, max depth = 2)
║  │ │ [Icon] Deep Child Title (14px semibold)        [↗] │ │  ║    Light: #ffffff / border #e4e7ec / r=10px / p=10px
║  │ │ [● PROD]  [Direct Link]                            │ │  ║    Dark:  #18181b / border #2e2e33
║  │ └────────────────────────────────────────────────────┘ │  ║
║  └────────────────────────────────────────────────────────┘  ║
╚══════════════════════════════════════════════════════════════╝
   Vertical gap between stacked children: 8px (zero left-rail lines)
```

### Universal Header, Chip & Nesting Rules
- **Flat-on-Disk Composition**: Store all cards flat in `ModuleManifest.cards[]` and link child cards to a parent via `"parentEntityKey": "<parent.entityKey>"` (hydrated dynamically up to depth 2 via SQLite `entity_links`).
- **Pin Button (`📌`)**: Level 0 only (`depth === 0 && pinnable !== false`), max **3 pins** globally. Active pinned state always displays the `--chip-blue-bg` / `--chip-blue-border` pill container (including on `link` cards).
- **Zero Empty Accordions**: Expand chevron (`▸` / `▼`) renders ONLY when the card has renderable leaf content OR resolved `children[]`.
- **Glance Chips (`chips[]`, max 4, zero title/chip echo)**:
  - **Tones**: `"neutral"`, `"blue"`, `"amber"`, `"green"`, `"red"`, `"mono"`.
  - **Stoplight Dots (`dot`)**: Optional `"green" | "amber" | "red"`.
  - **Live IANA Clock Chip**: Any chip whose `label` is a valid IANA timezone (e.g., `"Europe/Amsterdam"`, `"America/Los_Angeles"`) automatically renders a live 24-hour monospace clock (`08:09`) across all 9 layouts.

### Quick Layout Picker

| Layout (`layout`) | Use When the User Wants To... | Strict `data` Contract |
| :--- | :--- | :--- |
| **`group`** | Drill into stacked child cards under one entity | `{ meta?: Record<string, string \| number> }` *(Header-only)* |
| **`link`** | Click once to open 1 URL (`↗`, no chevron) | `{ url: string }` |
| **`action-list`** | Pick among env hosts, multi-env portals, commands, or contacts | `{ rows: [{ badge?, primary, mono?, subtitle?, actions: [{ label, type: "copy"\|"open_url", value, tone?, dot? }] }] }` |
| **`facts`** | Scan structured key/value specs (with optional unit/variant switcher) | `{ summary?, facts?: [{ label, value, mono?, copyable? }], views?: [{ label, facts: [...] }] }` |
| **`segments`** | Decode parts of a structured ID / code | `{ segments: [{ value, role, meaning, tone: "blue"\|"amber"\|"green"\|"neutral" }], explanation }` |
| **`steps`** | Follow a numbered lifecycle, SOP, or checklist | `{ steps: [{ label, caption? }], activeStep?: number, summary, bullets?: string[] }` |
| **`convert`** | View & copy an `Input ⇄ Output` calculation | `{ input, output, equivalents?: [{ label, value }] }` |
| **`table`** | Compare 2–4 columns across ≤8 visible rows & export CSV/MD | `{ columns: string[], rows: string[][], monoColumns?: number[], sourceUrl? }` |
| **`snippet`** | Fill inline `{{vars}}` and copy a monospace query/template | `{ language?, body: string, vars?: [{ key, label, defaultValue? }] }` |

---

## 2. The 9 Pure Generic Layouts

### 2.1 `group` — Header-Only Container (`GroupCardComponent`)
```text
┌──────────────────────────────────────────────────────────┐
│ 📦  40' High Cube (40' HC)                     [📌]  [▼] │
│ [40'×8'×9'6"]  [76.4 CBM]  [26.5t max]                   │
├──────────────────────────────────────────────────────────┤
│ ┌──────────────────────────────────────────────────────┐ │  ← Child 1 (parentEntityKey: "container:40HC")
│ │ 📐  Dimensions & Door Clearance                  [▸] │ │
│ └──────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```
```json
{
  "id": "container-40hc",
  "entityKey": "container:40HC",
  "title": "40' High Cube (40' HC)",
  "collection": "containers",
  "layout": "group",
  "icon": "📦",
  "chips": [
    { "label": "40'×8'×9'6\"", "tone": "mono" },
    { "label": "76.4 CBM", "tone": "amber" }
  ]
}
```

### 2.2 `link` — Single-Button Launch Card (`LinkCardComponent` / Shell)
```text
┌──────────────────────────────────────────────────────────┐
│ 🌐  CBP ACE Portal                             [📌]  [↗] │  ← Entire card is one <button>; NO chevron
│ [● PROD]  [Direct Link]                                  │
└──────────────────────────────────────────────────────────┘
```
```json
{
  "id": "sea-cbp-ace-portal",
  "parentEntityKey": "loc:SEA",
  "title": "CBP ACE Portal",
  "collection": "gateways",
  "layout": "link",
  "icon": "🌐",
  "chips": [
    { "label": "PROD", "tone": "red", "dot": "red" },
    { "label": "Direct Link", "tone": "neutral" }
  ],
  "data": { "url": "https://ace.cbp.dhs.gov" }
}
```

### 2.3 `action-list` — Compact Action Rows (`ActionListCardComponent`)
```text
┌──────────────────────────────────────────────────────────┐
│ 🖥️  AMS EDI Servers                                  [▼] │
│ [● DEV]  [● QA]  [● PROD]                                │
├──────────────────────────────────────────────────────────┤
│ ┌──────────────────────────────────────────────────────┐ │
│ │ [● DEV]  ams-dev-edi01.local:2222   [Copy] [SSH] [↗] │ │
│ │          Sandbox SFTP                                │ │
│ └──────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```
```json
{
  "id": "ams-edi-servers",
  "parentEntityKey": "loc:AMS",
  "title": "AMS EDI Servers",
  "collection": "gateways",
  "layout": "action-list",
  "icon": "🖥️",
  "chips": [
    { "label": "DEV", "tone": "green", "dot": "green" },
    { "label": "PROD", "tone": "red", "dot": "red" }
  ],
  "data": {
    "rows": [
      {
        "badge": { "label": "DEV", "tone": "green", "dot": "green" },
        "primary": "ams-dev-edi01.local:2222",
        "mono": true,
        "subtitle": "Sandbox SFTP",
        "actions": [
          { "label": "Copy", "type": "copy", "value": "ams-dev-edi01.local" },
          { "label": "SSH", "type": "copy", "value": "ssh ops@ams-dev-edi01.local -p 2222" },
          { "label": "Open", "type": "open_url", "value": "https://ams-dev-edi01.local" }
        ]
      }
    ]
  }
}
```

### 2.4 `facts` — Read-Only Spec Grid & View Switcher (`FactsCardComponent`)
- Render `[Copy]` **only** when `fact.copyable === true` (high-friction values only; short glance codes stay read-only).
```text
┌──────────────────────────────────────────────────────────┐
│ 📐  Dimensions & Door Clearance                      [▼] │
│ [ISO 45G1]  [Metric / Imperial]                          │
├──────────────────────────────────────────────────────────┤
│ [ Metric | Imperial ]                                    │
│ ┌─────────────────────┐  ┌─────────────────────────────┐ │
│ │ INTERNAL LENGTH     │  │ DOOR OPENING                │ │
│ │ 12.03 m             │  │ 2.34 m × 2.58 m             │ │
│ └─────────────────────┘  └─────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```
```json
{
  "id": "container-40hc-dims",
  "parentEntityKey": "container:40HC",
  "title": "Dimensions & Door Clearance",
  "collection": "containers",
  "layout": "facts",
  "icon": "📐",
  "data": {
    "views": [
      {
        "label": "Metric",
        "facts": [
          { "label": "Internal Length", "value": "12.03 m", "mono": true },
          { "label": "Door Opening", "value": "2.34 m × 2.58 m", "mono": true }
        ]
      },
      {
        "label": "Imperial",
        "facts": [
          { "label": "Internal Length", "value": "39' 6\"", "mono": true },
          { "label": "Door Opening", "value": "7' 8\" × 8' 5\"", "mono": true }
        ]
      }
    ]
  }
}
```

### 2.5 `segments` — 2–4 Column Token Breakdown (`SegmentsCardComponent`)
```text
┌──────────────────────────────────────────────────────────┐
│ 🧩  House Bill · 7031234567                    [📌]  [▼] │
│ [HBL]  [Valid ✓]  [Origin: SEA]                          │
├──────────────────────────────────────────────────────────┤
│ ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│ │      7       │  │      03      │  │   1234567    │     │
│ │     MODE     │  │    BRANCH    │  │   SEQUENCE   │     │
│ │ Ocean Import │  │ Seattle SEA  │  │  Shipment #  │     │
│ └──────────────┘  └──────────────┘  └──────────────┘     │
│ Forwarder-issued House Bill of Lading linking an         │
│ ocean import shipment to a master consolidation.         │
└──────────────────────────────────────────────────────────┘
```

### 2.6 `steps` — N-Step Lifecycle Timeline (`StepsCardComponent`)
```text
┌──────────────────────────────────────────────────────────┐
│ 📖  What is a House Bill (HBL)?                      [▼] │
│ [Step 2/4 · Origin]  [Dictionary]                        │
├──────────────────────────────────────────────────────────┤
│ (1)────── (2)────── (3)────── (4)                        │
│ Booking   Origin    Transit   Release                    │
│ A House Bill of Lading (HBL) is issued by a forwarder... │
│ • Links individual cargo to the carrier's MBL            │
└──────────────────────────────────────────────────────────┘
```

### 2.7 `convert` — `Input ⇄ Output` Strip (`ConvertCardComponent`)
```text
┌──────────────────────────────────────────────────────────┐
│ ⇄  100 kg = 220.46 lbs                         [📌]  [▼] │
│ [Weight]  [Instant]                                      │
├──────────────────────────────────────────────────────────┤
│ [ 100 kg  ⇄  220.46 lbs                         [Copy] ] │
│ [Kilograms  100 kg]  [Metric Tons  0.10 t]               │
└──────────────────────────────────────────────────────────┘
```

### 2.8 `table` — Sticky Data Table with CSV/MD Export (`TableCardComponent`)
```text
┌──────────────────────────────────────────────────────────┐
│ 📊  ISO 6346 Size Codes                              [▼] │
│ [Reference]                                              │
├──────────────────────────────────────────────────────────┤
│ CODE   │ TYPE           │ VOLUME                         │  ← Sticky header, max 8 visible rows
│ 22G1   │ 20' Dry        │ 33.2 CBM                       │
│ 45G1   │ 40' High Cube  │ 76.4 CBM                       │
│ 2 rows                              [Copy CSV] [Copy MD] │
└──────────────────────────────────────────────────────────┘
```
```json
{
  "id": "iso-size-table",
  "title": "ISO 6346 Size Codes",
  "collection": "containers",
  "layout": "table",
  "icon": "📊",
  "data": {
    "columns": ["Code", "Type", "Volume"],
    "rows": [
      ["22G1", "20' Dry", "33.2 CBM"],
      ["45G1", "40' High Cube", "76.4 CBM"]
    ],
    "monoColumns": [0, 2]
  }
}
```

### 2.9 `snippet` — Inline `{{var}}` Interpolated Code Block (`SnippetCardComponent`)
```text
┌──────────────────────────────────────────────────────────┐
│ 💻  Tail Pod Logs                                    [▼] │
│ [Kubernetes]                                             │
├──────────────────────────────────────────────────────────┤
│ NAMESPACE [ prod       ]   POD [ api-0               ]   │
│ ┌──────────────────────────────────────────────────────┐ │
│ │ BASH                                          [Copy] │ │
│ │ kubectl logs -n prod api-0 --tail=200                │ │
│ └──────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```
```json
{
  "id": "snippet-k8s-logs",
  "title": "Tail Pod Logs",
  "collection": "devops",
  "layout": "snippet",
  "icon": "💻",
  "data": {
    "language": "bash",
    "body": "kubectl logs -n {{namespace}} {{pod}} --tail=200",
    "vars": [
      { "key": "namespace", "label": "Namespace", "defaultValue": "prod" },
      { "key": "pod", "label": "Pod", "defaultValue": "api-0" }
    ]
  }
}
```
