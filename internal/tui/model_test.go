package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"vex/internal/indexer"
	"vex/internal/plugins"
	"vex/internal/search"
)

func newTestTUIModel(t *testing.T) (Model, *string) {
	t.Helper()
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	compiled := indexer.CompileActiveModules(indexer.BuiltInModules(), nil)
	if err := store.ReplaceSourceFile(ctx, indexer.CompiledModulesSource, compiled); err != nil {
		t.Fatalf("ReplaceSourceFile: %v", err)
	}

	searchSvc := search.NewService(store)
	var copied string
	runner := plugins.NewRunner()
	runner.ClipboardFunc = func(_ context.Context, text string) error {
		copied = text
		return nil
	}
	runner.OpenFunc = func(_ context.Context, _, target string) error {
		copied = target
		return nil
	}

	m := NewModel(Config{
		AppName:          "Vex",
		Edition:          "community",
		Version:          "0.1.0",
		PinnedCategories: []string{"ports", "air-freight", "customs"},
		Search:           searchSvc,
		Store:            store,
		Runner:           runner,
	})
	return m, &copied
}

func sendText(m Model, text string) Model {
	for _, r := range text {
		var msg tea.KeyMsg
		if r == ' ' {
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		} else {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func sendKey(m Model, keyType tea.KeyType) Model {
	next, _ := m.Update(tea.KeyMsg{Type: keyType})
	return next.(Model)
}

func TestTUIHeroRunnerUpsAndPerspectiveTabs(t *testing.T) {
	m, _ := newTestTUIModel(t)

	// 1. Typing "NLAMS" renders Primary Hero Card (port-nlams) + Compact Runner-Ups
	m = sendText(m, "NLAMS")
	if m.Query() != "NLAMS" {
		t.Fatalf("expected query 'NLAMS', got %q", m.Query())
	}
	hero := m.HeroCard()
	if hero == nil || hero.ID != "port-nlams" {
		t.Fatalf("expected port-nlams as primary hero card, got %+v", hero)
	}
	if m.ActiveTab() != "base" {
		t.Fatalf("expected 'base' Overview tab for bare NLAMS query, got %q", m.ActiveTab())
	}
	runners := m.RunnerUpCards()
	if len(runners) == 0 || len(runners) > 3 {
		t.Fatalf("expected 1..3 compact runner-ups, got %d", len(runners))
	}

	// 2. Tab / Shift+Tab cycles through module perspective tabs on the Hero Card
	m = sendKey(m, tea.KeyTab)
	if m.ActiveTab() == "base" {
		t.Fatalf("expected Tab to switch from 'base' to first module section tab")
	}
	firstSectionTab := m.ActiveTab()
	m = sendKey(m, tea.KeyShiftTab)
	if m.ActiveTab() != "base" {
		t.Fatalf("expected Shift+Tab to return to 'base', got %q (from %q)", m.ActiveTab(), firstSectionTab)
	}

	// 3. Multi-token query "NLAMS customs" auto-selects the "customs" perspective tab
	m = sendText(m, " customs")
	if m.ActiveTab() != "customs" {
		t.Fatalf("expected 'NLAMS customs' to auto-select 'customs' perspective tab, got %q", m.ActiveTab())
	}
	view := m.View()
	if !strings.Contains(view, "Amsterdam") || !strings.Contains(view, "Source:") {
		t.Fatalf("expected Hero Card title and provenance footer in View(), got:\n%s", view)
	}
}

func TestTUICategoryShortcutsPromotionAndStagedEsc(t *testing.T) {
	m, copied := newTestTUIModel(t)

	// 1. Bare '4' toggles Category 4 ("customs") when query is empty
	m = sendText(m, "4")
	if m.ActiveCategory() != "customs" {
		t.Fatalf("expected category 'customs' after pressing '4', got %q", m.ActiveCategory())
	}

	// 2. Typing query text inside active category searches within @customs
	m = sendText(m, "AMS")
	if m.Query() != "AMS" || m.ActiveCategory() != "customs" {
		t.Fatalf("expected query='AMS' in category='customs', got query=%q cat=%q", m.Query(), m.ActiveCategory())
	}

	// 3. Switch back to '1 All' via Alt+1 so we have multiple runner-ups for "AMS"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}, Alt: true})
	m = next.(Model)
	if m.ActiveCategory() != "" {
		t.Fatalf("expected Alt+1 to reset category to All (''), got %q", m.ActiveCategory())
	}

	origHero := m.HeroCard()
	runners := m.RunnerUpCards()
	if origHero == nil || len(runners) == 0 {
		t.Fatalf("expected hero and runner-ups for 'AMS'")
	}
	firstRunnerID := runners[0].ID

	// 4. Down arrow + Enter promotes Runner-Up 1 into the Primary Hero Card slot
	m = sendKey(m, tea.KeyDown)
	if m.FocusIndex() != 1 {
		t.Fatalf("expected FocusIndex 1 after Down arrow, got %d", m.FocusIndex())
	}
	m = sendKey(m, tea.KeyEnter)
	if m.FocusIndex() != 0 {
		t.Fatalf("expected FocusIndex reset to 0 after promoting runner-up, got %d", m.FocusIndex())
	}
	if m.HeroCard() == nil || m.HeroCard().ID != firstRunnerID {
		t.Fatalf("expected promoted hero card %q, got %+v", firstRunnerID, m.HeroCard())
	}

	// 5. Enter on focused Hero Card executes its primary action
	m = sendKey(m, tea.KeyEnter)
	if m.StatusMessage() == "" || *copied == "" {
		t.Fatalf("expected action execution status & clipboard/open payload, got status=%q copied=%q", m.StatusMessage(), *copied)
	}

	// 6. Activate category 2 via Alt+2 so we can test all 4 stages of Esc
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}, Alt: true})
	m = next.(Model)
	// Re-promote a runner-up if available
	if len(m.RunnerUpCards()) > 0 {
		targetID := m.RunnerUpCards()[0].ID
		m = sendKey(m, tea.KeyDown)
		m = sendKey(m, tea.KeyEnter)
		if m.HeroCard().ID != targetID {
			t.Fatalf("expected promoted hero %q", targetID)
		}
		// Stage 1 Esc: resets promoted runner-up
		m = sendKey(m, tea.KeyEsc)
		if m.HeroCard().ID == targetID {
			t.Fatalf("expected 1st Esc to reset promoted runner-up")
		}
	}

	// Stage 2 Esc: clears active category scope ("ports")
	m = sendKey(m, tea.KeyEsc)
	if m.ActiveCategory() != "" {
		t.Fatalf("expected Esc to clear active category, got %q", m.ActiveCategory())
	}
	if m.Query() != "AMS" {
		t.Fatalf("expected query 'AMS' preserved when clearing category, got %q", m.Query())
	}

	// Stage 3 Esc: clears query text
	m = sendKey(m, tea.KeyEsc)
	if m.Query() != "" {
		t.Fatalf("expected Esc to clear query, got %q", m.Query())
	}
	if m.Quitting() {
		t.Fatalf("expected TUI not to quit on query-clear Esc")
	}

	// Stage 4 Esc: quits TUI when clean
	m = sendKey(m, tea.KeyEsc)
	if !m.Quitting() {
		t.Fatalf("expected final Esc on clean state to quit TUI")
	}
}

