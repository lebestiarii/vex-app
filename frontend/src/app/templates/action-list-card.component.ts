import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { ActionListLayoutData, ActionListRow, RowAction } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

/**
 * ActionListCardComponent (`action-list`):
 * Compact full-width rows with optional stoplight badge, primary text (optional mono),
 * muted subtitle, and 1–3 copy / open_url buttons laid out so titles never wrap awkwardly
 * against right-side action pills.
 */
@Component({
  selector: 'app-action-list-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="action-list" role="list">
      @for (row of rows(); track row.primary + $index) {
        <div class="action-row" role="listitem">
          <div class="row-identity">
            <div class="row-headline">
              @if (row.badge; as badge) {
                <span class="row-badge" [attr.data-tone]="badge.tone || 'neutral'">
                  @if (badge.dot) {
                    <span class="stoplight-dot" [attr.data-dot]="badge.dot" aria-hidden="true"></span>
                  }
                  {{ badge.label }}
                </span>
              }
              <span class="row-primary" [class.font-mono]="row.mono">{{ row.primary }}</span>
            </div>
            @if (row.subtitle) {
              <div class="row-subtitle">{{ row.subtitle }}</div>
            }
          </div>

          <div class="row-actions">
            @for (act of row.actions; track act.label + act.value + $index) {
              <button
                type="button"
                class="row-action-btn"
                [attr.data-tone]="act.tone || 'neutral'"
                [class.has-dot]="!!act.dot"
                (click)="runRowAction(row, act)"
                [attr.aria-label]="actionAriaLabel(row, act)"
              >
                @if (act.dot) {
                  <span class="stoplight-dot" [attr.data-dot]="act.dot" aria-hidden="true"></span>
                }
                <span>{{ act.label }}</span>
                @if (act.type === 'open_url' && !act.label.includes('↗')) {
                  <span class="open-glyph" aria-hidden="true">↗</span>
                }
              </button>
            }
          </div>
        </div>
      }
      @if (rows().length === 0) {
        <p class="empty-note">No action rows configured.</p>
      }
    </div>
  `,
  styles: [
    `
      .action-list {
        display: flex;
        flex-direction: column;
        gap: 6px;
      }
      .action-row {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        justify-content: space-between;
        gap: 6px 10px;
        padding: 8px 10px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
      }
      /* Give primary text enough flex-basis so titles never crush into narrow vertical slivers next to pills */
      .row-identity {
        flex: 1 1 180px;
        min-width: 165px;
        display: flex;
        flex-direction: column;
        gap: 2px;
      }
      .row-headline {
        display: flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
      }
      .row-badge {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        padding: 1.5px 7px;
        border-radius: 999px;
        font-size: 10.5px;
        font-weight: 700;
        letter-spacing: 0.02em;
        border: 1px solid var(--chip-neutral-border);
        background: var(--chip-neutral-bg);
        color: var(--chip-neutral-text);
        white-space: nowrap;
        flex-shrink: 0;
      }
      .row-badge[data-tone='green'],
      .row-action-btn[data-tone='green'] {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .row-badge[data-tone='amber'],
      .row-action-btn[data-tone='amber'] {
        background: var(--chip-amber-bg);
        border-color: var(--chip-amber-border);
        color: var(--chip-amber-text);
      }
      .row-badge[data-tone='red'],
      .row-action-btn[data-tone='red'] {
        background: var(--chip-red-bg);
        border-color: var(--chip-red-border);
        color: var(--chip-red-text);
      }
      .row-badge[data-tone='blue'],
      .row-action-btn[data-tone='blue'] {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .row-primary {
        font-size: 12.5px;
        font-weight: 600;
        color: var(--text-primary);
        line-height: 1.3;
        min-width: 0;
        overflow-wrap: break-word;
      }
      .row-primary.font-mono {
        font-size: 12px;
        word-break: break-all;
      }
      .row-subtitle {
        font-size: 11px;
        color: var(--text-muted);
        line-height: 1.3;
      }
      .row-actions {
        display: inline-flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 4px;
        flex-shrink: 0;
        margin-inline-start: auto;
      }
      .row-action-btn {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        min-block-size: 24px;
        padding: 2px 8px;
        border-radius: 6px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
        white-space: nowrap;
        transition: background-color 0.14s ease, border-color 0.14s ease, color 0.14s ease, filter 0.14s ease;
      }
      .row-action-btn.has-dot {
        border-radius: 999px;
        font-weight: 700;
      }
      .row-action-btn[data-tone='neutral']:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .row-action-btn:not([data-tone='neutral']):hover {
        filter: brightness(0.95);
      }
      .stoplight-dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        flex-shrink: 0;
      }
      .stoplight-dot[data-dot='green'] {
        background: var(--dot-green);
      }
      .stoplight-dot[data-dot='amber'] {
        background: var(--dot-amber);
      }
      .stoplight-dot[data-dot='red'] {
        background: var(--dot-red);
      }
      .open-glyph {
        font-size: 10.5px;
        opacity: 0.85;
      }
      .empty-note {
        margin: 0;
        font-size: 12px;
        color: var(--text-muted);
      }
    `,
  ],
})
export class ActionListCardComponent extends BaseCardTemplate<ActionListLayoutData> {
  readonly rows = computed<ActionListRow[]>(() => {
    const raw = this.node().data?.rows;
    return Array.isArray(raw) ? raw : [];
  });

  runRowAction(row: ActionListRow, act: RowAction): void {
    if (act.type === 'open_url') {
      this.openExternal(act.value);
      return;
    }
    const label = row.badge?.label ? `${row.badge.label} ${act.label}` : act.label;
    this.triggerCopy(label, act.value);
  }

  actionAriaLabel(row: ActionListRow, act: RowAction): string {
    const verb = act.type === 'open_url' ? 'Open' : 'Copy';
    return `${verb} ${row.primary} (${act.label})`;
  }
}
