package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CardTemplateType defines how a search card renders and behaves in the UI.
type CardTemplateType string

const (
	TemplateAppShortcut CardTemplateType = "shortcut"    // App, URL, or PWA launcher
	TemplateKnowledge   CardTemplateType = "knowledge"   // Rich text/markdown/JSON fields for RAG & lookup
	TemplateCommand     CardTemplateType = "command"     // Parameterized shell/script/webhook action
	TemplateKeyValue    CardTemplateType = "kv-grid"     // Structured metadata card (server IPs, cheatsheets)
	TemplateMetric      CardTemplateType = "metric-card" // Dynamic data widget rendered from script/HTTP output
)

// ValidTemplates lists all supported built-in card templates.
var ValidTemplates = map[CardTemplateType]bool{
	TemplateAppShortcut: true,
	TemplateKnowledge:   true,
	TemplateCommand:     true,
	TemplateKeyValue:    true,
	TemplateMetric:      true,
}

// ValidActionTypes lists all supported built-in action types.
var ValidActionTypes = map[string]bool{
	"open_url":   true,
	"open_pwa":   true,
	"launch_app": true,
	"copy":       true,
	"http":       true,
	"ai_prompt":  true,
}

// CardAction defines an executable action associated with a SearchCard.
type CardAction struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Shortcut string            `json:"shortcut,omitempty"` // e.g. "Enter", "Cmd+K", "Cmd+C"
	Type     string            `json:"type"`               // open_url, open_pwa, launch_app, copy, http, ai_prompt
	Target   string            `json:"target"`             // URL, HTTP endpoint, or clipboard value
	Env      map[string]string `json:"env,omitempty"`
}

// Provenance tracks where a card or section's information originated, its authority tier,
// and when it was verified so humans, AI RAG, and MCP clients can fact-check freshness and resolve conflicts.
type Provenance struct {
	SourceName string `json:"sourceName,omitempty"` // e.g. "UNECE UN/LOCODE & IATA", "U.S. Treasury OFAC", "GitHub Catalog"
	SourceURL  string `json:"sourceUrl,omitempty"`  // Canonical reference URL or internal doc link
	VerifiedAt string `json:"verifiedAt,omitempty"` // ISO date e.g. "2026-11-21"
	Authority  string `json:"authority,omitempty"`  // "official" | "internal-curated" | "automated-sync"
	LocalOnly  bool   `json:"localOnly,omitempty"`  // Exclude from external cloud LLM RAG calls
}

// CardSection represents an attributed, self-contained facet contributed by a module
// (e.g., an "Air Freight", "Ocean Freight", "Customs", or "Global Technology" block on an AMS card).
type CardSection struct {
	ModuleID   string         `json:"moduleId"`
	ModuleName string         `json:"moduleName"`
	Title      string         `json:"title"`
	Badge      string         `json:"badge,omitempty"`
	Notes      string         `json:"notes,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
	Actions    []CardAction   `json:"actions,omitempty"`
	Provenance *Provenance    `json:"provenance,omitempty"`
	LocalOnly  bool           `json:"localOnly,omitempty"`
}

// SearchCard is the canonical unit stored in JSON files and indexed for search & RAG.
type SearchCard struct {
	ID          string           `json:"id"`
	EntityKey   string           `json:"entityKey,omitempty"`   // Canonical shared entity key across modules, e.g. "loc:AMS"
	ModuleID    string           `json:"moduleId,omitempty"`    // Originating module ID if provided by a module
	Title       string           `json:"title"`
	Subtitle    string           `json:"subtitle,omitempty"`
	Description string           `json:"description,omitempty"`
	Collection  string           `json:"collection"`
	Template    CardTemplateType `json:"template"`
	Icon        string           `json:"icon,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Modifiers   []string         `json:"modifiers,omitempty"`
	GlancePills []string         `json:"glancePills,omitempty"` // 2-3 high-signal summary pills shown on the compact card row
	Data        map[string]any   `json:"data,omitempty"`
	Sections    []CardSection    `json:"sections,omitempty"`    // Composed module overlay sections
	RelatedKeys []string         `json:"relatedKeys,omitempty"` // Synergy keys linking related cards across modules (e.g. ["AMS", "NL"])
	Actions     []CardAction     `json:"actions,omitempty"`
	Provenance  *Provenance      `json:"provenance,omitempty"`
	LocalOnly   bool             `json:"localOnly,omitempty"` // Exclude from external cloud LLM RAG calls
	UpdatedAt   time.Time        `json:"updatedAt"`
}

