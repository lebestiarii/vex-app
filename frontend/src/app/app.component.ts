import {
  ChangeDetectionStrategy,
  Component,
  computed,
  DestroyRef,
  ElementRef,
  HostListener,
  inject,
  OnInit,
  signal,
  viewChild,
  viewChildren,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ModulesDialogComponent } from './components/modules-dialog.component';
import { PatternsDialogComponent } from './components/patterns-dialog.component';
import { RolodexCardShellComponent } from './components/rolodex-card-shell.component';
import { ToastRegionComponent } from './components/toast-region.component';
import { CardNode, ThemePreference, UISizePreset, UserPrefs } from './models/card-node.model';
import { MAX_PINS, RolodexStateService, UI_SIZE_PRESETS } from './services/rolodex-state.service';
import { VexBridgeService } from './services/vex-bridge.service';

export interface ScopePill {
  key: string;
  label: string;
  count: number;
}

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [
    FormsModule,
    RolodexCardShellComponent,
    ModulesDialogComponent,
    PatternsDialogComponent,
    ToastRegionComponent,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="widget-viewport">
      <div
        class="widget-frame"
        #widgetFrameEl
        [class.is-modal-expanded]="isModalExpanded()"
        [class.is-transitioning]="shellTransitioning()"
        [style.width.px]="activeShellWidth()"
      >
        <header class="frame-header">
          <h1 class="visually-hidden">Vex</h1>

          <div class="search-row">
            <label for="rolodex-search" class="visually-hidden">
              Search cards, modules, patterns, or unit conversions
            </label>
            <div class="search-field">
              <svg class="search-icon" viewBox="0 0 20 20" aria-hidden="true">
                <circle cx="9" cy="9" r="6" fill="none" stroke="currentColor" stroke-width="1.8" />
                <line x1="13.6" y1="13.6" x2="17.5" y2="17.5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
              </svg>
              <input
                #searchInputEl
                id="rolodex-search"
                type="search"
                class="search-input"
                autocomplete="off"
                spellcheck="false"
                placeholder="Search cards, patterns, conversions…"
                [ngModel]="query()"
                (ngModelChange)="onQueryChange($event)"
              />
              <div class="input-trailing">
                @if (state.stayAwake()) {
                  <button
                    type="button"
                    class="stay-awake-indicator"
                    (click)="disableStayAwakeFromIndicator($event)"
                    aria-label="Stay Awake: window stays open — click to disable"
                    title="Stay Awake: window stays open — click to disable"
                  >
                    <svg viewBox="0 0 16 16" aria-hidden="true">
                      <path
                        d="M3 6.5h7.5v3.4a3.2 3.2 0 0 1-3.2 3.2H6.2A3.2 3.2 0 0 1 3 9.9V6.5Z"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.35"
                        stroke-linejoin="round"
                      />
                      <path
                        d="M10.5 7.4h1.3a1.55 1.55 0 0 1 0 3.1h-1.3"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.35"
                        stroke-linecap="round"
                      />
                      <path
                        d="M5.4 4.2c0-.7.5-1 .5-1.6M8.1 4.2c0-.7.5-1 .5-1.6"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.2"
                        stroke-linecap="round"
                      />
                    </svg>
                  </button>
                }
                @if (query().length > 0) {
                  <button
                    type="button"
                    class="input-clear-btn"
                    (click)="clearQuery()"
                    aria-label="Clear search"
                    title="Clear search (Esc)"
                  >
                    <svg viewBox="0 0 16 16" aria-hidden="true">
                      <path
                        d="M4.5 4.5 11.5 11.5M11.5 4.5 4.5 11.5"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.6"
                        stroke-linecap="round"
                      />
                    </svg>
                  </button>
                }
              </div>
            </div>

            <div class="header-actions">
              <button
                type="button"
                class="ghost-icon-btn"
                [class.is-active]="quickAddOpen()"
                (click)="toggleQuickAddMenu($event)"
                aria-label="Quick add"
                aria-haspopup="dialog"
                [attr.aria-expanded]="quickAddOpen()"
                title="Quick add"
              >
                <svg viewBox="0 0 16 16" aria-hidden="true">
                  <path d="M8 3.25v9.5M3.25 8h9.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
                </svg>
              </button>
              <button
                type="button"
                class="ghost-icon-btn"
                [class.is-active]="settingsOpen()"
                (click)="toggleSettingsMenu($event)"
                aria-label="Settings"
                aria-haspopup="dialog"
                [attr.aria-expanded]="settingsOpen()"
                title="Settings"
              >
                <svg viewBox="0 0 16 16" aria-hidden="true">
                  <circle cx="8" cy="8" r="2.2" fill="none" stroke="currentColor" stroke-width="1.4" />
                  <path
                    d="M8 2.2v1.3M8 12.5v1.3M2.2 8h1.3M12.5 8h1.3M3.9 3.9l.95.95M11.15 11.15l.95.95M12.1 3.9l-.95.95M4.85 11.15l-.95.95"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.4"
                    stroke-linecap="round"
                  />
                </svg>
              </button>
            </div>
          </div>

          <div class="scope-strip-wrap" [class.is-open]="showScopePills()">
            <div class="scope-strip-inner">
              @if (showScopePills()) {
                <div class="scope-strip" role="tablist" aria-label="Filter by module">
                  <button
                    type="button"
                    role="tab"
                    class="scope-pill"
                    [class.is-active]="activeScope() === 'all'"
                    [attr.aria-selected]="activeScope() === 'all'"
                    (click)="selectScope('all')"
                  >
                    All ({{ orderedSearchCards().length }})
                  </button>
                  @for (scope of matchedScopes(); track scope.key) {
                    <button
                      type="button"
                      role="tab"
                      class="scope-pill"
                      [class.is-active]="activeScope() === scope.key"
                      [attr.aria-selected]="activeScope() === scope.key"
                      (click)="selectScope(scope.key)"
                    >
                      {{ scope.label }} ({{ scope.count }})
                    </button>
                  }
                </div>
              }
            </div>
          </div>

          <div
            class="header-popover quick-add-popover"
            [class.is-open]="quickAddOpen()"
            [attr.aria-hidden]="!quickAddOpen()"
            role="dialog"
            aria-label="Quick add"
            (click)="$event.stopPropagation()"
          >
            <div class="popover-section-label">Quick Add</div>
            <p class="quick-add-copy">
              Fast inline card creation is coming soon. For now, import or manage packs in Modules &amp; MCP.
            </p>
            <div class="quick-add-actions">
              <button type="button" class="popover-action-btn" (click)="openModulesFromMenu()">
                Open Modules &amp; MCP
              </button>
            </div>
          </div>

          <div
            class="header-popover settings-popover"
            [class.is-open]="settingsOpen()"
            [attr.aria-hidden]="!settingsOpen()"
            role="dialog"
            aria-label="Settings"
            (click)="$event.stopPropagation()"
          >
            <div class="popover-section">
              <div class="popover-section-label">Manage</div>
              <button
                type="button"
                class="settings-item"
                (click)="openModulesFromMenu()"
              >
                <span>Modules &amp; MCP</span>
                <span class="settings-count-badge">{{ installedModulesCount() }}</span>
              </button>
              <button
                type="button"
                class="settings-item"
                (click)="openPatternsFromMenu()"
              >
                <span>Pattern Rules</span>
                <span class="settings-count-badge">{{ activePatternRulesCount() }}</span>
              </button>
            </div>

            <div class="settings-divider" role="separator"></div>

            <div class="popover-section">
              <div class="popover-section-label">Appearance</div>
              <div class="setting-row">
                <span class="setting-row-label">Theme</span>
                <div class="segmented-control theme-control" role="radiogroup" aria-label="Theme">
                  @for (opt of themeOptions; track opt.value) {
                    <button
                      type="button"
                      role="radio"
                      class="seg-btn"
                      [class.is-active]="state.themePreference() === opt.value"
                      [attr.aria-checked]="state.themePreference() === opt.value"
                      (click)="selectTheme(opt.value)"
                    >
                      {{ opt.label }}
                    </button>
                  }
                </div>
              </div>
              <div class="setting-row">
                <span class="setting-row-label">Animations</span>
                <div class="segmented-control motion-control" role="radiogroup" aria-label="Animations">
                  <button
                    type="button"
                    role="radio"
                    class="seg-btn"
                    [class.is-active]="state.animationsEnabled()"
                    [attr.aria-checked]="state.animationsEnabled()"
                    (click)="selectAnimations(true)"
                  >
                    On
                  </button>
                  <button
                    type="button"
                    role="radio"
                    class="seg-btn"
                    [class.is-active]="!state.animationsEnabled()"
                    [attr.aria-checked]="!state.animationsEnabled()"
                    (click)="selectAnimations(false)"
                  >
                    Off
                  </button>
                </div>
              </div>
            </div>

            <div class="settings-divider" role="separator"></div>

            <div class="popover-section">
              <button
                type="button"
                role="switch"
                class="stay-awake-row"
                [class.is-on]="state.stayAwake()"
                [attr.aria-checked]="state.stayAwake()"
                (click)="toggleStayAwake()"
              >
                <span class="stay-awake-copy">
                  <span class="stay-awake-title">
                    <svg class="stay-awake-menu-icon" viewBox="0 0 16 16" aria-hidden="true">
                      <path
                        d="M3 6.5h7.5v3.4a3.2 3.2 0 0 1-3.2 3.2H6.2A3.2 3.2 0 0 1 3 9.9V6.5Z"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.35"
                        stroke-linejoin="round"
                      />
                      <path
                        d="M10.5 7.4h1.3a1.55 1.55 0 0 1 0 3.1h-1.3"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.35"
                        stroke-linecap="round"
                      />
                      <path
                        d="M5.4 4.2c0-.7.5-1 .5-1.6M8.1 4.2c0-.7.5-1 .5-1.6"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.2"
                        stroke-linecap="round"
                      />
                    </svg>
                    Stay Awake
                  </span>
                  <span class="stay-awake-sub">Keep window open on top</span>
                </span>
                <span class="switch-track" aria-hidden="true">
                  <span class="switch-thumb"></span>
                </span>
              </button>
            </div>

            <div class="settings-divider" role="separator"></div>

            <div class="popover-section">
              <div class="popover-section-label">UI Size</div>
              <div class="segmented-control size-control" role="radiogroup" aria-label="UI Size">
                @for (preset of uiSizeOptions; track preset.size) {
                  <button
                    type="button"
                    role="radio"
                    class="seg-btn"
                    [class.is-active]="state.uiSize() === preset.size"
                    [attr.aria-checked]="state.uiSize() === preset.size"
                    [title]="preset.label"
                    (click)="selectUISize(preset.size)"
                  >
                    {{ preset.size }}
                  </button>
                }
              </div>
            </div>
          </div>
        </header>

        <main
          class="frame-scroll rolodex-list"
          [class.is-loading]="loading()"
          #rolodexListEl
          id="rolodex-main"
        >
          @if (isEmptyQuery()) {
            @if (pinnedEmptyCards().length > 0 || recentEmptyCards().length > 0) {
              @if (pinnedEmptyCards().length > 0) {
                <section class="card-section" aria-label="Pinned cards">
                  <div class="section-meta">
                    <span>Pinned ({{ pinnedEmptyCards().length }}/{{ maxPins }})</span>
                  </div>
                  <div class="card-stack">
                    @for (card of pinnedEmptyCards(); track card.id) {
                      <app-rolodex-card-shell
                        [node]="card"
                        [depth]="0"
                        [selected]="selectedIndex() === $index"
                      />
                    }
                  </div>
                </section>
              }

              @if (recentEmptyCards().length > 0) {
                <section class="card-section" aria-label="Recent cards">
                  <div class="section-meta">
                    <span>Recent</span>
                    <button
                      type="button"
                      class="section-clear-btn"
                      (click)="clearRecents()"
                      aria-label="Clear recent cards"
                    >
                      Clear
                    </button>
                  </div>
                  <div class="card-stack">
                    @for (card of recentEmptyCards(); track card.id) {
                      <app-rolodex-card-shell
                        [node]="card"
                        [depth]="0"
                        [selected]="selectedIndex() === pinnedEmptyCards().length + $index"
                      />
                    }
                  </div>
                </section>
              }
            } @else if (totalIndexedCount() > 0) {
              <div class="empty-state ready-state">
                <div class="empty-icon" aria-hidden="true">🔎</div>
                <h2 class="empty-title">
                  {{ totalIndexedCount() }} indexed {{ totalIndexedCount() === 1 ? 'card' : 'cards' }} ready
                </h2>
                <p class="empty-desc">
                  Type to search cards, decode tracking IDs, or convert units. Cards you expand, open, or copy appear here under Recent.
                </p>
              </div>
            } @else {
              <div class="empty-state">
                <div class="empty-icon" aria-hidden="true">🗂️</div>
                <h2 class="empty-title">No optional modules loaded</h2>
                <p class="empty-desc">
                  All modules are optional and deletable. Install reference packs in Modules or push cards via MCP.
                </p>
                <div class="empty-actions">
                  <button type="button" class="primary-btn" (click)="openModules(true)">
                    Choose Starter Modules
                  </button>
                </div>
              </div>
            }
          } @else if (filteredSearchCards().length > 0) {
            <div class="card-stack">
              @for (card of filteredSearchCards(); track card.id) {
                <app-rolodex-card-shell
                  [node]="card"
                  [depth]="0"
                  [selected]="selectedIndex() === $index"
                  [autoExpand]="filteredSearchCards().length === 1"
                />
              }
            </div>
          } @else {
            <div class="empty-state">
              <div class="empty-icon" aria-hidden="true">🗂️</div>
              <h2 class="empty-title">No matching cards</h2>
              <p class="empty-desc">
                No indexed cards or active pattern rules matched your query.
              </p>
              <div class="empty-actions">
                <button type="button" class="primary-btn" (click)="clearQuery()">
                  Reset Search
                </button>
                <button type="button" class="primary-btn" (click)="openModules(true)">
                  Choose Starter Modules
                </button>
              </div>
            </div>
          }
        </main>

        <footer class="frame-footer" aria-label="Status and keyboard hints">
          <span class="footer-hints">↑↓ Move · ↵ Open/Copy · Esc Clear</span>
          <span class="footer-count">
            @if (isEmptyQuery()) {
              {{ totalIndexedCount() }} indexed
            } @else {
              {{ filteredSearchCards().length }} / {{ filterDenominator() }} shown
            }
          </span>
        </footer>
      </div>
    </div>

    <app-modules-dialog
      [open]="modulesOpen()"
      [onboardingMode]="onboardingMode()"
      (closed)="closeModules()"
      (modulesChanged)="onCatalogChanged()"
    />

    <app-patterns-dialog
      [open]="patternsOpen()"
      (closed)="closePatterns()"
      (patternsChanged)="onCatalogChanged()"
    />

    <app-toast-region />
  `,
  styles: [
    `
      .widget-viewport {
        padding: 10px;
        display: flex;
        flex-direction: column;
        align-items: center;
        background: transparent;
      }

      /* Full-window desktop widget frame with clean internal scroll & zero outer bezel */
      .widget-frame {
        width: 480px;
        max-width: 100%;
        min-height: 0;
        max-height: 680px;
        display: flex;
        flex-direction: column;
        background: var(--bg-shell);
        border: 1px solid var(--border-l0);
        border-radius: 14px;
        box-shadow: var(--shadow-shell);
        overflow: visible;
        transform-origin: top center;
        opacity: 1;
        transform: scale(1) translateY(0);
        transition:
          width 280ms cubic-bezier(0.22, 1, 0.36, 1),
          min-height 280ms cubic-bezier(0.22, 1, 0.36, 1),
          opacity 180ms ease,
          transform 200ms cubic-bezier(0.22, 1, 0.36, 1);
      }

      .widget-frame.is-transitioning {
        opacity: 0.72;
        transform: scale(0.985) translateY(-2px);
      }

      .frame-scroll {
        transition: opacity 140ms ease;
      }

      .frame-scroll.is-loading {
        opacity: 0.72;
      }

      .widget-frame.is-modal-expanded {
        min-height: 620px;
        max-height: 620px;
      }

      .frame-header {
        position: relative;
        padding: 10px 10px 9px;
        border-bottom: 1px solid var(--border-divider);
        border-radius: 13px 13px 0 0;
        background: var(--surface-l0);
        display: flex;
        flex-direction: column;
        gap: 0;
        flex-shrink: 0;
        --wails-draggable: drag;
      }

      .frame-header input,
      .frame-header button {
        --wails-draggable: no-drag;
      }

      .search-row {
        display: flex;
        align-items: center;
        gap: 5px;
      }

      .search-field {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: 7px;
        padding: 0 7px 0 10px;
        min-block-size: 36px;
        border-radius: 9px;
        border: 1px solid var(--border-l0);
        background: var(--bg-input);
        transition: border-color 140ms ease;
      }

      .search-field:focus-within {
        border-color: var(--accent);
      }

      .search-icon {
        width: 15px;
        height: 15px;
        color: var(--text-muted);
        flex-shrink: 0;
      }

      .search-input {
        flex: 1;
        min-width: 0;
        border: 0;
        background: transparent;
        font-size: 13.5px;
        color: var(--text-primary);
        outline: none;
      }

      .search-input::placeholder {
        color: var(--text-muted);
      }

      .search-input::-webkit-search-cancel-button {
        -webkit-appearance: none;
        appearance: none;
      }

      .input-trailing {
        display: inline-flex;
        align-items: center;
        gap: 2px;
        flex-shrink: 0;
      }

      .stay-awake-indicator,
      .input-clear-btn {
        width: 24px;
        height: 24px;
        padding: 0;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        border-radius: 6px;
        border: 0;
        background: transparent;
        cursor: pointer;
        transition: background-color 120ms ease, color 120ms ease;
      }

      .stay-awake-indicator {
        color: var(--stay-awake-ink);
      }

      .stay-awake-indicator:hover {
        background: var(--bg-subtle);
      }

      .stay-awake-indicator svg {
        width: 14px;
        height: 14px;
      }

      .input-clear-btn {
        color: var(--text-muted);
      }

      .input-clear-btn:hover {
        background: var(--bg-subtle);
        color: var(--text-primary);
      }

      .input-clear-btn svg {
        width: 13px;
        height: 13px;
      }

      .header-actions {
        display: inline-flex;
        align-items: center;
        gap: 2px;
        flex-shrink: 0;
      }

      .ghost-icon-btn {
        width: 28px;
        height: 28px;
        padding: 0;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        border-radius: 8px;
        border: 0;
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
        transition: background-color 120ms ease, color 120ms ease;
      }

      .ghost-icon-btn svg {
        width: 15px;
        height: 15px;
      }

      .ghost-icon-btn:hover,
      .ghost-icon-btn.is-active,
      .ghost-icon-btn[aria-expanded='true'] {
        background: var(--bg-subtle);
        color: var(--text-primary);
      }

      /* Adaptive Scope Pills: 0px hidden for <=1 matched module; smooth 28px strip for 2+ */
      .scope-strip-wrap {
        display: grid;
        grid-template-rows: 0fr;
        opacity: 0;
        margin-top: 0;
        pointer-events: none;
        transition:
          grid-template-rows 180ms cubic-bezier(0.2, 0.9, 0.3, 1),
          opacity 150ms ease,
          margin-top 180ms cubic-bezier(0.2, 0.9, 0.3, 1);
      }

      .scope-strip-wrap.is-open {
        grid-template-rows: 1fr;
        opacity: 1;
        margin-top: 7px;
        pointer-events: auto;
      }

      .scope-strip-inner {
        overflow: hidden;
        min-height: 0;
      }

      .scope-strip {
        height: 28px;
        display: flex;
        align-items: center;
        gap: 5px;
        overflow-x: auto;
        scrollbar-width: none;
      }

      .scope-strip::-webkit-scrollbar {
        display: none;
      }

      .scope-pill {
        flex-shrink: 0;
        height: 24px;
        padding: 0 9px;
        border-radius: 999px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
        transition: background-color 120ms ease, border-color 120ms ease, color 120ms ease;
      }

      .scope-pill:hover {
        background: var(--surface-hover);
        color: var(--text-primary);
      }

      .scope-pill.is-active {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }

      /* Smooth Open + Close Anchored Popovers */
      .header-popover {
        position: absolute;
        top: calc(100% + 4px);
        inset-inline-end: 10px;
        z-index: 30;
        padding: 8px;
        border-radius: 11px;
        border: 1px solid var(--border-l0);
        background: var(--surface-l0);
        box-shadow: var(--shadow-shell);
        display: none;
        flex-direction: column;
        gap: 6px;
        opacity: 0;
        transform: scale(0.96) translateY(-4px);
        transform-origin: top right;
        transition:
          opacity 140ms ease,
          transform 160ms cubic-bezier(0.2, 0.8, 0.2, 1),
          display 160ms allow-discrete;
        --wails-draggable: no-drag;
      }

      .header-popover.is-open {
        display: flex;
        opacity: 1;
        transform: scale(1) translateY(0);
      }

      @starting-style {
        .header-popover.is-open {
          opacity: 0;
          transform: scale(0.96) translateY(-4px);
        }
      }

      .quick-add-popover {
        inset-inline-end: 40px;
        width: 224px;
      }

      .quick-add-copy {
        margin: 0;
        font-size: 11.5px;
        line-height: 1.45;
        color: var(--text-muted);
      }

      .quick-add-actions {
        display: flex;
        justify-content: flex-end;
        margin-top: 2px;
      }

      .popover-action-btn {
        padding: 4px 9px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-primary);
        font-size: 11.5px;
        font-weight: 600;
        cursor: pointer;
      }

      .popover-action-btn:hover {
        background: var(--surface-hover);
      }

      .settings-popover {
        width: 244px;
      }

      .popover-section {
        display: flex;
        flex-direction: column;
        gap: 4px;
      }

      .popover-section-label {
        font-size: 10.5px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.05em;
        color: var(--text-muted);
        padding-inline: 4px;
      }

      .settings-item {
        width: 100%;
        min-block-size: 30px;
        padding: 5px 8px;
        border: 0;
        border-radius: 7px;
        background: transparent;
        color: var(--text-primary);
        font-size: 12.5px;
        font-weight: 500;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
        cursor: pointer;
        text-align: start;
        transition: background-color 120ms ease;
      }

      .settings-item:hover {
        background: var(--surface-hover);
      }

      .settings-count-badge {
        min-width: 22px;
        padding: 1px 7px;
        border-radius: 999px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        text-align: center;
      }

      .settings-divider {
        height: 1px;
        margin: 1px 2px;
        background: var(--border-divider);
      }

      .segmented-control {
        display: grid;
        grid-auto-flow: column;
        grid-auto-columns: 1fr;
        gap: 2px;
        padding: 2px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
      }

      .seg-btn {
        min-block-size: 24px;
        padding: 2px 6px;
        border: 0;
        border-radius: 6px;
        background: transparent;
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
        transition: background-color 120ms ease, color 120ms ease;
      }

      .seg-btn:hover {
        color: var(--text-primary);
      }

      .seg-btn.is-active {
        background: var(--surface-l0);
        color: var(--text-primary);
        box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
      }

      .setting-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
        padding: 2px 4px;
      }

      .setting-row-label {
        font-size: 12.5px;
        font-weight: 500;
        color: var(--text-primary);
        flex-shrink: 0;
      }

      .theme-control {
        width: 152px;
      }

      .motion-control {
        width: 80px;
      }

      .stay-awake-row {
        width: 100%;
        padding: 5px 6px;
        border: 0;
        border-radius: 8px;
        background: transparent;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
        cursor: pointer;
        text-align: start;
        transition: background-color 120ms ease;
      }

      .stay-awake-row:hover {
        background: var(--surface-hover);
      }

      .stay-awake-copy {
        display: flex;
        flex-direction: column;
        gap: 1px;
      }

      .stay-awake-title {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        font-size: 12.5px;
        font-weight: 600;
        color: var(--text-primary);
      }

      .stay-awake-menu-icon {
        width: 14px;
        height: 14px;
        color: var(--text-muted);
      }

      .stay-awake-row.is-on .stay-awake-menu-icon {
        color: var(--stay-awake-ink);
      }

      .stay-awake-sub {
        font-size: 11px;
        color: var(--text-muted);
      }

      .switch-track {
        width: 30px;
        height: 17px;
        border-radius: 999px;
        background: var(--border-nested);
        position: relative;
        flex-shrink: 0;
        transition: background-color 140ms ease;
      }

      .stay-awake-row.is-on .switch-track {
        background: var(--stay-awake-ink);
      }

      .switch-thumb {
        position: absolute;
        top: 2px;
        left: 2px;
        width: 13px;
        height: 13px;
        border-radius: 50%;
        background: var(--surface-l0);
        transition: transform 140ms cubic-bezier(0.2, 0.8, 0.2, 1);
      }

      .stay-awake-row.is-on .switch-thumb {
        transform: translateX(13px);
      }

      .frame-footer {
        height: 26px;
        padding: 0 12px;
        border-top: 1px solid var(--border-divider);
        border-radius: 0 0 13px 13px;
        background: var(--surface-l0);
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        flex-shrink: 0;
        font-size: 11px;
        color: var(--text-muted);
      }

      .footer-hints {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }

      .footer-count {
        flex-shrink: 0;
        font-variant-numeric: tabular-nums;
      }

      .frame-scroll {
        padding: 12px;
        overflow-y: auto;
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 12px;
        scrollbar-color: var(--scrollbar-thumb) var(--scrollbar-track);
      }

      .card-section {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }

      .section-meta {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        font-size: 11px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.05em;
        color: var(--text-muted);
        padding-inline: 2px;
      }

      .section-clear-btn {
        padding: 1px 6px;
        border: 0;
        border-radius: 5px;
        background: transparent;
        color: var(--text-muted);
        font-size: 11px;
        font-weight: 600;
        text-transform: none;
        letter-spacing: normal;
        cursor: pointer;
        transition: background-color 0.12s ease, color 0.12s ease;
      }

      .section-clear-btn:hover {
        background: var(--bg-subtle);
        color: var(--text-primary);
      }

      .card-stack {
        display: flex;
        flex-direction: column;
        gap: 10px;
      }

      .status-card,
      .empty-state {
        padding: 24px 16px;
        border-radius: 12px;
        border: 1px dashed var(--border-l0);
        background: var(--surface-l0);
        text-align: center;
      }

      .empty-icon {
        font-size: 24px;
        margin-block-end: 6px;
      }

      .empty-title {
        margin: 0;
        font-size: 14.5px;
        font-weight: 600;
        color: var(--text-primary);
      }

      .empty-desc {
        margin: 6px auto 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-muted);
      }

      .empty-actions {
        margin-block-start: 12px;
        display: inline-flex;
        gap: 8px;
      }

      .primary-btn {
        min-block-size: 30px;
        padding: 5px 12px;
        border-radius: 8px;
        border: 1px solid var(--chip-blue-border);
        background: var(--chip-blue-bg);
        color: var(--chip-blue-text);
        font-size: 12px;
        font-weight: 700;
        cursor: pointer;
      }
    `,
  ],
})
export class AppComponent implements OnInit {
  readonly bridge = inject(VexBridgeService);
  readonly state = inject(RolodexStateService);
  private readonly destroyRef = inject(DestroyRef);

  readonly maxPins = MAX_PINS;
  readonly uiSizeOptions: readonly { size: UISizePreset; label: string }[] = [
    { size: 460, label: 'Compact (460px)' },
    { size: 480, label: 'Standard (480px)' },
    { size: 560, label: 'Comfy (560px)' },
    { size: 620, label: 'Wide (620px)' },
  ];
  readonly themeOptions: readonly { value: ThemePreference; label: string }[] = [
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
    { value: 'system', label: 'System' },
  ];

  readonly query = signal<string>('');
  readonly searchCards = signal<CardNode[]>([]);
  readonly recentCards = signal<CardNode[]>([]);
  readonly totalIndexedCount = signal<number>(0);
  readonly installedModulesCount = signal<number>(0);
  readonly activePatternRulesCount = signal<number>(0);
  readonly moduleNamesById = signal<Record<string, string>>({});
  readonly activeScope = signal<string>('all');
  readonly selectedIndex = signal<number>(-1);
  readonly loading = signal<boolean>(false);
  readonly modulesOpen = signal<boolean>(false);
  readonly onboardingMode = signal<boolean>(false);
  readonly patternsOpen = signal<boolean>(false);
  readonly settingsOpen = signal<boolean>(false);
  readonly quickAddOpen = signal<boolean>(false);
  readonly shellTransitioning = signal<boolean>(false);
  readonly modalExpandedVisual = signal<boolean>(false);

  private readonly searchInputEl = viewChild<ElementRef<HTMLInputElement>>('searchInputEl');
  private readonly rolodexListEl = viewChild<ElementRef<HTMLElement>>('rolodexListEl');
  private readonly cardShells = viewChildren(RolodexCardShellComponent);

  private searchTimer: ReturnType<typeof setTimeout> | null = null;
  private cachedPrefs: UserPrefs = {
    onboardingCompleted: false,
    powerUserMode: false,
    uiSize: 480,
    theme: 'system',
    animationsEnabled: true,
    stayAwake: false,
  };

  readonly isEmptyQuery = computed(() => this.query().trim() === '');
  readonly isModalExpanded = computed(() => this.modulesOpen() || this.patternsOpen() || this.modalExpandedVisual());
  readonly activeShellWidth = computed<number>(() =>
    this.isModalExpanded() ? 620 : this.state.uiSize()
  );

  readonly pinnedEmptyCards = computed<CardNode[]>(() => {
    const ids = this.state.pinnedIds().slice(0, MAX_PINS);
    if (ids.length === 0) return [];
    const cached = this.state.pinnedCardsMap();
    const recentById = new Map(this.recentCards().map((c) => [c.id, c]));
    const searchById = new Map(this.searchCards().map((c) => [c.id, c]));

    const resolved: CardNode[] = [];
    for (const id of ids) {
      const node = recentById.get(id) ?? searchById.get(id) ?? cached[id];
      if (node) resolved.push(node);
    }
    return resolved;
  });

  readonly recentEmptyCards = computed<CardNode[]>(() => {
    const pinnedSet = new Set(this.pinnedEmptyCards().map((c) => c.id));
    return this.recentCards()
      .filter((c) => !pinnedSet.has(c.id))
      .slice(0, 6);
  });

  readonly orderedSearchCards = computed<CardNode[]>(() => {
    const pinned = this.state.pinnedIds();
    const list = this.searchCards();
    if (!pinned.length) return list;

    const pinRank = new Map<string, number>();
    pinned.forEach((id, idx) => pinRank.set(id, idx));

    return [...list].sort((a, b) => {
      const aPinned = pinRank.has(a.id);
      const bPinned = pinRank.has(b.id);
      if (aPinned && bPinned) return (pinRank.get(a.id) ?? 0) - (pinRank.get(b.id) ?? 0);
      if (aPinned) return -1;
      if (bPinned) return 1;
      return 0;
    });
  });

  readonly matchedScopes = computed<ScopePill[]>(() => {
    if (this.isEmptyQuery()) return [];
    const names = this.moduleNamesById();
    const counts = new Map<string, number>();
    for (const card of this.orderedSearchCards()) {
      const key = this.scopeKeyForCard(card);
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    const pills: ScopePill[] = [];
    for (const [key, count] of counts.entries()) {
      pills.push({
        key,
        label: names[key] || this.formatScopeLabel(key),
        count,
      });
    }
    return pills;
  });

  readonly showScopePills = computed(
    () => !this.isEmptyQuery() && this.matchedScopes().length >= 2
  );

  readonly filteredSearchCards = computed<CardNode[]>(() => {
    const list = this.orderedSearchCards();
    const scope = this.activeScope();
    if (scope === 'all' || !this.showScopePills()) {
      return list;
    }
    return list.filter((c) => this.scopeKeyForCard(c) === scope);
  });

  readonly filterDenominator = computed<number>(() => {
    if (this.activeScope() !== 'all' && this.showScopePills()) {
      return this.orderedSearchCards().length;
    }
    return Math.max(this.totalIndexedCount(), this.orderedSearchCards().length);
  });

  readonly visibleCards = computed<CardNode[]>(() => {
    if (this.isEmptyQuery()) {
      return [...this.pinnedEmptyCards(), ...this.recentEmptyCards()];
    }
    return this.filteredSearchCards();
  });

  constructor() {
    this.destroyRef.onDestroy(() => {
      if (this.searchTimer) {
        clearTimeout(this.searchTimer);
      }
    });
  }

  async ngOnInit(): Promise<void> {
    this.setupWindowAutoResize();
    this.setupWindowFocusListeners();
    this.setupScrollHoverGuard();
    this.focusSearchInput();
    const prefs = await this.bridge.getUserPrefs();
    this.cachedPrefs = { ...this.cachedPrefs, ...prefs };
    this.state.hydrateFromPrefs(this.cachedPrefs);
    if (typeof this.bridge.getStayAwake === 'function') {
      const awake = await this.bridge.getStayAwake();
      this.state.setStayAwake(awake);
      this.cachedPrefs.stayAwake = awake;
    }
    await Promise.all([this.refreshResults(), this.refreshMetadataCounts()]);
    this.scheduleResizeFn?.();

    if (!prefs.onboardingCompleted && this.totalIndexedCount() === 0 && this.visibleCards().length === 0) {
      this.openModules(true);
    }
  }

  private readonly widgetFrameEl = viewChild<ElementRef<HTMLElement>>('widgetFrameEl');
  private lastWindowWidth = 0;
  private lastWindowHeight = 0;
  private resizeTimer: ReturnType<typeof setTimeout> | null = null;
  private modalCloseResizeTimer: ReturnType<typeof setTimeout> | null = null;
  private modalTransitionLock = false;
  private scheduleResizeFn: ((immediate?: boolean) => void) | null = null;
  private shellFadeTimer: ReturnType<typeof setTimeout> | null = null;
  private scrollGuardTimer: ReturnType<typeof setTimeout> | null = null;

  private setupScrollHoverGuard(): void {
    const listEl = this.rolodexListEl()?.nativeElement;
    if (!listEl) {
      return;
    }

    const onScroll = () => {
      listEl.classList.add('is-scrolling');
      if (this.scrollGuardTimer !== null) {
        clearTimeout(this.scrollGuardTimer);
      }
      this.scrollGuardTimer = setTimeout(() => {
        this.scrollGuardTimer = null;
        listEl.classList.remove('is-scrolling');
      }, 100);
    };

    listEl.addEventListener('scroll', onScroll, { passive: true });

    this.destroyRef.onDestroy(() => {
      listEl.removeEventListener('scroll', onScroll);
      if (this.scrollGuardTimer !== null) {
        clearTimeout(this.scrollGuardTimer);
        this.scrollGuardTimer = null;
      }
      listEl.classList.remove('is-scrolling');
    });
  }

  private triggerShellSpring(deltaPx: number): void {
    if (this.shellFadeTimer !== null) {
      clearTimeout(this.shellFadeTimer);
      this.shellFadeTimer = null;
    }
    if (this.state.isMotionReduced() || deltaPx === 0) {
      this.shellTransitioning.set(false);
      return;
    }
    this.shellTransitioning.set(true);
    this.shellFadeTimer = setTimeout(() => {
      this.shellFadeTimer = null;
      this.shellTransitioning.set(false);
    }, 110);
  }

  private setupWindowAutoResize(): void {
    const frameEl = this.widgetFrameEl()?.nativeElement;
    if (!frameEl || typeof ResizeObserver === 'undefined') {
      return;
    }

    const WINDOW_BUFFER = 20;

    const applyResize = () => {
      if (this.resizeTimer !== null) {
        clearTimeout(this.resizeTimer);
        this.resizeTimer = null;
      }
      if (!frameEl.isConnected || this.modalTransitionLock) {
        return;
      }
      const targetShellWidth = this.activeShellWidth();
      let targetShellHeight = Math.ceil(frameEl.getBoundingClientRect().height);
      if (this.isModalExpanded()) {
        targetShellHeight = 620;
      } else if (this.settingsOpen()) {
        targetShellHeight = Math.max(targetShellHeight, 480);
      } else if (this.quickAddOpen()) {
        targetShellHeight = Math.max(targetShellHeight, 220);
      }
      const nextWidth = targetShellWidth + WINDOW_BUFFER;
      const nextHeight = targetShellHeight + WINDOW_BUFFER;
      if (
        targetShellHeight > 0 &&
        (nextWidth !== this.lastWindowWidth || nextHeight !== this.lastWindowHeight)
      ) {
        this.lastWindowWidth = nextWidth;
        this.lastWindowHeight = nextHeight;
        void this.bridge.resizeWindow?.(nextWidth, nextHeight);
      }
    };

    const scheduleResize = (immediate = false) => {
      if (this.modalTransitionLock) {
        return;
      }
      if (immediate) {
        applyResize();
        return;
      }
      if (this.resizeTimer !== null) {
        clearTimeout(this.resizeTimer);
      }
      this.resizeTimer = setTimeout(() => {
        this.resizeTimer = null;
        applyResize();
      }, 75);
    };

    this.scheduleResizeFn = scheduleResize;

    const observer = new ResizeObserver(() => scheduleResize(false));
    observer.observe(frameEl);
    scheduleResize(true);

    this.destroyRef.onDestroy(() => {
      observer.disconnect();
      this.scheduleResizeFn = null;
      if (this.resizeTimer !== null) {
        clearTimeout(this.resizeTimer);
        this.resizeTimer = null;
      }
      if (this.modalCloseResizeTimer !== null) {
        clearTimeout(this.modalCloseResizeTimer);
        this.modalCloseResizeTimer = null;
      }
      if (this.shellFadeTimer !== null) {
        clearTimeout(this.shellFadeTimer);
        this.shellFadeTimer = null;
      }
    });
  }

  private setupWindowFocusListeners(): void {
    const onVisibility = () => {
      if (typeof document !== 'undefined' && !document.hidden) {
        this.focusSearchInput();
      }
    };
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', onVisibility);
    }

    const wailsEvents = (window as unknown as {
      wails?: { Events?: { On?: (name: string, cb: () => void) => (() => void) | void } };
    })?.wails?.Events;
    const offFocus = wailsEvents?.On?.('vex:focus', () => {
      this.focusSearchInput();
    });

    this.destroyRef.onDestroy(() => {
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', onVisibility);
      }
      if (typeof offFocus === 'function') {
        offFocus();
      }
    });
  }

  focusSearchInput(): void {
    if (this.modulesOpen() || this.patternsOpen()) {
      return;
    }
    const input = this.searchInputEl()?.nativeElement;
    if (!input) {
      return;
    }
    input.focus();
  }

  toggleQuickAddMenu(event: MouseEvent): void {
    event.stopPropagation();
    this.settingsOpen.set(false);
    this.quickAddOpen.update((open) => !open);
    this.scheduleResizeFn?.(true);
  }

  toggleSettingsMenu(event: MouseEvent): void {
    event.stopPropagation();
    this.quickAddOpen.set(false);
    this.settingsOpen.update((open) => !open);
    if (this.settingsOpen()) {
      void this.refreshMetadataCounts();
    }
    this.scheduleResizeFn?.(true);
  }

  openModulesFromMenu(): void {
    this.settingsOpen.set(false);
    this.quickAddOpen.set(false);
    this.openModules(false);
  }

  openPatternsFromMenu(): void {
    this.settingsOpen.set(false);
    this.quickAddOpen.set(false);
    this.expandWindowForModal(() => {
      this.patternsOpen.set(true);
    });
  }

  selectUISize(size: UISizePreset): void {
    if (!UI_SIZE_PRESETS.includes(size)) return;
    const prevWidth = this.activeShellWidth();
    this.state.setUISize(size);
    this.persistPrefsPatch({ uiSize: size });
    this.triggerShellSpring(size - prevWidth);
    this.scheduleResizeFn?.(true);
  }

  selectTheme(pref: ThemePreference): void {
    this.state.setThemePreference(pref);
    this.persistPrefsPatch({ theme: pref });
  }

  selectAnimations(enabled: boolean): void {
    this.state.setAnimationsEnabled(enabled);
    this.persistPrefsPatch({ animationsEnabled: enabled });
  }

  toggleStayAwake(): void {
    const next = !this.state.stayAwake();
    this.state.setStayAwake(next);
    this.cachedPrefs = { ...this.cachedPrefs, stayAwake: next };
    void this.bridge.setStayAwake?.(next);
  }

  disableStayAwakeFromIndicator(event: MouseEvent): void {
    event.stopPropagation();
    this.state.setStayAwake(false);
    this.cachedPrefs = { ...this.cachedPrefs, stayAwake: false };
    void this.bridge.setStayAwake?.(false);
    this.focusSearchInput();
  }

  selectScope(scopeKey: string): void {
    this.activeScope.set(scopeKey);
    this.selectedIndex.set(this.visibleCards().length > 0 ? 0 : -1);
    this.scheduleResizeFn?.();
  }

  closeModules(): void {
    this.modulesOpen.set(false);
    this.restoreWindowAfterModalClose();
    this.focusSearchInput();
  }

  closePatterns(): void {
    this.patternsOpen.set(false);
    this.restoreWindowAfterModalClose();
    this.focusSearchInput();
  }

  private expandWindowForModal(openFn: () => void): void {
    if (this.modalCloseResizeTimer !== null) {
      clearTimeout(this.modalCloseResizeTimer);
      this.modalCloseResizeTimer = null;
    }
    const nextWidth = 620 + 20;
    const nextHeight = 620 + 20;
    this.lastWindowWidth = nextWidth;
    this.lastWindowHeight = nextHeight;

    if (this.state.isMotionReduced()) {
      this.modalExpandedVisual.set(true);
      this.modalTransitionLock = false;
      void this.bridge.resizeWindow?.(nextWidth, nextHeight);
      openFn();
      return;
    }

    // 1. Expand invisible transparent OS window to 640x640 and let Cocoa settle (~48ms).
    // 2. Open the <dialog> at popup proportions and expand both <dialog> + .widget-frame over 280ms.
    this.modalTransitionLock = true;
    void (async () => {
      await this.bridge.resizeWindow?.(nextWidth, nextHeight);
      this.modalCloseResizeTimer = setTimeout(() => {
        this.modalExpandedVisual.set(true);
        openFn();
        this.modalCloseResizeTimer = setTimeout(() => {
          this.modalCloseResizeTimer = null;
          this.modalTransitionLock = false;
        }, 300);
      }, 48);
    })();
  }

  private restoreWindowAfterModalClose(): void {
    if (this.modalCloseResizeTimer !== null) {
      clearTimeout(this.modalCloseResizeTimer);
      this.modalCloseResizeTimer = null;
    }
    this.modalExpandedVisual.set(false);
    if (this.state.isMotionReduced()) {
      this.modalTransitionLock = false;
      this.scheduleResizeFn?.(true);
      return;
    }
    // Keep transparent OS window steady at 640x640 while modal thins down & .widget-frame shrinks (280ms),
    // then silently trim the OS window bounds once settled.
    this.modalTransitionLock = true;
    this.modalCloseResizeTimer = setTimeout(() => {
      this.modalCloseResizeTimer = null;
      this.modalTransitionLock = false;
      this.scheduleResizeFn?.(true);
    }, 300);
  }

  async onCatalogChanged(): Promise<void> {
    await Promise.all([this.refreshResults(), this.refreshMetadataCounts()]);
  }

  async clearRecents(): Promise<void> {
    await this.bridge.clearRecentCards?.();
    this.recentCards.set([]);
    this.selectedIndex.set(this.visibleCards().length > 0 ? 0 : -1);
    this.scheduleResizeFn?.();
    this.focusSearchInput();
  }

  onQueryChange(next: string): void {
    this.query.set(next);
    if (!next.trim()) {
      this.activeScope.set('all');
    }
    if (this.searchTimer) {
      clearTimeout(this.searchTimer);
    }
    this.searchTimer = setTimeout(() => {
      void this.refreshResults();
    }, 130);
  }

  clearQuery(): void {
    if (this.searchTimer) {
      clearTimeout(this.searchTimer);
      this.searchTimer = null;
    }
    this.query.set('');
    this.activeScope.set('all');
    void this.refreshResults();
    this.searchInputEl()?.nativeElement.focus();
  }

  openModules(onboarding: boolean): void {
    this.onboardingMode.set(onboarding);
    this.expandWindowForModal(() => {
      this.modulesOpen.set(true);
    });
  }

  private persistPrefsPatch(patch: Partial<UserPrefs>): void {
    this.cachedPrefs = { ...this.cachedPrefs, ...patch };
    void this.bridge.setUserPrefs?.(this.cachedPrefs);
  }

  private async refreshMetadataCounts(): Promise<void> {
    try {
      const [modules, rules] = await Promise.all([
        this.bridge.listModules?.() ?? Promise.resolve([]),
        this.bridge.listPatternRules?.() ?? Promise.resolve([]),
      ]);
      this.installedModulesCount.set(modules.length);
      const nameMap: Record<string, string> = {};
      for (const m of modules) {
        if (m.id && m.name) {
          nameMap[m.id] = m.name;
        }
      }
      this.moduleNamesById.set(nameMap);
      const activeRules = rules.filter((r) => r.enabled !== false).length;
      this.activePatternRulesCount.set(activeRules);
    } catch {
      // ignore metadata count errors
    }
  }

  private scopeKeyForCard(card: CardNode): string {
    return (card.moduleId || card.collection || 'general').trim() || 'general';
  }

  private formatScopeLabel(key: string): string {
    return key
      .replace(/[-_]+/g, ' ')
      .replace(/\b\w/g, (ch) => ch.toUpperCase());
  }

  async refreshResults(): Promise<void> {
    this.loading.set(true);
    try {
      const q = this.query().trim();
      if (!q) {
        const [recents, cols] = await Promise.all([
          this.bridge.listRecentCards(6),
          this.bridge.listCollections(),
        ]);
        this.recentCards.set(recents);
        this.state.syncPinnedCards(recents);

        const total = cols.reduce((sum, col) => sum + (col.cardCount ?? 0), 0);
        this.totalIndexedCount.set(total);

        // If persisted pinned IDs aren't in recents or snapshot cache yet, hydrate them once
        const pinnedIds = this.state.pinnedIds();
        const cached = this.state.pinnedCardsMap();
        const recentSet = new Set(recents.map((r) => r.id));
        const missingPinned = pinnedIds.some((id) => !recentSet.has(id) && !cached[id]);
        if (missingPinned && total > 0) {
          const fallback = await this.bridge.searchRolodex('', 50);
          this.state.syncPinnedCards(fallback);
        }

        this.selectedIndex.set(this.visibleCards().length > 0 ? 0 : -1);
      } else {
        const res = await this.bridge.searchRolodex(q, 10);
        this.searchCards.set(res);
        this.state.syncPinnedCards(res);
        if (
          this.activeScope() !== 'all' &&
          !this.matchedScopes().some((s) => s.key === this.activeScope())
        ) {
          this.activeScope.set('all');
        }
        this.selectedIndex.set(this.visibleCards().length > 0 ? 0 : -1);
      }
    } finally {
      this.loading.set(false);
    }
  }

  @HostListener('window:focus')
  onWindowFocus(): void {
    this.focusSearchInput();
  }

  @HostListener('document:click')
  onDocumentClick(): void {
    if (this.settingsOpen() || this.quickAddOpen()) {
      this.settingsOpen.set(false);
      this.quickAddOpen.set(false);
      this.scheduleResizeFn?.();
    }
  }

  @HostListener('window:keydown', ['$event'])
  onKeyDown(event: KeyboardEvent): void {
    if (this.modulesOpen() || this.patternsOpen()) {
      return;
    }

    if (event.key === 'Escape') {
      if (this.settingsOpen() || this.quickAddOpen()) {
        event.preventDefault();
        this.settingsOpen.set(false);
        this.quickAddOpen.set(false);
        this.scheduleResizeFn?.();
        return;
      }
      if (this.query().trim()) {
        event.preventDefault();
        this.clearQuery();
        return;
      }
      if (this.bridge.isDesktop()) {
        event.preventDefault();
        void this.bridge.hideWindow();
      }
      return;
    }

    const target = event.target as HTMLElement | null;
    const isSearchInput = target?.id === 'rolodex-search';
    const isOtherInput =
      !isSearchInput &&
      (target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        target?.tagName === 'SELECT');

    if (isOtherInput) {
      return;
    }

    const list = this.visibleCards();
    if (list.length === 0) {
      return;
    }

    if (event.key === 'ArrowDown') {
      event.preventDefault();
      this.selectedIndex.update((prev) => (prev < 0 ? 0 : (prev + 1) % list.length));
      this.scrollSelectedIntoView();
      return;
    }

    if (event.key === 'ArrowUp') {
      event.preventDefault();
      this.selectedIndex.update((prev) => (prev <= 0 ? list.length - 1 : prev - 1));
      this.scrollSelectedIntoView();
      return;
    }

    if (event.key === 'Enter') {
      const isInteractiveTarget =
        !isSearchInput &&
        Boolean(target?.closest('button, a, [role="button"], input, textarea, select'));
      if (isInteractiveTarget) {
        return;
      }

      const idx = this.selectedIndex();
      if (idx >= 0 && idx < list.length) {
        const shells = this.cardShells();
        const targetShell = shells[idx];
        if (targetShell) {
          event.preventDefault();
          targetShell.triggerPrimaryAction();
        }
      }
    }
  }

  private scrollSelectedIntoView(): void {
    const idx = this.selectedIndex();
    const shell = this.cardShells()[idx];
    shell?.scrollIntoViewIfNeeded();
  }
}
