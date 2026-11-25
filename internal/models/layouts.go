package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// CardLayoutType defines which of the 9 pure generic layouts renders a card.
type CardLayoutType string

const (
	LayoutGroup      CardLayoutType = "group"
	LayoutLink       CardLayoutType = "link"
	LayoutActionList CardLayoutType = "action-list"
	LayoutFacts      CardLayoutType = "facts"
	LayoutSegments   CardLayoutType = "segments"
	LayoutSteps      CardLayoutType = "steps"
	LayoutConvert    CardLayoutType = "convert"
	LayoutTable      CardLayoutType = "table"
	LayoutSnippet    CardLayoutType = "snippet"
)

// ValidLayouts is the strict, zero-alias set of supported card layouts.
var ValidLayouts = map[CardLayoutType]bool{
	LayoutGroup:      true,
	LayoutLink:       true,
	LayoutActionList: true,
	LayoutFacts:      true,
	LayoutSegments:   true,
	LayoutSteps:      true,
	LayoutConvert:    true,
	LayoutTable:      true,
	LayoutSnippet:    true,
}

// AllLayouts returns the 9 canonical layout names in deterministic order.
func AllLayouts() []CardLayoutType {
	return []CardLayoutType{
		LayoutGroup,
		LayoutLink,
		LayoutActionList,
		LayoutFacts,
		LayoutSegments,
		LayoutSteps,
		LayoutConvert,
		LayoutTable,
		LayoutSnippet,
	}
}

// ValidChipTones lists the allowed visual tones for GlanceChip and row badges.
var ValidChipTones = map[string]bool{
	"neutral": true,
	"blue":    true,
	"amber":   true,
	"green":   true,
	"red":     true,
	"mono":    true,
}

// ValidChipDots lists the allowed stoplight dots for chips and actions.
var ValidChipDots = map[string]bool{
	"":      true,
	"green": true,
	"amber": true,
	"red":   true,
}

// ValidSegmentTones lists the allowed tile tones for segments layout cards.
var ValidSegmentTones = map[string]bool{
	"blue":    true,
	"amber":   true,
	"green":   true,
	"neutral": true,
}

// --- Strict Typed Layout Payloads (9 Pure Generic Layouts) ---

// GroupLayoutData is the strict payload for layout "group" (header-only container).
type GroupLayoutData struct {
	Meta map[string]any `json:"meta,omitempty"`
}

// LinkLayoutData is the strict payload for layout "link" (single-button direct launch).
type LinkLayoutData struct {
	URL string `json:"url"`
}

// RowAction defines an inline copy or open_url action on an action-list row.
type RowAction struct {
	Label string `json:"label"`
	Type  string `json:"type"` // "copy" | "open_url"
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"`
	Dot   string `json:"dot,omitempty"`
}

// ActionListRow defines one stacked row in an "action-list" layout card.
type ActionListRow struct {
	Badge    *GlanceChip `json:"badge,omitempty"`
	Primary  string      `json:"primary"`
	Mono     bool        `json:"mono,omitempty"`
	Subtitle string      `json:"subtitle,omitempty"`
	Actions  []RowAction `json:"actions"`
}

// ActionListLayoutData is the strict payload for layout "action-list".
type ActionListLayoutData struct {
	Rows []ActionListRow `json:"rows"`
}

// FactItem defines one key/value entry inside a "facts" layout card.
type FactItem struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Mono     bool   `json:"mono,omitempty"`
	Copyable bool   `json:"copyable,omitempty"`
}

// FactView defines one selectable tab/view (e.g. Metric / Imperial) inside a "facts" card.
type FactView struct {
	Label string     `json:"label"`
	Facts []FactItem `json:"facts"`
}

// FactsLayoutData is the strict payload for layout "facts".
type FactsLayoutData struct {
	Summary string     `json:"summary,omitempty"`
	Facts   []FactItem `json:"facts,omitempty"`
	Views   []FactView `json:"views,omitempty"`
}

// SegmentTile defines one breakdown tile in a "segments" layout card.
type SegmentTile struct {
	Value   string `json:"value"`
	Role    string `json:"role"`
	Meaning string `json:"meaning"`
	Tone    string `json:"tone"`
}

// SegmentsLayoutData is the strict payload for layout "segments".
type SegmentsLayoutData struct {
	Segments    []SegmentTile `json:"segments"`
	Explanation string        `json:"explanation"`
}

// StepItem defines one milestone in a "steps" layout card.
type StepItem struct {
	Label   string `json:"label"`
	Caption string `json:"caption,omitempty"`
}

// StepsLayoutData is the strict payload for layout "steps".
type StepsLayoutData struct {
	Steps      []StepItem `json:"steps"`
	ActiveStep int        `json:"activeStep,omitempty"`
	Summary    string     `json:"summary"`
	Bullets    []string   `json:"bullets,omitempty"`
}

