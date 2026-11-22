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

// PatternMatchResult bundles the synthesized decoded card and any linked local
// cards discovered via LinkEntityKeys for runner-up enrichment.
type PatternMatchResult struct {
	Card          models.SearchCard
	EnrichedCards []models.SearchCard
}

type compiledPattern struct {
	rule models.PatternRule
	re   *regexp.Regexp
}

// PatternEngine evaluates declarative PatternRules against user queries,
// validates standard checksums (IATA Mod-7, ISO 6346), enriches results from
// the local SQLite Store via EntityKey, and optionally queries connected MCP servers.
type PatternEngine struct {
	mu       sync.RWMutex
	compiled []compiledPattern
	allRules []models.PatternRule
}

// NewPatternEngine constructs a PatternEngine pre-loaded with built-in generic standards.
func NewPatternEngine(customRules []models.PatternRule) *PatternEngine {
	e := &PatternEngine{}
	e.SetCustomRules(customRules)
	return e
}

// SetCustomRules replaces custom user/module rules (evaluated ahead of built-in standards).
func (e *PatternEngine) SetCustomRules(customRules []models.PatternRule) {
	seen := make(map[string]bool)
	merged := make([]models.PatternRule, 0, len(customRules)+4)

	for _, r := range customRules {
		id := strings.TrimSpace(r.ID)
		if id == "" || strings.TrimSpace(r.Regex) == "" || seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, r)
	}
	for _, r := range BuiltInPatternRules() {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		merged = append(merged, r)
	}

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
		compiled = append(compiled, compiledPattern{rule: r, re: re})
		validRules = append(validRules, r)
	}

	e.mu.Lock()
	e.compiled = compiled
	e.allRules = validRules
	e.mu.Unlock()
}

// Rules returns a copy of all active compiled PatternRules (custom + built-in).
func (e *PatternEngine) Rules() []models.PatternRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]models.PatternRule, len(e.allRules))
	copy(out, e.allRules)
	return out
}

