import type {
  AIConfig,
  ActionResult,
  AnchorDirection,
  AppInfo,
  CollectionStats,
  ConnectedServerStatus,
  ETLMappingPreview,
  ETLMappingRule,
  LogDiagnostics,
  LogEntry,
  ModuleStatus,
  ParsedQuery,
  RAGRequest,
  RAGResponse,
  RawDataSample,
  SearchCard,
  SearchResponse,
  UpdateCheckResult,
  UserPrefs,
  WeatherInfo,
} from '../types/models';

type WailsRuntimeModule = {
  Call: {
    ByName: <T = unknown>(methodName: string, ...args: unknown[]) => Promise<T>;
  };
  Events?: {
    On: (eventName: string, callback: (ev: { data?: unknown }) => void) => () => void;
  };
};

let wailsRuntimePromise: Promise<WailsRuntimeModule | null> | null = null;

function getWailsRuntime(): Promise<WailsRuntimeModule | null> {
  if (!wailsRuntimePromise) {
    const runtimeUrl = '/wails/runtime.js';
    wailsRuntimePromise = import(/* @vite-ignore */ runtimeUrl)
      .then((mod) => (mod?.Call?.ByName ? (mod as WailsRuntimeModule) : null))
      .catch(() => null);
  }
  return wailsRuntimePromise;
}

async function tryCallWails<T>(
  method: string,
  ...args: unknown[]
): Promise<{ handled: true; value: T } | { handled: false }> {
  const rt = await getWailsRuntime();
  if (!rt) {
    return { handled: false };
  }
  const value = await rt.Call.ByName<T>(`main.AppService.${method}`, ...args);
  return { handled: true, value };
}

// --- Seeded dev fallback cards (mirrors indexer.SeedDefaultCollections) ---
let devCards: SearchCard[] = [
  {
    id: 'shortcuts-github',
    title: 'GitHub Dashboard',
    subtitle: 'Open GitHub repositories and pull requests',
    description:
      'Quick shortcut to GitHub web app. Supports searching repositories via modifier /gh.',
    collection: 'shortcuts',
    template: 'shortcut',
    icon: 'globe',
    tags: ['dev', 'web', 'git'],
    modifiers: ['gh', 'bm'],
    data: { url: 'https://github.com', mode: 'browser' },
    actions: [
      {
        id: 'open',
        label: 'Open in Browser',
        shortcut: 'Enter',
        type: 'open_url',
        target: 'https://github.com/{{arg1}}',
      },
      {
        id: 'pwa',
        label: 'Open as Standalone Web App',
        shortcut: 'Cmd+Enter',
        type: 'open_pwa',
        target: 'https://github.com',
      },
    ],
  },
  {
    id: 'knowledge-wails-v3',
    title: 'Vex Quick Guide & Modifiers',
    subtitle: 'How to use /, @, #, and ? in the omnibar',
    description:
      'Use /modifier to scope commands, @collection to filter JSON files, #tag to filter tags, and ? or Tab to ask the AI RAG assistant grounded in your local JSON cards.',
    collection: 'knowledge',
    template: 'knowledge',
    icon: 'book-open',
    tags: ['help', 'guide', 'rag'],
    modifiers: ['help', 'docs'],
    data: {
      modifiers: '/alias',
      collection: '@name',
      tags: '#tag',
      ai_rag: '? question',
    },
    actions: [
      {
        id: 'ask',
        label: 'Ask AI about this Guide',
        shortcut: 'Enter',
        type: 'ai_prompt',
        target: 'Summarize how to use Vex modifiers',
      },
    ],
  },
  {
    id: 'devops-git-status',
    title: 'System Uptime & Date Snippet',
    subtitle: 'Copy quick local health check command',
    description:
      'Demonstrates a parameterized command snippet card that copies safely to your clipboard.',
    collection: 'devops',
    template: 'command',
    icon: 'terminal',
    tags: ['ops', 'snippet', 'system'],
    modifiers: ['ops', 'sys'],
    data: { command: 'uname -a && date' },
    actions: [
      {
        id: 'copy',
        label: 'Copy Command',
        shortcut: 'Enter',
        type: 'copy',
        target: 'uname -a && date',
      },
    ],
  },
  {
    id: 'devops-env-cheatsheet',
    title: 'Staging Cluster Endpoints',
    subtitle: 'Key-Value Reference Card',
    description:
      'Example kv-grid card storing structured metadata for instant lookup and AI RAG grounding.',
    collection: 'devops',
    template: 'kv-grid',
    icon: 'server',
    tags: ['staging', 'k8s', 'ops'],
    modifiers: ['k8s', 'env'],
    data: {
      cluster: 'staging-us-west-2',
      api_url: 'https://api.staging.internal',
      namespace: 'search-platform',
      mcp_port: '17380',
    },
    actions: [
      {
        id: 'copy-api',
        label: 'Copy API URL',
        shortcut: 'Enter',
        type: 'copy',
        target: 'https://api.staging.internal',
      },
    ],
  },
  {
    id: 'devops-cluster-metrics',
    title: 'Cluster Health Metrics',
    subtitle: 'Live status widget example',
    description: 'Metric card template displaying structured status counters.',
    collection: 'devops',
    template: 'metric-card',
    icon: 'activity',
    tags: ['metrics', 'ops'],
    modifiers: ['status'],
    data: {
      status: 'healthy',
      uptime: '99.98%',
      latency_ms: 42,
      active_pods: 18,
    },
    actions: [
      {
        id: 'copy-json',
        label: 'Copy Metrics JSON',
        shortcut: 'Enter',
        type: 'copy',
        target: '{"status":"healthy","uptime":"99.98%","latency_ms":42}',
      },
    ],
  },
];

