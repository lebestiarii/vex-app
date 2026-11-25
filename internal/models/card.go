package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CardTemplateType defines how a search card renders and behaves in the UI.
type CardTemplateType string

const (
	// Canonical 5 Generic Full-Width Stacked Templates
	TemplateLocation         CardTemplateType = "location"
	TemplateServerMatrix     CardTemplateType = "server-matrix"
	TemplateWebsiteLink      CardTemplateType = "website-link"
	TemplatePatternBreakdown CardTemplateType = "pattern-breakdown"
	TemplateConversion       CardTemplateType = "conversion"

	// Extended & Backward-Compatible Templates
	TemplateBranchGateway      CardTemplateType = "branch-gateway"
	TemplateEnvBookmarks       CardTemplateType = "env-bookmarks"
	TemplateReferenceDecoder   CardTemplateType = "reference-decoder"
	TemplateContainerBlueprint CardTemplateType = "container-blueprint"
	TemplateBranchContact      CardTemplateType = "branch-contact"
	TemplateDictionaryGuide    CardTemplateType = "dictionary-guide"

	// Core Generic Templates (user ETL & freeform cards)
	TemplateAppShortcut CardTemplateType = "shortcut"    // App, URL, or PWA launcher
	TemplateKnowledge   CardTemplateType = "knowledge"   // Rich text/markdown/JSON fields for RAG & lookup
	TemplateCommand     CardTemplateType = "command"     // Parameterized snippet/webhook action
	TemplateKeyValue    CardTemplateType = "kv-grid"     // Structured metadata card
	TemplateMetric      CardTemplateType = "metric-card" // Dynamic data widget rendered from HTTP output
)

// ValidTemplates lists all supported built-in card templates.
var ValidTemplates = map[CardTemplateType]bool{
	TemplateLocation:           true,
	TemplateServerMatrix:       true,
	TemplateWebsiteLink:        true,
	TemplatePatternBreakdown:   true,
	TemplateConversion:         true,
	TemplateBranchGateway:      true,
	TemplateEnvBookmarks:       true,
	TemplateReferenceDecoder:   true,
	TemplateContainerBlueprint: true,
	TemplateBranchContact:      true,
	TemplateDictionaryGuide:    true,
	TemplateAppShortcut:        true,
	TemplateKnowledge:          true,
	TemplateCommand:            true,
	TemplateKeyValue:           true,
	TemplateMetric:             true,
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

// GlanceChip represents a styled Row-2 header chip in the Rolodex UI.
type GlanceChip struct {
	Label string `json:"label"`
	Tone  string `json:"tone"`          // "neutral" | "amber" | "green" | "red" | "mono"
	Dot   string `json:"dot,omitempty"` // Optional stoplight indicator: "green" | "amber" | "red"
}

// CardAction defines an executable action associated with a SearchCard.
type CardAction struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Shortcut string            `json:"shortcut,omitempty"` // Platform-agnostic hint e.g. "Enter"
	Type     string            `json:"type"`               // open_url, open_pwa, launch_app, copy, http, ai_prompt
	Target   string            `json:"target"`             // URL, HTTP endpoint, or clipboard value
	Env      map[string]string `json:"env,omitempty"`
}

