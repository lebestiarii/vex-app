package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateAndNormalizeCard_All9Layouts(t *testing.T) {
	validCards := []SearchCard{
		{
			ID:         "grp-1",
			EntityKey:  "loc:AMS",
			Title:      "Amsterdam Gateway (AMS)",
			Collection: "locations",
			Layout:     LayoutGroup,
			Chips: []GlanceChip{
				{Label: "NLAMS · AMS", Tone: "mono"},
				{Label: "Europe/Amsterdam", Tone: "neutral"},
			},
			Data: map[string]any{
				"meta": map[string]any{
					"ianaTimezone": "Europe/Amsterdam",
					"lat":          52.3105,
					"lon":          4.7683,
				},
			},
		},
		{
			ID:         "lnk-1",
			Title:      "CBP ACE Portal",
			Collection: "links",
			Layout:     LayoutLink,
			Data:       map[string]any{"url": "https://ace.cbp.dhs.gov"},
		},
		{
			ID:         "act-1",
			Title:      "AMS EDI Servers",
			Collection: "servers",
			Layout:     LayoutActionList,
			Data: map[string]any{
				"rows": []map[string]any{
					{
						"badge":    map[string]any{"label": "PROD", "tone": "red", "dot": "red"},
						"primary":  "ams-prd-edi01.internal",
						"mono":     true,
						"subtitle": "Primary broker",
						"actions": []map[string]any{
							{"label": "Copy", "type": "copy", "value": "ams-prd-edi01.internal"},
							{"label": "Open", "type": "open_url", "value": "https://edi.ams.internal"},
						},
					},
				},
			},
		},
		{
			ID:         "fct-1",
			Title:      "Unifeeder (UNFE)",
			Collection: "ocean-carriers",
			Layout:     LayoutFacts,
			Chips: []GlanceChip{
				{Label: "SCAC UNFE", Tone: "amber"},
				{Label: "UNFU", Tone: "mono"}, // Distinct secondary BIC code must be preserved
			},
			Data: map[string]any{
				"summary": "European feeder line",
				"facts": []map[string]any{
					{"label": "SCAC", "value": "UNFE", "mono": true},
					{"label": "BIC Prefixes", "value": "UNFU, UNFE", "mono": true, "copyable": true},
				},
			},
		},
		{
			ID:         "seg-1",
			Title:      "House Bill · 7031234567",
			Collection: "patterns",
			Layout:     LayoutSegments,
			Data: map[string]any{
				"segments": []map[string]any{
					{"value": "7", "role": "MODE", "meaning": "Ocean Import", "tone": "blue"},
					{"value": "03", "role": "BRANCH", "meaning": "Seattle SEA", "tone": "amber"},
					{"value": "1234567", "role": "SEQUENCE", "meaning": "Shipment #", "tone": "green"},
				},
				"explanation": "Forwarder-issued House Bill of Lading.",
			},
		},
		{
			ID:         "stp-1",
			Title:      "What is a House Bill (HBL)?",
			Collection: "dictionary",
			Layout:     LayoutSteps,
			Data: map[string]any{
				"steps": []map[string]any{
					{"label": "Booking", "caption": "Space reserved"},
					{"label": "Origin HBL", "caption": "Receipt issued"},
				},
				"activeStep": 2,
				"summary":    "Issued by forwarder to shipper.",
				"bullets":    []string{"Receipt of goods", "Contract of carriage"},
			},
		},
		{
			ID:         "cnv-1",
			Title:      "100 kg = 220.5 lbs",
			Collection: "system",
			Layout:     LayoutConvert,
			Data: map[string]any{
				"input":  "100 kg",
				"output": "220.5 lbs",
				"equivalents": []map[string]any{
					{"label": "Metric Tons", "value": "0.1 t"},
				},
			},
		},
		{
			ID:         "tbl-1",
			Title:      "Container Comparison",
			Collection: "equipment",
			Layout:     LayoutTable,
			Data: map[string]any{
				"columns":     []string{"Code", "Volume", "Payload"},
				"rows":        [][]string{{"20GP", "33.2 CBM", "28.2t"}, {"40HC", "76.4 CBM", "26.5t"}},
				"monoColumns": []int{0},
				"sourceUrl":   "https://iso.org/6346",
			},
		},
		{
			ID:         "snp-1",
			Title:      "SQL Shipment Lookup",
			Collection: "queries",
			Layout:     LayoutSnippet,
			Data: map[string]any{
				"language": "sql",
				"body":     "SELECT * FROM shipments WHERE hbl = '{{hbl}}';",
				"vars": []map[string]any{
					{"key": "hbl", "label": "House Bill", "defaultValue": "7031234567"},
				},
			},
		},
	}

	for _, c := range validCards {
		card := c
		if err := ValidateAndNormalizeCard(&card, "default"); err != nil {
			t.Errorf("expected layout %q card %q to pass validation, got: %v", card.Layout, card.ID, err)
		}
	}
}

