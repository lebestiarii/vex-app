package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vex/internal/models"
	"vex/internal/search"
)

// DynamicSourceConfig defines an HTTP endpoint that returns dynamic SearchCards at query time.
type DynamicSourceConfig struct {
	Type           string            `json:"type"`                     // "http"
	URL            string            `json:"url,omitempty"`            // HTTP URL template (supports {{query}}, {{arg1}})
	Method         string            `json:"method,omitempty"`         // Default GET
	Headers        map[string]string `json:"headers,omitempty"`        // Optional HTTP headers
	TimeoutMs      int               `json:"timeoutMs,omitempty"`      // Default 3000
	ResponseFormat string            `json:"responseFormat,omitempty"` // "cards" (default), "kv-grid", "metric-card"
}

// PluginManifest is the declarative JSON schema for user-authored plugins (~/.vex/plugins/<id>/plugin.json).
type PluginManifest struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Description   string               `json:"description,omitempty"`
	Version       string               `json:"version,omitempty"`
	Icon          string               `json:"icon,omitempty"`
	Modifiers     []string             `json:"modifiers,omitempty"`
	StaticCards   []models.SearchCard  `json:"staticCards,omitempty"`
	DynamicSource *DynamicSourceConfig `json:"dynamicSource,omitempty"`
}

// Manager loads declarative plugins and serves dynamic query results.
type Manager struct {
	mu         sync.RWMutex
	plugins    map[string]PluginManifest
	order      []string
	httpClient *http.Client
}

// NewManager creates an empty plugin Manager.
func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]PluginManifest),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Register validates and registers a PluginManifest in memory.
func (m *Manager) Register(manifest PluginManifest) error {
	manifest.ID = models.Slugify(manifest.ID)
	if manifest.ID == "" || manifest.ID == "item" {
		if strings.TrimSpace(manifest.Name) != "" {
			manifest.ID = models.Slugify(manifest.Name)
		} else {
			return errors.New("plugin manifest requires id or name")
		}
	}
	if strings.TrimSpace(manifest.Name) == "" {
		manifest.Name = manifest.ID
	}
	for i, mod := range manifest.Modifiers {
		manifest.Modifiers[i] = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(mod, "/")))
	}
	for i := range manifest.StaticCards {
		if err := models.ValidateAndNormalizeCard(&manifest.StaticCards[i], "plugin-"+manifest.ID); err != nil {
			return fmt.Errorf("plugin %q staticCard %d: %w", manifest.ID, i, err)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.plugins[manifest.ID]; !exists {
		m.order = append(m.order, manifest.ID)
	}
	m.plugins[manifest.ID] = manifest
	return nil
}

// LoadDir scans pluginsDir for <plugin>/plugin.json or <plugin>.json manifests.
func (m *Manager) LoadDir(pluginsDir string) error {
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		var manifestPath string
		if entry.IsDir() {
			candidate := filepath.Join(pluginsDir, entry.Name(), "plugin.json")
			if _, err := os.Stat(candidate); err == nil {
				manifestPath = candidate
			}
		} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			manifestPath = filepath.Join(pluginsDir, entry.Name())
		}
		if manifestPath == "" {
			continue
		}
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest PluginManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			continue
		}
		_ = m.Register(manifest)
	}
	return nil
}

// ListPlugins returns all registered plugin manifests in registration order.
func (m *Manager) ListPlugins() []PluginManifest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]PluginManifest, 0, len(m.order))
	for _, id := range m.order {
		if p, ok := m.plugins[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

// QueryPlugins implements search.DynamicPluginProvider.
// It returns matching static cards and executes matching dynamic plugin sources when a /modifier matches.
func (m *Manager) QueryPlugins(ctx context.Context, q search.ParsedQuery) ([]models.SearchCard, error) {
	m.mu.RLock()
	matched := make([]PluginManifest, 0)
	for _, id := range m.order {
		p := m.plugins[id]
		if q.Modifier != "" {
			if pluginMatchesModifier(p, q.Modifier) {
				matched = append(matched, p)
			}
		} else if q.Text != "" && len(p.StaticCards) > 0 {
			matched = append(matched, p)
		}
	}
	m.mu.RUnlock()

	var results []models.SearchCard
	needle := strings.ToLower(strings.TrimSpace(q.Text))

	for _, p := range matched {
		// 1. Static cards
		for _, c := range p.StaticCards {
			if needle == "" ||
				strings.Contains(strings.ToLower(c.Title), needle) ||
				strings.Contains(strings.ToLower(c.Subtitle), needle) ||
				strings.Contains(strings.ToLower(c.Description), needle) {
				results = append(results, c)
			}
		}

		// 2. Dynamic source (only run when explicitly scoped by /modifier to keep global typing instant)
		if q.Modifier != "" && p.DynamicSource != nil {
			dynCards, err := m.runDynamicSource(ctx, p, q)
			if err == nil {
				results = append(results, dynCards...)
			}
		}
	}

	return results, nil
}

func pluginMatchesModifier(p PluginManifest, modifier string) bool {
	modifier = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(modifier), "/"))
	if modifier == "" {
		return false
	}
	if strings.EqualFold(p.ID, modifier) {
		return true
	}
	for _, mod := range p.Modifiers {
		if strings.EqualFold(mod, modifier) {
			return true
		}
	}
	return false
}