// CardOverlay enriches an existing base card (matched by TargetEntityKey or TargetCardID)
// when its parent module is enabled, or promotes to a standalone card if no base card is active.
type CardOverlay struct {
	TargetEntityKey string      `json:"targetEntityKey,omitempty"` // e.g. "loc:AMS"
	TargetCardID    string      `json:"targetCardId,omitempty"`    // e.g. "port-nlams"
	FallbackTitle   string      `json:"fallbackTitle,omitempty"`   // Used when rendered standalone without the base module
	FallbackCol     string      `json:"fallbackCollection,omitempty"`
	Section         CardSection `json:"section"`
	AddTags         []string    `json:"addTags,omitempty"`
	AddModifiers    []string    `json:"addModifiers,omitempty"`
	AddRelatedKeys  []string    `json:"addRelatedKeys,omitempty"`
}

// ModuleManifest is a shareable, toggleable domain knowledge & template pack.
type ModuleManifest struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Category       string        `json:"category"` // e.g. "Core & Locations", "Product & Operations", "Compliance", "Technology"
	Description    string        `json:"description"`
	Icon           string        `json:"icon,omitempty"`
	Version        string        `json:"version"`
	DefaultEnabled bool          `json:"defaultEnabled"`
	LocalOnly      bool          `json:"localOnly,omitempty"`
	Cards          []SearchCard  `json:"cards,omitempty"`
	Overlays       []CardOverlay `json:"overlays,omitempty"`
	Patterns       []PatternRule `json:"patterns,omitempty"`
}

// ModuleStatus summarizes an installed module and its current enabled state for the UI.
type ModuleStatus struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Description    string `json:"description"`
	Icon           string `json:"icon,omitempty"`
	Version        string `json:"version"`
	Enabled        bool   `json:"enabled"`
	DefaultEnabled bool   `json:"defaultEnabled"`
	BuiltIn        bool   `json:"builtIn"`
	LocalOnly      bool   `json:"localOnly,omitempty"`
	CardCount      int    `json:"cardCount"`
	OverlayCount   int    `json:"overlayCount"`
}

// CardCollectionFile is an optional envelope structure for user JSON files.
type CardCollectionFile struct {
	Collection string       `json:"collection"`
	Cards      []SearchCard `json:"cards"`
}

// ETLMappingRule describes how to transform rows from a CSV/TSV or arbitrary JSON file into SearchCards.
type ETLMappingRule struct {
	Name             string           `json:"name"`
	SourceFormat     string           `json:"sourceFormat"`               // "csv", "tsv", "json", "jsonl"
	RootPath         string           `json:"rootPath,omitempty"`         // Dot path for nested JSON array, e.g. "items"
	TargetCollection string           `json:"targetCollection"`           // Collection name to save into
	DefaultTemplate  CardTemplateType `json:"defaultTemplate"`            // Fallback template type
	TitleField       string           `json:"titleField"`                 // Column/key name or "{{FieldA}} - {{FieldB}}"
	SubtitleField    string           `json:"subtitleField,omitempty"`    // Column/key name or template
	DescriptionField string           `json:"descriptionField,omitempty"` // Column/key name or template
	TagsField        string           `json:"tagsField,omitempty"`        // Comma-separated string or array column
	StaticTags       []string         `json:"staticTags,omitempty"`       // Tags applied to every imported card
	Modifiers        []string         `json:"modifiers,omitempty"`        // Modifiers applied to every imported card
	IconField        string           `json:"iconField,omitempty"`        // Column/key name for icon
	DefaultIcon      string           `json:"defaultIcon,omitempty"`      // Fallback icon
	ActionType       string           `json:"actionType,omitempty"`       // e.g. "open_url", "copy", "http"
	ActionLabel      string           `json:"actionLabel,omitempty"`      // e.g. "Open Link"
	ActionTarget     string           `json:"actionTarget,omitempty"`     // e.g. "{{url}}" or "ssh {{host}}"
	DataFields       []string         `json:"dataFields,omitempty"`       // Fields to preserve in card.Data (empty = all)
}