// ConvertEquivalent defines an equivalent unit pill inside a "convert" layout card.
type ConvertEquivalent struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ConvertLayoutData is the strict payload for layout "convert".
type ConvertLayoutData struct {
	Input       string              `json:"input"`
	Output      string              `json:"output"`
	Equivalents []ConvertEquivalent `json:"equivalents,omitempty"`
}

// TableLayoutData is the strict payload for layout "table" (2..4 columns).
type TableLayoutData struct {
	Columns     []string   `json:"columns"`
	Rows        [][]string `json:"rows"`
	MonoColumns []int      `json:"monoColumns,omitempty"`
	SourceURL   string     `json:"sourceUrl,omitempty"`
}

// SnippetVar defines one inline {{var}} input in a "snippet" layout card.
type SnippetVar struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	DefaultValue string `json:"defaultValue,omitempty"`
}

// SnippetLayoutData is the strict payload for layout "snippet".
type SnippetLayoutData struct {
	Language string       `json:"language,omitempty"`
	Body     string       `json:"body"`
	Vars     []SnippetVar `json:"vars,omitempty"`
}

// --- Structured Compiler-as-Referee Validation ---

// FieldError reports a single deterministic schema or invariant violation.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e FieldError) String() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationResult summarizes the outcome of validating a ModuleManifest or SearchCard.
type ValidationResult struct {
	Valid  bool         `json:"valid"`
	Errors []FieldError `json:"errors,omitempty"`
}

// Err returns nil if Valid is true, or a combined error listing all violations.
func (r ValidationResult) Err() error {
	if r.Valid && len(r.Errors) == 0 {
		return nil
	}
	parts := make([]string, 0, len(r.Errors))
	for _, fe := range r.Errors {
		parts = append(parts, fe.String())
	}
	if len(parts) == 0 {
		return errors.New("validation failed")
	}
	return errors.New(strings.Join(parts, "; "))
}

func decodeStrictMap(data map[string]any, dst any) error {
	if len(data) == 0 {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func validateHTTPOrHTTPS(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid url %q: %w", trimmed, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url %q must use http or https scheme", trimmed)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("url %q is missing a host", trimmed)
	}
	return nil
}

