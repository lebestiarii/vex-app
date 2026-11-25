import { NgComponentOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  computed,
  ElementRef,
  inject,
  input,
  linkedSignal,
} from '@angular/core';
import {
  ActionListLayoutData,
  CardNode,
  ConvertLayoutData,
  FactsLayoutData,
  GlanceChip,
  LinkLayoutData,
  SegmentsLayoutData,
  SnippetLayoutData,
  StepsLayoutData,
  TableLayoutData,
} from '../models/card-node.model';
import { CardTemplateRegistry } from '../services/card-template-registry.service';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';

function hasRenderableLayoutBody(node: CardNode<any>): boolean {
  const data = node.data ?? {};
  switch (node.layout) {
    case 'group':
    case 'link':
      return false;
    case 'action-list': {
      const d = data as ActionListLayoutData;
      return Array.isArray(d.rows) && d.rows.length > 0;
    }
    case 'facts': {
      const d = data as FactsLayoutData;
      return (
        Boolean(d.summary?.trim()) ||
        (Array.isArray(d.facts) && d.facts.length > 0) ||
        (Array.isArray(d.views) && d.views.some((v) => Array.isArray(v.facts) && v.facts.length > 0)) ||
        (Array.isArray(node.actions) && node.actions.length > 0)
      );
    }
    case 'segments': {
      const d = data as SegmentsLayoutData;
      return (
        (Array.isArray(d.segments) && d.segments.length > 0) ||
        Boolean(d.explanation?.trim()) ||
        Boolean(node.description?.trim())
      );
    }
    case 'steps': {
      const d = data as StepsLayoutData;
      return (
        (Array.isArray(d.steps) && d.steps.length > 0) ||
        Boolean(d.summary?.trim()) ||
        (Array.isArray(d.bullets) && d.bullets.length > 0)
      );
    }
    case 'convert': {
      const d = data as ConvertLayoutData;
      return Boolean(d.input?.trim()) || Boolean(d.output?.trim());
    }
    case 'table': {
      const d = data as TableLayoutData;
      return Array.isArray(d.columns) && d.columns.length > 0;
    }
    case 'snippet': {
      const d = data as SnippetLayoutData;
      return Boolean(d.body?.trim());
    }
    default:
      return false;
  }
}