// BuiltInPatternRules returns Vex's generic, public-standard pattern rules.
// Company-specific rules belong in ~/.vex/patterns/*.json or ModuleManifest.Patterns.
func BuiltInPatternRules() []models.PatternRule {
	return []models.PatternRule{
		{
			ID:          "iata-awb",
			Name:        "IATA Air Waybill (AWB)",
			Category:    "logistics",
			Description: "Decodes 11-digit IATA Master Air Waybill numbers with airline prefix lookup and Mod-7 check-digit validation.",
			Regex:       `^(?P<prefix>\d{3})[-\s]?(?P<serial>\d{7})(?P<check>\d)$`,
			Checksum:    "iata-mod7",
			Lookups: map[string]map[string]string{
				"prefix": {
					"001": "American Airlines Cargo (AA)",
					"006": "Delta Cargo (DL)",
					"016": "United Cargo (UA)",
					"020": "Lufthansa Cargo (LH)",
					"023": "FedEx Express (FX)",
					"057": "Air France Cargo (AF)",
					"074": "KLM Cargo (KL)",
					"125": "IAG Cargo / British Airways (BA)",
					"157": "Qatar Airways Cargo (QR)",
					"160": "Cathay Cargo (CX)",
					"176": "Emirates SkyCargo (EK)",
					"180": "Korean Air Cargo (KE)",
					"205": "ANA Cargo (NH)",
					"235": "Turkish Cargo (TK)",
					"406": "UPS Air Cargo (5X)",
					"618": "Singapore Airlines Cargo (SQ)",
				},
			},
			TitleTemplate:       "AWB {{prefix}}-{{serial}}{{check}}",
			SubtitleTemplate:    "{{lookup.prefix}} · Check Digit {{checksum.status}}",
			DescriptionTemplate: "Decoded 11-digit IATA Air Waybill (Prefix {{prefix}}, Serial {{serial}}, Check Digit {{check}} — {{checksum.status}}).",
			Collection:          "air-freight",
			Template:            models.TemplateKeyValue,
			Icon:                "plane",
			Fields: map[string]string{
				"reference_type": "IATA Air Waybill (MAWB)",
				"airline_prefix": "{{prefix}}",
				"carrier":        "{{lookup.prefix}}",
				"serial_number":  "{{serial}}",
				"check_digit":    "{{check}} ({{checksum.status}})",
			},
			GlancePills: []string{"AWB", "{{lookup.prefix}}", "Mod-7: {{checksum.status}}"},
			Actions: []models.CardAction{
				{ID: "copy-awb", Label: "Copy AWB Number", Shortcut: "Enter", Type: "copy", Target: "{{prefix}}-{{serial}}{{check}}"},
			},
			Provenance: &models.Provenance{
				SourceName: "IATA Resolution 600a (AWB Standard)",
				Authority:  "official",
			},
		},
		{
			ID:              "iso-container",
			Name:            "ISO 6346 Shipping Container",
			Category:        "logistics",
			Description:     "Decodes 11-character ISO 6346 intermodal container numbers with BIC owner prefix and weighted Mod-11 check-digit validation.",
			Regex:           `^(?P<bic>(?P<owner>[A-Z]{3})(?P<category>[UJZ]))(?P<serial>\d{6})(?P<check>\d)$`,
			CaseInsensitive: true,
			Checksum:        "iso6346",
			Lookups: map[string]map[string]string{
				"bic": {
					"MSCU": "MSC Mediterranean Shipping",
					"MEDU": "MSC Mediterranean Shipping",
					"MAEU": "Maersk Line",
					"MSKU": "Maersk Line",
					"MRKU": "Maersk Line",
					"CMAU": "CMA CGM",
					"HLXU": "Hapag-Lloyd",
					"HLCU": "Hapag-Lloyd",
					"OOLU": "OOCL",
					"COSU": "COSCO Shipping",
					"EGLV": "Evergreen Marine",
					"EMCU": "Evergreen Marine",
					"ONEU": "Ocean Network Express (ONE)",
					"YMLU": "Yang Ming Marine",
					"ZIMU": "ZIM Integrated Shipping",
				},
				"category": {
					"U": "Freight Container",
					"J": "Detachable Freight Equipment",
					"Z": "Trailer / Chassis",
				},
			},
			TitleTemplate:       "Container {{bic}}{{serial}}{{check}}",
			SubtitleTemplate:    "{{lookup.bic}} · {{lookup.category}} · Check Digit {{checksum.status}}",
			DescriptionTemplate: "Decoded ISO 6346 intermodal container identifier (Owner {{owner}}, Category {{category}}, Check Digit {{check}} — {{checksum.status}}).",
			Collection:          "ocean-freight",
			Template:            models.TemplateKeyValue,
			Icon:                "box",
			Fields: map[string]string{
				"reference_type": "ISO 6346 Container",
				"owner_code":     "{{bic}}",
				"carrier_owner":  "{{lookup.bic}}",
				"equipment_type": "{{lookup.category}}",
				"serial_number":  "{{serial}}",
				"check_digit":    "{{check}} ({{checksum.status}})",
			},
			GlancePills: []string{"ISO 6346", "{{lookup.bic}}", "Check: {{checksum.status}}"},
			Actions: []models.CardAction{
				{ID: "copy-container", Label: "Copy Container ID", Shortcut: "Enter", Type: "copy", Target: "{{bic}}{{serial}}{{check}}"},
			},
			Provenance: &models.Provenance{
				SourceName: "ISO 6346 / BIC Container Standard",
				Authority:  "official",
			},
		},
		{
			ID:                  "ado-work-item",
			Name:                "Azure DevOps Work Item",
			Category:            "work-items",
			Description:         "Recognizes Azure DevOps Work Item IDs (AB#12345 or ADO-12345) and queries the connected azure-devops MCP server when available.",
			Regex:               `^(?:AB#|ADO[-#])(?P<id>\d{3,9})$`,
			CaseInsensitive:     true,
			TitleTemplate:       "ADO Work Item #{{id}}",
			SubtitleTemplate:    "Azure DevOps · {{match}}",
			DescriptionTemplate: "Matched Azure DevOps Work Item #{{id}}.",
			Collection:          "work-items",
			Template:            models.TemplateKeyValue,
			Icon:                "git-pull-request",
			Fields: map[string]string{
				"work_item_id": "{{id}}",
				"source":       "Azure DevOps (MCP)",
			},
			GlancePills: []string{"ADO", "#{{id}}"},
			Actions: []models.CardAction{
				{ID: "copy-ado-id", Label: "Copy Work Item ID", Shortcut: "Enter", Type: "copy", Target: "{{id}}"},
			},
			LiveMCP: &models.PatternMCPLookup{
				ServerID: "azure-devops",
				ToolName: "wit_get_work_item",
				Arguments: map[string]string{
					"id": "{{id}}",
				},
			},
			LocalOnly: true,
			Provenance: &models.Provenance{
				SourceName: "Azure DevOps MCP Bridge",
				Authority:  "automated-sync",
				LocalOnly:  true,
			},
		},
		{
			ID:                  "github-issue",
			Name:                "GitHub Issue / Pull Request",
			Category:            "engineering",
			Description:         "Recognizes owner/repo#123 references and queries the connected github MCP server when available.",
			Regex:               `^(?P<owner>[A-Za-z0-9_.-]+)/(?P<repo>[A-Za-z0-9_.-]+)#(?P<number>\d+)$`,
			TitleTemplate:       "GitHub {{owner}}/{{repo}}#{{number}}",
			SubtitleTemplate:    "GitHub Issue / PR Reference",
			DescriptionTemplate: "Matched GitHub reference {{owner}}/{{repo}}#{{number}}.",
			Collection:          "github-catalog",
			Template:            models.TemplateKeyValue,
			Icon:                "git-branch",
			Fields: map[string]string{
				"repository": "{{owner}}/{{repo}}",
				"number":     "#{{number}}",
				"source":     "GitHub (MCP)",
			},
			GlancePills: []string{"GitHub", "#{{number}}"},
			Actions: []models.CardAction{
				{ID: "copy-gh-ref", Label: "Copy Reference", Shortcut: "Enter", Type: "copy", Target: "{{owner}}/{{repo}}#{{number}}"},
			},
			LiveMCP: &models.PatternMCPLookup{
				ServerID: "github",
				ToolName: "get_issue",
				Arguments: map[string]string{
					"owner":        "{{owner}}",
					"repo":         "{{repo}}",
					"issue_number": "{{number}}",
				},
			},
			LocalOnly: true,
			Provenance: &models.Provenance{
				SourceName: "GitHub MCP Bridge",
				Authority:  "automated-sync",
				LocalOnly:  true,
			},
		},
	}
}

