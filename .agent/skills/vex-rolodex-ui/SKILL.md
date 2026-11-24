---
name: vex-rolodex-ui
description: Authoritative UI design system, layout contract, and Angular 19 component guide for the Vex Full-Width Stacked Nested Card System (Light/Dark Mode). Use whenever creating, styling, or updating Angular UI elements, Rolodex cards, chips, modals, or toasts in Vex.
---

# Vex — Full-Width Stacked Nested Card System (Angular 19 + Light/Dark Mode)

Use this skill whenever building or modifying UI elements in `frontend/`.

## 1. Visual System & Stacked Surface Hierarchy (Strict)

- **Widget Frame**: Responsive desktop widget supporting `420px`–`520px` width (default `460px`) with clean internal scrolling (`max-height: 680px`).
- **NO Accordion Tree Lines**: **NEVER** render vertical left-rail lines (`border-left`), branch connectors, or indented tree guides.
- **100% Full-Width Nesting**: Every nested child card spans **100% of its parent's inner width**, stacked vertically with `gap: 8px`.
- **Desktop-First & Strictly Platform-Agnostic**:
  - **NEVER** render OS-specific window buttons (no macOS traffic lights, no Windows minimize/maximize/close chrome).
  - **NEVER** hardcode OS-specific key glyphs (`⌘`, `⌥`, `Win`) in UI controls. Use neutral labels (`Modules`, `Patterns`, `Light`/`Dark`, `Copy`, `SSH`, `Open`, `Close`).
- **Depth via Alternating Inset Surfaces** (1px borders, `12px` L0 radius, `10px` nested radius):

| Token | Light (`[data-theme="light"]`) | Dark (`[data-theme="dark"]`) | Purpose |
| :--- | :--- | :--- | :--- |
| `--bg-canvas` | `#f4f5f7` | `#09090b` | Outer canvas & widget shell |
| `--surface-l0` | `#ffffff` | `#18181b` | Level 0 top-level card (`radius: 12px; padding: 12px`) |
| `--surface-l1` | `#f8f9fb` | `#202024` | Level 1 nested card (`radius: 10px; padding: 10px`) |
| `--surface-l2` | `#ffffff` | `#18181b` | Level 2 nested card (`radius: 10px; padding: 10px`) |
| `--border-l0` | `#e4e7ec` | `#27272a` | Level 0 card border |
| `--border-nested` | `#e4e7ec` | `#2e2e33` | Level 1 & Level 2 nested card border |
| `--text-primary` | `#111827` | `#f4f4f5` | Primary ink |
| `--text-muted` | `#6b7280` | `#a1a1aa` | Secondary/muted ink |
| `--accent` | `#2563eb` | `#3b82f6` | Interactive blue accent & focus ring |

### Semantic & Stoplight Chips
| Tone | Light (bg / border / text) | Dot | Use Case |
| :--- | :--- | :--- | :--- |
| `green` | `#ecfdf5` / `#a7f3d0` / `#047857` | `#10b981` | `DEV`, `Valid ✓`, Currency (`EUR €`, `USD $`) |
| `amber` | `#fffbeb` / `#fde68a` / `#b45309` | `#f59e0b` | `QA`, Branch highlights, warnings |
| `red` | `#fff1f2` / `#fecdd3` / `#be123c` | `#f43f5e` | `PROD`, Alerts, invalid checksums |
| `blue` | `#eff6ff` / `#bfdbfe` / `#1d4ed8` | — | Port/UNLOC codes (`NLAMS · AMS`), `HBL`, modes, pinned pill |
| `neutral` / `mono` | `#f3f4f6` / `#e5e7eb` / `#374151` | — | Live clocks (`08:09`), temps (`14°C`), origins, link counts |

## 2. Exact Card Anatomy & Button Positioning

Every card (Level 0, Level 1, and Level 2) renders through `RolodexCardShellComponent` (`frontend/src/app/components/rolodex-card-shell.component.ts`):

