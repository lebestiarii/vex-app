package search

import (
	"context"
	"testing"

	"vex/internal/indexer"
	"vex/internal/models"
)

type stubPluginProvider struct {
	cards []models.SearchCard
}

func (s stubPluginProvider) QueryPlugins(_ context.Context, q ParsedQuery) ([]models.SearchCard, error) {
	if q.Modifier == "live" {
		return s.cards, nil
	}
	return nil, nil
}

func TestSearchServiceModesAndPlugins(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	_ = store.UpsertCard(ctx, models.SearchCard{
		ID:          "gh-card",
		Title:       "GitHub Pull Requests",
		Subtitle:    "Review open PRs",
		Collection:  "shortcuts",
		Template:    models.TemplateAppShortcut,
		Tags:        []string{"git", "work"},
		Modifiers:   []string{"gh"},
		Description: "Opens repository pull request dashboard",
	}, "shortcuts.json")

	svc := NewService(store)
	svc.SetPluginProvider(stubPluginProvider{
		cards: []models.SearchCard{
			{
				ID:         "plugin-live-1",
				Title:      "Dynamic Plugin Metric",
				Collection: "plugin-live",
				Template:   models.TemplateMetric,
			},
		},
	})

	t.Run("modifier query merges dynamic plugin cards at top", func(t *testing.T) {
		resp, err := svc.Search(ctx, "/live status", 10)
		if err != nil {
			t.Fatalf("Search plugin: %v", err)
		}
		if len(resp.Results) == 0 || resp.Results[0].Card.ID != "plugin-live-1" {
			t.Fatalf("expected dynamic plugin card first, got %+v", resp.Results)
		}
		if resp.Results[0].MatchSource != "plugin" {
			t.Errorf("expected matchSource plugin, got %q", resp.Results[0].MatchSource)
		}
	})

	t.Run("quick URL fallback when no cards match", func(t *testing.T) {
		resp, err := svc.Search(ctx, "https://example.org/docs", 10)
		if err != nil {
			t.Fatalf("Search URL fallback: %v", err)
		}
		if len(resp.Results) != 1 || resp.Results[0].Card.ID != "builtin-quick-url" {
			t.Fatalf("expected quick URL fallback card, got %+v", resp.Results)
		}
	})
}

func TestSearchServicePrecisionAndPrecedence(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	seed := []models.SearchCard{
		{
			ID:         "loc-ams",
			EntityKey:  "loc:AMS",
			Title:      "Amsterdam Gateway (AMS)",
			Subtitle:   "Netherlands Hub",
			Collection: "locations",
			Template:   models.TemplateLocation,
			Chips:      []models.GlanceChip{{Label: "NLAMS · AMS", Tone: "mono"}},
		},
		{
			ID:          "noise-teams",
			Title:       "Platform Teams",
			Description: "On-call teams roster",
			Collection:  "org",
			Template:    models.TemplateKnowledge,
		},
		{
			ID:          "weight-guide",
			Title:       "100 kg Cargo Limit",
			Description: "Standard 100 kg pallet rule",
			Collection:  "guides",
			Template:    models.TemplateKnowledge,
		},
	}
	for _, c := range seed {
		if err := store.UpsertCard(ctx, c, "seed.json"); err != nil {
			t.Fatalf("UpsertCard(%s): %v", c.ID, err)
		}
	}

	svc := NewService(store)
	enabled := true
	svc.SetPatternRules([]models.PatternRule{
		{
			ID:            "test-ref-code",
			Name:          "Test Reference",
			Enabled:       &enabled,
			Priority:      10,
			Regex:         `^REF-(?P<num>\d{4})$`,
			TitleTemplate: "Reference {{num}}",
			Template:      models.TemplatePatternBreakdown,
		},
	})

	t.Run("unit conversion calculator ranks #1 above indexed cards", func(t *testing.T) {
		resp, err := svc.Search(ctx, "100 kg to lb", 10)
		if err != nil {
			t.Fatalf("Search conversion: %v", err)
		}
		if len(resp.Results) == 0 || resp.Results[0].MatchSource != "calculator" || resp.Results[0].Score != 150.0 {
			t.Fatalf("expected calculator rank #1 (score 150), got %+v", resp.Results)
		}
	})

	t.Run("pattern decoder ranks #1", func(t *testing.T) {
		resp, err := svc.Search(ctx, "REF-4242", 10)
		if err != nil {
			t.Fatalf("Search pattern: %v", err)
		}
		if len(resp.Results) == 0 || resp.Results[0].MatchSource != "pattern" || resp.Results[0].Score != 145.0 {
			t.Fatalf("expected pattern rank #1 (score 145), got %+v", resp.Results)
		}
	})

	t.Run("exact AMS query returns exact match without noise sprawl", func(t *testing.T) {
		resp, err := svc.Search(ctx, "AMS", 10)
		if err != nil {
			t.Fatalf("Search AMS: %v", err)
		}
		if len(resp.Results) != 1 || resp.Results[0].Card.ID != "loc-ams" || resp.Results[0].MatchSource != "exact" {
			t.Fatalf("expected only loc-ams (exact), got %+v", resp.Results)
		}
	})
}
