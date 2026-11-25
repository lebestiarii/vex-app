package search

import (
	"context"
	"strings"
	"sync"
	"time"

	"vex/internal/indexer"
	"vex/internal/models"
)

// DynamicPluginProvider supplies on-the-fly cards from declarative plugins matching a ParsedQuery.
type DynamicPluginProvider interface {
	QueryPlugins(ctx context.Context, q ParsedQuery) ([]models.SearchCard, error)
}

// EmbedderFunc generates a vector embedding for semantic search when configured.
type EmbedderFunc func(ctx context.Context, text string) ([]float32, string, error)

// SearchResponse is returned to the Wails frontend and MCP tools for every query.
type SearchResponse struct {
	Parsed  ParsedQuery            `json:"parsed"`
	Results []indexer.SearchResult `json:"results"`
	Total   int                    `json:"total"`
}

// Service coordinates query parsing, hybrid SQLite search, pattern decoding, and dynamic plugin cards.
type Service struct {
	store    *indexer.Store
	patterns *PatternEngine

	mu             sync.RWMutex
	pluginProvider DynamicPluginProvider
	embedder       EmbedderFunc
	livePatterns   LivePatternExecutor
}

// NewService creates a SearchService backed by the hybrid SQLite Store.
func NewService(store *indexer.Store) *Service {
	return &Service{
		store:    store,
		patterns: NewPatternEngine(nil),
	}
}

// SetPatternRules updates custom user/module PatternRules evaluated alongside built-in standards.
func (s *Service) SetPatternRules(rules []models.PatternRule) {
	if s.patterns != nil {
		s.patterns.SetCustomRules(rules)
	}
}

// PatternRules returns all active compiled PatternRules (custom + built-in).
func (s *Service) PatternRules() []models.PatternRule {
	if s.patterns == nil {
		return nil
	}
	return s.patterns.Rules()
}

// SetLivePatternExecutor attaches an external MCP bridge for PatternRules with LiveMCP lookups.
func (s *Service) SetLivePatternExecutor(exec LivePatternExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.livePatterns = exec
}

// SetPluginProvider attaches a dynamic plugin manager to the search pipeline.
func (s *Service) SetPluginProvider(p DynamicPluginProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pluginProvider = p
}

// SetEmbedder attaches an optional embedding generator for semantic RAG search.
func (s *Service) SetEmbedder(fn EmbedderFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embedder = fn
}

// Parse exposes the Raycast query parser directly (useful for live UI badge rendering).
func (s *Service) Parse(raw string) ParsedQuery {
	return Parse(raw)
}

// Search executes a full Raycast-style search across indexed JSON cards and dynamic plugins.
func (s *Service) Search(ctx context.Context, rawQuery string, limit int) (*SearchResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	pq := Parse(rawQuery)

	s.mu.RLock()
	pluginProvider := s.pluginProvider
	embedder := s.embedder
	livePatterns := s.livePatterns
	s.mu.RUnlock()

	opts := indexer.SearchOptions{
		Query:      pq.Text,
		Collection: pq.Collection,
		Tags:       pq.Tags,
		Modifier:   pq.Modifier,
		Template:   pq.Template,
		Limit:      limit,
	}

	// Optional semantic embedding for hybrid RRF (especially valuable in AI RAG mode).
	if embedder != nil && pq.Text != "" {
		if vec, modelName, err := embedder(ctx, pq.Text); err == nil && len(vec) > 0 {
			opts.QueryEmbedding = vec
			opts.EmbeddingModel = modelName
		}
	}

	results, err := s.store.SearchHybrid(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Prepend instant unit conversion or 3D cargo dimensional weight calculator card when matched.
	if pq.Mode == ModeSearch {
		if convCard := TryConvertQuery(pq.Text); convCard != nil {
			results = append([]indexer.SearchResult{
				{Card: *convCard, Score: 150.0, MatchSource: "calculator"},
			}, results...)
			if len(results) > limit {
				results = results[:limit]
			}
		} else if s.patterns != nil {
			if patMatch := s.patterns.Match(ctx, pq.Text, s.store, livePatterns); patMatch != nil {
				seen := make(map[string]struct{}, len(results)+1+len(patMatch.EnrichedCards))
				prepended := make([]indexer.SearchResult, 0, 1+len(patMatch.EnrichedCards))
				prepended = append(prepended, indexer.SearchResult{
					Card:        patMatch.Card,
					Score:       145.0,
					MatchSource: "pattern",
				})
				seen[patMatch.Card.ID] = struct{}{}
				for i, ec := range patMatch.EnrichedCards {
					if _, dup := seen[ec.ID]; dup {
						continue
					}
					seen[ec.ID] = struct{}{}
					prepended = append(prepended, indexer.SearchResult{
						Card:        ec,
						Score:       140.0 - float64(i)*0.5,
						MatchSource: "pattern-enrichment",
					})
				}
				for _, r := range results {
					if _, dup := seen[r.Card.ID]; dup {
						continue
					}
					prepended = append(prepended, r)
				}
				results = prepended
				if len(results) > limit {
					results = results[:limit]
				}
			}
		}
	}

	// Merge dynamic plugin results when a plugin provider is attached and query is not AI-only.
	if pluginProvider != nil && pq.Mode != ModeAI {
		dynCards, dynErr := pluginProvider.QueryPlugins(ctx, pq)
		if dynErr == nil && len(dynCards) > 0 {
			seen := make(map[string]struct{}, len(results))
			for _, r := range results {
				seen[r.Card.ID] = struct{}{}
			}
			// Dynamic plugin cards triggered by an explicit /modifier rank right at the top.
			pluginResults := make([]indexer.SearchResult, 0, len(dynCards))
			for i, dc := range dynCards {
				if _, exists := seen[dc.ID]; exists {
					continue
				}
				seen[dc.ID] = struct{}{}
				score := 120.0 - float64(i)*0.5
				if pq.Modifier == "" {
					score = 40.0 - float64(i)*0.5
				}
				pluginResults = append(pluginResults, indexer.SearchResult{
					Card:        dc,
					Score:       score,
					MatchSource: "plugin",
				})
			}
			if pq.Modifier != "" {
				results = append(pluginResults, results...)
			} else {
				results = append(results, pluginResults...)
			}
			if len(results) > limit {
				results = results[:limit]
			}
		}
	}

	// If no results and user typed a URL-like query, offer a quick open-URL fallback card.
	if len(results) == 0 && (strings.HasPrefix(pq.Text, "http://") || strings.HasPrefix(pq.Text, "https://")) {
		urlCard := models.SearchCard{
			ID:         "builtin-quick-url",
			Title:      "Open URL",
			Subtitle:   pq.Text,
			Collection: "system",
			Template:   models.TemplateAppShortcut,
			Icon:       "globe",
			Actions: []models.CardAction{
				{ID: "open", Label: "Open in Browser", Shortcut: "Enter", Type: "open_url", Target: pq.Text},
				{ID: "pwa", Label: "Open as Web App", Shortcut: "Cmd+Enter", Type: "open_pwa", Target: pq.Text},
			},
			UpdatedAt: time.Now().UTC(),
		}
		results = append(results, indexer.SearchResult{
			Card:        urlCard,
			Score:       1.0,
			MatchSource: "fallback",
		})
	}

	return &SearchResponse{
		Parsed:  pq,
		Results: results,
		Total:   len(results),
	}, nil
}