// RawDataSample holds a preview of an external CSV/JSON file for human or AI schema mapping.
type RawDataSample struct {
	Format           string           `json:"format"` // csv, tsv, json, jsonl
	Fields           []string         `json:"fields"`
	SampleRows       []map[string]any `json:"sampleRows"`
	TotalRows        int              `json:"totalRows"`
	DetectedRootPath string           `json:"detectedRootPath,omitempty"`
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a string into a safe lowercase identifier segment.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "item"
	}
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "-")
	}
	return s
}

// ValidateAndNormalizeCard fills sensible defaults and validates required fields.
func ValidateAndNormalizeCard(card *SearchCard, defaultCollection string) error {
	if card == nil {
		return errors.New("card is nil")
	}

	card.Title = strings.TrimSpace(card.Title)
	if card.Title == "" {
		return errors.New("card title is required")
	}

	card.Collection = strings.TrimSpace(card.Collection)
	if card.Collection == "" {
		card.Collection = strings.TrimSpace(defaultCollection)
	}
	if card.Collection == "" {
		card.Collection = "default"
	}
	card.Collection = Slugify(card.Collection)

	if card.Template == "" {
		card.Template = TemplateKnowledge
	}
	if !ValidTemplates[card.Template] {
		return fmt.Errorf("invalid card template %q (valid: shortcut, knowledge, command, kv-grid, metric-card)", card.Template)
	}

	card.ID = strings.TrimSpace(card.ID)
	if card.ID == "" {
		h := sha256.Sum256([]byte(card.Collection + ":" + card.Title + ":" + card.Subtitle))
		card.ID = fmt.Sprintf("%s-%s-%s", card.Collection, Slugify(card.Title), hex.EncodeToString(h[:4]))
	}

	// Normalize tags (lowercase, trimmed, deduplicated)
	if len(card.Tags) > 0 {
		seen := make(map[string]struct{}, len(card.Tags))
		cleanTags := make([]string, 0, len(card.Tags))
		for _, t := range card.Tags {
			t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(t, "#")))
			if t == "" {
				continue
			}
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				cleanTags = append(cleanTags, t)
			}
		}
		card.Tags = cleanTags
	}

	// Normalize modifiers (lowercase, trimmed, without leading slash, deduplicated)
	if len(card.Modifiers) > 0 {
		seen := make(map[string]struct{}, len(card.Modifiers))
		cleanMods := make([]string, 0, len(card.Modifiers))
		for _, m := range card.Modifiers {
			m = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(m, "/")))
			if m == "" {
				continue
			}
			if _, ok := seen[m]; !ok {
				seen[m] = struct{}{}
				cleanMods = append(cleanMods, m)
			}
		}
		card.Modifiers = cleanMods
	}

	// Validate and normalize actions
	for i := range card.Actions {
		act := &card.Actions[i]
		act.Type = strings.ToLower(strings.TrimSpace(act.Type))
		if !ValidActionTypes[act.Type] {
			return fmt.Errorf("action %d has invalid type %q", i, act.Type)
		}
		act.Label = strings.TrimSpace(act.Label)
		if act.Label == "" {
			act.Label = defaultActionLabel(act.Type)
		}
		act.ID = strings.TrimSpace(act.ID)
		if act.ID == "" {
			act.ID = fmt.Sprintf("act-%d-%s", i, Slugify(act.Label))
		}
	}

	if card.UpdatedAt.IsZero() {
		card.UpdatedAt = time.Now().UTC()
	}

	if len(card.GlancePills) == 0 {
		card.GlancePills = ComputeGlancePills(*card)
	}

	return nil
}

