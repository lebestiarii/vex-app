import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { ConversionData, ConversionEquivalent } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

@Component({
  selector: 'app-conversion-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="conv-wrap">
      <div class="conv-strip">
        <span class="conv-input font-mono">{{ inputText() }}</span>
        <span class="conv-arrow" aria-hidden="true">⇄</span>
        <span class="conv-output font-mono">{{ outputText() }}</span>
        <button
          type="button"
          class="copy-btn"
          (click)="triggerCopy('Converted Value', outputText())"
          [attr.aria-label]="'Copy ' + outputText()"
        >
          Copy
        </button>
      </div>

      @if (equivalents().length > 0) {
        <div class="equiv-row">
          @for (eq of equivalents(); track eq.label + eq.value) {
            <button
              type="button"
              class="equiv-chip"
              (click)="triggerCopy(eq.label, eq.value)"
              [attr.aria-label]="'Copy ' + eq.label + ' ' + eq.value"
            >
              <span class="eq-label">{{ eq.label }}</span>
              <span class="eq-val font-mono">{{ eq.value }}</span>
            </button>
          }
        </div>
      }
    </div>
  `,
  styles: [
    `
      .conv-wrap {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .conv-strip {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        padding: 9px 11px;
        border-radius: 9px;
        border: 1px solid var(--chip-blue-border);
        background: var(--chip-blue-bg);
      }
      .conv-input {
        font-size: 13px;
        font-weight: 600;
        color: var(--text-secondary);
      }
      .conv-arrow {
        color: var(--accent);
        font-weight: 700;
        font-size: 14px;
      }
      .conv-output {
        font-size: 14px;
        font-weight: 700;
        color: var(--chip-blue-text);
        flex: 1;
      }
      .copy-btn {
        min-block-size: 26px;
        padding: 3px 10px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l0);
        color: var(--text-primary);
        font-size: 11.5px;
        font-weight: 600;
        cursor: pointer;
      }
      .copy-btn:hover {
        border-color: var(--accent);
        color: var(--accent);
      }
      .equiv-row {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
      }
      .equiv-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 4px 8px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        font-size: 11.5px;
        cursor: pointer;
      }
      .equiv-chip:hover {
        border-color: var(--chip-blue-border);
        background: var(--chip-blue-bg);
      }
      .eq-label {
        color: var(--text-muted);
      }
      .eq-val {
        font-weight: 600;
        color: var(--text-primary);
      }
    `,
  ],
})
export class ConversionCardComponent extends BaseCardTemplate<ConversionData> {
  readonly inputText = computed(() => this.node().data?.input || this.node().title);
  readonly outputText = computed(() => this.node().data?.output || this.node().data?.result || '');
  readonly equivalents = computed<ConversionEquivalent[]>(() => {
    const raw = this.node().data?.equivalents;
    return Array.isArray(raw) ? raw : [];
  });
}
