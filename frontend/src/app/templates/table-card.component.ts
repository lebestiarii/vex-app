import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { TableLayoutData } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

/**
 * TableCardComponent (`table`):
 * Sticky header (2–4 cols, max 8 visible rows with vertical scroll),
 * optional monospace columns, and [Copy CSV] / [Copy MD] footer buttons.
 */
@Component({
  selector: 'app-table-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="table-wrap">
      @if (columns().length > 0) {
        <div class="table-scroll" role="region" [attr.aria-label]="node().title + ' table'">
          <table class="data-table">
            <thead>
              <tr>
                @for (col of columns(); track col + $index) {
                  <th scope="col">{{ col }}</th>
                }
              </tr>
            </thead>
            <tbody>
              @for (row of rows(); track $index) {
                <tr>
                  @for (cell of row; track $index) {
                    <td [class.font-mono]="isMonoColumn($index)">{{ cell }}</td>
                  }
                </tr>
              }
            </tbody>
          </table>
        </div>

        <div class="table-footer">
          <span class="row-count font-mono">
            {{ rows().length }} {{ rows().length === 1 ? 'row' : 'rows' }}
          </span>
          <div class="footer-actions">
            <button
              type="button"
              class="footer-btn"
              (click)="copyCsv()"
              aria-label="Copy table as CSV"
            >
              Copy CSV
            </button>
            <button
              type="button"
              class="footer-btn"
              (click)="copyMarkdown()"
              aria-label="Copy table as Markdown"
            >
              Copy MD
            </button>
            @if (sourceUrl(); as src) {
              <button
                type="button"
                class="footer-btn"
                (click)="openExternal(src)"
                aria-label="Open table source"
              >
                Source ↗
              </button>
            }
          </div>
        </div>
      } @else {
        <p class="empty-note">No table columns configured.</p>
      }
    </div>
  `,
  styles: [
    `
      .table-wrap {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      /* Cap visible viewport to 8 data rows + sticky header */
      .table-scroll {
        max-block-size: 276px;
        overflow-y: auto;
        overflow-x: auto;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
        scrollbar-color: var(--scrollbar-thumb) var(--scrollbar-track);
      }
      .data-table {
        width: 100%;
        border-collapse: separate;
        border-spacing: 0;
        font-size: 12px;
        text-align: start;
      }
      .data-table thead th {
        position: sticky;
        top: 0;
        z-index: 1;
        padding: 7px 9px;
        font-size: 10.5px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        color: var(--text-muted);
        background: var(--bg-subtle);
        border-bottom: 1px solid var(--border-divider);
        white-space: nowrap;
      }
      .data-table tbody td {
        padding: 6px 9px;
        color: var(--text-primary);
        border-bottom: 1px solid var(--border-divider);
        line-height: 1.35;
        vertical-align: top;
      }
      .data-table tbody tr:last-child td {
        border-bottom: 0;
      }
      .data-table tbody tr:hover td {
        background: var(--surface-hover);
      }
      .data-table tbody td.font-mono {
        font-size: 11.5px;
      }
      .table-footer {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }
      .row-count {
        font-size: 11px;
        color: var(--text-muted);
      }
      .footer-actions {
        display: inline-flex;
        align-items: center;
        gap: 5px;
      }
      .footer-btn {
        min-block-size: 24px;
        padding: 2px 8px;
        border-radius: 6px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
      }
      .footer-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .empty-note {
        margin: 0;
        font-size: 12px;
        color: var(--text-muted);
      }
    `,
  ],
})
export class TableCardComponent extends BaseCardTemplate<TableLayoutData> {
  readonly columns = computed<string[]>(() => {
    const raw = this.node().data?.columns;
    return Array.isArray(raw) ? raw : [];
  });

  readonly rows = computed<string[][]>(() => {
    const raw = this.node().data?.rows;
    return Array.isArray(raw) ? raw : [];
  });

  readonly monoColumnSet = computed<Set<number>>(() => {
    const raw = this.node().data?.monoColumns;
    return new Set(Array.isArray(raw) ? raw : []);
  });

  readonly sourceUrl = computed(() => (this.node().data?.sourceUrl ?? '').trim());

  isMonoColumn(index: number): boolean {
    return this.monoColumnSet().has(index);
  }

  toCsv(): string {
    const escape = (cell: string): string => {
      const str = String(cell ?? '');
      if (/[",\r\n]/.test(str)) {
        return `"${str.replace(/"/g, '""')}"`;
      }
      return str;
    };
    const header = this.columns().map(escape).join(',');
    const body = this.rows().map((r) => r.map(escape).join(','));
    return [header, ...body].join('\n');
  }

  toMarkdown(): string {
    const escape = (cell: string): string => String(cell ?? '').replace(/\|/g, '\\|').replace(/\r?\n/g, ' ');
    const cols = this.columns();
    const header = `| ${cols.map(escape).join(' | ')} |`;
    const divider = `| ${cols.map(() => '---').join(' | ')} |`;
    const body = this.rows().map((r) => `| ${r.map(escape).join(' | ')} |`);
    return [header, divider, ...body].join('\n');
  }

  copyCsv(): void {
    this.triggerCopy('CSV', this.toCsv());
  }

  copyMarkdown(): void {
    this.triggerCopy('Markdown', this.toMarkdown());
  }
}