let devAIConfig: AIConfig = {
  provider: 'ollama',
  baseUrl: 'http://localhost:11434',
  chatModel: 'llama3.2',
  embeddingModel: 'nomic-embed-text',
  temperature: 0.2,
  maxContextCards: 8,
};

function parseQueryLocal(raw: string): ParsedQuery {
  const trimmed = raw.trim();
  const pq: ParsedQuery = { raw, mode: 'search', text: '', tags: [], params: {}, args: [] };
  if (!trimmed) return pq;
  let rest = trimmed;
  if (rest.startsWith('?')) {
    pq.mode = 'ai';
    rest = rest.slice(1).trim();
  }
  const textParts: string[] = [];
  for (const tok of rest.split(/\s+/).filter(Boolean)) {
    if (tok.startsWith('/') && tok.length > 1 && !pq.modifier && !tok.slice(1).includes('/')) {
      pq.modifier = tok.slice(1).toLowerCase();
    } else if (tok.startsWith('@') && tok.length > 1 && !pq.collection) {
      pq.collection = tok.slice(1).toLowerCase();
    } else if (tok.startsWith('#') && tok.length > 1) {
      pq.tags = [...(pq.tags ?? []), tok.slice(1).toLowerCase()];
    } else if (tok.includes(':') && !tok.includes('://')) {
      const [k, v] = tok.split(':', 2);
      if (k === 'type' || k === 'tpl') {
        pq.template = v as ParsedQuery['template'];
      } else {
        pq.params = { ...(pq.params ?? {}), [k.toLowerCase()]: v };
      }
    } else {
      textParts.push(tok);
      pq.args = [...(pq.args ?? []), tok];
    }
  }
  pq.text = textParts.join(' ');
  return pq;
}

let devHotkey = 'Ctrl+Shift+B';
let devUserPrefs: UserPrefs = {
  onboardingCompleted: false,
  powerUserMode: false,
  aiEnabled: false,
  layoutMode: 'compact',
  selectedGoals: ['links', 'notes'],
};