// Match evaluates rawText against compiled PatternRules, returning the first match
// enriched with local EntityKey cards and optional live MCP data.
func (e *PatternEngine) Match(ctx context.Context, rawText string, store *indexer.Store, liveExec LivePatternExecutor) *PatternMatchResult {
	input := strings.TrimSpace(rawText)
	if input == "" {
		return nil
	}

	e.mu.RLock()
	compiled := e.compiled
	e.mu.RUnlock()

	for _, cp := range compiled {
		sub := cp.re.FindStringSubmatch(input)
		if sub == nil {
			continue
		}

		captures := make(map[string]string, len(sub))
		captures["match"] = input
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
			} else {
				captures["checksum.status"] = "Invalid"
			}
			if detail != "" {
				captures["checksum.detail"] = detail
			}
		}

		card := buildPatternCard(cp.rule, captures)

		// Optional live MCP lookup (tight timeout so disconnected/slow servers never stall search)
		if cp.rule.LiveMCP != nil {
			enrichCardFromLiveMCP(ctx, &card, cp.rule, captures, liveExec)
		}

		// Optional cross-card EntityKey enrichment (e.g. "loc:SEA" -> Seattle gateway card)
		var enriched []models.SearchCard
		if store != nil && len(cp.rule.LinkEntityKeys) > 0 {
			resolvedKeys := make([]string, 0, len(cp.rule.LinkEntityKeys))
			for _, keyTmpl := range cp.rule.LinkEntityKeys {
				k := strings.TrimSpace(interpolatePattern(keyTmpl, cp.rule, captures))
				if k != "" {
					resolvedKeys = append(resolvedKeys, k)
				}
			}
			if len(resolvedKeys) > 0 {
				if linked, err := store.FindCardsByEntityKeys(ctx, resolvedKeys); err == nil && len(linked) > 0 {
					enriched = linked
					for _, lc := range linked {
						if lc.EntityKey != "" {
							card.RelatedKeys = appendUniquePatternStr(card.RelatedKeys, lc.EntityKey)
						}
						secFields := make(map[string]any)
						for _, fieldKey := range []string{"unlocode", "iata", "country", "currency", "timezone", "region"} {
							if v, ok := lc.Data[fieldKey]; ok && v != nil && fmt.Sprintf("%v", v) != "" {
								secFields[fieldKey] = v
							}
						}
						card.Sections = append(card.Sections, models.CardSection{
							ModuleID:   "linked-" + lc.ID,
							ModuleName: lc.Title,
							Title:      lc.Title,
							Badge:      firstNonEmptyPattern(lc.EntityKey, lc.Collection),
							Notes:      firstNonEmptyPattern(lc.Subtitle, lc.Description),
							Fields:     secFields,
							Actions:    lc.Actions,
							Provenance: lc.Provenance,
							LocalOnly:  lc.LocalOnly,
						})
					}
				}
			}
		}

		card.UpdatedAt = time.Now().UTC()
		return &PatternMatchResult{
			Card:          card,
			EnrichedCards: enriched,
		}
	}

	return nil
}

