package search

import (
	"strings"
	"testing"
)

func TestTryConvertQueryUnits(t *testing.T) {
	cases := []struct {
		query      string
		wantSubstr string
	}{
		{"1588 kg to lbs", "3500.9 lbs"},
		{"3500 lbs in kg", "1587.6 kg"},
		{"28.2 mt to lbs", "lbs"},
		{"125 in to cm", "317.5 cm"},
		{"40 ft to m", "12.19 m"},
		{"4.3 cbm to ft3", "151.9 ft³"},
		{"-30c to f", "-22 °F"},
		{"32f to c", "0 °C"},
	}

	for _, tc := range cases {
		card := TryConvertQuery(tc.query)
		if card == nil {
			t.Fatalf("TryConvertQuery(%q) returned nil, expected match", tc.query)
		}
		if card.ID != "builtin-unit-convert" {
			t.Fatalf("expected ID builtin-unit-convert, got %q", card.ID)
		}
		if !strings.Contains(card.Title, tc.wantSubstr) {
			t.Fatalf("TryConvertQuery(%q): title %q does not contain %q", tc.query, card.Title, tc.wantSubstr)
		}
		if len(card.Actions) == 0 || card.Actions[0].Type != "copy" {
			t.Fatalf("TryConvertQuery(%q): expected copy primary action, got %+v", tc.query, card.Actions)
		}
	}
}

func TestTryConvertQueryDimensionalBoxes(t *testing.T) {
	card := TryConvertQuery("120x80x100 cm")
	if card == nil {
		t.Fatalf("expected dimensional box conversion for '120x80x100 cm'")
	}
	if !strings.Contains(card.Title, "0.96 CBM") {
		t.Fatalf("expected 0.96 CBM in title, got %q", card.Title)
	}
	if !strings.Contains(card.Title, "160.0 kg") {
		t.Fatalf("expected ~160.0 kg IATA air volumetric weight in title, got %q", card.Title)
	}

	multi := TryConvertQuery("2 x 120x80x100 cm")
	if multi == nil || !strings.Contains(multi.Title, "1.92 CBM") {
		t.Fatalf("expected 1.92 CBM for 2-piece box, got %+v", multi)
	}

	imperial := TryConvertQuery("dim 48x40x60 in")
	if imperial == nil || !strings.Contains(imperial.Title, "1.89 CBM") {
		t.Fatalf("expected ~1.89 CBM for 48x40x60 in, got %+v", imperial)
	}

	if got := TryConvertQuery("AMS"); got != nil {
		t.Fatalf("expected nil for non-conversion query 'AMS', got %+v", got)
	}
	if got := TryConvertQuery("10 kg to cm"); got != nil {
		t.Fatalf("expected nil for mismatched unit categories, got %+v", got)
	}
}
