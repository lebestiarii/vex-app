package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ValidActionTypes lists all supported built-in card action types.
var ValidActionTypes = map[string]bool{
	"open_url":   true,
	"open_pwa":   true,
	"launch_app": true,
	"copy":       true,
	"http":       true,
}

// GlanceChip represents a styled Row-2 header chip in the Rolodex UI.
type GlanceChip struct {
	Label string `json:"label"`
	Tone  string `json:"tone"`          // "neutral" | "blue" | "amber" | "green" | "red" | "mono"
	Dot   string `json:"dot,omitempty"` // Optional stoplight indicator: "green" | "amber" | "red"
}

// CardAction defines an executable action associated with a SearchCard.
type CardAction struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Shortcut string            `json:"shortcut,omitempty"` // Platform-agnostic hint e.g. "Enter"
	Type     string            `json:"type"`               // open_url, open_pwa, launch_app, copy, http
	Target   string            `json:"target"`             // URL, HTTP endpoint, or clipboard value
	Env      map[string]string `json:"env,omitempty"`
}

// Provenance tracks where a card or section's information originated, its authority tier,
// and when it was verified so humans and MCP clients can fact-check freshness.
type Provenance struct {
	SourceName string `json:"sourceName,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
	VerifiedAt string `json:"verifiedAt,omitempty"` // ISO date e.g. "2026-11-23"
	Authority  string `json:"authority,omitempty"`  // "official" | "internal-curated" | "automated-sync"
	LocalOnly  bool   `json:"localOnly,omitempty"`
}

// CardSection represents an attributed, self-contained facet contributed by a module overlay.
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

// SearchCard is the canonical unit stored flat on disk in JSON files/modules and indexed in SQLite.
// Flat cards compose dynamically at query time via ParentEntityKey -> EntityKey (`entity_links`).
type SearchCard struct {
	ID              string         `json:"id"`
	EntityKey       string         `json:"entityKey,omitempty"`       // Canonical cross-module entity key, e.g. "loc:AMS", "air-carrier:176"
	ParentEntityKey string         `json:"parentEntityKey,omitempty"` // Flat-on-disk parent entityKey for dynamic child composition
	ModuleID        string         `json:"moduleId,omitempty"`        // Originating module ID
	Title           string         `json:"title"`
	Subtitle        string         `json:"subtitle,omitempty"`
	Description     string         `json:"description,omitempty"`
	Collection      string         `json:"collection"`
	Layout          CardLayoutType `json:"layout"`
	Icon            string         `json:"icon,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Modifiers       []string       `json:"modifiers,omitempty"`
	Chips           []GlanceChip   `json:"chips,omitempty"`           // Typed Row-2 header chips (max 4, zero echo)
	Data            map[string]any `json:"data,omitempty"`            // Strict per-layout payload (additionalProperties: false)
	Children        []SearchCard   `json:"children,omitempty"`        // Dynamically assembled (or inline) child cards (max depth 2)
	DefaultExpanded bool           `json:"defaultExpanded,omitempty"` // Whether card starts expanded
	Pinnable        *bool          `json:"pinnable,omitempty"`        // Defaults to true for top-level cards, false for children
	Sections        []CardSection  `json:"sections,omitempty"`        // Composed module overlay sections
	RelatedKeys     []string       `json:"relatedKeys,omitempty"`     // Synergy keys linking related cards across modules
	Actions         []CardAction   `json:"actions,omitempty"`
	Provenance      *Provenance    `json:"provenance,omitempty"`
	LocalOnly       bool           `json:"localOnly,omitempty"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// CardNode is the recursive UI-ready Rolodex node projected from a SearchCard.
type CardNode struct {
	ID              string         `json:"id"`
	Layout          CardLayoutType `json:"layout"`
	Icon            string         `json:"icon"`
	Title           string         `json:"title"`
	Subtitle        string         `json:"subtitle,omitempty"`
	Description     string         `json:"description,omitempty"`
	Collection      string         `json:"collection,omitempty"`
	EntityKey       string         `json:"entityKey,omitempty"`
	ParentEntityKey string         `json:"parentEntityKey,omitempty"`
	ModuleID        string         `json:"moduleId,omitempty"`
	Chips           []GlanceChip   `json:"chips"`
	Data            map[string]any `json:"data"`
	Children        []CardNode     `json:"children,omitempty"`
	Actions         []CardAction   `json:"actions,omitempty"`
	DefaultExpanded bool           `json:"defaultExpanded,omitempty"`
	Pinnable        bool           `json:"pinnable"`
}

// ToCardNode converts a SearchCard (and its assembled children) into a Rolodex CardNode (max depth 2).
func (c SearchCard) ToCardNode(depth int) CardNode {
	layout := c.Layout
	if !ValidLayouts[layout] {
		layout = LayoutFacts
	}

	chips := make([]GlanceChip, 0, len(c.Chips))
	chips = append(chips, c.Chips...)
	if len(chips) > 4 {
		chips = chips[:4]
	}

	pinnable := depth == 0
	if c.Pinnable != nil {
		pinnable = *c.Pinnable && depth == 0
	}

	data := c.Data
	if data == nil {
		data = map[string]any{}
	}

	children := make([]CardNode, 0, len(c.Children))
	if depth < 2 {
		for _, ch := range c.Children {
			children = append(children, ch.ToCardNode(depth+1))
		}
	}

	return CardNode{
		ID:              c.ID,
		Layout:          layout,
		Icon:            c.Icon,
		Title:           c.Title,
		Subtitle:        c.Subtitle,
		Description:     c.Description,
		Collection:      c.Collection,
		EntityKey:       c.EntityKey,
		ParentEntityKey: c.ParentEntityKey,
		ModuleID:        c.ModuleID,
		Chips:           chips,
		Data:            data,
		Children:        children,
		Actions:         c.Actions,
		DefaultExpanded: c.DefaultExpanded,
		Pinnable:        pinnable,
	}
}

// CardOverlay enriches an existing base card (matched by TargetEntityKey or TargetCardID)
// with either a CardSection and/or nested ChildCards when its parent module is enabled.
type CardOverlay struct {
	TargetEntityKey string       `json:"targetEntityKey,omitempty"` // e.g. "loc:AMS"
	TargetCardID    string       `json:"targetCardId,omitempty"`
	FallbackTitle   string       `json:"fallbackTitle,omitempty"`
	FallbackCol     string       `json:"fallbackCollection,omitempty"`
	Section         CardSection  `json:"section,omitempty"`
	ChildCards      []SearchCard `json:"childCards,omitempty"` // Nested child cards attached to the target card
	AddTags         []string     `json:"addTags,omitempty"`
	AddModifiers    []string     `json:"addModifiers,omitempty"`
	AddRelatedKeys  []string     `json:"addRelatedKeys,omitempty"`
}

// ModuleManifest is a shareable, optional, deletable domain knowledge & pattern pack
// stored as a JSON file in ~/.vex/modules/<id>.json.
type ModuleManifest struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Category       string        `json:"category"`
	Description    string        `json:"description"`
	Icon           string        `json:"icon,omitempty"`
	Version        string        `json:"version"`
	DefaultEnabled bool          `json:"defaultEnabled"`
	LocalOnly      bool          `json:"localOnly,omitempty"`
	Cards          []SearchCard  `json:"cards,omitempty"`
	Overlays       []CardOverlay `json:"overlays,omitempty"`
	Patterns       []PatternRule `json:"patterns,omitempty"`
}

// ModuleStatus summarizes an installed user module in ~/.vex/modules/ and its current enabled state.
type ModuleStatus struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Description    string `json:"description"`
	Icon           string `json:"icon,omitempty"`
	Version        string `json:"version"`
	Enabled        bool   `json:"enabled"`
	DefaultEnabled bool   `json:"defaultEnabled"`
	LocalOnly      bool   `json:"localOnly,omitempty"`
	CardCount      int    `json:"cardCount"`
	OverlayCount   int    `json:"overlayCount"`
	PatternCount   int    `json:"patternCount"`
	FilePath       string `json:"filePath,omitempty"`
}

// StarterModuleInfo describes a packaged optional module available to install/load at onboarding or later.
type StarterModuleInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Description    string `json:"description"`
	Icon           string `json:"icon,omitempty"`
	Version        string `json:"version"`
	DefaultEnabled bool   `json:"defaultEnabled"`
	Installed      bool   `json:"installed"`
	CardCount      int    `json:"cardCount"`
	OverlayCount   int    `json:"overlayCount"`
	PatternCount   int    `json:"patternCount"`
}

// CardCollectionFile is an optional envelope structure for user JSON files.
type CardCollectionFile struct {
	Collection string       `json:"collection"`
	Cards      []SearchCard `json:"cards"`
}

// ETLMappingRule describes how to transform rows from a CSV/TSV or arbitrary JSON file into SearchCards.
type ETLMappingRule struct {
	Name             string         `json:"name"`
	SourceFormat     string         `json:"sourceFormat"`               // "csv", "tsv", "json", "jsonl"
	RootPath         string         `json:"rootPath,omitempty"`         // Dot path for nested JSON array, e.g. "items"
	TargetCollection string         `json:"targetCollection"`           // Collection name to save into
	DefaultLayout    CardLayoutType `json:"defaultLayout"`              // Target generic layout (defaults to "facts")
	TitleField       string         `json:"titleField"`                 // Column/key name or "{{FieldA}} - {{FieldB}}"
	SubtitleField    string         `json:"subtitleField,omitempty"`    // Column/key name or template
	DescriptionField string         `json:"descriptionField,omitempty"` // Column/key name or template
	TagsField        string         `json:"tagsField,omitempty"`        // Comma-separated string or array column
	StaticTags       []string       `json:"staticTags,omitempty"`       // Tags applied to every imported card
	Modifiers        []string       `json:"modifiers,omitempty"`        // Modifiers applied to every imported card
	IconField        string         `json:"iconField,omitempty"`        // Column/key name for icon
	DefaultIcon      string         `json:"defaultIcon,omitempty"`      // Fallback icon
	ActionType       string         `json:"actionType,omitempty"`       // e.g. "open_url", "copy", "http"
	ActionLabel      string         `json:"actionLabel,omitempty"`      // e.g. "Open Link"
	ActionTarget     string         `json:"actionTarget,omitempty"`     // e.g. "{{url}}"
	DataFields       []string       `json:"dataFields,omitempty"`       // Fields to preserve as facts (empty = all)
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

// ValidateAndNormalizeCard enforces strict zero-alias 9-layout schema rules, chip/echo invariants,
// depth <= 2 nesting, and normalizes tags/modifiers/IDs/actions recursively.
func ValidateAndNormalizeCard(card *SearchCard, defaultCollection string) error {
	errs := validateAndNormalizeCardDetailed(card, defaultCollection, 0, "")
	if len(errs) > 0 {
		return ValidationResult{Valid: false, Errors: errs}.Err()
	}
	return nil
}

func validateAndNormalizeCardDetailed(card *SearchCard, defaultCollection string, depth int, prefix string) []FieldError {
	var errs []FieldError
	add := func(subPath, msg string) {
		p := prefix
		if subPath != "" {
			if p != "" {
				p = p + "." + subPath
			} else {
				p = subPath
			}
		}
		errs = append(errs, FieldError{Path: p, Message: msg})
	}

	if card == nil {
		add("", "card is nil")
		return errs
	}
	if depth > 2 {
		add("children", fmt.Sprintf("card nesting depth %d exceeds maximum of 2", depth))
		return errs
	}

	card.Title = strings.TrimSpace(card.Title)
	if card.Title == "" {
		add("title", "card title is required")
	}

	card.EntityKey = strings.TrimSpace(card.EntityKey)
	card.ParentEntityKey = strings.TrimSpace(card.ParentEntityKey)
	if card.EntityKey != "" && card.ParentEntityKey != "" && strings.EqualFold(card.EntityKey, card.ParentEntityKey) {
		add("parentEntityKey", fmt.Sprintf("card cannot set parentEntityKey equal to its own entityKey %q", card.EntityKey))
	}

	card.Collection = strings.TrimSpace(card.Collection)
	if card.Collection == "" {
		card.Collection = strings.TrimSpace(defaultCollection)
	}
	if card.Collection == "" {
		card.Collection = "default"
	}
	card.Collection = Slugify(card.Collection)

	card.Layout = CardLayoutType(strings.TrimSpace(string(card.Layout)))
	if card.Layout == "" {
		add("layout", fmt.Sprintf("card layout is required (one of: %s)", strings.Join(LayoutNames(), ", ")))
	} else if !ValidLayouts[card.Layout] {
		add("layout", fmt.Sprintf("invalid card layout %q (must be one of: %s)", card.Layout, strings.Join(LayoutNames(), ", ")))
	}

	card.ID = strings.TrimSpace(card.ID)
	if card.ID == "" && card.Title != "" {
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

	// Normalize and validate chips (max 4, valid tones/dots, zero duplicates, zero title echo)
	for i := range card.Chips {
		card.Chips[i].Label = strings.TrimSpace(card.Chips[i].Label)
		card.Chips[i].Tone = strings.ToLower(strings.TrimSpace(card.Chips[i].Tone))
		card.Chips[i].Dot = strings.ToLower(strings.TrimSpace(card.Chips[i].Dot))
		if card.Chips[i].Tone == "" {
			card.Chips[i].Tone = "neutral"
		}
	}
	errs = append(errs, validateChipsAndEcho(card.Title, card.Chips, prefix)...)

	// Strict layout payload validation
	if ValidLayouts[card.Layout] {
		errs = append(errs, validateLayoutData(card.Layout, card.Data, prefix)...)
	}

	// Validate and normalize actions
	for i := range card.Actions {
		act := &card.Actions[i]
		act.Type = strings.ToLower(strings.TrimSpace(act.Type))
		if !ValidActionTypes[act.Type] {
			add(fmt.Sprintf("actions[%d].type", i), fmt.Sprintf("invalid action type %q", act.Type))
			continue
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

	// Recursively validate inline nested child cards (max depth 2)
	for i := range card.Children {
		if card.Children[i].Collection == "" {
			card.Children[i].Collection = card.Collection
		}
		childPrefix := fmt.Sprintf("children[%d]", i)
		if prefix != "" {
			childPrefix = prefix + "." + childPrefix
		}
		errs = append(errs, validateAndNormalizeCardDetailed(&card.Children[i], card.Collection, depth+1, childPrefix)...)
	}

	if card.UpdatedAt.IsZero() {
		card.UpdatedAt = time.Now().UTC()
	}

	return errs
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
	default:
		return "Run Action"
	}
}

func isURLString(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isURLActionTarget(act CardAction) bool {
	if act.Type == "open_url" || act.Type == "open_pwa" || act.Type == "http" {
		return true
	}
	return isURLString(act.Target)
}

// ChipsText returns a space-joined string of the card's non-empty glance chip labels.
func (c SearchCard) ChipsText() string {
	if len(c.Chips) == 0 {
		return ""
	}
	parts := make([]string, 0, len(c.Chips))
	for _, ch := range c.Chips {
		if lbl := strings.TrimSpace(ch.Label); lbl != "" {
			parts = append(parts, lbl)
		}
	}
	return strings.Join(parts, " ")
}

// SearchableBody extracts only human-readable searchable text using a strict layout-aware
// whitelist across the 9 generic layouts (never indexing URLs, tones, UI flags, or JSON keys).
func (c SearchCard) SearchableBody() string {
	var b strings.Builder
	writeCardBody(&b, c, 0)
	return strings.TrimSpace(b.String())
}

func writeLine(b *strings.Builder, s string) {
	s = strings.TrimSpace(s)
	if s != "" {
		b.WriteString(s)
		b.WriteByte('\n')
	}
}

func writeCardBody(b *strings.Builder, c SearchCard, depth int) {
	if depth > 2 {
		return
	}
	if depth > 0 {
		writeLine(b, c.Title)
		writeLine(b, c.Subtitle)
		writeLine(b, c.EntityKey)
		writeLine(b, c.ChipsText())
	}
	writeLine(b, c.Description)
	writeLayoutSearchableData(b, c.Layout, c.Data)

	for _, sec := range c.Sections {
		writeLine(b, sec.ModuleName)
		writeLine(b, sec.Title)
		writeLine(b, sec.Notes)
		for _, act := range sec.Actions {
			writeActionBody(b, act)
		}
	}
	for _, act := range c.Actions {
		writeActionBody(b, act)
	}
	for _, child := range c.Children {
		writeCardBody(b, child, depth+1)
	}
	if c.Provenance != nil {
		writeLine(b, c.Provenance.SourceName)
		writeLine(b, c.Provenance.Authority)
	}
}

func writeLayoutSearchableData(b *strings.Builder, layout CardLayoutType, data map[string]any) {
	if len(data) == 0 {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}

	switch layout {
	case LayoutGroup:
		// Header-only container; data.meta holds technical metadata (ianaTimezone, lat, lon) and is never indexed as body text.
		return

	case LayoutLink:
		// data.url is a launch URL and is intentionally excluded from FTS body text.
		return

	case LayoutActionList:
		var payload ActionListLayoutData
		if json.Unmarshal(raw, &payload) != nil {
			return
		}
		for _, row := range payload.Rows {
			if row.Badge != nil {
				writeLine(b, row.Badge.Label)
			}
			writeLine(b, row.Primary)
			writeLine(b, row.Subtitle)
			for _, act := range row.Actions {
				writeLine(b, act.Label)
				if act.Type == "copy" && !isURLString(act.Value) {
					writeLine(b, act.Value)
				}
			}
		}

	case LayoutFacts:
		var payload FactsLayoutData
		if json.Unmarshal(raw, &payload) != nil {
			return
		}
		writeLine(b, payload.Summary)
		for _, f := range payload.Facts {
			writeLine(b, f.Label)
			if !isURLString(f.Value) {
				writeLine(b, f.Value)
			}
		}
		for _, v := range payload.Views {
			writeLine(b, v.Label)
			for _, f := range v.Facts {
				writeLine(b, f.Label)
				if !isURLString(f.Value) {
					writeLine(b, f.Value)
				}
			}
		}

	case LayoutSegments:
		var payload SegmentsLayoutData
		if json.Unmarshal(raw, &payload) != nil {
			return
		}
		for _, seg := range payload.Segments {
			writeLine(b, seg.Value)
			writeLine(b, seg.Role)
			writeLine(b, seg.Meaning)
		}
		writeLine(b, payload.Explanation)

	case LayoutSteps:
		var payload StepsLayoutData
		if json.Unmarshal(raw, &payload) != nil {
			return
		}
		for _, st := range payload.Steps {
			writeLine(b, st.Label)
			writeLine(b, st.Caption)
		}
		writeLine(b, payload.Summary)
		for _, bullet := range payload.Bullets {
			writeLine(b, bullet)
		}

	case LayoutConvert:
		var payload ConvertLayoutData
		if json.Unmarshal(raw, &payload) != nil {
			return
		}
		writeLine(b, payload.Input)
		writeLine(b, payload.Output)
		for _, eq := range payload.Equivalents {
			writeLine(b, eq.Label)
			writeLine(b, eq.Value)
		}

	case LayoutTable:
		var payload TableLayoutData
		if json.Unmarshal(raw, &payload) != nil {
			return
		}
		for _, col := range payload.Columns {
			writeLine(b, col)
		}
		for _, row := range payload.Rows {
			for _, cell := range row {
				if !isURLString(cell) {
					writeLine(b, cell)
				}
			}
		}

	case LayoutSnippet:
		var payload SnippetLayoutData
		if json.Unmarshal(raw, &payload) != nil {
			return
		}
		writeLine(b, payload.Language)
		writeLine(b, payload.Body)
		for _, v := range payload.Vars {
			writeLine(b, v.Label)
		}
	}
}

func writeActionBody(b *strings.Builder, act CardAction) {
	writeLine(b, act.Label)
	if !isURLActionTarget(act) {
		writeLine(b, act.Target)
	}
}

// ContentHash computes a deterministic SHA-256 hash of the card's searchable fields.
func (c SearchCard) ContentHash() string {
	payload := struct {
		EntityKey       string         `json:"entityKey,omitempty"`
		ParentEntityKey string         `json:"parentEntityKey,omitempty"`
		Title           string         `json:"title"`
		Subtitle        string         `json:"subtitle"`
		Description     string         `json:"description"`
		Collection      string         `json:"collection"`
		Layout          CardLayoutType `json:"layout"`
		Tags            []string       `json:"tags"`
		Modifiers       []string       `json:"modifiers"`
		Chips           []GlanceChip   `json:"chips,omitempty"`
		Data            map[string]any `json:"data"`
		Children        []SearchCard   `json:"children,omitempty"`
		Sections        []CardSection  `json:"sections,omitempty"`
		Actions         []CardAction   `json:"actions"`
	}{
		EntityKey:       c.EntityKey,
		ParentEntityKey: c.ParentEntityKey,
		Title:           c.Title,
		Subtitle:        c.Subtitle,
		Description:     c.Description,
		Collection:      c.Collection,
		Layout:          c.Layout,
		Tags:            c.Tags,
		Modifiers:       c.Modifiers,
		Chips:           c.Chips,
		Data:            c.Data,
		Children:        c.Children,
		Sections:        c.Sections,
		Actions:         c.Actions,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CardJSONSchema returns the strict Draft 2020-12 JSON Schema describing the 9 pure generic
// layouts, SearchCard, ModuleManifest, PatternRule, and ETLMappingRule.
func CardJSONSchema() string {
	return `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Vex Desk Rolodex Strict 9-Layout Schema",
  "description": "Strict, zero-alias JSON Schema for Vex SearchCard items (~/.vex/data/*.json), flat-on-disk ModuleManifest packs (~/.vex/modules/*.json), PatternRule definitions (~/.vex/patterns/*.json), and ETLMappingRule imports.",
  "$defs": {
    "GlanceChip": {
      "type": "object",
      "additionalProperties": false,
      "required": ["label", "tone"],
      "properties": {
        "label": { "type": "string", "minLength": 1 },
        "tone": { "type": "string", "enum": ["neutral", "blue", "amber", "green", "red", "mono"] },
        "dot": { "type": "string", "enum": ["green", "amber", "red"] }
      }
    },
    "CardAction": {
      "type": "object",
      "additionalProperties": false,
      "required": ["type", "target"],
      "properties": {
        "id": { "type": "string" },
        "label": { "type": "string" },
        "shortcut": { "type": "string" },
        "type": { "type": "string", "enum": ["open_url", "open_pwa", "launch_app", "copy", "http"] },
        "target": { "type": "string", "minLength": 1 },
        "env": { "type": "object", "additionalProperties": { "type": "string" } }
      }
    },
    "GroupLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "description": "Header-only container payload; zero body fields rendered.",
      "properties": {
        "meta": {
          "type": "object",
          "additionalProperties": { "type": ["string", "number"] },
          "properties": {
            "ianaTimezone": { "type": "string" },
            "lat": { "type": "number" },
            "lon": { "type": "number" }
          }
        }
      }
    },
    "LinkLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "required": ["url"],
      "properties": {
        "url": { "type": "string", "format": "uri", "pattern": "^https?://" }
      }
    },
    "ActionListLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "required": ["rows"],
      "properties": {
        "rows": {
          "type": "array",
          "minItems": 1,
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["primary", "actions"],
            "properties": {
              "badge": { "$ref": "#/$defs/GlanceChip" },
              "primary": { "type": "string", "minLength": 1 },
              "mono": { "type": "boolean" },
              "subtitle": { "type": "string" },
              "actions": {
                "type": "array",
                "minItems": 1,
                "items": {
                  "type": "object",
                  "additionalProperties": false,
                  "required": ["label", "type", "value"],
                  "properties": {
                    "label": { "type": "string", "minLength": 1 },
                    "type": { "type": "string", "enum": ["copy", "open_url"] },
                    "value": { "type": "string", "minLength": 1 },
                    "tone": { "type": "string", "enum": ["neutral", "blue", "amber", "green", "red", "mono"] },
                    "dot": { "type": "string", "enum": ["green", "amber", "red"] }
                  }
                }
              }
            }
          }
        }
      }
    },
    "FactItem": {
      "type": "object",
      "additionalProperties": false,
      "required": ["label", "value"],
      "properties": {
        "label": { "type": "string", "minLength": 1 },
        "value": { "type": "string", "minLength": 1 },
        "mono": { "type": "boolean" },
        "copyable": { "type": "boolean", "default": false }
      }
    },
    "FactsLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "summary": { "type": "string" },
        "facts": {
          "type": "array",
          "items": { "$ref": "#/$defs/FactItem" }
        },
        "views": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["label", "facts"],
            "properties": {
              "label": { "type": "string", "minLength": 1 },
              "facts": {
                "type": "array",
                "minItems": 1,
                "items": { "$ref": "#/$defs/FactItem" }
              }
            }
          }
        }
      }
    },
    "SegmentsLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "required": ["segments", "explanation"],
      "properties": {
        "segments": {
          "type": "array",
          "minItems": 1,
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["value", "role", "meaning", "tone"],
            "properties": {
              "value": { "type": "string", "minLength": 1 },
              "role": { "type": "string", "minLength": 1 },
              "meaning": { "type": "string", "minLength": 1 },
              "tone": { "type": "string", "enum": ["blue", "amber", "green", "neutral"] }
            }
          }
        },
        "explanation": { "type": "string", "minLength": 1 }
      }
    },
    "StepsLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "required": ["steps", "summary"],
      "properties": {
        "steps": {
          "type": "array",
          "minItems": 1,
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["label"],
            "properties": {
              "label": { "type": "string", "minLength": 1 },
              "caption": { "type": "string" }
            }
          }
        },
        "activeStep": { "type": "integer", "minimum": 1 },
        "summary": { "type": "string", "minLength": 1 },
        "bullets": { "type": "array", "items": { "type": "string" } }
      }
    },
    "ConvertLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "required": ["input", "output"],
      "properties": {
        "input": { "type": "string", "minLength": 1 },
        "output": { "type": "string", "minLength": 1 },
        "equivalents": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["label", "value"],
            "properties": {
              "label": { "type": "string", "minLength": 1 },
              "value": { "type": "string", "minLength": 1 }
            }
          }
        }
      }
    },
    "TableLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "required": ["columns", "rows"],
      "properties": {
        "columns": {
          "type": "array",
          "minItems": 2,
          "maxItems": 4,
          "items": { "type": "string", "minLength": 1 }
        },
        "rows": {
          "type": "array",
          "minItems": 1,
          "items": {
            "type": "array",
            "items": { "type": "string" }
          }
        },
        "monoColumns": {
          "type": "array",
          "items": { "type": "integer", "minimum": 0, "maximum": 3 }
        },
        "sourceUrl": { "type": "string", "format": "uri", "pattern": "^https?://" }
      }
    },
    "SnippetLayoutData": {
      "type": "object",
      "additionalProperties": false,
      "required": ["body"],
      "properties": {
        "language": { "type": "string" },
        "body": { "type": "string", "minLength": 1 },
        "vars": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["key", "label"],
            "properties": {
              "key": { "type": "string", "minLength": 1 },
              "label": { "type": "string", "minLength": 1 },
              "defaultValue": { "type": "string" }
            }
          }
        }
      }
    },
    "SearchCard": {
      "type": "object",
      "additionalProperties": false,
      "required": ["title", "collection", "layout"],
      "properties": {
        "id": { "type": "string" },
        "entityKey": { "type": "string", "description": "Canonical cross-module entity key (e.g. loc:AMS, air-carrier:176, ocean-carrier:MAEU, container:40HC, guide:hbl)." },
        "parentEntityKey": { "type": "string", "description": "Flat-on-disk parent entityKey for automatic cross-module nesting (max resolved depth: 2)." },
        "moduleId": { "type": "string" },
        "title": { "type": "string", "minLength": 1 },
        "subtitle": { "type": "string" },
        "description": { "type": "string" },
        "collection": { "type": "string", "minLength": 1 },
        "layout": {
          "type": "string",
          "enum": ["group", "link", "action-list", "facts", "segments", "steps", "convert", "table", "snippet"]
        },
        "icon": { "type": "string" },
        "tags": { "type": "array", "items": { "type": "string" } },
        "modifiers": { "type": "array", "items": { "type": "string" } },
        "chips": { "type": "array", "maxItems": 4, "items": { "$ref": "#/$defs/GlanceChip" } },
        "data": { "type": "object" },
        "children": { "type": "array", "items": { "$ref": "#/$defs/SearchCard" } },
        "defaultExpanded": { "type": "boolean" },
        "pinnable": { "type": "boolean" },
        "relatedKeys": { "type": "array", "items": { "type": "string" } },
        "actions": { "type": "array", "items": { "$ref": "#/$defs/CardAction" } },
        "localOnly": { "type": "boolean" }
      },
      "allOf": [
        { "if": { "properties": { "layout": { "const": "group" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/GroupLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "link" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/LinkLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "action-list" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/ActionListLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "facts" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/FactsLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "segments" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/SegmentsLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "steps" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/StepsLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "convert" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/ConvertLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "table" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/TableLayoutData" } } } },
        { "if": { "properties": { "layout": { "const": "snippet" } } }, "then": { "properties": { "data": { "$ref": "#/$defs/SnippetLayoutData" } } } }
      ]
    },
    "ModuleManifest": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id", "name"],
      "description": "Optional, deletable flat-on-disk module stored in ~/.vex/modules/<id>.json.",
      "properties": {
        "id": { "type": "string", "minLength": 1 },
        "name": { "type": "string", "minLength": 1 },
        "category": { "type": "string" },
        "description": { "type": "string" },
        "icon": { "type": "string" },
        "version": { "type": "string" },
        "defaultEnabled": { "type": "boolean" },
        "localOnly": { "type": "boolean" },
        "cards": { "type": "array", "items": { "$ref": "#/$defs/SearchCard" } },
        "overlays": { "type": "array" },
        "patterns": { "type": "array", "items": { "$ref": "#/$defs/PatternRule" } }
      }
    },
    "PatternRule": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id", "name", "regex", "titleTemplate"],
      "description": "Configurable pattern decoder and cross-card link rule stored in ~/.vex/patterns/*.json or ModuleManifest.patterns.",
      "properties": {
        "id": { "type": "string", "minLength": 1 },
        "name": { "type": "string", "minLength": 1 },
        "category": { "type": "string" },
        "description": { "type": "string" },
        "example": { "type": "string" },
        "enabled": { "type": "boolean", "default": true },
        "priority": { "type": "integer", "default": 0 },
        "normalize": { "type": "array", "items": { "type": "string", "enum": ["trim", "uppercase", "lowercase", "strip-spaces", "strip-dashes"] } },
        "regex": { "type": "string", "minLength": 1 },
        "caseInsensitive": { "type": "boolean" },
        "checksum": { "type": "string", "enum": ["", "iata-mod7", "iso6346", "luhn"] },
        "lookups": { "type": "object", "additionalProperties": { "type": "object", "additionalProperties": { "type": "string" } } },
        "dataLookups": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["alias", "targetEntityKey"],
            "properties": {
              "alias": { "type": "string" },
              "targetEntityKey": { "type": "string" },
              "extractFields": { "type": "object", "additionalProperties": { "type": "string" } },
              "fallbackValues": { "type": "object", "additionalProperties": { "type": "string" } }
            }
          }
        },
        "titleTemplate": { "type": "string", "minLength": 1 },
        "subtitleTemplate": { "type": "string" },
        "descriptionTemplate": { "type": "string" },
        "explanationTemplate": { "type": "string" },
        "segments": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["value", "role", "meaning"],
            "properties": {
              "value": { "type": "string" },
              "role": { "type": "string" },
              "meaning": { "type": "string" },
              "tone": { "type": "string", "enum": ["blue", "amber", "green", "neutral"] }
            }
          }
        },
        "collection": { "type": "string" },
        "layout": { "type": "string", "enum": ["group", "link", "action-list", "facts", "segments", "steps", "convert", "table", "snippet"], "default": "segments" },
        "icon": { "type": "string" },
        "defaultExpanded": { "type": "boolean" },
        "chips": { "type": "array", "maxItems": 4, "items": { "$ref": "#/$defs/GlanceChip" } },
        "fields": { "type": "object", "additionalProperties": { "type": "string" } },
        "linkEntityKeys": { "type": "array", "items": { "type": "string" } },
        "linkRules": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
              "targetEntityKey": { "type": "string" },
              "targetCollection": { "type": "string" },
              "targetTag": { "type": "string" },
              "relationLabel": { "type": "string" },
              "autoExpand": { "type": "boolean" },
              "nestChildren": { "type": "boolean" },
              "copyChildrenOnly": { "type": "boolean" },
              "attachUnder": { "type": "string" },
              "maxDepth": { "type": "integer" },
              "includeFields": { "type": "array", "items": { "type": "string" } }
            }
          }
        },
        "actions": { "type": "array", "items": { "$ref": "#/$defs/CardAction" } },
        "liveHttp": { "$ref": "#/$defs/PatternLiveHTTP" },
        "liveMcp": {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "serverId": { "type": "string" },
            "toolName": { "type": "string" },
            "arguments": { "type": "object", "additionalProperties": { "type": "string" } }
          }
        },
        "localOnly": { "type": "boolean" }
      }
    },
    "PatternLiveHTTP": {
      "type": "object",
      "additionalProperties": false,
      "required": ["urlTemplate", "resultCard"],
      "description": "Strictly read-only HTTP GET enrichment for a matched PatternRule. Inline secrets and auth headers are forbidden.",
      "properties": {
        "urlTemplate": { "type": "string", "format": "uri", "pattern": "^https?://" },
        "trigger": { "type": "string", "enum": ["auto", "manual"], "default": "manual" },
        "timeoutMs": { "type": "integer", "minimum": 1, "maximum": 5000, "default": 1500 },
        "cacheTtlSec": { "type": "integer", "minimum": 1, "maximum": 86400, "default": 60 },
        "auth": {
          "type": "string",
          "default": "none",
          "description": "Allowed: none, gh_cli, az_devops, or env:VAR_NAME. Raw tokens are rejected."
        },
        "headers": {
          "type": "object",
          "additionalProperties": { "type": "string" },
          "description": "Optional non-auth headers (e.g. Accept). Authorization/Cookie/API-Key headers are rejected."
        },
        "resultCard": { "$ref": "#/$defs/PatternLiveResultSpec" }
      }
    },
    "PatternLiveResultSpec": {
      "type": "object",
      "additionalProperties": false,
      "required": ["layout"],
      "description": "Projects JSON response dot-paths ({{json.field.sub}}) into a nested facts, table, steps, or link child card.",
      "properties": {
        "layout": { "type": "string", "enum": ["facts", "table", "steps", "link"] },
        "title": { "type": "string" },
        "icon": { "type": "string" },
        "defaultExpanded": { "type": "boolean" },
        "chips": { "type": "array", "maxItems": 4, "items": { "$ref": "#/$defs/GlanceChip" } },
        "summary": { "type": "string" },
        "facts": { "type": "array", "items": { "$ref": "#/$defs/FactItem" } },
        "url": { "type": "string" },
        "columns": { "type": "array", "minItems": 2, "maxItems": 4, "items": { "type": "string" } },
        "rowsPath": { "type": "string" },
        "rowFields": { "type": "array", "items": { "type": "string" } },
        "rows": { "type": "array", "items": { "type": "array", "items": { "type": "string" } } },
        "monoColumns": { "type": "array", "items": { "type": "integer", "minimum": 0, "maximum": 3 } },
        "sourceUrl": { "type": "string" },
        "stepsPath": { "type": "string" },
        "stepLabel": { "type": "string" },
        "stepCaption": { "type": "string" },
        "steps": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["label"],
            "properties": {
              "label": { "type": "string" },
              "caption": { "type": "string" }
            }
          }
        },
        "activeStep": { "type": "integer", "minimum": 1 },
        "activeStepPath": { "type": "string" },
        "bullets": { "type": "array", "items": { "type": "string" } }
      }
    },
    "ETLMappingRule": {
      "type": "object",
      "additionalProperties": false,
      "required": ["sourceFormat", "targetCollection", "defaultLayout", "titleField"],
      "properties": {
        "name": { "type": "string" },
        "sourceFormat": { "type": "string", "enum": ["csv", "tsv", "json", "jsonl"] },
        "rootPath": { "type": "string" },
        "targetCollection": { "type": "string" },
        "defaultLayout": { "type": "string", "enum": ["group", "link", "action-list", "facts", "segments", "steps", "convert", "table", "snippet"] },
        "titleField": { "type": "string" },
        "subtitleField": { "type": "string" },
        "descriptionField": { "type": "string" },
        "tagsField": { "type": "string" },
        "staticTags": { "type": "array", "items": { "type": "string" } },
        "modifiers": { "type": "array", "items": { "type": "string" } },
        "iconField": { "type": "string" },
        "defaultIcon": { "type": "string" },
        "actionType": { "type": "string", "enum": ["open_url", "open_pwa", "launch_app", "copy", "http"] },
        "actionLabel": { "type": "string" },
        "actionTarget": { "type": "string" },
        "dataFields": { "type": "array", "items": { "type": "string" } }
      }
    }
  }
}`
}
