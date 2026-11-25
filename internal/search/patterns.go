package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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

// LiveHTTPRequest describes a strictly read-only HTTP GET enrichment request
// issued for a matched PatternRule.
type LiveHTTPRequest struct {
	URL         string
	Auth        string            // "none" | "gh_cli" | "az_devops" | "env:VAR_NAME"
	Headers     map[string]string // Non-auth headers only (e.g. Accept)
	TimeoutMs   int               // Default 1500, max 5000
	CacheTTLSec int               // Default 60
}

// LiveHTTPExecutor performs strictly read-only GET lookups with TTL caching and
// safe auth resolution (implemented in internal/mcp to keep package deps acyclic).
type LiveHTTPExecutor interface {
	ExecuteLiveHTTP(ctx context.Context, req LiveHTTPRequest) (body []byte, statusCode int, err error)
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
	liveHTTP LiveHTTPExecutor
}

// NewPatternEngine constructs a PatternEngine from the supplied user/module pattern rules.
func NewPatternEngine(rules []models.PatternRule) *PatternEngine {
	e := &PatternEngine{}
	e.SetCustomRules(rules)
	return e
}

// SetLiveHTTPExecutor attaches a read-only LiveHTTPExecutor used for PatternRules
// configured with liveHttp GET lookups.
func (e *PatternEngine) SetLiveHTTPExecutor(exec LiveHTTPExecutor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.liveHTTP = exec
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
	liveHTTP := e.liveHTTP
	e.mu.RUnlock()

	var results []*PatternMatchResult
	for _, cp := range compiled {
		res := evaluateCompiledPattern(ctx, cp, input, store, liveExec, liveHTTP, false)
		if res != nil {
			results = append(results, res)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

// FetchLiveHTTP forces execution of a rule's liveHttp GET lookup on demand (used when
// trigger is "manual" and the user clicks "Fetch Live Data", or when retrying a lookup).
func (e *PatternEngine) FetchLiveHTTP(
	ctx context.Context,
	ruleID, rawQuery string,
	store *indexer.Store,
	liveExec LivePatternExecutor,
	overrideHTTP LiveHTTPExecutor,
) (*PatternMatchResult, error) {
	targetID := models.Slugify(strings.TrimSpace(ruleID))
	input := strings.TrimSpace(rawQuery)
	if targetID == "" || input == "" {
		return nil, fmt.Errorf("ruleID and rawQuery are required")
	}

	e.mu.RLock()
	liveHTTP := overrideHTTP
	if liveHTTP == nil {
		liveHTTP = e.liveHTTP
	}
	var targetRule *models.PatternRule
	for i := range e.allRules {
		if models.Slugify(e.allRules[i].ID) == targetID {
			r := e.allRules[i]
			targetRule = &r
			break
		}
	}
	e.mu.RUnlock()

	if targetRule == nil {
		return nil, fmt.Errorf("pattern rule %q not found", ruleID)
	}
	if targetRule.LiveHTTP == nil {
		return nil, fmt.Errorf("pattern rule %q has no liveHttp configuration", ruleID)
	}

	expr := strings.TrimSpace(targetRule.Regex)
	if targetRule.CaseInsensitive && !strings.HasPrefix(expr, "(?i)") {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex on rule %q: %w", ruleID, err)
	}

	res := evaluateCompiledPattern(ctx, compiledPattern{rule: *targetRule, re: re}, input, store, liveExec, liveHTTP, true)
	if res == nil {
		return nil, fmt.Errorf("query %q did not match rule %q", rawQuery, ruleID)
	}
	return res, nil
}

// TestRule compiles and evaluates a single PatternRule against sampleText without persisting it.
func TestRule(ctx context.Context, rule models.PatternRule, sampleText string, store *indexer.Store, liveExec LivePatternExecutor) (*PatternMatchResult, error) {
	return TestRuleWithLive(ctx, rule, sampleText, store, liveExec, nil)
}

// TestRuleWithLive compiles and evaluates a single PatternRule with optional MCP and LiveHTTP executors.
func TestRuleWithLive(
	ctx context.Context,
	rule models.PatternRule,
	sampleText string,
	store *indexer.Store,
	liveExec LivePatternExecutor,
	liveHTTP LiveHTTPExecutor,
) (*PatternMatchResult, error) {
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
	return evaluateCompiledPattern(ctx, compiledPattern{rule: rule, re: re}, strings.TrimSpace(sampleText), store, liveExec, liveHTTP, false), nil
}

func evaluateCompiledPattern(
	ctx context.Context,
	cp compiledPattern,
	rawInput string,
	store *indexer.Store,
	liveExec LivePatternExecutor,
	liveHTTP LiveHTTPExecutor,
	forceLiveHTTP bool,
) *PatternMatchResult {
	normalized := applyPatternNormalizers(rawInput, cp.rule.Normalize)
	sub := cp.re.FindStringSubmatch(normalized)
	if sub == nil {
		return nil
	}

	captures := make(map[string]string, len(sub)*2+8)
	captures["raw"] = rawInput
	captures["match"] = normalized
	captures["0"] = sub[0]
	names := cp.re.SubexpNames()
	for i := 1; i < len(sub); i++ {
		val := sub[i]
		if cp.rule.CaseInsensitive {
			val = strings.ToUpper(val)
		}
		captures[strconv.Itoa(i)] = val
		if i < len(names) && names[i] != "" {
			captures[names[i]] = val
		}
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

	// Optional declarative read-only liveHttp GET enrichment
	if cp.rule.LiveHTTP != nil {
		if forceLiveHTTP || cp.rule.LiveHTTP.EffectiveTrigger() == "auto" {
			ok := enrichCardFromLiveHTTP(ctx, &card, cp.rule, captures, liveHTTP)
			if !ok && cp.rule.LiveHTTP.EffectiveTrigger() == "manual" {
				// Keep retry button available on manual lookup failure
				attachManualLiveHTTPAction(&card, cp.rule, rawInput, captures)
			}
		} else {
			attachManualLiveHTTPAction(&card, cp.rule, rawInput, captures)
		}
	}

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

const (
	LiveHTTPActionID          = "fetch-live-http"
	LiveLookupUnavailableChip = "Live lookup unavailable"
)

func attachManualLiveHTTPAction(card *models.SearchCard, rule models.PatternRule, rawInput string, captures map[string]string) {
	if rule.LiveHTTP == nil {
		return
	}
	resolvedURL := interpolatePatternURL(rule.LiveHTTP.URLTemplate, rule, captures)
	for _, a := range card.Actions {
		if a.ID == LiveHTTPActionID {
			return
		}
	}
	card.Actions = append(card.Actions, models.CardAction{
		ID:     LiveHTTPActionID,
		Label:  "Fetch Live Data",
		Type:   "http",
		Target: resolvedURL,
		Env: map[string]string{
			"liveHttpRuleId":   rule.ID,
			"liveHttpRawQuery": rawInput,
			"liveHttpAuth":     rule.LiveHTTP.EffectiveAuth(),
		},
	})
}

func appendLiveUnavailableChip(card *models.SearchCard) {
	for _, ch := range card.Chips {
		if strings.EqualFold(ch.Label, LiveLookupUnavailableChip) {
			return
		}
	}
	chip := models.GlanceChip{
		Label: LiveLookupUnavailableChip,
		Tone:  "neutral",
	}
	if len(card.Chips) >= 4 {
		card.Chips[3] = chip
	} else {
		card.Chips = append(card.Chips, chip)
	}
}

func enrichCardFromLiveHTTP(
	ctx context.Context,
	card *models.SearchCard,
	rule models.PatternRule,
	captures map[string]string,
	liveHTTP LiveHTTPExecutor,
) bool {
	if rule.LiveHTTP == nil {
		return false
	}
	if liveHTTP == nil {
		appendLiveUnavailableChip(card)
		return false
	}

	resolvedURL := interpolatePatternURL(rule.LiveHTTP.URLTemplate, rule, captures)
	if err := models.ValidateHTTPURLStrict(resolvedURL); err != nil {
		appendLiveUnavailableChip(card)
		return false
	}

	timeoutMs := rule.LiveHTTP.EffectiveTimeoutMs()
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	body, _, err := liveHTTP.ExecuteLiveHTTP(reqCtx, LiveHTTPRequest{
		URL:         resolvedURL,
		Auth:        rule.LiveHTTP.EffectiveAuth(),
		Headers:     rule.LiveHTTP.Headers,
		TimeoutMs:   timeoutMs,
		CacheTTLSec: rule.LiveHTTP.EffectiveCacheTTLSec(),
	})
	if err != nil {
		appendLiveUnavailableChip(card)
		return false
	}

	var parsedJSON any
	if err := json.Unmarshal(body, &parsedJSON); err != nil {
		appendLiveUnavailableChip(card)
		return false
	}

	if rule.LiveHTTP.EffectiveAuth() != "none" || rule.LocalOnly {
		card.LocalOnly = true
		if card.Provenance != nil {
			card.Provenance.LocalOnly = true
		}
	}

	child, ok := projectLiveHTTPResultCard(card, rule, captures, parsedJSON)
	if !ok {
		appendLiveUnavailableChip(card)
		return false
	}
	card.Children = append(card.Children, child)
	return true
}

// interpolatePatternURL safely escapes capture values using url.PathEscape for the
// URL path portion and url.QueryEscape for the query string portion.
func interpolatePatternURL(tmpl string, rule models.PatternRule, captures map[string]string) string {
	trimmed := strings.TrimSpace(tmpl)
	if !strings.Contains(trimmed, "{{") {
		return trimmed
	}

	escapeWith := func(part string, esc func(string) string) string {
		return patternPlaceholderRe.ReplaceAllStringFunc(part, func(m string) string {
			sub := patternPlaceholderRe.FindStringSubmatch(m)
			if len(sub) < 2 {
				return ""
			}
			key := strings.TrimSpace(sub[1])
			var val string
			if strings.HasPrefix(key, "lookup.") {
				val = resolvePatternLookup(rule, strings.TrimPrefix(key, "lookup."), captures)
			} else {
				val = captures[key]
			}
			return esc(val)
		})
	}

	if qIdx := strings.IndexByte(trimmed, '?'); qIdx >= 0 {
		basePart := trimmed[:qIdx]
		queryPart := trimmed[qIdx+1:]
		return escapeWith(basePart, url.PathEscape) + "?" + escapeWith(queryPart, url.QueryEscape)
	}
	return escapeWith(trimmed, url.PathEscape)
}

func interpolatePatternWithJSON(tmpl string, rule models.PatternRule, captures map[string]string, root any, item any) string {
	if !strings.Contains(tmpl, "{{") {
		return tmpl
	}
	return patternPlaceholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		sub := patternPlaceholderRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		key := strings.TrimSpace(sub[1])
		if strings.HasPrefix(key, "json.") {
			if val, ok := resolveJSONDotPath(root, strings.TrimPrefix(key, "json.")); ok {
				return formatJSONScalar(val)
			}
			return ""
		}
		if strings.HasPrefix(key, "item.") && item != nil {
			if val, ok := resolveJSONDotPath(item, strings.TrimPrefix(key, "item.")); ok {
				return formatJSONScalar(val)
			}
			return ""
		}
		if strings.HasPrefix(key, "lookup.") {
			return resolvePatternLookup(rule, strings.TrimPrefix(key, "lookup."), captures)
		}
		if v, ok := captures[key]; ok {
			return v
		}
		return ""
	})
}

func resolveJSONDotPath(root any, dotPath string) (any, bool) {
	path := strings.TrimSpace(dotPath)
	path = strings.TrimPrefix(path, "json.")
	if path == "" || root == nil {
		return root, root != nil
	}

	// Normalize bracket indices like items[0].name -> items.0.name
	normalized := strings.ReplaceAll(path, "[", ".")
	normalized = strings.ReplaceAll(normalized, "]", "")
	rawParts := strings.Split(normalized, ".")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		if p != "" {
			parts = append(parts, p)
		}
	}

	return walkJSONPath(root, parts)
}

func walkJSONPath(cur any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return cur, cur != nil
	}
	switch node := cur.(type) {
	case map[string]any:
		// First try exact segment match
		if next, ok := node[parts[0]]; ok {
			return walkJSONPath(next, parts[1:])
		}
		// Support dotted map keys (e.g. Azure DevOps "System.Title" inside "fields")
		for end := len(parts); end >= 2; end-- {
			joined := strings.Join(parts[:end], ".")
			if next, ok := node[joined]; ok {
				return walkJSONPath(next, parts[end:])
			}
		}
		return nil, false
	case []any:
		idx, err := strconv.Atoi(parts[0])
		if err != nil || idx < 0 || idx >= len(node) {
			return nil, false
		}
		return walkJSONPath(node[idx], parts[1:])
	default:
		return nil, false
	}
}

func formatJSONScalar(v any) string {
	switch tv := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(tv)
	case float64:
		if tv == float64(int64(tv)) {
			return strconv.FormatInt(int64(tv), 10)
		}
		return strconv.FormatFloat(tv, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(tv)
	case []any:
		parts := make([]string, 0, len(tv))
		for _, elem := range tv {
			if s := formatJSONScalar(elem); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		raw, err := json.Marshal(tv)
		if err != nil {
			return fmt.Sprintf("%v", tv)
		}
		return string(raw)
	}
}

func resolveItemField(rule models.PatternRule, captures map[string]string, root any, item any, expr string) string {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "{{") {
		return strings.TrimSpace(interpolatePatternWithJSON(trimmed, rule, captures, root, item))
	}
	if val, ok := resolveJSONDotPath(item, trimmed); ok {
		return formatJSONScalar(val)
	}
	return ""
}

func projectLiveHTTPResultCard(
	parent *models.SearchCard,
	rule models.PatternRule,
	captures map[string]string,
	parsedJSON any,
) (models.SearchCard, bool) {
	spec := rule.LiveHTTP.ResultCard
	title := strings.TrimSpace(interpolatePatternWithJSON(spec.Title, rule, captures, parsedJSON, nil))
	if title == "" {
		title = "Live Details"
	}
	icon := strings.TrimSpace(spec.Icon)
	if icon == "" {
		icon = "⚡"
	}

	var chips []models.GlanceChip
	for _, ch := range spec.Chips {
		lbl := strings.TrimSpace(interpolatePatternWithJSON(ch.Label, rule, captures, parsedJSON, nil))
		if lbl == "" {
			continue
		}
		tone := ch.Tone
		if tone == "" {
			tone = "neutral"
		}
		chips = append(chips, models.GlanceChip{Label: lbl, Tone: tone, Dot: ch.Dot})
		if len(chips) == 4 {
			break
		}
	}

	data := make(map[string]any)
	var actions []models.CardAction

	switch spec.Layout {
	case models.LayoutFacts:
		if summary := strings.TrimSpace(interpolatePatternWithJSON(spec.Summary, rule, captures, parsedJSON, nil)); summary != "" {
			data["summary"] = summary
		}
		facts := make([]map[string]any, 0, len(spec.Facts))
		for _, f := range spec.Facts {
			lbl := strings.TrimSpace(interpolatePatternWithJSON(f.Label, rule, captures, parsedJSON, nil))
			val := strings.TrimSpace(interpolatePatternWithJSON(f.Value, rule, captures, parsedJSON, nil))
			if lbl == "" || val == "" {
				continue
			}
			item := map[string]any{"label": lbl, "value": val}
			if f.Mono {
				item["mono"] = true
			}
			if f.Copyable {
				item["copyable"] = true
			}
			facts = append(facts, item)
		}
		if len(facts) > 0 {
			data["facts"] = facts
		}
		if len(data) == 0 {
			return models.SearchCard{}, false
		}

	case models.LayoutLink:
		resolvedURL := strings.TrimSpace(interpolatePatternWithJSON(spec.URL, rule, captures, parsedJSON, nil))
		if err := models.ValidateHTTPURLStrict(resolvedURL); err != nil {
			return models.SearchCard{}, false
		}
		data["url"] = resolvedURL
		actions = append(actions, models.CardAction{
			ID:       "open-live-link",
			Label:    "Open in Browser",
			Shortcut: "Enter",
			Type:     "open_url",
			Target:   resolvedURL,
		})

	case models.LayoutTable:
		if len(spec.Columns) < 2 || len(spec.Columns) > 4 {
			return models.SearchCard{}, false
		}
		var rows [][]string
		if strings.TrimSpace(spec.RowsPath) != "" {
			arrVal, ok := resolveJSONDotPath(parsedJSON, spec.RowsPath)
			if !ok {
				return models.SearchCard{}, false
			}
			arr, ok := arrVal.([]any)
			if !ok || len(arr) == 0 {
				return models.SearchCard{}, false
			}
			maxRows := len(arr)
			if maxRows > 8 {
				maxRows = 8
			}
			rows = make([][]string, 0, maxRows)
			for i := 0; i < maxRows; i++ {
				row := make([]string, len(spec.Columns))
				for cIdx := range spec.Columns {
					if cIdx < len(spec.RowFields) {
						row[cIdx] = resolveItemField(rule, captures, parsedJSON, arr[i], spec.RowFields[cIdx])
					}
				}
				rows = append(rows, row)
			}
		} else {
			rows = make([][]string, 0, len(spec.Rows))
			for _, rawRow := range spec.Rows {
				if len(rawRow) != len(spec.Columns) {
					continue
				}
				row := make([]string, len(rawRow))
				for cIdx, cell := range rawRow {
					row[cIdx] = strings.TrimSpace(interpolatePatternWithJSON(cell, rule, captures, parsedJSON, nil))
				}
				rows = append(rows, row)
				if len(rows) >= 8 {
					break
				}
			}
		}
		if len(rows) == 0 {
			return models.SearchCard{}, false
		}
		data["columns"] = spec.Columns
		data["rows"] = rows
		if len(spec.MonoColumns) > 0 {
			data["monoColumns"] = spec.MonoColumns
		}
		if srcURL := strings.TrimSpace(interpolatePatternWithJSON(spec.SourceURL, rule, captures, parsedJSON, nil)); srcURL != "" {
			if models.ValidateHTTPURLStrict(srcURL) == nil {
				data["sourceUrl"] = srcURL
			}
		}

	case models.LayoutSteps:
		summary := strings.TrimSpace(interpolatePatternWithJSON(spec.Summary, rule, captures, parsedJSON, nil))
		if summary == "" {
			summary = title
		}
		var steps []map[string]any
		if strings.TrimSpace(spec.StepsPath) != "" {
			arrVal, ok := resolveJSONDotPath(parsedJSON, spec.StepsPath)
			if !ok {
				return models.SearchCard{}, false
			}
			arr, ok := arrVal.([]any)
			if !ok || len(arr) == 0 {
				return models.SearchCard{}, false
			}
			for _, elem := range arr {
				lbl := resolveItemField(rule, captures, parsedJSON, elem, spec.StepLabel)
				if lbl == "" {
					continue
				}
				st := map[string]any{"label": lbl}
				if capStr := resolveItemField(rule, captures, parsedJSON, elem, spec.StepCaption); capStr != "" {
					st["caption"] = capStr
				}
				steps = append(steps, st)
			}
		} else {
			for _, s := range spec.Steps {
				lbl := strings.TrimSpace(interpolatePatternWithJSON(s.Label, rule, captures, parsedJSON, nil))
				if lbl == "" {
					continue
				}
				st := map[string]any{"label": lbl}
				if capStr := strings.TrimSpace(interpolatePatternWithJSON(s.Caption, rule, captures, parsedJSON, nil)); capStr != "" {
					st["caption"] = capStr
				}
				steps = append(steps, st)
			}
		}
		if len(steps) == 0 {
			return models.SearchCard{}, false
		}
		data["steps"] = steps
		data["summary"] = summary
		activeStep := spec.ActiveStep
		if strings.TrimSpace(spec.ActiveStepPath) != "" {
			rawActive := resolveItemField(rule, captures, parsedJSON, parsedJSON, spec.ActiveStepPath)
			if n, err := strconv.Atoi(strings.TrimSpace(rawActive)); err == nil && n >= 1 && n <= len(steps) {
				activeStep = n
			}
		}
		if activeStep >= 1 && activeStep <= len(steps) {
			data["activeStep"] = activeStep
		}
		if len(spec.Bullets) > 0 {
			bullets := make([]string, 0, len(spec.Bullets))
			for _, b := range spec.Bullets {
				if resolved := strings.TrimSpace(interpolatePatternWithJSON(b, rule, captures, parsedJSON, nil)); resolved != "" {
					bullets = append(bullets, resolved)
				}
			}
			if len(bullets) > 0 {
				data["bullets"] = bullets
			}
		}

	default:
		return models.SearchCard{}, false
	}

	child := models.SearchCard{
		ID:              parent.ID + "-live-http",
		Title:           title,
		Collection:      parent.Collection,
		Layout:          spec.Layout,
		Icon:            icon,
		Chips:           chips,
		Data:            data,
		Actions:         actions,
		DefaultExpanded: spec.DefaultExpanded,
		LocalOnly:       parent.LocalOnly || rule.LocalOnly || rule.LiveHTTP.EffectiveAuth() != "none",
		UpdatedAt:       time.Now().UTC(),
	}
	return child, true
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
