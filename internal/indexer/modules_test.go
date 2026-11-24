package indexer

import (
	"context"
	"os"
	"path/filepath"
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

func TestStarterModulesAreOptionalInstallableAndDeletable(t *testing.T) {
	modulesDir := t.TempDir()

	// 1. Fresh user modulesDir has 0 installed modules (nothing forced from Go code!)
	initial, _, err := LoadAllModules(modulesDir)
	if err != nil {
		t.Fatalf("LoadAllModules: %v", err)
	}
	if len(initial) != 0 {
		t.Fatalf("expected 0 modules in clean modulesDir, got %d", len(initial))
	}

	// 2. Starter catalog contains all 6 real/generic optional modules and every card validates cleanly
	starters := StarterModules()
	if len(starters) != 6 {
		t.Fatalf("expected 6 optional starter modules (zero sample packs), got %d", len(starters))
	}
	for _, m := range starters {
		for i := range m.Cards {
			c := m.Cards[i]
			if err := models.ValidateAndNormalizeCard(&c, m.ID); err != nil {
				t.Fatalf("starter module %q card %q invalid: %v", m.ID, c.ID, err)
			}
		}
	}

	// 3. Install selected starter modules -> written as real editable JSON files in modulesDir
	installed, err := InstallStarterModules(modulesDir, []string{"air-carriers", "ocean-carriers", "containers", "logistics-dictionary", "reference-patterns", "team-templates"})
	if err != nil {
		t.Fatalf("InstallStarterModules: %v", err)
	}
	if len(installed) != 6 {
		t.Fatalf("expected 6 installed modules, got %d", len(installed))
	}
	if _, err := os.Stat(filepath.Join(modulesDir, "air-carriers.json")); err != nil {
		t.Fatalf("expected air-carriers.json on disk: %v", err)
	}

	// Verify carrier counts and container blueprint cards
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
	if len(statuses) != 6 {
		t.Fatalf("expected 6 module statuses, got %d", len(statuses))
	}

	hc40, err := store.GetCard(ctx, "container-40hc")
	if err != nil || hc40 == nil || len(hc40.Children) != 2 {
		t.Fatalf("expected container-40hc with 2 nested children, got %+v (err=%v)", hc40, err)
	}

	// 4. Deleting a module removes the JSON file permanently and never resurrects it
	if err := DeleteModule(modulesDir, "containers"); err != nil {
		t.Fatalf("DeleteModule(containers): %v", err)
	}
	if _, err := os.Stat(filepath.Join(modulesDir, "containers.json")); !os.IsNotExist(err) {
		t.Fatalf("expected containers.json to be deleted from disk")
	}
	afterDel, _, err := LoadAllModules(modulesDir)
	if err != nil || len(afterDel) != 5 {
		t.Fatalf("expected 5 modules after deleting containers, got %d (err=%v)", len(afterDel), err)
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
					Template:   models.TemplateKnowledge,
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
					Template:   models.TemplateKnowledge,
				},
			},
			Overlays: []models.CardOverlay{
				{
					TargetEntityKey: "ent:1",
					Section: models.CardSection{
						Title: "Internal Credentials",
						Notes: "Do not send to cloud LLMs",
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
