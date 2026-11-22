import { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  BookOpen,
  Check,
  Clock,
  CloudSun,
  Coins,
  Copy,
  ExternalLink,
  Globe,
  Layers,
  Link2,
  MapPin,
  Play,
  Plug,
  Server,
  ShieldCheck,
  Sparkles,
  Terminal,
} from 'lucide-react';
import { bridge } from '../lib/bridge';
import type { ActionResult, CardAction, SearchCard, WeatherInfo } from '../types/models';

interface CardRendererProps {
  card: SearchCard;
  actionResult: ActionResult | null;
  running: boolean;
  powerUserMode?: boolean;
  aiEnabled?: boolean;
  activeQuery?: string;
  compactHero?: boolean;
  onExecute: (action: CardAction, extraArgs: Record<string, string>) => void;
  onAskAI: (prompt: string) => void;
  onSelectCard?: (card: SearchCard) => void;
}

function friendlyTemplateLabel(template: string): string {
  switch (template) {
    case 'shortcut':
      return 'Website / App';
    case 'knowledge':
      return 'Note';
    case 'command':
      return 'Command';
    case 'kv-grid':
      return 'Info Card';
    case 'metric-card':
      return 'Status';
    default:
      return 'Item';
  }
}

const PLACEHOLDER_RE = /\{\{\s*([^}]+?)\s*\}\}/g;

function extractPlaceholders(card: SearchCard): string[] {
  const keys = new Set<string>();
  for (const act of card.actions ?? []) {
    let m: RegExpExecArray | null;
    PLACEHOLDER_RE.lastIndex = 0;
    while ((m = PLACEHOLDER_RE.exec(act.target)) !== null) {
      const k = m[1].trim();
      if (!k.startsWith('data.') && k !== 'query' && k !== 'raw' && k !== 'card.title') {
        keys.add(k);
      }
    }
  }
  return Array.from(keys);
}

export function TemplateIcon({ template, className = 'w-4 h-4' }: { template: string; className?: string }) {
  switch (template) {
    case 'shortcut':
      return <Globe className={className} />;
    case 'command':
      return <Terminal className={className} />;
    case 'kv-grid':
      return <Server className={className} />;
    case 'metric-card':
      return <Activity className={className} />;
    default:
      return <BookOpen className={className} />;
  }
}

function formatLocalTime(tz: string): string | null {
  try {
    return new Intl.DateTimeFormat(undefined, {
      timeZone: tz,
      hour: 'numeric',
      minute: '2-digit',
      weekday: 'short',
      timeZoneName: 'short',
    }).format(new Date());
  } catch {
    return null;
  }
}