export const bridge = {
  async search(rawQuery: string, limit = 20): Promise<SearchResponse> {
    const res = await tryCallWails<SearchResponse>('Search', rawQuery, limit);
    if (res.handled) return res.value;
    const parsed = parseQueryLocal(rawQuery);
    const needle = parsed.text.toLowerCase();
    const filtered = devCards
      .filter((c) => {
        if (parsed.collection && c.collection !== parsed.collection.replace(':', '-')) return false;
        if (parsed.template && c.template !== parsed.template) return false;
        if (parsed.modifier && !(c.modifiers ?? []).some((m) => m.startsWith(parsed.modifier!))) return false;
        if (parsed.tags?.length && !parsed.tags.every((t) => (c.tags ?? []).includes(t))) return false;
        if (!needle) return true;
        return (
          c.title.toLowerCase().includes(needle) ||
          (c.subtitle ?? '').toLowerCase().includes(needle) ||
          (c.description ?? '').toLowerCase().includes(needle) ||
          (c.tags ?? []).some((t) => t.includes(needle))
        );
      })
      .slice(0, limit)
      .map((card, i) => ({
        card,
        score: 1 - i * 0.05,
        matchSource: needle ? 'fts' : 'filter',
      }));

    return { parsed, results: filtered, total: filtered.length };
  },

  async executeAction(
    cardId: string,
    actionId: string,
    rawQuery: string,
    extraArgs: Record<string, string>,
  ): Promise<ActionResult> {
    const res = await tryCallWails<ActionResult>('ExecuteAction', cardId, actionId, rawQuery, extraArgs);
    if (res.handled) return res.value;
    const card = devCards.find((c) => c.id === cardId);
    const act = card?.actions?.find((a) => a.id === actionId) ?? card?.actions?.[0];
    if (!act) {
      return { success: false, actionType: '', resolvedTarget: '', error: 'No action found', durationMs: 1 };
    }
    if (act.type === 'copy' && navigator.clipboard) {
      await navigator.clipboard.writeText(act.target).catch(() => {});
    }
    return {
      success: true,
      actionType: act.type,
      resolvedTarget: act.target,
      output:
        act.type === 'copy'
          ? act.target
          : `Triggered ${act.type}: ${act.target}`,
      durationMs: 4,
      openAiMode: act.type === 'ai_prompt',
    };
  },

  async upsertCard(card: SearchCard): Promise<SearchCard> {
    const res = await tryCallWails<SearchCard>('UpsertCard', card);
    if (res.handled) return res.value;
    const normalized: SearchCard = {
      ...card,
      id: card.id || `${card.collection || 'default'}-${Date.now()}`,
      collection: (card.collection || 'default').toLowerCase().replace(/[^a-z0-9]+/g, '-'),
    };
    const idx = devCards.findIndex((c) => c.id === normalized.id);
    if (idx >= 0) devCards[idx] = normalized;
    else devCards = [normalized, ...devCards];
    return normalized;
  },

  async sampleRawData(rawContent: string, filenameHint: string): Promise<RawDataSample> {
    const res = await tryCallWails<RawDataSample>('SampleRawData', rawContent, filenameHint);
    if (res.handled) return res.value;
    const trimmed = rawContent.trim();
    if (trimmed.startsWith('[') || trimmed.startsWith('{')) {
      const parsed = JSON.parse(trimmed);
      const arr: Record<string, unknown>[] = Array.isArray(parsed) ? parsed : [parsed];
      const fields = Array.from(new Set(arr.flatMap((r) => Object.keys(r))));
      return { format: 'json', fields, sampleRows: arr.slice(0, 5), totalRows: arr.length };
    }
    const lines = trimmed.split(/\r?\n/).filter(Boolean);
    const headers = (lines[0] ?? '').split(',').map((h) => h.trim());
    const rows = lines.slice(1).map((line) => {
      const vals = line.split(',').map((v) => v.trim());
      return Object.fromEntries(headers.map((h, i) => [h, vals[i] ?? '']));
    });
    return { format: 'csv', fields: headers, sampleRows: rows.slice(0, 5), totalRows: rows.length };
  },

  async generateETLMappingWithPreview(
    rawContent: string,
    filenameHint: string,
    userHint: string,
  ): Promise<ETLMappingPreview> {
    const res = await tryCallWails<ETLMappingPreview>(
      'GenerateETLMappingWithPreview',
      rawContent,
      filenameHint,
      userHint,
    );
    if (res.handled) return res.value;
    const sample = await this.sampleRawData(rawContent, filenameHint);
    const titleField = sample.fields[0] ?? 'title';
    const urlField = sample.fields.find((f) => /url|link|href/i.test(f));
    const rule: ETLMappingRule = {
      name: `AI Mapped ${filenameHint || 'Import'}`,
      sourceFormat: sample.format,
      rootPath: sample.detectedRootPath,
      targetCollection: 'imported',
      defaultTemplate: urlField ? 'shortcut' : 'kv-grid',
      titleField: `{{${titleField}}}`,
      subtitleField: sample.fields[1] ? `{{${sample.fields[1]}}}` : '',
      actionType: urlField ? 'open_url' : 'copy',
      actionLabel: urlField ? 'Open Link' : 'Copy Title',
      actionTarget: urlField ? `{{${urlField}}}` : `{{${titleField}}}`,
    };
    const previewCards: SearchCard[] = sample.sampleRows.map((row, idx) => ({
      id: `preview-${idx}`,
      title: String(row[titleField] ?? `Row ${idx + 1}`),
      subtitle: sample.fields[1] ? String(row[sample.fields[1]] ?? '') : undefined,
      collection: rule.targetCollection,
      template: rule.defaultTemplate,
      data: row,
      actions: urlField
        ? [{ id: 'open', label: 'Open Link', type: 'open_url', target: String(row[urlField] ?? '') }]
        : [],
    }));
    return { rule, sample, previewCards, totalCards: sample.totalRows };
  },

  async importWithRule(rawContent: string, rule: ETLMappingRule): Promise<{ importedCount: number; collection: string }> {
    const res = await tryCallWails<{ importedCount: number; collection: string }>(
      'ImportWithRule',
      rawContent,
      rule,
    );
    if (res.handled) return res.value;
    const prev = await this.generateETLMappingWithPreview(rawContent, '', '');
    for (const c of prev.previewCards) {
      await this.upsertCard({ ...c, collection: rule.targetCollection, template: rule.defaultTemplate });
    }
    return { importedCount: prev.previewCards.length, collection: rule.targetCollection };
  },

  async askRAG(req: RAGRequest): Promise<RAGResponse> {
    const res = await tryCallWails<RAGResponse>('AskRAG', req);
    if (res.handled) return res.value;
    const searchRes = await this.search(req.query, req.limit ?? 5);
    const sources = searchRes.results.map((r, i) => ({
      index: i + 1,
      cardId: r.card.id,
      title: r.card.title,
      subtitle: r.card.subtitle,
      collection: r.card.collection,
      template: r.card.template,
      score: r.score,
      matchSource: r.matchSource,
    }));
    const answer =
      sources.length > 0
        ? `Based on your indexed cards, the most relevant match is **${sources[0].title}** [1] in \`@${sources[0].collection}\`. Connect Ollama or a cloud API key in Settings (Cmd+,) for live LLM synthesis.`
        : 'No matching cards were found in your local index for that question.';
    return {
      answer,
      sources,
      model: devAIConfig.chatModel,
      provider: devAIConfig.provider,
      durationMs: 12,
    };
  },

  async getAIConfig(): Promise<AIConfig> {
    const res = await tryCallWails<AIConfig>('GetAIConfig');
    if (res.handled) return res.value;
    return devAIConfig;
  },

  async setAIConfig(cfg: AIConfig): Promise<void> {
    const res = await tryCallWails<void>('SetAIConfig', cfg);
    if (res.handled) return;
    devAIConfig = cfg;
  },

  async syncEmbeddings(): Promise<number> {
    const res = await tryCallWails<number>('SyncEmbeddings');
    if (res.handled) return res.value;
    return devCards.length;
  },

  async listCollections(): Promise<CollectionStats[]> {
    const res = await tryCallWails<CollectionStats[]>('ListCollections');
    if (res.handled) return res.value;
    const map = new Map<string, { count: number; tags: Set<string> }>();
    for (const c of devCards) {
      const entry = map.get(c.collection) ?? { count: 0, tags: new Set<string>() };
      entry.count++;
      for (const t of c.tags ?? []) entry.tags.add(t);
      map.set(c.collection, entry);
    }
    return Array.from(map.entries()).map(([collection, v]) => ({
      collection,
      cardCount: v.count,
      tags: Array.from(v.tags),
    }));
  },

  async listMCPServers(): Promise<ConnectedServerStatus[]> {
    const res = await tryCallWails<ConnectedServerStatus[]>('ListMCPServers');
    if (res.handled) return res.value;
    return [];
  },

  async getHotkey(): Promise<string> {
    const res = await tryCallWails<string>('GetHotkey');
    if (res.handled) return res.value;
    return devHotkey;
  },

  async setHotkey(accelerator: string): Promise<string> {
    const res = await tryCallWails<string>('SetHotkey', accelerator);
    if (res.handled) return res.value;
    devHotkey = accelerator;
    return devHotkey;
  },

  async getLaunchAtLogin(): Promise<boolean> {
    const res = await tryCallWails<boolean>('GetLaunchAtLogin');
    if (res.handled) return Boolean(res.value);
    return Boolean(devUserPrefs.launchAtLogin);
  },

  async setLaunchAtLogin(enabled: boolean): Promise<boolean> {
    const res = await tryCallWails<boolean>('SetLaunchAtLogin', enabled);
    if (res.handled) return Boolean(res.value);
    devUserPrefs = { ...devUserPrefs, launchAtLogin: enabled };
    return enabled;
  },

  async getUserPrefs(): Promise<UserPrefs> {
    const res = await tryCallWails<UserPrefs>('GetUserPrefs');
    if (res.handled) return res.value;
    return devUserPrefs;
  },

  async setUserPrefs(prefs: UserPrefs): Promise<void> {
    const res = await tryCallWails<void>('SetUserPrefs', prefs);
    if (res.handled) return;
    devUserPrefs = prefs;
  },

  async loadLogisticsDemoPack(): Promise<number> {
    const res = await tryCallWails<number>('LoadLogisticsDemoPack');
    if (res.handled) return res.value;
    return devCards.length;
  },

  async listModules(): Promise<ModuleStatus[]> {
    const res = await tryCallWails<ModuleStatus[]>('ListModules');
    if (res.handled) return res.value;
    return [];
  },

  async setEnabledModules(moduleIds: string[]): Promise<ModuleStatus[]> {
    const res = await tryCallWails<ModuleStatus[]>('SetEnabledModules', moduleIds);
    if (res.handled) return res.value;
    return [];
  },

  async importModuleJSON(rawJson: string): Promise<ModuleStatus | null> {
    const res = await tryCallWails<ModuleStatus>('ImportModuleJSON', rawJson);
    if (res.handled) return res.value;
    return null;
  },

  async importGitHubReposJSON(rawJson: string): Promise<ModuleStatus | null> {
    const res = await tryCallWails<ModuleStatus>('ImportGitHubReposJSON', rawJson);
    if (res.handled) return res.value;
    return null;
  },

  async exportModuleJSON(moduleId: string): Promise<string> {
    const res = await tryCallWails<string>('ExportModuleJSON', moduleId);
    if (res.handled) return res.value;
    return '{}';
  },

  async getRelatedCards(cardId: string, limit = 5): Promise<SearchCard[]> {
    const res = await tryCallWails<SearchCard[]>('GetRelatedCards', cardId, limit);
    if (res.handled) return res.value ?? [];
    return [];
  },

  async fetchWeather(lat: number, lon: number): Promise<WeatherInfo | null> {
    try {
      const res = await tryCallWails<WeatherInfo>('FetchWeather', lat, lon);
      if (res.handled) return res.value;
    } catch {
      return null;
    }
    return null;
  },

  async getLogInfo(): Promise<LogDiagnostics> {
    const res = await tryCallWails<LogDiagnostics>('GetLogInfo');
    if (res.handled) return res.value;
    return { filePath: '~/.vex/logs/vex.jsonl', level: 'info', sizeBytes: 0, metricsAddr: 'http://127.0.0.1:17380/metrics' };
  },

  async getRecentLogs(limit = 40): Promise<LogEntry[]> {
    const res = await tryCallWails<LogEntry[]>('GetRecentLogs', limit);
    if (res.handled) return res.value ?? [];
    return [];
  },

  async setLogLevel(level: string): Promise<string> {
    const res = await tryCallWails<string>('SetLogLevel', level);
    if (res.handled) return res.value;
    return level;
  },

  async clearLogs(): Promise<void> {
    await tryCallWails<void>('ClearLogs').catch(() => {});
  },

  async getAppInfo(): Promise<AppInfo> {
    const res = await tryCallWails<AppInfo>('GetAppInfo');
    if (res.handled && res.value) return res.value;
    return { name: 'Vex', edition: 'community', version: '0.1.0' };
  },

  async getAppVersion(): Promise<string> {
    const res = await tryCallWails<string>('GetAppVersion');
    if (res.handled) return res.value;
    return '0.1.0';
  },

  async getUpdateFeedURL(): Promise<string> {
    const res = await tryCallWails<string>('GetUpdateFeedURL');
    if (res.handled) return res.value;
    return '';
  },

  async setUpdateFeedURL(feedUrl: string): Promise<void> {
    await tryCallWails<void>('SetUpdateFeedURL', feedUrl);
  },

  async checkForUpdates(): Promise<UpdateCheckResult> {
    const res = await tryCallWails<UpdateCheckResult>('CheckForUpdates');
    if (res.handled) return res.value;
    return {
      currentVersion: '0.1.0',
      latestVersion: '0.1.0',
      updateAvailable: false,
      platformKey: 'browser',
      source: '',
    };
  },

  async getAnchorDirection(): Promise<AnchorDirection> {
    const res = await tryCallWails<string>('GetAnchorDirection');
    if (res.handled && res.value === 'up') return 'up';
    return 'down';
  },

  onAnchorChange(cb: (dir: AnchorDirection) => void): () => void {
    let unsub: (() => void) | null = null;
    let disposed = false;
    void getWailsRuntime().then((rt) => {
      if (disposed || !rt?.Events?.On) return;
      unsub = rt.Events.On('vex:anchor', (ev) => {
        const raw = Array.isArray(ev?.data) ? ev.data[0] : ev?.data;
        cb(raw === 'up' ? 'up' : 'down');
      });
    });
    return () => {
      disposed = true;
      unsub?.();
    };
  },

  async hideWindow(): Promise<void> {
    await tryCallWails<void>('HideWindow').catch(() => {});
  },
};
