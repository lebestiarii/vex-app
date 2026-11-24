import { NgComponentOutlet } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  linkedSignal,
} from '@angular/core';
import { CardNode, GlanceChip, WebsiteLinkData } from '../models/card-node.model';
import { CardTemplateRegistry } from '../services/card-template-registry.service';
import { RolodexStateService } from '../services/rolodex-state.service';

@Component({
  selector: 'app-rolodex-card-shell',
  standalone: true,
  imports: [NgComponentOutlet],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (singleLinkUrl(); as directUrl) {
      <!-- WebsiteLink Single-Button Mode (1 link): entire card is a clickable launch button with top-right ↗, no chevron -->
      <button
        type="button"
        class="rolodex-card single-link-card"
        [attr.data-depth]="depthClamped()"
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
                <span class="pin-glyph" aria-hidden="true">📌</span>
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
      <!-- Standard Expandable Stacked Card -->
      <article
        class="rolodex-card"
        [attr.data-depth]="depthClamped()"
        [class.expanded]="isExpanded()"
      >
        <div class="header-strip" (click)="toggleExpanded()">
          <div class="header-row-1">
            <div class="identity">
              <span class="card-icon" aria-hidden="true">{{ node().icon || '📄' }}</span>
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

              <span class="chevron" [class.open]="isExpanded()" aria-hidden="true">
                <svg viewBox="0 0 12 12" class="chevron-svg">
                  <path d="M4 2.5L8.5 6L4 9.5Z" fill="currentColor" />
                </svg>
              </span>
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

        @if (isExpanded()) {
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
        transition: background-color 0.15s ease, border-color 0.15s ease;
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

      /* Single-button WebsiteLink mode */
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

      .pin-btn.pinned {
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

  private readonly registry = inject(CardTemplateRegistry);
  private readonly state = inject(RolodexStateService);

  readonly isExpanded = linkedSignal<boolean>(() => Boolean(this.node().defaultExpanded));

  readonly depthClamped = computed(() => Math.min(2, Math.max(0, this.depth())));
  readonly canPin = computed(() => this.depth() === 0 && this.node().pinnable !== false);
  readonly isPinned = computed(() => this.state.isPinned(this.node().id));
  readonly isShaking = computed(() => this.state.shakingPinId() === this.node().id);

  readonly templateComponent = computed(() => this.registry.resolve(this.node().template));

  readonly outletInputs = computed(() => ({
    node: this.node(),
    depth: this.depth(),
  }));

  readonly nestedChildren = computed(() => {
    if (this.depth() >= 2) return [];
    return this.node().children ?? [];
  });

  /**
   * WebsiteLink Single-Button Mode:
   * When a website-link / env-bookmarks card has exactly 1 link (data.url or 1 bookmark with 1 URL)
   * and no nested children, omit the chevron and render the entire card as a clickable button.
   */
  readonly singleLinkUrl = computed<string | null>(() => {
    const n = this.node();
    if (n.template !== 'website-link' && n.template !== 'env-bookmarks') return null;
    if ((n.children?.length ?? 0) > 0) return null;
    const d = (n.data ?? {}) as WebsiteLinkData;
    if (d.url && (!d.bookmarks || d.bookmarks.length === 0)) {
      return d.url;
    }
    if (Array.isArray(d.bookmarks) && d.bookmarks.length === 1) {
      const bm = d.bookmarks[0];
      const urls: string[] = [];
      if (Array.isArray(bm.links)) {
        bm.links.forEach((l) => {
          if (l.url) urls.push(l.url);
        });
      } else {
        if (bm.qaUrl) urls.push(bm.qaUrl);
        if (bm.prodUrl) urls.push(bm.prodUrl);
      }
      if (urls.length === 1) return urls[0];
    }
    return null;
  });

  readonly showLeafBody = computed(() => {
    const n = this.node();
    // Location is strictly header-only (expanded state shows ONLY full-width nested children)
    if (n.template === 'location' || n.template === 'branch-gateway') {
      return false;
    }
    if (n.template === 'container-blueprint' && !n.data?.mode && this.nestedChildren().length > 0) {
      return false;
    }
    return true;
  });

  readonly displayChips = computed(() => {
    this.state.clockNow();
    const raw = (this.node().chips ?? []).slice(0, 4);
    return raw.map((chip) => ({
      ...chip,
      renderedLabel: this.state.formatClockChip(chip.label),
    }));
  });

  shouldUseMono(chip: GlanceChip & { renderedLabel: string }): boolean {
    if (chip.tone === 'mono') return true;
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
    this.isExpanded.update((open) => !open);
  }

  onTitleClick(event: MouseEvent): void {
    event.stopPropagation();
    this.toggleExpanded();
  }

  onPinClick(event: Event): void {
    event.stopPropagation();
    event.preventDefault();
    this.state.togglePin(this.node().id);
  }

  openSingleLink(url: string): void {
    this.state.openExternalUrl(url);
  }
}
