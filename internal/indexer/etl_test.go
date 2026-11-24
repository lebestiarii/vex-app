package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vex/internal/models"
)

func TestETLCSVSampleAndTransform(t *testing.T) {
	// Include UTF-8 BOM to simulate Excel-exported CSV
	csvPayload := "\xEF\xBB\xBFName,Host,User,Tags,Notes\n" +
		"Prod API,10.0.1.10,ubuntu,\"prod,api\",Primary API node\n" +
		"Staging DB,10.0.2.20,postgres,\"staging,db\",Postgres replica\n"

	sample, err := SampleRawData([]byte(csvPayload), "servers.csv")
	if err != nil {
		t.Fatalf("SampleRawData: %v", err)
	}
	if sample.Format != "csv" {
		t.Errorf("got format %q, want csv", sample.Format)
	}
	if sample.TotalRows != 2 || len(sample.SampleRows) != 2 {
		t.Fatalf("expected 2 rows, got total=%d sample=%d", sample.TotalRows, len(sample.SampleRows))
	}

	rule := models.ETLMappingRule{
		Name:             "Import SSH Hosts",
		SourceFormat:     "csv",
		TargetCollection: "servers",
		DefaultTemplate:  models.TemplateKeyValue,
		TitleField:       "Name",
		SubtitleField:    "{{User}}@{{Host}}",
		DescriptionField: "Notes",
		TagsField:        "Tags",
		StaticTags:       []string{"ssh"},
		Modifiers:        []string{"ssh"},
		ActionType:       "copy",
		ActionLabel:      "Copy SSH Command",
		ActionTarget:     "ssh {{User}}@{{Host}}",
	}

	cards, err := TransformWithRule([]byte(csvPayload), rule)
	if err != nil {
		t.Fatalf("TransformWithRule: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	if cards[0].Title != "Prod API" || cards[0].Subtitle != "ubuntu@10.0.1.10" {
		t.Errorf("unexpected card[0]: %+v", cards[0])
	}
	if len(cards[0].Actions) != 1 || cards[0].Actions[0].Target != "ssh ubuntu@10.0.1.10" {
		t.Errorf("unexpected action: %+v", cards[0].Actions)
	}
	if cards[0].Data["Host"] != "10.0.1.10" {
		t.Errorf("expected Data.Host to be preserved, got %+v", cards[0].Data)
	}
}

func TestETLNestedIncompatibleJSON(t *testing.T) {
	// Simulates a browser bookmark export with nested object array
	rawJSON := `{
		"version": 1,
		"roots": {
			"bookmark_bar": {
				"children": [
					{"name": "Wails Docs", "url": "https://v3alpha.wails.io", "meta": {"category": "go"}},
					{"name": "MCP Spec", "url": "https://modelcontextprotocol.io", "meta": {"category": "ai"}}
				]
			}
		}
	}`

	sample, err := SampleRawData([]byte(rawJSON), "bookmarks.json")
	if err != nil {
		t.Fatalf("SampleRawData nested JSON: %v", err)
	}
	if sample.DetectedRootPath != "roots.bookmark_bar.children" {
		t.Fatalf("expected detectedRootPath roots.bookmark_bar.children, got %q", sample.DetectedRootPath)
	}
	if sample.TotalRows != 2 {
		t.Fatalf("expected 2 rows, got %d", sample.TotalRows)
	}

	rule := models.ETLMappingRule{
		SourceFormat:     "json",
		RootPath:         sample.DetectedRootPath,
		TargetCollection: "bookmarks",
		DefaultTemplate:  models.TemplateAppShortcut,
		TitleField:       "name",
		SubtitleField:    "url",
		TagsField:        "meta.category",
		ActionType:       "open_url",
		ActionTarget:     "{{url}}",
	}

	cards, err := TransformWithRule([]byte(rawJSON), rule)
	if err != nil {
		t.Fatalf("TransformWithRule nested JSON: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 bookmark cards, got %d", len(cards))
	}
	if cards[0].Actions[0].Target != "https://v3alpha.wails.io" {
		t.Errorf("unexpected bookmark action target: %+v", cards[0].Actions[0])
	}
	if len(cards[0].Tags) != 1 || cards[0].Tags[0] != "go" {
		t.Errorf("expected nested tag 'go', got %v", cards[0].Tags)
	}
}

func TestWatcherLiveReload(t *testing.T) {
	tmpDir := t.TempDir()
	if err := EnsureDataDir(tmpDir); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	if _, err := UpsertCardToDisk(tmpDir, models.SearchCard{
		ID:          "initial-card-1",
		Title:       "Initial Card",
		Description: "Verifies startup scan.",
		Collection:  "notes",
		Template:    models.TemplateKnowledge,
	}); err != nil {
		t.Fatalf("UpsertCardToDisk initial: %v", err)
	}

	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	updates := make(chan UpdateEvent, 16)
	watcher := NewWatcher(store, tmpDir, func(ev UpdateEvent) {
		select {
		case updates <- ev:
		default:
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("watcher.Start: %v", err)
	}
	defer watcher.Stop()

	// Verify initial card was indexed
	initial, err := store.ListCards(ctx, "")
	if err != nil {
		t.Fatalf("ListCards: %v", err)
	}
	if len(initial) != 1 {
		t.Fatalf("expected 1 initial card, got %d", len(initial))
	}

	// Add a brand new collection file and wait for fsnotify live indexing
	newCard := models.SearchCard{
		ID:          "live-note-1",
		Title:       "Live Reloaded RAG Note",
		Description: "Verifies real-time fsnotify file watching.",
		Collection:  "live-notes",
		Template:    models.TemplateKnowledge,
		Tags:        []string{"realtime"},
	}
	savedPath, err := UpsertCardToDisk(tmpDir, newCard)
	if err != nil {
		t.Fatalf("UpsertCardToDisk: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		found, err := store.GetCard(ctx, "live-note-1")
		if err == nil && found != nil {
			break
		}
		select {
		case <-updates:
		case <-deadline:
			t.Fatalf("timed out waiting for fsnotify to index %s", savedPath)
		case <-time.After(30 * time.Millisecond):
		}
	}

	// Remove the file and verify watcher prunes the card
	if err := os.Remove(savedPath); err != nil {
		t.Fatalf("os.Remove(%s): %v", savedPath, err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "live-notes.json")); !os.IsNotExist(err) {
		t.Fatalf("expected live-notes.json to be deleted")
	}

	deadline = time.After(3 * time.Second)
	for {
		found, err := store.GetCard(ctx, "live-note-1")
		if err == nil && found == nil {
			break
		}
		select {
		case <-updates:
		case <-deadline:
			t.Fatalf("timed out waiting for fsnotify to prune deleted file %s", savedPath)
		case <-time.After(30 * time.Millisecond):
		}
	}
}
