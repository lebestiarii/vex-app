package indexer

import (
	"context"
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

func TestCompileActiveModulesAndSynergy(t *testing.T) {
	all := BuiltInModules()

	// 1. Default enabled modules: geographies + air + ocean + customs -> port-nlams gets 3 overlay sections
	compiled := CompileActiveModules(all, nil)
	var amsFound bool
	for _, c := range compiled {
		if c.ID == "port-nlams" {
			amsFound = true
			if len(c.Sections) != 3 {
				t.Fatalf("expected 3 overlay sections on port-nlams by default, got %d", len(c.Sections))
			}
			if c.Provenance == nil || c.Provenance.Authority != "official" {
				t.Fatalf("expected official provenance on port-nlams, got %+v", c.Provenance)
			}
			if len(c.GlancePills) == 0 {
				t.Fatalf("expected GlancePills to be populated on port-nlams")
			}
		}
	}
	if !amsFound {
		t.Fatalf("expected port-nlams in compiled default modules")
	}

	// 2. Standalone overlay promotion: only global-tech enabled -> port-nlams still exists as standalone card
	techOnly := CompileActiveModules(all, []string{"global-tech"})
	var promoted bool
	for _, c := range techOnly {
		if c.ID == "port-nlams" && c.ModuleID == "global-tech" && len(c.Sections) == 1 {
			promoted = true
		}
	}
	if !promoted {
		t.Fatalf("expected port-nlams overlay to promote to standalone card when geographies is disabled")
	}

	// 3. Store ReplaceSourceFile + FindRelatedCards synergy
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.ReplaceSourceFile(ctx, CompiledModulesSource, compiled); err != nil {
		t.Fatalf("ReplaceSourceFile: %v", err)
	}
	related, err := store.FindRelatedCards(ctx, "port-nlams", 5)
	if err != nil || len(related) < 3 {
		t.Fatalf("expected >= 3 related synergy cards for port-nlams, got %d (err=%v)", len(related), err)
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
