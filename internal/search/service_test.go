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
