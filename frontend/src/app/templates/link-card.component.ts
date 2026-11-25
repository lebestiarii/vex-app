import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { LinkLayoutData } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

/**
 * LinkCardComponent (`link`):
 * Primarily rendered as a Single-Button clickable card with top-right ↗ (no chevron)
 * directly by RolodexCardShellComponent. This fallback template handles direct instantiation.
 */
@Component({
  selector: 'app-link-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (url(); as target) {
      <button
        type="button"
        class="link-fallback-btn"
        (click)="openExternal(target)"
        [attr.aria-label]="'Open ' + node().title"
      >
        <span class="link-url font-mono">{{ target }}</span>
        <span class="link-arrow" aria-hidden="true">↗</span>
      </button>
    }
  `,
  styles: [
    `
      .link-fallback-btn {
        width: 100%;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        padding: 8px 10px;
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
        color: var(--text-primary);
        cursor: pointer;
        text-align: start;
      }
      .link-fallback-btn:hover {
        border-color: var(--chip-blue-border);
        background: var(--chip-blue-bg);
      }
      .link-url {
        font-size: 12px;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        min-width: 0;
      }
      .link-arrow {
        color: var(--accent);
        font-weight: 700;
        flex-shrink: 0;
      }
    `,
  ],
})
export class LinkCardComponent extends BaseCardTemplate<LinkLayoutData> {
  readonly url = computed(() => (this.node().data?.url ?? '').trim());
}
