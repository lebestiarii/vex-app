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
import {
  CLIAuthStatus,
  ConnectedServerStatus,
  ExternalServerConfig,
  ModuleStatus,
  StarterModuleInfo,
} from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';

@Component({
  selector: 'app-modules-dialog',
  standalone: true,
  imports: [FormsModule],
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
          <div class="header-copy">
            <h2 #dialogTitle id="modules-dialog-title" class="dialog-title" tabindex="-1">
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
            <section class="section-block" aria-label="Installed Modules">
              <h3 class="section-heading">Installed Modules ({{ installed().length }})</h3>
              <div class="module-list">
                @for (mod of installed(); track mod.id) {
                  <article class="module-card">
                    <div class="mod-row-1">
                      <span class="mod-icon" aria-hidden="true">{{ mod.icon || '📦' }}</span>
                      <span class="mod-name">{{ mod.name }}</span>
                      <span class="mod-cat">{{ mod.category }}</span>
                    </div>

                    <p class="mod-desc">{{ mod.description }}</p>

                    <div class="mod-row-3">
                      <span class="mod-stats font-mono">
                        {{ mod.cardCount }} {{ mod.cardCount === 1 ? 'card' : 'cards' }} ·
                        {{ mod.patternCount }} {{ mod.patternCount === 1 ? 'pattern' : 'patterns' }}
                      </span>
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
                  </article>
                }
              </div>
            </section>
          }

          <section class="section-block" aria-label="Starter Catalog">
            <div class="section-head-row">
              <h3 class="section-heading">Starter Catalog</h3>
              @if (onboardingMode() && availableStarters().length > 0) {
                <button
                  type="button"
                  class="primary-btn"
                  (click)="installRecommendedStarters()"
                >
                  Install Recommended
                </button>
              }
            </div>

            @if (availableStarters().length > 0) {
              <div class="module-list">
                @for (st of availableStarters(); track st.id) {
                  <article class="module-card starter-card">
                    <div class="mod-row-1">
                      <span class="mod-icon" aria-hidden="true">{{ st.icon || '📦' }}</span>
                      <span class="mod-name">{{ st.name }}</span>
                      <span class="mod-cat">{{ st.category }}</span>
                    </div>

                    <p class="mod-desc">{{ st.description }}</p>

                    <div class="mod-row-3">
                      <span class="mod-stats font-mono">
                        {{ st.cardCount }} {{ st.cardCount === 1 ? 'card' : 'cards' }} ·
                        {{ st.patternCount }} {{ st.patternCount === 1 ? 'pattern' : 'patterns' }}
                      </span>
                      <div class="mod-controls">
                        <button
                          type="button"
                          class="primary-btn compact"
                          (click)="installStarter(st)"
                          [attr.aria-label]="'Install module ' + st.name"
                        >
                          Install
                        </button>
                      </div>
                    </div>
                  </article>
                }
              </div>
            } @else {
              <p class="all-installed-note">All packaged starter modules are installed.</p>
            }
          </section>

          <section class="section-block mcp-box" aria-label="MCP Server">
            <div class="mcp-header-row">
              <div class="mcp-title-group">
                <h3 class="section-heading">MCP Server</h3>
                <span
                  class="mcp-status-pill"
                  [class.listening]="mcpListening()"
                  [class.off]="!mcpListening()"
                >
                  {{ mcpListening() ? '● Listening' : '○ Off' }}
                </span>
              </div>
              <button
                type="button"
                class="mini-btn accent"
                (click)="copyClientJson()"
              >
                Copy Client JSON
              </button>
            </div>

            <p class="mod-desc">
              Connect Claude Desktop, Cursor, Copilot, or VS Code to validate and publish modules or pattern rules live.
            </p>

            <div class="mcp-endpoints">
              <div class="mcp-row">
                <span class="mcp-lbl">HTTP</span>
                <code class="mcp-code font-mono">{{ mcpHttpUrl() }}</code>
                <button
                  type="button"
                  class="mini-btn"
                  (click)="state.copyText('MCP HTTP URL', mcpHttpUrl())"
                >
                  Copy
                </button>
              </div>
              <div class="mcp-row">
                <span class="mcp-lbl">Stdio</span>
                <code class="mcp-code font-mono">vex --mcp-stdio</code>
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

          @if (!onboardingMode()) {
            <section class="section-block mcp-box" aria-label="External MCP & CLI Auth">
              <div class="mcp-header-row">
                <h3 class="section-heading">External MCP Presets &amp; CLI Auth</h3>
                <button
                  type="button"
                  class="mini-btn"
                  (click)="refreshExternalMCP()"
                >
                  Refresh Auth
                </button>
              </div>

              <p class="mod-desc">
                Query GitHub (<code>/gh</code>) and Azure DevOps (<code>/ado</code>) live using local OS CLI sessions — zero tokens stored in Vex.
              </p>

              @if (cliAuth().length > 0) {
                <div class="cli-auth-row" role="list" aria-label="Local CLI Auth Status">
                  @for (auth of cliAuth(); track auth.tool) {
                    <span
                      role="listitem"
                      class="cli-auth-pill"
                      [class.authed]="auth.authenticated"
                      [attr.title]="auth.detail || ''"
                    >
                      <span class="font-mono">{{ auth.tool }}</span>:
                      @if (auth.authenticated) {
                        ● {{ auth.account || 'Signed in' }}
                      } @else if (auth.installed) {
                        ○ Not signed in
                      } @else {
                        ○ Not installed
                      }
                    </span>
                  }
                </div>
              }

              @if (externalMcpPresets().length > 0) {
                <div class="module-list">
                  @for (srv of externalMcpPresets(); track srv.id) {
                    @let status = connectedServerMap()[srv.id];
                    <article class="module-card preset-card">
                      <div class="mod-row-1">
                        <span class="mod-icon" aria-hidden="true">{{ srv.id === 'github' ? '🐙' : '🔷' }}</span>
                        <span class="mod-name">{{ srv.name }}</span>
                        <span
                          class="mcp-status-pill"
                          [class.listening]="status?.connected"
                          [class.off]="!status?.connected"
                        >
                          {{ status?.connected ? '● Connected' : '○ Off' }}
                        </span>
                        @if (srv.localOnly) {
                          <span class="mod-cat">Local Only</span>
                        }
                      </div>

                      <div class="preset-cmd font-mono">
                        {{ formatServerCommand(srv) }}
                      </div>

                      @if (srv.id === 'azure-devops' && !status?.connected) {
                        <div class="preset-org-row">
                          <label for="ado-org-input" class="mcp-lbl">Org</label>
                          <input
                            id="ado-org-input"
                            type="text"
                            class="org-input font-mono"
                            placeholder="your-ado-org"
                            [ngModel]="adoOrg()"
                            (ngModelChange)="adoOrg.set($event)"
                          />
                        </div>
                      }

                      @if (presetError()[srv.id]; as errMsg) {
                        <p class="preset-error" role="alert">{{ errMsg }}</p>
                      }

                      <div class="mod-row-3">
                        <span class="mod-stats font-mono">
                          @if (status?.connected) {
                            {{ status.toolCount }} tools · {{ status.resourceCount }} resources
                          } @else {
                            {{ presetQueryHint(srv.id) }}
                          }
                        </span>
                        <div class="mod-controls">
                          @if (status?.connected) {
                            <button
                              type="button"
                              class="mini-btn accent"
                              [disabled]="busyServerId() === srv.id"
                              (click)="testServer(srv.id)"
                            >
                              Test
                            </button>
                            <button
                              type="button"
                              class="mini-btn danger"
                              [disabled]="busyServerId() === srv.id"
                              (click)="disconnectServer(srv.id)"
                            >
                              Disconnect
                            </button>
                          } @else {
                            <button
                              type="button"
                              class="primary-btn compact"
                              [disabled]="busyServerId() === srv.id"
                              (click)="connectServer(srv)"
                            >
                              {{ busyServerId() === srv.id ? 'Connecting…' : 'Enable & Connect' }}
                            </button>
                          }
                        </div>
                      </div>
                    </article>
                  }
                </div>
              }
            </section>
          }
        </div>
      </div>
    </dialog>
  `,
  styles: [
    `
      .vex-dialog {
        width: min(640px, calc(100vw - 24px));
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
        font-size: 11.5px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        color: var(--text-secondary);
      }
      .module-list {
        display: flex;
        flex-direction: column;
        gap: 8px;
      }
      /* Full-width 3-row module card layout */
      .module-card {
        display: flex;
        flex-direction: column;
        gap: 6px;
        padding: 11px 12px;
        border-radius: 10px;
        border: 1px solid var(--border-l0);
        background: var(--surface-l0);
      }
      .mod-row-1 {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
        width: 100%;
      }
      .mod-icon {
        font-size: 15px;
        line-height: 1;
        flex-shrink: 0;
      }
      .mod-name {
        font-size: 13.5px;
        font-weight: 700;
        color: var(--text-primary);
      }
      .mod-cat {
        font-size: 10.5px;
        font-weight: 600;
        padding: 1px 7px;
        border-radius: 999px;
        background: var(--bg-subtle);
        border: 1px solid var(--border-divider);
        color: var(--text-secondary);
      }
      .mod-desc {
        margin: 0;
        font-size: 12px;
        color: var(--text-muted);
        line-height: 1.45;
        width: 100%;
      }
      .mod-row-3 {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        padding-top: 7px;
        margin-top: 2px;
        border-top: 1px solid var(--border-divider);
      }
      .mod-stats {
        font-size: 11px;
        color: var(--text-secondary);
      }
      .mod-controls {
        display: inline-flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 6px;
        flex-shrink: 0;
      }
      .toggle-pill {
        min-block-size: 25px;
        padding: 2px 10px;
        border-radius: 999px;
        border: 1px solid var(--border-nested);
        background: var(--bg-subtle);
        color: var(--text-secondary);
        font-size: 11px;
        font-weight: 700;
        cursor: pointer;
      }
      .toggle-pill.on {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
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
        min-block-size: 28px;
        padding: 4px 12px;
        border-radius: 8px;
        border: 1px solid var(--chip-blue-border);
        background: var(--chip-blue-bg);
        color: var(--chip-blue-text);
        font-size: 11.5px;
        font-weight: 700;
        cursor: pointer;
      }
      .primary-btn.compact {
        min-block-size: 25px;
        padding: 2px 11px;
      }
      .all-installed-note {
        margin: 0;
        padding: 9px 12px;
        border-radius: 9px;
        border: 1px dashed var(--border-nested);
        background: var(--surface-l1);
        font-size: 12px;
        color: var(--text-muted);
      }
      .mcp-box {
        padding: 12px;
        border-radius: 10px;
        border: 1px solid var(--border-nested);
        background: var(--surface-l1);
      }
      .mcp-header-row {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
      }
      .mcp-title-group {
        display: inline-flex;
        align-items: center;
        gap: 8px;
      }
      .mcp-status-pill {
        display: inline-flex;
        align-items: center;
        padding: 1.5px 8px;
        border-radius: 999px;
        font-size: 11px;
        font-weight: 700;
        border: 1px solid var(--chip-neutral-border);
        background: var(--chip-neutral-bg);
        color: var(--text-muted);
      }
      .mcp-status-pill.listening {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .mcp-endpoints {
        display: flex;
        flex-direction: column;
        gap: 6px;
        margin-top: 4px;
      }
      .mcp-row {
        display: grid;
        grid-template-columns: 52px minmax(0, 1fr) auto;
        align-items: center;
        gap: 8px;
        font-size: 12px;
      }
      .mcp-lbl {
        font-weight: 600;
        color: var(--text-muted);
      }
      .mcp-code {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: 11.5px;
        color: var(--text-primary);
      }
      .cli-auth-row {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
      }
      .cli-auth-pill {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        padding: 2.5px 9px;
        border-radius: 999px;
        font-size: 11px;
        font-weight: 600;
        border: 1px solid var(--chip-neutral-border);
        background: var(--chip-neutral-bg);
        color: var(--text-secondary);
      }
      .cli-auth-pill.authed {
        background: var(--chip-green-bg);
        border-color: var(--chip-green-border);
        color: var(--chip-green-text);
      }
      .preset-cmd {
        font-size: 11px;
        color: var(--text-secondary);
        overflow-x: auto;
        white-space: nowrap;
      }
      .preset-org-row {
        display: grid;
        grid-template-columns: 36px minmax(0, 1fr);
        align-items: center;
        gap: 8px;
      }
      .org-input {
        min-block-size: 28px;
        padding: 3px 8px;
        border-radius: 7px;
        border: 1px solid var(--border-nested);
        background: var(--bg-input);
        color: var(--text-primary);
        font-size: 11.5px;
      }
      .preset-error {
        margin: 0;
        font-size: 11.5px;
        color: var(--chip-red-text);
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
  readonly mcpListening = signal<boolean>(false);
  readonly mcpHttpUrl = signal<string>('http://127.0.0.1:17380/mcp');
  readonly cliAuth = signal<CLIAuthStatus[]>([]);
  readonly externalMcpPresets = signal<ExternalServerConfig[]>([]);
  readonly connectedServers = signal<ConnectedServerStatus[]>([]);
  readonly busyServerId = signal<string | null>(null);
  readonly adoOrg = signal<string>('');
  readonly presetError = signal<Record<string, string>>({});

  readonly connectedServerMap = computed<Record<string, ConnectedServerStatus>>(() => {
    const map: Record<string, ConnectedServerStatus> = {};
    for (const st of this.connectedServers()) {
      map[st.config.id] = st;
    }
    return map;
  });

  readonly availableStarters = computed<StarterModuleInfo[]>(() =>
    this.starters().filter((s) => !s.installed)
  );

  private readonly dialogEl = viewChild<ElementRef<HTMLDialogElement>>('dialogEl');
  private readonly dialogTitle = viewChild<ElementRef<HTMLElement>>('dialogTitle');

  constructor() {
    effect(() => {
      const isOpen = this.open();
      const el = this.dialogEl()?.nativeElement;
      if (!el) return;
      if (isOpen && !el.open) {
        el.showModal();
        // Focus modal heading (tabindex="-1") instead of leaving a harsh focus ring on Close
        this.dialogTitle()?.nativeElement.focus({ preventScroll: true });
        void this.refresh();
      } else if (!isOpen && el.open) {
        el.close();
      }
    });
  }

  async refresh(): Promise<void> {
    const [mods, catalog, logInfo] = await Promise.all([
      this.bridge.listModules(),
      this.bridge.listStarterModules(),
      this.bridge.getLogInfo(),
    ]);
    this.installed.set(mods);
    this.starters.set(catalog);

    const metricsAddr = logInfo?.metricsAddr?.trim() ?? '';
    if (metricsAddr) {
      this.mcpListening.set(true);
      this.mcpHttpUrl.set(metricsAddr.replace(/\/metrics\/?$/, '/mcp'));
    } else {
      this.mcpListening.set(false);
      this.mcpHttpUrl.set('http://127.0.0.1:17380/mcp');
    }

    await this.refreshExternalMCP();
  }

  async refreshExternalMCP(): Promise<void> {
    const [auth, presets, connected] = await Promise.all([
      typeof this.bridge.probeCLIAuth === 'function' ? this.bridge.probeCLIAuth() : Promise.resolve([]),
      typeof this.bridge.getConfiguredMCPServers === 'function'
        ? this.bridge.getConfiguredMCPServers()
        : Promise.resolve([]),
      typeof this.bridge.listMCPServers === 'function' ? this.bridge.listMCPServers() : Promise.resolve([]),
    ]);
    this.cliAuth.set(auth);
    this.externalMcpPresets.set(presets);
    this.connectedServers.set(connected);

    const ado = presets.find((p) => p.id === 'azure-devops');
    if (ado?.args?.length) {
      const lastArg = ado.args[ado.args.length - 1] ?? '';
      if (lastArg && lastArg !== '<your-org-name>' && !this.adoOrg()) {
        this.adoOrg.set(lastArg);
      }
    }
  }

  formatServerCommand(srv: ExternalServerConfig): string {
    if (srv.transport === 'http' || srv.transport === 'sse') {
      return srv.url || srv.transport;
    }
    return [srv.command, ...(srv.args ?? [])].filter(Boolean).join(' ');
  }

  presetQueryHint(serverID: string): string {
    if (serverID === 'github') return '/gh <query>';
    if (serverID === 'azure-devops') return '/ado <query>';
    return `@mcp:${serverID}`;
  }

  async connectServer(srv: ExternalServerConfig): Promise<void> {
    this.busyServerId.set(srv.id);
    this.presetError.update((prev) => ({ ...prev, [srv.id]: '' }));
    try {
      const cfg: ExternalServerConfig = {
        ...srv,
        args: [...(srv.args ?? [])],
        enabled: true,
        localOnly: true,
      };
      if (cfg.id === 'azure-devops' && cfg.args) {
        const org = this.adoOrg().trim();
        const idx = cfg.args.findIndex((a) => a === '<your-org-name>');
        if (idx >= 0) {
          if (!org) {
            this.presetError.update((prev) => ({
              ...prev,
              [srv.id]: 'Enter your Azure DevOps organization name first.',
            }));
            return;
          }
          cfg.args[idx] = org;
        }
      }
      const status = await this.bridge.connectMCPServer(cfg);
      if (status?.connected) {
        this.state.showToast(`Connected ${srv.name} (${status.toolCount} tools)`, 'info');
      } else {
        this.presetError.update((prev) => ({
          ...prev,
          [srv.id]: `Could not connect ${srv.name}. Verify the server binary is installed.`,
        }));
      }
      await this.refreshExternalMCP();
      this.modulesChanged.emit();
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Connection failed';
      this.presetError.update((prev) => ({ ...prev, [srv.id]: msg }));
    } finally {
      this.busyServerId.set(null);
    }
  }

  async testServer(serverID: string): Promise<void> {
    this.busyServerId.set(serverID);
    this.presetError.update((prev) => ({ ...prev, [serverID]: '' }));
    try {
      const status = await this.bridge.testMCPServer(serverID);
      if (status?.connected) {
        this.state.showToast(
          `${status.config.name} OK · ${status.toolCount} tools, ${status.resourceCount} resources`,
          'info'
        );
      } else {
        this.presetError.update((prev) => ({ ...prev, [serverID]: 'Server health check failed.' }));
      }
      await this.refreshExternalMCP();
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Health check failed';
      this.presetError.update((prev) => ({ ...prev, [serverID]: msg }));
    } finally {
      this.busyServerId.set(null);
    }
  }

  async disconnectServer(serverID: string): Promise<void> {
    this.busyServerId.set(serverID);
    try {
      await this.bridge.disconnectMCPServer(serverID);
      this.state.showToast(`Disconnected ${serverID} MCP server`, 'info');
      await this.refreshExternalMCP();
      this.modulesChanged.emit();
    } finally {
      this.busyServerId.set(null);
    }
  }

  async installStarter(starter: StarterModuleInfo): Promise<void> {
    await this.bridge.installStarterModules([starter.id]);
    if (this.onboardingMode()) {
      const prefs = await this.bridge.getUserPrefs();
      await this.bridge.setUserPrefs({ ...prefs, onboardingCompleted: true });
    }
    this.state.showToast(`Installed module "${starter.name}"`, 'info');
    await this.refresh();
    this.modulesChanged.emit();
  }

  async installRecommendedStarters(): Promise<void> {
    const recommended = this.availableStarters()
      .filter((s) => s.defaultEnabled)
      .map((s) => s.id);
    const ids = recommended.length > 0 ? recommended : this.availableStarters().map((s) => s.id);
    if (ids.length > 0) {
      await this.bridge.installStarterModules(ids);
      this.state.showToast(`Installed ${ids.length} starter module(s)`, 'info');
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

  copyClientJson(): void {
    const snippet = JSON.stringify(
      {
        mcpServers: {
          vex: {
            url: this.mcpHttpUrl(),
          },
        },
      },
      null,
      2
    );
    void this.state.copyText('MCP Client JSON', snippet);
  }

  closeDialog(): void {
    this.dialogEl()?.nativeElement.close();
    this.closed.emit();
  }

  onNativeClose(): void {
    this.closed.emit();
  }
}