// Provenance tracks where a card or section's information originated, its authority tier,
// and when it was verified so humans, AI RAG, and MCP clients can fact-check freshness.
type Provenance struct {
	SourceName string `json:"sourceName,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
	VerifiedAt string `json:"verifiedAt,omitempty"` // ISO date e.g. "2026-11-23"
	Authority  string `json:"authority,omitempty"`  // "official" | "internal-curated" | "automated-sync"
	LocalOnly  bool   `json:"localOnly,omitempty"`  // Exclude from external cloud LLM RAG calls
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

// SearchCard is the canonical unit stored in JSON files, modules, and indexed for search & RAG.
// It also natively supports recursive Children and typed GlanceChips for the Rolodex UI.
type SearchCard struct {
	ID              string           `json:"id"`
	EntityKey       string           `json:"entityKey,omitempty"` // Canonical cross-module entity key, e.g. "loc:AMS", "air-carrier:176"
	ModuleID        string           `json:"moduleId,omitempty"`  // Originating module ID
	Title           string           `json:"title"`
	Subtitle        string           `json:"subtitle,omitempty"`
	Description     string           `json:"description,omitempty"`
	Collection      string           `json:"collection"`
	Template        CardTemplateType `json:"template"`
	Icon            string           `json:"icon,omitempty"`
	Tags            []string         `json:"tags,omitempty"`
	Modifiers       []string         `json:"modifiers,omitempty"`
	GlancePills     []string         `json:"glancePills,omitempty"`
	Chips           []GlanceChip     `json:"chips,omitempty"`           // Typed Row-2 header chips for Rolodex UI
	Data            map[string]any   `json:"data,omitempty"`            // Strongly-typed template payload
	Children        []SearchCard     `json:"children,omitempty"`        // Recursive nested child cards
	DefaultExpanded bool             `json:"defaultExpanded,omitempty"` // Whether card starts expanded
	Pinnable        *bool            `json:"pinnable,omitempty"`        // Defaults to true for top-level cards, false for children
	Sections        []CardSection    `json:"sections,omitempty"`        // Composed module overlay sections
	RelatedKeys     []string         `json:"relatedKeys,omitempty"`     // Synergy keys linking related cards across modules
	Actions         []CardAction     `json:"actions,omitempty"`
	Provenance      *Provenance      `json:"provenance,omitempty"`
	LocalOnly       bool             `json:"localOnly,omitempty"` // Exclude from external cloud LLM RAG calls
	UpdatedAt       time.Time        `json:"updatedAt"`
}

// CardNode is the recursive UI-ready Rolodex node projected from a SearchCard.
type CardNode struct {
	ID              string           `json:"id"`
	Template        CardTemplateType `json:"template"`
	Icon            string           `json:"icon"`
	Title           string           `json:"title"`
	Subtitle        string           `json:"subtitle,omitempty"`
	Description     string           `json:"description,omitempty"`
	Collection      string           `json:"collection,omitempty"`
	EntityKey       string           `json:"entityKey,omitempty"`
	ModuleID        string           `json:"moduleId,omitempty"`
	Chips           []GlanceChip     `json:"chips"`
	Data            map[string]any   `json:"data"`
	Children        []CardNode       `json:"children,omitempty"`
	Actions         []CardAction     `json:"actions,omitempty"`
	DefaultExpanded bool             `json:"defaultExpanded,omitempty"`
	Pinnable        bool             `json:"pinnable"`
}

// ToCardNode converts a SearchCard (and its recursive children/sections) into a Rolodex CardNode.
func (c SearchCard) ToCardNode(depth int) CardNode {
	tpl := c.Template
	if tpl == "" {
		tpl = TemplateKnowledge
	}
	chips := make([]GlanceChip, 0, len(c.Chips)+len(c.GlancePills))
	if len(c.Chips) > 0 {
		chips = append(chips, c.Chips...)
	} else {
		for i, pill := range c.GlancePills {
			tone := "neutral"
			if i == 0 {
				tone = "amber"
			}
			chips = append(chips, GlanceChip{Label: pill, Tone: tone})
		}
	}
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
		Template:        tpl,
		Icon:            c.Icon,
		Title:           c.Title,
		Subtitle:        c.Subtitle,
		Description:     c.Description,
		Collection:      c.Collection,
		EntityKey:       c.EntityKey,
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

// ModuleManifest is a shareable, optional, deletable domain knowledge, template & pattern pack
// stored as a JSON file in ~/.vex/modules/<id>.json.
type ModuleManifest struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Category       string        `json:"category"` // e.g. "Gateways", "Air Freight", "Ocean Freight", "Equipment", "Patterns", "Templates"
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
	ActionTarget     string           `json:"actionTarget,omitempty"`     // e.g. "{{url}}"
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

// ValidateAndNormalizeCard fills sensible defaults and validates required fields recursively.
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
		return fmt.Errorf("invalid card template %q", card.Template)
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

	// Normalize chips
	for i := range card.Chips {
		card.Chips[i].Label = strings.TrimSpace(card.Chips[i].Label)
		if card.Chips[i].Tone == "" {
			card.Chips[i].Tone = "neutral"
		}
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

	// Recursively normalize nested child cards
	for i := range card.Children {
		if card.Children[i].Collection == "" {
			card.Children[i].Collection = card.Collection
		}
		if err := ValidateAndNormalizeCard(&card.Children[i], card.Collection); err != nil {
			return fmt.Errorf("child %d: %w", i, err)
		}
	}

	if card.UpdatedAt.IsZero() {
		card.UpdatedAt = time.Now().UTC()
	}

	if len(card.GlancePills) == 0 {
		if len(card.Chips) > 0 {
			for i := 0; i < len(card.Chips) && i < 3; i++ {
				if card.Chips[i].Label != "" {
					card.GlancePills = append(card.GlancePills, card.Chips[i].Label)
				}
			}
		} else {
			card.GlancePills = ComputeGlancePills(*card)
		}
	}

	return nil
}

// ComputeGlancePills extracts up to 3 high-signal summary pills from a card's structured Data.
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

	for _, k := range []string{
		"prefix", "scac", "iata_prefix", "iso_code", "hs_code", "acronym", "event_code", "incoterm",
		"capacity_cbm", "internal_volume", "max_volume", "max_gross_weight", "max_payload",
		"dimensions", "base_dimensions", "general_duty", "pallet_capacity",
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

// skippedDataKeys lists structural, URL, and UI-styling keys excluded from SearchableBody
// so FTS5 and embeddings index human-readable values instead of JSON boilerplate.
var skippedDataKeys = map[string]struct{}{
	"contract":      {},
	"url":           {},
	"devurl":        {},
	"dev_url":       {},
	"qaurl":         {},
	"qa_url":        {},
	"produrl":       {},
	"prod_url":      {},
	"trackingurl":   {},
	"tracking_url":  {},
	"mapurl":        {},
	"map_url":       {},
	"sshcommand":    {},
	"ssh_command":   {},
	"mode":          {},
	"aspect":        {},
	"tone":          {},
	"dot":           {},
	"icon":          {},
	"ianatimezone":  {},
	"iana_timezone": {},
}

func isSkippedDataKey(key string) bool {
	_, skip := skippedDataKeys[strings.ToLower(strings.TrimSpace(key))]
	return skip
}

func isURLActionTarget(act CardAction) bool {
	if act.Type == "open_url" || act.Type == "open_pwa" || act.Type == "http" {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(act.Target))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
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

// SearchableBody flattens Description, value-only Data, Children, Sections, and human-readable Action metadata for FTS5 and embeddings.
func (c SearchCard) SearchableBody() string {
	var b strings.Builder
	writeCardBody(&b, c, 0)
	return strings.TrimSpace(b.String())
}

func writeCardBody(b *strings.Builder, c SearchCard, depth int) {
	if depth > 3 {
		return
	}
	if depth > 0 {
		if title := strings.TrimSpace(c.Title); title != "" {
			b.WriteString(title)
			b.WriteByte('\n')
		}
		if sub := strings.TrimSpace(c.Subtitle); sub != "" {
			b.WriteString(sub)
			b.WriteByte('\n')
		}
		if ek := strings.TrimSpace(c.EntityKey); ek != "" {
			b.WriteString(ek)
			b.WriteByte('\n')
		}
		if chips := c.ChipsText(); chips != "" {
			b.WriteString(chips)
			b.WriteByte('\n')
		}
	}
	if desc := strings.TrimSpace(c.Description); desc != "" {
		b.WriteString(desc)
		b.WriteByte('\n')
	}
	if len(c.Data) > 0 {
		writeIndexableValue(b, c.Data, 0)
	}
	for _, sec := range c.Sections {
		if mn := strings.TrimSpace(sec.ModuleName); mn != "" {
			b.WriteString(mn)
			b.WriteByte(' ')
		}
		if st := strings.TrimSpace(sec.Title); st != "" {
			b.WriteString(st)
			b.WriteByte('\n')
		}
		if notes := strings.TrimSpace(sec.Notes); notes != "" {
			b.WriteString(notes)
			b.WriteByte('\n')
		}
		if len(sec.Fields) > 0 {
			writeIndexableValue(b, sec.Fields, 0)
		}
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
		if src := strings.TrimSpace(c.Provenance.SourceName); src != "" {
			b.WriteString(src)
			b.WriteByte(' ')
		}
		if auth := strings.TrimSpace(c.Provenance.Authority); auth != "" {
			b.WriteString(auth)
			b.WriteByte('\n')
		}
	}
}

func writeActionBody(b *strings.Builder, act CardAction) {
	if lbl := strings.TrimSpace(act.Label); lbl != "" {
		b.WriteString(lbl)
		b.WriteByte('\n')
	}
	if target := strings.TrimSpace(act.Target); target != "" && !isURLActionTarget(act) {
		b.WriteString(target)
		b.WriteByte('\n')
	}
}

func writeIndexableValue(b *strings.Builder, val any, depth int) {
	if val == nil || depth > 6 {
		return
	}
	switch v := val.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	case bool:
		// Skip boolean flags (UI/layout state, never meaningful search terms).
	case int:
		b.WriteString(strconv.Itoa(v))
		b.WriteByte('\n')
	case int8:
		b.WriteString(strconv.FormatInt(int64(v), 10))
		b.WriteByte('\n')
	case int16:
		b.WriteString(strconv.FormatInt(int64(v), 10))
		b.WriteByte('\n')
	case int32:
		b.WriteString(strconv.FormatInt(int64(v), 10))
		b.WriteByte('\n')
	case int64:
		b.WriteString(strconv.FormatInt(v, 10))
		b.WriteByte('\n')
	case uint:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteByte('\n')
	case uint8:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteByte('\n')
	case uint16:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteByte('\n')
	case uint32:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteByte('\n')
	case uint64:
		b.WriteString(strconv.FormatUint(v, 10))
		b.WriteByte('\n')
	case float32:
		b.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 32))
		b.WriteByte('\n')
	case float64:
		b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		b.WriteByte('\n')
	case json.Number:
		if s := strings.TrimSpace(v.String()); s != "" {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	case map[string]any:
		if len(v) == 0 {
			return
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if isSkippedDataKey(k) {
				continue
			}
			writeIndexableValue(b, v[k], depth+1)
		}
	case map[string]string:
		if len(v) == 0 {
			return
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if isSkippedDataKey(k) {
				continue
			}
			writeIndexableValue(b, v[k], depth+1)
		}
	case []any:
		for _, elem := range v {
			writeIndexableValue(b, elem, depth+1)
		}
	case []string:
		for _, elem := range v {
			writeIndexableValue(b, elem, depth+1)
		}
	case []map[string]any:
		for _, elem := range v {
			writeIndexableValue(b, elem, depth+1)
		}
	default:
		if s, ok := val.(fmt.Stringer); ok {
			if txt := strings.TrimSpace(s.String()); txt != "" {
				b.WriteString(txt)
				b.WriteByte('\n')
			}
		}
	}
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
		Chips       []GlanceChip     `json:"chips,omitempty"`
		Data        map[string]any   `json:"data"`
		Children    []SearchCard     `json:"children,omitempty"`
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
		Chips:       c.Chips,
		Data:        c.Data,
		Children:    c.Children,
		Sections:    c.Sections,
		Actions:     c.Actions,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CardJSONSchema returns the formal JSON Schema describing SearchCard, ModuleManifest, PatternRule, and ETLMappingRule.
func CardJSONSchema() string {
	return `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Vex Desk Rolodex Schema (SearchCard, ModuleManifest, PatternRule, ETLMappingRule)",
  "description": "Canonical schema for Vex SearchCard items (~/.vex/data/*.json), optional ModuleManifest packs (~/.vex/modules/*.json), and PatternRule definitions (~/.vex/patterns/*.json).",
  "definitions": {
    "GlanceChip": {
      "type": "object",
      "required": ["label", "tone"],
      "properties": {
        "label": { "type": "string" },
        "tone": { "type": "string", "enum": ["neutral", "blue", "amber", "green", "red", "mono"] },
        "dot": { "type": "string", "enum": ["green", "amber", "red"], "description": "Optional stoplight status dot for environment chips (DEV, QA, PROD)." }
      }
    },
    "PatternSegment": {
      "type": "object",
      "required": ["value", "role", "meaning"],
      "description": "One of 3 equal-width breakdown tiles rendered by pattern-breakdown cards.",
      "properties": {
        "value": { "type": "string", "description": "Matched raw segment value (supports {{templates}})." },
        "role": { "type": "string", "description": "Uppercase micro role label e.g. MODE, BRANCH, SEQUENCE." },
        "meaning": { "type": "string", "description": "Decoded human-readable meaning (supports {{lookup.*}} / {{alias.*}})." },
        "tone": { "type": "string", "enum": ["blue", "amber", "green", "neutral"] }
      }
    },
    "SearchCard": {
      "type": "object",
      "required": ["title", "collection", "template"],
      "properties": {
        "id": { "type": "string" },
        "entityKey": { "type": "string", "description": "Canonical cross-module entity key (e.g. loc:AMS, air-carrier:176, ocean-carrier:MAEU, guide:hbl)." },
        "moduleId": { "type": "string" },
        "title": { "type": "string" },
        "subtitle": { "type": "string" },
        "description": { "type": "string" },
        "collection": { "type": "string" },
        "template": {
          "type": "string",
          "enum": [
            "location",
            "server-matrix",
            "website-link",
            "pattern-breakdown",
            "conversion",
            "branch-gateway",
            "env-bookmarks",
            "reference-decoder",
            "container-blueprint",
            "branch-contact",
            "dictionary-guide",
            "shortcut",
            "knowledge",
            "command",
            "kv-grid",
            "metric-card"
          ],
          "description": "Prefer the 5 canonical full-width stacked templates: location (header-only entity nesting children), server-matrix (DEV/QA/PROD rows with Copy/SSH/Open), website-link (multi-link expandable OR 1-link single-button card with no chevron), pattern-breakdown (3-tile breakdown + explanation + nested cards), conversion (Input ⇄ Output strip + equivalents)."
        },
        "icon": { "type": "string", "description": "Emoji, flag emoji, or icon key." },
        "tags": { "type": "array", "items": { "type": "string" } },
        "modifiers": { "type": "array", "items": { "type": "string" } },
        "chips": { "type": "array", "maxItems": 4, "items": { "$ref": "#/definitions/GlanceChip" } },
        "glancePills": { "type": "array", "items": { "type": "string" } },
        "data": { "type": "object", "additionalProperties": true },
        "children": { "type": "array", "items": { "$ref": "#/definitions/SearchCard" }, "description": "Recursive nested child cards (up to 2 levels deep)." },
        "defaultExpanded": { "type": "boolean" },
        "pinnable": { "type": "boolean" },
        "relatedKeys": { "type": "array", "items": { "type": "string" } },
        "localOnly": { "type": "boolean" },
        "actions": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["type", "target"],
            "properties": {
              "id": { "type": "string" },
              "label": { "type": "string" },
              "shortcut": { "type": "string" },
              "type": { "type": "string", "enum": ["open_url", "open_pwa", "launch_app", "copy", "http", "ai_prompt"] },
              "target": { "type": "string" }
            }
          }
        }
      }
    },
    "ModuleManifest": {
      "type": "object",
      "required": ["id", "name"],
      "description": "Optional, deletable module stored in ~/.vex/modules/<id>.json containing cards, overlays, and/or pattern rules.",
      "properties": {
        "id": { "type": "string" },
        "name": { "type": "string" },
        "category": { "type": "string" },
        "description": { "type": "string" },
        "icon": { "type": "string" },
        "version": { "type": "string" },
        "defaultEnabled": { "type": "boolean" },
        "localOnly": { "type": "boolean" },
        "cards": { "type": "array", "items": { "$ref": "#/definitions/SearchCard" } },
        "overlays": { "type": "array" },
        "patterns": { "type": "array", "items": { "$ref": "#/definitions/PatternRule" } }
      }
    },
    "PatternRule": {
      "type": "object",
      "required": ["id", "name", "regex", "titleTemplate"],
      "description": "Loadable & configurable pattern decoder and cross-card link rule stored in ~/.vex/patterns/*.json or ModuleManifest.patterns.",
      "properties": {
        "id": { "type": "string" },
        "name": { "type": "string" },
        "category": { "type": "string" },
        "description": { "type": "string" },
        "enabled": { "type": "boolean", "default": true },
        "priority": { "type": "integer", "default": 0 },
        "normalize": { "type": "array", "items": { "type": "string", "enum": ["trim", "uppercase", "lowercase", "strip-spaces", "strip-dashes"] } },
        "regex": { "type": "string" },
        "caseInsensitive": { "type": "boolean" },
        "checksum": { "type": "string", "enum": ["", "iata-mod7", "iso6346", "luhn"] },
        "lookups": { "type": "object", "additionalProperties": { "type": "object", "additionalProperties": { "type": "string" } } },
        "dataLookups": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["alias", "targetEntityKey"],
            "properties": {
              "alias": { "type": "string" },
              "targetEntityKey": { "type": "string" },
              "extractFields": { "type": "object", "additionalProperties": { "type": "string" } },
              "fallbackValues": { "type": "object", "additionalProperties": { "type": "string" } }
            }
          }
        },
        "titleTemplate": { "type": "string" },
        "subtitleTemplate": { "type": "string" },
        "descriptionTemplate": { "type": "string" },
        "collection": { "type": "string" },
        "template": { "type": "string" },
        "icon": { "type": "string" },
        "defaultExpanded": { "type": "boolean" },
        "chips": { "type": "array", "items": { "$ref": "#/definitions/GlanceChip" } },
        "fields": { "type": "object", "additionalProperties": { "type": "string" } },
        "linkEntityKeys": { "type": "array", "items": { "type": "string" } },
        "linkRules": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "targetEntityKey": { "type": "string" },
              "targetCollection": { "type": "string" },
              "targetTag": { "type": "string" },
              "relationLabel": { "type": "string" },
              "autoExpand": { "type": "boolean" },
              "nestChildren": { "type": "boolean" },
              "copyChildrenOnly": { "type": "boolean" },
              "maxDepth": { "type": "integer" },
              "includeFields": { "type": "array", "items": { "type": "string" } }
            }
          }
        },
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
    },
    "ETLMappingRule": {
      "type": "object",
      "required": ["sourceFormat", "targetCollection", "defaultTemplate", "titleField"],
      "properties": {
        "name": { "type": "string" },
        "sourceFormat": { "type": "string", "enum": ["csv", "tsv", "json", "jsonl"] },
        "rootPath": { "type": "string" },
        "targetCollection": { "type": "string" },
        "defaultTemplate": { "type": "string" },
        "titleField": { "type": "string" },
        "subtitleField": { "type": "string" },
        "descriptionField": { "type": "string" },
        "tagsField": { "type": "string" },
        "staticTags": { "type": "array", "items": { "type": "string" } },
        "modifiers": { "type": "array", "items": { "type": "string" } },
        "iconField": { "type": "string" },
        "defaultIcon": { "type": "string" },
        "actionType": { "type": "string", "enum": ["open_url", "open_pwa", "launch_app", "copy", "http", "ai_prompt"] },
        "actionLabel": { "type": "string" },
        "actionTarget": { "type": "string" },
        "dataFields": { "type": "array", "items": { "type": "string" } }
      }
    }
  }
}`
}