func TestValidateAndNormalizeCard_NormalizesMetadata(t *testing.T) {
	card := SearchCard{
		Title:     "  Production Bastion  ",
		Subtitle:  "SSH jump host",
		Layout:    LayoutSnippet,
		Tags:      []string{"#Prod", "ssh", "PROD", "  "},
		Modifiers: []string{"/SSH", "ops", "/ssh"},
		Data:      map[string]any{"language": "bash", "body": "ssh ops@prod-bastion"},
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
}

func TestStrictValidatorRejections(t *testing.T) {
	t.Run("rejects all 16 legacy template names and aliases", func(t *testing.T) {
		legacyNames := []string{
			"location", "server-matrix", "website-link", "pattern-breakdown",
			"conversion", "kv-grid", "container-blueprint", "dictionary-guide",
			"branch-contact", "branch-gateway", "env-bookmarks", "reference-decoder",
			"shortcut", "knowledge", "command", "metric-card",
		}
		for _, legacy := range legacyNames {
			c := SearchCard{
				Title:      "Legacy Card",
				Collection: "test",
				Layout:     CardLayoutType(legacy),
				Data:       map[string]any{"summary": "x"},
			}
			if err := ValidateAndNormalizeCard(&c, "test"); err == nil {
				t.Errorf("expected legacy layout %q to be rejected", legacy)
			}
		}
	})

	t.Run("rejects unknown payload keys (additionalProperties: false)", func(t *testing.T) {
		c := SearchCard{
			Title:      "Bad Link",
			Collection: "test",
			Layout:     LayoutLink,
			Data: map[string]any{
				"url":     "https://example.com",
				"prodUrl": "https://alias.example.com",
			},
		}
		if err := ValidateAndNormalizeCard(&c, "test"); err == nil {
			t.Fatal("expected unknown key 'prodUrl' on link layout to be rejected")
		}
	})

	t.Run("rejects group cards with body fields", func(t *testing.T) {
		c := SearchCard{
			Title:      "Amsterdam Gateway (AMS)",
			Collection: "locations",
			Layout:     LayoutGroup,
			Data: map[string]any{
				"facts": []map[string]any{{"label": "Code", "value": "AMS"}},
			},
		}
		if err := ValidateAndNormalizeCard(&c, "locations"); err == nil {
			t.Fatal("expected group card with body fields to be rejected")
		}
	})

	t.Run("rejects table with fewer than 2 or more than 4 columns", func(t *testing.T) {
		tooFew := SearchCard{
			Title:      "One Col",
			Collection: "test",
			Layout:     LayoutTable,
			Data:       map[string]any{"columns": []string{"Only"}, "rows": [][]string{{"a"}}},
		}
		if err := ValidateAndNormalizeCard(&tooFew, "test"); err == nil {
			t.Fatal("expected 1-column table to be rejected")
		}

		tooMany := SearchCard{
			Title:      "Five Cols",
			Collection: "test",
			Layout:     LayoutTable,
			Data: map[string]any{
				"columns": []string{"A", "B", "C", "D", "E"},
				"rows":    [][]string{{"1", "2", "3", "4", "5"}},
			},
		}
		if err := ValidateAndNormalizeCard(&tooMany, "test"); err == nil {
			t.Fatal("expected 5-column table to be rejected")
		}
	})

	t.Run("rejects more than 4 chips and duplicate chips", func(t *testing.T) {
		fiveChips := SearchCard{
			Title:      "Too Many Chips",
			Collection: "test",
			Layout:     LayoutGroup,
			Chips: []GlanceChip{
				{Label: "A", Tone: "neutral"},
				{Label: "B", Tone: "neutral"},
				{Label: "C", Tone: "neutral"},
				{Label: "D", Tone: "neutral"},
				{Label: "E", Tone: "neutral"},
			},
		}
		if err := ValidateAndNormalizeCard(&fiveChips, "test"); err == nil {
			t.Fatal("expected >4 chips to be rejected")
		}

		dupChips := SearchCard{
			Title:      "Dup Chips",
			Collection: "test",
			Layout:     LayoutGroup,
			Chips: []GlanceChip{
				{Label: "Alpha", Tone: "neutral"},
				{Label: "alpha", Tone: "blue"},
			},
		}
		if err := ValidateAndNormalizeCard(&dupChips, "test"); err == nil {
			t.Fatal("expected duplicate chip label to be rejected")
		}
	})

	t.Run("rejects exact-token echo between title and standalone chip while allowing distinct secondary code", func(t *testing.T) {
		echoCard := SearchCard{
			Title:      "Bahri Logistics (BAXU)",
			Collection: "ocean-carriers",
			Layout:     LayoutFacts,
			Chips: []GlanceChip{
				{Label: "SCAC BAXU", Tone: "amber"},
				{Label: "BAXU", Tone: "mono"},
			},
			Data: map[string]any{"summary": "Bahri"},
		}
		if err := ValidateAndNormalizeCard(&echoCard, "ocean-carriers"); err == nil {
			t.Fatal("expected standalone BAXU chip echoing title/SCAC chip to be rejected")
		}

		distinctCard := SearchCard{
			Title:      "Unifeeder (UNFE)",
			Collection: "ocean-carriers",
			Layout:     LayoutFacts,
			Chips: []GlanceChip{
				{Label: "SCAC UNFE", Tone: "amber"},
				{Label: "UNFU", Tone: "mono"},
			},
			Data: map[string]any{"summary": "Unifeeder"},
		}
		if err := ValidateAndNormalizeCard(&distinctCard, "ocean-carriers"); err != nil {
			t.Fatalf("expected distinct secondary code UNFU to be allowed, got: %v", err)
		}
	})

	t.Run("rejects inline nesting depth > 2 and flat parentEntityKey chain depth > 2 or cycles", func(t *testing.T) {
		deepInline := SearchCard{
			Title:      "L0",
			Collection: "test",
			Layout:     LayoutGroup,
			Children: []SearchCard{
				{
					Title:  "L1",
					Layout: LayoutGroup,
					Children: []SearchCard{
						{
							Title:  "L2",
							Layout: LayoutGroup,
							Children: []SearchCard{
								{Title: "L3", Layout: LayoutFacts, Data: map[string]any{"summary": "too deep"}},
							},
						},
					},
				},
			},
		}
		if err := ValidateAndNormalizeCard(&deepInline, "test"); err == nil {
			t.Fatal("expected depth-3 inline child to be rejected")
		}

		deepFlat := ModuleManifest{
			ID:   "deep-mod",
			Name: "Deep Flat Module",
			Cards: []SearchCard{
				{ID: "c0", EntityKey: "e:0", Title: "Root", Collection: "test", Layout: LayoutGroup},
				{ID: "c1", EntityKey: "e:1", ParentEntityKey: "e:0", Title: "Level 1", Collection: "test", Layout: LayoutGroup},
				{ID: "c2", EntityKey: "e:2", ParentEntityKey: "e:1", Title: "Level 2", Collection: "test", Layout: LayoutGroup},
				{ID: "c3", EntityKey: "e:3", ParentEntityKey: "e:2", Title: "Level 3", Collection: "test", Layout: LayoutFacts, Data: map[string]any{"summary": "too deep"}},
			},
		}
		if res := ValidateModuleManifest(&deepFlat); res.Valid {
			t.Fatal("expected depth-3 flat parentEntityKey chain to be rejected")
		}

		cyclic := ModuleManifest{
			ID:   "cycle-mod",
			Name: "Cyclic Module",
			Cards: []SearchCard{
				{ID: "a", EntityKey: "e:a", ParentEntityKey: "e:b", Title: "A", Collection: "test", Layout: LayoutGroup},
				{ID: "b", EntityKey: "e:b", ParentEntityKey: "e:a", Title: "B", Collection: "test", Layout: LayoutGroup},
			},
		}
		if res := ValidateModuleManifest(&cyclic); res.Valid {
			t.Fatal("expected cyclic parentEntityKey module to be rejected")
		}
	})

	t.Run("rejects shell and ai_prompt action types and legacy template JSON key", func(t *testing.T) {
		for _, badAct := range []string{"shell", "ai_prompt", "rm_rf"} {
			c := SearchCard{
				Title:      "Bad Action",
				Collection: "test",
				Layout:     LayoutGroup,
				Actions:    []CardAction{{Type: badAct, Target: "x"}},
			}
			if err := ValidateAndNormalizeCard(&c, "test"); err == nil {
				t.Errorf("expected action type %q to be rejected", badAct)
			}
		}

		legacyJSON := `{"id":"bad","name":"Bad","cards":[{"title":"Legacy","collection":"test","template":"kv-grid","data":{"a":"b"}}]}`
		if _, res := ValidateModuleJSON([]byte(legacyJSON)); res.Valid {
			t.Fatal("expected ValidateModuleJSON to reject legacy 'template' field")
		}
	})
}

func TestSearchableBodyLayoutAwareWhitelist(t *testing.T) {
	groupCard := SearchCard{
		ID:          "loc-ams",
		EntityKey:   "loc:AMS",
		Title:       "Amsterdam Gateway (AMS)",
		Description: "Primary European hub.",
		Collection:  "locations",
		Layout:      LayoutGroup,
		Chips:       []GlanceChip{{Label: "NLAMS · AMS", Tone: "mono"}, {Label: "EUR €", Tone: "neutral"}},
		Data: map[string]any{
			"meta": map[string]any{"ianaTimezone": "Europe/Amsterdam", "lat": 52.31, "lon": 4.76},
		},
		Actions: []CardAction{
			{ID: "open", Label: "Open Gateway Portal", Type: "open_url", Target: "https://portal.example/ams-hidden"},
			{ID: "copy", Label: "Copy UNLOCODE", Type: "copy", Target: "NLAMS"},
		},
	}
	if got := groupCard.ChipsText(); got != "NLAMS · AMS EUR €" {
		t.Fatalf("ChipsText() = %q, want %q", got, "NLAMS · AMS EUR €")
	}
	body := groupCard.SearchableBody()
	for _, want := range []string{"Primary European hub.", "Open Gateway Portal", "Copy UNLOCODE", "NLAMS"} {
		if !strings.Contains(body, want) {
			t.Errorf("group SearchableBody missing %q in %q", want, body)
		}
	}
	for _, unwanted := range []string{"Europe/Amsterdam", "52.31", "ams-hidden"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("group SearchableBody should exclude %q, got %q", unwanted, body)
		}
	}

	linkCard := SearchCard{
		Title:      "CDN Static Assets",
		Collection: "infra",
		Layout:     LayoutLink,
		Data:       map[string]any{"url": "https://example.test/ams-assets"},
	}
	if strings.Contains(linkCard.SearchableBody(), "ams-assets") {
		t.Fatalf("link SearchableBody must exclude URL, got %q", linkCard.SearchableBody())
	}

	actionList := SearchCard{
		Title:      "EDI Hosts",
		Collection: "servers",
		Layout:     LayoutActionList,
		Data: map[string]any{
			"rows": []map[string]any{
				{
					"badge":    map[string]any{"label": "PROD", "tone": "red", "dot": "red"},
					"primary":  "ams-edi01.internal",
					"subtitle": "Primary broker",
					"actions": []map[string]any{
						{"label": "SSH", "type": "copy", "value": "ssh ops@ams-edi01"},
						{"label": "Console", "type": "open_url", "value": "https://hidden-console.internal/secret-slug"},
					},
				},
			},
		},
	}
	actBody := actionList.SearchableBody()
	for _, want := range []string{"PROD", "ams-edi01.internal", "Primary broker", "ssh ops@ams-edi01"} {
		if !strings.Contains(actBody, want) {
			t.Errorf("action-list SearchableBody missing %q in %q", want, actBody)
		}
	}
	if strings.Contains(actBody, "secret-slug") {
		t.Errorf("action-list SearchableBody should exclude open_url values, got %q", actBody)
	}

	tableCard := SearchCard{
		Title:      "Specs",
		Collection: "docs",
		Layout:     LayoutTable,
		Data: map[string]any{
			"columns":     []string{"Code", "Volume"},
			"rows":        [][]string{{"40HC", "76.4 CBM"}},
			"monoColumns": []int{0},
			"sourceUrl":   "https://iso.example/hidden-table-url",
		},
	}
	tblBody := tableCard.SearchableBody()
	if !strings.Contains(tblBody, "40HC") || !strings.Contains(tblBody, "76.4 CBM") {
		t.Errorf("table SearchableBody missing cells: %q", tblBody)
	}
	if strings.Contains(tblBody, "hidden-table-url") {
		t.Errorf("table SearchableBody must exclude sourceUrl: %q", tblBody)
	}
}