export function CardRenderer({
  card,
  actionResult,
  running,
  powerUserMode = false,
  aiEnabled = false,
  activeQuery = '',
  compactHero = false,
  onExecute,
  onAskAI,
  onSelectCard,
}: CardRendererProps) {
  const [extraArgs, setExtraArgs] = useState<Record<string, string>>({});
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [localTime, setLocalTime] = useState<string | null>(null);
  const [weather, setWeather] = useState<WeatherInfo | null>(null);
  const [activeTab, setActiveTab] = useState<string>('overview');
  const [relatedCards, setRelatedCards] = useState<SearchCard[]>([]);

  const placeholders = useMemo(() => extractPlaceholders(card), [card]);

  const timezone = typeof card.data?.timezone === 'string' ? card.data.timezone : null;
  const lat = typeof card.data?.lat === 'number' ? card.data.lat : null;
  const lon = typeof card.data?.lon === 'number' ? card.data.lon : null;
  const country = typeof card.data?.country === 'string' ? card.data.country : null;
  const countryCode = typeof card.data?.country_code === 'string' ? card.data.country_code : null;
  const currency = typeof card.data?.currency === 'string' ? card.data.currency : null;

  useEffect(() => {
    setExtraArgs({});
    // Auto-select a module perspective tab if any query token specifically matched inside that section
    let initialTab = 'overview';
    const rawTokens = activeQuery
      .trim()
      .toLowerCase()
      .split(/\s+/)
      .filter((t) => t.length >= 2 && !t.startsWith('@') && !t.startsWith('#') && !t.startsWith('/'));
    const titleLower = card.title.toLowerCase();
    const sectionTokens = rawTokens.filter((t) => !titleLower.includes(t));
    if (sectionTokens.length > 0 && card.sections && card.sections.length > 0) {
      const matchedSec = card.sections.find((sec) => {
        const hay = [
          sec.moduleId,
          sec.moduleName,
          sec.badge ?? '',
          sec.title ?? '',
          sec.notes ?? '',
          ...Object.entries(sec.fields ?? {}).map(([k, v]) => `${k} ${String(v)}`),
        ]
          .join(' ')
          .toLowerCase();
        return sectionTokens.some((tok) => hay.includes(tok));
      });
      if (matchedSec) {
        initialTab = matchedSec.moduleId;
      }
    }
    setActiveTab(initialTab);

    let cancelled = false;
    setRelatedCards([]);
    bridge.getRelatedCards(card.id, 5).then((list) => {
      if (!cancelled) setRelatedCards(list ?? []);
    }).catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [card.id, activeQuery]);

  // Live local clock for location / port cards
  useEffect(() => {
    if (!timezone) {
      setLocalTime(null);
      return;
    }
    const update = () => setLocalTime(formatLocalTime(timezone));
    update();
    const timer = setInterval(update, 30_000);
    return () => clearInterval(timer);
  }, [timezone]);

  // Lightweight live weather for location / port cards with lat & lon
  useEffect(() => {
    let cancelled = false;
    setWeather(null);
    if (lat === null || lon === null) return;
    bridge.fetchWeather(lat, lon).then((info) => {
      if (!cancelled && info) setWeather(info);
    });
    return () => {
      cancelled = true;
    };
  }, [card.id, lat, lon]);

  const copyValue = async (key: string, val: string) => {
    await navigator.clipboard?.writeText(val).catch(() => {});
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 1500);
  };

  const hasLocationGlance = Boolean(localTime || country || currency || weather);
  const GLANCE_KEYS = new Set(['lat', 'lon', 'country', 'country_code', 'currency', 'timezone']);
  const dataEntries = Object.entries(card.data ?? {}).filter(([k]) => {
    if (powerUserMode) return true;
    if (k === 'lat' || k === 'lon') return false;
    if (hasLocationGlance && GLANCE_KEYS.has(k)) return false;
    return true;
  });

  return (
    <div className={`h-full overflow-y-auto ${compactHero ? 'p-3.5 space-y-3' : 'p-4 space-y-4'}`}>
      {/* Card Header */}
      <div className="flex items-start justify-between gap-3 pb-2.5 border-b border-white/8">
        <div className="flex items-start gap-3 min-w-0">
          <div className="w-8 h-8 rounded-lg bg-indigo-500/15 border border-indigo-500/30 flex items-center justify-center text-indigo-300 shrink-0 mt-0.5">
            {card.icon === 'plug' ? (
              <Plug className="w-4 h-4" />
            ) : (
              <TemplateIcon template={card.template} className="w-4 h-4" />
            )}
          </div>
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-zinc-100 truncate">{card.title}</h2>
            {card.subtitle && <p className="text-xs text-zinc-400 truncate mt-0.5">{card.subtitle}</p>}
            <div className="flex flex-wrap items-center gap-1.5 mt-1.5">
              {powerUserMode ? (
                <>
                  <span className="px-1.5 py-0.5 text-[10px] font-medium rounded bg-zinc-800 text-zinc-300 border border-zinc-700/60">
                    @{card.collection}
                  </span>
                  <span className="px-1.5 py-0.5 text-[10px] font-medium rounded bg-indigo-500/15 text-indigo-300 border border-indigo-500/30">
                    {card.template}
                  </span>
                  {(card.modifiers ?? []).map((m) => (
                    <span
                      key={m}
                      className="px-1.5 py-0.5 text-[10px] font-mono rounded bg-emerald-500/10 text-emerald-300 border border-emerald-500/25"
                    >
                      /{m}
                    </span>
                  ))}
                  {(card.tags ?? []).map((t) => (
                    <span key={t} className="px-1.5 py-0.5 text-[10px] rounded bg-zinc-800/70 text-zinc-400">
                      #{t}
                    </span>
                  ))}
                </>
              ) : (
                <>
                  <span className="px-2 py-0.5 text-[10px] font-medium rounded-md bg-indigo-500/15 text-indigo-300 border border-indigo-500/25">
                    {friendlyTemplateLabel(card.template)}
                  </span>
                  {(card.tags ?? []).slice(0, 3).map((t) => (
                    <span key={t} className="px-2 py-0.5 text-[10px] rounded-md bg-zinc-800/70 text-zinc-400">
                      {t}
                    </span>
                  ))}
                </>
              )}
            </div>
          </div>
        </div>

        {/* Right-aligned high-signal Glance Pills */}
        {(card.glancePills?.length ?? 0) > 0 && (
          <div className="hidden sm:flex flex-wrap justify-end gap-1.5 max-w-[42%] shrink-0">
            {card.glancePills!.slice(0, 3).map((pill, idx) => (
              <span
                key={idx}
                className="px-2 py-0.5 rounded-md bg-indigo-500/15 border border-indigo-400/25 text-[10px] font-mono text-indigo-200 whitespace-nowrap"
              >
                {pill}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Live Location / Port Glance Strip (Time, Country, Currency, Weather) */}
      {hasLocationGlance && (
        <div className="grid grid-cols-2 gap-2">
          {localTime && (
            <div className="rounded-xl bg-indigo-500/10 border border-indigo-500/25 px-3 py-2 flex items-center gap-2.5">
              <Clock className="w-4 h-4 text-indigo-300 shrink-0" />
              <div className="min-w-0">
                <div className="text-[10px] uppercase tracking-wider text-indigo-300/80">Local Time</div>
                <div className="text-xs font-semibold text-zinc-100 truncate">{localTime}</div>
              </div>
            </div>
          )}
          {weather && (
            <div className="rounded-xl bg-sky-500/10 border border-sky-500/25 px-3 py-2 flex items-center gap-2.5">
              <CloudSun className="w-4 h-4 text-sky-300 shrink-0" />
              <div className="min-w-0">
                <div className="text-[10px] uppercase tracking-wider text-sky-300/80">Weather</div>
                <div className="text-xs font-semibold text-zinc-100 truncate">
                  {Math.round(weather.tempC)}°C / {Math.round(weather.tempF)}°F · {weather.condition}
                </div>
              </div>
            </div>
          )}
          {country && (
            <div className="rounded-xl bg-zinc-900/80 border border-zinc-800 px-3 py-2 flex items-center gap-2.5">
              <MapPin className="w-4 h-4 text-emerald-400 shrink-0" />
              <div className="min-w-0">
                <div className="text-[10px] uppercase tracking-wider text-zinc-500">Country</div>
                <div className="text-xs font-medium text-zinc-100 truncate">
                  {country}
                  {countryCode ? ` (${countryCode})` : ''}
                </div>
              </div>
            </div>
          )}
          {currency && (
            <div className="rounded-xl bg-zinc-900/80 border border-zinc-800 px-3 py-2 flex items-center gap-2.5">
              <Coins className="w-4 h-4 text-amber-400 shrink-0" />
              <div className="min-w-0">
                <div className="text-[10px] uppercase tracking-wider text-zinc-500">Currency</div>
                <div className="text-xs font-medium text-zinc-100 truncate">{currency}</div>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Perspective Tab Bar (One Active Perspective at a Time to Prevent Information Density Overload) */}
      {card.sections && card.sections.length > 0 && (
        <div className="flex items-center gap-1.5 border-b border-zinc-800/80 pb-2.5 flex-wrap">
          <button
            type="button"
            onClick={() => setActiveTab('overview')}
            className={`px-3 py-1 rounded-lg text-xs font-medium transition ${
              activeTab === 'overview'
                ? 'bg-indigo-600 text-white shadow-sm'
                : 'bg-zinc-900 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800'
            }`}
          >
            Overview
          </button>
          {card.sections.map((sec) => {
            const isTech = sec.moduleId === 'global-tech' || sec.moduleId === 'github-catalog';
            const selected = activeTab === sec.moduleId;
            return (
              <button
                type="button"
                key={sec.moduleId}
                onClick={() => setActiveTab(sec.moduleId)}
                className={`px-3 py-1 rounded-lg text-xs font-medium transition flex items-center gap-1.5 ${
                  selected
                    ? isTech
                      ? 'bg-emerald-600 text-white shadow-sm'
                      : 'bg-indigo-600 text-white shadow-sm'
                    : 'bg-zinc-900 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800'
                }`}
              >
                <Layers className="w-3 h-3 opacity-80" />
                <span>{sec.badge || sec.moduleName}</span>
              </button>
            );
          })}
        </div>
      )}

      {activeTab === 'overview' ? (
        <>
          {/* Template-Specific Overview Body */}
          {card.template === 'metric-card' && dataEntries.length > 0 && (
            <div className="grid grid-cols-2 gap-2.5">
              {dataEntries.map(([k, v]) => (
                <div
                  key={k}
                  className="rounded-lg bg-zinc-900/90 border border-zinc-800 p-3 flex flex-col justify-between"
                >
                  <span className="text-[10px] uppercase tracking-wider text-zinc-500">{k.replace(/_/g, ' ')}</span>
                  <span className="text-base font-semibold text-zinc-100 mt-1 font-mono truncate">
                    {typeof v === 'object' ? JSON.stringify(v) : String(v)}
                  </span>
                </div>
              ))}
            </div>
          )}

          {card.description && (
            <div className="rounded-lg bg-zinc-900/50 border border-zinc-800/70 p-3 text-xs text-zinc-300 leading-relaxed whitespace-pre-wrap">
              {card.description}
            </div>
          )}

          {card.template === 'kv-grid' && dataEntries.length > 0 && (
            <div className="rounded-lg border border-zinc-800 bg-zinc-900/70 divide-y divide-zinc-800/80 overflow-hidden">
              {dataEntries.map(([k, v]) => {
                const strVal = typeof v === 'object' ? JSON.stringify(v) : String(v);
                return (
                  <div key={k} className="flex items-center justify-between gap-2 px-3 py-2 text-xs hover:bg-zinc-800/40">
                    <span className="text-zinc-400 font-mono shrink-0">{k.replace(/_/g, ' ')}</span>
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="text-zinc-100 font-mono truncate">{strVal}</span>
                      <button
                        type="button"
                        onClick={() => void copyValue(k, strVal)}
                        className="p-1 rounded hover:bg-zinc-700/60 text-zinc-400 hover:text-zinc-100 shrink-0"
                        title="Copy value"
                      >
                        {copiedKey === k ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          {card.template === 'shortcut' && typeof card.data?.url === 'string' && (
            <div className="flex items-center justify-between gap-2 px-3 py-2 rounded-lg bg-zinc-900/80 border border-zinc-800 text-xs">
              <span className="text-zinc-400 truncate font-mono">{card.data.url}</span>
              <button
                type="button"
                onClick={() => void copyValue('url', String(card.data?.url))}
                className="text-zinc-400 hover:text-zinc-100 shrink-0"
              >
                {copiedKey === 'url' ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
              </button>
            </div>
          )}

          {card.template === 'knowledge' && dataEntries.length > 0 && (
            <div className="rounded-lg border border-zinc-800/70 bg-zinc-900/40 p-2.5 space-y-1">
              <div className="text-[10px] uppercase tracking-wider text-zinc-500 mb-1">
                {powerUserMode ? 'Structured Fields' : 'Details'}
              </div>
              {dataEntries.map(([k, v]) => (
                <div key={k} className="flex justify-between gap-2 text-xs">
                  <span className="text-zinc-400 font-mono">{k.replace(/_/g, ' ')}</span>
                  <span className="text-zinc-200 font-mono truncate">{String(v)}</span>
                </div>
              ))}
            </div>
          )}
        </>
      ) : (
        /* Focused Single Module Perspective View */
        card.sections
          ?.filter((sec) => sec.moduleId === activeTab)
          .map((sec, sIdx) => {
            const secEntries = Object.entries(sec.fields ?? {});
            const isTech = sec.moduleId === 'global-tech' || sec.moduleId === 'github-catalog';
            return (
              <div
                key={`${sec.moduleId}-${sIdx}`}
                className={`rounded-xl border p-3.5 space-y-2.5 ${
                  isTech
                    ? 'bg-emerald-950/20 border-emerald-500/30'
                    : 'bg-zinc-900/65 border-zinc-800/90'
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-semibold text-zinc-100">{sec.title}</span>
                  <span
                    className={`px-2 py-0.5 rounded-md text-[10px] font-medium ${
                      isTech
                        ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'
                        : 'bg-indigo-500/15 text-indigo-300 border border-indigo-500/25'
                    }`}
                  >
                    {sec.moduleName}
                  </span>
                </div>

                {sec.notes && (
                  <p className="text-xs text-zinc-300 leading-relaxed">{sec.notes}</p>
                )}

                {secEntries.length > 0 && (
                  <div className="rounded-lg border border-zinc-800/80 bg-zinc-950/60 divide-y divide-zinc-800/70 overflow-hidden">
                    {secEntries.map(([k, v]) => {
                      const strVal = typeof v === 'object' ? JSON.stringify(v) : String(v);
                      const copyId = `${sec.moduleId}:${k}`;
                      return (
                        <div
                          key={k}
                          className="flex items-center justify-between gap-2 px-3 py-2 text-xs"
                        >
                          <span className="text-zinc-400 font-mono shrink-0">
                            {k.replace(/_/g, ' ')}
                          </span>
                          <div className="flex items-center gap-2 min-w-0">
                            <span className="text-zinc-100 font-mono truncate">{strVal}</span>
                            <button
                              type="button"
                              onClick={() => void copyValue(copyId, strVal)}
                              className="p-0.5 rounded hover:bg-zinc-800 text-zinc-400 hover:text-zinc-100 shrink-0"
                              title="Copy value"
                            >
                              {copiedKey === copyId ? (
                                <Check className="w-3.5 h-3.5 text-emerald-400" />
                              ) : (
                                <Copy className="w-3.5 h-3.5" />
                              )}
                            </button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}

                {sec.actions && sec.actions.length > 0 && (
                  <div className="flex flex-wrap gap-2 pt-1">
                    {sec.actions.map((act) => (
                      <button
                        type="button"
                        key={act.id}
                        disabled={running}
                        onClick={() => onExecute(act, extraArgs)}
                        className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium transition"
                      >
                        {act.type === 'copy' ? (
                          <Copy className="w-3.5 h-3.5" />
                        ) : (
                          <ExternalLink className="w-3.5 h-3.5" />
                        )}
                        <span>{act.label}</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            );
          })
      )}

      {/* Interactive Template Parameters for Command / Parameterized Cards */}
      {placeholders.length > 0 && (
        <div className="rounded-lg border border-zinc-800 bg-zinc-900/60 p-3 space-y-2">
          <div className="text-[10px] uppercase tracking-wider text-zinc-400">
            {powerUserMode ? 'Template Arguments' : 'Optional Details'}
          </div>
          <div className="grid grid-cols-1 gap-2">
            {placeholders.map((key) => (
              <label key={key} className="flex items-center gap-2 text-xs">
                <span className="w-20 font-mono text-zinc-400 shrink-0">{key}</span>
                <input
                  type="text"
                  value={extraArgs[key] ?? ''}
                  onChange={(e) => setExtraArgs((prev) => ({ ...prev, [key]: e.target.value }))}
                  placeholder={`Enter ${key}...`}
                  className="flex-1 bg-zinc-950 border border-zinc-700/80 rounded px-2 py-1 text-xs text-zinc-100 focus:outline-none focus:border-indigo-500"
                />
              </label>
            ))}
          </div>
        </div>
      )}

      {/* Action Buttons */}
      <div className="space-y-1.5">
        <div className="text-[10px] uppercase tracking-wider text-zinc-500">Actions</div>
        <div className="flex flex-wrap gap-2">
          {(card.actions ?? []).map((act, idx) => (
            <button
              key={act.id || idx}
              disabled={running}
              onClick={() => onExecute(act, extraArgs)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                idx === 0
                  ? 'bg-indigo-600 hover:bg-indigo-500 text-white shadow-sm'
                  : 'bg-zinc-800 hover:bg-zinc-700 text-zinc-200 border border-zinc-700/70'
              } disabled:opacity-50`}
            >
              {act.type === 'open_url' || act.type === 'open_pwa' ? (
                <ExternalLink className="w-3.5 h-3.5" />
              ) : act.type === 'copy' ? (
                <Copy className="w-3.5 h-3.5" />
              ) : act.type === 'ai_prompt' ? (
                <Sparkles className="w-3.5 h-3.5" />
              ) : (
                <Play className="w-3.5 h-3.5" />
              )}
              <span>{act.label || act.type}</span>
              {act.shortcut && (
                <kbd className="ml-1 px-1 py-0.2 text-[10px] rounded bg-black/30 text-zinc-300 font-mono">
                  {act.shortcut}
                </kbd>
              )}
            </button>
          ))}
          {(aiEnabled || powerUserMode) && (
            <button
              onClick={() => onAskAI(`Explain and summarize "${card.title}" from @${card.collection}`)}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-zinc-800/80 hover:bg-zinc-700 text-purple-300 border border-purple-500/30 transition"
            >
              <Sparkles className="w-3.5 h-3.5" />
              <span>Ask AI</span>
            </button>
          )}
        </div>
      </div>

      {/* Live Execution Output Console */}
      {actionResult && (
        <div
          className={`rounded-lg border p-3 text-xs font-mono space-y-1.5 ${
            actionResult.success
              ? 'bg-zinc-950/90 border-emerald-500/30 text-emerald-200'
              : 'bg-red-950/40 border-red-500/40 text-red-200'
          }`}
        >
          <div className="flex items-center justify-between text-[10px] text-zinc-400">
            <span>
              {actionResult.actionType.toUpperCase()} • {actionResult.durationMs}ms
            </span>
            <span className={actionResult.success ? 'text-emerald-400' : 'text-red-400'}>
              {actionResult.success ? 'SUCCESS' : 'ERROR'}
            </span>
          </div>
          {actionResult.output && (
            <pre className="whitespace-pre-wrap break-words text-[11px] max-h-40 overflow-y-auto">
              {actionResult.output}
            </pre>
          )}
          {actionResult.error && <div className="text-red-300">{actionResult.error}</div>}
        </div>
      )}

      {/* Cross-Module Synergy: Related Items in Enabled Modules */}
      {relatedCards.length > 0 && onSelectCard && (
        <div className="pt-2 border-t border-zinc-800/80 space-y-2">
          <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-wider text-zinc-400">
            <Link2 className="w-3.5 h-3.5 text-indigo-400" />
            <span>Related in Enabled Modules</span>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {relatedCards.map((rel) => (
              <button
                type="button"
                key={rel.id}
                onClick={() => onSelectCard(rel)}
                className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-zinc-900/90 hover:bg-indigo-600/20 border border-zinc-800 hover:border-indigo-500/40 text-xs text-zinc-200 transition max-w-full"
              >
                <TemplateIcon template={rel.template} className="w-3.5 h-3.5 text-indigo-300 shrink-0" />
                <span className="truncate">{rel.title}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Provenance & Source Attribution Footer (For Human Trust & AI/MCP Fact-Checking) */}
      {(() => {
        const activeSec = card.sections?.find((s) => s.moduleId === activeTab);
        const prov = activeSec?.provenance ?? card.provenance;
        const isLocalOnly = Boolean(
          activeSec?.localOnly || activeSec?.provenance?.localOnly || card.localOnly || card.provenance?.localOnly
        );
        if ((!prov || !prov.sourceName) && !isLocalOnly) return null;
        return (
          <div className="pt-2 border-t border-zinc-800/60 flex items-center justify-between gap-2 text-[11px] text-zinc-500">
            <div className="flex items-center gap-1.5 min-w-0">
              <ShieldCheck className="w-3.5 h-3.5 text-emerald-400/80 shrink-0" />
              {prov?.sourceName ? (
                <span className="truncate">
                  Source: <span className="text-zinc-400">{prov.sourceName}</span>
                  {prov.authority ? ` (${prov.authority})` : ''}
                </span>
              ) : (
                <span className="truncate text-zinc-400">Local Index</span>
              )}
              {isLocalOnly && (
                <span className="px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-300 border border-amber-500/25 text-[10px] font-medium shrink-0">
                  Local Only
                </span>
              )}
            </div>
            {prov?.verifiedAt && (
              <span className="font-mono text-[10px] text-zinc-500 shrink-0">
                Verified {prov.verifiedAt}
              </span>
            )}
          </div>
        );
      })()}
    </div>
  );
}
