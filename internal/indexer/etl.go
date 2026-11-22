package indexer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"vex/internal/models"
)

var templatePlaceholderRe = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// SampleRawData inspects a CSV, TSV, JSON, or JSONL payload and returns headers + up to 5 sample rows.
func SampleRawData(raw []byte, filenameHint string) (*models.RawDataSample, error) {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // Strip UTF-8 BOM
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("raw data is empty")
	}

	format := detectFormat(trimmed, filenameHint)
	rows, detectedRoot, err := parseRawRecords(trimmed, format, "")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("no records found in raw data")
	}

	fieldSet := make(map[string]struct{})
	for i := 0; i < len(rows) && i < 20; i++ {
		for k := range rows[i] {
			fieldSet[k] = struct{}{}
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for k := range fieldSet {
		fields = append(fields, k)
	}
	sort.Strings(fields)

	sampleCount := min(5, len(rows))
	return &models.RawDataSample{
		Format:           format,
		Fields:           fields,
		SampleRows:       rows[:sampleCount],
		TotalRows:        len(rows),
		DetectedRootPath: detectedRoot,
	}, nil
}

// TransformWithRule converts a CSV, TSV, JSON, or JSONL payload into validated SearchCard records.
func TransformWithRule(raw []byte, rule models.ETLMappingRule) ([]models.SearchCard, error) {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("raw data is empty")
	}
	if strings.TrimSpace(rule.TitleField) == "" {
		return nil, errors.New("rule.titleField is required")
	}

	format := strings.ToLower(strings.TrimSpace(rule.SourceFormat))
	if format == "" {
		format = detectFormat(trimmed, "")
	}

	rows, _, err := parseRawRecords(trimmed, format, rule.RootPath)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("no records matched rule")
	}

	collection := models.Slugify(rule.TargetCollection)
	if collection == "" || collection == "item" {
		if strings.TrimSpace(rule.TargetCollection) == "" {
			collection = "imported"
		}
	}
	template := rule.DefaultTemplate
	if template == "" {
		template = models.TemplateKnowledge
	}

	now := time.Now().UTC()
	cards := make([]models.SearchCard, 0, len(rows))
	for _, row := range rows {
		title := resolveFieldExpr(row, rule.TitleField)
		if strings.TrimSpace(title) == "" {
			continue
		}

		card := models.SearchCard{
			Title:       title,
			Subtitle:    resolveFieldExpr(row, rule.SubtitleField),
			Description: resolveFieldExpr(row, rule.DescriptionField),
			Collection:  collection,
			Template:    template,
			Icon:        resolveFieldExpr(row, rule.IconField),
			Modifiers:   append([]string(nil), rule.Modifiers...),
			Data:        make(map[string]any),
			UpdatedAt:   now,
		}
		if card.Icon == "" {
			card.Icon = rule.DefaultIcon
		}

		// Tags from staticTags + tagsField
		card.Tags = append(card.Tags, rule.StaticTags...)
		if rule.TagsField != "" {
			if rawTags, ok := lookupNested(row, rule.TagsField); ok {
				switch v := rawTags.(type) {
				case []any:
					for _, item := range v {
						card.Tags = append(card.Tags, fmt.Sprintf("%v", item))
					}
				case string:
					for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' }) {
						card.Tags = append(card.Tags, strings.TrimSpace(part))
					}
				}
			}
		}

		// Action
		if rule.ActionType != "" && rule.ActionTarget != "" {
			target := resolveFieldExpr(row, rule.ActionTarget)
			if target != "" {
				card.Actions = append(card.Actions, models.CardAction{
					Label:    rule.ActionLabel,
					Shortcut: "Enter",
					Type:     rule.ActionType,
					Target:   target,
				})
			}
		}

		// Preserve structured data fields for RAG & kv-grid rendering
		if len(rule.DataFields) > 0 {
			for _, f := range rule.DataFields {
				if val, ok := lookupNested(row, f); ok && val != nil && fmt.Sprintf("%v", val) != "" {
					card.Data[f] = val
				}
			}
		} else {
			for k, val := range row {
				if val != nil && fmt.Sprintf("%v", val) != "" {
					card.Data[k] = val
				}
			}
		}

		if err := models.ValidateAndNormalizeCard(&card, collection); err != nil {
			return nil, fmt.Errorf("transformed card %q invalid: %w", title, err)
		}
		cards = append(cards, card)
	}

	if len(cards) == 0 {
		return nil, errors.New("ETL rule produced 0 valid cards (check titleField)")
	}
	return cards, nil
}