func validateLayoutData(layout CardLayoutType, data map[string]any, prefix string) []FieldError {
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

	switch layout {
	case LayoutGroup:
		if len(data) == 0 {
			return nil
		}
		var payload GroupLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("group cards are header-only and allow only optional data.meta: %v", err))
			return errs
		}
		for k, v := range payload.Meta {
			switch v.(type) {
			case string, float64, int, int64, json.Number:
				// Allowed primitive string or number metadata (e.g. ianaTimezone, lat, lon).
			default:
				add("data.meta."+k, "meta values must be strings or numbers")
			}
		}

	case LayoutLink:
		if len(data) == 0 {
			add("data.url", "link layout requires data.url")
			return errs
		}
		var payload LinkLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict link payload violation: %v", err))
			return errs
		}
		if err := validateHTTPOrHTTPS(payload.URL); err != nil {
			add("data.url", err.Error())
		}

	case LayoutActionList:
		if len(data) == 0 {
			add("data.rows", "action-list layout requires non-empty data.rows")
			return errs
		}
		var payload ActionListLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict action-list payload violation: %v", err))
			return errs
		}
		if len(payload.Rows) == 0 {
			add("data.rows", "action-list requires at least one row")
		}
		for i, row := range payload.Rows {
			rp := fmt.Sprintf("data.rows[%d]", i)
			if strings.TrimSpace(row.Primary) == "" {
				add(rp+".primary", "row primary text is required")
			}
			if row.Badge != nil {
				if strings.TrimSpace(row.Badge.Label) == "" {
					add(rp+".badge.label", "badge label is required")
				}
				tone := strings.TrimSpace(row.Badge.Tone)
				if tone != "" && !ValidChipTones[tone] {
					add(rp+".badge.tone", fmt.Sprintf("invalid badge tone %q", row.Badge.Tone))
				}
				if !ValidChipDots[strings.TrimSpace(row.Badge.Dot)] {
					add(rp+".badge.dot", fmt.Sprintf("invalid badge dot %q", row.Badge.Dot))
				}
			}
			if len(row.Actions) == 0 {
				add(rp+".actions", "each action-list row requires at least one action")
			}
			for j, act := range row.Actions {
				ap := fmt.Sprintf("%s.actions[%d]", rp, j)
				if strings.TrimSpace(act.Label) == "" {
					add(ap+".label", "action label is required")
				}
				if act.Type != "copy" && act.Type != "open_url" {
					add(ap+".type", fmt.Sprintf("action type %q must be \"copy\" or \"open_url\"", act.Type))
				}
				if strings.TrimSpace(act.Value) == "" {
					add(ap+".value", "action value is required")
				} else if act.Type == "open_url" {
					if err := validateHTTPOrHTTPS(act.Value); err != nil {
						add(ap+".value", err.Error())
					}
				}
				if tone := strings.TrimSpace(act.Tone); tone != "" && !ValidChipTones[tone] {
					add(ap+".tone", fmt.Sprintf("invalid action tone %q", act.Tone))
				}
				if !ValidChipDots[strings.TrimSpace(act.Dot)] {
					add(ap+".dot", fmt.Sprintf("invalid action dot %q", act.Dot))
				}
			}
		}

	case LayoutFacts:
		if len(data) == 0 {
			add("data", "facts layout requires summary, facts[], or views[]")
			return errs
		}
		var payload FactsLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict facts payload violation: %v", err))
			return errs
		}
		if strings.TrimSpace(payload.Summary) == "" && len(payload.Facts) == 0 && len(payload.Views) == 0 {
			add("data", "facts layout must provide summary, facts[], or views[]")
		}
		for i, f := range payload.Facts {
			fp := fmt.Sprintf("data.facts[%d]", i)
			if strings.TrimSpace(f.Label) == "" {
				add(fp+".label", "fact label is required")
			}
			if strings.TrimSpace(f.Value) == "" {
				add(fp+".value", "fact value is required")
			}
		}
		for i, v := range payload.Views {
			vp := fmt.Sprintf("data.views[%d]", i)
			if strings.TrimSpace(v.Label) == "" {
				add(vp+".label", "view label is required")
			}
			if len(v.Facts) == 0 {
				add(vp+".facts", "view must include at least one fact")
			}
			for j, f := range v.Facts {
				fp := fmt.Sprintf("%s.facts[%d]", vp, j)
				if strings.TrimSpace(f.Label) == "" {
					add(fp+".label", "fact label is required")
				}
				if strings.TrimSpace(f.Value) == "" {
					add(fp+".value", "fact value is required")
				}
			}
		}

	case LayoutSegments:
		if len(data) == 0 {
			add("data", "segments layout requires segments[] and explanation")
			return errs
		}
		var payload SegmentsLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict segments payload violation: %v", err))
			return errs
		}
		if len(payload.Segments) == 0 {
			add("data.segments", "segments layout requires at least one segment")
		}
		for i, seg := range payload.Segments {
			sp := fmt.Sprintf("data.segments[%d]", i)
			if strings.TrimSpace(seg.Value) == "" {
				add(sp+".value", "segment value is required")
			}
			if strings.TrimSpace(seg.Role) == "" {
				add(sp+".role", "segment role is required")
			}
			if strings.TrimSpace(seg.Meaning) == "" {
				add(sp+".meaning", "segment meaning is required")
			}
			if !ValidSegmentTones[strings.TrimSpace(seg.Tone)] {
				add(sp+".tone", fmt.Sprintf("invalid segment tone %q (expected blue, amber, green, or neutral)", seg.Tone))
			}
		}
		if strings.TrimSpace(payload.Explanation) == "" {
			add("data.explanation", "segments layout requires non-empty explanation")
		}

	case LayoutSteps:
		if len(data) == 0 {
			add("data", "steps layout requires steps[] and summary")
			return errs
		}
		var payload StepsLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict steps payload violation: %v", err))
			return errs
		}
		if len(payload.Steps) == 0 {
			add("data.steps", "steps layout requires at least one step")
		}
		for i, st := range payload.Steps {
			sp := fmt.Sprintf("data.steps[%d]", i)
			if strings.TrimSpace(st.Label) == "" {
				add(sp+".label", "step label is required")
			}
		}
		if payload.ActiveStep < 0 || (payload.ActiveStep > 0 && payload.ActiveStep > len(payload.Steps)) {
			add("data.activeStep", fmt.Sprintf("activeStep %d out of range [1..%d]", payload.ActiveStep, len(payload.Steps)))
		}
		if strings.TrimSpace(payload.Summary) == "" {
			add("data.summary", "steps layout requires non-empty summary")
		}

	case LayoutConvert:
		if len(data) == 0 {
			add("data", "convert layout requires input and output")
			return errs
		}
		var payload ConvertLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict convert payload violation: %v", err))
			return errs
		}
		if strings.TrimSpace(payload.Input) == "" {
			add("data.input", "convert input is required")
		}
		if strings.TrimSpace(payload.Output) == "" {
			add("data.output", "convert output is required")
		}
		for i, eq := range payload.Equivalents {
			ep := fmt.Sprintf("data.equivalents[%d]", i)
			if strings.TrimSpace(eq.Label) == "" {
				add(ep+".label", "equivalent label is required")
			}
			if strings.TrimSpace(eq.Value) == "" {
				add(ep+".value", "equivalent value is required")
			}
		}

	case LayoutTable:
		if len(data) == 0 {
			add("data", "table layout requires columns (2..4) and rows")
			return errs
		}
		var payload TableLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict table payload violation: %v", err))
			return errs
		}
		if len(payload.Columns) < 2 || len(payload.Columns) > 4 {
			add("data.columns", fmt.Sprintf("table layout requires 2 to 4 columns (got %d)", len(payload.Columns)))
		}
		for i, col := range payload.Columns {
			if strings.TrimSpace(col) == "" {
				add(fmt.Sprintf("data.columns[%d]", i), "column header cannot be empty")
			}
		}
		if len(payload.Rows) == 0 {
			add("data.rows", "table layout requires at least one row")
		}
		for i, row := range payload.Rows {
			if len(payload.Columns) > 0 && len(row) != len(payload.Columns) {
				add(fmt.Sprintf("data.rows[%d]", i), fmt.Sprintf("row has %d cells, expected %d", len(row), len(payload.Columns)))
			}
		}
		for i, idx := range payload.MonoColumns {
			if idx < 0 || idx >= len(payload.Columns) {
				add(fmt.Sprintf("data.monoColumns[%d]", i), fmt.Sprintf("mono column index %d out of range [0..%d]", idx, len(payload.Columns)-1))
			}
		}
		if strings.TrimSpace(payload.SourceURL) != "" {
			if err := validateHTTPOrHTTPS(payload.SourceURL); err != nil {
				add("data.sourceUrl", err.Error())
			}
		}

	case LayoutSnippet:
		if len(data) == 0 {
			add("data", "snippet layout requires non-empty body")
			return errs
		}
		var payload SnippetLayoutData
		if err := decodeStrictMap(data, &payload); err != nil {
			add("data", fmt.Sprintf("strict snippet payload violation: %v", err))
			return errs
		}
		if strings.TrimSpace(payload.Body) == "" {
			add("data.body", "snippet body is required")
		}
		seenVarKeys := make(map[string]bool, len(payload.Vars))
		for i, v := range payload.Vars {
			vp := fmt.Sprintf("data.vars[%d]", i)
			key := strings.TrimSpace(v.Key)
			if key == "" {
				add(vp+".key", "snippet var key is required")
			} else if seenVarKeys[key] {
				add(vp+".key", fmt.Sprintf("duplicate snippet var key %q", key))
			} else {
				seenVarKeys[key] = true
			}
			if strings.TrimSpace(v.Label) == "" {
				add(vp+".label", "snippet var label is required")
			}
		}

	default:
		add("layout", fmt.Sprintf("invalid card layout %q", layout))
	}

	return errs
}

