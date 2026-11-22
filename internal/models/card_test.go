package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateAndNormalizeCard(t *testing.T) {
	t.Run("fills defaults and normalizes tags and modifiers", func(t *testing.T) {
		card := SearchCard{
			Title:     "  Production Bastion  ",
			Subtitle:  "SSH jump host",
			Tags:      []string{"#Prod", "ssh", "PROD", "  "},
			Modifiers: []string{"/SSH", "ops", "/ssh"},
			Actions: []CardAction{
				{Type: "COPY", Target: "ssh prod-bastion"},
			},
		}

		if err := ValidateAndNormalizeCard(&card, "DevOps Tools"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if card.Title != "Production Bastion" {
			t.Errorf("got title %q, want %q", card.Title, "Production Bastion")
		}
		if card.Collection != "devops-tools" {
			t.Errorf("got collection %q, want %q", card.Collection, "devops-tools")
		}
		if card.Template != TemplateKnowledge {
			t.Errorf("got template %q, want %q", card.Template, TemplateKnowledge)
		}
		if card.ID == "" {
			t.Error("expected auto-generated ID")
		}
		if len(card.Tags) != 2 || card.Tags[0] != "prod" || card.Tags[1] != "ssh" {
			t.Errorf("unexpected tags: %v", card.Tags)
		}
		if len(card.Modifiers) != 2 || card.Modifiers[0] != "ssh" || card.Modifiers[1] != "ops" {
			t.Errorf("unexpected modifiers: %v", card.Modifiers)
		}
		if card.Actions[0].Label != "Copy to Clipboard" || card.Actions[0].ID == "" {
			t.Errorf("expected normalized action label and ID, got %+v", card.Actions[0])
		}
		if card.UpdatedAt.IsZero() {
			t.Error("expected UpdatedAt to be populated")
		}
	})

	t.Run("rejects shell action type for security", func(t *testing.T) {
		card := SearchCard{
			Title: "Blocked Shell Card",
			Actions: []CardAction{
				{Type: "shell", Target: "rm -rf /"},
			},
		}
		if err := ValidateAndNormalizeCard(&card, "tools"); err == nil {
			t.Fatalf("expected ValidateAndNormalizeCard to reject shell action type")
		}
	})

	t.Run("rejects empty title", func(t *testing.T) {
		card := SearchCard{Collection: "test"}
		if err := ValidateAndNormalizeCard(&card, ""); err == nil {
			t.Fatal("expected error for missing title")
		}
	})

	t.Run("rejects invalid template", func(t *testing.T) {
		card := SearchCard{Title: "Bad", Template: "unknown-widget"}
		if err := ValidateAndNormalizeCard(&card, "test"); err == nil {
			t.Fatal("expected error for invalid template")
		}
	})

	t.Run("rejects invalid action type", func(t *testing.T) {
		card := SearchCard{
			Title: "Bad Action",
			Actions: []CardAction{
				{Type: "rm_rf", Target: "/"},
			},
		}
		if err := ValidateAndNormalizeCard(&card, "test"); err == nil {
			t.Fatal("expected error for invalid action type")
		}
	})
}

func TestContentHashDeterministic(t *testing.T) {
	c1 := SearchCard{
		Title:       "Redis Cache",
		Collection:  "devops",
		Template:    TemplateKeyValue,
		Data:        map[string]any{"host": "10.0.0.5", "port": 6379},
		Description: "Primary cache cluster",
	}
	c2 := c1
	if c1.ContentHash() != c2.ContentHash() {
		t.Fatal("expected identical content hashes for identical cards")
	}
	c2.Description = "Secondary cache cluster"
	if c1.ContentHash() == c2.ContentHash() {
		t.Fatal("expected different content hashes after mutation")
	}

	body := c1.SearchableBody()
	if !strings.Contains(body, "host: 10.0.0.5") || !strings.Contains(body, "Primary cache cluster") {
		t.Errorf("unexpected searchable body: %q", body)
	}
}

func TestCardJSONSchemaIsValidJSON(t *testing.T) {
	schema := CardJSONSchema()
	if !json.Valid([]byte(schema)) {
		t.Fatal("CardJSONSchema() returned invalid JSON")
	}
}
