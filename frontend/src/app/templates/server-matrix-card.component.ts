import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { ServerMatrixData } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

interface NormalizedMatrixRow {
  badge: string;
  tone: 'green' | 'amber' | 'red' | 'neutral';
  host: string;
  port?: string;
  subtitle?: string;
  copyTarget: string;
  sshCommand: string;
  url: string;
}

@Component({
  selector: 'app-server-matrix-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="matrix-list" role="list">
      @for (row of normalizedRows(); track row.host + row.badge + $index) {
        <div class="matrix-row" role="listitem">
          <span class="env-badge" [attr.data-tone]="row.tone">
            @if (row.tone !== 'neutral') {
              <span class="stoplight-dot" [attr.data-dot]="row.tone" aria-hidden="true"></span>
            }
            {{ row.badge }}
          </span>

          <div class="host-col">
            <code class="host-name font-mono">{{ row.host }}@if (row.port) {:{{ row.port }}}</code>
            @if (row.subtitle) {
              <span class="host-role">{{ row.subtitle }}</span>
            }
          </div>

          <div class="row-actions">
            <button
              type="button"
              class="action-btn"
              (click)="triggerCopy(row.badge + ' Host', row.copyTarget)"
              [attr.aria-label]="'Copy hostname ' + row.copyTarget"
            >
              Copy
            </button>
            <button
              type="button"
              class="action-btn"
              (click)="triggerCopy(row.badge + ' SSH', row.sshCommand)"
              [attr.aria-label]="'Copy SSH command for ' + row.host"
            >
              SSH
            </button>
            <button
              type="button"
              class="action-btn icon-btn"
              (click)="openExternal(row.url)"
              [attr.aria-label]="'Open ' + row.host"
            >
              ↗
            </button>
          </div>
        </div>
      }
      @if (normalizedRows().length === 0) {
        <p class="empty-note">No server rows configured. Update via MCP or module JSON.</p>
      }
    </div>
  `,
  styles: [
    `
      .matrix-list {
        display: flex;
        flex-direction: column;
        gap: 6px;
      }
      .matrix-row {
        display: grid;
        grid-template-columns: auto minmax(0, 1fr) auto;
        align-items: center;
        gap: 8px;
        padding: 7px 9px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
      }
      .env-badge {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        padding: 2px 8px;
        border-radius: 999px;
        font-size: 11px;
        font-weight: 700;
        letter-spacing: 0.02em;
        border: 1px solid var(--chip-neutral-border);
        background: var(--chip-neutral-bg);
        color: var(--chip-neutral-text);
        white-space: nowrap;
      }
      .env-badge[data-tone='green'] {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .env-badge[data-tone='amber'] {
        background: var(--chip-amber-bg);
        border-color: var(--chip-amber-border);
        color: var(--chip-amber-text);
      }
      .env-badge[data-tone='red'] {
        background: var(--chip-red-bg);
        border-color: var(--chip-red-border);
        color: var(--chip-red-text);
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
      .host-col {
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
      }
      .host-name {
        font-size: 12px;
        font-weight: 600;
        color: var(--text-primary);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      .host-role {
        font-size: 11px;
        color: var(--text-muted);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      .row-actions {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        flex-shrink: 0;
      }
      .action-btn {
        min-block-size: 24px;
        padding: 2px 7px;
        border-radius: 6px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
        transition: background-color 0.14s ease, border-color 0.14s ease, color 0.14s ease;
      }
      .action-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .action-btn.icon-btn {
        min-inline-size: 24px;
        padding: 2px 6px;
      }
      .empty-note {
        margin: 0;
        font-size: 12px;
        color: var(--text-muted);
      }
    `,
  ],
})
export class ServerMatrixCardComponent extends BaseCardTemplate<ServerMatrixData> {
  readonly normalizedRows = computed<NormalizedMatrixRow[]>(() => {
    const d = this.node().data;
    const source = Array.isArray(d?.servers)
      ? d.servers
      : Array.isArray(d?.rows)
        ? d.rows
        : [];
    return source
      .map((r) => {
        const host = (r.host ?? r.value ?? '').trim();
        const badge = (r.env ?? r.badge ?? 'ENV').trim();
        const portPart = r.port ? ` -p ${r.port}` : '';
        return {
          badge,
          tone: r.tone ?? 'neutral',
          host,
          port: r.port,
          subtitle: r.role ?? r.subtitle,
          copyTarget: (r.copyValue ?? host).trim(),
          sshCommand: (r.sshCommand ?? `ssh ${host}${portPart}`).trim(),
          url: (r.url ?? (host ? `https://${host}` : '')).trim(),
        };
      })
      .filter((r) => r.host !== '');
  });
}