// extractExactTokens splits a title or chip string into lowercase alphanumeric tokens.
func extractExactTokens(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		low := strings.ToLower(strings.TrimSpace(f))
		if low != "" {
			out = append(out, low)
		}
	}
	return out
}

// isStandaloneChipToken returns the lowercased single token if label is a bare standalone token
// (e.g. "BAXU", "EK", "MSCU") with no spaces or compound separators (like "SCAC BAXU" or "NLAMS · AMS").
func isStandaloneChipToken(label string) (string, bool) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return "", false
	}
	for _, r := range trimmed {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return "", false
		}
	}
	return strings.ToLower(trimmed), true
}

func validateChipsAndEcho(title string, chips []GlanceChip, prefix string) []FieldError {
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

	if len(chips) > 4 {
		add("chips", fmt.Sprintf("max 4 chips allowed (got %d)", len(chips)))
	}

	titleTokens := make(map[string]struct{})
	for _, tok := range extractExactTokens(title) {
		titleTokens[tok] = struct{}{}
	}

	// Collect tokens from multi-token labeled chips (e.g. "SCAC BAXU" -> {"scac", "baxu"})
	// so we can reject a second standalone redundant chip like "BAXU" while allowing distinct secondary codes like "UNFU".
	compoundChipTokens := make(map[string]string)
	for _, ch := range chips {
		lbl := strings.TrimSpace(ch.Label)
		if _, standalone := isStandaloneChipToken(lbl); !standalone && lbl != "" {
			for _, tok := range extractExactTokens(lbl) {
				if _, exists := compoundChipTokens[tok]; !exists {
					compoundChipTokens[tok] = lbl
				}
			}
		}
	}

	seenLabels := make(map[string]bool, len(chips))
	for i, ch := range chips {
		cp := fmt.Sprintf("chips[%d]", i)
		lbl := strings.TrimSpace(ch.Label)
		if lbl == "" {
			add(cp+".label", "chip label cannot be empty")
			continue
		}
		tone := strings.TrimSpace(ch.Tone)
		if tone != "" && !ValidChipTones[tone] {
			add(cp+".tone", fmt.Sprintf("invalid chip tone %q", ch.Tone))
		}
		if !ValidChipDots[strings.TrimSpace(ch.Dot)] {
			add(cp+".dot", fmt.Sprintf("invalid chip dot %q", ch.Dot))
		}

		normLabel := strings.ToLower(lbl)
		if seenLabels[normLabel] {
			add(cp+".label", fmt.Sprintf("duplicate chip label %q", lbl))
			continue
		}
		seenLabels[normLabel] = true

		if tok, standalone := isStandaloneChipToken(lbl); standalone {
			if _, inTitle := titleTokens[tok]; inTitle {
				add(cp+".label", fmt.Sprintf("standalone chip %q echoes exact token already in title %q", lbl, title))
			} else if otherChip, inCompound := compoundChipTokens[tok]; inCompound {
				add(cp+".label", fmt.Sprintf("standalone chip %q duplicates token already present in chip %q", lbl, otherChip))
			}
		}
	}

	return errs
}

