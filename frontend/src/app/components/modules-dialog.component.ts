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
import { ModuleStatus, StarterModuleInfo } from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';

@Component({
  selector: 'app-modules-dialog',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <dialog
      #dialogEl
      class="vex-dialog"
      aria-labelledby="modules-dialog-title"
      (cancel)="onNativeClose()"
    >
      <div class="dialog-inner">
        <header class="dialog-header">
          <div>
            <h2 id="modules-dialog-title" class="dialog-title">
              {{ onboardingMode() ? 'Choose Starter Modules' : 'Optional Modules & MCP' }}
            </h2>
            <p class="dialog-sub">
              Every module is optional and stored as an editable JSON file in your user modules folder.
              Toggle, update via MCP, or delete any module at any time.
            </p>
          </div>
          <button type="button" class="close-btn" (click)="closeDialog()">Close</button>
        </header>

        <div class="dialog-body">
          @if (!onboardingMode() && installed().length > 0) {
            <section class="section-block">
              <h3 class="section-heading">Installed Modules ({{ installed().length }})</h3>
              <div class="module-list">
                @for (mod of installed(); track mod.id) {
                  <div class="module-row">
                    <div class="mod-meta">
                      <div class="mod-title-line">
                        <span aria-hidden="true">{{ mod.icon || '📦' }}</span>
                        <span class="mod-name">{{ mod.name }}</span>
                        <span class="mod-cat">{{ mod.category }}</span>
                      </div>
                      <p class="mod-desc">{{ mod.description }}</p>
                      <div class="mod-stats font-mono">
                        {{ mod.cardCount }} cards · {{ mod.patternCount }} patterns
                      </div>
                    </div>
                    <div class="mod-controls">
                      <button
                        type="button"
                        class="toggle-pill"
                        [class.on]="mod.enabled"
                        [attr.aria-pressed]="mod.enabled"
                        (click)="toggleModule(mod)"
                      >
                        {{ mod.enabled ? 'Enabled' : 'Disabled' }}
                      </button>
                      <button type="button" class="mini-btn" (click)="exportModule(mod)">
                        Copy JSON
                      </button>
                      <button
                        type="button"
                        class="mini-btn danger"
                        (click)="deleteModule(mod)"
                        [attr.aria-label]="'Delete module ' + mod.name"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                }
              </div>
            </section>
          }

          <section class="section-block">
            <div class="section-head-row">
              <h3 class="section-heading">Packaged Optional Starter Catalog</h3>
              <button type="button" class="primary-btn" (click)="installSelectedStarters()">
                {{ onboardingMode() ? 'Load Selected & Start' : 'Install Selected' }}
              </button>
            </div>
            <div class="module-list">
              @for (st of starters(); track st.id) {
                <label class="starter-row" [class.already]="st.installed">
                  <input
                    type="checkbox"
                    [checked]="selectedStarters().has(st.id)"
                    (change)="toggleStarterSelection(st.id)"
                  />
                  <div class="mod-meta">
                    <div class="mod-title-line">
                      <span aria-hidden="true">{{ st.icon || '📦' }}</span>
                      <span class="mod-name">{{ st.name }}</span>
                      <span class="mod-cat">{{ st.category }}</span>
                      @if (st.installed) {
                        <span class="installed-tag">Installed</span>
                      }
                    </div>
                    <p class="mod-desc">{{ st.description }}</p>
                    <div class="mod-stats font-mono">
                      {{ st.cardCount }} cards · {{ st.patternCount }} patterns
                    </div>
                  </div>
                </label>
              }
            </div>
          </section>

          <section class="section-block mcp-box">
            <h3 class="section-heading">MCP Server for Live Module & Template Updates</h3>
            <p class="mod-desc">
              Connect Claude Desktop, Cursor, Copilot, or VS Code to update modules, templates, and pattern rules directly.
            </p>
            <div class="mcp-endpoints">
              <div class="mcp-row">
                <span class="mcp-lbl">HTTP Endpoint</span>
                <code class="font-mono">http://127.0.0.1:17380/mcp</code>
                <button
                  type="button"
                  class="mini-btn"
                  (click)="state.copyText('MCP HTTP URL', 'http://127.0.0.1:17380/mcp')"
                >
                  Copy
                </button>
              </div>
              <div class="mcp-row">
                <span class="mcp-lbl">Stdio Mode</span>
                <code class="font-mono">vex --mcp-stdio</code>
                <button
                  type="button"
                  class="mini-btn"
                  (click)="state.copyText('MCP Stdio Flag', 'vex --mcp-stdio')"
                >
                  Copy
                </button>
              </div>
            </div>
          </section>
        </div>
      </div>
    </dialog>
  `,
  styles: [
    `
      .vex-dialog {
        width: min(680px, calc(100vw - 28px));
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
        line-height: 1.4;
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
        gap: 16px;
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
        gap: 10px;
      }
      .section-heading {
        margin: 0;
        font-size: 12.5px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        color: var(--text-secondary);
      }
      .module-list {
        display: flex;
        flex-direction: column;
        gap: 7px;
      }
      .module-row,
      .starter-row {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 12px;
        padding: 10px 12px;
        border-radius: 10px;
        border: 1px solid var(--border-l0);
        background: var(--surface-l0);
      }
      .starter-row {
        justify-content: flex-start;
        cursor: pointer;
      }
      .starter-row input[type='checkbox'] {
        margin-top: 3px;
        width: 16px;
        height: 16px;
        accent-color: var(--accent);
        flex-shrink: 0;
      }
      .mod-meta {
        flex: 1;
        min-width: 0;
      }
      .mod-title-line {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
      }
      .mod-name {
        font-size: 13.5px;
        font-weight: 700;
      }
      .mod-cat {
        font-size: 11px;
        font-weight: 600;
        padding: 1px 7px;
        border-radius: 999px;
        background: var(--bg-subtle);
        border: 1px solid var(--border-divider);
        color: var(--text-secondary);
      }
      .installed-tag {
        font-size: 11px;
        font-weight: 700;
        padding: 1px 7px;
        border-radius: 999px;
        background: var(--chip-green-bg);
        border: 1px solid var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .mod-desc {
        margin: 4px 0 0;
        font-size: 12px;
        color: var(--text-muted);
        line-height: 1.4;
      }
      .mod-stats {
        margin-top: 4px;
        font-size: 11px;
        color: var(--text-secondary);
      }
      .mod-controls {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
        flex-shrink: 0;
      }
      .toggle-pill {
        min-block-size: 26px;
        padding: 3px 10px;
        border-radius: 999px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        font-size: 11.5px;
        font-weight: 700;
        cursor: pointer;
      }
      .toggle-pill.on {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
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
        min-block-size: 30px;
        padding: 5px 13px;
        border-radius: 8px;
        border: 1px solid var(--chip-blue-border);
        background: var(--chip-blue-bg);
        color: var(--chip-blue-text);
        font-size: 12px;
        font-weight: 700;
        cursor: pointer;
      }
      .mcp-box {
        padding: 12px;
        border-radius: 10px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l1);
      }
      .mcp-endpoints {
        display: flex;
        flex-direction: column;
        gap: 6px;
        margin-top: 6px;
      }
      .mcp-row {
        display: grid;
        grid-template-columns: 100px minmax(0, 1fr) auto;
        align-items: center;
        gap: 8px;
        font-size: 12px;
      }
      .mcp-lbl {
        font-weight: 600;
        color: var(--text-muted);
      }
    `,
  ],
})
export class ModulesDialogComponent {
  readonly open = input<boolean>(false);
  readonly onboardingMode = input<boolean>(false);
  readonly closed = output<void>();
  readonly modulesChanged = output<void>();

  readonly bridge = inject(VexBridgeService);
  readonly state = inject(RolodexStateService);

  readonly installed = signal<ModuleStatus[]>([]);
  readonly starters = signal<StarterModuleInfo[]>([]);
  readonly selectedStarters = signal<Set<string>>(new Set());

  private readonly dialogEl = viewChild<ElementRef<HTMLDialogElement>>('dialogEl');

  constructor() {
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
    const [mods, catalog] = await Promise.all([
      this.bridge.listModules(),
      this.bridge.listStarterModules(),
    ]);
    this.installed.set(mods);
    this.starters.set(catalog);

    // Pre-select default-enabled starter modules when in onboarding or none installed
    if (this.onboardingMode() || mods.length === 0) {
      this.selectedStarters.set(new Set(catalog.filter((s) => s.defaultEnabled).map((s) => s.id)));
    } else {
      this.selectedStarters.set(new Set(catalog.filter((s) => !s.installed).map((s) => s.id)));
    }
  }

  toggleStarterSelection(id: string): void {
    this.selectedStarters.update((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async installSelectedStarters(): Promise<void> {
    const ids = Array.from(this.selectedStarters());
    if (ids.length > 0) {
      await this.bridge.installStarterModules(ids);
      this.state.showToast(`Loaded ${ids.length} optional module(s)`, 'info');
    }
    if (this.onboardingMode()) {
      const prefs = await this.bridge.getUserPrefs();
      await this.bridge.setUserPrefs({ ...prefs, onboardingCompleted: true });
    }
    await this.refresh();
    this.modulesChanged.emit();
    if (this.onboardingMode()) {
      this.closeDialog();
    }
  }

  async toggleModule(mod: ModuleStatus): Promise<void> {
    const nextEnabled = this.installed()
      .filter((m) => (m.id === mod.id ? !m.enabled : m.enabled))
      .map((m) => m.id);
    const updated = await this.bridge.setEnabledModules(nextEnabled);
    this.installed.set(updated);
    this.modulesChanged.emit();
    this.state.showToast(`${mod.name} ${!mod.enabled ? 'enabled' : 'disabled'}`, 'info');
  }

  async deleteModule(mod: ModuleStatus): Promise<void> {
    await this.bridge.deleteModule(mod.id);
    await this.refresh();
    this.modulesChanged.emit();
    this.state.showToast(`Deleted module ${mod.name}`, 'info');
  }

  async exportModule(mod: ModuleStatus): Promise<void> {
    const raw = await this.bridge.exportModuleJSON(mod.id);
    await this.state.copyText(`${mod.id}.json`, raw);
  }

  closeDialog(): void {
    this.dialogEl()?.nativeElement.close();
    this.closed.emit();
  }

  onNativeClose(): void {
    this.closed.emit();
  }
}
