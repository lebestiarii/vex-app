import { Injectable, signal } from '@angular/core';
import {
  AppInfo,
  CardNode,
  CollectionStats,
  LogDiagnostics,
  ModuleStatus,
  PatternRule,
  SegmentTile,
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

  async searchRolodex(query: string, limit = 10): Promise<CardNode[]> {
    const desktopRes = await this.call<CardNode[]>('SearchRolodex', query, limit);
    return desktopRes ?? [];
  }

  async listRecentCards(limit = 6): Promise<CardNode[]> {
    const res = await this.call<CardNode[]>('ListRecentCards', limit);
    return res ?? [];
  }

  async recordCardUsed(id: string): Promise<void> {
    const clean = (id ?? '').trim();
    if (!clean) return;
    await this.call('RecordCardUsed', clean);
  }

  async listCollections(): Promise<CollectionStats[]> {
    const res = await this.call<CollectionStats[]>('ListCollections');
    return res ?? [];
  }

  async getLogInfo(): Promise<LogDiagnostics | null> {
    return this.call<LogDiagnostics>('GetLogInfo');
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

    // Standalone browser fallback when Wails runtime is unavailable
    let normalized = sampleInput;
    for (const norm of rule.normalize ?? []) {
      switch (norm.toLowerCase()) {
        case 'trim':
          normalized = normalized.trim();
          break;
        case 'uppercase':
        case 'upper':
          normalized = normalized.toUpperCase();
          break;
        case 'lowercase':
        case 'lower':
          normalized = normalized.toLowerCase();
          break;
        case 'strip-spaces':
          normalized = normalized.replace(/\s+/g, '');
          break;
        case 'strip-dashes':
          normalized = normalized.replace(/-/g, '');
          break;
      }
    }

    const re = new RegExp(
      rule.regex.replace(/\(\?P<([^>]+)>/g, '(?<$1>'),
      rule.caseInsensitive ? 'i' : ''
    );
    const m = re.exec(normalized);
    if (!m) return null;

    const interpolate = (tmpl: string): string =>
      tmpl.replace(/\{\{\s*([^}]+?)\s*\}\}/g, (_, k: string) => {
        if (k === 'match' || k === '0') return m[0];
        if (k.startsWith('lookup.')) {
          const name = k.slice('lookup.'.length);
          const rawVal = m.groups?.[name] ?? '';
          return rule.lookups?.[name]?.[rawVal] ?? rawVal;
        }
        return m.groups?.[k] ?? '';
      });

    const segments: SegmentTile[] = (rule.segments ?? []).map((seg) => ({
      value: interpolate(seg.value),
      role: interpolate(seg.role),
      meaning: interpolate(seg.meaning),
      tone: seg.tone,
    }));

    return {
      id: `preview-${rule.id}`,
      layout: rule.layout ?? 'segments',
      icon: rule.icon ?? '🧩',
      title: interpolate(rule.titleTemplate),
      chips: (rule.chips ?? [{ label: rule.id, tone: 'blue' }]).map((c) => ({
        ...c,
        label: interpolate(c.label),
      })),
      data: {
        segments,
        explanation: rule.explanationTemplate ? interpolate(rule.explanationTemplate) : '',
      },
      defaultExpanded: true,
      pinnable: false,
    };
  }

  async hideWindow(): Promise<void> {
    await this.call('HideWindow');
  }
}
