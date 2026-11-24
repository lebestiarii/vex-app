import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { BaseCardTemplate } from './base-card-template';

@Component({
  selector: 'app-generic-kv-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="kv-wrap">
      @if (node().description) {
        <p class="kv-desc">{{ node().description }}</p>
      }

      @if (kvPairs().length > 0) {
        <div class="kv-grid">
          @for (item of kvPairs(); track item.key) {
            <div class="kv-item">
              <span class="kv-k">{{ item.label }}</span>
              <button
                type="button"
                class="kv-v font-mono"
                (click)="triggerCopy(item.label, item.value)"
                [attr.aria-label]="'Copy ' + item.label + ' ' + item.value"
              >
                {{ item.value }}
              </button>
            </div>
          }
        </div>
      }

      @if (actions().length > 0) {
        <div class="kv-actions">
          @for (act of actions(); track act.id) {
            <button type="button" class="kv-action-btn" (click)="runAction(act.type, act.label, act.target)">
              {{ act.label }}
            </button>
          }
        </div>
      }
    </div>
  `,
  styles: [
    `
      .kv-wrap {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .kv-desc {
        margin: 0;
        font-size: 12.5px;
        line-height: 1.48;
        color: var(--text-secondary);
      }
      .kv-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(145px, 1fr));
        gap: 6px;
      }
      .kv-item {
        padding: 7px 9px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
        display: flex;
        flex-direction: column;
        gap: 2px;
      }
      .kv-k {
        font-size: 10.5px;
        text-transform: uppercase;
        letter-spacing: 0.03em;
        color: var(--text-muted);
      }
      .kv-v {
        text-align: start;
        border: 0;
        background: transparent;
        padding: 0;
        font-size: 12px;
        font-weight: 600;
        color: var(--text-primary);
        cursor: pointer;
        word-break: break-word;
      }
      .kv-v:hover {
        color: var(--accent);
      }
      .kv-actions {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
      }
      .kv-action-btn {
        min-block-size: 26px;
        padding: 4px 10px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        font-size: 12px;
        font-weight: 600;
        cursor: pointer;
      }
      .kv-action-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
    `,
  ],
})
export class GenericKvCardComponent extends BaseCardTemplate<Record<string, unknown>> {
  readonly actions = computed(() => this.node().actions ?? []);

  readonly kvPairs = computed(() => {
    const data = this.node().data ?? {};
    const skip = new Set(['trackingUrl', 'shortName', 'portalTag', 'contract']);
    return Object.entries(data)
      .filter(
        ([k, v]) =>
          !skip.has(k) &&
          v !== null &&
          v !== undefined &&
          typeof v !== 'object' &&
          String(v).trim() !== ''
      )
      .slice(0, 8)
      .map(([k, v]) => ({
        key: k,
        label: k.replace(/([A-Z])/g, ' $1').replace(/_/g, ' ').trim(),
        value: String(v),
      }));
  });

  runAction(type: string, label: string, target: string): void {
    if (type === 'open_url' || type === 'open_pwa') {
      this.openExternal(target);
      return;
    }
    this.triggerCopy(label, target);
  }
}
