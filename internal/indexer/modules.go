package indexer

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vex/internal/models"
)

const CompiledModulesSource = "module://compiled"

//go:embed starter_modules/*.json
var embeddedStarterModules embed.FS

// StarterModules returns the optional packaged module catalog embedded with Vex.
// These modules are NEVER forced into the active index — users choose which ones to
// install/load during onboarding or from the Module Catalog, and any installed module
// can be edited, toggled off, or deleted directly from ~/.vex/modules/.
func StarterModules() []models.ModuleManifest {
	entries, err := fs.ReadDir(embeddedStarterModules, "starter_modules")
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	out := make([]models.ModuleManifest, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		raw, readErr := embeddedStarterModules.ReadFile("starter_modules/" + e.Name())
		if readErr != nil {
			continue
		}
		var mod models.ModuleManifest
		if unmarshalErr := json.Unmarshal(raw, &mod); unmarshalErr != nil {
			continue
		}
		if strings.TrimSpace(mod.ID) == "" || strings.TrimSpace(mod.Name) == "" {
			continue
		}
		out = append(out, mod)
	}
	return out
}

// ListStarterModules returns metadata for every packaged optional module along with
// whether it is currently installed as a JSON file in modulesDir.
func ListStarterModules(modulesDir string) []models.StarterModuleInfo {
	starters := StarterModules()
	installed, _, _ := LoadAllModules(modulesDir)
	installedSet := make(map[string]bool, len(installed))
	for _, m := range installed {
		installedSet[m.ID] = true
	}

	out := make([]models.StarterModuleInfo, 0, len(starters))
	for _, m := range starters {
		out = append(out, models.StarterModuleInfo{
			ID:             m.ID,
			Name:           m.Name,
			Category:       m.Category,
			Description:    m.Description,
			Icon:           m.Icon,
			Version:        m.Version,
			DefaultEnabled: m.DefaultEnabled,
			Installed:      installedSet[m.ID],
			CardCount:      len(m.Cards),
			OverlayCount:   len(m.Overlays),
			PatternCount:   len(m.Patterns),
		})
	}
	return out
}

// InstallStarterModule copies a packaged optional starter module into modulesDir/<id>.json
// so the user owns the file and can customize, toggle, or delete it at any time.
func InstallStarterModule(modulesDir, starterID string) (models.ModuleManifest, error) {
	id := models.Slugify(starterID)
	for _, m := range StarterModules() {
		if m.ID == id {
			return SaveCustomModule(modulesDir, m)
		}
	}
	return models.ModuleManifest{}, fmt.Errorf("starter module %q not found", starterID)
}

// InstallStarterModules installs a batch of starter module IDs into modulesDir.
// Passing an empty slice installs nothing (user chose a clean slate).
func InstallStarterModules(modulesDir string, starterIDs []string) ([]models.ModuleManifest, error) {
	installed := make([]models.ModuleManifest, 0, len(starterIDs))
	for _, id := range starterIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		mod, err := InstallStarterModule(modulesDir, id)
		if err != nil {
			return installed, err
		}
		installed = append(installed, mod)
	}
	return installed, nil
}

// LoadAllModules loads all installed optional module JSON files from modulesDir.
// Modules deleted from modulesDir are NOT resurrected from code.
func LoadAllModules(modulesDir string) ([]models.ModuleManifest, map[string]string, error) {
	if strings.TrimSpace(modulesDir) == "" {
		return nil, map[string]string{}, nil
	}
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, map[string]string{}, nil
		}
		return nil, nil, err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	byID := make(map[string]models.ModuleManifest)
	filePaths := make(map[string]string)
	order := make([]string, 0, len(entries))

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		fullPath := filepath.Join(modulesDir, e.Name())
		raw, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			continue
		}
		var mod models.ModuleManifest
		if err := json.Unmarshal(raw, &mod); err != nil {
			continue
		}
		if strings.TrimSpace(mod.ID) == "" || strings.TrimSpace(mod.Name) == "" {
			continue
		}
		mod.ID = models.Slugify(mod.ID)
		if _, exists := byID[mod.ID]; !exists {
			order = append(order, mod.ID)
		}
		byID[mod.ID] = mod
		filePaths[mod.ID] = fullPath
	}

	out := make([]models.ModuleManifest, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, filePaths, nil
}

// SaveCustomModule validates and persists a ModuleManifest JSON file to modulesDir/<id>.json.
func SaveCustomModule(modulesDir string, mod models.ModuleManifest) (models.ModuleManifest, error) {
	mod.Name = strings.TrimSpace(mod.Name)
	if mod.Name == "" {
		return mod, errors.New("module name is required")
	}
	mod.ID = models.Slugify(mod.ID)
	if mod.ID == "" || mod.ID == "item" {
		mod.ID = models.Slugify(mod.Name)
	}
	if mod.Category == "" {
		mod.Category = "Custom"
	}
	if mod.Version == "" {
		mod.Version = "1.0.0"
	}
	for i := range mod.Cards {
		mod.Cards[i].ModuleID = mod.ID
		if err := models.ValidateAndNormalizeCard(&mod.Cards[i], mod.ID); err != nil {
			return mod, fmt.Errorf("card %d: %w", i, err)
		}
	}
	if err := os.MkdirAll(modulesDir, 0o755); err != nil {
		return mod, err
	}
	raw, err := json.MarshalIndent(mod, "", "  ")
	if err != nil {
		return mod, err
	}
	raw = append(raw, '\n')
	dest := filepath.Join(modulesDir, mod.ID+".json")
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return mod, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return mod, err
	}
	return mod, nil
}