func detectFormat(trimmed []byte, filenameHint string) string {
	ext := strings.ToLower(filepath.Ext(filenameHint))
	switch ext {
	case ".csv":
		return "csv"
	case ".tsv":
		return "tsv"
	case ".jsonl", ".ndjson":
		return "jsonl"
	case ".json":
		return "json"
	}

	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		// Check if it's JSONL (multiple lines each starting with '{')
		if trimmed[0] == '{' && bytes.Contains(trimmed, []byte("\n")) {
			var probe any
			if err := json.Unmarshal(trimmed, &probe); err != nil {
				return "jsonl"
			}
		}
		return "json"
	}

	// Sniff first line for tabs vs commas
	firstLine := string(trimmed)
	if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	if strings.Count(firstLine, "\t") > strings.Count(firstLine, ",") {
		return "tsv"
	}
	return "csv"
}

func parseRawRecords(trimmed []byte, format, rootPath string) ([]map[string]any, string, error) {
	switch format {
	case "csv", "tsv":
		delim := ','
		if format == "tsv" {
			delim = '\t'
		}
		r := csv.NewReader(bytes.NewReader(trimmed))
		r.Comma = delim
		r.FieldsPerRecord = -1 // tolerate jagged CSVs
		r.LazyQuotes = true

		headers, err := r.Read()
		if err != nil {
			return nil, "", fmt.Errorf("read %s headers: %w", format, err)
		}
		for i := range headers {
			headers[i] = strings.TrimSpace(headers[i])
		}

		var rows []map[string]any
		for {
			rec, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, "", fmt.Errorf("read %s row: %w", format, err)
			}
			m := make(map[string]any, len(headers))
			nonEmpty := false
			for i, h := range headers {
				if h == "" {
					continue
				}
				var val string
				if i < len(rec) {
					val = strings.TrimSpace(rec[i])
				}
				if val != "" {
					nonEmpty = true
				}
				m[h] = val
			}
			if nonEmpty {
				rows = append(rows, m)
			}
		}
		return rows, "", nil

	case "jsonl":
		scanner := bufio.NewScanner(bytes.NewReader(trimmed))
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		var rows []map[string]any
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal(line, &obj); err != nil {
				return nil, "", fmt.Errorf("parse jsonl line: %w", err)
			}
			rows = append(rows, obj)
		}
		return rows, "", scanner.Err()

	case "json":
		var root any
		if err := json.Unmarshal(trimmed, &root); err != nil {
			return nil, "", fmt.Errorf("parse json: %w", err)
		}

		target := root
		detectedPath := rootPath
		if rootPath != "" {
			val, ok := lookupPathAny(root, rootPath)
			if !ok {
				return nil, "", fmt.Errorf("rootPath %q not found in JSON", rootPath)
			}
			target = val
		} else if obj, ok := root.(map[string]any); ok {
			// Auto-detect first nested array of objects (e.g., {"items": [...]} or {"roots": {"bookmark_bar": {"children": [...]}}})
			if foundArr, foundPath := findFirstObjectArray(obj, ""); foundArr != nil {
				target = foundArr
				detectedPath = foundPath
			} else {
				// Single object JSON
				return []map[string]any{obj}, "", nil
			}
		}

		arr, ok := target.([]any)
		if !ok {
			if singleObj, ok := target.(map[string]any); ok {
				return []map[string]any{singleObj}, detectedPath, nil
			}
			return nil, detectedPath, errors.New("JSON target is not an array of objects")
		}
		rows := make([]map[string]any, 0, len(arr))
		for _, elem := range arr {
			if m, ok := elem.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
		return rows, detectedPath, nil

	default:
		return nil, "", fmt.Errorf("unsupported ETL sourceFormat %q", format)
	}
}

func findFirstObjectArray(obj map[string]any, prefix string) ([]any, string) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Breadth-first check direct array children first
	for _, k := range keys {
		fullPath := k
		if prefix != "" {
			fullPath = prefix + "." + k
		}
		if arr, ok := obj[k].([]any); ok && len(arr) > 0 {
			if _, isObj := arr[0].(map[string]any); isObj {
				return arr, fullPath
			}
		}
	}
	// Recurse into nested maps
	for _, k := range keys {
		fullPath := k
		if prefix != "" {
			fullPath = prefix + "." + k
		}
		if sub, ok := obj[k].(map[string]any); ok {
			if arr, path := findFirstObjectArray(sub, fullPath); arr != nil {
				return arr, path
			}
		}
	}
	return nil, ""
}