var validPatternNormalizers = map[string]bool{
	"trim":         true,
	"uppercase":    true,
	"upper":        true,
	"lowercase":    true,
	"lower":        true,
	"strip-spaces": true,
	"no-spaces":    true,
	"strip-dashes": true,
	"no-dashes":    true,
}

var validPatternChecksums = map[string]bool{
	"":          true,
	"iata-mod7": true,
	"iso6346":   true,
	"luhn":      true,
}

// ValidatePatternRule validates a PatternRule (regex compilation, layout, checksum, normalizers, chips).
func ValidatePatternRule(rule *PatternRule, prefix string) []FieldError {
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

	if rule == nil {
		add("", "pattern rule is nil")
		return errs
	}
	rule.ID = Slugify(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Regex = strings.TrimSpace(rule.Regex)
	rule.TitleTemplate = strings.TrimSpace(rule.TitleTemplate)

	if rule.ID == "" || rule.ID == "item" {
		add("id", "pattern rule id is required")
	}
	if rule.Name == "" {
		add("name", "pattern rule name is required")
	}
	if rule.Regex == "" {
		add("regex", "pattern rule regex is required")
	} else {
		expr := rule.Regex
		if rule.CaseInsensitive && !strings.HasPrefix(expr, "(?i)") {
			expr = "(?i)" + expr
		}
		if _, err := regexp.Compile(expr); err != nil {
			add("regex", fmt.Sprintf("invalid regex %q: %v", rule.Regex, err))
		}
	}
	if rule.TitleTemplate == "" {
		add("titleTemplate", "pattern rule titleTemplate is required")
	}
	if rule.Layout == "" {
		rule.Layout = LayoutSegments
	} else if !ValidLayouts[rule.Layout] {
		add("layout", fmt.Sprintf("invalid pattern layout %q", rule.Layout))
	}
	if !validPatternChecksums[strings.ToLower(strings.TrimSpace(rule.Checksum))] {
		add("checksum", fmt.Sprintf("invalid checksum %q (expected iata-mod7, iso6346, or luhn)", rule.Checksum))
	}
	for i, norm := range rule.Normalize {
		if !validPatternNormalizers[strings.ToLower(strings.TrimSpace(norm))] {
			add(fmt.Sprintf("normalize[%d]", i), fmt.Sprintf("invalid normalizer %q", norm))
		}
	}
	if len(rule.Chips) > 4 {
		add("chips", fmt.Sprintf("max 4 chips allowed (got %d)", len(rule.Chips)))
	}
	for i, ch := range rule.Chips {
		if strings.TrimSpace(ch.Label) == "" {
			add(fmt.Sprintf("chips[%d].label", i), "chip label cannot be empty")
		}
		if tone := strings.TrimSpace(ch.Tone); tone != "" && !ValidChipTones[tone] {
			add(fmt.Sprintf("chips[%d].tone", i), fmt.Sprintf("invalid chip tone %q", ch.Tone))
		}
		if !ValidChipDots[strings.TrimSpace(ch.Dot)] {
			add(fmt.Sprintf("chips[%d].dot", i), fmt.Sprintf("invalid chip dot %q", ch.Dot))
		}
	}
	for i, seg := range rule.Segments {
		if tone := strings.TrimSpace(seg.Tone); tone != "" && !ValidSegmentTones[tone] {
			add(fmt.Sprintf("segments[%d].tone", i), fmt.Sprintf("invalid segment tone %q", seg.Tone))
		}
	}
	for i, act := range rule.Actions {
		actType := strings.ToLower(strings.TrimSpace(act.Type))
		if !ValidActionTypes[actType] {
			add(fmt.Sprintf("actions[%d].type", i), fmt.Sprintf("invalid action type %q", act.Type))
		}
	}
	if rule.LiveHTTP != nil {
		livePrefix := "liveHttp"
		if prefix != "" {
			livePrefix = prefix + ".liveHttp"
		}
		errs = append(errs, ValidatePatternLiveHTTP(rule.LiveHTTP, livePrefix)...)
	}

	return errs
}

var (
	liveHTTPPlaceholderRe = regexp.MustCompile(`\{\{\s*[^}]+?\s*\}\}`)
	envVarNameRe          = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	validLiveHTTPAuth     = map[string]bool{
		"":          true,
		"none":      true,
		"gh_cli":    true,
		"az_devops": true,
	}
	validLiveResultLayouts = map[CardLayoutType]bool{
		LayoutFacts: true,
		LayoutTable: true,
		LayoutSteps: true,
		LayoutLink:  true,
	}
	forbiddenLiveHTTPHeaders = map[string]bool{
		"authorization":       true,
		"proxy-authorization": true,
		"cookie":              true,
		"set-cookie":          true,
		"x-api-key":           true,
		"api-key":             true,
		"x-auth-token":        true,
		"x-access-token":      true,
		"private-token":       true,
		"x-github-token":      true,
		"x-token":             true,
	}
	inlineSecretValuePrefixes = []string{
		"bearer ",
		"basic ",
		"token ",
		"ghp_",
		"gho_",
		"ghu_",
		"ghs_",
		"github_pat_",
		"glpat-",
		"sk-",
		"pat-",
	}
)

// ValidateHTTPURLStrict validates a resolved HTTP/HTTPS URL, rejecting control/shell
// characters, non-http(s) schemes, missing hosts, and embedded userinfo credentials.
func ValidateHTTPURLStrict(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("url is required")
	}
	if strings.ContainsAny(trimmed, "\r\n\x00|&;^`$") {
		return fmt.Errorf("unsafe characters in url %q", trimmed)
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid url %q: %w", trimmed, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url %q must use http or https scheme", trimmed)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("url %q is missing a host", trimmed)
	}
	if u.User != nil {
		return errors.New("embedded URL credentials (userinfo) are forbidden")
	}
	return nil
}