func (m *Manager) runDynamicSource(ctx context.Context, p PluginManifest, q search.ParsedQuery) ([]models.SearchCard, error) {
	src := p.DynamicSource
	timeout := time.Duration(src.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var rawOutput []byte
	var err error

	dummyCard := models.SearchCard{Title: p.Name, Collection: "plugin-" + p.ID}

	switch strings.ToLower(src.Type) {
	case "http":
		// URL-encode query for safe HTTP interpolation
		encodedQ := q
		encodedQ.Text = url.QueryEscape(q.Text)
		for i, a := range encodedQ.Args {
			encodedQ.Args[i] = url.QueryEscape(a)
		}
		targetURL := InterpolateTemplate(src.URL, dummyCard, encodedQ, nil)
		method := strings.ToUpper(strings.TrimSpace(src.Method))
		if method == "" {
			method = http.MethodGet
		}
		req, reqErr := http.NewRequestWithContext(runCtx, method, targetURL, nil)
		if reqErr != nil {
			return nil, reqErr
		}
		for k, v := range src.Headers {
			req.Header.Set(k, v)
		}
		resp, respErr := m.httpClient.Do(req)
		if respErr != nil {
			return nil, respErr
		}
		defer resp.Body.Close()
		rawOutput, err = io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported dynamicSource type %q", src.Type)
	}

	return parseDynamicOutput(p, rawOutput)
}

func parseDynamicOutput(p PluginManifest, raw []byte) ([]models.SearchCard, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}

	col := "plugin-" + p.ID
	format := strings.ToLower(strings.TrimSpace(p.DynamicSource.ResponseFormat))

	if format == "" || format == "cards" {
		if trimmed[0] == '[' {
			var cards []models.SearchCard
			if err := json.Unmarshal(trimmed, &cards); err == nil {
				valid := make([]models.SearchCard, 0, len(cards))
				for _, c := range cards {
					if c.Icon == "" {
						c.Icon = p.Icon
					}
					if err := models.ValidateAndNormalizeCard(&c, col); err == nil {
						valid = append(valid, c)
					}
				}
				return valid, nil
			}
		}
		// Single SearchCard object?
		if trimmed[0] == '{' {
			var single models.SearchCard
			if err := json.Unmarshal(trimmed, &single); err == nil && single.Title != "" {
				if single.Icon == "" {
					single.Icon = p.Icon
				}
				if err := models.ValidateAndNormalizeCard(&single, col); err == nil {
					return []models.SearchCard{single}, nil
				}
			}
		}
	}

	// Wrap arbitrary JSON object into a metric-card or kv-grid widget card
	var dataMap map[string]any
	if err := json.Unmarshal(trimmed, &dataMap); err == nil && len(dataMap) > 0 {
		tpl := models.TemplateMetric
		if format == "kv-grid" {
			tpl = models.TemplateKeyValue
		}
		card := models.SearchCard{
			ID:         fmt.Sprintf("plugin-%s-live", p.ID),
			Title:      p.Name,
			Subtitle:   p.Description,
			Collection: col,
			Template:   tpl,
			Icon:       p.Icon,
			Modifiers:  p.Modifiers,
			Data:       dataMap,
			Actions: []models.CardAction{
				{ID: "copy-json", Label: "Copy JSON Output", Shortcut: "Enter", Type: "copy", Target: string(trimmed)},
			},
			UpdatedAt: time.Now().UTC(),
		}
		_ = models.ValidateAndNormalizeCard(&card, col)
		return []models.SearchCard{card}, nil
	}

	// Plain-text fallback card
	textOut := string(trimmed)
	card := models.SearchCard{
		ID:          fmt.Sprintf("plugin-%s-out", p.ID),
		Title:       p.Name,
		Subtitle:    firstLine(textOut),
		Description: textOut,
		Collection:  col,
		Template:    models.TemplateKnowledge,
		Icon:        p.Icon,
		Modifiers:   p.Modifiers,
		Actions: []models.CardAction{
			{ID: "copy-out", Label: "Copy Output", Shortcut: "Enter", Type: "copy", Target: textOut},
		},
		UpdatedAt: time.Now().UTC(),
	}
	_ = models.ValidateAndNormalizeCard(&card, col)
	return []models.SearchCard{card}, nil
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
