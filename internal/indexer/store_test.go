package indexer

import (
	"context"
	"testing"

	"vex/internal/models"
)

func TestStoreFTSAndHybridRRF(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	cards := []models.SearchCard{
		{
			ID:          "k8s-deploy",
			Title:       "Kubernetes Staging Deploy",
			Subtitle:    "Rollout helm chart to staging cluster",
			Description: "Runs helm upgrade for the vex backend services.",
			Collection:  "devops",
			Template:    models.TemplateCommand,
			Tags:        []string{"k8s", "staging", "deploy"},
			Modifiers:   []string{"k8s", "deploy"},
		},
		{
			ID:          "note-rollback",
			Title:       "Incident Runbook",
			Subtitle:    "Emergency recovery steps",
			Description: "If a Kubernetes release fails, revert the helm chart and inspect pod logs.",
			Collection:  "knowledge",
			Template:    models.TemplateKnowledge,
			Tags:        []string{"ops", "incident"},
			Modifiers:   []string{"docs"},
		},
		{
			ID:          "semantic-only",
			Title:       "Container Orchestration Guide",
			Subtitle:    "Managing microservices in production",
			Description: "Cluster autoscaling and pod scheduling best practices.",
			Collection:  "knowledge",
			Template:    models.TemplateKnowledge,
			Tags:        []string{"architecture"},
		},
	}

	for _, c := range cards {
		if err := store.UpsertCard(ctx, c, "test.json"); err != nil {
			t.Fatalf("UpsertCard(%s): %v", c.ID, err)
		}
	}

	t.Run("BM25 ranks title match higher than body-only match", func(t *testing.T) {
		res, err := store.SearchHybrid(ctx, SearchOptions{Query: "kubernetes"})
		if err != nil {
			t.Fatalf("SearchHybrid: %v", err)
		}
		if len(res) < 2 {
			t.Fatalf("expected at least 2 results, got %d", len(res))
		}
		if res[0].Card.ID != "k8s-deploy" {
			t.Errorf("expected k8s-deploy first, got %s", res[0].Card.ID)
		}
	})

	t.Run("filters by collection, modifier, and tags", func(t *testing.T) {
		res, err := store.SearchHybrid(ctx, SearchOptions{
			Collection: "devops",
			Modifier:   "k8s",
			Tags:       []string{"staging", "deploy"},
		})
		if err != nil {
			t.Fatalf("SearchHybrid filtered: %v", err)
		}
		if len(res) != 1 || res[0].Card.ID != "k8s-deploy" {
			t.Fatalf("unexpected filtered results: %+v", res)
		}
	})

	t.Run("hybrid RRF combines FTS lexical and vector cosine similarity", func(t *testing.T) {
		// Store mock 3D embeddings: k8s-deploy and semantic-only align closely with [1, 0, 0]
		if err := store.UpsertEmbedding(ctx, "k8s-deploy", "mock-embed", cards[0].ContentHash(), []float32{0.95, 0.05, 0.0}); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
		if err := store.UpsertEmbedding(ctx, "semantic-only", "mock-embed", cards[2].ContentHash(), []float32{0.99, 0.01, 0.0}); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}
		if err := store.UpsertEmbedding(ctx, "note-rollback", "mock-embed", cards[1].ContentHash(), []float32{0.0, 1.0, 0.0}); err != nil {
			t.Fatalf("UpsertEmbedding: %v", err)
		}

		res, err := store.SearchHybrid(ctx, SearchOptions{
			Query:          "kubernetes",
			QueryEmbedding: []float32{1.0, 0.0, 0.0},
			EmbeddingModel: "mock-embed",
		})
		if err != nil {
			t.Fatalf("SearchHybrid vector+fts: %v", err)
		}
		if len(res) != 3 {
			t.Fatalf("expected 3 results, got %d", len(res))
		}
		// k8s-deploy matches BOTH FTS and Vector -> highest RRF score and "hybrid" source
		if res[0].Card.ID != "k8s-deploy" || res[0].MatchSource != "hybrid" {
			t.Errorf("expected k8s-deploy (hybrid) first, got %s (%s)", res[0].Card.ID, res[0].MatchSource)
		}
		// semantic-only matches via vector even without the word "kubernetes"
		foundSemantic := false
		for _, r := range res {
			if r.Card.ID == "semantic-only" && r.MatchSource == "vector" {
				foundSemantic = true
			}
		}
		if !foundSemantic {
			t.Errorf("expected semantic-only to be returned via vector search, got %+v", res)
		}
	})

	t.Run("CardsNeedingEmbedding detects hash changes", func(t *testing.T) {
		needing, err := store.CardsNeedingEmbedding(ctx, "mock-embed")
		if err != nil {
			t.Fatalf("CardsNeedingEmbedding: %v", err)
		}
		if len(needing) != 0 {
			t.Fatalf("expected 0 cards needing embedding, got %d", len(needing))
		}

		// Mutate description of k8s-deploy
		updated := cards[0]
		updated.Description = "Updated deployment steps with canary rollout"
		if err := store.UpsertCard(ctx, updated, "test.json"); err != nil {
			t.Fatalf("UpsertCard update: %v", err)
		}

		needing, err = store.CardsNeedingEmbedding(ctx, "mock-embed")
		if err != nil {
			t.Fatalf("CardsNeedingEmbedding after update: %v", err)
		}
		if len(needing) != 1 || needing[0].ID != "k8s-deploy" {
			t.Fatalf("expected k8s-deploy to need re-embedding, got %+v", needing)
		}
	})

	t.Run("ListCollections and ReplaceCollection pruning", func(t *testing.T) {
		stats, err := store.ListCollections(ctx)
		if err != nil {
			t.Fatalf("ListCollections: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("expected 2 collections, got %d", len(stats))
		}

		// Replace test.json with only 1 card -> other 2 should be pruned
		if err := store.ReplaceCollection(ctx, "devops", []models.SearchCard{cards[0]}, "test.json"); err != nil {
			t.Fatalf("ReplaceCollection: %v", err)
		}
		remaining, err := store.ListCards(ctx, "")
		if err != nil {
			t.Fatalf("ListCards: %v", err)
		}
		if len(remaining) != 1 || remaining[0].ID != "k8s-deploy" {
			t.Fatalf("expected only k8s-deploy remaining after prune, got %d", len(remaining))
		}
	})
}
