import {
  ChangeDetectionStrategy,
  Component,
  computed,
  HostListener,
  inject,
  OnInit,
  signal,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ModulesDialogComponent } from './components/modules-dialog.component';
import { PatternsDialogComponent } from './components/patterns-dialog.component';
import { RolodexCardShellComponent } from './components/rolodex-card-shell.component';
import { ToastRegionComponent } from './components/toast-region.component';
import { CardNode } from './models/card-node.model';
import { MAX_PINS, RolodexStateService } from './services/rolodex-state.service';
import { VexBridgeService } from './services/vex-bridge.service';

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
      <div class="widget-frame">
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
                id="rolodex-search"
                type="search"
                class="search-input"
                autocomplete="off"
                spellcheck="false"
                placeholder="Search cards, patterns, or conversions…"
                [ngModel]="query()"
                (ngModelChange)="onQueryChange($event)"
              />
              @if (query()) {
                <button
                  type="button"
                  class="clear-btn"
                  (click)="onQueryChange('')"
                  aria-label="Clear search"
                >
                  Clear
                </button>
              }
            </div>
          </div>

          <div class="controls-bar">
            <span class="bar-hint">
              {{ orderedCards().length }} {{ orderedCards().length === 1 ? 'card' : 'cards' }}
            </span>

            <div class="toolbar-actions">
              <button
                type="button"
                class="tool-btn"
                (click)="openModules(false)"
                aria-label="Manage optional modules and MCP"
              >
                Modules
              </button>
              <button
                type="button"
                class="tool-btn"
                (click)="patternsOpen.set(true)"
                aria-label="Configure pattern matching rules"
              >
                Patterns
              </button>
              <button
                type="button"
                class="tool-btn"
                (click)="state.toggleTheme()"
                [attr.aria-label]="'Switch to ' + (state.theme() === 'light' ? 'dark' : 'light') + ' mode'"
              >
                {{ state.theme() === 'light' ? 'Dark' : 'Light' }}
              </button>
            </div>
          </div>
        </header>

        <main class="frame-scroll" id="rolodex-main">
          @if (loading()) {
            <div class="status-card" role="status">Searching…</div>
          } @else if (orderedCards().length > 0) {
            @if (pinnedCountInView() > 0 && !query().trim()) {
              <div class="section-meta">
                <span>Pinned ({{ state.pinnedIds().length }}/{{ maxPins }})</span>
              </div>
            }

            <div class="card-stack">
              @for (card of orderedCards(); track card.id) {
                <app-rolodex-card-shell [node]="card" [depth]="0" />
              }
            </div>
          } @else {
            <div class="empty-state">
              <div class="empty-icon" aria-hidden="true">🗂️</div>
              <h2 class="empty-title">
                {{ query().trim() ? 'No matching cards' : 'No optional modules loaded' }}
              </h2>
              <p class="empty-desc">
                @if (query().trim()) {
                  No indexed cards or active pattern rules matched your query.
                } @else {
                  All modules are optional and deletable. Install reference packs in Modules or push cards via MCP.
                }
              </p>
              <div class="empty-actions">
                @if (query().trim()) {
                  <button type="button" class="primary-btn" (click)="onQueryChange('')">
                    Reset Search
                  </button>
                }
                <button type="button" class="primary-btn" (click)="openModules(true)">
                  Choose Starter Modules
                </button>
              </div>
            </div>
          }
        </main>
      </div>
    </div>

    <app-modules-dialog
      [open]="modulesOpen()"
      [onboardingMode]="onboardingMode()"
      (closed)="modulesOpen.set(false)"
      (modulesChanged)="refreshResults()"
    />

    <app-patterns-dialog
      [open]="patternsOpen()"
      (closed)="patternsOpen.set(false)"
      (patternsChanged)="refreshResults()"
    />

    <app-toast-region />
  `,
  styles: [
    `
      .widget-viewport {
        min-block-size: 100vh;
        padding: 12px;
        display: flex;
        justify-content: center;
        align-items: flex-start;
        background: var(--bg-canvas);
      }

      /* Responsive 420px-520px desktop widget frame (default 460px) with clean internal scroll */
      .widget-frame {
        width: clamp(420px, 100%, 460px);
        max-width: 520px;
        max-height: min(680px, calc(100vh - 24px));
        display: flex;
        flex-direction: column;
        background: var(--bg-shell);
        border: 1px solid var(--border-l0);
        border-radius: 16px;
        box-shadow: var(--shadow-shell);
        overflow: hidden;
      }

      @media (max-width: 444px) {
        .widget-viewport {
          padding: 6px;
        }
        .widget-frame {
          width: 100%;
          min-width: 0;
          max-height: min(680px, calc(100vh - 12px));
        }
      }

      .frame-header {
        padding: 12px 12px 10px;
        border-bottom: 1px solid var(--border-divider);
        background: var(--surface-l0);
        display: flex;
        flex-direction: column;
        gap: 8px;
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
        gap: 6px;
      }

      .search-field {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 0 10px;
        min-block-size: 38px;
        border-radius: 10px;
        border: 1px solid var(--border-l0);
        background: var(--bg-input);
        transition: border-color 0.14s ease;
      }

      .search-field:focus-within {
        border-color: var(--accent);
      }

      .search-icon {
        width: 16px;
        height: 16px;
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

      .clear-btn {
        min-block-size: 24px;
        padding: 2px 7px;
        border-radius: 6px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
      }

      .controls-bar {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }

      .bar-hint {
        font-size: 11.5px;
        font-weight: 500;
        color: var(--text-muted);
        padding-inline-start: 2px;
      }

      .toolbar-actions {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        flex-shrink: 0;
      }

      .tool-btn {
        min-block-size: 26px;
        padding: 2px 8px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11.5px;
        font-weight: 600;
        cursor: pointer;
      }

      .tool-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }

      .frame-scroll {
        padding: 12px;
        overflow-y: auto;
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 10px;
        scrollbar-color: var(--scrollbar-thumb) var(--scrollbar-track);
      }

      .section-meta {
        font-size: 11px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.05em;
        color: var(--text-muted);
        padding-inline: 2px;
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

  readonly maxPins = MAX_PINS;

  readonly query = signal<string>('');
  readonly cards = signal<CardNode[]>([]);
  readonly loading = signal<boolean>(false);
  readonly modulesOpen = signal<boolean>(false);
  readonly onboardingMode = signal<boolean>(false);
  readonly patternsOpen = signal<boolean>(false);

  private searchTimer: ReturnType<typeof setTimeout> | null = null;

  readonly orderedCards = computed<CardNode[]>(() => {
    const pinned = this.state.pinnedIds();
    const list = this.cards();
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

  readonly pinnedCountInView = computed(() => {
    const pinned = new Set(this.state.pinnedIds());
    return this.cards().filter((c) => pinned.has(c.id)).length;
  });

  async ngOnInit(): Promise<void> {
    const prefs = await this.bridge.getUserPrefs();
    await this.refreshResults();

    if (!prefs.onboardingCompleted && this.cards().length === 0) {
      this.openModules(true);
    }
  }

  onQueryChange(next: string): void {
    this.query.set(next);
    if (this.searchTimer) {
      clearTimeout(this.searchTimer);
    }
    this.searchTimer = setTimeout(() => {
      void this.refreshResults();
    }, 90);
  }

  openModules(onboarding: boolean): void {
    this.onboardingMode.set(onboarding);
    this.modulesOpen.set(true);
  }

  async refreshResults(): Promise<void> {
    this.loading.set(true);
    try {
      const res = await this.bridge.searchRolodex(this.query(), 24);
      this.cards.set(res);
    } finally {
      this.loading.set(false);
    }
  }

  @HostListener('window:keydown.escape')
  onEscape(): void {
    if (this.modulesOpen() || this.patternsOpen()) {
      return;
    }
    if (this.query().trim()) {
      this.onQueryChange('');
      return;
    }
    if (this.bridge.isDesktop()) {
      void this.bridge.hideWindow();
    }
  }
}
