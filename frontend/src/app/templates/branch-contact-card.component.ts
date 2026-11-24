import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { BranchContactData, ContactRow } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

@Component({
  selector: 'app-branch-contact-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="contact-card">
      @for (row of rows(); track row.label + row.value) {
        <div class="contact-row">
          <span class="c-label">{{ row.label }}</span>
          <span class="c-val" [class.font-mono]="row.mono">{{ row.value }}</span>
          <div class="c-btn-group">
            <button type="button" class="c-btn" (click)="triggerCopy(row.label, row.value)">Copy</button>
            @if (row.url) {
              <button type="button" class="c-btn accent" (click)="openExternal(row.url)">
                {{ row.actionLabel || 'Open' }}
              </button>
            }
          </div>
        </div>
      }
      @if (footerNote(); as note) {
        <div class="hours-note">{{ note }}</div>
      }
    </div>
  `,
  styles: [
    `
      .contact-card {
        display: flex;
        flex-direction: column;
        gap: 6px;
      }
      .contact-row {
        display: grid;
        grid-template-columns: 84px minmax(0, 1fr) auto;
        align-items: center;
        gap: 8px;
        padding: 7px 10px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
      }
      .c-label {
        font-size: 11px;
        font-weight: 600;
        color: var(--text-muted);
      }
      .c-val {
        font-size: 12.5px;
        color: var(--text-primary);
        word-break: break-word;
      }
      .c-btn-group {
        display: inline-flex;
        gap: 5px;
      }
      .c-btn {
        min-block-size: 26px;
        padding: 3px 9px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        font-size: 11.5px;
        font-weight: 600;
        cursor: pointer;
      }
      .c-btn:hover {
        background: var(--surface-hover);
      }
      .c-btn.accent {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .hours-note {
        font-size: 11.5px;
        color: var(--text-muted);
        padding-inline: 2px;
      }
    `,
  ],
})
export class BranchContactCardComponent extends BaseCardTemplate<BranchContactData> {
  readonly rows = computed<ContactRow[]>(() => {
    const d = this.node().data ?? {};
    if (Array.isArray(d.rows) && d.rows.length > 0) {
      return d.rows;
    }
    const out: ContactRow[] = [];
    if (d.phone) out.push({ label: 'Phone', value: d.phone, mono: true });
    if (d.email) out.push({ label: 'Email', value: d.email, mono: true });
    if (d.address) {
      out.push({
        label: 'Address',
        value: d.address,
        url: d.mapUrl,
        actionLabel: 'Map',
      });
    }
    return out;
  });

  readonly footerNote = computed(() => {
    const d = this.node().data ?? {};
    if (d.note) return d.note;
    if (d.hours) return `Hours: ${d.hours}`;
    return '';
  });
}
