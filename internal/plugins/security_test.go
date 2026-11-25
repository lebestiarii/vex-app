package plugins

import (
	"context"
	"testing"

	"vex/internal/models"
	"vex/internal/search"
)

func TestOpenURLSecurityValidation(t *testing.T) {
	runner := NewRunner()
	called := false
	runner.OpenFunc = func(_ context.Context, _, _ string) error {
		called = true
		return nil
	}

	unsafeTargets := []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"https://example.com & calc.exe",
		"https://example.com|whoami",
		"ms-msdt:/id PCWDiagnostic",
	}

	for _, target := range unsafeTargets {
		called = false
		card := models.SearchCard{
			ID:         "sec-test",
			Title:      "Test",
			Collection: "test",
			Layout:     models.LayoutLink,
			Actions:    []models.CardAction{{ID: "open", Label: "Open", Type: "open_url", Target: target}},
		}
		res := runner.Execute(context.Background(), card, "open", search.ParsedQuery{}, nil)
		if res.Success || called {
			t.Fatalf("expected unsafe URL %q to be blocked, got success=%v called=%v", target, res.Success, called)
		}
	}

	// Valid HTTPS URL should succeed
	called = false
	validCard := models.SearchCard{
		ID:         "sec-ok",
		Title:      "Valid",
		Collection: "test",
		Layout:     models.LayoutLink,
		Data:       map[string]any{"url": "https://hts.usitc.gov/search?query=8471"},
		Actions:    []models.CardAction{{ID: "open", Label: "Open", Type: "open_url", Target: "https://hts.usitc.gov/search?query=8471"}},
	}
	res := runner.Execute(context.Background(), validCard, "open", search.ParsedQuery{}, nil)
	if !res.Success || !called {
		t.Fatalf("expected valid HTTPS URL to succeed, got %+v", res)
	}
}
