package indexer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"vex/internal/models"
)

type patternFileEnvelope struct {
	Patterns []models.PatternRule `json:"patterns"`
}

// LoadPatternRulesDir loads declarative PatternRule definitions from *.json files in patternsDir.
// Each JSON file may contain a single PatternRule object, a JSON array of PatternRules,
// or an envelope object {"patterns": [...]}.
func LoadPatternRulesDir(patternsDir string) ([]models.PatternRule, error) {
	if strings.TrimSpace(patternsDir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(patternsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []models.PatternRule
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(patternsDir, e.Name()))
		if readErr != nil {
			continue
		}
		parsed := parsePatternRuleBytes(raw)
		out = append(out, parsed...)
	}
	return out, nil
}

// LoadActivePatternRules merges custom rules from patternsDir with any PatternRules
// bundled inside currently enabled modules.
func LoadActivePatternRules(patternsDir, modulesDir string, enabledModuleIDs []string) []models.PatternRule {
	out, _ := LoadPatternRulesDir(patternsDir)

	allMods, _, err := LoadAllModules(modulesDir)
	if err != nil {
		return out
	}
	enabledSet := make(map[string]bool)
	if enabledModuleIDs == nil {
		for _, m := range allMods {
			if m.DefaultEnabled {
				enabledSet[m.ID] = true
			}
		}
	} else {
		for _, id := range enabledModuleIDs {
			enabledSet[strings.TrimSpace(id)] = true
		}
	}

	for _, mod := range allMods {
		if !enabledSet[mod.ID] || len(mod.Patterns) == 0 {
			continue
		}
		for _, p := range mod.Patterns {
			if mod.LocalOnly {
				p.LocalOnly = true
			}
			out = append(out, p)
		}
	}
	return out
}

func parsePatternRuleBytes(raw []byte) []models.PatternRule {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	// 1. JSON array of rules
	if strings.HasPrefix(trimmed, "[") {
		var list []models.PatternRule
		if err := json.Unmarshal([]byte(trimmed), &list); err == nil {
			return filterValidPatternRules(list)
		}
		return nil
	}
	// 2. Envelope {"patterns": [...]}
	var env patternFileEnvelope
	if err := json.Unmarshal([]byte(trimmed), &env); err == nil && len(env.Patterns) > 0 {
		return filterValidPatternRules(env.Patterns)
	}
	// 3. Single PatternRule object
	var single models.PatternRule
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil && single.Regex != "" {
		return filterValidPatternRules([]models.PatternRule{single})
	}
	return nil
}

func filterValidPatternRules(in []models.PatternRule) []models.PatternRule {
	out := make([]models.PatternRule, 0, len(in))
	for _, r := range in {
		if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Regex) == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// SavePatternRule validates a PatternRule (including Go RE2 regex compilation)
// and atomically writes it to <patternsDir>/<slug-id>.json with 0o600 permissions.
func SavePatternRule(patternsDir string, rule models.PatternRule) (string, error) {
	if strings.TrimSpace(patternsDir) == "" {
		return "", errors.New("patterns directory is not configured")
	}
	rule.ID = models.Slugify(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Regex = strings.TrimSpace(rule.Regex)
	rule.TitleTemplate = strings.TrimSpace(rule.TitleTemplate)

	if rule.ID == "" || rule.ID == "item" {
		return "", errors.New("pattern rule id is required")
	}
	if rule.Name == "" {
		rule.Name = rule.ID
	}
	if rule.Regex == "" {
		return "", errors.New("pattern rule regex is required")
	}
	if rule.TitleTemplate == "" {
		return "", errors.New("pattern rule titleTemplate is required")
	}

	expr := rule.Regex
	if rule.CaseInsensitive && !strings.HasPrefix(expr, "(?i)") {
		expr = "(?i)" + expr
	}
	if _, err := regexp.Compile(expr); err != nil {
		return "", fmt.Errorf("invalid pattern regex %q: %w", rule.Regex, err)
	}

	if err := os.MkdirAll(patternsDir, 0o755); err != nil {
		return "", fmt.Errorf("create patterns dir: %w", err)
	}
	raw, err := json.MarshalIndent(rule, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')

	target := filepath.Join(patternsDir, rule.ID+".json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", err
	}
	return target, nil
}

// DeletePatternRule removes a standalone user pattern rule file from patternsDir.
func DeletePatternRule(patternsDir, ruleID string) error {
	if strings.TrimSpace(patternsDir) == "" {
		return errors.New("patterns directory is not configured")
	}
	id := models.Slugify(ruleID)
	target := filepath.Join(patternsDir, id+".json")
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
