package indexer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vex/internal/models"
)

func TestParseRepoTaxonomy(t *testing.T) {
	tax := ParseRepoTaxonomy("gt-customs-brokerage-declaration-api")
	if !tax.Standard || tax.Prefix != "gt" || tax.Portfolio != "Customs" || tax.Team != "Brokerage" || tax.Product != "Declaration" || tax.Component != "api" {
		t.Fatalf("unexpected taxonomy: %+v", tax)
	}

	nonStd := ParseRepoTaxonomy("legacy-tool")
	if nonStd.Standard || nonStd.Portfolio != "General" {
		t.Fatalf("expected non-standard fallback taxonomy, got %+v", nonStd)
	}
}

func TestStarterModulesStrictValidationInstallAndDynamicNesting(t *testing.T) {
	modulesDir := t.TempDir()

	// 1. Fresh user modulesDir has 0 installed modules (nothing forced from Go code!)
	initial, _, err := LoadAllModules(modulesDir)
	if err != nil {
		t.Fatalf("LoadAllModules: %v", err)
	}
	if len(initial) != 0 {
		t.Fatalf("expected 0 modules in clean modulesDir, got %d", len(initial))
	}

	// 2. Starter catalog contains exactly 5 real optional modules (team-templates removed)
	// and every embedded JSON file passes strict ValidateModuleJSON with 0 errors.
	entries, err := fs.ReadDir(embeddedStarterModules, "starter_modules")
	if err != nil {
		t.Fatalf("ReadDir starter_modules: %v", err)
	}
	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			if e.Name() == "team-templates.json" {
				t.Fatalf("team-templates.json must be removed from starter_modules")
			}
			jsonFiles = append(jsonFiles, e.Name())
			raw, readErr := embeddedStarterModules.ReadFile("starter_modules/" + e.Name())
			if readErr != nil {
				t.Fatalf("ReadFile(%s): %v", e.Name(), readErr)
			}
			mod, res := models.ValidateModuleJSON(raw)
			if !res.Valid || mod == nil {
				t.Fatalf("starter module %s failed strict ValidateModuleJSON: %v", e.Name(), res.Err())
			}
		}
	}
	if len(jsonFiles) != 5 {
		t.Fatalf("expected 5 starter module JSON files, got %d (%v)", len(jsonFiles), jsonFiles)
	}

	// 3. Install all 5 starter modules -> written as real editable JSON files in modulesDir
	installed, err := InstallStarterModules(modulesDir, []string{
		"air-carriers",
		"ocean-carriers",
		"containers",
		"logistics-dictionary",
		"reference-patterns",
	})
	if err != nil {
		t.Fatalf("InstallStarterModules: %v", err)
	}
	if len(installed) != 5 {
		t.Fatalf("expected 5 installed modules, got %d", len(installed))
	}
	if _, err := os.Stat(filepath.Join(modulesDir, "air-carriers.json")); err != nil {
		t.Fatalf("expected air-carriers.json on disk: %v", err)
	}

	// 4. Sync into SQLite v3 and verify flat-on-disk children hydrate dynamically via entity_links
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	statuses, err := SyncModules(ctx, store, modulesDir, nil)
	if err != nil {
		t.Fatalf("SyncModules: %v", err)
	}
	if len(statuses) != 5 {
		t.Fatalf("expected 5 module statuses, got %d", len(statuses))
	}

	hc40, err := store.GetCard(ctx, "container-40hc")
	if err != nil || hc40 == nil || hc40.Layout != models.LayoutGroup || len(hc40.Children) != 2 {
		t.Fatalf("expected container-40hc (group) with 2 dynamically hydrated children, got %+v (err=%v)", hc40, err)
	}
	ek176, err := store.GetCard(ctx, "air-carrier-176")
	if err != nil || ek176 == nil || ek176.Layout != models.LayoutFacts || len(ek176.Children) != 1 || ek176.Children[0].Layout != models.LayoutLink {
		t.Fatalf("expected air-carrier-176 (facts) with 1 link child, got %+v (err=%v)", ek176, err)
	}

	// 5. Deleting a module removes the JSON file permanently and never resurrects it
	if err := DeleteModule(modulesDir, "containers"); err != nil {
		t.Fatalf("DeleteModule(containers): %v", err)
	}
	if _, err := os.Stat(filepath.Join(modulesDir, "containers.json")); !os.IsNotExist(err) {
		t.Fatalf("expected containers.json to be deleted from disk")
	}
	afterDel, _, err := LoadAllModules(modulesDir)
	if err != nil || len(afterDel) != 4 {
		t.Fatalf("expected 4 modules after deleting containers, got %d (err=%v)", len(afterDel), err)
	}
}

func TestCompileLocalOnlyModulePropagation(t *testing.T) {
	manifests := []models.ModuleManifest{
		{
			ID:             "base-public",
			Name:           "Base Public",
			Category:       "Core",
			DefaultEnabled: true,
			Cards: []models.SearchCard{
				{
					ID:         "ent-1",
					EntityKey:  "ent:1",
					Title:      "Entity One",
					Collection: "core",
					Layout:     models.LayoutFacts,
					Data:       map[string]any{"summary": "Base entity"},
				},
			},
		},
		{
			ID:             "internal-vault",
			Name:           "Internal Vault",
			Category:       "Security",
			DefaultEnabled: true,
			LocalOnly:      true,
			Cards: []models.SearchCard{
				{
					ID:         "secret-runbook",
					Title:      "Secret Runbook",
					Collection: "vault",
					Layout:     models.LayoutFacts,
					Data:       map[string]any{"summary": "Internal runbook"},
				},
			},
			Overlays: []models.CardOverlay{
				{
					TargetEntityKey: "ent:1",
					Section: models.CardSection{
						Title: "Internal Credentials",
						Notes: "Local only notes",
					},
				},
				{
					TargetEntityKey: "missing:2",
					FallbackTitle:   "Standalone Secret",
					Section: models.CardSection{
						Title: "Private Endpoint",
						Notes: "Local only",
					},
				},
			},
		},
	}

	compiled := CompileActiveModules(manifests, []string{"base-public", "internal-vault"})
	var foundBase, foundSecret, foundPromoted bool
	for _, c := range compiled {
		switch c.ID {
		case "ent-1":
			foundBase = true
			if c.LocalOnly {
				t.Fatalf("base public card should not be marked localOnly")
			}
			if len(c.Sections) != 1 || !c.Sections[0].LocalOnly || c.Sections[0].Provenance == nil || !c.Sections[0].Provenance.LocalOnly {
				t.Fatalf("expected localOnly propagated to overlay section, got %+v", c.Sections)
			}
		case "secret-runbook":
			foundSecret = true
			if !c.LocalOnly || c.Provenance == nil || !c.Provenance.LocalOnly {
				t.Fatalf("expected localOnly propagated to module card and provenance, got %+v", c)
			}
		case "internal-vault-missing-2":
			foundPromoted = true
			if !c.LocalOnly || c.Provenance == nil || !c.Provenance.LocalOnly {
				t.Fatalf("expected localOnly propagated to promoted overlay card, got %+v", c)
			}
		}
	}
	if !foundBase || !foundSecret || !foundPromoted {
		t.Fatalf("missing expected compiled cards: base=%v secret=%v promoted=%v", foundBase, foundSecret, foundPromoted)
	}
}
