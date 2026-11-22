import { useEffect, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import {
  Check,
  Compass,
  Copy,
  Download,
  ExternalLink,
  FileText,
  Keyboard,
  Layers,
  Plug,
  Plus,
  RefreshCw,
  Share2,
  Sliders,
  Sparkles,
  Upload,
  X,
} from 'lucide-react';
import { bridge } from '../lib/bridge';
import type {
  AIConfig,
  AIProviderType,
  CollectionStats,
  ConnectedServerStatus,
  LogDiagnostics,
  LogEntry,
  ModuleStatus,
  UpdateCheckResult,
  UserPrefs,
} from '../types/models';

interface SettingsModalProps {
  open: boolean;
  prefs: UserPrefs;
  onPrefsChange: (prefs: UserPrefs) => void;
  onHotkeyChange: (hotkey: string) => void;
  onReplayTour: () => void;
  onDataChanged: () => void;
  onClose: () => void;
}

const PROVIDER_DEFAULTS: Record<AIProviderType, { baseUrl: string; chatModel: string; embeddingModel: string }> = {
  ollama: { baseUrl: 'http://localhost:11434', chatModel: 'llama3.2', embeddingModel: 'nomic-embed-text' },
  openai: { baseUrl: 'https://api.openai.com/v1', chatModel: 'gpt-4o-mini', embeddingModel: 'text-embedding-3-small' },
  anthropic: { baseUrl: 'https://api.anthropic.com/v1', chatModel: 'claude-3-5-sonnet-latest', embeddingModel: '' },
  gemini: { baseUrl: 'https://generativelanguage.googleapis.com/v1beta', chatModel: 'gemini-2.5-flash', embeddingModel: 'text-embedding-004' },
};

export function SettingsModal({
  open,
  prefs,
  onPrefsChange,
  onHotkeyChange,
  onReplayTour,
  onDataChanged,
  onClose,
}: SettingsModalProps) {
  const [tab, setTab] = useState<'general' | 'modules' | 'ai' | 'mcp'>('general');
  const [modules, setModules] = useState<ModuleStatus[]>([]);
  const [showImportModule, setShowImportModule] = useState(false);
  const [importModuleRaw, setImportModuleRaw] = useState('');
  const [moduleMsg, setModuleMsg] = useState<string | null>(null);
  const [copiedModuleId, setCopiedModuleId] = useState<string | null>(null);
  const [cfg, setCfg] = useState<AIConfig>({
    provider: 'ollama',
    baseUrl: 'http://localhost:11434',
    chatModel: 'llama3.2',
    embeddingModel: 'nomic-embed-text',
    temperature: 0.2,
    maxContextCards: 8,
  });
  const [mcpServers, setMcpServers] = useState<ConnectedServerStatus[]>([]);
  const [osTarget, setOsTarget] = useState<'windows' | 'macos'>('windows');
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [statusMsg, setStatusMsg] = useState<string | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [hotkey, setHotkey] = useState('Ctrl+Shift+B');
  const [recordingHotkey, setRecordingHotkey] = useState(false);
  const [hotkeyMsg, setHotkeyMsg] = useState<string | null>(null);
  const [appVersion, setAppVersion] = useState('0.1.0');
  const [updateFeedUrl, setUpdateFeedUrl] = useState('');
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [updateResult, setUpdateResult] = useState<UpdateCheckResult | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [logInfo, setLogInfo] = useState<LogDiagnostics | null>(null);
  const [recentLogs, setRecentLogs] = useState<LogEntry[]>([]);
  const [showLogs, setShowLogs] = useState(false);
  const [availableCollections, setAvailableCollections] = useState<CollectionStats[]>([]);
  const [launchAtLogin, setLaunchAtLogin] = useState(Boolean(prefs.launchAtLogin));

  useEffect(() => {
    if (!open) return;
    bridge.getAIConfig().then(setCfg).catch(() => {});
    bridge.listMCPServers().then(setMcpServers).catch(() => {});
    bridge.getHotkey().then(setHotkey).catch(() => {});
    bridge.getLaunchAtLogin().then(setLaunchAtLogin).catch(() => {});
    bridge.listModules().then(setModules).catch(() => {});
    bridge.listCollections().then((list) => setAvailableCollections(list ?? [])).catch(() => {});
    bridge.getLogInfo().then(setLogInfo).catch(() => {});
    bridge.getAppVersion().then(setAppVersion).catch(() => {});
    bridge.getUpdateFeedURL().then(setUpdateFeedUrl).catch(() => {});
  }, [open]);

  if (!open) return null;

  const binPath = osTarget === 'windows' ? 'C:\\\\path\\\\to\\\\vex.exe' : '/usr/local/bin/vex';

  const snippets = [
    {
      id: 'vscode-copilot',
      title: 'VS Code GitHub Copilot (.vscode/mcp.json)',
      code: JSON.stringify(
        {
          servers: {
            vex: {
              type: 'http',
              url: 'http://127.0.0.1:17380/mcp',
            },
          },
        },
        null,
        2,
      ),
    },
    {
      id: 'copilot-cli',
      title: 'GitHub Copilot CLI (~/.copilot/mcp-config.json)',
      code: JSON.stringify(
        {
          mcpServers: {
            vex: {
              type: 'local',
              command: binPath,
              args: ['--mcp-stdio'],
            },
          },
        },
        null,
        2,
      ),
    },
    {
      id: 'claude-cursor',
      title: 'Claude Desktop / Cursor (stdio)',
      code: JSON.stringify(
        {
          mcpServers: {
            vex: {
              command: binPath,
              args: ['--mcp-stdio'],
            },
          },
        },
        null,
        2,
      ),
    },
  ];

  const updatePref = async (patch: Partial<UserPrefs>) => {
    const next: UserPrefs = { ...prefs, ...patch };
    onPrefsChange(next);
    await bridge.setUserPrefs(next).catch(() => {});
  };

  const handleSaveAI = async () => {
    await bridge.setAIConfig(cfg);
    setStatusMsg('AI settings saved.');
    setTimeout(() => setStatusMsg(null), 2000);
  };

  const handleSyncEmbeddings = async () => {
    setSyncing(true);
    setStatusMsg(null);
    try {
      const count = await bridge.syncEmbeddings();
      setStatusMsg(`Synced smart search index for ${count} items.`);
    } catch (e) {
      setStatusMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setSyncing(false);
    }
  };

  const handleCaptureHotkey = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (!recordingHotkey) return;
    e.preventDefault();
    e.stopPropagation();
    if (['Control', 'Shift', 'Alt', 'Meta'].includes(e.key)) return;

    const parts: string[] = [];
    if (e.ctrlKey) parts.push('Ctrl');
    if (e.metaKey) parts.push('Cmd');
    if (e.altKey) parts.push('Alt');
    if (e.shiftKey) parts.push('Shift');

    let keyName = e.key;
    if (keyName === ' ') keyName = 'Space';
    else if (keyName.length === 1) keyName = keyName.toUpperCase();
    parts.push(keyName);

    setHotkey(parts.join('+'));
    setRecordingHotkey(false);
  };

  const handleApplyHotkey = async () => {
    setHotkeyMsg(null);
    try {
      const applied = await bridge.setHotkey(hotkey);
      setHotkey(applied);
      onHotkeyChange(applied);
      setHotkeyMsg(`Global shortcut updated to ${applied}.`);
      setTimeout(() => setHotkeyMsg(null), 2500);
    } catch (e) {
      setHotkeyMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const handleToggleModule = async (modId: string) => {
    const currentEnabled = modules.filter((m) => m.enabled).map((m) => m.id);
    const nextEnabled = currentEnabled.includes(modId)
      ? currentEnabled.filter((id) => id !== modId)
      : [...currentEnabled, modId];
    try {
      const updated = await bridge.setEnabledModules(nextEnabled);
      setModules(updated);
      onPrefsChange({ ...prefs, enabledModules: nextEnabled });
      onDataChanged();
    } catch (e) {
      setModuleMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const handleShareModule = async (modId: string) => {
    try {
      const raw = await bridge.exportModuleJSON(modId);
      await navigator.clipboard?.writeText(raw).catch(() => {});
      setCopiedModuleId(modId);
      setTimeout(() => setCopiedModuleId(null), 1800);
    } catch (e) {
      setModuleMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const handleImportModule = async () => {
    if (!importModuleRaw.trim()) return;
    setModuleMsg(null);
    try {
      const imported = await bridge.importModuleJSON(importModuleRaw);
      const updated = await bridge.listModules();
      setModules(updated);
      setImportModuleRaw('');
      setShowImportModule(false);
      onDataChanged();
      if (imported) {
        setModuleMsg(`Imported and enabled module "${imported.name}"!`);
      }
    } catch (e) {
      setModuleMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const handleImportGitHubRepos = async () => {
    if (!importModuleRaw.trim()) return;
    setModuleMsg(null);
    try {
      const imported = await bridge.importGitHubReposJSON(importModuleRaw);
      const updated = await bridge.listModules();
      setModules(updated);
      setImportModuleRaw('');
      setShowImportModule(false);
      onDataChanged();
      if (imported) {
        setModuleMsg(`Indexed ${imported.cardCount} GitHub Catalog items (Portfolios, Teams & Repos)!`);
      }
    } catch (e) {
      setModuleMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const copySnippet = async (id: string, code: string) => {
    await navigator.clipboard?.writeText(code).catch(() => {});
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="w-full max-w-2xl max-h-[88vh] bg-zinc-950 border border-zinc-800 rounded-2xl shadow-2xl flex flex-col overflow-hidden">
        {/* Header Tabs */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-zinc-800">
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={() => setTab('general')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                tab === 'general' ? 'bg-indigo-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'
              }`}
            >
              <Sliders className="w-3.5 h-3.5" />
              <span>General</span>
            </button>
            <button
              type="button"
              onClick={() => setTab('modules')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                tab === 'modules' ? 'bg-indigo-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'
              }`}
            >
              <Layers className="w-3.5 h-3.5" />
              <span>Modules</span>
            </button>
            <button
              type="button"
              onClick={() => setTab('ai')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                tab === 'ai' ? 'bg-indigo-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'
              }`}
            >
              <Sparkles className="w-3.5 h-3.5" />
              <span>AI Assistant</span>
            </button>
            <button
              type="button"
              onClick={() => setTab('mcp')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                tab === 'mcp' ? 'bg-indigo-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'
              }`}
            >
              <Plug className="w-3.5 h-3.5" />
              <span>Integrations (MCP)</span>
            </button>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close settings"
            className="p-1 text-zinc-400 hover:text-zinc-100 rounded hover:bg-zinc-800"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Tab Body */}
        <div className="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
          {tab === 'general' && (
            <>
              {/* Global Hotkey */}
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 space-y-2">
                <div className="flex items-center gap-1.5 text-zinc-100 font-medium">
                  <Keyboard className="w-4 h-4 text-indigo-400" />
                  <span>Open / Hide Shortcut</span>
                </div>
                <p className="text-zinc-400 text-[11px]">
                  Press this key combination anywhere on your computer to show or hide Vex.
                </p>
                <div className="flex items-center gap-2 pt-0.5">
                  <input
                    value={recordingHotkey ? 'Press your key combo...' : hotkey}
                    onChange={(e) => setHotkey(e.target.value)}
                    onKeyDown={handleCaptureHotkey}
                    onBlur={() => setRecordingHotkey(false)}
                    placeholder="e.g. Ctrl+Shift+B"
                    className={`flex-1 bg-zinc-950 border rounded-lg px-2.5 py-1.5 font-mono text-zinc-100 focus:outline-none ${
                      recordingHotkey ? 'border-indigo-400 ring-1 ring-indigo-500/50' : 'border-zinc-800 focus:border-indigo-500'
                    }`}
                  />
                  <button
                    type="button"
                    onClick={() => setRecordingHotkey((v) => !v)}
                    className={`px-3 py-1.5 rounded-lg border text-xs transition ${
                      recordingHotkey
                        ? 'bg-indigo-600/20 border-indigo-500 text-indigo-200'
                        : 'bg-zinc-800 border-zinc-700 text-zinc-200 hover:bg-zinc-700'
                    }`}
                  >
                    {recordingHotkey ? 'Listening...' : 'Record'}
                  </button>
                  <button
                    type="button"
                    onClick={handleApplyHotkey}
                    className="px-3.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium"
                  >
                    Apply
                  </button>
                </div>
                {hotkeyMsg && <div className="text-[11px] text-emerald-400">{hotkeyMsg}</div>}
              </div>

              {/* Results Layout Selector */}
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 space-y-2.5">
                <div className="text-zinc-100 font-medium">Search Results Layout</div>
                <div className="grid grid-cols-2 gap-2.5">
                  <button
                    type="button"
                    onClick={() => void updatePref({ layoutMode: 'compact' })}
                    className={`text-left p-3 rounded-xl border transition ${
                      (prefs.layoutMode ?? 'compact') === 'compact'
                        ? 'bg-indigo-600/15 border-indigo-500/60 text-zinc-100'
                        : 'bg-zinc-950/60 border-zinc-800 text-zinc-400 hover:border-zinc-700'
                    }`}
                  >
                    <div className="font-semibold text-xs">Hero + Runner-Ups (Recommended)</div>
                    <p className="text-[11px] text-zinc-400 mt-1">
                      One primary hero card with perspective tabs plus compact runner-up rows below.
                    </p>
                  </button>
                  <button
                    type="button"
                    onClick={() => void updatePref({ layoutMode: 'split' })}
                    className={`text-left p-3 rounded-xl border transition ${
                      prefs.layoutMode === 'split'
                        ? 'bg-indigo-600/15 border-indigo-500/60 text-zinc-100'
                        : 'bg-zinc-950/60 border-zinc-800 text-zinc-400 hover:border-zinc-700'
                    }`}
                  >
                    <div className="font-semibold text-xs">Split Preview</div>
                    <p className="text-[11px] text-zinc-400 mt-1">
                      Side-by-side results list on the left and live preview pane on the right.
                    </p>
                  </button>
                </div>
              </div>

              {/* Pinned Categories (⌘2–⌘5) */}
              {availableCollections.length > 0 && (
                <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 space-y-2.5">
                  <div className="flex items-center justify-between">
                    <div className="text-zinc-100 font-medium">Pinned Categories (⌘2–⌘5)</div>
                    {(prefs.pinnedCategories?.length ?? 0) > 0 && (
                      <button
                        type="button"
                        onClick={() => void updatePref({ pinnedCategories: [] })}
                        className="text-[11px] text-zinc-400 hover:text-zinc-200 transition"
                      >
                        Reset defaults
                      </button>
                    )}
                  </div>
                  <p className="text-[11px] text-zinc-400">
                    Pin up to 4 categories for the shortcut bar alongside <kbd className="px-1 rounded bg-zinc-800 font-mono text-[10px]">⌘1</kbd> All.
                  </p>
                  {(prefs.pinnedCategories?.length ?? 0) > 0 && (
                    <div className="flex flex-wrap gap-1.5 pb-1">
                      {prefs.pinnedCategories!.map((col, idx) => (
                        <div
                          key={col}
                          className="inline-flex items-center gap-1 px-2 py-1 rounded-lg bg-indigo-600/20 border border-indigo-500/40 text-indigo-100 text-[11px]"
                        >
                          <span className="font-mono text-[10px] text-indigo-300">⌘{idx + 2}</span>
                          <span>{col}</span>
                          {idx > 0 && (
                            <button
                              type="button"
                              title="Move left"
                              onClick={() => {
                                const next = [...(prefs.pinnedCategories ?? [])];
                                [next[idx - 1], next[idx]] = [next[idx], next[idx - 1]];
                                void updatePref({ pinnedCategories: next });
                              }}
                              className="px-0.5 text-indigo-300 hover:text-white"
                            >
                              ←
                            </button>
                          )}
                          {idx < (prefs.pinnedCategories?.length ?? 0) - 1 && (
                            <button
                              type="button"
                              title="Move right"
                              onClick={() => {
                                const next = [...(prefs.pinnedCategories ?? [])];
                                [next[idx], next[idx + 1]] = [next[idx + 1], next[idx]];
                                void updatePref({ pinnedCategories: next });
                              }}
                              className="px-0.5 text-indigo-300 hover:text-white"
                            >
                              →
                            </button>
                          )}
                          <button
                            type="button"
                            title="Unpin"
                            onClick={() => {
                              const next = (prefs.pinnedCategories ?? []).filter((c) => c !== col);
                              void updatePref({ pinnedCategories: next });
                            }}
                            className="ml-0.5 text-indigo-300 hover:text-white"
                          >
                            ×
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                  <div className="flex flex-wrap gap-1.5">
                    {availableCollections.map((c) => {
                      const pinned = (prefs.pinnedCategories ?? []).includes(c.collection);
                      const atLimit = (prefs.pinnedCategories?.length ?? 0) >= 4;
                      return (
                        <button
                          key={c.collection}
                          type="button"
                          disabled={!pinned && atLimit}
                          onClick={() => {
                            const cur = prefs.pinnedCategories ?? [];
                            const next = pinned
                              ? cur.filter((x) => x !== c.collection)
                              : [...cur, c.collection].slice(0, 4);
                            void updatePref({ pinnedCategories: next });
                          }}
                          className={`px-2 py-1 rounded-lg border text-[11px] transition ${
                            pinned
                              ? 'bg-indigo-600/20 border-indigo-500/50 text-indigo-200'
                              : atLimit
                                ? 'bg-zinc-950/40 border-zinc-800/60 text-zinc-600 cursor-not-allowed'
                                : 'bg-zinc-950/70 border-zinc-800 text-zinc-300 hover:border-zinc-600'
                          }`}
                        >
                          {pinned ? '✓ ' : '+ '}
                          {c.collection} ({c.cardCount})
                        </button>
                      );
                    })}
                  </div>
                </div>
              )}

              {/* Launch at Login */}
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 flex items-center justify-between gap-3">
                <div>
                  <div className="text-zinc-100 font-medium">Launch at Login</div>
                  <p className="text-[11px] text-zinc-400 mt-0.5">
                    Start automatically in the system tray when you sign in.
                  </p>
                </div>
                <button
                  type="button"
                  role="switch"
                  aria-checked={launchAtLogin}
                  onClick={async () => {
                    const next = !launchAtLogin;
                    const applied = await bridge.setLaunchAtLogin(next).catch(() => launchAtLogin);
                    setLaunchAtLogin(applied);
                    onPrefsChange({ ...prefs, launchAtLogin: applied });
                  }}
                  className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors ${
                    launchAtLogin ? 'bg-indigo-600' : 'bg-zinc-700'
                  }`}
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      launchAtLogin ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>

              {/* Simple vs Power User Mode */}
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 space-y-2.5">
                <div className="text-zinc-100 font-medium">Interface Style</div>
                <div className="grid grid-cols-2 gap-2.5">
                  <button
                    type="button"
                    onClick={() => void updatePref({ powerUserMode: false })}
                    className={`text-left p-3 rounded-xl border transition ${
                      !prefs.powerUserMode
                        ? 'bg-indigo-600/15 border-indigo-500/60 text-zinc-100'
                        : 'bg-zinc-950/60 border-zinc-800 text-zinc-400 hover:border-zinc-700'
                    }`}
                  >
                    <div className="font-semibold text-xs">Simple Mode (Recommended)</div>
                    <p className="text-[11px] text-zinc-400 mt-1">
                      Clean, calm layout with plain-English labels and fewer buttons.
                    </p>
                  </button>
                  <button
                    type="button"
                    onClick={() => void updatePref({ powerUserMode: true })}
                    className={`text-left p-3 rounded-xl border transition ${
                      prefs.powerUserMode
                        ? 'bg-indigo-600/15 border-indigo-500/60 text-zinc-100'
                        : 'bg-zinc-950/60 border-zinc-800 text-zinc-400 hover:border-zinc-700'
                    }`}
                  >
                    <div className="font-semibold text-xs">Power User Mode</div>
                    <p className="text-[11px] text-zinc-400 mt-1">
                      Shows /modifiers, @collections, and match details.
                    </p>
                  </button>
                </div>
              </div>

              {/* No-Login App Update Checker */}
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 space-y-2.5">
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-1.5 text-zinc-100 font-medium">
                    <Download className="w-4 h-4 text-emerald-400" />
                    <span>App Updates (No Login Required)</span>
                  </div>
                  <span className="px-2 py-0.5 rounded-md bg-zinc-800 text-zinc-300 font-mono text-[10px]">
                    v{appVersion}
                  </span>
                </div>
                <p className="text-[11px] text-zinc-400">
                  Point to a hosted <code className="text-zinc-300">manifest.json</code> URL or shared network folder path to check for newer versions without logging in.
                </p>
                <div className="flex items-center gap-2">
                  <input
                    value={updateFeedUrl}
                    onChange={(e) => setUpdateFeedUrl(e.target.value)}
                    placeholder="https://intranet/vex/manifest.json or /Volumes/Share/manifest.json"
                    className="flex-1 bg-zinc-950 border border-zinc-800 rounded-lg px-2.5 py-1.5 font-mono text-[11px] text-zinc-100 focus:outline-none focus:border-indigo-500"
                  />
                  <button
                    type="button"
                    disabled={checkingUpdates}
                    onClick={async () => {
                      setCheckingUpdates(true);
                      setUpdateError(null);
                      setUpdateResult(null);
                      try {
                        await bridge.setUpdateFeedURL(updateFeedUrl);
                        const res = await bridge.checkForUpdates();
                        setUpdateResult(res);
                      } catch (e) {
                        setUpdateError(e instanceof Error ? e.message : String(e));
                      } finally {
                        setCheckingUpdates(false);
                      }
                    }}
                    className="px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium shrink-0 disabled:opacity-50 transition"
                  >
                    {checkingUpdates ? 'Checking...' : 'Check Now'}
                  </button>
                </div>

                {updateError && (
                  <div className="text-[11px] text-amber-300 bg-amber-500/10 border border-amber-500/25 rounded-lg px-2.5 py-1.5">
                    {updateError}
                  </div>
                )}

                {updateResult && (
                  <div
                    className={`rounded-lg border px-3 py-2 text-[11px] space-y-1 ${
                      updateResult.updateAvailable
                        ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-200'
                        : 'bg-zinc-950/60 border-zinc-800 text-zinc-300'
                    }`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium">
                        {updateResult.updateAvailable
                          ? `Update available: v${updateResult.latestVersion} (current: v${updateResult.currentVersion})`
                          : `You're up to date (v${updateResult.currentVersion})`}
                      </span>
                      {updateResult.updateAvailable && updateResult.downloadUrl && (
                        <a
                          href={updateResult.downloadUrl}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-emerald-600 hover:bg-emerald-500 text-white font-medium"
                        >
                          <span>Download</span>
                          <ExternalLink className="w-3 h-3" />
                        </a>
                      )}
                    </div>
                    {updateResult.notes && <p className="text-zinc-400">{updateResult.notes}</p>}
                  </div>
                )}
              </div>

              {/* Welcome Tour */}
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 flex items-center justify-between gap-4">
                <div>
                  <div className="text-zinc-100 font-medium">Welcome Guide</div>
                  <p className="text-[11px] text-zinc-400 mt-0.5">
                    Want a quick refresher on your shortcut, goals, and buttons?
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    onClose();
                    onReplayTour();
                  }}
                  className="flex items-center gap-1.5 px-3.5 py-2 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-100 font-medium shrink-0 transition"
                >
                  <Compass className="w-3.5 h-3.5 text-indigo-400" />
                  <span>Replay Welcome Tour</span>
                </button>
              </div>

              {/* Local Logs & Observability Diagnostics (Loki JSONL + Prometheus /metrics) */}
              {logInfo && (
                <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 space-y-2.5">
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-1.5 text-zinc-100 font-medium">
                      <FileText className="w-4 h-4 text-indigo-400" />
                      <span>Local Logs &amp; Metrics (Loki / Prometheus Ready)</span>
                    </div>
                    <select
                      value={logInfo.level}
                      onChange={async (e) => {
                        const lvl = await bridge.setLogLevel(e.target.value);
                        setLogInfo({ ...logInfo, level: lvl });
                      }}
                      className="bg-zinc-950 border border-zinc-800 rounded-lg px-2 py-1 text-[11px] text-zinc-200 font-mono"
                    >
                      <option value="debug">level: debug</option>
                      <option value="info">level: info</option>
                      <option value="warn">level: warn</option>
                      <option value="error">level: error</option>
                    </select>
                  </div>

                  <div className="text-[11px] text-zinc-400 font-mono truncate">
                    JSONL: <span className="text-zinc-300">{logInfo.filePath}</span>
                  </div>
                  {logInfo.metricsAddr && (
                    <div className="text-[11px] text-zinc-400 font-mono truncate">
                      Prometheus: <span className="text-emerald-300">{logInfo.metricsAddr}</span>
                    </div>
                  )}

                  <div className="flex items-center gap-2 pt-0.5">
                    <button
                      type="button"
                      onClick={async () => {
                        const next = !showLogs;
                        setShowLogs(next);
                        if (next) {
                          const list = await bridge.getRecentLogs(30);
                          setRecentLogs(list);
                        }
                      }}
                      className="px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-200 text-[11px] font-medium transition"
                    >
                      {showLogs ? 'Hide Recent Logs' : 'View Recent Logs'}
                    </button>
                    <button
                      type="button"
                      onClick={() => void copySnippet('log-path', logInfo.filePath)}
                      className="px-2.5 py-1.5 rounded-lg bg-zinc-800/80 hover:bg-zinc-700 text-zinc-300 text-[11px] transition"
                    >
                      {copiedId === 'log-path' ? 'Copied Path' : 'Copy Log Path'}
                    </button>
                    {showLogs && (
                      <button
                        type="button"
                        onClick={async () => {
                          await bridge.clearLogs();
                          setRecentLogs([]);
                        }}
                        className="px-2.5 py-1.5 rounded-lg bg-zinc-800/80 hover:bg-red-950/60 text-zinc-400 hover:text-red-300 text-[11px] transition ml-auto"
                      >
                        Clear
                      </button>
                    )}
                  </div>

                  {showLogs && (
                    <div className="rounded-lg border border-zinc-800 bg-zinc-950/90 max-h-40 overflow-y-auto divide-y divide-zinc-800/70 font-mono text-[10px]">
                      {recentLogs.length === 0 ? (
                        <div className="p-2.5 text-zinc-500">No log entries yet.</div>
                      ) : (
                        recentLogs.map((entry, idx) => (
                          <div key={idx} className="px-2.5 py-1.5 flex items-center gap-2">
                            <span
                              className={`px-1 py-0.5 rounded text-[9px] shrink-0 ${
                                entry.level === 'ERROR'
                                  ? 'bg-red-500/20 text-red-300'
                                  : entry.level === 'WARN'
                                    ? 'bg-amber-500/20 text-amber-300'
                                    : 'bg-indigo-500/15 text-indigo-300'
                              }`}
                            >
                              {entry.level}
                            </span>
                            {entry.component && (
                              <span className="text-zinc-500 shrink-0">[{entry.component}]</span>
                            )}
                            <span className="text-zinc-200 truncate flex-1">{entry.msg}</span>
                          </div>
                        ))
                      )}
                    </div>
                  )}
                </div>
              )}
            </>
          )}

          {tab === 'modules' && (
            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <div className="text-zinc-100 font-medium">Knowledge Modules &amp; Overlays</div>
                  <p className="text-[11px] text-zinc-400 mt-0.5">
                    Toggle modules on or off to keep your results focused. Modules can add standalone cards and enrich existing cards (like AMS).
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => setShowImportModule((v) => !v)}
                  className="flex items-center gap-1 px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-100 font-medium shrink-0 transition"
                >
                  <Plus className="w-3.5 h-3.5 text-indigo-400" />
                  <span>Import Module</span>
                </button>
              </div>

              {moduleMsg && (
                <div className="rounded-lg bg-indigo-500/10 border border-indigo-500/30 text-indigo-200 px-3 py-2">
                  {moduleMsg}
                </div>
              )}

              {showImportModule && (
                <div className="rounded-xl border border-indigo-500/30 bg-indigo-500/5 p-3.5 space-y-2.5">
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-zinc-200">Import Shared Module (.json)</span>
                    <label className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-zinc-900 border border-zinc-700 hover:border-zinc-500 cursor-pointer text-zinc-200">
                      <Upload className="w-3 h-3" />
                      <span>Choose File</span>
                      <input
                        type="file"
                        accept=".json"
                        className="hidden"
                        onChange={(e) => {
                          const file = e.target.files?.[0];
                          if (!file) return;
                          const reader = new FileReader();
                          reader.onload = () => {
                            if (typeof reader.result === 'string') setImportModuleRaw(reader.result);
                          };
                          reader.readAsText(file);
                        }}
                      />
                    </label>
                  </div>
                  <textarea
                    rows={4}
                    value={importModuleRaw}
                    onChange={(e) => setImportModuleRaw(e.target.value)}
                    placeholder='Paste shared ModuleManifest JSON here ({"id":"my-team","name":"Team Pack","cards":[...],"overlays":[...]})...'
                    className="w-full bg-zinc-950 border border-zinc-800 rounded-lg p-2.5 font-mono text-[11px] text-zinc-100"
                  />
                  <div className="flex justify-end gap-2">
                    <button
                      type="button"
                      onClick={() => setShowImportModule(false)}
                      className="px-3 py-1.5 rounded-lg bg-zinc-800 text-zinc-300"
                    >
                      Cancel
                    </button>
                    <button
                      type="button"
                      disabled={!importModuleRaw.trim()}
                      onClick={() => void handleImportGitHubRepos()}
                      className="px-3 py-1.5 rounded-lg bg-emerald-600/25 hover:bg-emerald-600/35 border border-emerald-500/40 text-emerald-200 font-medium disabled:opacity-50"
                      title="Parse JSON array from `gh repo list --json name,description,url,primaryLanguage` using gt-portfolio-team-product-component taxonomy"
                    >
                      Import as GitHub Repo List
                    </button>
                    <button
                      type="button"
                      disabled={!importModuleRaw.trim()}
                      onClick={() => void handleImportModule()}
                      className="px-3.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium disabled:opacity-50"
                    >
                      Save &amp; Enable Module
                    </button>
                  </div>
                </div>
              )}

              <div className="space-y-2">
                {modules.map((mod) => (
                  <div
                    key={mod.id}
                    className={`rounded-xl border p-3.5 transition flex items-start justify-between gap-3 ${
                      mod.enabled
                        ? 'bg-zinc-900/70 border-indigo-500/40'
                        : 'bg-zinc-900/30 border-zinc-800/80 opacity-75'
                    }`}
                  >
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="font-semibold text-zinc-100">{mod.name}</span>
                        <span className="px-2 py-0.5 rounded-md bg-zinc-800 text-zinc-400 text-[10px]">
                          {mod.category}
                        </span>
                        {mod.cardCount > 0 && (
                          <span className="px-1.5 py-0.5 rounded bg-indigo-500/15 text-indigo-300 text-[10px]">
                            {mod.cardCount} cards
                          </span>
                        )}
                        {mod.overlayCount > 0 && (
                          <span className="px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-300 text-[10px]">
                            +{mod.overlayCount} enrichments
                          </span>
                        )}
                        {mod.localOnly && (
                          <span className="px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-300 border border-amber-500/25 text-[10px]">
                            Local Only
                          </span>
                        )}
                      </div>
                      <p className="text-[11px] text-zinc-400 leading-relaxed">{mod.description}</p>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      <button
                        type="button"
                        onClick={() => void handleShareModule(mod.id)}
                        title="Copy shareable module JSON to clipboard"
                        className="flex items-center gap-1 px-2 py-1 rounded-lg bg-zinc-800/80 hover:bg-zinc-700 text-zinc-300 text-[11px] transition"
                      >
                        {copiedModuleId === mod.id ? (
                          <>
                            <Check className="w-3 h-3 text-emerald-400" />
                            <span>Copied</span>
                          </>
                        ) : (
                          <>
                            <Share2 className="w-3 h-3" />
                            <span>Share</span>
                          </>
                        )}
                      </button>

                      <button
                        type="button"
                        role="switch"
                        aria-checked={mod.enabled}
                        onClick={() => void handleToggleModule(mod.id)}
                        className={`w-10 h-6 rounded-full transition-colors p-0.5 shrink-0 ${
                          mod.enabled ? 'bg-indigo-600' : 'bg-zinc-700'
                        }`}
                      >
                        <div
                          className={`w-5 h-5 rounded-full bg-white transition-transform ${
                            mod.enabled ? 'translate-x-4' : 'translate-x-0'
                          }`}
                        />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {tab === 'ai' && (
            <>
              {/* Enable / Disable AI Assistant */}
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3.5 flex items-center justify-between gap-4">
                <div>
                  <div className="text-zinc-100 font-medium">Show &quot;Ask AI&quot; in Launcher</div>
                  <p className="text-[11px] text-zinc-400 mt-0.5">
                    Turn this on if you want to ask questions about your saved notes and links.
                  </p>
                </div>
                <button
                  type="button"
                  role="switch"
                  aria-checked={prefs.aiEnabled}
                  onClick={() => void updatePref({ aiEnabled: !prefs.aiEnabled })}
                  className={`w-10 h-6 rounded-full transition-colors p-0.5 shrink-0 ${
                    prefs.aiEnabled ? 'bg-indigo-600' : 'bg-zinc-700'
                  }`}
                >
                  <div
                    className={`w-5 h-5 rounded-full bg-white transition-transform ${
                      prefs.aiEnabled ? 'translate-x-4' : 'translate-x-0'
                    }`}
                  />
                </button>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <label className="space-y-1">
                  <span className="text-zinc-400">AI Service</span>
                  <select
                    value={cfg.provider}
                    onChange={(e) => {
                      const p = e.target.value as AIProviderType;
                      setCfg({ ...cfg, provider: p, ...PROVIDER_DEFAULTS[p] });
                    }}
                    className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100"
                  >
                    <option value="ollama">Free &amp; Local (Ollama on this PC)</option>
                    <option value="openai">OpenAI / GitHub Models / LM Studio</option>
                    <option value="anthropic">Anthropic Claude</option>
                    <option value="gemini">Google Gemini</option>
                  </select>
                </label>
                <label className="space-y-1">
                  <span className="text-zinc-400">Server Address (Base URL)</span>
                  <input
                    value={cfg.baseUrl}
                    onChange={(e) => setCfg({ ...cfg, baseUrl: e.target.value })}
                    className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100 font-mono"
                  />
                </label>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <label className="space-y-1">
                  <span className="text-zinc-400">Chat Model</span>
                  <input
                    value={cfg.chatModel}
                    onChange={(e) => setCfg({ ...cfg, chatModel: e.target.value })}
                    className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100 font-mono"
                  />
                </label>
                <label className="space-y-1">
                  <span className="text-zinc-400">Smart Search Model (optional)</span>
                  <input
                    value={cfg.embeddingModel ?? ''}
                    onChange={(e) => setCfg({ ...cfg, embeddingModel: e.target.value })}
                    placeholder="nomic-embed-text"
                    className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100 font-mono"
                  />
                </label>
              </div>

              <label className="block space-y-1">
                <span className="text-zinc-400">API Key (not needed for local Ollama)</span>
                <input
                  type="password"
                  value={cfg.apiKey ?? ''}
                  onChange={(e) => setCfg({ ...cfg, apiKey: e.target.value })}
                  placeholder="sk-... or github_pat_..."
                  className="w-full bg-zinc-900 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-zinc-100 font-mono"
                />
              </label>

              {statusMsg && (
                <div className="rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 px-3 py-2">
                  {statusMsg}
                </div>
              )}

              <div className="flex items-center justify-between pt-2">
                <button
                  type="button"
                  disabled={syncing}
                  onClick={handleSyncEmbeddings}
                  className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-200 disabled:opacity-50"
                >
                  <RefreshCw className={`w-3.5 h-3.5 ${syncing ? 'animate-spin' : ''}`} />
                  <span>Refresh Smart Search Index</span>
                </button>
                <button
                  type="button"
                  onClick={handleSaveAI}
                  className="px-4 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium"
                >
                  Save AI Settings
                </button>
              </div>
            </>
          )}

          {tab === 'mcp' && (
            <>
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 p-3 text-zinc-400 leading-relaxed">
                <span className="text-zinc-200 font-medium">Optional developer integrations:</span>{' '}
                Connect Vex to GitHub Copilot, VS Code, Claude Desktop, or Cursor
                so those tools can search your local notes and links.
              </div>

              <div className="flex items-center justify-between">
                <span className="text-zinc-400">
                  Local MCP Address: <code className="text-emerald-300">http://127.0.0.1:17380/mcp</code>
                </span>
                <div className="flex rounded-lg bg-zinc-900 border border-zinc-800 p-0.5">
                  <button
                    type="button"
                    onClick={() => setOsTarget('windows')}
                    className={`px-2 py-0.5 rounded text-[11px] ${osTarget === 'windows' ? 'bg-indigo-600 text-white' : 'text-zinc-400'}`}
                  >
                    Windows
                  </button>
                  <button
                    type="button"
                    onClick={() => setOsTarget('macos')}
                    className={`px-2 py-0.5 rounded text-[11px] ${osTarget === 'macos' ? 'bg-indigo-600 text-white' : 'text-zinc-400'}`}
                  >
                    macOS
                  </button>
                </div>
              </div>

              <div className="space-y-2.5">
                {snippets.map((s) => (
                  <div key={s.id} className="rounded-lg border border-zinc-800 bg-zinc-900/70 overflow-hidden">
                    <div className="flex items-center justify-between px-3 py-1.5 border-b border-zinc-800 bg-zinc-900">
                      <span className="font-medium text-zinc-200">{s.title}</span>
                      <button
                        type="button"
                        onClick={() => copySnippet(s.id, s.code)}
                        className="flex items-center gap-1 text-zinc-400 hover:text-zinc-100"
                      >
                        {copiedId === s.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                        <span>{copiedId === s.id ? 'Copied' : 'Copy'}</span>
                      </button>
                    </div>
                    <pre className="p-2.5 text-[11px] font-mono text-zinc-300 overflow-x-auto">{s.code}</pre>
                  </div>
                ))}
              </div>

              {mcpServers.length > 0 && (
                <div className="space-y-1.5 pt-2">
                  <div className="text-[10px] uppercase tracking-wider text-zinc-500">Connected External MCP Servers</div>
                  {mcpServers.map((srv) => (
                    <div key={srv.config.id} className="flex items-center justify-between px-3 py-2 rounded-lg bg-zinc-900 border border-zinc-800">
                      <div>
                        <div className="font-medium text-zinc-100">{srv.config.name}</div>
                        <div className="text-[11px] text-zinc-400 font-mono">
                          @mcp:{srv.config.id} • {srv.toolCount} tools • {srv.resourceCount} resources
                        </div>
                      </div>
                      <span
                        className={`px-2 py-0.5 rounded text-[10px] ${
                          srv.connected ? 'bg-emerald-500/15 text-emerald-300' : 'bg-red-500/15 text-red-300'
                        }`}
                      >
                        {srv.connected ? 'Connected' : 'Disconnected'}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