func TestContentHashDeterministicAndJSONSchemaValid(t *testing.T) {
	c1 := SearchCard{
		Title:       "Redis Cache",
		Collection:  "devops",
		Layout:      LayoutFacts,
		Data:        map[string]any{"facts": []map[string]any{{"label": "Host", "value": "10.0.0.5"}}},
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

	schema := CardJSONSchema()
	if !json.Valid([]byte(schema)) {
		t.Fatal("CardJSONSchema() returned invalid JSON")
	}
	for _, def := range []string{
		"GroupLayoutData", "LinkLayoutData", "ActionListLayoutData",
		"FactsLayoutData", "SegmentsLayoutData", "StepsLayoutData",
		"ConvertLayoutData", "TableLayoutData", "SnippetLayoutData",
		"PatternLiveHTTP", "PatternLiveResultSpec",
		"parentEntityKey",
	} {
		if !strings.Contains(schema, def) {
			t.Errorf("CardJSONSchema() missing %q", def)
		}
	}
}

func TestValidatePatternLiveHTTP(t *testing.T) {
	baseRule := func(live *PatternLiveHTTP) PatternRule {
		return PatternRule{
			ID:            "gh-issue",
			Name:          "GitHub Issue",
			Regex:         `^(?P<owner>[A-Za-z0-9_.-]+)/(?P<repo>[A-Za-z0-9_.-]+)#(?P<num>\d+)$`,
			TitleTemplate: "GitHub {{owner}}/{{repo}}#{{num}}",
			Layout:        LayoutSegments,
			LiveHTTP:      live,
		}
	}

	t.Run("accepts valid facts, link, table, and steps resultCards with allowed auth modes", func(t *testing.T) {
		cases := []PatternLiveHTTP{
			{
				URLTemplate: "https://api.github.com/repos/{{owner}}/{{repo}}/issues/{{num}}",
				Trigger:     "manual",
				TimeoutMs:   1500,
				CacheTTLSec: 60,
				Auth:        "gh_cli",
				Headers:     map[string]string{"Accept": "application/vnd.github+json"},
				ResultCard: PatternLiveResultSpec{
					Layout:  LayoutFacts,
					Title:   "Issue #{{num}}",
					Summary: "{{json.title}}",
					Facts: []FactItem{
						{Label: "State", Value: "{{json.state}}"},
						{Label: "Author", Value: "{{json.user.login}}"},
					},
				},
			},
			{
				URLTemplate: "https://dev.azure.com/org/proj/_apis/wit/workitems/{{num}}?api-version=7.1",
				Trigger:     "auto",
				TimeoutMs:   3000,
				CacheTTLSec: 120,
				Auth:        "az_devops",
				ResultCard: PatternLiveResultSpec{
					Layout: LayoutLink,
					Title:  "Open Work Item",
					URL:    "{{json._links.html.href}}",
				},
			},
			{
				URLTemplate: "https://api.example.com/shipments/{{0}}",
				Auth:        "env:TRACKING_API_TOKEN",
				ResultCard: PatternLiveResultSpec{
					Layout:    LayoutTable,
					Title:     "Tracking Events",
					Columns:   []string{"Time", "Location", "Status"},
					RowsPath:  "events",
					RowFields: []string{"timestamp", "port", "status"},
				},
			},
			{
				URLTemplate: "https://api.example.com/milestones/{{0}}",
				Auth:        "none",
				ResultCard: PatternLiveResultSpec{
					Layout:     LayoutSteps,
					Title:      "Shipment Milestones",
					Summary:    "Status: {{json.status}}",
					StepsPath:  "milestones",
					StepLabel:  "name",
					ActiveStep: 1,
				},
			},
		}

		for i := range cases {
			r := baseRule(&cases[i])
			if errs := ValidatePatternRule(&r, ""); len(errs) > 0 {
				t.Errorf("case %d: expected valid liveHttp rule, got errors: %v", i, errs)
			}
		}
	})

	t.Run("rejects inline tokens, auth headers, non-http schemes, embedded userinfo, and out-of-range timeouts", func(t *testing.T) {
		badCases := []struct {
			name string
			live PatternLiveHTTP
		}{
			{
				name: "inline bearer token in auth",
				live: PatternLiveHTTP{
					URLTemplate: "https://api.github.com/repos/{{owner}}/{{repo}}/issues/{{num}}",
					Auth:        "Bearer ghp_1234567890abcdef",
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "lowercase env var name",
				live: PatternLiveHTTP{
					URLTemplate: "https://api.example.com/items/{{0}}",
					Auth:        "env:bad_lower",
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "authorization header with inline secret",
				live: PatternLiveHTTP{
					URLTemplate: "https://api.example.com/items/{{0}}",
					Auth:        "none",
					Headers:     map[string]string{"Authorization": "Bearer secret"},
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "custom header with inline ghp token",
				live: PatternLiveHTTP{
					URLTemplate: "https://api.example.com/items/{{0}}",
					Headers:     map[string]string{"X-Custom": "ghp_secret123"},
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "file scheme urlTemplate",
				live: PatternLiveHTTP{
					URLTemplate: "file:///etc/passwd",
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "embedded URL credentials",
				live: PatternLiveHTTP{
					URLTemplate: "https://user:pass@api.example.com/items/{{0}}",
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "timeout above 5000ms",
				live: PatternLiveHTTP{
					URLTemplate: "https://api.example.com/items/{{0}}",
					TimeoutMs:   8000,
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "invalid trigger",
				live: PatternLiveHTTP{
					URLTemplate: "https://api.example.com/items/{{0}}",
					Trigger:     "always",
					ResultCard:  PatternLiveResultSpec{Layout: LayoutFacts, Summary: "{{json.title}}"},
				},
			},
			{
				name: "unsupported resultCard layout",
				live: PatternLiveHTTP{
					URLTemplate: "https://api.example.com/items/{{0}}",
					ResultCard:  PatternLiveResultSpec{Layout: LayoutSnippet},
				},
			},
		}

		for _, tc := range badCases {
			r := baseRule(&tc.live)
			if errs := ValidatePatternRule(&r, ""); len(errs) == 0 {
				t.Errorf("%s: expected validation error, got none", tc.name)
			}
		}
	})
}
