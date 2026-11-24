import { ChangeDetectionStrategy, Component, effect, ElementRef, inject, viewChild } from '@angular/core';
import { RolodexStateService } from '../services/rolodex-state.service';

@Component({
  selector: 'app-toast-region',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <p class="visually-hidden">Status notifications</p>
    <div class="visually-hidden" aria-live="polite" aria-atomic="true">
      {{ state.liveAnnouncement() }}
    </div>

    <div
      #popoverEl
      class="toast-stack"
      popover="manual"
      role="region"
      aria-label="Notifications"
    >
      @for (toast of state.toasts(); track toast.id) {
        <div class="toast-item" [attr.data-tone]="toast.tone">
          <span class="toast-text">{{ toast.text }}</span>
          <button
            type="button"
            class="toast-close"
            (click)="state.dismissToast(toast.id)"
            aria-label="Close notification"
          >
            Close
          </button>
        </div>
      }
    </div>
  `,
  styles: [
    `
      .toast-stack {
        position: fixed;
        inset: auto 16px 16px auto;
        margin: 0;
        padding: 0;
        border: 0;
        background: transparent;
        display: none;
        flex-direction: column;
        gap: 8px;
        max-width: min(360px, calc(100vw - 32px));
        overflow: visible;
      }

      .toast-stack:where(:popover-open, .\\:popover-open) {
        display: flex;
      }

      .toast-item {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
        padding: 9px 12px;
        border-radius: 10px;
        border: 1px solid var(--border-l0);
        background: var(--surface-l0);
        color: var(--text-primary);
        box-shadow: var(--shadow-shell);
        font-size: 12.5px;
        line-height: 1.35;
        transition: opacity 0.18s ease, transform 0.18s ease;
        transition-behavior: allow-discrete;
      }

      .toast-item[data-tone='warn'] {
        background: var(--chip-amber-bg);
        border-color: var(--chip-amber-border);
        color: var(--chip-amber-text);
      }

      .toast-text {
        font-weight: 500;
      }

      .toast-close {
        min-block-size: 24px;
        padding: 2px 8px;
        border-radius: 6px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
        flex-shrink: 0;
      }

      .toast-close:hover {
        background: var(--surface-hover);
        color: var(--text-primary);
      }

      @media (prefers-reduced-motion: reduce) {
        .toast-item {
          transition: none;
        }
      }
    `,
  ],
})
export class ToastRegionComponent {
  readonly state = inject(RolodexStateService);
  private readonly popoverEl = viewChild<ElementRef<HTMLElement>>('popoverEl');

  constructor() {
    effect(() => {
      const hasToasts = this.state.toasts().length > 0;
      const el = this.popoverEl()?.nativeElement as
        | (HTMLElement & { showPopover?: () => void; hidePopover?: () => void })
        | undefined;
      if (!el) return;

      if (typeof el.showPopover === 'function' && typeof el.hidePopover === 'function') {
        try {
          if (hasToasts && !el.matches(':popover-open')) {
            el.showPopover();
          } else if (!hasToasts && el.matches(':popover-open')) {
            el.hidePopover();
          }
        } catch {
          // Ignore transient top-layer state transitions
        }
      } else {
        el.classList.toggle(':popover-open', hasToasts);
      }
    });
  }
}
