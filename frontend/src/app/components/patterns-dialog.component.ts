import {
  ChangeDetectionStrategy,
  Component,
  effect,
  ElementRef,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CardNode, PatternRule } from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';

@Component({
  selector: 'app-patterns-dialog',
  standalone: true,
  imports: [FormsModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <dialog
      #dialogEl
      class="vex-dialog"
      aria-labelledby="patterns-dialog-title"
      (cancel)="onNativeClose()"
    >
      <div class="dialog-inner">
        <header class="dialog-header">
          <div>
            <h2 id="patterns-dialog-title" class="dialog-title">Configurable Pattern Matching</h2>
            <p class="dialog-sub">
              Patterns are loadable modules & JSON rules (~/.vex/patterns/). Edit regexes, checksums,
              data lookups, and cross-card nesting links live.
            </p>
          </div>
          <button type="button" class="close-btn" (click)="closeDialog()">Close</button>
        </header>

        <div class="dialog-body">
          <section class="section-block">
            <h3 class="section-heading">Active Pattern Rules ({{ rules().length }})</h3>
            <div class="rule-list">
              @for (rule of rules(); track rule.id) {
                <div class="rule-row">
                  <div class="rule-meta">
                    <div class="rule-title">
                      <span aria-hidden="true">{{ rule.icon || '🔍' }}</span>
                      <strong>{{ rule.name }}</strong>
                      <code class="rule-id font-mono">{{ rule.id }}</code>
                      @if (rule.checksum) {
                        <span class="badge-green">{{ rule.checksum }}</span>
                      }
                    </div>
                    <code class="rule-regex font-mono">{{ rule.regex }}</code>
                    @if (rule.linkRules?.length) {
                      <div class="link-summary">
                        Links:
                        @for (lr of rule.linkRules; track $index) {
                          <span class="link-pill font-mono">{{ lr.targetEntityKey || lr.targetTag }}</span>
                        }
                      </div>
                    }
                  </div>
                  <div class="rule-actions">
                    <button type="button" class="mini-btn" (click)="loadIntoEditor(rule)">Edit</button>
                    <button type="button" class="mini-btn danger" (click)="removeRule(rule)">Delete</button>
                  </div>
                </div>
              }
              @if (rules().length === 0) {
                <p class="empty-note">
                  No pattern rules loaded. Install the "Reference Number Decoders" starter module or add a custom rule below.
                </p>
              }
            </div>
          </section>

          <section class="section-block editor-card">
            <div class="section-head-row">
              <h3 class="section-heading">Add / Edit Pattern Rule JSON</h3>
              <div class="editor-btns">
                <button type="button" class="mini-btn" (click)="resetDraft()">New Template</button>
                <button type="button" class="primary-btn" (click)="saveDraft()">Save Rule</button>
              </div>
            </div>

            <label for="pattern-json-editor" class="visually-hidden">PatternRule JSON</label>
            <textarea
              id="pattern-json-editor"
              class="json-editor font-mono"
              rows="9"
              [(ngModel)]="draftJson"
            ></textarea>

            <div class="tester-row">
              <label for="pattern-test-input" class="tester-lbl">Live Test</label>
              <input
                id="pattern-test-input"
                type="text"
                class="tester-input font-mono"
                placeholder="Enter test input to evaluate regex match"
                [(ngModel)]="testSample"
              />
              <button type="button" class="mini-btn" (click)="runTest()">Test</button>
            </div>

            @if (testError()) {
              <p class="err-msg">{{ testError() }}</p>
            }
            @if (testPreview(); as preview) {
              <div class="preview-box">
                <strong>Matched:</strong> {{ preview.title }}
                @if (preview.children?.length) {
                  <span class="preview-sub">
                    · Nests {{ preview.children?.length }} linked card(s)
                  </span>
                }
              </div>
            }
          </section>
        </div>
      </div>
    </dialog>
  `,
  styles: [
    `
      .vex-dialog {
        width: min(700px, calc(100vw - 28px));
        max-height: calc(100vh - 36px);
        padding: 0;
        border: 1px solid var(--border-l0);
        border-radius: 14px;
        background: var(--bg-shell);
        color: var(--text-primary);
        box-shadow: var(--shadow-shell);
      }
      .vex-dialog::backdrop {
        background: rgba(9, 9, 11, 0.45);
        backdrop-filter: blur(2px);
      }
      .dialog-inner {
        display: flex;
        flex-direction: column;
        max-height: calc(100vh - 40px);
      }
      .dialog-header {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 12px;
        padding: 16px 18px 12px;
        border-bottom: 1px solid var(--border-divider);
      }
      .dialog-title {
        margin: 0;
        font-size: 16px;
        font-weight: 700;
      }
      .dialog-sub {
        margin: 4px 0 0;
        font-size: 12.5px;
        color: var(--text-muted);
      }
      .close-btn {
        min-block-size: 28px;
        padding: 4px 11px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l0);
        font-size: 12px;
        font-weight: 600;
        cursor: pointer;
      }
      .close-btn:hover {
        background: var(--surface-hover);
      }
      .dialog-body {
        padding: 14px 18px 18px;
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        gap: 14px;
      }
      .section-block {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .section-head-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }
      .section-heading {
        margin: 0;
        font-size: 12px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        color: var(--text-secondary);
      }
      .rule-list {
        display: flex;
        flex-direction: column;
        gap: 7px;
      }
      .rule-row {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 10px;
        padding: 9px 11px;
        border-radius: 10px;
        border: 1px solid var(--border-l0);
        background: var(--surface-l0);
      }
      .rule-meta {
        display: flex;
        flex-direction: column;
        gap: 4px;
        min-width: 0;
        flex: 1;
      }
      .rule-title {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
        font-size: 13px;
      }
      .rule-id {
        font-size: 11px;
        color: var(--text-muted);
      }
      .badge-green {
        font-size: 10.5px;
        font-weight: 700;
        padding: 1px 7px;
        border-radius: 999px;
        background: var(--chip-green-bg);
        border: 1px solid var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .rule-regex {
        font-size: 11.5px;
        color: var(--text-secondary);
        overflow-wrap: anywhere;
      }
      .link-summary {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 5px;
        font-size: 11px;
        color: var(--text-muted);
      }
      .link-pill {
        padding: 1px 6px;
        border-radius: 5px;
        background: var(--chip-blue-bg);
        border: 1px solid var(--chip-blue-border);
        color: var(--chip-blue-text);
        font-size: 10.5px;
      }
      .rule-actions,
      .editor-btns {
        display: flex;
        gap: 6px;
        flex-shrink: 0;
      }
      .editor-card {
        padding: 12px;
        border-radius: 10px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l1);
      }
      .json-editor {
        width: 100%;
        padding: 9px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--bg-input);
        font-size: 11.5px;
        line-height: 1.45;
        resize: vertical;
      }
      .tester-row {
        display: flex;
        align-items: center;
        gap: 8px;
      }
      .tester-lbl {
        font-size: 12px;
        font-weight: 600;
        color: var(--text-secondary);
      }
      .tester-input {
        flex: 1;
        padding: 6px 9px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--bg-input);
        font-size: 12px;
      }
      .mini-btn {
        min-block-size: 26px;
        padding: 3px 9px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        font-size: 11.5px;
        font-weight: 600;
        cursor: pointer;
      }
      .mini-btn:hover {
        background: var(--surface-hover);
      }
      .mini-btn.danger {
        background: var(--chip-red-bg);
        border-color: var(--chip-red-border);
        color: var(--chip-red-text);
      }
      .primary-btn {
        min-block-size: 28px;
        padding: 4px 12px;
        border-radius: 8px;
        border: 1px solid var(--chip-blue-border);
        background: var(--chip-blue-bg);
        color: var(--chip-blue-text);
        font-size: 12px;
        font-weight: 700;
        cursor: pointer;
      }
      .err-msg {
        margin: 0;
        font-size: 12px;
        color: var(--chip-red-text);
      }
      .preview-box {
        padding: 8px 10px;
        border-radius: 8px;
        background: var(--chip-green-bg);
        border: 1px solid var(--chip-green-border);
        color: var(--chip-green-text);
        font-size: 12px;
      }
      .preview-sub {
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
export class PatternsDialogComponent {
  readonly open = input<boolean>(false);
  readonly closed = output<void>();
  readonly patternsChanged = output<void>();

  readonly bridge = inject(VexBridgeService);
  readonly state = inject(RolodexStateService);

  readonly rules = signal<PatternRule[]>([]);
  readonly testPreview = signal<CardNode | null>(null);
  readonly testError = signal<string>('');

  draftJson = '';
  testSample = '';

  private readonly dialogEl = viewChild<ElementRef<HTMLDialogElement>>('dialogEl');

  constructor() {
    this.resetDraft();
    effect(() => {
      const isOpen = this.open();
      const el = this.dialogEl()?.nativeElement;
      if (!el) return;
      if (isOpen && !el.open) {
        el.showModal();
        void this.refresh();
      } else if (!isOpen && el.open) {
        el.close();
      }
    });
  }

  async refresh(): Promise<void> {
    this.rules.set(await this.bridge.listPatternRules());
  }

  loadIntoEditor(rule: PatternRule): void {
    this.draftJson = JSON.stringify(rule, null, 2);
    this.testError.set('');
    this.testPreview.set(null);
  }

  resetDraft(): void {
    this.draftJson = JSON.stringify(
      {
        id: 'custom-ref',
        name: 'Custom Reference Decoder',
        priority: 60,
        normalize: ['trim', 'uppercase'],
        regex: '^(?P<prefix>REF)-(?P<branch>[A-Z]{3})-(?P<num>\\d{6})$',
        titleTemplate: 'Reference · {{prefix}}-{{branch}}-{{num}}',
        template: 'pattern-breakdown',
        icon: '🧩',
        defaultExpanded: true,
        chips: [
          { label: 'REF', tone: 'blue' },
          { label: 'Valid ✓', tone: 'green' },
          { label: 'Origin: {{branch}}', tone: 'neutral' },
        ],
        segments: [
          { value: '{{prefix}}', role: 'TYPE', meaning: 'Standard Ref', tone: 'blue' },
          { value: '{{branch}}', role: 'BRANCH', meaning: 'Origin {{branch}}', tone: 'amber' },
          { value: '{{num}}', role: 'SEQUENCE', meaning: 'Record #', tone: 'green' },
        ],
        explanationTemplate:
          'Structured reference linking record {{num}} directly to branch {{branch}}.',
        linkRules: [
          { targetEntityKey: 'loc:{{branch}}', autoExpand: true, nestChildren: true, maxDepth: 2 },
        ],
      },
      null,
      2
    );
  }

  async saveDraft(): Promise<void> {
    try {
      const parsed = JSON.parse(this.draftJson) as PatternRule;
      const updated = await this.bridge.upsertPatternRule(parsed);
      this.rules.set(updated);
      this.testError.set('');
      this.patternsChanged.emit();
      this.state.showToast(`Saved pattern rule "${parsed.name || parsed.id}"`, 'info');
    } catch (err) {
      this.testError.set(err instanceof Error ? err.message : 'Invalid PatternRule JSON');
    }
  }

  async removeRule(rule: PatternRule): Promise<void> {
    const updated = await this.bridge.deletePatternRule(rule.id);
    this.rules.set(updated);
    this.patternsChanged.emit();
    this.state.showToast(`Deleted pattern rule "${rule.id}"`, 'info');
  }

  async runTest(): Promise<void> {
    this.testError.set('');
    this.testPreview.set(null);
    try {
      const parsed = JSON.parse(this.draftJson) as PatternRule;
      const preview = await this.bridge.testPatternRule(parsed, this.testSample);
      if (!preview) {
        this.testError.set(`No match for "${this.testSample}"`);
      } else {
        this.testPreview.set(preview);
      }
    } catch (err) {
      this.testError.set(err instanceof Error ? err.message : 'Test failed');
    }
  }

  closeDialog(): void {
    this.dialogEl()?.nativeElement.close();
    this.closed.emit();
  }

  onNativeClose(): void {
    this.closed.emit();
  }
}
