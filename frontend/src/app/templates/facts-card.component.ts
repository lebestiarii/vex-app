import { ChangeDetectionStrategy, Component, computed, linkedSignal } from '@angular/core';
import { CardAction, FactItem, FactsLayoutData, FactView } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

/**
 * FactsCardComponent (`facts`):
 * Optional summary, optional segmented `views[]` switcher (e.g. Metric / Imperial),
 * and read-only Label / Value grid. Renders [Copy] ONLY when `fact.copyable === true`.
 */
@Component({
  selector: 'app-facts-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="facts-wrap">
      @if (summary()) {
        <p class="facts-summary">{{ summary() }}</p>
      }

      @if (views().length > 1) {
        <div class="views-bar" role="tablist" aria-label="Fact views">
          @for (view of views(); track view.label + $index) {
            <button
              type="button"
              role="tab"
              class="view-pill"
              [class.active]="activeViewIndex() === $index"
              [attr.aria-selected]="activeViewIndex() === $index"
              (click)="activeViewIndex.set($index)"
            >
              {{ view.label }}
            </button>
          }
        </div>
      }

      @if (activeFacts().length > 0) {
        <dl class="facts-grid">
          @for (fact of activeFacts(); track fact.label + $index) {
            <div class="fact-tile">
              <div class="fact-head">
                <dt class="fact-label">{{ fact.label }}</dt>
                @if (fact.copyable === true) {
                  <button
                    type="button"
                    class="fact-copy-btn"
                    (click)="triggerCopy(fact.label, fact.value)"
                    [attr.aria-label]="'Copy ' + fact.label"
                  >
                    Copy
                  </button>
                }
              </div>
              <dd class="fact-value" [class.font-mono]="fact.mono">{{ fact.value }}</dd>
            </div>
          }
        </dl>
      }

      @if (actions().length > 0) {
        <div class="facts-actions">
          @for (act of actions(); track act.id) {
            <button
              type="button"
              class="card-action-btn"
              (click)="runCardAction(act)"
            >
              {{ act.label }}@if (act.type === 'open_url' || act.type === 'open_pwa') { <span aria-hidden="true">↗</span>}
            </button>
          }
        </div>
      }
    </div>
  `,
  styles: [
    `
      .facts-wrap {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .facts-summary {
        margin: 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-secondary);
      }
      .views-bar {
        display: inline-flex;
        align-self: flex-start;
        padding: 2px;
        gap: 2px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--bg-subtle);
      }
      .view-pill {
        min-block-size: 24px;
        padding: 2px 10px;
        border-radius: 6px;
        border: 1px solid transparent;
        background: transparent;
        color: var(--text-secondary);
        font-size: 11.5px;
        font-weight: 600;
        cursor: pointer;
        transition: background-color 0.14s ease, color 0.14s ease, border-color 0.14s ease;
      }
      .view-pill.active {
        background: var(--surface-l0);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
        box-shadow: var(--shadow-card);
      }
      .facts-grid {
        margin: 0;
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
        gap: 6px;
      }
      .fact-tile {
        padding: 7px 9px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
        display: flex;
        flex-direction: column;
        gap: 3px;
        min-width: 0;
      }
      .fact-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 6px;
      }
      .fact-label {
        font-size: 10.5px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.03em;
        color: var(--text-muted);
      }
      .fact-copy-btn {
        min-block-size: 20px;
        padding: 1px 6px;
        border-radius: 5px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 10.5px;
        font-weight: 600;
        cursor: pointer;
        flex-shrink: 0;
      }
      .fact-copy-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .fact-value {
        margin: 0;
        font-size: 12.5px;
        font-weight: 600;
        color: var(--text-primary);
        line-height: 1.35;
        overflow-wrap: anywhere;
      }
      .fact-value.font-mono {
        font-size: 12px;
      }
      .facts-actions {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
      }
      .card-action-btn {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        min-block-size: 26px;
        padding: 4px 10px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11.5px;
        font-weight: 600;
        cursor: pointer;
      }
      .card-action-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
    `,
  ],
})
export class FactsCardComponent extends BaseCardTemplate<FactsLayoutData> {
  readonly activeViewIndex = linkedSignal<number>(() => {
    this.node();
    return 0;
  });

  readonly summary = computed(() => (this.node().data?.summary ?? '').trim());

  readonly views = computed<FactView[]>(() => {
    const raw = this.node().data?.views;
    return Array.isArray(raw) ? raw : [];
  });

  readonly activeFacts = computed<FactItem[]>(() => {
    const v = this.views();
    if (v.length > 0) {
      const idx = Math.min(v.length - 1, Math.max(0, this.activeViewIndex()));
      return Array.isArray(v[idx]?.facts) ? v[idx].facts : [];
    }
    const raw = this.node().data?.facts;
    return Array.isArray(raw) ? raw : [];
  });

  readonly actions = computed<CardAction[]>(() => this.node().actions ?? []);

  runCardAction(act: CardAction): void {
    if (act.type === 'open_url' || act.type === 'open_pwa') {
      this.openExternal(act.target);
      return;
    }
    this.triggerCopy(act.label, act.target);
  }
}
