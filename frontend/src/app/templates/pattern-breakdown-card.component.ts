import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { PatternBreakdownData, PatternSegment } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

@Component({
  selector: 'app-pattern-breakdown-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="breakdown-body">
      @if (segments().length > 0) {
        <div class="segment-grid" role="list" aria-label="Decoded segments">
          @for (seg of segments(); track seg.role + seg.value + $index) {
            <button
              type="button"
              class="segment-tile"
              role="listitem"
              [attr.data-tone]="seg.tone || defaultTone($index)"
              (click)="triggerCopy(seg.role, seg.value)"
              [attr.aria-label]="'Copy ' + seg.role + ' ' + seg.value"
            >
              <span class="seg-value font-mono">{{ seg.value }}</span>
              <span class="seg-role">{{ seg.role }}</span>
              <span class="seg-meaning">{{ seg.meaning }}</span>
            </button>
          }
        </div>
      }

      @if (explanation()) {
        <p class="breakdown-explanation">{{ explanation() }}</p>
      }
    </div>
  `,
  styles: [
    `
      .breakdown-body {
        display: flex;
        flex-direction: column;
        gap: 10px;
      }
      .segment-grid {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: 8px;
      }
      .segment-tile {
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        text-align: center;
        gap: 3px;
        padding: 10px 6px;
        border-radius: 10px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        cursor: pointer;
        transition: filter 0.14s ease, transform 0.12s ease;
      }
      .segment-tile:hover {
        filter: brightness(0.97);
      }
      .segment-tile[data-tone='blue'] {
        background: var(--tile-blue-bg);
        border-color: var(--tile-blue-border);
      }
      .segment-tile[data-tone='blue'] .seg-value {
        color: var(--tile-blue-val);
      }
      .segment-tile[data-tone='blue'] .seg-role {
        color: var(--tile-blue-role);
      }

      .segment-tile[data-tone='amber'] {
        background: var(--tile-amber-bg);
        border-color: var(--tile-amber-border);
      }
      .segment-tile[data-tone='amber'] .seg-value {
        color: var(--tile-amber-val);
      }
      .segment-tile[data-tone='amber'] .seg-role {
        color: var(--tile-amber-role);
      }

      .segment-tile[data-tone='green'] {
        background: var(--tile-green-bg);
        border-color: var(--tile-green-border);
      }
      .segment-tile[data-tone='green'] .seg-value {
        color: var(--tile-green-val);
      }
      .segment-tile[data-tone='green'] .seg-role {
        color: var(--tile-green-role);
      }

      .seg-value {
        font-size: 14px;
        font-weight: 700;
        line-height: 1.2;
        word-break: break-all;
      }
      .seg-role {
        font-size: 10px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.05em;
      }
      .seg-meaning {
        font-size: 11.5px;
        font-weight: 600;
        color: var(--text-primary);
        line-height: 1.25;
      }
      .breakdown-explanation {
        margin: 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-secondary);
      }
    `,
  ],
})
export class PatternBreakdownCardComponent extends BaseCardTemplate<PatternBreakdownData> {
  readonly segments = computed<PatternSegment[]>(() => {
    const raw = this.node().data?.segments;
    return Array.isArray(raw) ? raw : [];
  });

  readonly explanation = computed(() => {
    const d = this.node().data;
    return d?.explanation || this.node().description || '';
  });

  defaultTone(index: number): 'blue' | 'amber' | 'green' {
    const order: ('blue' | 'amber' | 'green')[] = ['blue', 'amber', 'green'];
    return order[index % order.length];
  }
}
