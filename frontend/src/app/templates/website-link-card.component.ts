import { ChangeDetectionStrategy, Component, computed } from '@angular/core';
import { BookmarkRow, EnvLinkButton, WebsiteLinkData } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

interface NormalizedBookmark {
  name: string;
  subtitle?: string;
  buttons: EnvLinkButton[];
  primaryCopyUrl: string;
}

@Component({
  selector: 'app-website-link-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="bm-list" role="list">
      @for (bm of normalizedBookmarks(); track bm.name + $index) {
        <div class="bm-row" role="listitem">
          <div class="bm-info">
            <div class="bm-name">{{ bm.name }}</div>
            @if (bm.subtitle) {
              <div class="bm-sub">{{ bm.subtitle }}</div>
            }
          </div>
          <div class="bm-actions">
            @for (btn of bm.buttons; track btn.label + btn.url) {
              <button
                type="button"
                class="env-launch"
                [attr.data-tone]="btn.tone || 'neutral'"
                (click)="openExternal(btn.url)"
                [attr.aria-label]="'Open ' + bm.name + ' (' + btn.label + ')'"
              >
                @if (btn.dot) {
                  <span class="dot" [attr.data-dot]="btn.dot" aria-hidden="true"></span>
                }
                {{ btn.label }}
              </button>
            }
            @if (bm.primaryCopyUrl) {
              <button
                type="button"
                class="copy-url-btn"
                (click)="triggerCopy(bm.name + ' URL', bm.primaryCopyUrl)"
                [attr.aria-label]="'Copy URL for ' + bm.name"
              >
                Copy
              </button>
            }
          </div>
        </div>
      }
      @if (normalizedBookmarks().length === 0) {
        <p class="empty-note">No bookmark links configured.</p>
      }
    </div>
  `,
  styles: [
    `
      .bm-list {
        display: flex;
        flex-direction: column;
        gap: 6px;
      }
      .bm-row {
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
      .bm-info {
        min-width: 140px;
        flex: 1 1 160px;
      }
      .bm-name {
        font-size: 12.5px;
        font-weight: 600;
        color: var(--text-primary);
      }
      .bm-sub {
        font-size: 11px;
        color: var(--text-muted);
        margin-block-start: 1px;
      }
      .bm-actions {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 5px;
      }
      .env-launch {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        min-block-size: 24px;
        padding: 2px 8px;
        border-radius: 999px;
        font-size: 11px;
        font-weight: 700;
        border: 1px solid var(--chip-neutral-border);
        background: var(--chip-neutral-bg);
        color: var(--chip-neutral-text);
        cursor: pointer;
        transition: filter 0.14s ease;
      }
      .env-launch:hover {
        filter: brightness(0.95);
      }
      .env-launch[data-tone='green'] {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .env-launch[data-tone='amber'] {
        background: var(--chip-amber-bg);
        border-color: var(--chip-amber-border);
        color: var(--chip-amber-text);
      }
      .env-launch[data-tone='red'] {
        background: var(--chip-red-bg);
        border-color: var(--chip-red-border);
        color: var(--chip-red-text);
      }
      .env-launch[data-tone='blue'] {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
      }
      .dot[data-dot='green'] {
        background: var(--dot-green);
      }
      .dot[data-dot='amber'] {
        background: var(--dot-amber);
      }
      .dot[data-dot='red'] {
        background: var(--dot-red);
      }
      .copy-url-btn {
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
      .copy-url-btn:hover {
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
export class WebsiteLinkCardComponent extends BaseCardTemplate<WebsiteLinkData> {
  readonly normalizedBookmarks = computed<NormalizedBookmark[]>(() => {
    const raw = this.node().data?.bookmarks;
    if (!Array.isArray(raw)) return [];
    return raw.map((bm) => this.normalizeRow(bm));
  });

  private normalizeRow(bm: BookmarkRow): NormalizedBookmark {
    const buttons: EnvLinkButton[] = [];
    if (Array.isArray(bm.links) && bm.links.length > 0) {
      for (const l of bm.links) {
        if (!l?.url) continue;
        const tone = l.tone ?? 'neutral';
        buttons.push({
          label: l.label || 'Open',
          url: l.url,
          tone,
          dot: l.dot ?? (tone === 'green' || tone === 'amber' || tone === 'red' ? tone : undefined),
        });
      }
    } else {
      if (bm.qaUrl) {
        buttons.push({ label: bm.qaLabel || 'QA', url: bm.qaUrl, tone: 'amber', dot: 'amber' });
      }
      if (bm.prodUrl) {
        buttons.push({ label: bm.prodLabel || 'PROD', url: bm.prodUrl, tone: 'red', dot: 'red' });
      }
    }
    const primaryCopyUrl = bm.prodUrl || buttons[buttons.length - 1]?.url || bm.qaUrl || '';
    return {
      name: bm.name,
      subtitle: bm.subtitle,
      buttons,
      primaryCopyUrl,
    };
  }
}