func TestTUIInstantUnitAndCargoVolumetricMath(t *testing.T) {
	m, _ := newTestTUIModel(t)

	// Typing "100 kg to lb" rune-by-rune starting with '1' disambiguates from category 1 and triggers converter
	m = sendText(m, "100 kg to lb")
	if m.Query() != "100 kg to lb" {
		t.Fatalf("expected numeric query '100 kg to lb', got %q", m.Query())
	}
	hero := m.HeroCard()
	if hero == nil || hero.ID != "builtin-unit-convert" {
		t.Fatalf("expected builtin-unit-convert hero card for '100 kg to lb', got %+v", hero)
	}
	if !strings.Contains(hero.Title, "220.5") {
		t.Fatalf("expected 220.5 lbs conversion in hero title, got %q", hero.Title)
	}

	// Clear and test 3D cargo volumetric math ("120x80x100 cm")
	m = sendKey(m, tea.KeyEsc)
	m = sendText(m, "120x80x100 cm")
	hero2 := m.HeroCard()
	if hero2 == nil || hero2.ID != "builtin-unit-convert" {
		t.Fatalf("expected builtin-unit-convert hero card for '120x80x100 cm', got %+v", hero2)
	}
	if !strings.Contains(m.View(), "0.96") {
		t.Fatalf("expected volumetric CBM output in View(), got:\n%s", m.View())
	}
}