func buildPatternCard(rule models.PatternRule, captures map[string]string) models.SearchCard {
	col := strings.TrimSpace(rule.Collection)
	if col == "" {
		col = "patterns"
	}
	tpl := rule.Template
	if tpl == "" {
		tpl = models.TemplateKeyValue
	}
	icon := rule.Icon
	if icon == "" {
		icon = "hash"
	}

	data := make(map[string]any, len(rule.Fields))
	for k, tmpl := range rule.Fields {
		v := strings.TrimSpace(interpolatePattern(tmpl, rule, captures))
		if v != "" {
			data[k] = v
		}
	}

	pills := make([]string, 0, len(rule.GlancePills))
	for _, tmpl := range rule.GlancePills {
		v := strings.TrimSpace(interpolatePattern(tmpl, rule, captures))
		if v != "" {
			pills = append(pills, v)
		}
		if len(pills) == 3 {
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
		ID:          "builtin-pattern-" + models.Slugify(rule.ID),
		Title:       strings.TrimSpace(interpolatePattern(rule.TitleTemplate, rule, captures)),
		Subtitle:    strings.TrimSpace(interpolatePattern(rule.SubtitleTemplate, rule, captures)),
		Description: strings.TrimSpace(interpolatePattern(rule.DescriptionTemplate, rule, captures)),
		Collection:  col,
		Template:    tpl,
		Icon:        icon,
		Tags:        []string{"pattern", rule.ID},
		GlancePills: pills,
		Data:        data,
		Actions:     actions,
		Provenance:  prov,
		LocalOnly:   rule.LocalOnly,
	}
}

func enrichCardFromLiveMCP(ctx context.Context, card *models.SearchCard, rule models.PatternRule, captures map[string]string, liveExec LivePatternExecutor) {
	if liveExec == nil {
		card.Data["live_status"] = fmt.Sprintf("Connect %q MCP server in Settings to fetch live details", rule.LiveMCP.ServerID)
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
		card.Data["live_status"] = fmt.Sprintf("MCP %q offline (%s)", rule.LiveMCP.ServerID, err.Error())
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

	// If the MCP tool returned structured JSON, unpack scalar fields into the template card.
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err == nil && len(obj) > 0 {
		if title, ok := obj["title"].(string); ok && strings.TrimSpace(title) != "" {
			card.Subtitle = strings.TrimSpace(title)
		}
		if desc, ok := obj["description"].(string); ok && strings.TrimSpace(desc) != "" {
			card.Description = strings.TrimSpace(desc)
		}
		if state, ok := obj["state"].(string); ok && strings.TrimSpace(state) != "" {
			if len(card.GlancePills) < 3 {
				card.GlancePills = append(card.GlancePills, strings.TrimSpace(state))
			}
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
		for _, k := range keys {
			v := obj[k]
			switch tv := v.(type) {
			case string:
				if strings.TrimSpace(tv) != "" && k != "description" {
					card.Data[k] = strings.TrimSpace(tv)
				}
			case float64, bool:
				card.Data[k] = tv
			}
		}
		return
	}

	card.Description = text
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
	// Resolve raw value by exact capture name first, or by prefix before "_"
	// (e.g. lookup "branch_entity" reads capture "branch").
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
	}
	return true, ""
}

// computeISO6346CheckDigit implements the official ISO 6346 Annex A check-digit algorithm.
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
	// ISO 6346 assigns A=10..Z=38, skipping multiples of 11 (11, 22, 33).
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
