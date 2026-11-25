package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"vex/internal/models"

	_ "modernc.org/sqlite"
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

func TestStoreLegacyV0ToV2Migration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy_v0.db")

	// Seed a legacy v0 schema (12-col cards, 7-col cards_fts, user_version = 0).
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open legacy db: %v", err)
	}
	legacyStmts := []string{
		`CREATE TABLE cards (
			id TEXT PRIMARY KEY,
			collection TEXT NOT NULL,
			template TEXT NOT NULL,
			title TEXT NOT NULL,
			subtitle TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			modifiers_json TEXT NOT NULL DEFAULT '[]',
			card_json TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			source_file TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,
		`CREATE VIRTUAL TABLE cards_fts USING fts5(
			id UNINDEXED,
			collection UNINDEXED,
			title,
			modifiers,
			tags,
			subtitle,
			body,
			tokenize = 'porter unicode61'
		);`,
		`CREATE TABLE card_embeddings (
			card_id TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			vector BLOB NOT NULL,
			dims INTEGER NOT NULL,
			FOREIGN KEY (card_id) REFERENCES cards(id) ON DELETE CASCADE
		);`,
	}
	for _, stmt := range legacyStmts {
		if _, err := rawDB.Exec(stmt); err != nil {
			_ = rawDB.Close()
			t.Fatalf("exec legacy ddl: %v", err)
		}
	}

	legacyCard := models.SearchCard{
		ID:          "loc-ams",
		EntityKey:   "loc:AMS",
		RelatedKeys: []string{"edi:AMS"},
		Title:       "Amsterdam Gateway (AMS)",
		Subtitle:    "Netherlands Hub",
		Collection:  "locations",
		Template:    models.TemplateLocation,
		Tags:        []string{"port", "europe"},
		Chips:       []models.GlanceChip{{Label: "NLAMS · AMS", Tone: "mono"}},
	}
	if err := models.ValidateAndNormalizeCard(&legacyCard, "locations"); err != nil {
		_ = rawDB.Close()
		t.Fatalf("ValidateAndNormalizeCard: %v", err)
	}
	cardBytes, _ := json.Marshal(legacyCard)
	tagsBytes, _ := json.Marshal(legacyCard.Tags)
	modsBytes, _ := json.Marshal(legacyCard.Modifiers)

	if _, err := rawDB.Exec(`
		INSERT INTO cards (id, collection, template, title, subtitle, description, tags_json, modifiers_json, card_json, content_hash, source_file, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, legacyCard.ID, legacyCard.Collection, string(legacyCard.Template), legacyCard.Title, legacyCard.Subtitle,
		legacyCard.Description, string(tagsBytes), string(modsBytes), string(cardBytes), legacyCard.ContentHash(), "legacy.json", "2026-11-01T00:00:00Z",
	); err != nil {
		_ = rawDB.Close()
		t.Fatalf("insert legacy card: %v", err)
	}
	if _, err := rawDB.Exec(`
		INSERT INTO card_embeddings (card_id, model, content_hash, vector, dims)
		VALUES (?, 'mock-embed', ?, X'0000803F', 1)
	`, legacyCard.ID, legacyCard.ContentHash()); err != nil {
		_ = rawDB.Close()
		t.Fatalf("insert legacy embedding: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	// Reopen via OpenStore -> triggers atomic v0 -> v2 migration.
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore migrated: %v", err)
	}
	defer store.Close()

	var version int
	if err := store.db.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, currentSchemaVersion)
	}

	var ek, ec, chipsText, relJSON string
	if err := store.db.QueryRow(`SELECT entity_key, entity_code, chips_text, related_keys_json FROM cards WHERE id = ?`, legacyCard.ID).
		Scan(&ek, &ec, &chipsText, &relJSON); err != nil {
		t.Fatalf("query backfilled columns: %v", err)
	}
	if ek != "loc:ams" || ec != "ams" || chipsText != "NLAMS · AMS" || relJSON != `["edi:AMS"]` {
		t.Fatalf("unexpected backfilled metadata: ek=%q ec=%q chips=%q rel=%q", ek, ec, chipsText, relJSON)
	}

	var embedCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM card_embeddings WHERE card_id = ?`, legacyCard.ID).Scan(&embedCount); err != nil {
		t.Fatalf("count embeddings: %v", err)
	}
	if embedCount != 1 {
		t.Fatalf("expected legacy embedding preserved, got count=%d", embedCount)
	}

	byKeys, err := store.FindCardsByEntityKeys(context.Background(), []string{"AMS"})
	if err != nil {
		t.Fatalf("FindCardsByEntityKeys: %v", err)
	}
	if len(byKeys) != 1 || byKeys[0].ID != legacyCard.ID {
		t.Fatalf("expected FindCardsByEntityKeys('AMS') to find %s, got %+v", legacyCard.ID, byKeys)
	}

	res, err := store.SearchHybrid(context.Background(), SearchOptions{Query: "AMS", Limit: 10})
	if err != nil {
		t.Fatalf("SearchHybrid after migration: %v", err)
	}
	if len(res) != 1 || res[0].Card.ID != legacyCard.ID || res[0].MatchSource != "exact" {
		t.Fatalf("expected exact AMS hit after migration, got %+v", res)
	}
}

func TestStoreSearchPrecisionAndAntiSprawl(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	cards := []models.SearchCard{
		{
			ID:          "loc-ams",
			EntityKey:   "loc:AMS",
			Title:       "Amsterdam Gateway (AMS)",
			Subtitle:    "Netherlands Air & Ocean Hub",
			Description: "Primary European gateway.",
			Collection:  "locations",
			Template:    models.TemplateLocation,
			Tags:        []string{"gateway", "europe"},
			Chips:       []models.GlanceChip{{Label: "NLAMS · AMS", Tone: "mono"}},
		},
		{
			ID:          "ams-edi-servers",
			EntityKey:   "servers:ams-edi",
			RelatedKeys: []string{"loc:AMS"},
			Title:       "AMS EDI Servers",
			Subtitle:    "DEV / QA / PROD EDI endpoints",
			Description: "Message broker cluster for Amsterdam.",
			Collection:  "servers",
			Template:    models.TemplateServerMatrix,
			Tags:        []string{"edi", "servers"},
		},
		{
			ID:          "amsterdam-customs",
			Title:       "Amsterdam Customs Notes",
			Subtitle:    "EU ICS2 filing rules",
			Description: "Pre-arrival customs declarations.",
			Collection:  "guides",
			Template:    models.TemplateDictionaryGuide,
			Tags:        []string{"customs", "eu"},
		},
		{
			ID:          "noise-teams",
			Title:       "Platform Teams",
			Subtitle:    "Engineering roster",
			Description: "Directory of platform teams and on-call rotations.",
			Collection:  "org",
			Template:    models.TemplateKnowledge,
			Tags:        []string{"teams", "org"},
		},
		{
			ID:          "noise-diagrams",
			Title:       "Architecture Diagrams",
			Subtitle:    "System topology blueprints",
			Description: "Network diagrams for core services.",
			Collection:  "docs",
			Template:    models.TemplateKnowledge,
			Tags:        []string{"diagrams"},
		},
		{
			ID:          "noise-params",
			Title:       "Query Params",
			Subtitle:    "HTTP API conventions",
			Description: "Standard query params for pagination.",
			Collection:  "docs",
			Template:    models.TemplateKnowledge,
			Tags:        []string{"api"},
		},
		{
			ID:          "noise-url-only",
			Title:       "CDN Static Assets",
			Subtitle:    "Asset bucket",
			Description: "Public CDN mirror.",
			Collection:  "infra",
			Template:    models.TemplateWebsiteLink,
			Data: map[string]any{
				"url":         "https://example.test/ams-assets",
				"trackingUrl": "https://track.example.test/ams-events",
			},
		},
	}

	for _, c := range cards {
		if err := store.UpsertCard(ctx, c, "precision.json"); err != nil {
			t.Fatalf("UpsertCard(%s): %v", c.ID, err)
		}
	}

	t.Run("exact AMS ranks #1 with contextual neighbor and prunes substring sprawl", func(t *testing.T) {
		res, err := store.SearchHybrid(ctx, SearchOptions{Query: "AMS", Limit: 10})
		if err != nil {
			t.Fatalf("SearchHybrid(AMS): %v", err)
		}
		if len(res) < 2 {
			t.Fatalf("expected at least 2 hits (exact + contextual), got %d", len(res))
		}
		if res[0].Card.ID != "loc-ams" || res[0].MatchSource != "exact" {
			t.Fatalf("expected loc-ams (exact) #1, got %s (%s, score=%.2f)", res[0].Card.ID, res[0].MatchSource, res[0].Score)
		}
		if res[1].Card.ID != "ams-edi-servers" || res[1].MatchSource != "contextual" {
			t.Fatalf("expected ams-edi-servers (contextual) #2, got %s (%s, score=%.2f)", res[1].Card.ID, res[1].MatchSource, res[1].Score)
		}

		banned := map[string]bool{
			"noise-teams":    true,
			"noise-diagrams": true,
			"noise-params":   true,
			"noise-url-only": true,
		}
		for _, r := range res {
			if banned[r.Card.ID] {
				t.Errorf("unexpected sprawl card %q returned for AMS (source=%s, score=%.2f)", r.Card.ID, r.MatchSource, r.Score)
			}
		}
	})

	t.Run("prefix query matches Amsterdam cards", func(t *testing.T) {
		res, err := store.SearchHybrid(ctx, SearchOptions{Query: "amst", Limit: 10})
		if err != nil {
			t.Fatalf("SearchHybrid(amst): %v", err)
		}
		if len(res) < 2 {
			t.Fatalf("expected at least 2 prefix hits for 'amst', got %d", len(res))
		}
		seen := make(map[string]bool)
		for _, r := range res {
			seen[r.Card.ID] = true
		}
		if !seen["loc-ams"] || !seen["amsterdam-customs"] {
			t.Errorf("expected loc-ams and amsterdam-customs in prefix hits, got %+v", seen)
		}
	})

	t.Run("multi-word query ranks AMS EDI Servers first", func(t *testing.T) {
		res, err := store.SearchHybrid(ctx, SearchOptions{Query: "ams edi", Limit: 10})
		if err != nil {
			t.Fatalf("SearchHybrid(ams edi): %v", err)
		}
		if len(res) == 0 || res[0].Card.ID != "ams-edi-servers" {
			t.Fatalf("expected ams-edi-servers first for 'ams edi', got %+v", res)
		}
	})

	t.Run("guarded fuzzy matches metadata but never raw card_json URLs", func(t *testing.T) {
		// Mid-word substring in title triggers Tier 4 fuzzy when FTS yields < 3 hits.
		res, err := store.SearchHybrid(ctx, SearchOptions{Query: "iagra", Limit: 10})
		if err != nil {
			t.Fatalf("SearchHybrid(iagra): %v", err)
		}
		if len(res) != 1 || res[0].Card.ID != "noise-diagrams" || res[0].MatchSource != "fuzzy" {
			t.Fatalf("expected noise-diagrams (fuzzy), got %+v", res)
		}

		// URL-only substring inside data.url must NOT match even in fuzzy fallback.
		urlRes, err := store.SearchHybrid(ctx, SearchOptions{Query: "ams-assets", Limit: 10})
		if err != nil {
			t.Fatalf("SearchHybrid(ams-assets): %v", err)
		}
		if len(urlRes) != 0 {
			t.Fatalf("expected 0 results for URL-only 'ams-assets', got %+v", urlRes)
		}
	})

	t.Run("FindRelatedCards resolves linked neighbor via metadata", func(t *testing.T) {
		rel, err := store.FindRelatedCards(ctx, "loc-ams", 5)
		if err != nil {
			t.Fatalf("FindRelatedCards: %v", err)
		}
		if len(rel) == 0 || rel[0].ID != "ams-edi-servers" {
			t.Fatalf("expected ams-edi-servers related to loc-ams, got %+v", rel)
		}
	})
}
