package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"vex/internal/indexer"
	"vex/internal/models"
	"vex/internal/search"
)

// ProviderType identifies the configured LLM/embedding backend.
type ProviderType string

const (
	ProviderOllama    ProviderType = "ollama"
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderGemini    ProviderType = "gemini"
)

// Config holds user-configurable AI provider, model, and RAG settings.
type Config struct {
	Provider        ProviderType `json:"provider"`
	BaseURL         string       `json:"baseUrl"`
	APIKey          string       `json:"apiKey,omitempty"`
	ChatModel       string       `json:"chatModel"`
	EmbeddingModel  string       `json:"embeddingModel,omitempty"`
	Temperature     float64      `json:"temperature,omitempty"`
	MaxContextCards int          `json:"maxContextCards,omitempty"`
}

// DefaultConfig returns sensible local-first defaults (Ollama).
func DefaultConfig() Config {
	return Config{
		Provider:        ProviderOllama,
		BaseURL:         "http://localhost:11434",
		ChatModel:       "llama3.2",
		EmbeddingModel:  "nomic-embed-text",
		Temperature:     0.2,
		MaxContextCards: 8,
	}
}

// RAGRequest scopes a grounded question against indexed SearchCards.
type RAGRequest struct {
	Query      string   `json:"query"`
	Collection string   `json:"collection,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// RAGSource identifies a SearchCard cited in a RAG answer.
type RAGSource struct {
	Index       int                     `json:"index"`
	CardID      string                  `json:"cardId"`
	Title       string                  `json:"title"`
	Subtitle    string                  `json:"subtitle,omitempty"`
	Collection  string                  `json:"collection"`
	Template    models.CardTemplateType `json:"template"`
	Score       float64                 `json:"score"`
	MatchSource string                  `json:"matchSource"`
}

// RAGResponse contains the grounded LLM answer and its cited source cards.
type RAGResponse struct {
	Answer     string      `json:"answer"`
	Sources    []RAGSource `json:"sources"`
	Model      string      `json:"model"`
	Provider   string      `json:"provider"`
	DurationMs int64       `json:"durationMs"`
}

// ETLMappingPreview bundles an AI-inferred ETLMappingRule with sample preview cards.
type ETLMappingPreview struct {
	Rule         models.ETLMappingRule `json:"rule"`
	Sample       *models.RawDataSample `json:"sample"`
	PreviewCards []models.SearchCard   `json:"previewCards"`
	TotalCards   int                   `json:"totalCards"`
}

type chatMessage struct {
	Role    string
	Content string
}

// Service orchestrates embeddings, grounded RAG chat, and AI-assisted CSV/JSON ETL mapping.
type Service struct {
	searchSvc *search.Service
	store     *indexer.Store

	mu         sync.RWMutex
	cfg        Config
	httpClient *http.Client
}

// NewService constructs an AI Service backed by the search and indexer layers.
func NewService(searchSvc *search.Service, store *indexer.Store, cfg Config) *Service {
	cfg = normalizeConfig(cfg)
	return &Service{
		searchSvc: searchSvc,
		store:     store,
		cfg:       cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// SetHTTPClient overrides the HTTP client (used for custom timeouts or sandbox-safe RoundTripper tests).
func (s *Service) SetHTTPClient(c *http.Client) {
	if c == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.httpClient = c
}

// GetConfig returns a copy of the active AI configuration.
func (s *Service) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// SetConfig updates the active AI provider and model configuration.
func (s *Service) SetConfig(cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = normalizeConfig(cfg)
}

func normalizeConfig(cfg Config) Config {
	if cfg.Provider == "" {
		cfg.Provider = ProviderOllama
	}
	cfg.Provider = ProviderType(strings.ToLower(strings.TrimSpace(string(cfg.Provider))))
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		switch cfg.Provider {
		case ProviderOllama:
			cfg.BaseURL = "http://localhost:11434"
		case ProviderOpenAI:
			cfg.BaseURL = "https://api.openai.com/v1"
		case ProviderAnthropic:
			cfg.BaseURL = "https://api.anthropic.com/v1"
		case ProviderGemini:
			cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
		}
	}
	if cfg.ChatModel == "" {
		switch cfg.Provider {
		case ProviderOllama:
			cfg.ChatModel = "llama3.2"
		case ProviderOpenAI:
			cfg.ChatModel = "gpt-4o-mini"
		case ProviderAnthropic:
			cfg.ChatModel = "claude-3-5-sonnet-latest"
		case ProviderGemini:
			cfg.ChatModel = "gemini-2.5-flash"
		}
	}
	if cfg.MaxContextCards <= 0 {
		cfg.MaxContextCards = 8
	}
	if cfg.Temperature < 0 {
		cfg.Temperature = 0
	}
	return cfg
}

// Embed generates a vector embedding using the configured provider.
// Satisfies search.EmbedderFunc directly.
func (s *Service) Embed(ctx context.Context, text string) ([]float32, string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", errors.New("text to embed is empty")
	}

	s.mu.RLock()
	cfg := s.cfg
	client := s.httpClient
	s.mu.RUnlock()

	if strings.TrimSpace(cfg.EmbeddingModel) == "" {
		return nil, "", errors.New("embeddingModel is not configured")
	}

	switch cfg.Provider {
	case ProviderOllama:
		vec, err := embedOllama(ctx, client, cfg, text)
		return vec, cfg.EmbeddingModel, err
	case ProviderOpenAI:
		vec, err := embedOpenAI(ctx, client, cfg, text)
		return vec, cfg.EmbeddingModel, err
	case ProviderGemini:
		vec, err := embedGemini(ctx, client, cfg, text)
		return vec, cfg.EmbeddingModel, err
	case ProviderAnthropic:
		return nil, "", errors.New("anthropic does not provide a native embeddings endpoint; use Ollama, OpenAI, or Gemini for vector search or rely on FTS5 BM25")
	default:
		return nil, "", fmt.Errorf("unsupported AI provider %q", cfg.Provider)
	}
}

// SyncMissingEmbeddings generates and persists embeddings for all cards that lack one
// or whose content hash has changed since their last embedding.
func (s *Service) SyncMissingEmbeddings(ctx context.Context) (int, error) {
	if s.store == nil {
		return 0, errors.New("indexer store is nil")
	}
	cfg := s.GetConfig()
	if strings.TrimSpace(cfg.EmbeddingModel) == "" {
		return 0, errors.New("embeddingModel is not configured")
	}

	cards, err := s.store.CardsNeedingEmbedding(ctx, cfg.EmbeddingModel)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, card := range cards {
		select {
		case <-ctx.Done():
			return updated, ctx.Err()
		default:
		}
		filtered, allowed := filterCardForProvider(card, cfg.Provider)
		if !allowed {
			continue
		}
		docText := formatCardForEmbedding(filtered)
		vec, modelName, err := s.Embed(ctx, docText)
		if err != nil {
			return updated, fmt.Errorf("embed card %q: %w", card.ID, err)
		}
		if err := s.store.UpsertEmbedding(ctx, card.ID, modelName, card.ContentHash(), vec); err != nil {
			return updated, fmt.Errorf("store embedding for %q: %w", card.ID, err)
		}
		updated++
	}
	return updated, nil
}

func formatCardForEmbedding(c models.SearchCard) string {
	var b strings.Builder
	b.WriteString(c.Title)
	if c.Subtitle != "" {
		b.WriteString(" — ")
		b.WriteString(c.Subtitle)
	}
	if len(c.Tags) > 0 {
		b.WriteString("\nTags: ")
		b.WriteString(strings.Join(c.Tags, ", "))
	}
	if body := c.SearchableBody(); body != "" {
		b.WriteByte('\n')
		b.WriteString(body)
	}
	return b.String()
}

// AskRAG retrieves top-K hybrid cards, builds a grounded prompt with citations,
// streams chunks via onChunk (if non-nil), and returns the full RAGResponse.
func (s *Service) AskRAG(ctx context.Context, req RAGRequest, onChunk func(chunk string)) (*RAGResponse, error) {
	start := time.Now()
	rawQuery := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.Query), "?"))
	if rawQuery == "" {
		return nil, errors.New("RAG query is required")
	}

	s.mu.RLock()
	cfg := s.cfg
	client := s.httpClient
	s.mu.RUnlock()

	limit := req.Limit
	if limit <= 0 {
		limit = cfg.MaxContextCards
	}

	// Parse any inline @collection or #tag modifiers from the user's question and merge with explicit filters.
	pq := search.Parse(rawQuery)
	collection := req.Collection
	if collection == "" {
		collection = pq.Collection
	}
	tags := mergeUniqueStrings(req.Tags, pq.Tags)
	queryText := pq.Text
	if queryText == "" {
		queryText = rawQuery
	}

	var results []indexer.SearchResult
	if s.store != nil {
		opts := indexer.SearchOptions{
			Query:      queryText,
			Collection: collection,
			Tags:       tags,
			Modifier:   pq.Modifier,
			Template:   pq.Template,
			Limit:      limit,
		}
		if cfg.EmbeddingModel != "" {
			if vec, modelName, err := s.Embed(ctx, queryText); err == nil && len(vec) > 0 {
				opts.QueryEmbedding = vec
				opts.EmbeddingModel = modelName
			}
		}
		var err error
		results, err = s.store.SearchHybrid(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("retrieve RAG context: %w", err)
		}
		// Fallback to recent cards in scope if lexical/vector query returned 0 matches.
		if len(results) == 0 && (collection != "" || len(tags) > 0) {
			opts.Query = ""
			opts.QueryEmbedding = nil
			results, _ = s.store.SearchHybrid(ctx, opts)
		}
	}

	sources := make([]RAGSource, 0, len(results))
	var ctxBuilder strings.Builder
	for _, r := range results {
		filteredCard, allowed := filterCardForProvider(r.Card, cfg.Provider)
		if !allowed {
			continue
		}
		idx := len(sources) + 1
		sources = append(sources, RAGSource{
			Index:       idx,
			CardID:      filteredCard.ID,
			Title:       filteredCard.Title,
			Subtitle:    filteredCard.Subtitle,
			Collection:  filteredCard.Collection,
			Template:    filteredCard.Template,
			Score:       r.Score,
			MatchSource: r.MatchSource,
		})
		ctxBuilder.WriteString(formatCardForRAG(idx, filteredCard))
		ctxBuilder.WriteString("\n\n")
	}

	systemPrompt := buildRAGSystemPrompt(ctxBuilder.String(), len(sources))
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: queryText},
	}

	answer, err := completeChat(ctx, client, cfg, messages, onChunk)
	if err != nil {
		return nil, err
	}

	return &RAGResponse{
		Answer:     strings.TrimSpace(answer),
		Sources:    sources,
		Model:      cfg.ChatModel,
		Provider:   string(cfg.Provider),
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

func isCardLocalOnly(card models.SearchCard) bool {
	return card.LocalOnly || (card.Provenance != nil && card.Provenance.LocalOnly)
}

func isSectionLocalOnly(sec models.CardSection) bool {
	return sec.LocalOnly || (sec.Provenance != nil && sec.Provenance.LocalOnly)
}

// filterCardForProvider enforces the localOnly enterprise guardrail:
// local Ollama retains full access, while external cloud providers skip localOnly cards
// and strip localOnly overlay sections.
func filterCardForProvider(card models.SearchCard, provider ProviderType) (models.SearchCard, bool) {
	if provider == ProviderOllama {
		return card, true
	}
	if isCardLocalOnly(card) {
		return models.SearchCard{}, false
	}
	if len(card.Sections) > 0 {
		kept := make([]models.CardSection, 0, len(card.Sections))
		for _, sec := range card.Sections {
			if !isSectionLocalOnly(sec) {
				kept = append(kept, sec)
			}
		}
		card.Sections = kept
	}
	return card, true
}

func formatCardForRAG(index int, card models.SearchCard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%d] Title: %s (ID: %s | Collection: @%s | Template: %s)", index, card.Title, card.ID, card.Collection, card.Template)
	if card.Subtitle != "" {
		fmt.Fprintf(&b, "\nSubtitle: %s", card.Subtitle)
	}
	if card.Provenance != nil {
		fmt.Fprintf(&b, "\nProvenance: Source=%q | Authority=%q | VerifiedAt=%q | URL=%q",
			card.Provenance.SourceName, card.Provenance.Authority, card.Provenance.VerifiedAt, card.Provenance.SourceURL)
	}
	if len(card.Tags) > 0 {
		fmt.Fprintf(&b, "\nTags: %s", strings.Join(card.Tags, ", "))
	}
	if card.Description != "" {
		fmt.Fprintf(&b, "\nDescription: %s", card.Description)
	}
	if len(card.Data) > 0 {
		if raw, err := json.Marshal(card.Data); err == nil {
			fmt.Fprintf(&b, "\nData: %s", string(raw))
		}
	}
	for _, sec := range card.Sections {
		fmt.Fprintf(&b, "\nSection [%s — %s]: %s", sec.ModuleName, sec.Title, sec.Notes)
		if len(sec.Fields) > 0 {
			if raw, err := json.Marshal(sec.Fields); err == nil {
				fmt.Fprintf(&b, " | Fields: %s", string(raw))
			}
		}
	}
	if len(card.Actions) > 0 {
		var acts []string
		for _, a := range card.Actions {
			acts = append(acts, fmt.Sprintf("%s (%s: %s)", a.Label, a.Type, a.Target))
		}
		fmt.Fprintf(&b, "\nActions: %s", strings.Join(acts, "; "))
	}
	return b.String()
}

func buildRAGSystemPrompt(contextBlock string, sourceCount int) string {
	var b strings.Builder
	b.WriteString("You are Vex AI, a concise local knowledge assistant.\n")
	b.WriteString("Answer the user's question using ONLY the retrieved SearchCard context below whenever possible.\n")
	b.WriteString("Cite relevant cards inline using bracketed numbers like [1] or [2].\n")
	b.WriteString("When cards include Provenance (Source, Authority, VerifiedAt), prefer higher-authority ('official') and more recently verified sources if two sources conflict, and mention the provenance when relevant.\n")
	b.WriteString("If the retrieved cards do not contain enough information, clearly state what is missing.\n\n")
	if sourceCount == 0 {
		b.WriteString("Retrieved Context: (No matching cards found in the local knowledge index.)\n")
	} else {
		b.WriteString("Retrieved Context:\n")
		b.WriteString(contextBlock)
	}
	return b.String()
}

// GenerateETLMapping asks the configured LLM to inspect a RawDataSample (CSV/TSV/JSON/JSONL)
// and emit a valid models.ETLMappingRule conforming to Vex's SearchCard schema.
func (s *Service) GenerateETLMapping(ctx context.Context, sample *models.RawDataSample, userHint string) (*models.ETLMappingRule, error) {
	if sample == nil {
		return nil, errors.New("raw data sample is nil")
	}
	if len(sample.Fields) == 0 && len(sample.SampleRows) == 0 {
		return nil, errors.New("raw data sample has no fields or rows")
	}

	s.mu.RLock()
	cfg := s.cfg
	client := s.httpClient
	s.mu.RUnlock()

	sampleJSON, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return nil, err
	}

	systemPrompt := `You are a data-engineering schema mapper for Vex.
Given a raw CSV/TSV/JSON/JSONL data sample and the canonical SearchCard/ETLMappingRule JSON Schema, return ONLY a single valid JSON object matching ETLMappingRule.
Rules:
1. Do NOT include any markdown explanation outside the JSON object.
2. Choose the best defaultTemplate: "shortcut" for links/URLs/apps, "command" for CLI/SSH/scripts, "kv-grid" for structured records/servers/contacts, "metric-card" for KPI/status rows, or "knowledge" for notes/docs.
3. Use exact field names from sample.fields (or mustache expressions like "{{FirstName}} {{LastName}}" or "ssh {{user}}@{{host}}").
4. If URLs are present, set actionType to "open_url" and actionTarget to "{{<urlField>}}". If commands/hosts are present, set actionType to "copy".
5. Set sourceFormat to the sample's format and rootPath to sample.detectedRootPath when applicable.`

	var userPrompt strings.Builder
	userPrompt.WriteString("Canonical JSON Schema:\n")
	userPrompt.WriteString(models.CardJSONSchema())
	userPrompt.WriteString("\n\nRaw Data Sample:\n")
	userPrompt.Write(sampleJSON)
	if strings.TrimSpace(userHint) != "" {
		userPrompt.WriteString("\n\nUser Guidance:\n")
		userPrompt.WriteString(strings.TrimSpace(userHint))
	}
	userPrompt.WriteString("\n\nReturn ONLY the ETLMappingRule JSON object:")

	// Keep temperature low for deterministic structured JSON output.
	mapCfg := cfg
	mapCfg.Temperature = 0.0

	rawResp, err := completeChat(ctx, client, mapCfg, []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt.String()},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("AI schema mapping failed: %w", err)
	}

	jsonPayload, err := extractJSONObject(rawResp)
	if err != nil {
		return nil, fmt.Errorf("extract ETLMappingRule JSON from model output: %w", err)
	}

	var rule models.ETLMappingRule
	if err := json.Unmarshal([]byte(jsonPayload), &rule); err != nil {
		return nil, fmt.Errorf("parse ETLMappingRule JSON: %w", err)
	}

	normalizeETLRule(&rule, sample)
	if strings.TrimSpace(rule.TitleField) == "" {
		return nil, errors.New("AI-generated ETLMappingRule is missing required titleField")
	}
	return &rule, nil
}

