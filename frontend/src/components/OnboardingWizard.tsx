import { useEffect, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  Check,
  Globe,
  HelpCircle,
  Keyboard,
  Layers,
  MousePointerClick,
  Plus,
  Search,
  Sparkles,
  Terminal,
} from 'lucide-react';
import { bridge } from '../lib/bridge';
import type { AIProviderType, ModuleStatus, UserPrefs } from '../types/models';

interface OnboardingWizardProps {
  initialHotkey: string;
  initialPrefs: UserPrefs;
  onComplete: (prefs: UserPrefs, hotkey: string) => void;
}

type GoalId = 'links' | 'notes' | 'ai' | 'power';
type AISetupChoice = 'ollama' | 'cloud' | 'later';

const CLOUD_DEFAULTS: Record<Exclude<AIProviderType, 'ollama'>, { label: string; baseUrl: string; chatModel: string; embeddingModel: string }> = {
  openai: {
    label: 'OpenAI / GitHub Models',
    baseUrl: 'https://api.openai.com/v1',
    chatModel: 'gpt-4o-mini',
    embeddingModel: 'text-embedding-3-small',
  },
  gemini: {
    label: 'Google Gemini',
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
    chatModel: 'gemini-2.5-flash',
    embeddingModel: 'text-embedding-004',
  },
  anthropic: {
    label: 'Anthropic Claude',
    baseUrl: 'https://api.anthropic.com/v1',
    chatModel: 'claude-3-5-sonnet-latest',
    embeddingModel: '',
  },
};

