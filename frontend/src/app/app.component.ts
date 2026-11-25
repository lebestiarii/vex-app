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
                #searchInputEl
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
                  (click)="clearQuery()"
                  aria-label="Clear search"
                >
                  Clear
                </button>
              }
            </div>
          </div>

          <div class="controls-bar">
            <span class="bar-hint">
              @if (isEmptyQuery()) {
                {{ totalIndexedCount() }} indexed {{ totalIndexedCount() === 1 ? 'card' : 'cards' }}
              } @else {
                {{ orderedSearchCards().length }} {{ orderedSearchCards().length === 1 ? 'match' : 'matches' }}
                @if (totalIndexedCount() > 0) {
                  · {{ totalIndexedCount() }} indexed
                }
              }
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
          } @else if (isEmptyQuery()) {
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
          } @else if (orderedSearchCards().length > 0) {
            <div class="card-stack">
              @for (card of orderedSearchCards(); track card.id) {
                <app-rolodex-card-shell
                  [node]="card"
                  [depth]="0"
                  [selected]="selectedIndex() === $index"
                  [autoExpand]="orderedSearchCards().length === 1"
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

      .search-input::-webkit-search-cancel-button {
        -webkit-appearance: none;
        appearance: none;
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
        gap: 12px;
        scrollbar-color: var(--scrollbar-thumb) var(--scrollbar-track);
      }

      .card-section {
        display: flex;
        flex-direction: column;
        gap: 8px;
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
  private readonly destroyRef = inject(DestroyRef);

  readonly maxPins = MAX_PINS;

  readonly query = signal<string>('');
  readonly searchCards = signal<CardNode[]>([]);
  readonly recentCards = signal<CardNode[]>([]);
  readonly totalIndexedCount = signal<number>(0);
  readonly selectedIndex = signal<number>(-1);
  readonly loading = signal<boolean>(false);
  readonly modulesOpen = signal<boolean>(false);
  readonly onboardingMode = signal<boolean>(false);
  readonly patternsOpen = signal<boolean>(false);

  private readonly searchInputEl = viewChild<ElementRef<HTMLInputElement>>('searchInputEl');
  private readonly cardShells = viewChildren(RolodexCardShellComponent);

  private searchTimer: ReturnType<typeof setTimeout> | null = null;

  readonly isEmptyQuery = computed(() => this.query().trim() === '');

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

  readonly visibleCards = computed<CardNode[]>(() => {
    if (this.isEmptyQuery()) {
      return [...this.pinnedEmptyCards(), ...this.recentEmptyCards()];
    }
    return this.orderedSearchCards();
  });

  constructor() {
    this.destroyRef.onDestroy(() => {
      if (this.searchTimer) {
        clearTimeout(this.searchTimer);
      }
    });
  }

  async ngOnInit(): Promise<void> {
    const prefs = await this.bridge.getUserPrefs();
    await this.refreshResults();

    if (!prefs.onboardingCompleted && this.totalIndexedCount() === 0 && this.visibleCards().length === 0) {
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

  clearQuery(): void {
    if (this.searchTimer) {
      clearTimeout(this.searchTimer);
      this.searchTimer = null;
    }
    this.query.set('');
    void this.refreshResults();
    this.searchInputEl()?.nativeElement.focus();
  }

  openModules(onboarding: boolean): void {
    this.onboardingMode.set(onboarding);
    this.modulesOpen.set(true);
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
        this.selectedIndex.set(res.length > 0 ? 0 : -1);
      }
    } finally {
      this.loading.set(false);
    }
  }

  @HostListener('window:keydown', ['$event'])
  onKeyDown(event: KeyboardEvent): void {
    if (this.modulesOpen() || this.patternsOpen()) {
      return;
    }

    if (event.key === 'Escape') {
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
