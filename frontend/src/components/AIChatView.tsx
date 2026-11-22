import { ArrowLeft, Loader2, Sparkles } from 'lucide-react';
import type { RAGResponse, RAGSource } from '../types/models';

interface AIChatViewProps {
  query: string;
  loading: boolean;
  response: RAGResponse | null;
  error: string | null;
  onSelectSource: (source: RAGSource) => void;
  onBackToSearch: () => void;
}

export function AIChatView({
  query,
  loading,
  response,
  error,
  onSelectSource,
  onBackToSearch,
}: AIChatViewProps) {
  return (
    <div className="flex flex-col h-full p-4 overflow-y-auto space-y-4">
      <div className="flex items-center justify-between border-b border-zinc-800 pb-3">
        <div className="flex items-center gap-2 text-purple-300">
          <Sparkles className="w-4 h-4" />
          <span className="text-xs font-semibold uppercase tracking-wider">Vex AI</span>
          {response && (
            <span className="text-[11px] text-zinc-400 font-mono">
              ({response.provider}/{response.model} • {response.durationMs}ms)
            </span>
          )}
        </div>
        <button
          onClick={onBackToSearch}
          className="flex items-center gap-1 text-xs text-zinc-400 hover:text-zinc-100 px-2 py-1 rounded hover:bg-zinc-800"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          <span>Back to Search (Tab)</span>
        </button>
      </div>

      {!loading && !response && !error && (
        <div className="flex-1 flex flex-col items-center justify-center text-center text-zinc-400 py-10 space-y-2">
          <Sparkles className="w-8 h-8 text-purple-400/70" />
          <p className="text-sm text-zinc-200 font-medium">Ask anything about your local JSON cards</p>
          <p className="text-xs text-zinc-500 max-w-md">
            Type a question in the omnibar above and press <kbd className="px-1.5 py-0.5 rounded bg-zinc-800 text-zinc-300">Enter</kbd>.
            Scope with <code className="text-indigo-300">@collection</code> or <code className="text-emerald-300">#tag</code>.
          </p>
        </div>
      )}

      {loading && (
        <div className="flex items-center gap-2.5 text-sm text-purple-200 bg-purple-500/10 border border-purple-500/25 rounded-lg p-4">
          <Loader2 className="w-4 h-4 animate-spin" />
          <span>Retrieving hybrid cards & generating grounded answer for "{query}"...</span>
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-red-500/40 bg-red-950/40 p-3 text-xs text-red-200">
          {error}
        </div>
      )}

      {response && (
        <>
          <div className="rounded-xl bg-zinc-900/90 border border-zinc-800 p-4 text-sm text-zinc-100 leading-relaxed whitespace-pre-wrap">
            {response.answer}
          </div>

          {response.sources.length > 0 && (
            <div className="space-y-2">
              <div className="text-[10px] uppercase tracking-wider text-zinc-500">
                Grounded Sources ({response.sources.length})
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {response.sources.map((src) => (
                  <button
                    key={src.cardId}
                    onClick={() => onSelectSource(src)}
                    className="text-left rounded-lg border border-zinc-800 bg-zinc-900/60 hover:bg-zinc-800/80 hover:border-indigo-500/40 p-2.5 transition flex items-start gap-2.5"
                  >
                    <span className="px-1.5 py-0.5 text-[10px] font-mono font-semibold rounded bg-purple-500/20 text-purple-300 shrink-0">
                      [{src.index}]
                    </span>
                    <div className="min-w-0">
                      <div className="text-xs font-medium text-zinc-100 truncate">{src.title}</div>
                      <div className="text-[11px] text-zinc-400 truncate">
                        @{src.collection} • {src.template} • {src.matchSource}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