// GenerateETLMappingWithPreview samples raw CSV/JSON bytes, asks the AI to infer an ETLMappingRule,
// and transforms the data into live preview SearchCards (capped at 10 cards in PreviewCards).
func (s *Service) GenerateETLMappingWithPreview(ctx context.Context, rawBytes []byte, filenameHint, userHint string) (*ETLMappingPreview, error) {
	sample, err := indexer.SampleRawData(rawBytes, filenameHint)
	if err != nil {
		return nil, err
	}
	rule, err := s.GenerateETLMapping(ctx, sample, userHint)
	if err != nil {
		return nil, err
	}
	cards, err := indexer.TransformWithRule(rawBytes, *rule)
	if err != nil {
		return nil, fmt.Errorf("preview transform with AI rule: %w", err)
	}
	preview := cards
	if len(preview) > 10 {
		preview = preview[:10]
	}
	return &ETLMappingPreview{
		Rule:         *rule,
		Sample:       sample,
		PreviewCards: preview,
		TotalCards:   len(cards),
	}, nil
}

func normalizeETLRule(rule *models.ETLMappingRule, sample *models.RawDataSample) {
	if strings.TrimSpace(rule.SourceFormat) == "" && sample != nil {
		rule.SourceFormat = sample.Format
	}
	if strings.TrimSpace(rule.RootPath) == "" && sample != nil && sample.DetectedRootPath != "" {
		rule.RootPath = sample.DetectedRootPath
	}
	if strings.TrimSpace(rule.TargetCollection) == "" {
		rule.TargetCollection = "imported"
	} else {
		rule.TargetCollection = models.Slugify(rule.TargetCollection)
	}
	if !models.ValidTemplates[rule.DefaultTemplate] {
		rule.DefaultTemplate = models.TemplateKnowledge
	}
	if rule.ActionType != "" && !models.ValidActionTypes[strings.ToLower(strings.TrimSpace(rule.ActionType))] {
		rule.ActionType = ""
		rule.ActionTarget = ""
	}
}