// ComputeGlancePills extracts up to 3 high-signal summary pills from a card's structured Data
// so users see the most relevant metrics/codes directly on the small card row.
func ComputeGlancePills(c SearchCard) []string {
	if len(c.Data) == 0 {
		return nil
	}
	var pills []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || len(pills) >= 3 {
			return
		}
		if len(s) > 26 {
			s = s[:25] + "…"
		}
		for _, existing := range pills {
			if strings.EqualFold(existing, s) {
				return
			}
		}
		pills = append(pills, s)
	}

	strVal := func(k string) string {
		if v, ok := c.Data[k]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		return ""
	}

	// 1. Location / Gateway cards (UN/LOCODE, IATA, Country/Currency)
	if loc := strVal("unlocode"); loc != "" {
		if iata := strVal("iata"); iata != "" && iata != loc {
			add(loc + " · " + iata)
		} else {
			add(loc)
		}
		if cur := strVal("currency"); cur != "" {
			add(cur)
		}
		if cc := strVal("country_code"); cc != "" {
			add(cc)
		}
		return pills
	}

	// 2. GitHub Repository / Taxonomy cards (Portfolio, Team, Product)
	if portfolio := strVal("portfolio"); portfolio != "" {
		add("Portfolio: " + portfolio)
		if team := strVal("team"); team != "" {
			add("Team: " + team)
		}
		if product := strVal("product"); product != "" {
			add(product)
		}
		return pills
	}

	// 3. ULD / Container / Aircraft / Trailer cards
	for _, k := range []string{
		"iata_prefix", "iso_code", "hs_code", "acronym", "event_code", "incoterm",
		"capacity_cbm", "internal_volume", "max_volume", "max_gross_weight", "max_payload",
		"base_dimensions", "general_duty", "pallet_capacity", "nose_cargo_door", "main_deck_side_door",
	} {
		if v := strVal(k); v != "" {
			add(v)
		}
	}
	return pills
}

func defaultActionLabel(actionType string) string {
	switch actionType {
	case "open_url":
		return "Open URL"
	case "open_pwa":
		return "Open Web App"
	case "launch_app":
		return "Launch App"
	case "copy":
		return "Copy to Clipboard"
	case "http":
		return "Send HTTP Request"
	case "ai_prompt":
		return "Ask AI"
	default:
		return "Run Action"
	}
}

