import { Injectable, signal } from '@angular/core';
import {
  AppInfo,
  CardNode,
  ModuleStatus,
  PatternRule,
  StarterModuleInfo,
  UserPrefs,
} from '../models/card-node.model';

interface WailsRuntimeModule {
  Call?: {
    ByName?: (method: string, ...args: unknown[]) => Promise<unknown>;
  };
  Events?: {
    On?: (name: string, cb: (ev: { data?: unknown }) => void) => () => void;
  };
}

@Injectable({ providedIn: 'root' })
export class VexBridgeService {
  readonly isDesktop = signal<boolean>(false);

  private runtimePromise: Promise<WailsRuntimeModule | null> | null = null;
  private sessionPrefs: UserPrefs = {
    onboardingCompleted: false,
    powerUserMode: false,
    aiEnabled: false,
  };

  private async getRuntime(): Promise<WailsRuntimeModule | null> {
    if (this.runtimePromise) return this.runtimePromise;
    this.runtimePromise = (async () => {
      try {
        const path = '/wails/runtime.js';
        const mod = (await import(/* @vite-ignore */ path)) as WailsRuntimeModule;
        if (mod?.Call?.ByName) {
          this.isDesktop.set(true);
          return mod;
        }
      } catch {
        // Standalone browser mode (outside Wails desktop shell)
      }
      this.isDesktop.set(false);
      return null;
    })();
    return this.runtimePromise;
  }

  private async call<T>(method: string, ...args: unknown[]): Promise<T | null> {
    const rt = await this.getRuntime();
    if (!rt?.Call?.ByName) {
      return null;
    }
    return (await rt.Call.ByName(`main.AppService.${method}`, ...args)) as T;
  }

  async getAppInfo(): Promise<AppInfo> {
    const res = await this.call<AppInfo>('GetAppInfo');
    return res ?? { name: 'Vex', version: '0.2.0' };
  }

  async getUserPrefs(): Promise<UserPrefs> {
    const res = await this.call<UserPrefs>('GetUserPrefs');
    return res ?? { ...this.sessionPrefs };
  }

  async setUserPrefs(prefs: UserPrefs): Promise<void> {
    this.sessionPrefs = { ...prefs };
    await this.call('SetUserPrefs', prefs);
  }

  async searchRolodex(query: string, limit = 20): Promise<CardNode[]> {
    const desktopRes = await this.call<CardNode[]>('SearchRolodex', query, limit);
    return desktopRes ?? [];
  }

  async listModules(): Promise<ModuleStatus[]> {
    const res = await this.call<ModuleStatus[]>('ListModules');
    return res ?? [];
  }

  async listStarterModules(): Promise<StarterModuleInfo[]> {
    const res = await this.call<StarterModuleInfo[]>('ListStarterModules');
    return res ?? [];
  }

  async installStarterModules(ids: string[]): Promise<ModuleStatus[]> {
    const res = await this.call<ModuleStatus[]>('InstallStarterModules', ids);
    return res ?? [];
  }

  async setEnabledModules(ids: string[]): Promise<ModuleStatus[]> {
    const res = await this.call<ModuleStatus[]>('SetEnabledModules', ids);
    return res ?? [];
  }

  async deleteModule(id: string): Promise<ModuleStatus[]> {
    const res = await this.call<ModuleStatus[]>('DeleteModule', id);
    return res ?? [];
  }

  async importModuleJSON(rawJSON: string): Promise<ModuleStatus | null> {
    return this.call<ModuleStatus>('ImportModuleJSON', rawJSON);
  }

  async exportModuleJSON(id: string): Promise<string> {
    const res = await this.call<string>('ExportModuleJSON', id);
    return res ?? '{}';
  }

  async listPatternRules(): Promise<PatternRule[]> {
    const res = await this.call<PatternRule[]>('ListPatternRules');
    return res ?? [];
  }

  async upsertPatternRule(rule: PatternRule): Promise<PatternRule[]> {
    const res = await this.call<PatternRule[]>('UpsertPatternRule', rule);
    return res ?? [];
  }

  async deletePatternRule(id: string): Promise<PatternRule[]> {
    const res = await this.call<PatternRule[]>('DeletePatternRule', id);
    return res ?? [];
  }

  async testPatternRule(rule: PatternRule, sampleInput: string): Promise<CardNode | null> {
    const res = await this.call<CardNode | null>('TestPatternRule', rule, sampleInput);
    if (res !== null) return res;

    const re = new RegExp(
      rule.regex.replace(/\(\?P<([^>]+)>/g, '(?<$1>)'),
      rule.caseInsensitive ? 'i' : ''
    );
    const m = re.exec(sampleInput.trim());
    if (!m) return null;
    const title = rule.titleTemplate.replace(/\{\{\s*([^}]+?)\s*\}\}/g, (_, k: string) => {
      if (k === 'match' || k === '0') return m[0];
      return m.groups?.[k] ?? '';
    });
    return {
      id: `preview-${rule.id}`,
      template: rule.template ?? 'pattern-breakdown',
      icon: rule.icon ?? '🧩',
      title,
      chips: [{ label: rule.id, tone: 'blue' }],
      data: { ...(m.groups ?? {}), match: m[0] },
      defaultExpanded: true,
      pinnable: false,
    };
  }

  async hideWindow(): Promise<void> {
    await this.call('HideWindow');
  }
}