// ValidatePatternLiveHTTP validates a PatternLiveHTTP configuration, strictly enforcing
// GET-only http/https URL templates, bounded timeouts, zero inline secrets, and strict
// nested resultCard projection into facts, table, steps, or link layouts.
func ValidatePatternLiveHTTP(live *PatternLiveHTTP, prefix string) []FieldError {
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

	if live == nil {
		return nil
	}

	live.URLTemplate = strings.TrimSpace(live.URLTemplate)
	if live.URLTemplate == "" {
		add("urlTemplate", "liveHttp.urlTemplate is required")
	} else {
		sampleURL := liveHTTPPlaceholderRe.ReplaceAllString(live.URLTemplate, "sample")
		if err := ValidateHTTPURLStrict(sampleURL); err != nil {
			add("urlTemplate", err.Error())
		}
	}

	trigger := strings.ToLower(strings.TrimSpace(live.Trigger))
	if trigger == "" {
		live.Trigger = "manual"
	} else if trigger != "auto" && trigger != "manual" {
		add("trigger", fmt.Sprintf("invalid trigger %q (expected \"auto\" or \"manual\")", live.Trigger))
	} else {
		live.Trigger = trigger
	}

	if live.TimeoutMs < 0 || live.TimeoutMs > 5000 {
		add("timeoutMs", fmt.Sprintf("timeoutMs %d must be between 0 and 5000", live.TimeoutMs))
	}
	if live.CacheTTLSec < 0 || live.CacheTTLSec > 86400 {
		add("cacheTtlSec", fmt.Sprintf("cacheTtlSec %d must be between 0 and 86400", live.CacheTTLSec))
	}

	auth := strings.TrimSpace(live.Auth)
	if auth == "" {
		live.Auth = "none"
	} else if validLiveHTTPAuth[auth] {
		live.Auth = auth
	} else if strings.HasPrefix(auth, "env:") {
		varName := strings.TrimPrefix(auth, "env:")
		if !envVarNameRe.MatchString(varName) {
			add("auth", fmt.Sprintf("invalid env auth %q (expected env:VAR_NAME with uppercase letters, digits, underscores)", auth))
		} else {
			live.Auth = auth
		}
	} else {
		add("auth", fmt.Sprintf("invalid auth %q (allowed: none, gh_cli, az_devops, env:VAR_NAME; inline tokens/secrets are strictly forbidden)", auth))
	}

	for k, v := range live.Headers {
		keyTrim := strings.TrimSpace(k)
		if keyTrim == "" {
			add("headers", "header name cannot be empty")
			continue
		}
		lowerKey := strings.ToLower(keyTrim)
		if forbiddenLiveHTTPHeaders[lowerKey] || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "key") {
			add("headers."+keyTrim, fmt.Sprintf("header %q is forbidden; configure authentication via liveHttp.auth instead of inline secrets", keyTrim))
			continue
		}
		if strings.ContainsAny(keyTrim+v, "\r\n\x00") {
			add("headers."+keyTrim, "header contains illegal control characters")
			continue
		}
		lowerVal := strings.ToLower(strings.TrimSpace(v))
		for _, badPrefix := range inlineSecretValuePrefixes {
			if strings.HasPrefix(lowerVal, badPrefix) {
				add("headers."+keyTrim, "inline tokens or credentials in headers are strictly forbidden")
				break
			}
		}
	}

	rc := &live.ResultCard
	if !validLiveResultLayouts[rc.Layout] {
		add("resultCard.layout", fmt.Sprintf("invalid resultCard.layout %q (allowed: facts, table, steps, link)", rc.Layout))
		return errs
	}

	if len(rc.Chips) > 4 {
		add("resultCard.chips", fmt.Sprintf("max 4 chips allowed (got %d)", len(rc.Chips)))
	}
	for i, ch := range rc.Chips {
		if strings.TrimSpace(ch.Label) == "" {
			add(fmt.Sprintf("resultCard.chips[%d].label", i), "chip label cannot be empty")
		}
		if tone := strings.TrimSpace(ch.Tone); tone != "" && !ValidChipTones[tone] {
			add(fmt.Sprintf("resultCard.chips[%d].tone", i), fmt.Sprintf("invalid chip tone %q", ch.Tone))
		}
		if !ValidChipDots[strings.TrimSpace(ch.Dot)] {
			add(fmt.Sprintf("resultCard.chips[%d].dot", i), fmt.Sprintf("invalid chip dot %q", ch.Dot))
		}
	}

	switch rc.Layout {
	case LayoutFacts:
		if strings.TrimSpace(rc.Summary) == "" && len(rc.Facts) == 0 {
			add("resultCard", "facts resultCard requires summary or at least one fact mapping")
		}
		for i, f := range rc.Facts {
			if strings.TrimSpace(f.Label) == "" {
				add(fmt.Sprintf("resultCard.facts[%d].label", i), "fact label is required")
			}
			if strings.TrimSpace(f.Value) == "" {
				add(fmt.Sprintf("resultCard.facts[%d].value", i), "fact value template is required")
			}
		}

	case LayoutLink:
		u := strings.TrimSpace(rc.URL)
		if u == "" {
			add("resultCard.url", "link resultCard requires url template")
		} else if !strings.Contains(u, "{{") {
			if err := ValidateHTTPURLStrict(u); err != nil {
				add("resultCard.url", err.Error())
			}
		}

	case LayoutTable:
		if len(rc.Columns) < 2 || len(rc.Columns) > 4 {
			add("resultCard.columns", fmt.Sprintf("table resultCard requires 2 to 4 columns (got %d)", len(rc.Columns)))
		}
		for i, col := range rc.Columns {
			if strings.TrimSpace(col) == "" {
				add(fmt.Sprintf("resultCard.columns[%d]", i), "column header cannot be empty")
			}
		}
		if strings.TrimSpace(rc.RowsPath) != "" {
			if len(rc.RowFields) != len(rc.Columns) {
				add("resultCard.rowFields", fmt.Sprintf("rowFields length (%d) must match columns length (%d)", len(rc.RowFields), len(rc.Columns)))
			}
		} else if len(rc.Rows) == 0 {
			add("resultCard.rows", "table resultCard requires rowsPath + rowFields or at least one row")
		} else {
			for i, row := range rc.Rows {
				if len(rc.Columns) > 0 && len(row) != len(rc.Columns) {
					add(fmt.Sprintf("resultCard.rows[%d]", i), fmt.Sprintf("row has %d cells, expected %d", len(row), len(rc.Columns)))
				}
			}
		}
		for i, idx := range rc.MonoColumns {
			if idx < 0 || idx >= len(rc.Columns) {
				add(fmt.Sprintf("resultCard.monoColumns[%d]", i), fmt.Sprintf("mono column index %d out of range", idx))
			}
		}

	case LayoutSteps:
		if strings.TrimSpace(rc.Summary) == "" {
			add("resultCard.summary", "steps resultCard requires summary")
		}
		if strings.TrimSpace(rc.StepsPath) != "" {
			if strings.TrimSpace(rc.StepLabel) == "" {
				add("resultCard.stepLabel", "stepLabel is required when stepsPath is set")
			}
		} else if len(rc.Steps) == 0 {
			add("resultCard.steps", "steps resultCard requires stepsPath + stepLabel or at least one step")
		} else {
			for i, st := range rc.Steps {
				if strings.TrimSpace(st.Label) == "" {
					add(fmt.Sprintf("resultCard.steps[%d].label", i), "step label is required")
				}
			}
		}
	}

	return errs
}