func lookupPathAny(node any, path string) (any, bool) {
	if path == "" {
		return node, true
	}
	parts := strings.Split(path, ".")
	cur := node
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := m[p]
		if !exists {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func lookupNested(row map[string]any, key string) (any, bool) {
	if v, ok := row[key]; ok {
		return v, true
	}
	// Case-insensitive top-level fallback
	for k, v := range row {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	if strings.Contains(key, ".") {
		return lookupPathAny(row, key)
	}
	return nil, false
}

func resolveFieldExpr(row map[string]any, expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	if strings.Contains(expr, "{{") {
		out := templatePlaceholderRe.ReplaceAllStringFunc(expr, func(match string) string {
			sub := templatePlaceholderRe.FindStringSubmatch(match)
			if len(sub) < 2 {
				return ""
			}
			val, ok := lookupNested(row, strings.TrimSpace(sub[1]))
			if !ok || val == nil {
				return ""
			}
			return strings.TrimSpace(fmt.Sprintf("%v", val))
		})
		return strings.TrimSpace(out)
	}
	if val, ok := lookupNested(row, expr); ok && val != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", val))
	}
	return ""
}

// LoadCollectionFile reads a JSON collection file supporting both raw []SearchCard and CardCollectionFile envelope.
func LoadCollectionFile(path string) (string, []models.SearchCard, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	defaultCol := models.Slugify(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))

	if len(raw) == 0 {
		return defaultCol, nil, nil
	}

	if raw[0] == '[' {
		var cards []models.SearchCard
		if err := json.Unmarshal(raw, &cards); err != nil {
			return "", nil, fmt.Errorf("parse card array %s: %w", path, err)
		}
		for i := range cards {
			if err := models.ValidateAndNormalizeCard(&cards[i], defaultCol); err != nil {
				return "", nil, fmt.Errorf("card %d in %s: %w", i, path, err)
			}
		}
		return defaultCol, cards, nil
	}

	var envelope models.CardCollectionFile
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", nil, fmt.Errorf("parse collection envelope %s: %w", path, err)
	}
	col := defaultCol
	if strings.TrimSpace(envelope.Collection) != "" {
		col = models.Slugify(envelope.Collection)
	}
	for i := range envelope.Cards {
		if err := models.ValidateAndNormalizeCard(&envelope.Cards[i], col); err != nil {
			return "", nil, fmt.Errorf("card %d in %s: %w", i, path, err)
		}
	}
	return col, envelope.Cards, nil
}

