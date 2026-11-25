---
name: vex-rolodex-ui
description: Authoritative UI design system, 9-layout component contract, and Angular 22 (Zoneless + Signals) guide for the Vex Full-Width Stacked Nested Card System (Light/Dark Mode).
---

# Vex — Full-Width Stacked Nested Card UI System (9 Generic Layouts)

Use this skill whenever modifying Angular components, templates, dialogs, or styles in `frontend/`.

## 1. Visual System & Non-Negotiable Layout Rules
- **Widget Frame:** `420px`–`520px` width (default `460px`), `max-height: 680px` internal scroll, Light & Dark mode (`[data-theme="light"]` / `[data-theme="dark"]`, `<meta name="color-scheme" content="light dark">`).
- **Platform-Agnostic:** Zero OS window traffic lights or hardcoded OS key glyphs (`⌘`, `⌥`, `Win`).
- **Zero Accordion Tree Lines:** Never render `border-left` rails or indentation. Every nested child spans **100% width** in `.nested-stack` (`gap: 8px`), depth `0 → 1 → 2` communicated strictly via alternating inset surfaces (`--surface-l0`, `--surface-l1`, `--surface-l2`) and 1px borders.
- **Top-Right Controls (`RolodexCardShellComponent`):**
  - **Pin (`📌`)**: Level 0 only, max 3 pins (`RolodexStateService`). Active pinned state MUST display the `--chip-blue-bg` / `--chip-blue-border` pill container (including on `link` cards).
  - **Expand Chevron (`▸` / `▼`)**: Render ONLY if the card has a leaf body OR resolved `children[]`. Never render an empty expandable accordion.
  - **`link` Layout**: Omit chevron; whole card is a clickable `<button>` with top-right `↗`.

## 2. The 9 Registered Layout Components (`CardTemplateRegistry`)
All extend `BaseCardTemplate<T>` and render inside `RolodexCardShellComponent` via `*ngComponentOutlet`:
1. `GroupCardComponent` (`group`): Header-only; renders only nested `children[]`.
2. `LinkCardComponent` (`link`): Handled as Single-Button card by shell (`↗`).
3. `ActionListCardComponent` (`action-list`): Stacked full-width rows. Place `badge` + `primary`/`subtitle` cleanly without crowding; actions (`copy`/`open_url`) stay `flex-shrink: 0` on a right or second row so titles never wrap awkwardly.
4. `FactsCardComponent` (`facts`): Optional segmented view-switcher pills (`views[]`, e.g. Metric / Imperial) + read-only 2-col fact grid. Render `[Copy]` button ONLY on facts where `copyable === true`.
5. `SegmentsCardComponent` (`segments`): Equal-width Blue/Amber/Green token tiles + 1–2 sentence explanation + nested children.
6. `StepsCardComponent` (`steps`): N-column visual timeline + summary + bullets.
7. `ConvertCardComponent` (`convert`): `Input ⇄ Output` strip + `[Copy]` + clickable equivalent pills.
8. `TableCardComponent` (`table`): Sticky header (2–4 cols, max 8 visible rows), optional mono columns, `[Copy CSV]` + `[Copy MD]` footer.
9. `SnippetCardComponent` (`snippet`): Optional compact inline `{{var}}` inputs + monospace code block + 1-click `[Copy]`.

## 3. Empty State, Keyboard Flow & Dialog Standards
- **Empty Search State:** Show **Pinned** (up to 3) + **Recent** (up to 6 from `listRecentCards()`), with a subtle total indexed count. Record usage on expand, open, or copy.
- **Single-Result Auto-Expand:** When an active search returns exactly 1 result, pass `[autoExpand]="true"` to `RolodexCardShellComponent` so the lone match opens expanded immediately (while remaining collapsible via `linkedSignal`).
- **Keyboard Navigation:** `ArrowDown` / `ArrowUp` selects Level 0 cards; `Enter` opens `link`, toggles expand, or copies primary output; `Escape` clears query first, then hides window on second press.
- **Dialogs (`<dialog>` + `.showModal()`):**
  - Set initial focus on modal heading (`tabindex="-1"`) or search/test input—never leave an aggressive focus ring on `Close`.
  - **Modules Dialog:** Separate **Installed Modules** (full-width title row 1, description row 2, metadata + `[Enabled] [Copy JSON] [Delete]` footer row 3) from **Starter Catalog** (`[Install]` button; zero ambiguous checkboxes). Bottom MCP box shows live status badge + `[Copy Client JSON]`.
  - **Patterns Dialog:** Place **Live Test** bar at top with non-truncated placeholder; show per-rule Enabled toggle, example input, linked entity chips, and `Live GET` / `Live MCP` badge; keep raw regex + JSON editor inside a collapsible `Edit JSON` disclosure.

## 4. Angular 22 Zoneless & Verification Commands
- Standalone components, `ChangeDetectionStrategy.OnPush`, Signals, `linkedSignal` for input-derived mutable state, explicit `[ngModel]` + `(ngModelChange)` for signal-bound form fields.
- Verify offline:
  `cd frontend && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task typecheck && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task test && /opt/homebrew/Cellar/deno/2.9.8/bin/deno task build`
