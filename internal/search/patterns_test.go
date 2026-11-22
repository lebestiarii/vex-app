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

func TestPatternEngineIATAAWBAndISO6346(t *testing.T) {
	eng := NewPatternEngine(nil)
	ctx := context.Background()

	// 1. Valid IATA AWB: 1234567 % 7 == 5 -> Emirates SkyCargo (176)
	awb := eng.Match(ctx, "176-12345675", nil, nil)
	if awb == nil {
		t.Fatalf("expected IATA AWB match for 176-12345675")
	}
	if awb.Card.Data["carrier"] != "Emirates SkyCargo (EK)" {
		t.Fatalf("unexpected carrier: %+v", awb.Card.Data)
	}
	if !strings.Contains(fmt.Sprintf("%v", awb.Card.Data["check_digit"]), "Valid") {
		t.Fatalf("expected Valid Mod-7 check digit, got %v", awb.Card.Data["check_digit"])
	}

	// 2. Invalid IATA AWB check digit
	awbBad := eng.Match(ctx, "176-12345674", nil, nil)
	if awbBad == nil || !strings.Contains(fmt.Sprintf("%v", awbBad.Card.Data["check_digit"]), "Invalid") {
		t.Fatalf("expected Invalid Mod-7 check digit, got %+v", awbBad)
	}

	// 3. Valid ISO 6346 container (MSCU123456 + official check digit)
	check := computeISO6346CheckDigit("MSCU123456")
	validContainer := fmt.Sprintf("MSCU123456%d", check)
	ctr := eng.Match(ctx, validContainer, nil, nil)
	if ctr == nil {
		t.Fatalf("expected ISO 6346 match for %s", validContainer)
	}
	if ctr.Card.Data["carrier_owner"] != "MSC Mediterranean Shipping" {
		t.Fatalf("unexpected container owner: %+v", ctr.Card.Data)
	}
	if !strings.Contains(fmt.Sprintf("%v", ctr.Card.Data["check_digit"]), "Valid") {
		t.Fatalf("expected Valid ISO 6346 check digit, got %v", ctr.Card.Data["check_digit"])
	}

	// 4. Invalid ISO 6346 check digit
	badContainer := fmt.Sprintf("MSCU123456%d", (check+1)%10)
	ctrBad := eng.Match(ctx, badContainer, nil, nil)
	if ctrBad == nil || !strings.Contains(fmt.Sprintf("%v", ctrBad.Card.Data["check_digit"]), "Invalid") {
		t.Fatalf("expected Invalid ISO 6346 check digit for %s, got %+v", badContainer, ctrBad)
	}
}

func TestPatternEngineCustomHouseBillAndEntityEnrichment(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	compiled := indexer.CompileActiveModules(indexer.BuiltInModules(), []string{"geographies"})
	if err := store.ReplaceSourceFile(ctx, indexer.CompiledModulesSource, compiled); err != nil {
		t.Fatalf("ReplaceSourceFile: %v", err)
	}

	svc := NewService(store)
	svc.SetPatternRules([]models.PatternRule{
		{
			ID:       "house-bill-10",
			Name:     "10-Digit Shipment Reference",
			Category: "logistics",
			Regex:    `^(?P<mode>[0-9])(?P<branch>[0-9]{2})(?P<seq>[0-9]{7})$`,
			Lookups: map[string]map[string]string{
				"mode": {
					"7": "Ocean Import",
					"8": "Ocean Export",
				},
				"branch": {
					"03": "Seattle (SEA)",
				},
				"branch_entity": {
					"03": "loc:SEA",
				},
			},
			TitleTemplate:    "{{lookup.mode}} · {{match}}",
			SubtitleTemplate: "Origin Branch: {{lookup.branch}}",
			Collection:       "ocean-freight",
			Fields: map[string]string{
				"shipment_type": "{{lookup.mode}}",
				"origin_branch": "{{lookup.branch}}",
				"sequence":      "{{seq}}",
			},
			GlancePills:    []string{"{{lookup.mode}}", "{{lookup.branch}}"},
			LinkEntityKeys: []string{"{{lookup.branch_entity}}"},
			LocalOnly:      true,
		},
	})

	resp, err := svc.Search(ctx, "7031234567", 10)
	if err != nil {
		t.Fatalf("Search 7031234567: %v", err)
	}
	if len(resp.Results) < 2 {
		t.Fatalf("expected decoded card + enriched SEA branch card, got %d results", len(resp.Results))
	}

	hero := resp.Results[0]
	if hero.MatchSource != "pattern" || hero.Card.Data["shipment_type"] != "Ocean Import" || hero.Card.Data["origin_branch"] != "Seattle (SEA)" {
		t.Fatalf("unexpected decoded hero card: %+v", hero)
	}
	if !hero.Card.LocalOnly {
		t.Fatalf("expected custom rule LocalOnly=true to propagate to synthesized card")
	}
	if len(hero.Card.Sections) == 0 || !strings.Contains(hero.Card.Sections[0].Title, "Seattle") {
		t.Fatalf("expected linked Seattle section on decoded card, got %+v", hero.Card.Sections)
	}

	runnerUp := resp.Results[1]
	if runnerUp.MatchSource != "pattern-enrichment" || runnerUp.Card.EntityKey != "loc:SEA" {
		t.Fatalf("expected enriched loc:SEA runner-up card, got %+v", runnerUp)
	}
}

func TestPatternEngineLiveMCPLookupAndOfflineFallback(t *testing.T) {
	eng := NewPatternEngine(nil)
	ctx := context.Background()

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
	if len(live.Card.Actions) == 0 || live.Card.Actions[0].Type != "open_url" {
		t.Fatalf("expected Open in Browser primary action from live URL, got %+v", live.Card.Actions)
	}
}
