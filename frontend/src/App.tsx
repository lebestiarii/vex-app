import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  BookOpen,
  Box,
  ChevronRight,
  Command,
  Compass,
  FileCode2,
  FolderOpen,
  Globe,
  HelpCircle,
  Plane,
  Plus,
  Search,
  Settings,
  Sparkles,
  Tag,
  Terminal,
  X,
} from 'lucide-react';
import { bridge } from './lib/bridge';
import { CardRenderer, TemplateIcon } from './components/CardRenderer';
import { AIChatView } from './components/AIChatView';
import { CardStudioModal } from './components/CardStudioModal';
import { SettingsModal } from './components/SettingsModal';
import { OnboardingWizard } from './components/OnboardingWizard';
import type {
  ActionResult,
  AnchorDirection,
  CardAction,
  CollectionStats,
  ParsedQuery,
  RAGResponse,
  RAGSource,
  SearchResult,
  UserPrefs,
} from './types/models';

interface CategoryScope {
  collection: string;
  label: string;
  count: number;
}

const KNOWN_CATEGORY_META: Record<string, { label: string; priority: number }> = {
  ports: { label: 'Ports & Airports', priority: 1 },
  'air-freight': { label: 'Air Freight & ULDs', priority: 2 },
  containers: { label: 'Container Sizes', priority: 3 },
  'ocean-freight': { label: 'Ocean Freight', priority: 4 },
  customs: { label: 'Customs & Compliance', priority: 5 },
  'hs-codes': { label: 'HS Codes', priority: 6 },
  acronyms: { label: 'Acronyms & Events', priority: 7 },
  portals: { label: 'Portals & Tools', priority: 8 },
  transcon: { label: 'Transcon & Ground', priority: 9 },
  'global-tech': { label: 'Global Technology', priority: 10 },
  'github-catalog': { label: 'GitHub Catalog', priority: 11 },
  shortcuts: { label: 'Websites & Apps', priority: 12 },
  knowledge: { label: 'Notes & Guides', priority: 13 },
  notes: { label: 'Notes', priority: 14 },
  devops: { label: 'Commands & Systems', priority: 15 },
};

function friendlyCollectionName(col: string): string {
  if (!col) return 'General';
  if (KNOWN_CATEGORY_META[col]) return KNOWN_CATEGORY_META[col].label;
  return col
    .replace(/[-_]+/g, ' ')
    .replace(/\b\w/g, (ch) => ch.toUpperCase());
}

function CategoryIcon({ collection, className = 'w-4 h-4' }: { collection: string; className?: string }) {
  switch (collection) {
    case 'ports':
    case 'air-freight':
      return <Plane className={className} />;
    case 'containers':
    case 'ocean-freight':
    case 'transcon':
      return <Box className={className} />;
    case 'hs-codes':
    case 'customs':
      return <FileCode2 className={className} />;
    case 'acronyms':
      return <Tag className={className} />;
    case 'portals':
    case 'shortcuts':
      return <Globe className={className} />;
    case 'knowledge':
    case 'notes':
      return <BookOpen className={className} />;
    case 'devops':
    case 'global-tech':
    case 'github-catalog':
      return <Terminal className={className} />;
    default:
      return <FolderOpen className={className} />;
  }
}