// extractJSONObject strips markdown code fences and extracts the first balanced JSON object.
func extractJSONObject(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("empty response")
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		rest := s[idx+3:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			s = strings.TrimSpace(rest[:end])
		}
	}

	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", errors.New("no JSON object found in response")
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", errors.New("unterminated JSON object in response")
}

func mergeUniqueStrings(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, item := range list {
			item = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(item, "#")))
			if item == "" {
				continue
			}
			if _, ok := seen[item]; !ok {
				seen[item] = struct{}{}
				out = append(out, item)
			}
		}
	}
	return out
}

// --- Provider Chat Implementations ---

func completeChat(ctx context.Context, client *http.Client, cfg Config, messages []chatMessage, onChunk func(string)) (string, error) {
	switch cfg.Provider {
	case ProviderOllama:
		return chatOllama(ctx, client, cfg, messages, onChunk)
	case ProviderOpenAI:
		return chatOpenAI(ctx, client, cfg, messages, onChunk)
	case ProviderAnthropic:
		return chatAnthropic(ctx, client, cfg, messages, onChunk)
	case ProviderGemini:
		return chatGemini(ctx, client, cfg, messages, onChunk)
	default:
		return "", fmt.Errorf("unsupported AI provider %q", cfg.Provider)
	}
}

func chatOllama(ctx context.Context, client *http.Client, cfg Config, messages []chatMessage, onChunk func(string)) (string, error) {
	type ollamaMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := make([]ollamaMsg, len(messages))
	for i, m := range messages {
		msgs[i] = ollamaMsg{Role: m.Role, Content: m.Content}
	}
	payload := map[string]any{
		"model":    cfg.ChatModel,
		"messages": msgs,
		"stream":   onChunk != nil,
		"options":  map[string]any{"temperature": cfg.Temperature},
	}
	resp, err := postJSON(ctx, client, cfg.BaseURL+"/api/chat", payload, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if onChunk != nil {
		var full strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var chunk struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				Response string `json:"response"`
				Error    string `json:"error"`
			}
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}
			if chunk.Error != "" {
				return full.String(), errors.New(chunk.Error)
			}
			delta := chunk.Message.Content
			if delta == "" {
				delta = chunk.Response
			}
			if delta != "" {
				full.WriteString(delta)
				onChunk(delta)
			}
		}
		if err := scanner.Err(); err != nil {
			return full.String(), err
		}
		return full.String(), nil
	}

	var parsed struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Response string `json:"response"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	if parsed.Error != "" {
		return "", errors.New(parsed.Error)
	}
	if parsed.Message.Content != "" {
		return parsed.Message.Content, nil
	}
	return parsed.Response, nil
}

func chatOpenAI(ctx context.Context, client *http.Client, cfg Config, messages []chatMessage, onChunk func(string)) (string, error) {
	type oaiMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := make([]oaiMsg, len(messages))
	for i, m := range messages {
		msgs[i] = oaiMsg{Role: m.Role, Content: m.Content}
	}
	payload := map[string]any{
		"model":       cfg.ChatModel,
		"messages":    msgs,
		"temperature": cfg.Temperature,
		"stream":      onChunk != nil,
	}
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}
	resp, err := postJSON(ctx, client, cfg.BaseURL+"/chat/completions", payload, headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if onChunk != nil && strings.Contains(ct, "text/event-stream") {
		var full strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(data), &chunk) == nil && len(chunk.Choices) > 0 {
				if delta := chunk.Choices[0].Delta.Content; delta != "" {
					full.WriteString(delta)
					onChunk(delta)
				}
			}
		}
		return full.String(), scanner.Err()
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("openai response contained no choices")
	}
	text := parsed.Choices[0].Message.Content
	if onChunk != nil && text != "" {
		onChunk(text)
	}
	return text, nil
}

func chatAnthropic(ctx context.Context, client *http.Client, cfg Config, messages []chatMessage, onChunk func(string)) (string, error) {
	var systemParts []string
	var conv []map[string]string
	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
		} else {
			conv = append(conv, map[string]string{"role": m.Role, "content": m.Content})
		}
	}
	payload := map[string]any{
		"model":       cfg.ChatModel,
		"max_tokens":  2048,
		"temperature": cfg.Temperature,
		"system":      strings.Join(systemParts, "\n\n"),
		"messages":    conv,
		"stream":      onChunk != nil,
	}
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
	}
	if cfg.APIKey != "" {
		headers["x-api-key"] = cfg.APIKey
	}
	resp, err := postJSON(ctx, client, cfg.BaseURL+"/messages", payload, headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if onChunk != nil && strings.Contains(ct, "text/event-stream") {
		var full strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var ev struct {
				Type  string `json:"type"`
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &ev) == nil && ev.Type == "content_block_delta" && ev.Delta.Text != "" {
				full.WriteString(ev.Delta.Text)
				onChunk(ev.Delta.Text)
			}
		}
		return full.String(), scanner.Err()
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode anthropic response: %w", err)
	}
	var out strings.Builder
	for _, blk := range parsed.Content {
		if blk.Type == "text" || blk.Text != "" {
			out.WriteString(blk.Text)
		}
	}
	text := out.String()
	if onChunk != nil && text != "" {
		onChunk(text)
	}
	return text, nil
}

func chatGemini(ctx context.Context, client *http.Client, cfg Config, messages []chatMessage, onChunk func(string)) (string, error) {
	var systemParts []string
	var contents []map[string]any
	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
			continue
		}
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": m.Content}},
		})
	}
	payload := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"temperature": cfg.Temperature,
		},
	}
	if len(systemParts) > 0 {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]string{{"text": strings.Join(systemParts, "\n\n")}},
		}
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent", cfg.BaseURL, cfg.ChatModel)
	if onChunk != nil {
		endpoint = fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", cfg.BaseURL, cfg.ChatModel)
	}
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["x-goog-api-key"] = cfg.APIKey
	}
	resp, err := postJSON(ctx, client, endpoint, payload, headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	extractGeminiText := func(raw []byte) string {
		var g struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if json.Unmarshal(raw, &g) != nil || len(g.Candidates) == 0 {
			return ""
		}
		var b strings.Builder
		for _, p := range g.Candidates[0].Content.Parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if onChunk != nil && strings.Contains(ct, "text/event-stream") {
		var full strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if delta := extractGeminiText([]byte(data)); delta != "" {
				full.WriteString(delta)
				onChunk(delta)
			}
		}
		return full.String(), scanner.Err()
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return "", err
	}
	text := extractGeminiText(body)
	if text == "" {
		return "", errors.New("gemini response contained no candidate text")
	}
	if onChunk != nil {
		onChunk(text)
	}
	return text, nil
}

// --- Provider Embedding Implementations ---

func embedOllama(ctx context.Context, client *http.Client, cfg Config, text string) ([]float32, error) {
	payload := map[string]any{
		"model": cfg.EmbeddingModel,
		"input": text,
	}
	resp, err := postJSON(ctx, client, cfg.BaseURL+"/api/embed", payload, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Embeddings [][]float32 `json:"embeddings"`
		Embedding  []float32   `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode ollama embed response: %w", err)
	}
	if len(parsed.Embeddings) > 0 && len(parsed.Embeddings[0]) > 0 {
		return parsed.Embeddings[0], nil
	}
	if len(parsed.Embedding) > 0 {
		return parsed.Embedding, nil
	}
	return nil, errors.New("ollama embed response contained empty vector")
}

