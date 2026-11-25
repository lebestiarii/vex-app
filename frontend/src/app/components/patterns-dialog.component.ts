import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ALL_CARD_LAYOUTS, CardNode, PatternRule } from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';
import { RolodexCardShellComponent } from './rolodex-card-shell.component';

@Component({
  selector: 'app-patterns-dialog',
  standalone: true,
  imports: [FormsModule, RolodexCardShellComponent],
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
          <div class="header-copy">
            <h2 #dialogTitle id="patterns-dialog-title" class="dialog-title" tabindex="-1">
              Configurable Pattern Matching
            </h2>
            <p class="dialog-sub">
              Test tracking numbers and IDs live, toggle active decoders, or edit rule JSON (~/.vex/patterns/).
            </p>
          </div>
          <button type="button" class="close-btn" (click)="closeDialog()">Close</button>
        </header>

        <div class="dialog-body">
          <!-- Live Test Bar at top of modal -->
          <section class="section-block live-test-card" aria-label="Live Pattern Tester">
            <label for="pattern-test-input" class="section-heading">Live Test</label>
            <div class="tester-row">
              <input
                id="pattern-test-input"
                type="text"
                class="tester-input font-mono"
                placeholder="Test an ID, e.g. MSCU1234565 or 176-12345675"
                [ngModel]="testSample()"
                (ngModelChange)="testSample.set($event)"
                (keydown.enter)="runTest()"
              />
              <button type="button" class="primary-btn" (click)="runTest()">Test</button>
            </div>

            @if (testError()) {
              <p class="err-msg">{{ testError() }}</p>
            }

            @if (testPreview(); as preview) {
              <div class="preview-wrap">
                <div class="preview-banner">
                  <strong>Matched:</strong>
                  <span>{{ matchedRuleLabel() }}</span>
                  @if (preview.children?.length) {
                    <span class="preview-sub">
                      · Nests {{ preview.children?.length }} linked card(s)
                    </span>
                  }
                </div>
                <app-rolodex-card-shell [node]="preview" [depth]="0" />
              </div>
            }
          </section>

          <!-- Pattern Rules List -->
          <section class="section-block" aria-label="Pattern Rules">
            <div class="section-head-row">
              <h3 class="section-heading">Pattern Rules ({{ rules().length }})</h3>
              <button type="button" class="mini-btn accent" (click)="startNewRule()">
                + New Rule
              </button>
            </div>

            <div class="rule-list">
              @for (rule of rules(); track rule.id) {
                <article class="rule-card" [class.disabled]="!isRuleEnabled(rule)">
                  <div class="rule-row-1">
                    <div class="rule-title">
                      <span aria-hidden="true">{{ rule.icon || '🔍' }}</span>
                      <strong>{{ rule.name }}</strong>
                      @if (rule.checksum) {
                        <span class="badge-green">{{ rule.checksum }}</span>
                      }
                      @if (rule.liveHttp) {
                        <span class="badge-blue">Live GET</span>
                      }
                      @if (rule.liveMcp) {
                        <span class="badge-amber">Live MCP</span>
                      }
                    </div>

                    <button
                      type="button"
                      role="switch"
                      class="switch-btn"
                      [class.on]="isRuleEnabled(rule)"
                      [attr.aria-checked]="isRuleEnabled(rule)"
                      [attr.aria-label]="(isRuleEnabled(rule) ? 'Disable ' : 'Enable ') + rule.name"
                      (click)="toggleRuleEnabled(rule)"
                    >
                      <span class="switch-track" aria-hidden="true">
                        <span class="switch-thumb"></span>
                      </span>
                      <span class="switch-text">{{ isRuleEnabled(rule) ? 'On' : 'Off' }}</span>
                    </button>
                  </div>

                  <p class="rule-example">{{ ruleExample(rule) }}</p>

                  <div class="rule-row-3">
                    <div class="link-pills">
                      @for (target of linkedEntityTargets(rule); track target + $index) {
                        <span class="link-pill font-mono">{{ target }}</span>
                      }
                    </div>
                    <div class="rule-actions">
                      <button type="button" class="mini-btn" (click)="loadIntoEditor(rule)">
                        Edit JSON
                      </button>
                      <button
                        type="button"
                        class="mini-btn danger"
                        (click)="removeRule(rule)"
                        [attr.aria-label]="'Delete rule ' + rule.name"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                </article>
              }
              @if (rules().length === 0) {
                <p class="empty-note">
                  No pattern rules loaded. Install the "Reference Number Decoders" starter module or add a custom rule below.
                </p>
              }
            </div>
          </section>

          <!-- Collapsible Edit JSON Disclosure -->
          <section class="section-block editor-disclosure" aria-label="Pattern JSON Editor">
            <button
              type="button"
              class="disclosure-trigger"
              [attr.aria-expanded]="editorOpen()"
              (click)="toggleEditor()"
            >
              <span class="disclosure-chevron" [class.open]="editorOpen()" aria-hidden="true">▸</span>
              <span class="section-heading">Edit JSON</span>
              @if (editingRuleId()) {
                <code class="editing-id font-mono">{{ editingRuleId() }}</code>
              }
            </button>

            @if (editorOpen()) {
              <div class="editor-panel">
                @if (draftRegexPreview(); as rawRe) {
                  <div class="regex-preview">
                    <span class="regex-lbl">Regex</span>
                    <code class="regex-code font-mono">{{ rawRe }}</code>
                  </div>
                }

                <label for="pattern-json-editor" class="visually-hidden">PatternRule JSON</label>
                <textarea
                  id="pattern-json-editor"
                  class="json-editor font-mono"
                  rows="9"
                  spellcheck="false"
                  [ngModel]="draftJson()"
                  (ngModelChange)="onDraftChange($event)"
                ></textarea>

                @if (validationErrors().length > 0) {
                  <ul class="validation-errors" role="alert">
                    @for (err of validationErrors(); track err) {
                      <li>{{ err }}</li>
                    }
                  </ul>
                }

                @if (saveError()) {
                  <p class="err-msg" role="alert">{{ saveError() }}</p>
                }

                <div class="editor-footer">
                  <button type="button" class="mini-btn" (click)="resetDraft()">New Template</button>
                  <button
                    type="button"
                    class="primary-btn"
                    [disabled]="validationErrors().length > 0"
                    (click)="saveDraft()"
                  >
                    Save Rule
                  </button>
                </div>
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
        width: min(680px, calc(100vw - 24px));
        max-height: calc(100vh - 32px);
        padding: 0;
        border: 1px solid var(--border-l0);
        border-radius: 14px;
        background: var(--bg-shell);
        color: var(--text-primary);
        box-shadow: var(--shadow-shell);
      }
      .vex-dialog::backdrop {
        background: rgba(9, 9, 11, 0.48);
        backdrop-filter: blur(2px);
      }
      .dialog-inner {
        display: flex;
        flex-direction: column;
        max-height: calc(100vh - 36px);
      }
      .dialog-header {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 12px;
        padding: 15px 16px 12px;
        border-bottom: 1px solid var(--border-divider);
      }
      .header-copy {
        min-width: 0;
        flex: 1;
      }
      .dialog-title {
        margin: 0;
        font-size: 16px;
        font-weight: 700;
      }
      .dialog-title:focus {
        outline: none;
      }
      .dialog-sub {
        margin: 4px 0 0;
        font-size: 12px;
        color: var(--text-muted);
        line-height: 1.45;
      }
      .close-btn {
        min-block-size: 28px;
        padding: 4px 11px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l0);
        color: var(--text-secondary);
        font-size: 12px;
        font-weight: 600;
        cursor: pointer;
        flex-shrink: 0;
      }
      .close-btn:hover {
        background: var(--surface-hover);
      }
      .dialog-body {
        padding: 14px 16px 16px;
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
        font-size: 11.5px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        color: var(--text-secondary);
      }
      .live-test-card {
        padding: 12px;
        border-radius: 10px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l1);
      }
      .tester-row {
        display: flex;
        align-items: center;
        gap: 8px;
      }
      .tester-input {
        flex: 1;
        min-width: 0;
        min-block-size: 34px;
        padding: 6px 10px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--bg-input);
        color: var(--text-primary);
        font-size: 12.5px;
      }
      .tester-input:focus {
        border-color: var(--accent);
        outline: none;
      }
      .preview-wrap {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .preview-banner {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 5px;
        padding: 6px 10px;
        border-radius: 8px;
        background: var(--chip-green-bg);
        border: 1px solid var(--chip-green-border);
        color: var(--chip-green-text);
        font-size: 12px;
      }
      .preview-sub {
        opacity: 0.85;
      }
      .rule-list {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .rule-card {
        display: flex;
        flex-direction: column;
        gap: 6px;
        padding: 10px 12px;
        border-radius: 10px;
        border: 1px solid var(--border-l0);
        background: var(--surface-l0);
        transition: opacity 0.14s ease;
      }
      .rule-card.disabled {
        opacity: 0.65;
      }
      .rule-row-1 {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }
      .rule-title {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
        font-size: 13px;
        min-width: 0;
      }
      .badge-green,
      .badge-blue,
      .badge-amber {
        font-size: 10.5px;
        font-weight: 700;
        padding: 1px 7px;
        border-radius: 999px;
      }
      .badge-green {
        background: var(--chip-green-bg);
        border: 1px solid var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .badge-blue {
        background: var(--chip-blue-bg);
        border: 1px solid var(--chip-blue-border);
        color: var(--chip-blue-text);
      }
      .badge-amber {
        background: var(--chip-amber-bg);
        border: 1px solid var(--chip-amber-border);
        color: var(--chip-amber-text);
      }
      .switch-btn {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 2px 6px 2px 4px;
        border-radius: 999px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-muted);
        font-size: 11px;
        font-weight: 700;
        cursor: pointer;
        flex-shrink: 0;
      }
      .switch-btn.on {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .switch-track {
        width: 24px;
        height: 14px;
        border-radius: 999px;
        background: var(--border-nested);
        position: relative;
        transition: background-color 0.14s ease;
      }
      .switch-btn.on .switch-track {
        background: var(--dot-green);
      }
      .switch-thumb {
        position: absolute;
        top: 2px;
        left: 2px;
        width: 10px;
        height: 10px;
        border-radius: 50%;
        background: var(--surface-l0);
        transition: transform 0.14s ease;
      }
      .switch-btn.on .switch-thumb {
        transform: translateX(10px);
      }
      .rule-example {
        margin: 0;
        font-size: 12px;
        color: var(--text-muted);
        line-height: 1.4;
      }
      .rule-row-3 {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        padding-top: 6px;
        border-top: 1px solid var(--border-divider);
      }
      .link-pills {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 5px;
        min-width: 0;
        flex: 1;
      }
      .link-pill {
        padding: 1.5px 6px;
        border-radius: 5px;
        background: var(--chip-blue-bg);
        border: 1px solid var(--chip-blue-border);
        color: var(--chip-blue-text);
        font-size: 10.5px;
      }
      .rule-actions {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        flex-shrink: 0;
      }
      .editor-disclosure {
        border-radius: 10px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l1);
        overflow: hidden;
      }
      .disclosure-trigger {
        width: 100%;
        display: flex;
        align-items: center;
        gap: 7px;
        padding: 10px 12px;
        border: 0;
        background: transparent;
        color: var(--text-primary);
        text-align: start;
        cursor: pointer;
      }
      .disclosure-trigger:hover {
        background: var(--surface-hover);
      }
      .disclosure-chevron {
        display: inline-block;
        font-size: 12px;
        color: var(--text-muted);
        transition: transform 0.14s ease;
      }
      .disclosure-chevron.open {
        transform: rotate(90deg);
      }
      .editing-id {
        font-size: 11px;
        color: var(--text-muted);
      }
      .editor-panel {
        padding: 0 12px 12px;
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      .regex-preview {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 6px 8px;
        border-radius: 7px;
        border: 1px solid var(--border-divider);
        background: var(--surface-l0);
      }
      .regex-lbl {
        font-size: 10.5px;
        font-weight: 700;
        text-transform: uppercase;
        color: var(--text-muted);
        flex-shrink: 0;
      }
      .regex-code {
        font-size: 11.5px;
        color: var(--text-secondary);
        white-space: nowrap;
        overflow-x: auto;
        min-width: 0;
        flex: 1;
      }
      .json-editor {
        width: 100%;
        padding: 9px;
        border-radius: 8px;
        border: 1px solid var(--border-nested);
        background: var(--bg-input);
        color: var(--text-primary);
        font-size: 11.5px;
        line-height: 1.45;
        resize: vertical;
      }
      .validation-errors {
        margin: 0;
        padding: 8px 10px 8px 24px;
        border-radius: 8px;
        background: var(--chip-red-bg);
        border: 1px solid var(--chip-red-border);
        color: var(--chip-red-text);
        font-size: 11.5px;
        line-height: 1.45;
      }
      .editor-footer {
        display: flex;
        align-items: center;
        justify-content: flex-end;
        gap: 8px;
      }
      .mini-btn {
        min-block-size: 25px;
        padding: 2px 9px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
      }
      .mini-btn:hover {
        background: var(--surface-hover);
      }
      .mini-btn.accent {
        background: var(--chip-blue-bg);
        border-color: var(--chip-blue-border);
        color: var(--chip-blue-text);
        font-weight: 700;
      }
      .mini-btn.danger {
        background: var(--chip-red-bg);
        border-color: var(--chip-red-border);
        color: var(--chip-red-text);
      }
      .primary-btn {
        min-block-size: 32px;
        padding: 4px 13px;
        border-radius: 8px;
        border: 1px solid var(--chip-blue-border);
        background: var(--chip-blue-bg);
        color: var(--chip-blue-text);
        font-size: 12px;
        font-weight: 700;
        cursor: pointer;
        flex-shrink: 0;
      }
      .primary-btn:disabled {
        opacity: 0.5;
        cursor: not-allowed;
      }
      .err-msg {
        margin: 0;
        font-size: 12px;
        color: var(--chip-red-text);
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
  readonly testSample = signal<string>('');
  readonly testPreview = signal<CardNode | null>(null);
  readonly matchedRuleLabel = signal<string>('');
  readonly testError = signal<string>('');
  readonly saveError = signal<string>('');
  readonly draftJson = signal<string>('');
  readonly editorOpen = signal<boolean>(false);

  private readonly dialogEl = viewChild<ElementRef<HTMLDialogElement>>('dialogEl');
  private readonly dialogTitle = viewChild<ElementRef<HTMLElement>>('dialogTitle');

  readonly parsedDraft = computed<{ rule: PatternRule | null; errors: string[] }>(() => {
    const raw = this.draftJson().trim();
    if (!raw) {
      return { rule: null, errors: ['JSON cannot be empty.'] };
    }
    let parsed: unknown;
    try {
      parsed = JSON.parse(raw);
    } catch (err) {
      return {
        rule: null,
        errors: [err instanceof Error ? `Invalid JSON: ${err.message}` : 'Invalid JSON syntax.'],
      };
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { rule: null, errors: ['PatternRule must be a JSON object.'] };
    }
    const r = parsed as Partial<PatternRule>;
    const errors: string[] = [];
    if (!r.id?.trim()) errors.push('Missing required field "id".');
    if (!r.name?.trim()) errors.push('Missing required field "name".');
    if (!r.titleTemplate?.trim()) errors.push('Missing required field "titleTemplate".');
    if (!r.regex?.trim()) {
      errors.push('Missing required field "regex".');
    } else {
      try {
        new RegExp(r.regex.replace(/\(\?P<([^>]+)>/g, '(?<$1>'));
      } catch (err) {
        errors.push(`Invalid regex: ${err instanceof Error ? err.message : 'syntax error'}`);
      }
    }
    if (r.layout && !ALL_CARD_LAYOUTS.includes(r.layout)) {
      errors.push(
        `Invalid layout "${r.layout}". Must be one of: ${ALL_CARD_LAYOUTS.join(', ')}.`
      );
    }
    if (r.liveHttp) {
      const lh = r.liveHttp;
      const urlT = lh.urlTemplate?.trim() ?? '';
      if (!urlT) {
        errors.push('liveHttp.urlTemplate is required.');
      } else if (!/^https?:\/\//i.test(urlT)) {
        errors.push('liveHttp.urlTemplate must use http:// or https://.');
      }
      if (lh.trigger && lh.trigger !== 'auto' && lh.trigger !== 'manual') {
        errors.push(`Invalid liveHttp.trigger "${lh.trigger}" (expected "auto" or "manual").`);
      }
      if (typeof lh.timeoutMs === 'number' && (lh.timeoutMs < 0 || lh.timeoutMs > 5000)) {
        errors.push('liveHttp.timeoutMs must be between 0 and 5000.');
      }
      const auth = lh.auth?.trim() ?? '';
      if (auth && !['none', 'gh_cli', 'az_devops'].includes(auth) && !/^env:[A-Z][A-Z0-9_]{0,63}$/.test(auth)) {
        errors.push(
          `Invalid liveHttp.auth "${auth}" (allowed: none, gh_cli, az_devops, env:VAR_NAME; inline secrets forbidden).`
        );
      }
      if (lh.headers) {
        for (const [hk, hv] of Object.entries(lh.headers)) {
          const lk = hk.toLowerCase();
          if (['authorization', 'proxy-authorization', 'cookie', 'x-api-key'].includes(lk) || lk.includes('token') || lk.includes('secret')) {
            errors.push(`Forbidden auth header "${hk}" in liveHttp.headers.`);
          } else if (/^(bearer |basic |ghp_|github_pat_|sk-)/i.test(String(hv).trim())) {
            errors.push(`Inline credential detected in liveHttp.headers["${hk}"].`);
          }
        }
      }
      const rcLayout = lh.resultCard?.layout;
      if (!rcLayout || !['facts', 'table', 'steps', 'link'].includes(rcLayout)) {
        errors.push('liveHttp.resultCard.layout must be one of: facts, table, steps, link.');
      }
    }
    return { rule: errors.length === 0 ? (r as PatternRule) : null, errors };
  });

  readonly validationErrors = computed<string[]>(() => this.parsedDraft().errors);
  readonly editingRuleId = computed<string>(() => this.parsedDraft().rule?.id ?? '');
  readonly draftRegexPreview = computed<string>(() => this.parsedDraft().rule?.regex ?? '');

  constructor() {
    this.resetDraft();
    effect(() => {
      const isOpen = this.open();
      const el = this.dialogEl()?.nativeElement;
      if (!el) return;
      if (isOpen && !el.open) {
        el.showModal();
        // Focus modal heading (tabindex="-1") to avoid heavy blue focus ring on Close
        this.dialogTitle()?.nativeElement.focus({ preventScroll: true });
        void this.refresh();
      } else if (!isOpen && el.open) {
        el.close();
      }
    });
  }

  async refresh(): Promise<void> {
    this.rules.set(await this.bridge.listPatternRules());
  }

  isRuleEnabled(rule: PatternRule): boolean {
    return rule.enabled !== false;
  }

  ruleExample(rule: PatternRule): string {
    if (rule.example?.trim()) return `Example: ${rule.example.trim()}`;
    const desc = rule.description?.trim() ?? '';
    const egMatch = desc.match(/\(e\.g\.\s*([^)]+)\)/i);
    if (egMatch?.[1]) {
      return `Example: ${egMatch[1].trim()}`;
    }
    if (desc) return desc;
    return rule.titleTemplate;
  }

  linkedEntityTargets(rule: PatternRule): string[] {
    const out: string[] = [];
    for (const lr of rule.linkRules ?? []) {
      const target = lr.targetEntityKey || lr.targetTag;
      if (target) out.push(target);
    }
    for (const key of rule.linkEntityKeys ?? []) {
      if (key && !out.includes(key)) out.push(key);
    }
    return out;
  }

  toggleEditor(): void {
    this.editorOpen.update((v) => !v);
  }

  startNewRule(): void {
    this.resetDraft();
    this.editorOpen.set(true);
  }

  loadIntoEditor(rule: PatternRule): void {
    this.draftJson.set(JSON.stringify(rule, null, 2));
    this.saveError.set('');
    this.editorOpen.set(true);
  }

  onDraftChange(next: string): void {
    this.draftJson.set(next);
    this.saveError.set('');
  }

  resetDraft(): void {
    this.saveError.set('');
    this.draftJson.set(
      JSON.stringify(
        {
          id: 'custom-ref',
          name: 'Custom Reference Decoder',
          enabled: true,
          priority: 60,
          normalize: ['trim', 'uppercase'],
          regex: '^(?P<prefix>REF)-(?P<branch>[A-Z]{3})-(?P<num>\\d{6})$',
          titleTemplate: 'Reference · {{prefix}}-{{branch}}-{{num}}',
          layout: 'segments',
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
      )
    );
  }

  async toggleRuleEnabled(rule: PatternRule): Promise<void> {
    const nextEnabled = !this.isRuleEnabled(rule);
    const updated = await this.bridge.upsertPatternRule({
      ...rule,
      enabled: nextEnabled,
    });
    this.rules.set(updated);
    this.patternsChanged.emit();
    this.state.showToast(
      `Pattern "${rule.name}" ${nextEnabled ? 'enabled' : 'disabled'}`,
      'info'
    );
  }

  async saveDraft(): Promise<void> {
    const { rule, errors } = this.parsedDraft();
    if (!rule || errors.length > 0) {
      return;
    }
    try {
      const updated = await this.bridge.upsertPatternRule(rule);
      this.rules.set(updated);
      this.saveError.set('');
      this.patternsChanged.emit();
      this.state.showToast(`Saved pattern rule "${rule.name || rule.id}"`, 'info');
    } catch (err) {
      this.saveError.set(err instanceof Error ? err.message : 'Failed to save PatternRule');
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
    this.matchedRuleLabel.set('');

    const sample = this.testSample().trim();
    if (!sample) {
      this.testError.set('Enter a sample ID to test.');
      return;
    }

    try {
      const candidates: PatternRule[] = [];
      const draftRule = this.parsedDraft().rule;
      if (this.editorOpen() && draftRule) {
        candidates.push(draftRule);
      }
      const activeRules = [...this.rules()]
        .filter((r) => this.isRuleEnabled(r))
        .sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0));
      for (const r of activeRules) {
        if (!candidates.some((c) => c.id === r.id)) {
          candidates.push(r);
        }
      }
      if (!this.editorOpen() && draftRule && !candidates.some((c) => c.id === draftRule.id)) {
        candidates.push(draftRule);
      }

      for (const candidate of candidates) {
        const preview = await this.bridge.testPatternRule(candidate, sample);
        if (preview) {
          this.matchedRuleLabel.set(`${candidate.name} (${candidate.id})`);
          this.testPreview.set({ ...preview, defaultExpanded: true, pinnable: false });
          return;
        }
      }

      this.testError.set(`No active pattern rule matched "${sample}"`);
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