// DeleteModule permanently removes a module JSON file from modulesDir.
func DeleteModule(modulesDir, moduleID string) error {
	if strings.TrimSpace(modulesDir) == "" {
		return errors.New("modules directory is not configured")
	}
	id := models.Slugify(moduleID)
	all, paths, err := LoadAllModules(modulesDir)
	if err != nil {
		return err
	}
	for _, m := range all {
		if m.ID == id {
			if p, ok := paths[id]; ok && p != "" {
				return os.Remove(p)
			}
		}
	}
	// Fallback to direct filename if present
	direct := filepath.Join(modulesDir, id+".json")
	if err := os.Remove(direct); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CompileActiveModules merges all enabled modules' standalone cards, overlays, and nested child cards.
func CompileActiveModules(all []models.ModuleManifest, enabledIDs []string) []models.SearchCard {
	enabledSet := make(map[string]bool)
	if enabledIDs == nil {
		for _, m := range all {
			if m.DefaultEnabled {
				enabledSet[m.ID] = true
			}
		}
	} else {
		for _, id := range enabledIDs {
			enabledSet[strings.TrimSpace(id)] = true
		}
	}

	cards := make([]models.SearchCard, 0, 64)
	byID := make(map[string]int)
	byEntity := make(map[string]int)

	// Pass 1: Collect standalone base cards from enabled modules
	for _, mod := range all {
		if !enabledSet[mod.ID] {
			continue
		}
		modProv := defaultModuleProvenance(mod)
		for _, c := range mod.Cards {
			cCopy := cloneCard(c)
			cCopy.ModuleID = mod.ID
			if mod.LocalOnly {
				cCopy.LocalOnly = true
			}
			if cCopy.Provenance == nil {
				cCopy.Provenance = modProv
			} else if mod.LocalOnly && !cCopy.Provenance.LocalOnly {
				provCopy := *cCopy.Provenance
				provCopy.LocalOnly = true
				cCopy.Provenance = &provCopy
			}
			if err := models.ValidateAndNormalizeCard(&cCopy, mod.ID); err != nil {
				continue
			}
			idx := len(cards)
			cards = append(cards, cCopy)
			byID[cCopy.ID] = idx
			if cCopy.EntityKey != "" {
				byEntity[strings.ToLower(cCopy.EntityKey)] = idx
			}
		}
	}

	// Pass 2: Apply overlays from enabled modules (enriching existing cards with sections & child cards, or promoting standalone)
	for _, mod := range all {
		if !enabledSet[mod.ID] {
			continue
		}
		modProv := defaultModuleProvenance(mod)
		for _, ov := range mod.Overlays {
			sec := ov.Section
			hasSection := strings.TrimSpace(sec.Title) != "" || strings.TrimSpace(sec.Notes) != "" || len(sec.Fields) > 0 || len(sec.Actions) > 0
			if hasSection {
				sec.ModuleID = mod.ID
				sec.ModuleName = mod.Name
				if mod.LocalOnly {
					sec.LocalOnly = true
				}
				if sec.Badge == "" {
					sec.Badge = mod.Name
				}
				if sec.Provenance == nil {
					sec.Provenance = modProv
				} else if mod.LocalOnly && !sec.Provenance.LocalOnly {
					provCopy := *sec.Provenance
					provCopy.LocalOnly = true
					sec.Provenance = &provCopy
				}
			}

			targetIdx := -1
			if ov.TargetEntityKey != "" {
				if idx, ok := byEntity[strings.ToLower(ov.TargetEntityKey)]; ok {
					targetIdx = idx
				}
			}
			if targetIdx < 0 && ov.TargetCardID != "" {
				if idx, ok := byID[ov.TargetCardID]; ok {
					targetIdx = idx
				}
			}

			if targetIdx >= 0 {
				dst := &cards[targetIdx]
				if hasSection {
					dst.Sections = append(dst.Sections, sec)
				}
				for _, child := range ov.ChildCards {
					cCopy := cloneCard(child)
					cCopy.ModuleID = mod.ID
					if err := models.ValidateAndNormalizeCard(&cCopy, dst.Collection); err == nil {
						dst.Children = append(dst.Children, cCopy)
					}
				}
				dst.Tags = appendUniqueStrings(dst.Tags, ov.AddTags...)
				dst.Modifiers = appendUniqueStrings(dst.Modifiers, ov.AddModifiers...)
				dst.RelatedKeys = appendUniqueStrings(dst.RelatedKeys, ov.AddRelatedKeys...)
			} else if hasSection || len(ov.ChildCards) > 0 {
				fallbackCol := ov.FallbackCol
				if fallbackCol == "" {
					fallbackCol = mod.ID
				}
				fallbackTitle := ov.FallbackTitle
				if fallbackTitle == "" {
					fallbackTitle = fmt.Sprintf("%s — %s", ov.TargetEntityKey, sec.Title)
				}
				synthID := ov.TargetCardID
				if synthID == "" {
					synthID = models.Slugify(mod.ID + "-" + ov.TargetEntityKey)
				}
				var secs []models.CardSection
				if hasSection {
					secs = []models.CardSection{sec}
				}
				synth := models.SearchCard{
					ID:          synthID,
					EntityKey:   ov.TargetEntityKey,
					ModuleID:    mod.ID,
					Title:       fallbackTitle,
					Subtitle:    sec.Title + " • " + mod.Name,
					Description: sec.Notes,
					Collection:  fallbackCol,
					Template:    models.TemplateKeyValue,
					Icon:        mod.Icon,
					Tags:        appendUniqueStrings(nil, ov.AddTags...),
					Modifiers:   appendUniqueStrings(nil, ov.AddModifiers...),
					RelatedKeys: appendUniqueStrings(nil, ov.AddRelatedKeys...),
					Sections:    secs,
					Children:    ov.ChildCards,
					Actions:     sec.Actions,
					Provenance:  sec.Provenance,
					LocalOnly:   mod.LocalOnly || sec.LocalOnly,
				}
				if err := models.ValidateAndNormalizeCard(&synth, fallbackCol); err == nil {
					idx := len(cards)
					cards = append(cards, synth)
					byID[synth.ID] = idx
					if synth.EntityKey != "" {
						byEntity[strings.ToLower(synth.EntityKey)] = idx
					}
				}
			}
		}
	}

	return cards
}

// SyncModules compiles enabled modules from modulesDir and atomically updates the SQLite store.
func SyncModules(ctx context.Context, store *Store, modulesDir string, enabledIDs []string) ([]models.ModuleStatus, error) {
	all, filePaths, err := LoadAllModules(modulesDir)
	if err != nil {
		return nil, err
	}
	compiled := CompileActiveModules(all, enabledIDs)
	if err := store.ReplaceSourceFile(ctx, CompiledModulesSource, compiled); err != nil {
		return nil, err
	}

	enabledSet := make(map[string]bool)
	if enabledIDs == nil {
		for _, m := range all {
			if m.DefaultEnabled {
				enabledSet[m.ID] = true
			}
		}
	} else {
		for _, id := range enabledIDs {
			enabledSet[strings.TrimSpace(id)] = true
		}
	}

	statuses := make([]models.ModuleStatus, 0, len(all))
	for _, m := range all {
		statuses = append(statuses, models.ModuleStatus{
			ID:             m.ID,
			Name:           m.Name,
			Category:       m.Category,
			Description:    m.Description,
			Icon:           m.Icon,
			Version:        m.Version,
			Enabled:        enabledSet[m.ID],
			DefaultEnabled: m.DefaultEnabled,
			LocalOnly:      m.LocalOnly,
			CardCount:      len(m.Cards),
			OverlayCount:   len(m.Overlays),
			PatternCount:   len(m.Patterns),
			FilePath:       filePaths[m.ID],
		})
	}
	return statuses, nil
}

func defaultModuleProvenance(mod models.ModuleManifest) *models.Provenance {
	return &models.Provenance{
		SourceName: mod.Name,
		Authority:  "internal-curated",
		LocalOnly:  mod.LocalOnly,
	}
}

func cloneCard(c models.SearchCard) models.SearchCard {
	out := c
	if len(c.Tags) > 0 {
		out.Tags = append([]string(nil), c.Tags...)
	}
	if len(c.Modifiers) > 0 {
		out.Modifiers = append([]string(nil), c.Modifiers...)
	}
	if len(c.GlancePills) > 0 {
		out.GlancePills = append([]string(nil), c.GlancePills...)
	}
	if len(c.Chips) > 0 {
		out.Chips = append([]models.GlanceChip(nil), c.Chips...)
	}
	if len(c.RelatedKeys) > 0 {
		out.RelatedKeys = append([]string(nil), c.RelatedKeys...)
	}
	if len(c.Actions) > 0 {
		out.Actions = append([]models.CardAction(nil), c.Actions...)
	}
	if len(c.Sections) > 0 {
		out.Sections = append([]models.CardSection(nil), c.Sections...)
	}
	if len(c.Children) > 0 {
		out.Children = make([]models.SearchCard, len(c.Children))
		for i, ch := range c.Children {
			out.Children[i] = cloneCard(ch)
		}
	}
	if len(c.Data) > 0 {
		out.Data = make(map[string]any, len(c.Data))
		for k, v := range c.Data {
			out.Data[k] = v
		}
	}
	return out
}

func appendUniqueStrings(dst []string, items ...string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, s := range dst {
		seen[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}
	for _, item := range items {
		clean := strings.TrimSpace(item)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			dst = append(dst, clean)
		}
	}
	sort.Strings(dst)
	return dst
}