// ValidateModuleManifest validates an entire ModuleManifest (metadata, all cards, flat parentEntityKey
// depth/cycles, overlays, and pattern rules) and normalizes cards in place.
func ValidateModuleManifest(mod *ModuleManifest) ValidationResult {
	var errs []FieldError
	add := func(path, msg string) {
		errs = append(errs, FieldError{Path: path, Message: msg})
	}

	if mod == nil {
		return ValidationResult{
			Valid:  false,
			Errors: []FieldError{{Path: "", Message: "module manifest is nil"}},
		}
	}

	mod.Name = strings.TrimSpace(mod.Name)
	if mod.Name == "" {
		add("name", "module name is required")
	}
	rawID := strings.TrimSpace(mod.ID)
	if rawID == "" && mod.Name != "" {
		mod.ID = Slugify(mod.Name)
	} else {
		mod.ID = Slugify(rawID)
	}
	if mod.ID == "" || mod.ID == "item" {
		add("id", "module id is required")
	}

	seenCardIDs := make(map[string]bool, len(mod.Cards))
	parentByEntity := make(map[string]string)
	entityCards := make(map[string]string) // normalized entityKey -> card path

	for i := range mod.Cards {
		cp := fmt.Sprintf("cards[%d]", i)
		cardErrs := validateAndNormalizeCardDetailed(&mod.Cards[i], mod.ID, 0, cp)
		errs = append(errs, cardErrs...)

		if mod.Cards[i].ID != "" {
			if seenCardIDs[mod.Cards[i].ID] {
				add(cp+".id", fmt.Sprintf("duplicate card id %q in module", mod.Cards[i].ID))
			}
			seenCardIDs[mod.Cards[i].ID] = true
		}

		ek := strings.ToLower(strings.TrimSpace(mod.Cards[i].EntityKey))
		pek := strings.ToLower(strings.TrimSpace(mod.Cards[i].ParentEntityKey))
		if ek != "" && pek != "" && ek == pek {
			add(cp+".parentEntityKey", fmt.Sprintf("card cannot reference its own entityKey %q as parentEntityKey", mod.Cards[i].ParentEntityKey))
		}
		if ek != "" {
			entityCards[ek] = cp
			if pek != "" {
				parentByEntity[ek] = pek
			}
		}
	}

	// Verify flat parentEntityKey chains do not cycle or exceed nesting depth 2.
	for i := range mod.Cards {
		pek := strings.ToLower(strings.TrimSpace(mod.Cards[i].ParentEntityKey))
		if pek == "" {
			continue
		}
		cp := fmt.Sprintf("cards[%d]", i)
		// Depth from this card's root ancestor inside the manifest.
		depth := 1
		visited := map[string]bool{}
		if ek := strings.ToLower(strings.TrimSpace(mod.Cards[i].EntityKey)); ek != "" {
			visited[ek] = true
		}
		cur := pek
		for cur != "" {
			if visited[cur] {
				add(cp+".parentEntityKey", fmt.Sprintf("cyclic parentEntityKey chain involving %q", cur))
				break
			}
			visited[cur] = true
			next, hasParent := parentByEntity[cur]
			if !hasParent {
				break
			}
			depth++
			if depth > 2 {
				add(cp+".parentEntityKey", fmt.Sprintf("parentEntityKey nesting depth exceeds maximum of 2 (chain depth %d)", depth))
				break
			}
			cur = next
		}
		// Account for inline children attached to a card that already has a parentEntityKey.
		if len(mod.Cards[i].Children) > 0 {
			maxInline := maxInlineChildDepth(mod.Cards[i])
			if depth+maxInline > 2 {
				add(cp+".children", fmt.Sprintf("combined parentEntityKey + inline children depth (%d) exceeds maximum of 2", depth+maxInline))
			}
		}
	}

	// Validate overlays
	for i := range mod.Overlays {
		op := fmt.Sprintf("overlays[%d]", i)
		for j := range mod.Overlays[i].ChildCards {
			cp := fmt.Sprintf("%s.childCards[%d]", op, j)
			// Overlay child cards attach at depth >= 1 under their target card.
			childErrs := validateAndNormalizeCardDetailed(&mod.Overlays[i].ChildCards[j], mod.ID, 1, cp)
			errs = append(errs, childErrs...)
		}
	}

	// Validate pattern rules
	seenPatternIDs := make(map[string]bool, len(mod.Patterns))
	for i := range mod.Patterns {
		pp := fmt.Sprintf("patterns[%d]", i)
		pErrs := ValidatePatternRule(&mod.Patterns[i], pp)
		errs = append(errs, pErrs...)
		if mod.Patterns[i].ID != "" {
			if seenPatternIDs[mod.Patterns[i].ID] {
				add(pp+".id", fmt.Sprintf("duplicate pattern id %q in module", mod.Patterns[i].ID))
			}
			seenPatternIDs[mod.Patterns[i].ID] = true
		}
	}

	_ = entityCards
	return ValidationResult{
		Valid:  len(errs) == 0,
		Errors: errs,
	}
}

