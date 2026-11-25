package search

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"vex/internal/indexer"
	"vex/internal/models"
)

// LivePatternExecutor bridges matched PatternRules with LiveMCP declarations
// to connected external MCP servers without introducing circular package imports.
type LivePatternExecutor interface {
	ExecutePatternMCP(ctx context.Context, serverID, toolName string, args map[string]any) (text string, localOnly bool, err error)
}

// PatternMatchResult bundles the synthesized decoded card (with nested Children populated)
// and any linked local cards discovered via LinkRules/LinkEntityKeys.
type PatternMatchResult struct {
	Card          models.SearchCard
	EnrichedCards []models.SearchCard
}

type compiledPattern struct {
	rule models.PatternRule
	re   *regexp.Regexp
}

// PatternEngine evaluates declarative, module-loaded PatternRules against user queries,
// normalizes inputs, validates standard checksums (IATA Mod-7, ISO 6346, Luhn),
// runs cross-card DataLookups & LinkRules from SQLite, and optionally queries MCP servers.
// No rules are hardcoded — all pattern rules are loadable and configurable via modules or ~/.vex/patterns/.
type PatternEngine struct {
	mu       sync.RWMutex
	compiled []compiledPattern
	allRules []models.PatternRule
}

// NewPatternEngine constructs a PatternEngine from the supplied user/module pattern rules.
func NewPatternEngine(rules []models.PatternRule) *PatternEngine {
	e := &PatternEngine{}
	e.SetCustomRules(rules)
	return e
}

// SetCustomRules replaces active pattern rules, deduplicating by ID and sorting by Priority DESC.
func (e *PatternEngine) SetCustomRules(rules []models.PatternRule) {
	seen := make(map[string]bool)
	merged := make([]models.PatternRule, 0, len(rules))

	for _, r := range rules {
		id := strings.TrimSpace(r.ID)
		if id == "" || strings.TrimSpace(r.Regex) == "" || seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, r)
	}

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Priority != merged[j].Priority {
			return merged[i].Priority > merged[j].Priority
		}
		return merged[i].ID < merged[j].ID
	})

	compiled := make([]compiledPattern, 0, len(merged))
	validRules := make([]models.PatternRule, 0, len(merged))
	for _, r := range merged {
		expr := r.Regex
		if r.CaseInsensitive && !strings.HasPrefix(expr, "(?i)") {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			continue
		}
		validRules = append(validRules, r)
		if r.IsEnabled() {
			compiled = append(compiled, compiledPattern{rule: r, re: re})
		}
	}

	e.mu.Lock()
	e.compiled = compiled
	e.allRules = validRules
	e.mu.Unlock()
}

// Rules returns a copy of all loaded PatternRules (including disabled rules so UI can toggle them).
func (e *PatternEngine) Rules() []models.PatternRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]models.PatternRule, len(e.allRules))
	copy(out, e.allRules)
	return out
}

// Match evaluates rawText against enabled compiled PatternRules, returning the highest-priority match
// enriched with cross-card DataLookups, recursive LinkRules, and optional live MCP data.
func (e *PatternEngine) Match(ctx context.Context, rawText string, store *indexer.Store, liveExec LivePatternExecutor) *PatternMatchResult {
	matches := e.MatchAll(ctx, rawText, store, liveExec, 1)
	if len(matches) == 0 {
		return nil
	}
	return matches[0]
}

