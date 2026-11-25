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

func TestStoreFTSAndFiltering(t *testing.T) {
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
			Layout:      models.LayoutSnippet,
			Tags:        []string{"k8s", "staging", "deploy"},
			Modifiers:   []string{"k8s", "deploy"},
			Data:        map[string]any{"language": "bash", "body": "helm upgrade vex ./chart"},
		},
		{
			ID:          "note-rollback",
			Title:       "Incident Runbook",
			Subtitle:    "Emergency recovery steps",
			Description: "If a Kubernetes release fails, revert the helm chart and inspect pod logs.",
			Collection:  "knowledge",
			Layout:      models.LayoutFacts,
			Tags:        []string{"ops", "incident"},
			Modifiers:   []string{"docs"},
			Data:        map[string]any{"summary": "Rollback guide for failed helm releases."},
		},
		{
			ID:          "arch-guide",
			Title:       "Container Orchestration Guide",
			Subtitle:    "Managing microservices in production",
			Description: "Cluster autoscaling and pod scheduling best practices.",
			Collection:  "knowledge",
			Layout:      models.LayoutFacts,
			Tags:        []string{"architecture"},
			Data:        map[string]any{"summary": "Cluster autoscaling practices."},
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

	t.Run("filters by collection, layout, modifier, and tags", func(t *testing.T) {
		res, err := store.SearchHybrid(ctx, SearchOptions{
			Collection: "devops",
			Layout:     models.LayoutSnippet,
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

	t.Run("ListCollections and ReplaceCollection pruning", func(t *testing.T) {
		stats, err := store.ListCollections(ctx)
		if err != nil {
			t.Fatalf("ListCollections: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("expected 2 collections, got %d", len(stats))
		}

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

func TestStoreV2ToV3Migration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v2_upgrade.db")

	// Seed a real SQLite schema v2 database (cards.template, card_embeddings, user_version = 2).
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open v2 db: %v", err)
	}
	v2Stmts := []string{
		`CREATE TABLE cards (
			id TEXT PRIMARY KEY,
			collection TEXT NOT NULL,
			template TEXT NOT NULL,
			title TEXT NOT NULL,
			subtitle TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			entity_key TEXT NOT NULL DEFAULT '',
			entity_code TEXT NOT NULL DEFAULT '',
			chips_text TEXT NOT NULL DEFAULT '',
			related_keys_json TEXT NOT NULL DEFAULT '[]',
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
			entity_key,
			title,
			chips,
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
		`PRAGMA user_version = 2;`,
	}
	for _, stmt := range v2Stmts {
		if _, err := rawDB.Exec(stmt); err != nil {
			_ = rawDB.Close()
			t.Fatalf("exec v2 ddl: %v", err)
		}
	}

	parentCard := models.SearchCard{
		ID:          "loc-ams",
		EntityKey:   "loc:AMS",
		RelatedKeys: []string{"region:eu"},
		Title:       "Amsterdam Gateway (AMS)",
		Subtitle:    "Netherlands Hub",
		Collection:  "locations",
		Layout:      models.LayoutGroup,
		Tags:        []string{"port", "europe"},
		Chips:       []models.GlanceChip{{Label: "NLAMS · AMS", Tone: "mono"}},
	}
	childCard := models.SearchCard{
		ID:              "ams-portal",
		ParentEntityKey: "loc:AMS",
		Title:           "AMS Customs Portal",
		Collection:      "links",
		Layout:          models.LayoutLink,
		Data:            map[string]any{"url": "https://customs.ams.internal"},
	}
	for _, c := range []*models.SearchCard{&parentCard, &childCard} {
		if err := models.ValidateAndNormalizeCard(c, c.Collection); err != nil {
			_ = rawDB.Close()
			t.Fatalf("ValidateAndNormalizeCard(%s): %v", c.ID, err)
		}
		cardBytes, _ := json.Marshal(c)
		ek, ec := normalizeEntityKey(c.EntityKey)
		if _, err := rawDB.Exec(`
			INSERT INTO cards (id, collection, template, title, subtitle, description, entity_key, entity_code, chips_text, related_keys_json, tags_json, modifiers_json, card_json, content_hash, source_file, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, c.ID, c.Collection, string(c.Layout), c.Title, c.Subtitle, c.Description,
			ek, ec, c.ChipsText(), marshalStringSliceJSON(c.RelatedKeys),
			marshalStringSliceJSON(c.Tags), marshalStringSliceJSON(c.Modifiers),
			string(cardBytes), c.ContentHash(), "v2.json", "2026-11-01T00:00:00Z",
		); err != nil {
			_ = rawDB.Close()
			t.Fatalf("insert v2 card %s: %v", c.ID, err)
		}
	}
	if _, err := rawDB.Exec(`
		INSERT INTO card_embeddings (card_id, model, content_hash, vector, dims)
		VALUES (?, 'legacy-embed', ?, X'0000803F', 1)
	`, parentCard.ID, parentCard.ContentHash()); err != nil {
		_ = rawDB.Close()
		t.Fatalf("insert legacy embedding: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close v2 db: %v", err)
	}

	// Reopen via OpenStore -> triggers atomic v2 -> v3 migration.
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore migrated v2->v3: %v", err)
	}
	defer store.Close()

	var version int
	if err := store.db.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 3 {
		t.Fatalf("user_version = %d, want 3", version)
	}

	// card_embeddings must be dropped in v3.
	var embTableCount int
	if err := store.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='card_embeddings';`).Scan(&embTableCount); err != nil {
		t.Fatalf("check card_embeddings table: %v", err)
	}
	if embTableCount != 0 {
		t.Fatalf("expected card_embeddings table to be dropped in v3")
	}

	// Verify parent_entity_key backfilled and entity_links populated for both child_of and related_to.
	var parentKey string
	if err := store.db.QueryRow(`SELECT parent_entity_key FROM cards WHERE id = ?`, childCard.ID).Scan(&parentKey); err != nil {
		t.Fatalf("query parent_entity_key: %v", err)
	}
	if parentKey != "loc:ams" {
		t.Fatalf("parent_entity_key = %q, want %q", parentKey, "loc:ams")
	}

	var childLinkCount, relLinkCount int
	_ = store.db.QueryRow(`SELECT count(*) FROM entity_links WHERE source_card_id = ? AND target_entity_key = 'loc:ams' AND relation_type = 'child_of'`, childCard.ID).Scan(&childLinkCount)
	_ = store.db.QueryRow(`SELECT count(*) FROM entity_links WHERE source_card_id = ? AND target_entity_key = 'region:eu' AND relation_type = 'related_to'`, parentCard.ID).Scan(&relLinkCount)
	if childLinkCount != 1 || relLinkCount != 1 {
		t.Fatalf("expected backfilled entity_links (child_of=%d related_to=%d), want 1/1", childLinkCount, relLinkCount)
	}

	// Verify SearchHybrid returns loc-ams with dynamically hydrated ams-portal child!
	res, err := store.SearchHybrid(context.Background(), SearchOptions{Query: "AMS", Limit: 10})
	if err != nil {
		t.Fatalf("SearchHybrid after v2->v3 migration: %v", err)
	}
	if len(res) != 1 || res[0].Card.ID != "loc-ams" || res[0].MatchSource != "exact" {
		t.Fatalf("expected exact loc-ams hit after migration, got %+v", res)
	}
	if len(res[0].Card.Children) != 1 || res[0].Card.Children[0].ID != "ams-portal" {
		t.Fatalf("expected dynamically hydrated child ams-portal under loc-ams, got %+v", res[0].Card.Children)
	}
}

func TestStoreCrossModuleParentEntityKeyNestingAndDepthCap(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Module A: Root location card
	if err := store.UpsertCard(ctx, models.SearchCard{
		ID:         "loc-ams",
		EntityKey:  "loc:AMS",
		ModuleID:   "mod-locations",
		Title:      "Amsterdam Gateway (AMS)",
		Collection: "locations",
		Layout:     models.LayoutGroup,
		Chips:      []models.GlanceChip{{Label: "NLAMS · AMS", Tone: "mono"}},
	}, "mod-locations.json"); err != nil {
		t.Fatalf("UpsertCard loc-ams: %v", err)
	}

	// Module B: Child card referencing Module A's entityKey
	if err := store.UpsertCard(ctx, models.SearchCard{
		ID:              "ams-edi-servers",
		EntityKey:       "svc:ams-edi",
		ParentEntityKey: "loc:AMS",
		ModuleID:        "mod-servers",
		Title:           "AMS EDI Servers",
		Collection:      "servers",
		Layout:          models.LayoutActionList,
		Data: map[string]any{
			"rows": []map[string]any{
				{
					"badge":   map[string]any{"label": "PROD", "tone": "red", "dot": "red"},
					"primary": "ams-prd-edi01.internal",
					"mono":    true,
					"actions": []map[string]any{{"label": "Copy", "type": "copy", "value": "ams-prd-edi01.internal"}},
				},
			},
		},
	}, "mod-servers.json"); err != nil {
		t.Fatalf("UpsertCard ams-edi-servers: %v", err)
	}

	// Module C: Grandchild referencing Module B's entityKey (L2 under loc:AMS)
	if err := store.UpsertCard(ctx, models.SearchCard{
		ID:              "ams-edi-runbook",
		EntityKey:       "doc:ams-edi-runbook",
		ParentEntityKey: "svc:ams-edi",
		ModuleID:        "mod-runbooks",
		Title:           "AMS EDI Restart SOP",
		Collection:      "runbooks",
		Layout:          models.LayoutSteps,
		Data: map[string]any{
			"steps":   []map[string]any{{"label": "Drain queue"}, {"label": "Restart broker"}},
			"summary": "Standard EDI restart steps.",
		},
	}, "mod-runbooks.json"); err != nil {
		t.Fatalf("UpsertCard ams-edi-runbook: %v", err)
	}

	// Module D: Great-grandchild referencing Module C's entityKey (L3 under loc:AMS -> must be capped at depth 2)
	if err := store.UpsertCard(ctx, models.SearchCard{
		ID:              "ams-edi-escalation",
		ParentEntityKey: "doc:ams-edi-runbook",
		ModuleID:        "mod-escalation",
		Title:           "Escalation Contact",
		Collection:      "runbooks",
		Layout:          models.LayoutFacts,
		Data:            map[string]any{"summary": "Page on-call SRE."},
	}, "mod-escalation.json"); err != nil {
		t.Fatalf("UpsertCard ams-edi-escalation: %v", err)
	}

	// 1. Search "AMS" -> loc-ams (L0) with ams-edi-servers (L1) -> ams-edi-runbook (L2), capped before L3.
	res, err := store.SearchHybrid(ctx, SearchOptions{Query: "AMS", Limit: 10})
	if err != nil {
		t.Fatalf("SearchHybrid(AMS): %v", err)
	}
	if len(res) != 1 || res[0].Card.ID != "loc-ams" {
		t.Fatalf("expected deduplicated top-level loc-ams only, got %+v", res)
	}
	l0 := res[0].Card
	if len(l0.Children) != 1 || l0.Children[0].ID != "ams-edi-servers" {
		t.Fatalf("expected L1 child ams-edi-servers from Module B, got %+v", l0.Children)
	}
	l1 := l0.Children[0]
	if len(l1.Children) != 1 || l1.Children[0].ID != "ams-edi-runbook" {
		t.Fatalf("expected L2 grandchild ams-edi-runbook from Module C, got %+v", l1.Children)
	}
	l2 := l1.Children[0]
	if len(l2.Children) != 0 {
		t.Fatalf("expected depth capped at L2 (0 children on L2), got %+v", l2.Children)
	}

	// 2. Directly querying Module C's card promotes it to L0, so Module D renders cleanly at L1!
	docCard, err := store.GetCard(ctx, "ams-edi-runbook")
	if err != nil || docCard == nil {
		t.Fatalf("GetCard(ams-edi-runbook): %+v err=%v", docCard, err)
	}
	if len(docCard.Children) != 1 || docCard.Children[0].ID != "ams-edi-escalation" {
		t.Fatalf("expected ams-edi-escalation at L1 when ams-edi-runbook is L0, got %+v", docCard.Children)
	}
}

func TestStoreCardRecents(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for _, c := range []models.SearchCard{
		{ID: "c1", EntityKey: "e:1", Title: "Card One", Collection: "test", Layout: models.LayoutGroup},
		{ID: "c1-child", ParentEntityKey: "e:1", Title: "Card One Link", Collection: "test", Layout: models.LayoutLink, Data: map[string]any{"url": "https://one.example"}},
		{ID: "c2", Title: "Card Two", Collection: "test", Layout: models.LayoutFacts, Data: map[string]any{"summary": "Two"}},
		{ID: "c3", Title: "Card Three", Collection: "test", Layout: models.LayoutFacts, Data: map[string]any{"summary": "Three"}},
	} {
		if err := store.UpsertCard(ctx, c, "recents.json"); err != nil {
			t.Fatalf("UpsertCard(%s): %v", c.ID, err)
		}
	}

	if err := store.RecordCardUsed(ctx, "c1"); err != nil {
		t.Fatalf("RecordCardUsed(c1): %v", err)
	}
	if err := store.RecordCardUsed(ctx, "c2"); err != nil {
		t.Fatalf("RecordCardUsed(c2): %v", err)
	}
	if err := store.RecordCardUsed(ctx, "c3"); err != nil {
		t.Fatalf("RecordCardUsed(c3): %v", err)
	}
	// Re-touch c1 so it moves to the top of recents.
	if err := store.RecordCardUsed(ctx, "c1"); err != nil {
		t.Fatalf("RecordCardUsed(c1) again: %v", err)
	}

	recents, err := store.ListRecentCards(ctx, 2)
	if err != nil {
		t.Fatalf("ListRecentCards: %v", err)
	}
	if len(recents) != 2 || recents[0].ID != "c1" || recents[1].ID != "c3" {
		t.Fatalf("expected [c1, c3], got %+v", recents)
	}
	if len(recents[0].Children) != 1 || recents[0].Children[0].ID != "c1-child" {
		t.Fatalf("expected hydrated child c1-child on recent card c1, got %+v", recents[0].Children)
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
			Layout:      models.LayoutGroup,
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
			Layout:      models.LayoutActionList,
			Tags:        []string{"edi", "servers"},
			Data: map[string]any{
				"rows": []map[string]any{
					{
						"badge":   map[string]any{"label": "PROD", "tone": "red", "dot": "red"},
						"primary": "ams-edi01.internal",
						"actions": []map[string]any{{"label": "Copy", "type": "copy", "value": "ams-edi01.internal"}},
					},
				},
			},
		},
		{
			ID:          "amsterdam-customs",
			Title:       "Amsterdam Customs Notes",
			Subtitle:    "EU ICS2 filing rules",
			Description: "Pre-arrival customs declarations.",
			Collection:  "guides",
			Layout:      models.LayoutSteps,
			Tags:        []string{"customs", "eu"},
			Data: map[string]any{
				"steps":   []map[string]any{{"label": "ENS"}, {"label": "Release"}},
				"summary": "EU ICS2 filing sequence.",
			},
		},
		{
			ID:          "noise-teams",
			Title:       "Platform Teams",
			Subtitle:    "Engineering roster",
			Description: "Directory of platform teams and on-call rotations.",
			Collection:  "org",
			Layout:      models.LayoutFacts,
			Tags:        []string{"teams", "org"},
			Data:        map[string]any{"summary": "Platform engineering directory."},
		},
		{
			ID:          "noise-diagrams",
			Title:       "Architecture Diagrams",
			Subtitle:    "System topology blueprints",
			Description: "Network diagrams for core services.",
			Collection:  "docs",
			Layout:      models.LayoutFacts,
			Tags:        []string{"diagrams"},
			Data:        map[string]any{"summary": "Network topology blueprints."},
		},
		{
			ID:          "noise-params",
			Title:       "Query Params",
			Subtitle:    "HTTP API conventions",
			Description: "Standard query params for pagination.",
			Collection:  "docs",
			Layout:      models.LayoutFacts,
			Tags:        []string{"api"},
			Data:        map[string]any{"summary": "Pagination query params."},
		},
		{
			ID:          "noise-url-only",
			Title:       "CDN Static Assets",
			Subtitle:    "Asset bucket",
			Description: "Public CDN mirror.",
			Collection:  "infra",
			Layout:      models.LayoutLink,
			Data:        map[string]any{"url": "https://example.test/ams-assets"},
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
		res, err := store.SearchHybrid(ctx, SearchOptions{Query: "iagra", Limit: 10})
		if err != nil {
			t.Fatalf("SearchHybrid(iagra): %v", err)
		}
		if len(res) != 1 || res[0].Card.ID != "noise-diagrams" || res[0].MatchSource != "fuzzy" {
			t.Fatalf("expected noise-diagrams (fuzzy), got %+v", res)
		}

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