// SearchableBody flattens Description, Data map, Sections, and Action targets for FTS5 and embeddings.
func (c SearchCard) SearchableBody() string {
	var b strings.Builder
	if c.EntityKey != "" {
		b.WriteString(c.EntityKey)
		b.WriteByte('\n')
	}
	if c.Description != "" {
		b.WriteString(c.Description)
		b.WriteByte('\n')
	}
	if len(c.Data) > 0 {
		keys := make([]string, 0, len(c.Data))
		for k := range c.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(fmt.Sprintf("%v", c.Data[k]))
			b.WriteByte('\n')
		}
	}
	for _, sec := range c.Sections {
		if sec.ModuleName != "" {
			b.WriteString(sec.ModuleName)
			b.WriteByte(' ')
		}
		if sec.Title != "" {
			b.WriteString(sec.Title)
			b.WriteByte('\n')
		}
		if sec.Notes != "" {
			b.WriteString(sec.Notes)
			b.WriteByte('\n')
		}
		if len(sec.Fields) > 0 {
			sKeys := make([]string, 0, len(sec.Fields))
			for k := range sec.Fields {
				sKeys = append(sKeys, k)
			}
			sort.Strings(sKeys)
			for _, k := range sKeys {
				b.WriteString(k)
				b.WriteString(": ")
				b.WriteString(fmt.Sprintf("%v", sec.Fields[k]))
				b.WriteByte('\n')
			}
		}
		for _, act := range sec.Actions {
			if act.Label != "" {
				b.WriteString(act.Label)
				b.WriteByte(' ')
			}
			if act.Target != "" {
				b.WriteString(act.Target)
				b.WriteByte('\n')
			}
		}
	}
	for _, act := range c.Actions {
		if act.Label != "" {
			b.WriteString(act.Label)
			b.WriteByte(' ')
		}
		if act.Target != "" {
			b.WriteString(act.Target)
			b.WriteByte('\n')
		}
	}
	if c.Provenance != nil {
		if c.Provenance.SourceName != "" {
			b.WriteString("source: ")
			b.WriteString(c.Provenance.SourceName)
			b.WriteByte(' ')
		}
		if c.Provenance.Authority != "" {
			b.WriteString(c.Provenance.Authority)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

// ContentHash computes a deterministic SHA-256 hash of the card's searchable fields.
func (c SearchCard) ContentHash() string {
	payload := struct {
		EntityKey   string           `json:"entityKey,omitempty"`
		Title       string           `json:"title"`
		Subtitle    string           `json:"subtitle"`
		Description string           `json:"description"`
		Collection  string           `json:"collection"`
		Template    CardTemplateType `json:"template"`
		Tags        []string         `json:"tags"`
		Modifiers   []string         `json:"modifiers"`
		Data        map[string]any   `json:"data"`
		Sections    []CardSection    `json:"sections,omitempty"`
		Actions     []CardAction     `json:"actions"`
	}{
		EntityKey:   c.EntityKey,
		Title:       c.Title,
		Subtitle:    c.Subtitle,
		Description: c.Description,
		Collection:  c.Collection,
		Template:    c.Template,
		Tags:        c.Tags,
		Modifiers:   c.Modifiers,
		Data:        c.Data,
		Sections:    c.Sections,
		Actions:     c.Actions,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CardJSONSchema returns the formal JSON Schema describing SearchCard and ETLMappingRule.
// Exposed via MCP (schema://search-card) and used by AI models to generate cards or ETL mappings.
func CardJSONSchema() string {
	return `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Vex SearchCard & ETL Schema",
  "description": "Schema for SearchCard items stored in ~/.vex/data/<collection>.json and ETLMappingRule objects used to convert CSV/JSON files into SearchCards.",
  "definitions": {
    "SearchCard": {
      "type": "object",
      "required": ["title", "collection", "template"],
      "properties": {
        "id": { "type": "string", "description": "Optional unique identifier (auto-generated if omitted)." },
        "entityKey": { "type": "string", "description": "Canonical cross-module entity key (e.g. loc:AMS, uld:AKE, repo:gt-customs-core-decl-api)." },
        "moduleId": { "type": "string", "description": "Originating module ID when supplied by a toggleable module." },
        "title": { "type": "string", "description": "Primary display title of the card." },
        "subtitle": { "type": "string", "description": "Secondary line shown beside or beneath the title." },
        "description": { "type": "string", "description": "Rich text or markdown body used for preview and RAG retrieval." },
        "collection": { "type": "string", "description": "Logical collection/file name (e.g. ports, air-freight, customs, shortcuts)." },
        "template": {
          "type": "string",
          "enum": ["shortcut", "knowledge", "command", "kv-grid", "metric-card"],
          "description": "UI template: shortcut (apps/URLs/PWAs), knowledge (markdown/RAG notes), command (parameterized shell/http), kv-grid (key-value cheatsheet), metric-card (dynamic status widget)."
        },
        "icon": { "type": "string", "description": "Lucide icon name (e.g. globe, terminal, book-open) or emoji." },
        "tags": { "type": "array", "items": { "type": "string" }, "description": "Filterable via #tag." },
        "modifiers": { "type": "array", "items": { "type": "string" }, "description": "Quick aliases triggered via /modifier." },
        "glancePills": { "type": "array", "items": { "type": "string" }, "description": "Up to 3 high-signal summary badges shown on compact list rows." },
        "relatedKeys": { "type": "array", "items": { "type": "string" }, "description": "Cross-card synergy keys (e.g. [\"AMS\", \"NL\"])." },
        "localOnly": { "type": "boolean", "description": "When true, excluded from external cloud LLM RAG calls while remaining searchable locally and via Ollama." },
        "provenance": {
          "type": "object",
          "description": "Source attribution & freshness metadata for human trust and LLM/MCP fact-checking.",
          "properties": {
            "sourceName": { "type": "string" },
            "sourceUrl": { "type": "string" },
            "verifiedAt": { "type": "string", "description": "ISO date e.g. 2026-11-21" },
            "authority": { "type": "string", "enum": ["official", "internal-curated", "automated-sync"] },
            "localOnly": { "type": "boolean" }
          }
        },
        "sections": {
          "type": "array",
          "description": "Attributed module overlay perspectives merged onto this card.",
          "items": {
            "type": "object",
            "properties": {
              "moduleId": { "type": "string" },
              "moduleName": { "type": "string" },
              "title": { "type": "string" },
              "badge": { "type": "string" },
              "notes": { "type": "string" },
              "fields": { "type": "object", "additionalProperties": true },
              "actions": { "type": "array" },
              "provenance": { "type": "object" },
              "localOnly": { "type": "boolean" }
            }
          }
        },
        "data": { "type": "object", "additionalProperties": true, "description": "Arbitrary structured JSON fields rendered by kv-grid/metric-card and indexed for RAG." },
        "actions": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["type", "target"],
            "properties": {
              "id": { "type": "string" },
              "label": { "type": "string" },
              "shortcut": { "type": "string", "description": "e.g. Enter, Cmd+C, Cmd+Enter" },
              "type": { "type": "string", "enum": ["open_url", "open_pwa", "launch_app", "copy", "http", "ai_prompt"] },
              "target": { "type": "string", "description": "URL, command template supporting {{arg1}} or {{data.key}}, or text to copy." },
              "env": { "type": "object", "additionalProperties": { "type": "string" } }
            }
          }
        }
      }
    },
    "ETLMappingRule": {
      "type": "object",
      "required": ["sourceFormat", "targetCollection", "defaultTemplate", "titleField"],
      "properties": {
        "name": { "type": "string" },
        "sourceFormat": { "type": "string", "enum": ["csv", "tsv", "json", "jsonl"] },
        "rootPath": { "type": "string", "description": "Dot-separated path to array inside a JSON object (e.g. items or roots.bookmark_bar.children)." },
        "targetCollection": { "type": "string" },
        "defaultTemplate": { "type": "string", "enum": ["shortcut", "knowledge", "command", "kv-grid", "metric-card"] },
        "titleField": { "type": "string", "description": "Column/key name or mustache template like {{Name}}." },
        "subtitleField": { "type": "string" },
        "descriptionField": { "type": "string" },
        "tagsField": { "type": "string" },
        "staticTags": { "type": "array", "items": { "type": "string" } },
        "modifiers": { "type": "array", "items": { "type": "string" } },
        "iconField": { "type": "string" },
        "defaultIcon": { "type": "string" },
        "actionType": { "type": "string", "enum": ["open_url", "open_pwa", "launch_app", "copy", "http", "ai_prompt"] },
        "actionLabel": { "type": "string" },
        "actionTarget": { "type": "string", "description": "e.g. {{URL}} or ssh {{User}}@{{Host}}" },
        "dataFields": { "type": "array", "items": { "type": "string" } }
      }
    },
    "PatternRule": {
      "type": "object",
      "required": ["id", "name", "regex", "titleTemplate"],
      "description": "Declarative reference-number decoder and live MCP lookup rule stored in ~/.vex/patterns/*.json or ModuleManifest.patterns.",
      "properties": {
        "id": { "type": "string" },
        "name": { "type": "string" },
        "category": { "type": "string" },
        "description": { "type": "string" },
        "regex": { "type": "string", "description": "Go RE2 regex supporting named capture groups (?P<name>...)." },
        "caseInsensitive": { "type": "boolean" },
        "checksum": { "type": "string", "enum": ["", "iata-mod7", "iso6346"] },
        "lookups": { "type": "object", "additionalProperties": { "type": "object", "additionalProperties": { "type": "string" } } },
        "titleTemplate": { "type": "string" },
        "subtitleTemplate": { "type": "string" },
        "descriptionTemplate": { "type": "string" },
        "collection": { "type": "string" },
        "template": { "type": "string", "enum": ["shortcut", "knowledge", "command", "kv-grid", "metric-card"] },
        "fields": { "type": "object", "additionalProperties": { "type": "string" } },
        "glancePills": { "type": "array", "items": { "type": "string" } },
        "linkEntityKeys": { "type": "array", "items": { "type": "string" }, "description": "Templates resolving to canonical EntityKeys (e.g. loc:SEA) to enrich with indexed cards." },
        "liveMcp": {
          "type": "object",
          "properties": {
            "serverId": { "type": "string" },
            "toolName": { "type": "string" },
            "arguments": { "type": "object", "additionalProperties": { "type": "string" } }
          }
        },
        "localOnly": { "type": "boolean" }
      }
    }
  }
}`
}