export function OnboardingWizard({ initialHotkey, initialPrefs, onComplete }: OnboardingWizardProps) {
  const [step, setStep] = useState<1 | 2 | 3>(1);

  // Step 1: Hotkey
  const [hotkey, setHotkey] = useState(initialHotkey || 'Ctrl+Shift+B');
  const [recording, setRecording] = useState(false);
  const [hotkeyStatus, setHotkeyStatus] = useState<string | null>(null);
  const [hotkeyError, setHotkeyError] = useState<string | null>(null);

  // Step 2: Goals & optional AI
  const [goals, setGoals] = useState<GoalId[]>(() => {
    const saved = (initialPrefs.selectedGoals ?? []).filter((g): g is GoalId =>
      ['links', 'notes', 'ai', 'power'].includes(g),
    );
    return saved.length > 0 ? saved : ['links', 'notes'];
  });
  const [aiChoice, setAiChoice] = useState<AISetupChoice>(initialPrefs.aiEnabled ? 'ollama' : 'later');
  const [cloudProvider, setCloudProvider] = useState<Exclude<AIProviderType, 'ollama'>>('openai');
  const [cloudApiKey, setCloudApiKey] = useState('');
  const [modules, setModules] = useState<ModuleStatus[]>([]);
  const [enabledModuleIds, setEnabledModuleIds] = useState<string[]>(
    initialPrefs.enabledModules ?? ['geographies', 'air-freight', 'ocean-freight', 'customs'],
  );
  const [launchAtLogin, setLaunchAtLogin] = useState<boolean>(
    Boolean(initialPrefs.launchAtLogin),
  );

  const [saving, setSaving] = useState(false);

  useEffect(() => {
    bridge.listModules().then((list) => {
      if (list && list.length > 0) {
        setModules(list);
        setEnabledModuleIds(list.filter((m) => m.enabled).map((m) => m.id));
      }
    }).catch(() => {});
  }, []);

  const toggleModule = (id: string) => {
    setEnabledModuleIds((prev) =>
      prev.includes(id) ? prev.filter((m) => m !== id) : [...prev, id],
    );
  };

  const toggleGoal = (id: GoalId) => {
    setGoals((prev) => {
      if (prev.includes(id)) {
        return prev.filter((g) => g !== id);
      }
      if (id === 'ai' && aiChoice === 'later') {
        setAiChoice('ollama');
      }
      return [...prev, id];
    });
  };

  const handleCaptureHotkey = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (!recording) return;
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

    const combo = parts.join('+');
    setHotkey(combo);
    setRecording(false);
    setHotkeyError(null);
    setHotkeyStatus('Press "Save Shortcut" or continue to apply.');
  };

  const applyHotkeyIfChanged = async (): Promise<string> => {
    setHotkeyError(null);
    const trimmed = hotkey.trim();
    if (!trimmed) return initialHotkey;
    try {
      const applied = await bridge.setHotkey(trimmed);
      setHotkey(applied);
      setHotkeyStatus(`Shortcut ready: ${applied}`);
      return applied;
    } catch (e) {
      setHotkeyError(e instanceof Error ? e.message : String(e));
      return initialHotkey;
    }
  };

  const handleFinish = async (skipDefaults = false) => {
    setSaving(true);
    try {
      const finalHotkey = await applyHotkeyIfChanged();
      const wantAI = !skipDefaults && goals.includes('ai') && aiChoice !== 'later';
      const wantPower = !skipDefaults && goals.includes('power');

      if (wantAI) {
        if (aiChoice === 'ollama') {
          await bridge.setAIConfig({
            provider: 'ollama',
            baseUrl: 'http://localhost:11434',
            chatModel: 'llama3.2',
            embeddingModel: 'nomic-embed-text',
            temperature: 0.2,
            maxContextCards: 8,
          });
        } else if (aiChoice === 'cloud') {
          const preset = CLOUD_DEFAULTS[cloudProvider];
          await bridge.setAIConfig({
            provider: cloudProvider,
            baseUrl: preset.baseUrl,
            apiKey: cloudApiKey.trim(),
            chatModel: preset.chatModel,
            embeddingModel: preset.embeddingModel,
            temperature: 0.2,
            maxContextCards: 8,
          });
        }
      }

      const nextPrefs: UserPrefs = {
        onboardingCompleted: true,
        powerUserMode: wantPower,
        aiEnabled: wantAI,
        launchAtLogin,
        layoutMode: initialPrefs.layoutMode ?? 'compact',
        selectedGoals: skipDefaults ? ['links', 'notes'] : goals,
        enabledModules: enabledModuleIds,
      };
      await bridge.setEnabledModules(enabledModuleIds).catch(() => {});
      await bridge.setLaunchAtLogin(launchAtLogin).catch(() => {});
      await bridge.setUserPrefs(nextPrefs);
      onComplete(nextPrefs, finalHotkey);
    } finally {
      setSaving(false);
    }
  };

  const goalCards: { id: GoalId; title: string; desc: string; icon: typeof Globe; badge?: string }[] = [
    {
      id: 'links',
      title: 'Everyday Websites & Apps',
      desc: 'Open your favorite sites, portals, and apps in one click.',
      icon: Globe,
    },
    {
      id: 'notes',
      title: 'Notes, Info & Spreadsheets',
      desc: 'Keep quick notes, reference details, or import a .csv file.',
      icon: BookOpen,
    },
    {
      id: 'ai',
      title: 'Personal AI Helper',
      desc: 'Ask questions in plain English about your saved notes and links.',
      icon: Sparkles,
      badge: 'Optional',
    },
    {
      id: 'power',
      title: 'Power User & Developer Tools',
      desc: 'Show /modifiers, @collections, JSON fields, and MCP integrations.',
      icon: Terminal,
      badge: 'Advanced',
    },
  ];

  return (
    <div className="fixed inset-0 z-50 bg-zinc-950/98 backdrop-blur-xl flex flex-col">
      {/* Top Progress Bar */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-zinc-800/80">
        <div className="flex items-center gap-2.5">
          <div className="w-7 h-7 rounded-lg bg-indigo-600/20 border border-indigo-500/40 flex items-center justify-center text-indigo-300 font-semibold text-xs">
            V
          </div>
          <div>
            <div className="text-sm font-semibold text-zinc-100">Vex Setup</div>
            <div className="text-[11px] text-zinc-400">Step {step} of 3</div>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {[1, 2, 3].map((s) => (
            <div
              key={s}
              className={`h-1.5 rounded-full transition-all ${
                s === step ? 'w-8 bg-indigo-500' : s < step ? 'w-4 bg-indigo-500/50' : 'w-4 bg-zinc-800'
              }`}
            />
          ))}
        </div>

        <button
          type="button"
          disabled={saving}
          onClick={() => void handleFinish(true)}
          className="text-xs text-zinc-400 hover:text-zinc-200 transition"
        >
          Skip setup
        </button>
      </div>

      {/* Main Step Content */}
      <div className="flex-1 overflow-y-auto px-8 py-6 flex flex-col justify-center">
        {step === 1 && (
          <div className="max-w-xl mx-auto w-full space-y-6">
            <div className="space-y-1.5 text-center">
              <h1 className="text-2xl font-semibold text-zinc-100 tracking-tight">
                Vex
              </h1>
              <p className="text-sm text-zinc-400">
                Choose your shortcut to get started.
              </p>
            </div>

            <div className="rounded-2xl border border-zinc-800 bg-zinc-900/60 p-5 space-y-3">
              <div className="flex items-center gap-2 text-sm font-medium text-zinc-100">
                <Keyboard className="w-4 h-4 text-indigo-400" />
                <span>Open / hide shortcut</span>
              </div>
              <p className="text-xs text-zinc-400">
                Press this combination anytime to toggle Vex.
              </p>

              <div className="flex items-center gap-2 pt-1">
                <input
                  value={recording ? 'Press your key combo now...' : hotkey}
                  onChange={(e) => setHotkey(e.target.value)}
                  onKeyDown={handleCaptureHotkey}
                  onBlur={() => setRecording(false)}
                  className={`flex-1 bg-zinc-950 border rounded-xl px-3.5 py-2.5 text-sm font-mono text-zinc-100 focus:outline-none ${
                    recording
                      ? 'border-indigo-400 ring-2 ring-indigo-500/30'
                      : 'border-zinc-700/80 focus:border-indigo-500'
                  }`}
                />
                <button
                  type="button"
                  onClick={() => setRecording((v) => !v)}
                  className={`px-3.5 py-2.5 rounded-xl border text-xs font-medium transition ${
                    recording
                      ? 'bg-indigo-600/20 border-indigo-500 text-indigo-200'
                      : 'bg-zinc-800 border-zinc-700 text-zinc-200 hover:bg-zinc-700'
                  }`}
                >
                  {recording ? 'Listening...' : 'Record Keys'}
                </button>
                <button
                  type="button"
                  onClick={() => void applyHotkeyIfChanged()}
                  className="px-3.5 py-2.5 rounded-xl bg-zinc-800 hover:bg-zinc-700 text-zinc-100 text-xs font-medium border border-zinc-700 transition"
                >
                  Save Shortcut
                </button>
              </div>

              {hotkeyStatus && <div className="text-xs text-emerald-400">{hotkeyStatus}</div>}
              {hotkeyError && <div className="text-xs text-red-400">{hotkeyError}</div>}
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="max-w-2xl mx-auto w-full space-y-4">
            <div className="space-y-1 text-center">
              <h2 className="text-xl font-semibold text-zinc-100">
                What do you want to keep handy?
              </h2>
              <p className="text-xs text-zinc-400">
                We&apos;ll keep your interface clean and only show the buttons you actually need. You
                can change this anytime in Settings.
              </p>
            </div>

            <div className="grid grid-cols-2 gap-3">
              {goalCards.map((g) => {
                const Icon = g.icon;
                const selected = goals.includes(g.id);
                return (
                  <button
                    key={g.id}
                    type="button"
                    onClick={() => toggleGoal(g.id)}
                    className={`text-left p-3.5 rounded-xl border transition flex items-start gap-3 ${
                      selected
                        ? 'bg-indigo-600/15 border-indigo-500/60 text-zinc-100'
                        : 'bg-zinc-900/50 border-zinc-800 text-zinc-300 hover:border-zinc-700'
                    }`}
                  >
                    <div
                      className={`w-8 h-8 rounded-lg flex items-center justify-center shrink-0 mt-0.5 ${
                        selected ? 'bg-indigo-500/30 text-indigo-200' : 'bg-zinc-800 text-zinc-400'
                      }`}
                    >
                      <Icon className="w-4 h-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-xs font-semibold">{g.title}</span>
                        <span
                          className={`w-4 h-4 rounded-full flex items-center justify-center shrink-0 border ${
                            selected
                              ? 'bg-indigo-500 border-indigo-400 text-white'
                              : 'border-zinc-700'
                          }`}
                        >
                          {selected && <Check className="w-2.5 h-2.5" />}
                        </span>
                      </div>
                      <p className="text-[11px] text-zinc-400 mt-1 leading-relaxed">{g.desc}</p>
                      {g.badge && (
                        <span className="inline-block mt-1.5 px-1.5 py-0.5 text-[10px] rounded bg-zinc-800 text-zinc-400">
                          {g.badge}
                        </span>
                      )}
                    </div>
                  </button>
                );
              })}
            </div>

            {modules.length > 0 && (
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/45 p-3.5 space-y-2">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-1.5 text-xs font-medium text-zinc-200">
                    <Layers className="w-3.5 h-3.5 text-indigo-400" />
                    <span>Optional Knowledge Modules (turn on what fits your role)</span>
                  </div>
                  <span className="text-[10px] text-zinc-500">Changeable anytime in Settings → Modules</span>
                </div>
                <div className="grid grid-cols-3 gap-2">
                  {modules.map((mod) => {
                    const active = enabledModuleIds.includes(mod.id);
                    return (
                      <button
                        type="button"
                        key={mod.id}
                        onClick={() => toggleModule(mod.id)}
                        className={`text-left px-2.5 py-2 rounded-lg border text-xs transition flex items-center justify-between gap-2 ${
                          active
                            ? 'bg-indigo-600/20 border-indigo-500/50 text-zinc-100'
                            : 'bg-zinc-950/60 border-zinc-800 text-zinc-400 hover:border-zinc-700'
                        }`}
                      >
                        <span className="truncate font-medium text-[11px]">{mod.name}</span>
                        <span
                          className={`w-3.5 h-3.5 rounded flex items-center justify-center shrink-0 border ${
                            active ? 'bg-indigo-500 border-indigo-400 text-white' : 'border-zinc-700'
                          }`}
                        >
                          {active && <Check className="w-2.5 h-2.5" />}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            )}

            {goals.includes('ai') && (
              <div className="rounded-xl border border-purple-500/30 bg-purple-500/5 p-3.5 space-y-2.5">
                <div className="text-xs font-medium text-purple-200">
                  How would you like to power your AI Helper?
                </div>
                <div className="grid grid-cols-3 gap-2">
                  {[
                    { id: 'ollama' as const, label: 'Free & Private (Ollama)', sub: 'Runs locally on your PC' },
                    { id: 'cloud' as const, label: 'Cloud AI Key', sub: 'OpenAI, Gemini, or Claude' },
                    { id: 'later' as const, label: 'Set up later', sub: 'Hide AI button for now' },
                  ].map((opt) => (
                    <button
                      key={opt.id}
                      type="button"
                      onClick={() => setAiChoice(opt.id)}
                      className={`text-left px-3 py-2 rounded-lg border text-xs transition ${
                        aiChoice === opt.id
                          ? 'bg-purple-600/25 border-purple-400/60 text-white'
                          : 'bg-zinc-900/70 border-zinc-800 text-zinc-300 hover:border-zinc-700'
                      }`}
                    >
                      <div className="font-medium">{opt.label}</div>
                      <div className="text-[10px] text-zinc-400 mt-0.5">{opt.sub}</div>
                    </button>
                  ))}
                </div>

                {aiChoice === 'cloud' && (
                  <div className="grid grid-cols-2 gap-2 pt-1">
                    <select
                      value={cloudProvider}
                      onChange={(e) => setCloudProvider(e.target.value as Exclude<AIProviderType, 'ollama'>)}
                      className="bg-zinc-950 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-xs text-zinc-100"
                    >
                      <option value="openai">OpenAI / GitHub Models</option>
                      <option value="gemini">Google Gemini</option>
                      <option value="anthropic">Anthropic Claude</option>
                    </select>
                    <input
                      type="password"
                      value={cloudApiKey}
                      onChange={(e) => setCloudApiKey(e.target.value)}
                      placeholder="Paste API key (optional now)"
                      className="bg-zinc-950 border border-zinc-800 rounded-lg px-2.5 py-1.5 text-xs text-zinc-100 font-mono"
                    />
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {step === 3 && (
          <div className="max-w-2xl mx-auto w-full space-y-5">
            <div className="space-y-1 text-center">
              <h2 className="text-xl font-semibold text-zinc-100">
                3 quick things to know
              </h2>
              <p className="text-xs text-zinc-400">
                Everything is designed to feel effortless. Here&apos;s how each part works:
              </p>
            </div>

            <div className="grid grid-cols-3 gap-3">
              <div className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-4 space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-indigo-500/20 text-indigo-300 flex items-center justify-center">
                  <Search className="w-4 h-4" />
                </div>
                <div className="text-xs font-semibold text-zinc-100">1. Just Type</div>
                <p className="text-[11px] text-zinc-400 leading-relaxed">
                  Start typing any word — a site name, note title, or keyword. No special codes
                  required.
                </p>
              </div>

              <div className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-4 space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-emerald-500/20 text-emerald-300 flex items-center justify-center">
                  <MousePointerClick className="w-4 h-4" />
                </div>
                <div className="text-xs font-semibold text-zinc-100">2. Click or Press Enter</div>
                <p className="text-[11px] text-zinc-400 leading-relaxed">
                  Select an item on the left to preview it, then press{' '}
                  <kbd className="px-1 py-0.5 rounded bg-zinc-800 text-zinc-200 font-mono text-[10px]">
                    Enter
                  </kbd>{' '}
                  or click the blue button.
                </p>
              </div>

              <div className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-4 space-y-2.5">
                <div className="w-8 h-8 rounded-lg bg-purple-500/20 text-purple-300 flex items-center justify-center">
                  <Plus className="w-4 h-4" />
                </div>
                <div className="text-xs font-semibold text-zinc-100">3. Add &amp; Help Anytime</div>
                <p className="text-[11px] text-zinc-400 leading-relaxed">
                  Use <strong className="text-zinc-200">+ Add</strong> to save a link, note, or
                  spreadsheet. Click{' '}
                  <HelpCircle className="inline w-3 h-3 text-zinc-300 -mt-0.5" /> in the bottom
                  corner anytime for a refresher.
                </p>
              </div>
            </div>

            <label className="rounded-xl border border-zinc-800/80 bg-zinc-900/50 px-4 py-3 flex items-center justify-between gap-3 cursor-pointer">
              <div className="text-left">
                <div className="text-xs font-medium text-zinc-100">Launch at login</div>
                <div className="text-[11px] text-zinc-400">
                  Start Vex in the background when you sign in to your computer.
                </div>
              </div>
              <input
                type="checkbox"
                checked={launchAtLogin}
                onChange={(e) => setLaunchAtLogin(e.target.checked)}
                className="w-4 h-4 accent-indigo-500 rounded cursor-pointer"
              />
            </label>

            <div className="rounded-xl border border-zinc-800/80 bg-zinc-900/40 px-4 py-3 text-center text-xs text-zinc-400">
              Tip: Press{' '}
              <kbd className="px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-200 font-mono text-[11px]">
                Esc
              </kbd>{' '}
              or click <strong className="text-zinc-200">×</strong> to hide Vex, and{' '}
              <kbd className="px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-200 font-mono text-[11px]">
                {hotkey}
              </kbd>{' '}
              to bring it right back.
            </div>
          </div>
        )}
      </div>

      {/* Bottom Navigation */}
      <div className="flex items-center justify-between px-6 py-4 border-t border-zinc-800/80 bg-zinc-900/40">
        {step > 1 ? (
          <button
            type="button"
            onClick={() => setStep((s) => (s === 3 ? 2 : 1))}
            className="flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-zinc-800 hover:bg-zinc-700 text-zinc-200 text-xs font-medium transition"
          >
            <ArrowLeft className="w-3.5 h-3.5" />
            <span>Back</span>
          </button>
        ) : (
          <div />
        )}

        {step < 3 ? (
          <button
            type="button"
            onClick={async () => {
              if (step === 1) {
                await applyHotkeyIfChanged();
                setStep(2);
              } else {
                setStep(3);
              }
            }}
            className="flex items-center gap-1.5 px-5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium shadow-sm transition"
          >
            <span>Continue</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        ) : (
          <button
            type="button"
            disabled={saving}
            onClick={() => void handleFinish(false)}
            className="flex items-center gap-1.5 px-5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium shadow-sm disabled:opacity-50 transition"
          >
            <span>{saving ? 'Finishing...' : 'Done'}</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    </div>
  );
}