// MatchAll evaluates rawText against all enabled compiled PatternRules and returns up to limit matches.
func (e *PatternEngine) MatchAll(ctx context.Context, rawText string, store *indexer.Store, liveExec LivePatternExecutor, limit int) []*PatternMatchResult {
	input := strings.TrimSpace(rawText)
	if input == "" {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}

	e.mu.RLock()
	compiled := e.compiled
	e.mu.RUnlock()

	var results []*PatternMatchResult
	for _, cp := range compiled {
		res := evaluateCompiledPattern(ctx, cp, input, store, liveExec)
		if res != nil {
			results = append(results, res)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

// TestRule compiles and evaluates a single PatternRule against sampleText without persisting it.
func TestRule(ctx context.Context, rule models.PatternRule, sampleText string, store *indexer.Store, liveExec LivePatternExecutor) (*PatternMatchResult, error) {
	expr := strings.TrimSpace(rule.Regex)
	if expr == "" {
		return nil, fmt.Errorf("pattern regex is required")
	}
	if rule.CaseInsensitive && !strings.HasPrefix(expr, "(?i)") {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}
	return evaluateCompiledPattern(ctx, compiledPattern{rule: rule, re: re}, strings.TrimSpace(sampleText), store, liveExec), nil
}

func evaluateCompiledPattern(ctx context.Context, cp compiledPattern, rawInput string, store *indexer.Store, liveExec LivePatternExecutor) *PatternMatchResult {
	normalized := applyPatternNormalizers(rawInput, cp.rule.Normalize)
	sub := cp.re.FindStringSubmatch(normalized)
	if sub == nil {
		return nil
	}

	captures := make(map[string]string, len(sub)+8)
	captures["raw"] = rawInput
	captures["match"] = normalized
	names := cp.re.SubexpNames()
	for i, name := range names {
		if i == 0 || name == "" || i >= len(sub) {
			continue
		}
		val := sub[i]
		if cp.rule.CaseInsensitive {
			val = strings.ToUpper(val)
		}
		captures[name] = val
	}

	// Optional standard checksum verification
	if cp.rule.Checksum != "" {
		valid, detail := validatePatternChecksum(cp.rule.Checksum, captures)
		if valid {
			captures["checksum.status"] = "Valid"
			captures["checksum.symbol"] = "✓"
		} else {
			captures["checksum.status"] = "Invalid"
			captures["checksum.symbol"] = "✗"
		}
		if detail != "" {
			captures["checksum.detail"] = detail
		}
	}

	// Optional dynamic DataLookups against indexed module cards in SQLite
	if store != nil && len(cp.rule.DataLookups) > 0 {
		resolveDataLookups(ctx, cp.rule, captures, store)
	} else if len(cp.rule.DataLookups) > 0 {
		applyDataLookupFallbacks(cp.rule, captures)
	}

	card := buildPatternCard(cp.rule, captures)

	// Optional live MCP lookup
	if cp.rule.LiveMCP != nil {
		enrichCardFromLiveMCP(ctx, &card, cp.rule, captures, liveExec)
	}

	// Combine rich LinkRules and legacy LinkEntityKeys into a unified list
	linkRules := make([]models.PatternLinkRule, 0, len(cp.rule.LinkRules)+len(cp.rule.LinkEntityKeys))
	linkRules = append(linkRules, cp.rule.LinkRules...)
	for _, keyTmpl := range cp.rule.LinkEntityKeys {
		linkRules = append(linkRules, models.PatternLinkRule{
			TargetEntityKey: keyTmpl,
			NestChildren:    true,
			MaxDepth:        2,
		})
	}

	var enriched []models.SearchCard
	if store != nil && len(linkRules) > 0 {
		enriched = applyPatternLinkRules(ctx, &card, cp.rule, linkRules, captures, store)
	}

	card.UpdatedAt = time.Now().UTC()
	return &PatternMatchResult{
		Card:          card,
		EnrichedCards: enriched,
	}
}

func applyPatternNormalizers(input string, normalizers []string) string {
	out := input
	for _, n := range normalizers {
		switch strings.ToLower(strings.TrimSpace(n)) {
		case "trim":
			out = strings.TrimSpace(out)
		case "uppercase", "upper":
			out = strings.ToUpper(out)
		case "lowercase", "lower":
			out = strings.ToLower(out)
		case "strip-spaces", "no-spaces":
			out = strings.ReplaceAll(out, " ", "")
		case "strip-dashes", "no-dashes":
			out = strings.ReplaceAll(out, "-", "")
		}
	}
	return out
}

func resolveDataLookups(ctx context.Context, rule models.PatternRule, captures map[string]string, store *indexer.Store) {
	for _, dl := range rule.DataLookups {
		alias := strings.TrimSpace(dl.Alias)
		if alias == "" {
			continue
		}
		targetKey := strings.TrimSpace(interpolatePattern(dl.TargetEntityKey, rule, captures))
		var matched *models.SearchCard
		if targetKey != "" {
			if found, err := store.FindCardsByEntityKeys(ctx, []string{targetKey}); err == nil && len(found) > 0 {
				matched = &found[0]
			}
		}

		if matched != nil {
			captures[alias+".title"] = matched.Title
			captures[alias+".subtitle"] = matched.Subtitle
			for outKey, srcExpr := range dl.ExtractFields {
				val := extractCardField(*matched, srcExpr)
				if val != "" {
					captures[alias+"."+outKey] = val
				}
			}
		}

		// Fill any missing keys with fallback values
		for outKey, fbTmpl := range dl.FallbackValues {
			fullKey := alias + "." + outKey
			if strings.TrimSpace(captures[fullKey]) == "" {
				captures[fullKey] = strings.TrimSpace(interpolatePattern(fbTmpl, rule, captures))
			}
		}
	}
}

func applyDataLookupFallbacks(rule models.PatternRule, captures map[string]string) {
	for _, dl := range rule.DataLookups {
		alias := strings.TrimSpace(dl.Alias)
		if alias == "" {
			continue
		}
		for outKey, fbTmpl := range dl.FallbackValues {
			captures[alias+"."+outKey] = strings.TrimSpace(interpolatePattern(fbTmpl, rule, captures))
		}
	}
}

func extractCardField(c models.SearchCard, expr string) string {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "title":
		return c.Title
	case "subtitle":
		return c.Subtitle
	case "description":
		return c.Description
	case "entityKey":
		return c.EntityKey
	case "collection":
		return c.Collection
	}
	if strings.HasPrefix(expr, "fact.") {
		label := strings.TrimSpace(strings.TrimPrefix(expr, "fact."))
		if val := extractFactByLabel(c.Data, label); val != "" {
			return val
		}
	}
	if strings.HasPrefix(expr, "meta.") {
		key := strings.TrimSpace(strings.TrimPrefix(expr, "meta."))
		if meta, ok := c.Data["meta"].(map[string]any); ok {
			if v, exists := meta[key]; exists && v != nil {
				return strings.TrimSpace(fmt.Sprintf("%v", v))
			}
		}
	}
	if strings.HasPrefix(expr, "data.") {
		key := strings.TrimPrefix(expr, "data.")
		if v, ok := c.Data[key]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		if val := extractFactByLabel(c.Data, key); val != "" {
			return val
		}
	}
	if v, ok := c.Data[expr]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func extractFactByLabel(data map[string]any, label string) string {
	if len(data) == 0 || label == "" {
		return ""
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	var payload models.FactsLayoutData
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	for _, f := range payload.Facts {
		if strings.EqualFold(strings.TrimSpace(f.Label), label) {
			return strings.TrimSpace(f.Value)
		}
	}
	for _, v := range payload.Views {
		for _, f := range v.Facts {
			if strings.EqualFold(strings.TrimSpace(f.Label), label) {
				return strings.TrimSpace(f.Value)
			}
		}
	}
	return ""
}

func applyPatternLinkRules(
	ctx context.Context,
	card *models.SearchCard,
	rule models.PatternRule,
	linkRules []models.PatternLinkRule,
	captures map[string]string,
	store *indexer.Store,
) []models.SearchCard {
	var enriched []models.SearchCard
	seenCardIDs := make(map[string]bool)
	childIdxByEntity := make(map[string]int)

	for _, lr := range linkRules {
		var linked []models.SearchCard
		if strings.TrimSpace(lr.TargetEntityKey) != "" {
			resolvedKey := strings.TrimSpace(interpolatePattern(lr.TargetEntityKey, rule, captures))
			if resolvedKey != "" {
				if found, err := store.FindCardsByEntityKeys(ctx, []string{resolvedKey}); err == nil {
					linked = found
				}
			}
		} else if strings.TrimSpace(lr.TargetCollection) != "" || strings.TrimSpace(lr.TargetTag) != "" {
			opts := indexer.SearchOptions{
				Collection: strings.TrimSpace(interpolatePattern(lr.TargetCollection, rule, captures)),
				Limit:      3,
			}
			if tag := strings.TrimSpace(interpolatePattern(lr.TargetTag, rule, captures)); tag != "" {
				opts.Tags = []string{tag}
			}
			if res, err := store.SearchHybrid(ctx, opts); err == nil {
				for _, r := range res {
					linked = append(linked, r.Card)
				}
			}
		}

		for _, lc := range linked {
			if seenCardIDs[lc.ID] && !lr.CopyChildrenOnly {
				continue
			}
			seenCardIDs[lc.ID] = true
			enriched = append(enriched, lc)

			if lc.EntityKey != "" {
				card.RelatedKeys = appendUniquePatternStr(card.RelatedKeys, lc.EntityKey)
			}

			// Record summary section for RAG/back-compat
			secFields := make(map[string]any)
			fieldKeys := lr.IncludeFields
			if len(fieldKeys) == 0 {
				fieldKeys = []string{"unlocode", "iata", "country", "currency", "timezone", "region", "prefix", "scac", "isoCode"}
			}
			for _, fk := range fieldKeys {
				if v, ok := lc.Data[fk]; ok && v != nil && fmt.Sprintf("%v", v) != "" {
					secFields[fk] = v
				}
			}
			card.Sections = append(card.Sections, models.CardSection{
				ModuleID:   "linked-" + lc.ID,
				ModuleName: firstNonEmptyPattern(lr.RelationLabel, lc.Title),
				Title:      lc.Title,
				Badge:      firstNonEmptyPattern(lc.EntityKey, lc.Collection),
				Notes:      firstNonEmptyPattern(lc.Subtitle, lc.Description),
				Fields:     secFields,
				Actions:    lc.Actions,
				Provenance: lc.Provenance,
				LocalOnly:  lc.LocalOnly,
			})

			// Attach as nested Rolodex child card(s)
			if lr.CopyChildrenOnly {
				for _, subChild := range lc.Children {
					sc := subChild
					sc.DefaultExpanded = lr.AutoExpand
					sc.Children = nil
					card.Children = append(card.Children, sc)
				}
				continue
			}

			nested := lc
			nested.DefaultExpanded = lr.AutoExpand
			maxDepth := lr.MaxDepth
			if maxDepth <= 0 {
				maxDepth = 2
			}
			if !lr.NestChildren || maxDepth < 2 {
				nested.Children = nil
			} else {
				// Trim any grandchildren beyond allowed depth
				for i := range nested.Children {
					nested.Children[i].DefaultExpanded = false
					nested.Children[i].Children = nil
				}
			}

			// If AttachUnder matches an already-attached parent child card, nest inside it!
			attachedUnderParent := false
			if strings.TrimSpace(lr.AttachUnder) != "" {
				parentKey := strings.ToLower(strings.TrimSpace(interpolatePattern(lr.AttachUnder, rule, captures)))
				if idx, ok := childIdxByEntity[parentKey]; ok && idx >= 0 && idx < len(card.Children) {
					nested.Children = nil
					card.Children[idx].Children = append(card.Children[idx].Children, nested)
					attachedUnderParent = true
				}
			}

			if !attachedUnderParent {
				idx := len(card.Children)
				card.Children = append(card.Children, nested)
				if nested.EntityKey != "" {
					childIdxByEntity[strings.ToLower(nested.EntityKey)] = idx
				}
			}
		}
	}
	return enriched
}

func buildPatternCard(rule models.PatternRule, captures map[string]string) models.SearchCard {
	col := strings.TrimSpace(rule.Collection)
	if col == "" {
		col = "patterns"
	}
	layout := rule.Layout
	if layout == "" {
		layout = models.LayoutSegments
	}
	icon := rule.Icon
	if icon == "" {
		icon = "🧩"
	}

	title := strings.TrimSpace(interpolatePattern(rule.TitleTemplate, rule, captures))
	subtitle := strings.TrimSpace(interpolatePattern(rule.SubtitleTemplate, rule, captures))
	description := strings.TrimSpace(interpolatePattern(rule.DescriptionTemplate, rule, captures))

	data := make(map[string]any)
	switch layout {
	case models.LayoutSegments:
		defaultTones := []string{"blue", "amber", "green"}
		segs := make([]map[string]string, 0, max(1, len(rule.Segments)))
		for i, seg := range rule.Segments {
			tone := strings.TrimSpace(seg.Tone)
			if tone == "" {
				tone = defaultTones[i%len(defaultTones)]
			}
			segs = append(segs, map[string]string{
				"value":   strings.TrimSpace(interpolatePattern(seg.Value, rule, captures)),
				"role":    strings.TrimSpace(interpolatePattern(seg.Role, rule, captures)),
				"meaning": strings.TrimSpace(interpolatePattern(seg.Meaning, rule, captures)),
				"tone":    tone,
			})
		}
		if len(segs) == 0 {
			segs = append(segs, map[string]string{
				"value":   captures["match"],
				"role":    "MATCH",
				"meaning": firstNonEmptyPattern(rule.Name, rule.ID),
				"tone":    "blue",
			})
		}
		data["segments"] = segs
		explanation := strings.TrimSpace(interpolatePattern(rule.ExplanationTemplate, rule, captures))
		if explanation == "" {
			explanation = firstNonEmptyPattern(description, subtitle, rule.Description, rule.Name)
		}
		data["explanation"] = explanation

	case models.LayoutFacts:
		if len(rule.Fields) > 0 {
			keys := make([]string, 0, len(rule.Fields))
			for k := range rule.Fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			items := make([]map[string]any, 0, len(keys))
			for _, k := range keys {
				if v := strings.TrimSpace(interpolatePattern(rule.Fields[k], rule, captures)); v != "" {
					items = append(items, map[string]any{"label": k, "value": v})
				}
			}
			if len(items) > 0 {
				data["facts"] = items
			}
		}
		if summary := firstNonEmptyPattern(strings.TrimSpace(interpolatePattern(rule.ExplanationTemplate, rule, captures)), description, subtitle); summary != "" {
			data["summary"] = summary
		} else if len(data) == 0 {
			data["summary"] = firstNonEmptyPattern(rule.Name, captures["match"])
		}
	}

	var chips []models.GlanceChip
	for _, ch := range rule.Chips {
		label := strings.TrimSpace(interpolatePattern(ch.Label, rule, captures))
		if label == "" {
			continue
		}
		tone := ch.Tone
		if tone == "" {
			tone = "neutral"
		}
		// Automatically switch Mod-7 / Mod-11 chip tone to red when checksum is Invalid
		if strings.Contains(ch.Label, "checksum.") && captures["checksum.status"] == "Invalid" {
			tone = "red"
		}
		chips = append(chips, models.GlanceChip{
			Label: label,
			Tone:  tone,
			Dot:   ch.Dot,
		})
		if len(chips) == 4 {
			break
		}
	}

	actions := make([]models.CardAction, 0, len(rule.Actions))
	for _, a := range rule.Actions {
		aCopy := a
		aCopy.Label = interpolatePattern(a.Label, rule, captures)
		aCopy.Target = interpolatePattern(a.Target, rule, captures)
		actions = append(actions, aCopy)
	}
	if len(actions) == 0 {
		actions = append(actions, models.CardAction{
			ID:       "copy-match",
			Label:    "Copy Reference",
			Shortcut: "Enter",
			Type:     "copy",
			Target:   captures["match"],
		})
	}

	var prov *models.Provenance
	if rule.Provenance != nil {
		p := *rule.Provenance
		if rule.LocalOnly {
			p.LocalOnly = true
		}
		prov = &p
	} else {
		prov = &models.Provenance{
			SourceName: rule.Name,
			Authority:  "internal-curated",
			LocalOnly:  rule.LocalOnly,
		}
	}

	return models.SearchCard{
		ID:              "pattern-" + models.Slugify(rule.ID) + "-" + models.Slugify(captures["match"]),
		Title:           title,
		Subtitle:        subtitle,
		Description:     description,
		Collection:      col,
		Layout:          layout,
		Icon:            icon,
		Tags:            []string{"pattern", rule.ID},
		Chips:           chips,
		Data:            data,
		DefaultExpanded: rule.DefaultExpanded,
		Actions:         actions,
		Provenance:      prov,
		LocalOnly:       rule.LocalOnly,
	}
}

func enrichCardFromLiveMCP(ctx context.Context, card *models.SearchCard, rule models.PatternRule, captures map[string]string, liveExec LivePatternExecutor) {
	attachLiveFacts := func(title, summary string, facts []map[string]any) {
		payload := map[string]any{}
		if strings.TrimSpace(summary) != "" {
			payload["summary"] = strings.TrimSpace(summary)
		}
		if len(facts) > 0 {
			payload["facts"] = facts
		}
		if len(payload) == 0 {
			return
		}
		card.Children = append(card.Children, models.SearchCard{
			ID:         card.ID + "-live-mcp",
			Title:      title,
			Collection: card.Collection,
			Layout:     models.LayoutFacts,
			Icon:       "⚡",
			Data:       payload,
			LocalOnly:  card.LocalOnly,
		})
	}

	if liveExec == nil {
		attachLiveFacts("Live MCP Lookup", fmt.Sprintf("Connect %q MCP server to fetch live details", rule.LiveMCP.ServerID), nil)
		return
	}

	args := make(map[string]any, len(rule.LiveMCP.Arguments))
	for k, tmpl := range rule.LiveMCP.Arguments {
		args[k] = strings.TrimSpace(interpolatePattern(tmpl, rule, captures))
	}

	liveCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	text, serverLocalOnly, err := liveExec.ExecutePatternMCP(liveCtx, rule.LiveMCP.ServerID, rule.LiveMCP.ToolName, args)
	if err != nil {
		attachLiveFacts("Live MCP Lookup", fmt.Sprintf("MCP %q offline (%s)", rule.LiveMCP.ServerID, err.Error()), nil)
		return
	}
	if serverLocalOnly || rule.LocalOnly {
		card.LocalOnly = true
		if card.Provenance != nil {
			card.Provenance.LocalOnly = true
		}
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err == nil && len(obj) > 0 {
		if title, ok := obj["title"].(string); ok && strings.TrimSpace(title) != "" {
			card.Subtitle = strings.TrimSpace(title)
		}
		if desc, ok := obj["description"].(string); ok && strings.TrimSpace(desc) != "" {
			card.Description = strings.TrimSpace(desc)
		}
		if rawURL, ok := obj["url"].(string); ok && (strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://")) {
			card.Actions = append([]models.CardAction{
				{ID: "open-live-url", Label: "Open in Browser", Shortcut: "Enter", Type: "open_url", Target: rawURL},
			}, card.Actions...)
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var facts []map[string]any
		for _, k := range keys {
			if k == "description" || k == "url" {
				continue
			}
			v := obj[k]
			switch tv := v.(type) {
			case string:
				if strings.TrimSpace(tv) != "" {
					facts = append(facts, map[string]any{"label": k, "value": strings.TrimSpace(tv)})
				}
			case float64, bool:
				facts = append(facts, map[string]any{"label": k, "value": fmt.Sprintf("%v", tv)})
			}
		}
		attachLiveFacts("Live MCP Details", "", facts)
		return
	}

	card.Description = text
	attachLiveFacts("Live MCP Details", text, nil)
}

var patternPlaceholderRe = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

func interpolatePattern(tmpl string, rule models.PatternRule, captures map[string]string) string {
	if !strings.Contains(tmpl, "{{") {
		return tmpl
	}
	return patternPlaceholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		sub := patternPlaceholderRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		key := strings.TrimSpace(sub[1])
		if strings.HasPrefix(key, "lookup.") {
			lookupName := strings.TrimPrefix(key, "lookup.")
			return resolvePatternLookup(rule, lookupName, captures)
		}
		if v, ok := captures[key]; ok {
			return v
		}
		return ""
	})
}

func resolvePatternLookup(rule models.PatternRule, lookupName string, captures map[string]string) string {
	table, ok := rule.Lookups[lookupName]
	if !ok {
		return ""
	}
	raw, exists := captures[lookupName]
	if !exists {
		if idx := strings.IndexByte(lookupName, '_'); idx > 0 {
			raw, exists = captures[lookupName[:idx]]
		}
	}
	if !exists {
		return ""
	}
	if mapped, found := table[raw]; found {
		return mapped
	}
	if mapped, found := table[strings.ToUpper(raw)]; found {
		return mapped
	}
	if mapped, found := table["*"]; found {
		return mapped
	}
	return raw
}

func validatePatternChecksum(kind string, captures map[string]string) (bool, string) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "iata-mod7":
		serial, err1 := strconv.Atoi(captures["serial"])
		check, err2 := strconv.Atoi(captures["check"])
		if err1 != nil || err2 != nil {
			return false, "non-numeric AWB serial"
		}
		expected := serial % 7
		if expected == check {
			return true, fmt.Sprintf("%d mod 7 = %d", serial, expected)
		}
		return false, fmt.Sprintf("expected %d, got %d", expected, check)

	case "iso6346":
		code := strings.ToUpper(captures["bic"] + captures["serial"])
		check, err := strconv.Atoi(captures["check"])
		if len(code) != 10 || err != nil {
			return false, "invalid ISO 6346 length"
		}
		expected := computeISO6346CheckDigit(code)
		if expected == check {
			return true, fmt.Sprintf("ISO 6346 check = %d", expected)
		}
		return false, fmt.Sprintf("expected %d, got %d", expected, check)

	case "luhn":
		digits := captures["match"]
		if d, ok := captures["digits"]; ok && d != "" {
			digits = d
		}
		if validateLuhn(digits) {
			return true, "Luhn check passed"
		}
		return false, "Luhn check failed"
	}
	return true, ""
}

func validateLuhn(s string) bool {
	var nums []int
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			nums = append(nums, int(s[i]-'0'))
		}
	}
	if len(nums) < 2 {
		return false
	}
	sum := 0
	alt := false
	for i := len(nums) - 1; i >= 0; i-- {
		n := nums[i]
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}

func computeISO6346CheckDigit(tenCharCode string) int {
	sum := 0
	weight := 1
	for i := 0; i < len(tenCharCode); i++ {
		ch := tenCharCode[i]
		var val int
		switch {
		case ch >= '0' && ch <= '9':
			val = int(ch - '0')
		case ch >= 'A' && ch <= 'Z':
			val = iso6346LetterValue(ch)
		default:
			return -1
		}
		sum += val * weight
		weight *= 2
	}
	return (sum % 11) % 10
}

func iso6346LetterValue(ch byte) int {
	val := 10
	for c := byte('A'); c <= 'Z'; c++ {
		if val%11 == 0 {
			val++
		}
		if c == ch {
			return val
		}
		val++
	}
	return 0
}

func appendUniquePatternStr(list []string, item string) []string {
	for _, existing := range list {
		if strings.EqualFold(existing, item) {
			return list
		}
	}
	return append(list, item)
}

func firstNonEmptyPattern(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