@Component({
  selector: 'app-rolodex-card-shell',
  standalone: true,
  imports: [NgComponentOutlet],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (singleLinkUrl(); as directUrl) {
      <!-- Link Layout: Single-Button clickable card with top-right ↗, no chevron -->
      <button
        type="button"
        class="rolodex-card single-link-card"
        [attr.data-depth]="depthClamped()"
        [class.selected]="selected()"
        (click)="openSingleLink(directUrl)"
        [attr.aria-label]="'Open ' + node().title"
      >
        <div class="header-row-1">
          <div class="identity">
            <span class="card-icon" aria-hidden="true">{{ node().icon || '🌐' }}</span>
            <span class="title-text" [class.title-l0]="depth() === 0" [class.title-nested]="depth() > 0">
              {{ node().title }}
            </span>
          </div>
          <div class="right-controls">
            @if (canPin()) {
              <span
                role="button"
                tabindex="0"
                class="pin-btn"
                [class.pinned]="isPinned()"
                [class.pin-shake]="isShaking()"
                [attr.aria-pressed]="isPinned()"
                [attr.aria-label]="isPinned() ? 'Unpin ' + node().title : 'Pin ' + node().title"
                (click)="onPinClick($event)"
                (keydown.enter)="onPinClick($event)"
                (keyup.space)="onPinClick($event)"
              >
                <span class="pin-glyph" [class.muted]="!isPinned()" aria-hidden="true">📌</span>
              </span>
            }
            <span class="external-arrow" aria-hidden="true">↗</span>
          </div>
        </div>

        @if (displayChips().length > 0) {
          <div class="header-row-2">
            @for (chip of displayChips(); track chip.label + $index) {
              <span
                class="glance-chip"
                [attr.data-tone]="chip.tone"
                [class.is-mono]="shouldUseMono(chip)"
              >
                @if (chip.dot) {
                  <span class="chip-dot" [attr.data-dot]="chip.dot" aria-hidden="true"></span>
                }
                {{ chip.renderedLabel }}
              </span>
            }
          </div>
        }
      </button>
    } @else {
      <!-- Standard Stacked Card (expandable ONLY when leaf body or children exist) -->
      <article
        class="rolodex-card"
        [attr.data-depth]="depthClamped()"
        [class.expanded]="isExpandable() && isExpanded()"
        [class.selected]="selected()"
      >
        <div
          class="header-strip"
          [class.is-static]="!isExpandable()"
          (click)="isExpandable() && toggleExpanded()"
        >
          <div class="header-row-1">
            <div class="identity">
              <span class="card-icon" aria-hidden="true">{{ node().icon || '📄' }}</span>
              @if (isExpandable()) {
                <button
                  type="button"
                  class="title-trigger"
                  [class.title-l0]="depth() === 0"
                  [class.title-nested]="depth() > 0"
                  [attr.aria-expanded]="isExpanded()"
                  (click)="onTitleClick($event)"
                >
                  {{ node().title }}
                </button>
              } @else {
                <span
                  class="title-text"
                  [class.title-l0]="depth() === 0"
                  [class.title-nested]="depth() > 0"
                >
                  {{ node().title }}
                </span>
              }
            </div>

            <div class="right-controls">
              @if (canPin()) {
                <button
                  type="button"
                  class="pin-btn"
                  [class.pinned]="isPinned()"
                  [class.pin-shake]="isShaking()"
                  [attr.aria-pressed]="isPinned()"
                  [attr.aria-label]="isPinned() ? 'Unpin ' + node().title : 'Pin ' + node().title"
                  (click)="onPinClick($event)"
                >
                  <span class="pin-glyph" [class.muted]="!isPinned()" aria-hidden="true">📌</span>
                </button>
              }

              @if (isExpandable()) {
                <span class="chevron" [class.open]="isExpanded()" aria-hidden="true">
                  <svg viewBox="0 0 12 12" class="chevron-svg">
                    <path d="M4 2.5L8.5 6L4 9.5Z" fill="currentColor" />
                  </svg>
                </span>
              }
            </div>
          </div>

          @if (displayChips().length > 0) {
            <div class="header-row-2">
              @for (chip of displayChips(); track chip.label + $index) {
                <span
                  class="glance-chip"
                  [attr.data-tone]="chip.tone"
                  [class.is-mono]="shouldUseMono(chip)"
                >
                  @if (chip.dot) {
                    <span class="chip-dot" [attr.data-dot]="chip.dot" aria-hidden="true"></span>
                  }
                  {{ chip.renderedLabel }}
                </span>
              }
            </div>
          }
        </div>

        @if (isExpandable() && isExpanded()) {
          <div class="expanded-body">
            @if (showLeafBody()) {
              <div class="leaf-slot">
                <ng-container
                  *ngComponentOutlet="templateComponent(); inputs: outletInputs()"
                />
              </div>
            }

            @if (nestedChildren().length > 0) {
              <div class="nested-stack" role="group" [attr.aria-label]="node().title + ' items'">
                @for (child of nestedChildren(); track child.id) {
                  <app-rolodex-card-shell [node]="child" [depth]="depth() + 1" />
                }
              </div>
            }
          </div>
        }
      </article>
    }
  `,
  styles: [
    `
      :host {
        display: block;
        width: 100%;
      }

      .rolodex-card {
        display: block;
        width: 100%;
        border: 1px solid var(--border-l0);
        transition: background-color 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease;
      }

      /* Depth-driven alternating inset surfaces (NO left-rail lines) */
      .rolodex-card[data-depth='0'] {
        background: var(--surface-l0);
        border-color: var(--border-l0);
        border-radius: 12px;
        padding: 12px;
        box-shadow: var(--shadow-card);
      }

      .rolodex-card[data-depth='1'] {
        background: var(--surface-l1);
        border-color: var(--border-nested);
        border-radius: 10px;
        padding: 10px;
      }

      .rolodex-card[data-depth='2'] {
        background: var(--surface-l2);
        border-color: var(--border-nested);
        border-radius: 10px;
        padding: 10px;
      }

      /* Level 0 keyboard selection highlight */
      .rolodex-card[data-depth='0'].selected {
        border-color: var(--accent);
        box-shadow: 0 0 0 1px var(--accent), var(--shadow-card);
      }

      /* Single-button Link mode */
      .single-link-card {
        text-align: start;
        cursor: pointer;
      }

      .single-link-card:hover {
        border-color: var(--chip-blue-border);
        background: var(--surface-hover);
      }

      .header-strip {
        cursor: pointer;
        user-select: none;
      }

      .header-strip.is-static {
        cursor: default;
      }

      .header-row-1 {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }

      .identity {
        display: flex;
        align-items: center;
        gap: 8px;
        min-width: 0;
        flex: 1;
      }

      .card-icon {
        font-size: 17px;
        line-height: 1;
        flex-shrink: 0;
      }

      .title-trigger,
      .title-text {
        appearance: none;
        background: transparent;
        border: 0;
        padding: 0;
        margin: 0;
        text-align: start;
        font-weight: 600;
        color: var(--text-primary);
        line-height: 1.3;
        min-width: 0;
        overflow-wrap: anywhere;
      }

      .title-trigger {
        cursor: pointer;
      }

      .title-l0 {
        font-size: 15px;
      }

      .title-nested {
        font-size: 14px;
      }

      .right-controls {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        flex-shrink: 0;
      }

      .pin-btn {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        min-inline-size: 28px;
        min-block-size: 28px;
        padding: 3px;
        border-radius: 8px;
        border: 1px solid transparent;
        background: transparent;
        cursor: pointer;
        transition: background-color 0.14s ease, border-color 0.14s ease;
      }

      .pin-btn:hover {
        background: var(--bg-subtle);
      }

      .pin-btn.pinned,
      .pin-btn.pinned:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
      }

      .pin-glyph {
        font-size: 14px;
        line-height: 1;
      }

      .pin-glyph.muted {
        opacity: 0.42;
        filter: grayscale(0.4);
      }

      .chevron {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 20px;
        height: 20px;
        color: var(--text-muted);
        transition: transform 0.16s ease;
      }

      .chevron.open {
        transform: rotate(90deg);
      }

      .external-arrow {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 24px;
        height: 24px;
        border-radius: 6px;
        color: var(--accent);
        font-size: 14px;
        font-weight: 700;
      }

      @media (prefers-reduced-motion: reduce) {
        .chevron {
          transition: none;
        }
      }

      .chevron-svg {
        width: 10px;
        height: 10px;
      }

      .header-row-2 {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
        margin-block-start: 8px;
      }

      .glance-chip {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        padding: 2px 8px;
        border-radius: 7px;
        font-size: 12px;
        font-weight: 600;
        line-height: 1.3;
        border: 1px solid var(--chip-neutral-border);
        background: var(--chip-neutral-bg);
        color: var(--chip-neutral-text);
      }

      .glance-chip[data-tone='blue'] {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }

      .glance-chip[data-tone='amber'] {
        background: var(--chip-amber-bg);
        border-color: var(--chip-amber-border);
        color: var(--chip-amber-text);
      }

      .glance-chip[data-tone='green'] {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }

      .glance-chip[data-tone='red'] {
        background: var(--chip-red-bg);
        border-color: var(--chip-red-border);
        color: var(--chip-red-text);
      }

      .glance-chip[data-tone='mono'],
      .glance-chip.is-mono {
        font-family: var(--font-mono);
        font-size: 11.5px;
        letter-spacing: 0.01em;
      }

      .glance-chip:has(.chip-dot) {
        border-radius: 999px;
        font-size: 11px;
        font-weight: 700;
        letter-spacing: 0.02em;
      }

      .chip-dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        flex-shrink: 0;
      }

      .chip-dot[data-dot='green'] {
        background: var(--dot-green);
      }

      .chip-dot[data-dot='amber'] {
        background: var(--dot-amber);
      }

      .chip-dot[data-dot='red'] {
        background: var(--dot-red);
      }

      /* Expanded section separated by subtle 1px top border; full-width 8px vertical stack */
      .expanded-body {
        margin-block-start: 10px;
        padding-block-start: 10px;
        border-block-start: 1px solid var(--border-divider);
        display: flex;
        flex-direction: column;
        gap: 10px;
      }

      .nested-stack {
        display: flex;
        flex-direction: column;
        gap: 8px;
        width: 100%;
      }
    `,
  ],
})
export class RolodexCardShellComponent {
  readonly node = input.required<CardNode<any>>();
  readonly depth = input<number>(0);
  readonly selected = input<boolean>(false);
  readonly autoExpand = input<boolean>(false);

  private readonly hostEl = inject(ElementRef<HTMLElement>);
  private readonly registry = inject(CardTemplateRegistry);
  private readonly state = inject(RolodexStateService);
  private readonly bridge = inject(VexBridgeService);

  readonly isExpanded = linkedSignal<boolean>(() =>
    Boolean(this.node().defaultExpanded || this.autoExpand())
  );

  readonly depthClamped = computed(() => Math.min(2, Math.max(0, this.depth())));
  readonly canPin = computed(() => this.depth() === 0 && this.node().pinnable !== false);
  readonly isPinned = computed(() => this.state.isPinned(this.node().id));
  readonly isShaking = computed(() => this.state.shakingPinId() === this.node().id);

  readonly templateComponent = computed(() => this.registry.resolve(this.node().layout));

  readonly outletInputs = computed(() => ({
    node: this.node(),
    depth: this.depth(),
  }));

  readonly nestedChildren = computed(() => {
    if (this.depth() >= 2) return [];
    return this.node().children ?? [];
  });

  /**
   * Link Single-Button Mode:
   * A `link` card with a valid URL and no nested children renders as a direct launch button
   * with top-right ↗ and no chevron.
   */
  readonly singleLinkUrl = computed<string | null>(() => {
    const n = this.node();
    if (n.layout !== 'link') return null;
    if (this.nestedChildren().length > 0) return null;
    const url = ((n.data ?? {}) as LinkLayoutData).url?.trim();
    return url ? url : null;
  });

  readonly showLeafBody = computed(() => hasRenderableLayoutBody(this.node()));

  readonly isExpandable = computed(
    () => !this.singleLinkUrl() && (this.showLeafBody() || this.nestedChildren().length > 0)
  );

  readonly displayChips = computed(() => {
    this.state.clockNow();
    const raw = (this.node().chips ?? []).slice(0, 4);
    return raw.map((chip) => {
      const isClock = this.state.isClockChip(chip.label);
      return {
        ...chip,
        isClock,
        renderedLabel: isClock ? this.state.formatClockChip(chip.label) : chip.label,
      };
    });
  });

  shouldUseMono(chip: GlanceChip & { renderedLabel: string; isClock: boolean }): boolean {
    if (chip.tone === 'mono' || chip.isClock) return true;
    const text = chip.renderedLabel;
    return (
      /^[A-Z]{5}\s*·\s*[A-Z]{3}$/.test(text) ||
      /^\d{2}:\d{2}$/.test(text) ||
      /^-?\d+(\.\d+)?°[CF]$/.test(text) ||
      /^ISO\s+/i.test(text) ||
      /^\d+'×\d+'/.test(text)
    );
  }

  toggleExpanded(): void {
    if (!this.isExpandable()) return;
    const next = !this.isExpanded();
    this.isExpanded.set(next);
    if (next) {
      void this.bridge.recordCardUsed(this.node().id);
    }
  }

  triggerPrimaryAction(): void {
    const directUrl = this.singleLinkUrl();
    if (directUrl) {
      this.openSingleLink(directUrl);
      return;
    }

    const n = this.node();
    if (n.layout === 'convert') {
      const output = ((n.data ?? {}) as ConvertLayoutData).output?.trim();
      if (output) {
        void this.state.copyText('Converted Value', output);
        void this.bridge.recordCardUsed(n.id);
        return;
      }
    }

    if (this.isExpandable()) {
      this.toggleExpanded();
    }
  }

  scrollIntoViewIfNeeded(): void {
    const el = this.hostEl.nativeElement;
    if (typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'nearest' });
    }
  }

  onTitleClick(event: MouseEvent): void {
    event.stopPropagation();
    this.toggleExpanded();
  }

  onPinClick(event: Event): void {
    event.stopPropagation();
    event.preventDefault();
    this.state.togglePin(this.node());
  }

  openSingleLink(url: string): void {
    this.state.openExternalUrl(url);
    void this.bridge.recordCardUsed(this.node().id);
  }
}