func maxInlineChildDepth(card SearchCard) int {
	if len(card.Children) == 0 {
		return 0
	}
	best := 0
	for _, ch := range card.Children {
		d := 1 + maxInlineChildDepth(ch)
		if d > best {
			best = d
		}
	}
	return best
}

// ValidateModuleJSON strictly decodes raw JSON bytes into a ModuleManifest (rejecting unknown fields
// such as legacy "template") and runs ValidateModuleManifest.
func ValidateModuleJSON(raw []byte) (*ModuleManifest, ValidationResult) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	if len(trimmed) == 0 {
		return nil, ValidationResult{
			Valid:  false,
			Errors: []FieldError{{Path: "", Message: "module JSON is empty"}},
		}
	}

	var mod ModuleManifest
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&mod); err != nil {
		return nil, ValidationResult{
			Valid:  false,
			Errors: []FieldError{{Path: "$", Message: fmt.Sprintf("invalid module JSON: %v", err)}},
		}
	}
	res := ValidateModuleManifest(&mod)
	return &mod, res
}

// LayoutNames returns sorted strings of the 9 canonical layout names for display/errors.
func LayoutNames() []string {
	all := AllLayouts()
	out := make([]string, len(all))
	for i, l := range all {
		out[i] = string(l)
	}
	sort.Strings(out)
	return out
}
