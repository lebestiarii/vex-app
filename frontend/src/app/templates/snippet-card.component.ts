import { ChangeDetectionStrategy, Component, computed, linkedSignal } from '@angular/core';
import { SnippetLayoutData, SnippetVar } from '../models/card-node.model';
import { BaseCardTemplate } from './base-card-template';

/**
 * SnippetCardComponent (`snippet`):
 * Optional compact inline {{var}} text inputs that live-interpolate into
 * a monospace code block + 1-click [Copy].
 */
@Component({
  selector: 'app-snippet-card',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="snippet-wrap">
      @if (vars().length > 0) {
        <div class="vars-grid">
          @for (v of vars(); track v.key) {
            <label class="var-field">
              <span class="var-label">{{ v.label }}</span>
              <input
                type="text"
                class="var-input font-mono"
                [value]="varValues()[v.key] ?? ''"
                [placeholder]="v.defaultValue || v.key"
                (input)="updateVar(v.key, $any($event.target).value)"
                [attr.aria-label]="v.label"
              />
            </label>
          }
        </div>
      }

      <div class="code-box">
        <div class="code-header">
          <span class="lang-badge font-mono">{{ language() || 'text' }}</span>
          <button
            type="button"
            class="copy-btn"
            (click)="triggerCopy(node().title, renderedBody())"
            [attr.aria-label]="'Copy snippet ' + node().title"
          >
            Copy
          </button>
        </div>
        <pre class="code-pre"><code class="font-mono">{{ renderedBody() }}</code></pre>
      </div>
    </div>
  `,
  styles: [
    `
      .snippet-wrap {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .vars-grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
        gap: 6px;
      }
      .var-field {
        display: flex;
        flex-direction: column;
        gap: 3px;
        min-width: 0;
      }
      .var-label {
        font-size: 10.5px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.03em;
        color: var(--text-muted);
      }
      .var-input {
        width: 100%;
        min-block-size: 28px;
        padding: 4px 8px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-input);
        color: var(--text-primary);
        font-size: 12px;
      }
      .var-input:focus {
        border-color: var(--accent);
        outline: none;
      }
      .code-box {
        border-radius: 8px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
        overflow: hidden;
      }
      .code-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        padding: 5px 8px;
        border-bottom: 1px solid var(--border-divider);
        background: var(--bg-subtle);
      }
      .lang-badge {
        font-size: 10.5px;
        font-weight: 600;
        text-transform: uppercase;
        color: var(--text-muted);
      }
      .copy-btn {
        min-block-size: 22px;
        padding: 2px 8px;
        border-radius: 6px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l0);
        color: var(--text-primary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
      }
      .copy-btn:hover {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .code-pre {
        margin: 0;
        padding: 9px 10px;
        overflow-x: auto;
        font-size: 12px;
        line-height: 1.5;
        color: var(--text-primary);
        white-space: pre-wrap;
        word-break: break-word;
      }
    `,
  ],
})
export class SnippetCardComponent extends BaseCardTemplate<SnippetLayoutData> {
  readonly language = computed(() => (this.node().data?.language ?? '').trim());

  readonly vars = computed<SnippetVar[]>(() => {
    const raw = this.node().data?.vars;
    return Array.isArray(raw) ? raw : [];
  });

  readonly varValues = linkedSignal<Record<string, string>>(() => {
    const initial: Record<string, string> = {};
    for (const v of this.vars()) {
      initial[v.key] = v.defaultValue ?? '';
    }
    return initial;
  });

  readonly renderedBody = computed(() => {
    const template = this.node().data?.body ?? '';
    const values = this.varValues();
    return template.replace(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g, (full, key: string) => {
      const val = values[key];
      return val !== undefined && val !== '' ? val : full;
    });
  });

  updateVar(key: string, value: string): void {
    this.varValues.update((prev) => ({ ...prev, [key]: value }));
  }
}