// SaveCollectionFile atomically writes a CardCollectionFile JSON to dataDir/<collection>.json.
func SaveCollectionFile(dataDir, collection string, cards []models.SearchCard) (string, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	col := models.Slugify(collection)
	for i := range cards {
		if err := models.ValidateAndNormalizeCard(&cards[i], col); err != nil {
			return "", err
		}
	}

	envelope := models.CardCollectionFile{
		Collection: col,
		Cards:      cards,
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')

	targetPath := filepath.Join(dataDir, col+".json")
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return targetPath, nil
}

// UpsertCardToDisk inserts or updates a single card inside dataDir/<card.Collection>.json.
func UpsertCardToDisk(dataDir string, card models.SearchCard) (string, error) {
	if err := models.ValidateAndNormalizeCard(&card, card.Collection); err != nil {
		return "", err
	}
	path, _, err := BatchUpsertCardsToDisk(dataDir, card.Collection, []models.SearchCard{card})
	return path, err
}

// BatchUpsertCardsToDisk merges cards by ID into dataDir/<collection>.json atomically.
func BatchUpsertCardsToDisk(dataDir, collection string, incoming []models.SearchCard) (string, int, error) {
	col := models.Slugify(collection)
	targetPath := filepath.Join(dataDir, col+".json")

	var existing []models.SearchCard
	if _, err := os.Stat(targetPath); err == nil {
		_, loaded, loadErr := LoadCollectionFile(targetPath)
		if loadErr == nil {
			existing = loaded
		}
	}

	idxByID := make(map[string]int, len(existing))
	for i, c := range existing {
		idxByID[c.ID] = i
	}

	for _, c := range incoming {
		if err := models.ValidateAndNormalizeCard(&c, col); err != nil {
			return "", 0, err
		}
		if pos, ok := idxByID[c.ID]; ok {
			existing[pos] = c
		} else {
			idxByID[c.ID] = len(existing)
			existing = append(existing, c)
		}
	}

	savedPath, err := SaveCollectionFile(dataDir, col, existing)
	if err != nil {
		return "", 0, err
	}
	return savedPath, len(incoming), nil
}

// SeedDefaultCollections creates starter JSON collections if dataDir contains no JSON files yet.
func SeedDefaultCollections(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			return nil // Already has user data
		}
	}

	now := time.Now().UTC()

	shortcuts := []models.SearchCard{
		{
			ID:          "shortcuts-github",
			Title:       "GitHub Dashboard",
			Subtitle:    "Open GitHub repositories and pull requests",
			Description: "Quick shortcut to GitHub web app. Supports searching repositories via modifier /gh.",
			Collection:  "shortcuts",
			Template:    models.TemplateAppShortcut,
			Icon:        "globe",
			Tags:        []string{"dev", "web", "git"},
			Modifiers:   []string{"gh", "bm"},
			Data:        map[string]any{"url": "https://github.com", "mode": "browser"},
			Actions: []models.CardAction{
				{ID: "open", Label: "Open in Browser", Shortcut: "Enter", Type: "open_url", Target: "https://github.com/{{arg1}}"},
				{ID: "pwa", Label: "Open as Standalone Web App", Shortcut: "Cmd+Enter", Type: "open_pwa", Target: "https://github.com"},
			},
			UpdatedAt: now,
		},
		{
			ID:          "shortcuts-localhost",
			Title:       "Localhost Dev Server",
			Subtitle:    "http://localhost:3000",
			Description: "Jump directly to your local development web app.",
			Collection:  "shortcuts",
			Template:    models.TemplateAppShortcut,
			Icon:        "laptop",
			Tags:        []string{"dev", "local"},
			Modifiers:   []string{"local", "dev"},
			Data:        map[string]any{"port": 3000, "url": "http://localhost:3000"},
			Actions: []models.CardAction{
				{ID: "open", Label: "Open Localhost", Shortcut: "Enter", Type: "open_url", Target: "http://localhost:3000"},
				{ID: "copy", Label: "Copy URL", Shortcut: "Cmd+C", Type: "copy", Target: "http://localhost:3000"},
			},
			UpdatedAt: now,
		},
	}

	knowledge := []models.SearchCard{
		{
			ID:          "knowledge-wails-v3",
			Title:       "Vex Quick Guide & Modifiers",
			Subtitle:    "How to use /, @, #, >, and ? in the omnibar",
			Description: "Use /modifier to scope commands, @collection to filter JSON files, #tag to filter tags, and ? or Tab to ask the AI RAG assistant grounded in your local JSON cards.",
			Collection:  "knowledge",
			Template:    models.TemplateKnowledge,
			Icon:        "book-open",
			Tags:        []string{"help", "guide", "rag"},
			Modifiers:   []string{"help", "docs"},
			Data: map[string]any{
				"modifiers":  "/alias",
				"collection": "@name",
				"tags":       "#tag",
				"ai_rag":     "? question",
			},
			Actions: []models.CardAction{
				{ID: "ask", Label: "Ask AI about this Guide", Shortcut: "Enter", Type: "ai_prompt", Target: "Summarize how to use Vex modifiers"},
			},
			UpdatedAt: now,
		},
	}

	devops := []models.SearchCard{
		{
			ID:          "devops-git-status",
			Title:       "System Uptime & Date Snippet",
			Subtitle:    "Copy quick local health check command",
			Description: "Demonstrates a parameterized command snippet card that copies safely to your clipboard.",
			Collection:  "devops",
			Template:    models.TemplateCommand,
			Icon:        "terminal",
			Tags:        []string{"ops", "snippet", "system"},
			Modifiers:   []string{"ops", "sys"},
			Data:        map[string]any{"command": "uname -a && date"},
			Actions: []models.CardAction{
				{ID: "copy", Label: "Copy Command", Shortcut: "Enter", Type: "copy", Target: "uname -a && date"},
			},
			UpdatedAt: now,
		},
		{
			ID:          "devops-env-cheatsheet",
			Title:       "Staging Cluster Endpoints",
			Subtitle:    "Key-Value Reference Card",
			Description: "Example kv-grid card storing structured metadata for instant lookup and AI RAG grounding.",
			Collection:  "devops",
			Template:    models.TemplateKeyValue,
			Icon:        "server",
			Tags:        []string{"staging", "k8s", "ops"},
			Modifiers:   []string{"k8s", "env"},
			Data: map[string]any{
				"cluster":   "staging-us-west-2",
				"api_url":   "https://api.staging.internal",
				"namespace": "search-platform",
				"mcp_port":  "17380",
			},
			Actions: []models.CardAction{
				{ID: "copy-api", Label: "Copy API URL", Shortcut: "Enter", Type: "copy", Target: "https://api.staging.internal"},
			},
			UpdatedAt: now,
		},
	}

	if _, err := SaveCollectionFile(dataDir, "shortcuts", shortcuts); err != nil {
		return err
	}
	if _, err := SaveCollectionFile(dataDir, "knowledge", knowledge); err != nil {
		return err
	}
	if _, err := SaveCollectionFile(dataDir, "devops", devops); err != nil {
		return err
	}
	return nil
}
