package search

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"vex/internal/indexer"
	"vex/internal/models"
)

type stubLivePatternExecutor struct {
	text      string
	localOnly bool
	err       error
	lastSrv   string
	lastTool  string
	lastArgs  map[string]any
}

func (s *stubLivePatternExecutor) ExecutePatternMCP(_ context.Context, serverID, toolName string, args map[string]any) (string, bool, error) {
	s.lastSrv = serverID
	s.lastTool = toolName
	s.lastArgs = args
	return s.text, s.localOnly, s.err
}

func TestPatternEngineModuleDrivenHBLAndMAWBAndISO6346(t *testing.T) {
	modulesDir := t.TempDir()
	_, err := indexer.InstallStarterModules(modulesDir, []string{
		"air-carriers",
		"ocean-carriers",
		"logistics-dictionary",
		"reference-patterns",
	})
	if err != nil {
		t.Fatalf("InstallStarterModules: %v", err)
	}
	// Seed an ephemeral test-only location module inside TempDir to verify cross-module HBL -> Location nesting
	_, err = indexer.SaveCustomModule(modulesDir, models.ModuleManifest{
		ID:             "test-locations",
		Name:           "Test Locations",
		Category:       "Locations",
		Version:        "1.0.0",
		DefaultEnabled: true,
		Cards: []models.SearchCard{
			{
				ID:         "loc-sea",
				EntityKey:  "loc:SEA",
				Title:      "Seattle Gateway (SEA)",
				Collection: "locations",
				Template:   models.TemplateLocation,
				Children: []models.SearchCard{
					{ID: "sea-srv", Title: "SEA EDI Servers", Collection: "locations", Template: models.TemplateServerMatrix},
					{ID: "sea-ace", Title: "CBP ACE Portal", Collection: "locations", Template: models.TemplateWebsiteLink, Data: map[string]any{"url": "https://ace.cbp.dhs.gov"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveModuleFile(test-locations): %v", err)
	}

	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := indexer.SyncModules(ctx, store, modulesDir, nil); err != nil {
		t.Fatalf("SyncModules: %v", err)
	}

	// Empty engine has zero hardcoded rules
	emptyEng := NewPatternEngine(nil)
	if len(emptyEng.Rules()) != 0 {
		t.Fatalf("expected 0 hardcoded rules when no pattern modules loaded, got %d", len(emptyEng.Rules()))
	}

	// Load rules from the installed reference-patterns module
	rules := indexer.LoadActivePatternRules("", modulesDir, nil)
	if len(rules) < 3 {
		t.Fatalf("expected >= 3 rules from reference-patterns module, got %d", len(rules))
	}

	eng := NewPatternEngine(rules)

	// 1. House Bill 7031234567 -> 3-segment pattern-breakdown nesting Seattle Gateway (SEA) auto-expanded with EDI Servers & CBP ACE Portal
	hbl := eng.Match(ctx, "7031234567", store, nil)
	if hbl == nil {
		t.Fatalf("expected HBL match for 7031234567")
	}
	if hbl.Card.Title != "House Bill · 7031234567" || hbl.Card.Template != models.TemplatePatternBreakdown {
		t.Fatalf("unexpected HBL card: title=%q template=%q", hbl.Card.Title, hbl.Card.Template)
	}
	segs, ok := hbl.Card.Data["segments"].([]map[string]string)
	if !ok || len(segs) != 3 || segs[0]["value"] != "7" || segs[1]["value"] != "03" || segs[2]["value"] != "1234567" {
		t.Fatalf("expected 3 decoded segments on HBL card, got %+v", hbl.Card.Data["segments"])
	}
	if len(hbl.Card.Children) != 1 || hbl.Card.Children[0].EntityKey != "loc:SEA" || !hbl.Card.Children[0].DefaultExpanded {
		t.Fatalf("expected auto-expanded loc:SEA child on HBL card, got %+v", hbl.Card.Children)
	}
	seaChildren := hbl.Card.Children[0].Children
	if len(seaChildren) < 2 {
		t.Fatalf("expected >= 2 nested Level-2 children under Seattle Gateway (EDI Servers, CBP ACE Portal), got %d", len(seaChildren))
	}

	// 2. Valid IATA AWB 176-12345675 -> dynamically looks up Emirates from air-carriers module + nests guide:mawb & Airline Tracking Links
	awb := eng.Match(ctx, "176-12345675", store, nil)
	if awb == nil {
		t.Fatalf("expected IATA AWB match for 176-12345675")
	}
	if awb.Card.Title != "Emirates MAWB · 176-12345675" {
		t.Fatalf("unexpected MAWB title: %q", awb.Card.Title)
	}
	if !strings.Contains(fmt.Sprintf("%v", awb.Card.Data["check_digit"]), "Valid") {
		t.Fatalf("expected Valid Mod-7 check digit, got %v", awb.Card.Data["check_digit"])
	}
	if len(awb.Card.Children) < 2 {
		t.Fatalf("expected guide:mawb + Airline Tracking Links children on MAWB card, got %d", len(awb.Card.Children))
	}

	// 3. Invalid IATA AWB check digit flips chip tone to red
	awbBad := eng.Match(ctx, "176-12345674", store, nil)
	if awbBad == nil || !strings.Contains(fmt.Sprintf("%v", awbBad.Card.Data["check_digit"]), "Invalid") {
		t.Fatalf("expected Invalid Mod-7 check digit, got %+v", awbBad)
	}
	if len(awbBad.Card.Chips) < 2 || awbBad.Card.Chips[1].Tone != "red" {
		t.Fatalf("expected Mod-7 chip tone to flip to red on invalid checksum, got %+v", awbBad.Card.Chips)
	}

	// 4. Valid ISO 6346 container (MSCU123456 + official check digit) with space normalization
	check := computeISO6346CheckDigit("MSCU123456")
	validContainer := fmt.Sprintf("mscu 123456-%d", check)
	ctr := eng.Match(ctx, validContainer, store, nil)
	if ctr == nil {
		t.Fatalf("expected ISO 6346 match for %s", validContainer)
	}
	if !strings.Contains(ctr.Card.Title, "MSC") {
		t.Fatalf("expected dynamic ocean-carrier lookup for MSC, got title %q", ctr.Card.Title)
	}
	if !strings.Contains(fmt.Sprintf("%v", ctr.Card.Data["check_digit"]), "Valid") {
		t.Fatalf("expected Valid ISO 6346 check digit, got %v", ctr.Card.Data["check_digit"])
	}
}

func TestPatternEngineLiveMCPLookupAndDisableToggle(t *testing.T) {
	disabled := false
	adoRule := models.PatternRule{
		ID:              "ado-work-item",
		Name:            "Azure DevOps Work Item",
		Regex:           `^(?:AB#|ADO[-#])(?P<id>\d{3,9})$`,
		CaseInsensitive: true,
		TitleTemplate:   "ADO Work Item #{{id}}",
		LiveMCP: &models.PatternMCPLookup{
			ServerID:  "azure-devops",
			ToolName:  "wit_get_work_item",
			Arguments: map[string]string{"id": "{{id}}"},
		},
		LocalOnly: true,
	}

	ctx := context.Background()
	eng := NewPatternEngine([]models.PatternRule{adoRule})

	// 1. Offline fallback (no MCP executor attached)
	offline := eng.Match(ctx, "AB#48219", nil, nil)
	if offline == nil || offline.Card.Title != "ADO Work Item #48219" || !offline.Card.LocalOnly {
		t.Fatalf("unexpected offline ADO match: %+v", offline)
	}
	if !strings.Contains(fmt.Sprintf("%v", offline.Card.Data["live_status"]), "azure-devops") {
		t.Fatalf("expected helpful offline MCP hint, got %+v", offline.Card.Data)
	}

	// 2. Live MCP hit returning structured JSON fields
	stub := &stubLivePatternExecutor{
		text:      `{"title":"Fix Customs EDI Timeout","state":"Active","assignedTo":"Alice","url":"https://dev.azure.com/org/proj/_workitems/edit/48219"}`,
		localOnly: true,
	}
	live := eng.Match(ctx, "AB#48219", nil, stub)
	if live == nil {
		t.Fatalf("expected live ADO match")
	}
	if stub.lastSrv != "azure-devops" || stub.lastTool != "wit_get_work_item" || stub.lastArgs["id"] != "48219" {
		t.Fatalf("unexpected MCP call args: srv=%q tool=%q args=%+v", stub.lastSrv, stub.lastTool, stub.lastArgs)
	}
	if live.Card.Subtitle != "Fix Customs EDI Timeout" || live.Card.Data["state"] != "Active" || !live.Card.LocalOnly {
		t.Fatalf("expected unpacked live ADO template card, got %+v", live.Card)
	}

	// 3. Disabling the rule stops matching while preserving it in Rules()
	adoRule.Enabled = &disabled
	eng.SetCustomRules([]models.PatternRule{adoRule})
	if got := eng.Match(ctx, "AB#48219", nil, stub); got != nil {
		t.Fatalf("expected disabled rule not to match, got %+v", got)
	}
	if len(eng.Rules()) != 1 {
		t.Fatalf("expected disabled rule to remain listed in Rules(), got %d", len(eng.Rules()))
	}
}