export default function App() {
  const [query, setQuery] = useState('');
  const [activeScope, setActiveScope] = useState<CategoryScope | null>(null);
  const [collections, setCollections] = useState<CollectionStats[]>([]);
  const [, setParsed] = useState<ParsedQuery | null>(null);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [selectedIdx, setSelectedIdx] = useState(0);
  const [heroIdx, setHeroIdx] = useState(0);
  const [focusedSlot, setFocusedSlot] = useState(0); // 0 = Hero Card, 1..3 = Runner-Up rows
  const [aiMode, setAiMode] = useState(false);

  const [prefsLoaded, setPrefsLoaded] = useState(false);
  const [prefs, setPrefs] = useState<UserPrefs>({
    onboardingCompleted: true,
    powerUserMode: false,
    aiEnabled: false,
    layoutMode: 'compact',
  });
  const [hotkey, setHotkey] = useState('Ctrl+Shift+B');
  const [anchor, setAnchor] = useState<AnchorDirection>('down');
  const [onboardingOpen, setOnboardingOpen] = useState(false);
  const [helpOpen, setHelpOpen] = useState(false);

  const [actionResult, setActionResult] = useState<ActionResult | null>(null);
  const [runningAction, setRunningAction] = useState(false);

  const [ragLoading, setRagLoading] = useState(false);
  const [ragResponse, setRagResponse] = useState<RAGResponse | null>(null);
  const [ragError, setRagError] = useState<string | null>(null);

  const [studioOpen, setStudioOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const inputRef = useRef<HTMLInputElement>(null);

  const refreshCollections = useCallback(async () => {
    try {
      const list = await bridge.listCollections();
      setCollections(list ?? []);
    } catch {
      // ignore transient errors
    }
  }, []);

  useEffect(() => {
    Promise.all([
      bridge.getUserPrefs(),
      bridge.getHotkey(),
      bridge.listCollections(),
      bridge.getAnchorDirection(),
    ])
      .then(([loadedPrefs, loadedHotkey, loadedCollections, loadedAnchor]) => {
        setPrefs(loadedPrefs);
        setHotkey(loadedHotkey || 'Ctrl+Shift+B');
        setCollections(loadedCollections ?? []);
        setAnchor(loadedAnchor);
        if (!loadedPrefs.onboardingCompleted) {
          setOnboardingOpen(true);
        }
      })
      .catch(() => {})
      .finally(() => setPrefsLoaded(true));

    return bridge.onAnchorChange((nextAnchor) => {
      setAnchor(nextAnchor);
      setTimeout(() => inputRef.current?.focus(), 0);
    });
  }, []);

  // Up to 4 user-pinned category scopes (⌘2–⌘5) paired with ⌘1 All
  const categoryScopes = useMemo<CategoryScope[]>(() => {
    const byName = new Map(collections.map((c) => [c.collection, c]));
    const pinned = prefs.pinnedCategories ?? [];
    const ordered: CollectionStats[] = [];
    const seen = new Set<string>();

    for (const col of pinned) {
      const stat = byName.get(col);
      if (stat && !seen.has(col)) {
        ordered.push(stat);
        seen.add(col);
      }
    }

    const fallbackSorted = [...collections].sort((a, b) => {
      const pa = KNOWN_CATEGORY_META[a.collection]?.priority ?? 50;
      const pb = KNOWN_CATEGORY_META[b.collection]?.priority ?? 50;
      if (pa !== pb) return pa - pb;
      return b.cardCount - a.cardCount;
    });

    for (const stat of fallbackSorted) {
      if (ordered.length >= 4) break;
      if (!seen.has(stat.collection)) {
        ordered.push(stat);
        seen.add(stat.collection);
      }
    }

    return ordered.slice(0, 4).map((c) => ({
      collection: c.collection,
      label: friendlyCollectionName(c.collection),
      count: c.cardCount,
    }));
  }, [collections, prefs.pinnedCategories]);

  const layoutMode = prefs.layoutMode ?? 'compact';
  const isIdle = !query.trim() && !activeScope && !aiMode;

  const effectiveQuery = useMemo(() => {
    const trimmed = query.trim();
    if (!activeScope) return query;
    if (trimmed.startsWith('?') || trimmed.startsWith('>') || trimmed.includes('@')) {
      return query;
    }
    return `@${activeScope.collection}${trimmed ? ` ${trimmed}` : ''}`;
  }, [query, activeScope]);

  const runSearch = useCallback(async (q: string) => {
    try {
      const resp = await bridge.search(q, 25);
      setParsed(resp.parsed);
      setResults(resp.results ?? []);
      setSelectedIdx(0);
      setHeroIdx(0);
      setFocusedSlot(0);
      if (resp.parsed.mode === 'ai') {
        setAiMode(true);
      }
    } catch {
      // Keep previous results on transient errors
    }
  }, []);

  useEffect(() => {
    if (isIdle) {
      setResults([]);
      setParsed(null);
      setActionResult(null);
      setHeroIdx(0);
      setFocusedSlot(0);
      return;
    }
    const timer = setTimeout(() => {
      void runSearch(effectiveQuery);
    }, 35);
    return () => clearTimeout(timer);
  }, [effectiveQuery, isIdle, runSearch]);

  const triggerRAG = useCallback(async (qText: string) => {
    const cleaned = qText.replace(/^\?\s*/, '').trim();
    if (!cleaned) return;
    setAiMode(true);
    setRagLoading(true);
    setRagError(null);
    try {
      const resp = await bridge.askRAG({
        query: cleaned,
        collection: activeScope?.collection,
      });
      setRagResponse(resp);
    } catch (e) {
      setRagError(e instanceof Error ? e.message : String(e));
    } finally {
      setRagLoading(false);
    }
  }, [activeScope]);

  const activeHeroIdx = results[heroIdx] ? heroIdx : 0;
  const heroCard = results[activeHeroIdx]?.card ?? null;
  const runnerUps = useMemo(
    () =>
      results
        .map((item, idx) => ({ item, originalIdx: idx }))
        .filter((entry) => entry.originalIdx !== activeHeroIdx)
        .slice(0, 3),
    [results, activeHeroIdx],
  );
  const selectedCard = layoutMode === 'compact' ? heroCard : (results[selectedIdx]?.card ?? null);

  const promoteRunnerUp = useCallback((originalIdx: number) => {
    setHeroIdx(originalIdx);
    setFocusedSlot(0);
    setActionResult(null);
    inputRef.current?.focus();
  }, []);

  const handleExecute = useCallback(
    async (action: CardAction, extraArgs: Record<string, string>) => {
      if (!selectedCard) return;
      if (action.type === 'ai_prompt') {
        const prompt = action.target || selectedCard.title;
        setQuery(`? ${prompt}`);
        void triggerRAG(prompt);
        return;
      }
      setRunningAction(true);
      setActionResult(null);
      try {
        const res = await bridge.executeAction(selectedCard.id, action.id, effectiveQuery, extraArgs);
        setActionResult(res);
        if (res.openAiMode && res.output) {
          setQuery(`? ${res.output}`);
          void triggerRAG(res.output);
        }
      } catch (e) {
        setActionResult({
          success: false,
          actionType: action.type,
          resolvedTarget: action.target,
          error: e instanceof Error ? e.message : String(e),
          durationMs: 0,
        });
      } finally {
        setRunningAction(false);
      }
    },
    [selectedCard, effectiveQuery, triggerRAG],
  );

  // Global keyboard shortcuts: ⌘1–⌘5 category scopes + staged Escape (disabled during Onboarding)
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (onboardingOpen) return;

      const mod = e.metaKey || e.ctrlKey;
      if (mod && e.key.toLowerCase() === 'n') {
        e.preventDefault();
        setSettingsOpen(false);
        setHelpOpen(false);
        setStudioOpen((v) => !v);
        return;
      }
      if (mod && e.key === ',') {
        e.preventDefault();
        setStudioOpen(false);
        setHelpOpen(false);
        setSettingsOpen((v) => !v);
        return;
      }

      // ⌘1 (All) & ⌘2..⌘5 pinned category scope selection
      if (mod && !studioOpen && !settingsOpen && !helpOpen && /^[1-5]$/.test(e.key)) {
        const num = Number(e.key);
        if (num === 1) {
          e.preventDefault();
          setAiMode(false);
          setActiveScope(null);
          inputRef.current?.focus();
          return;
        }
        const scope = categoryScopes[num - 2];
        if (scope) {
          e.preventDefault();
          setAiMode(false);
          setActiveScope((prev) => (prev?.collection === scope.collection ? null : scope));
          inputRef.current?.focus();
          return;
        }
      }

      if (e.key === 'Escape') {
        if (helpOpen) {
          setHelpOpen(false);
          inputRef.current?.focus();
        } else if (studioOpen || settingsOpen) {
          setStudioOpen(false);
          setSettingsOpen(false);
          inputRef.current?.focus();
        } else if (aiMode) {
          setAiMode(false);
          setQuery((q) => q.replace(/^\?\s*/, ''));
        } else if (layoutMode === 'compact' && (focusedSlot !== 0 || heroIdx !== 0)) {
          setFocusedSlot(0);
          setHeroIdx(0);
          setActionResult(null);
          inputRef.current?.focus();
        } else if (query) {
          setQuery('');
        } else if (activeScope) {
          setActiveScope(null);
        } else {
          void bridge.hideWindow();
        }
        return;
      }
      if (studioOpen || settingsOpen || helpOpen) return;

      if (e.key === 'Tab' && (prefs.aiEnabled || prefs.powerUserMode || aiMode)) {
        e.preventDefault();
        setAiMode((prev) => !prev);
        inputRef.current?.focus();
        return;
      }
      if (aiMode) {
        if (e.key === 'Enter') {
          e.preventDefault();
          void triggerRAG(query);
        }
        return;
      }
      if (!isIdle) {
        if (layoutMode === 'compact') {
          const totalSlots = heroCard ? 1 + runnerUps.length : 0;
          if (e.key === 'ArrowDown' && totalSlots > 0) {
            e.preventDefault();
            setFocusedSlot((s) => (s + 1) % totalSlots);
          } else if (e.key === 'ArrowUp' && totalSlots > 0) {
            e.preventDefault();
            setFocusedSlot((s) => (s - 1 + totalSlots) % totalSlots);
          } else if (e.key === 'Enter') {
            e.preventDefault();
            if (focusedSlot > 0 && runnerUps[focusedSlot - 1]) {
              promoteRunnerUp(runnerUps[focusedSlot - 1].originalIdx);
            } else if (heroCard?.actions?.length) {
              void handleExecute(heroCard.actions[0], {});
            }
          }
        } else {
          if (e.key === 'ArrowDown') {
            e.preventDefault();
            setSelectedIdx((i) => (results.length ? (i + 1) % results.length : 0));
            setActionResult(null);
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            setSelectedIdx((i) => (results.length ? (i - 1 + results.length) % results.length : 0));
            setActionResult(null);
          } else if (e.key === 'Enter' && selectedCard?.actions?.length) {
            e.preventDefault();
            void handleExecute(selectedCard.actions[0], {});
          }
        }
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [
    onboardingOpen,
    helpOpen,
    studioOpen,
    settingsOpen,
    aiMode,
    layoutMode,
    focusedSlot,
    heroIdx,
    heroCard,
    runnerUps,
    promoteRunnerUp,
    prefs.aiEnabled,
    prefs.powerUserMode,
    query,
    activeScope,
    categoryScopes,
    isIdle,
    results.length,
    selectedCard,
    handleExecute,
    triggerRAG,
  ]);

  const handleSelectRAGSource = (src: RAGSource) => {
    setAiMode(false);
    setActiveScope(null);
    setQuery(prefs.powerUserMode ? `@${src.collection} ${src.title}` : src.title);
  };

  const hasPortsPack = collections.some((c) => c.collection === 'ports');

  const isUp = anchor === 'up';

  return (
    <div
      data-anchor={anchor}
      className={`h-screen w-screen bg-transparent p-2.5 flex ${
        isUp ? 'flex-col-reverse justify-start' : 'flex-col'
      } gap-2.5 text-zinc-100 overflow-hidden relative`}
    >
      {/* PIECE 1: Floating Siri/Spotlight Glass Search Pill */}
      <div className="vex-glass-pill backdrop-blur-2xl shadow-[0_14px_40px_rgba(0,0,0,0.55)] px-5 py-3 flex items-center gap-3 shrink-0">
        {aiMode ? (
          <Sparkles className="w-5 h-5 text-purple-400 shrink-0" />
        ) : (
          <Search className="w-5 h-5 text-zinc-400 shrink-0" />
        )}

        {/* Active Category Scope Pill */}
        {activeScope && (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-indigo-500/20 border border-indigo-400/35 text-indigo-200 text-xs font-medium shrink-0">
            <CategoryIcon collection={activeScope.collection} className="w-3.5 h-3.5" />
            <span>{activeScope.label}</span>
            <button
              type="button"
              onClick={() => {
                setActiveScope(null);
                inputRef.current?.focus();
              }}
              aria-label="Clear category filter"
              className="hover:text-white ml-0.5"
            >
              <X className="w-3 h-3" />
            </button>
          </span>
        )}

        <input
          ref={inputRef}
          autoFocus={!onboardingOpen}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Backspace' && !query && activeScope) {
              setActiveScope(null);
            }
          }}
          placeholder={
            aiMode
              ? 'Ask a question about your saved items...'
              : activeScope
                ? `Search in ${activeScope.label}...`
                : prefs.powerUserMode
                  ? 'Search or Ask (/mod, @col, #tag, ?ai)...'
                  : 'Search or Ask'
          }
          className="flex-1 bg-transparent text-[17px] text-zinc-100 placeholder-zinc-500 focus:outline-none min-w-0"
        />

        {/* Minimal Pill Controls */}
        <div className="flex items-center gap-1 shrink-0">
          {(prefs.aiEnabled || aiMode) && (
            <button
              type="button"
              onClick={() => setAiMode((v) => !v)}
              title="Ask AI (Tab)"
              className={`flex items-center gap-1 px-2.5 py-1.5 rounded-full text-xs font-medium transition ${
                aiMode
                  ? 'bg-purple-600 text-white'
                  : 'text-purple-300 hover:bg-purple-500/15'
              }`}
            >
              <Sparkles className="w-3.5 h-3.5" />
              <span className="hidden sm:inline">AI</span>
            </button>
          )}
          <button
            type="button"
            onClick={() => setStudioOpen(true)}
            title="Add a link, note, or spreadsheet (⌘N)"
            aria-label="Add item"
            className="p-2 rounded-full text-zinc-400 hover:text-zinc-100 hover:bg-white/8 transition"
          >
            <Plus className="w-4 h-4" />
          </button>
          <button
            type="button"
            onClick={() => setSettingsOpen(true)}
            title="Settings (⌘,)"
            aria-label="Settings"
            className="p-2 rounded-full text-zinc-400 hover:text-zinc-100 hover:bg-white/8 transition"
          >
            <Settings className="w-4 h-4" />
          </button>
          <button
            type="button"
            onClick={() => void bridge.hideWindow()}
            title={`Hide to menu bar / tray (Esc • ${hotkey} to reopen)`}
            aria-label="Hide window"
            className="p-2 rounded-full text-zinc-500 hover:text-zinc-200 hover:bg-white/8 transition"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* PIECE 2: Detached Floating Glass Card (expands down or up based on viewport anchor) */}
      {isIdle ? (
        /* Calm, Compact Category Scope List (Siri-inspired) */
        <div
          className={`vex-glass-card backdrop-blur-2xl shadow-[0_18px_48px_rgba(0,0,0,0.55)] overflow-hidden flex ${
            isUp ? 'flex-col-reverse' : 'flex-col'
          } max-w-full`}
        >
          <div className="p-2 space-y-0.5">
            {categoryScopes.length === 0 ? (
              <div className="px-4 py-6 text-center text-xs text-zinc-400 space-y-2">
                <p>No categories yet — add your first link, note, or spreadsheet.</p>
                <button
                  type="button"
                  onClick={() => setStudioOpen(true)}
                  className="px-3 py-1.5 rounded-lg bg-indigo-600/20 border border-indigo-500/40 text-indigo-300 hover:bg-indigo-600/30 transition"
                >
                  + Add Item
                </button>
              </div>
            ) : (
              categoryScopes.map((scope, idx) => (
                <button
                  type="button"
                  key={scope.collection}
                  onClick={() => {
                    setActiveScope(scope);
                    inputRef.current?.focus();
                  }}
                  className="w-full flex items-center justify-between px-3.5 py-2.5 rounded-2xl text-left hover:bg-white/8 transition group"
                >
                  <div className="flex items-center gap-3.5 min-w-0">
                    <div className="w-7 h-7 rounded-xl bg-white/6 group-hover:bg-indigo-500/20 text-zinc-300 group-hover:text-indigo-200 flex items-center justify-center shrink-0 transition">
                      <CategoryIcon collection={scope.collection} className="w-4 h-4" />
                    </div>
                    <div className="min-w-0">
                      <span className="text-[14px] font-medium text-zinc-100">{scope.label}</span>
                      <span className="ml-2 text-[11px] text-zinc-500">{scope.count}</span>
                    </div>
                  </div>
                  <span className="text-xs font-mono text-zinc-500 group-hover:text-zinc-300">
                    ⌘{idx + 2}
                  </span>
                </button>
              ))
            )}
          </div>

          {/* Subtle Calm Footer inside Compact Card */}
          <div
            className={`flex items-center justify-between px-4 py-2 ${
              isUp ? 'border-b border-white/6' : 'border-t border-white/6'
            } bg-white/[0.02] text-[11px] text-zinc-400`}
          >
            <div className="flex items-center gap-2">
              {hasPortsPack ? (
                <>
                  <span className="text-zinc-500">Quick try:</span>
                  <button
                    type="button"
                    onClick={() => {
                      setQuery('AMS');
                      inputRef.current?.focus();
                    }}
                    className="px-2 py-0.5 rounded-md bg-white/6 hover:bg-white/12 text-zinc-200 font-mono text-[10px] transition"
                  >
                    AMS
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setQuery('40HC');
                      inputRef.current?.focus();
                    }}
                    className="px-2 py-0.5 rounded-md bg-white/6 hover:bg-white/12 text-zinc-200 font-mono text-[10px] transition"
                  >
                    40HC
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setQuery('8471');
                      inputRef.current?.focus();
                    }}
                    className="px-2 py-0.5 rounded-md bg-white/6 hover:bg-white/12 text-zinc-200 font-mono text-[10px] transition"
                  >
                    8471
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setQuery('120x80x100 cm');
                      inputRef.current?.focus();
                    }}
                    className="px-2 py-0.5 rounded-md bg-white/6 hover:bg-white/12 text-zinc-200 font-mono text-[10px] transition"
                  >
                    120x80x100 cm
                  </button>
                </>
              ) : (
                <span className="text-zinc-500">Just type anything to search immediately</span>
              )}
            </div>
            <button
              type="button"
              onClick={() => setHelpOpen((v) => !v)}
              className="flex items-center gap-1 px-2 py-0.5 rounded-md text-zinc-400 hover:text-zinc-100 hover:bg-white/8 transition"
            >
              <HelpCircle className="w-3.5 h-3.5 text-indigo-400" />
              <span>Help</span>
            </button>
          </div>
        </div>
      ) : (
        /* Expanded Hero Card + Compact Runner-Ups Glass Container */
        <div
          className={`flex-1 min-h-0 vex-glass-card backdrop-blur-2xl shadow-[0_18px_48px_rgba(0,0,0,0.6)] flex ${
            isUp ? 'flex-col-reverse' : 'flex-col'
          } overflow-hidden`}
        >
          {/* Category Modifier Pills (⌘1 All + ⌘2–⌘5 Pinned Categories) */}
          {!aiMode && categoryScopes.length > 0 && (
            <div
              className={`flex items-center justify-center gap-1.5 px-3.5 py-2 ${
                isUp ? 'border-t border-white/8' : 'border-b border-white/8'
              } bg-white/[0.015] shrink-0 overflow-x-auto`}
            >
              <button
                type="button"
                onClick={() => {
                  setActiveScope(null);
                  inputRef.current?.focus();
                }}
                className={`px-2.5 py-1 rounded-full text-[11px] font-medium transition whitespace-nowrap flex items-center gap-1.5 ${
                  !activeScope
                    ? 'bg-indigo-500/22 border border-indigo-400/45 text-indigo-100 shadow-[0_0_12px_rgba(99,102,241,0.22)]'
                    : 'bg-white/[0.04] border border-transparent text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.08]'
                }`}
              >
                <span className="font-mono text-[10px] opacity-75">⌘1</span>
                <span>All</span>
              </button>
              {categoryScopes.map((scope, idx) => {
                const isActive = activeScope?.collection === scope.collection;
                return (
                  <button
                    type="button"
                    key={scope.collection}
                    onClick={() => {
                      setActiveScope(isActive ? null : scope);
                      inputRef.current?.focus();
                    }}
                    className={`px-2.5 py-1 rounded-full text-[11px] font-medium transition whitespace-nowrap flex items-center gap-1.5 ${
                      isActive
                        ? 'bg-indigo-500/22 border border-indigo-400/45 text-indigo-100 shadow-[0_0_12px_rgba(99,102,241,0.22)]'
                        : 'bg-white/[0.04] border border-transparent text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.08]'
                    }`}
                  >
                    <span className="font-mono text-[10px] opacity-75">⌘{idx + 2}</span>
                    <span>{scope.label}</span>
                  </button>
                );
              })}
            </div>
          )}

          <div className="flex-1 min-h-0 flex">
            {aiMode ? (
              <div className="flex-1 min-w-0">
                <AIChatView
                  query={query}
                  loading={ragLoading}
                  response={ragResponse}
                  error={ragError}
                  onSelectSource={handleSelectRAGSource}
                  onBackToSearch={() => {
                    setAiMode(false);
                    setQuery((q) => q.replace(/^\?\s*/, ''));
                    inputRef.current?.focus();
                  }}
                />
              </div>
            ) : layoutMode === 'compact' ? (
              /* Primary Hero Card + Compact Horizontal Runner-Ups */
              <div className="flex-1 min-w-0 flex flex-col p-3 gap-2.5 overflow-hidden">
                {!heroCard ? (
                  <div className="flex-1 flex flex-col items-center justify-center text-center p-6 text-zinc-500 space-y-2.5">
                    <Command className="w-7 h-7 opacity-40" />
                    <p className="text-xs text-zinc-400">No matching items found.</p>
                    <button
                      type="button"
                      onClick={() => setStudioOpen(true)}
                      className="px-3 py-1.5 rounded-lg bg-indigo-600/20 border border-indigo-500/40 text-xs text-indigo-300 hover:bg-indigo-600/30 transition"
                    >
                      + Add item
                    </button>
                  </div>
                ) : (
                  <>
                    {/* Primary Hero Card */}
                    <div
                      onClick={() => setFocusedSlot(0)}
                      className={`flex-1 min-h-0 rounded-2xl overflow-hidden transition ${
                        focusedSlot === 0
                          ? 'vex-hero-surface vex-hero-surface-focused'
                          : 'vex-hero-surface'
                      }`}
                    >
                      <CardRenderer
                        card={heroCard}
                        actionResult={actionResult}
                        running={runningAction}
                        powerUserMode={prefs.powerUserMode}
                        aiEnabled={prefs.aiEnabled}
                        activeQuery={query}
                        compactHero
                        onExecute={handleExecute}
                        onAskAI={(prompt) => {
                          setQuery(`? ${prompt}`);
                          void triggerRAG(prompt);
                        }}
                        onSelectCard={(rel) => {
                          setActiveScope(null);
                          setQuery(rel.title);
                        }}
                      />
                    </div>

                    {/* Soft Divider + 2–3 Compact Horizontal Runner-Up Rows */}
                    {runnerUps.length > 0 && (
                      <div className="shrink-0 pt-2 border-t border-white/8 space-y-1">
                        {runnerUps.map(({ item, originalIdx }, rIdx) => {
                          const slotIdx = rIdx + 1;
                          const isFocused = focusedSlot === slotIdx;
                          return (
                            <button
                              type="button"
                              key={item.card.id}
                              onMouseEnter={() => setFocusedSlot(slotIdx)}
                              onClick={() => promoteRunnerUp(originalIdx)}
                              className={`w-full text-left px-3 py-2 rounded-xl flex items-center gap-3 transition ${
                                isFocused
                                  ? 'bg-indigo-500/18 border border-indigo-400/40 text-white'
                                  : 'bg-white/[0.03] border border-white/[0.05] hover:bg-white/[0.07] text-zinc-300'
                              }`}
                            >
                              <div
                                className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                                  isFocused ? 'bg-indigo-500/30 text-indigo-200' : 'bg-white/6 text-zinc-400'
                                }`}
                              >
                                <TemplateIcon template={item.card.template} className="w-3.5 h-3.5" />
                              </div>
                              <div className="min-w-0 flex-1">
                                <div className="text-[12.5px] font-medium truncate">{item.card.title}</div>
                                <div className="text-[11px] text-zinc-400 truncate">
                                  {item.card.subtitle || friendlyCollectionName(item.card.collection)}
                                </div>
                              </div>
                              {/* Up to 3 backend-computed GlancePills */}
                              <div className="hidden sm:flex items-center gap-1.5 shrink-0">
                                {(item.card.glancePills ?? []).slice(0, 3).map((pill, pIdx) => (
                                  <span
                                    key={pIdx}
                                    className="px-2 py-0.5 rounded-md bg-indigo-500/12 border border-indigo-400/25 text-[10px] font-mono text-indigo-200 whitespace-nowrap"
                                  >
                                    {pill}
                                  </span>
                                ))}
                                {(!item.card.glancePills || item.card.glancePills.length === 0) && (
                                  <span className="px-2 py-0.5 rounded-md bg-white/6 text-[10px] text-zinc-400">
                                    {friendlyCollectionName(item.card.collection)}
                                  </span>
                                )}
                              </div>
                              <ChevronRight
                                className={`w-3.5 h-3.5 shrink-0 transition ${
                                  isFocused ? 'text-indigo-300 translate-x-0.5' : 'text-zinc-600'
                                }`}
                              />
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </>
                )}
              </div>
            ) : (
              <>
                {/* Split Mode — Left: Multi-Facet Results List */}
                <div className="w-[42%] border-r border-white/8 overflow-y-auto p-2 space-y-1">
                  {results.length === 0 ? (
                    <div className="h-full flex flex-col items-center justify-center text-center p-6 text-zinc-500 space-y-2.5">
                      <Command className="w-7 h-7 opacity-40" />
                      <p className="text-xs text-zinc-400">No matching items found.</p>
                      <button
                        type="button"
                        onClick={() => setStudioOpen(true)}
                        className="px-3 py-1.5 rounded-lg bg-indigo-600/20 border border-indigo-500/40 text-xs text-indigo-300 hover:bg-indigo-600/30 transition"
                      >
                        + Add a link, note, or spreadsheet
                      </button>
                    </div>
                  ) : (
                    results.map((item, idx) => {
                      const active = idx === selectedIdx;
                      return (
                        <button
                          type="button"
                          key={item.card.id}
                          onClick={() => {
                            setSelectedIdx(idx);
                            setActionResult(null);
                          }}
                          onDoubleClick={() => {
                            if (item.card.actions?.length) {
                              void handleExecute(item.card.actions[0], {});
                            }
                          }}
                          className={`w-full text-left px-3 py-2.5 rounded-xl flex items-center gap-3 transition ${
                            active
                              ? 'bg-indigo-600/22 border border-indigo-400/40 text-white'
                              : 'border border-transparent hover:bg-white/6 text-zinc-300'
                          }`}
                        >
                          <div
                            className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 ${
                              active ? 'bg-indigo-500/30 text-indigo-200' : 'bg-white/6 text-zinc-400'
                            }`}
                          >
                            <TemplateIcon template={item.card.template} className="w-3.5 h-3.5" />
                          </div>
                          <div className="min-w-0 flex-1">
                            <div className="text-xs font-medium truncate">{item.card.title}</div>
                            <div className="text-[11px] text-zinc-400 truncate">
                              {item.card.subtitle || friendlyCollectionName(item.card.collection)}
                            </div>
                          </div>
                          {prefs.powerUserMode && (
                            <span className="text-[10px] font-mono text-zinc-500 shrink-0">
                              {item.matchSource}
                            </span>
                          )}
                        </button>
                      );
                    })
                  )}
                </div>

                {/* Right: Instant Answer & Live Widget Pane */}
                <div className="flex-1 min-w-0 bg-black/15">
                  {selectedCard ? (
                    <CardRenderer
                      card={selectedCard}
                      actionResult={actionResult}
                      running={runningAction}
                      powerUserMode={prefs.powerUserMode}
                      aiEnabled={prefs.aiEnabled}
                      activeQuery={query}
                      onExecute={handleExecute}
                      onAskAI={(prompt) => {
                        setQuery(`? ${prompt}`);
                        void triggerRAG(prompt);
                      }}
                      onSelectCard={(rel) => {
                        setActiveScope(null);
                        setQuery(rel.title);
                      }}
                    />
                  ) : (
                    <div className="h-full flex items-center justify-center text-xs text-zinc-500">
                      Select an item on the left to preview
                    </div>
                  )}
                </div>
              </>
            )}
          </div>

          {/* Slim Active Footer */}
          <div
            className={`flex items-center justify-between px-4 py-2 ${
              isUp ? 'border-b border-white/8' : 'border-t border-white/8'
            } bg-white/[0.02] text-[11px] text-zinc-400 shrink-0`}
          >
            <div className="flex items-center gap-3">
              <span>
                <kbd className="px-1.5 py-0.5 rounded bg-white/10 text-zinc-200 font-mono">↵</kbd>{' '}
                {layoutMode === 'compact' && focusedSlot > 0 ? 'Promote to Hero' : 'Open / Copy'}
              </span>
              <span>
                <kbd className="px-1.5 py-0.5 rounded bg-white/10 text-zinc-200 font-mono">Esc</kbd>{' '}
                {layoutMode === 'compact' && (focusedSlot > 0 || heroIdx > 0) ? 'Reset' : 'Back / Hide'}
              </span>
              {prefs.powerUserMode && (
                <span>
                  <kbd className="px-1.5 py-0.5 rounded bg-white/10 text-zinc-200 font-mono">Tab</kbd> AI
                </span>
              )}
            </div>
            <div className="flex items-center gap-3">
              <span className="text-zinc-500">{results.length} matches</span>
              <button
                type="button"
                onClick={() => setHelpOpen((v) => !v)}
                className="flex items-center gap-1 px-2 py-0.5 rounded-md text-zinc-400 hover:text-zinc-100 hover:bg-white/8 transition"
              >
                <HelpCircle className="w-3.5 h-3.5 text-indigo-400" />
                <span>Help</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Contextual Help Popover */}
      {helpOpen && (
        <div
          className={`fixed inset-0 z-40 bg-black/45 backdrop-blur-[2px] flex ${
            isUp ? 'items-start' : 'items-end'
          } justify-end p-5`}
        >
          <div className="w-80 bg-zinc-950/95 border border-white/12 rounded-2xl shadow-2xl p-4 space-y-3 text-xs">
            <div className="flex items-center justify-between">
              <div className="font-semibold text-zinc-100 flex items-center gap-1.5">
                <HelpCircle className="w-4 h-4 text-indigo-400" />
                <span>Quick Guide</span>
              </div>
              <button
                type="button"
                onClick={() => setHelpOpen(false)}
                className="p-1 rounded text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>

            <div className="space-y-2 text-zinc-300">
              <div className="rounded-lg bg-zinc-900/70 border border-zinc-800/80 p-2.5">
                <div className="font-medium text-zinc-100">Instant Lookup</div>
                <p className="text-[11px] text-zinc-400 mt-0.5">
                  Type a code like <strong>AMS</strong> or <strong>40HC</strong> to see ports, live local time, acronyms, and portals all at once.
                </p>
              </div>
              <div className="rounded-lg bg-zinc-900/70 border border-zinc-800/80 p-2.5">
                <div className="font-medium text-zinc-100">Category Shortcuts</div>
                <p className="text-[11px] text-zinc-400 mt-0.5">
                  Click any category or press <kbd className="px-1 rounded bg-zinc-800 font-mono text-[10px]">⌘1</kbd>–<kbd className="px-1 rounded bg-zinc-800 font-mono text-[10px]">⌘5</kbd> to narrow your search.
                </p>
              </div>
              <div className="rounded-lg bg-zinc-900/70 border border-zinc-800/80 p-2.5">
                <div className="font-medium text-zinc-100">Hide &amp; Reopen</div>
                <p className="text-[11px] text-zinc-400 mt-0.5">
                  Press <kbd className="px-1 rounded bg-zinc-800 font-mono text-[10px]">Esc</kbd> or <strong>×</strong> to hide. Press{' '}
                  <kbd className="px-1 rounded bg-zinc-800 font-mono text-[10px]">{hotkey}</kbd> anytime to pop it back up.
                </p>
              </div>
            </div>

            <button
              type="button"
              onClick={() => {
                setHelpOpen(false);
                setOnboardingOpen(true);
              }}
              className="w-full flex items-center justify-center gap-1.5 px-3 py-2 rounded-xl bg-indigo-600/20 hover:bg-indigo-600/30 border border-indigo-500/40 text-indigo-200 font-medium transition"
            >
              <Compass className="w-3.5 h-3.5" />
              <span>Replay Welcome Tour</span>
            </button>
          </div>
        </div>
      )}

      {/* Modals */}
      <CardStudioModal
        open={studioOpen}
        powerUserMode={prefs.powerUserMode}
        aiEnabled={prefs.aiEnabled}
        onClose={() => setStudioOpen(false)}
        onSaved={() => {
          void refreshCollections();
          if (!isIdle) void runSearch(effectiveQuery);
        }}
      />
      <SettingsModal
        open={settingsOpen}
        prefs={prefs}
        onPrefsChange={setPrefs}
        onHotkeyChange={setHotkey}
        onReplayTour={() => setOnboardingOpen(true)}
        onDataChanged={() => {
          void refreshCollections();
          if (!isIdle) void runSearch(effectiveQuery);
        }}
        onClose={() => setSettingsOpen(false)}
      />

      {/* First-Run Onboarding & Replayable Tour */}
      {prefsLoaded && onboardingOpen && (
        <OnboardingWizard
          initialHotkey={hotkey}
          initialPrefs={prefs}
          onComplete={(nextPrefs, nextHotkey) => {
            setPrefs(nextPrefs);
            setHotkey(nextHotkey);
            setOnboardingOpen(false);
            void refreshCollections();
            setTimeout(() => inputRef.current?.focus(), 50);
          }}
        />
      )}
    </div>
  );
}