```text
┌──────────────────────────────────────────────────────────┐
│ [Icon/Flag]  Title (15px Semibold)           [📌]   [▼]  │ ← Row 1: Identity + Right Controls
│ [Chip 1]  [Chip 2]  [Chip 3]  [Chip 4]                   │ ← Row 2: Glance Chips (gap: 6px, wrap)
├──────────────────────────────────────────────────────────┤ ← 1px top border (ONLY when expanded)
│ (Optional template body via NgComponentOutlet)           │
│ ┌──────────────────────────────────────────────────────┐ │
│ │ [Icon] Nested Level 1 Card (100% width, NO pin)  [▸] │ │ ← surface-l1, radius 10px, padding 10px
│ │ [● DEV]  [● QA]  [● PROD]                            │ │
│ └──────────────────────────────────────────────────────┘ │
│ ┌──────────────────────────────────────────────────────┐ │
│ │ [Icon] Single-Button Website Link                [↗] │ │ ← Single-Button Mode: no chevron, whole card clicks
│ └──────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

### Non-Negotiable Layout Rules
1. **Top-Right Controls**:
   - **Pin Button (`📌`)**: Rendered **ONLY** when `depth() === 0 && node().pinnable !== false`. Positioned immediately left of the expand chevron. When pinned, renders a subtle blue-tinted pill (`--chip-blue-bg` / `--chip-blue-border`).
   - **Max 3 Pins**: Managed by `RolodexStateService` (`MAX_PINS = 3`). Attempting a 4th pin triggers `.pin-shake` (disabled under `prefers-reduced-motion: reduce`) and an accessible toast notification.
   - **Expand/Collapse Chevron (`▸` / `▼`)**: Always pinned far top-right on every expandable card. Clicking anywhere on the header strip (except the Pin button) toggles expansion.
   - **Single-Button Mode (`website-link` with 1 URL)**: Omit the chevron entirely; render the whole card as a single clickable `<button>` with a top-right `↗` icon.
2. **Expanded Body + Full-Width Nested Children**:
   - Separated from header by `margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--border-divider);`.
   - Children sit in `.nested-stack` (`display: flex; flex-direction: column; gap: 8px; width: 100%;`) — zero left indentation or rail lines.
   - Support up to **2 levels of nesting** (`depth = 0 → 1 → 2`).

## 3. Five Canonical Generic Templates + Registry

All templates extend `BaseCardTemplate<T>` (`frontend/src/app/templates/base-card-template.ts`) and are registered in `CardTemplateRegistry` (`frontend/src/app/services/card-template-registry.service.ts`) for dynamic `*ngComponentOutlet` rendering:

1. **`LocationCardComponent` (`template: 'location'`)**: Strictly **header-only** (NO street addresses, phone numbers, or contact rows). Expanded state renders **only** its full-width nested child cards.
2. **`ServerMatrixCardComponent` (`template: 'server-matrix'`)**: Header displays 3 stoplight pills (`● DEV`, `● QA`, `● PROD`). Body renders compact rows with stoplight badge, monospace hostname, and inline `[Copy]`, `[SSH]`, and `[↗]` buttons.
3. **`WebsiteLinkCardComponent` (`template: 'website-link'`)**: Multi-Chip Mode (2+ links) expands to labeled launch/copy rows; Single-Button Mode (1 link) is handled directly by `RolodexCardShellComponent` as one clickable button with top-right `↗`.
4. **`PatternBreakdownCardComponent` (`template: 'pattern-breakdown'`)**: Renders a 3-column equal-width rounded segment tile grid (`grid-template-columns: repeat(3, 1fr); gap: 8px;`) in Blue / Amber / Green tinted tiles (`Value` bold + `ROLE` uppercase micro + `Meaning`), followed by a concise 1–2 sentence plain-English explanation, followed by full-width nested entity cards.
5. **`ConversionCardComponent` (`template: 'conversion'`)**: Compact `Input ⇄ Output` strip with 1-click Copy and secondary equivalent pills.

Extended domain templates (`container-blueprint`, `dictionary-guide`, `kv-grid`, and legacy `branch-contact`) also consume the same Light/Dark tokens cleanly.

## 4. Light/Dark Mode & Accessibility Checklist

- Include `<meta name="color-scheme" content="light dark">` in `frontend/src/index.html` and set `color-scheme: light` / `color-scheme: dark` on `[data-theme="light"]` / `[data-theme="dark"]` so native controls and scrollbars match automatically.
- Listen to `window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', ...)` in `RolodexStateService` while letting the user override via the header toggle.
- Never animate `scrollbar-color`.
- Use native `<dialog>` + `.showModal()` for modals, native `<button type="button">` for interactive triggers (`aria-expanded`, `aria-pressed`), `:focus-visible` outlines, and `popover="manual"` + `aria-live="polite"` for toasts.

## 5. Frontend Build & Typecheck Commands

```bash
cd frontend
/opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck   # Angular ngc strict AOT check
/opt/homebrew/Cellar/deno/2.9.8/bin/deno task build       # AOT compile + esbuild -> frontend/dist/
```