func embedOpenAI(ctx context.Context, client *http.Client, cfg Config, text string) ([]float32, error) {
	payload := map[string]any{
		"model": cfg.EmbeddingModel,
		"input": text,
	}
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}
	resp, err := postJSON(ctx, client, cfg.BaseURL+"/embeddings", payload, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode openai embed response: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, errors.New("openai embed response contained empty vector")
	}
	return parsed.Data[0].Embedding, nil
}

func embedGemini(ctx context.Context, client *http.Client, cfg Config, text string) ([]float32, error) {
	payload := map[string]any{
		"content": map[string]any{
			"parts": []map[string]string{{"text": text}},
		},
	}
	headers := map[string]string{}
	if cfg.APIKey != "" {
		headers["x-goog-api-key"] = cfg.APIKey
	}
	endpoint := fmt.Sprintf("%s/models/%s:embedContent", cfg.BaseURL, cfg.EmbeddingModel)
	resp, err := postJSON(ctx, client, endpoint, payload, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode gemini embed response: %w", err)
	}
	if len(parsed.Embedding.Values) == 0 {
		return nil, errors.New("gemini embed response contained empty vector")
	}
	return parsed.Embedding.Values, nil
}

func postJSON(ctx context.Context, client *http.Client, targetURL string, payload any, headers map[string]string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("AI HTTP %d from %s: %s", resp.StatusCode, targetURL, strings.TrimSpace(string(errBody)))
	}
	return resp, nil
}
