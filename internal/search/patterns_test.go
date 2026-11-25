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
	// Seed an ephemeral flat-on-disk test-only location module inside TempDir to verify cross-module
	// HBL (L0) -> Location (L1) -> Servers & Portal (L2) via entity_links!
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
				Layout:     models.LayoutGroup,
			},
			{
				ID:              "sea-srv",
				ParentEntityKey: "loc:SEA",
				Title:           "SEA EDI Servers",
				Collection:      "locations",
				Layout:          models.LayoutActionList,
				Data: map[string]any{
					"rows": []map[string]any{
						{
							"badge":   map[string]any{"label": "PROD", "tone": "red", "dot": "red"},
							"primary": "sea-prd-edi01.internal",
							"mono":    true,
							"actions": []map[string]any{{"label": "Copy", "type": "copy", "value": "sea-prd-edi01.internal"}},
						},
					},
				},
			},
			{
				ID:              "sea-ace",
				ParentEntityKey: "loc:SEA",
				Title:           "CBP ACE Portal",
				Collection:      "locations",
				Layout:          models.LayoutLink,
				Data:            map[string]any{"url": "https://ace.cbp.dhs.gov"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveCustomModule(test-locations): %v", err)
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

	emptyEng := NewPatternEngine(nil)
	if len(emptyEng.Rules()) != 0 {
		t.Fatalf("expected 0 hardcoded rules when no pattern modules loaded, got %d", len(emptyEng.Rules()))
	}

	rules := indexer.LoadActivePatternRules("", modulesDir, nil)
	if len(rules) < 3 {
		t.Fatalf("expected >= 3 rules from reference-patterns module, got %d", len(rules))
	}

	eng := NewPatternEngine(rules)

	// 1. House Bill 7031234567 -> 3-segment card (L0) nesting Seattle Gateway (L1) with flat-on-disk L2 children
	hbl := eng.Match(ctx, "7031234567", store, nil)
	if hbl == nil {
		t.Fatalf("expected HBL match for 7031234567")
	}
	if hbl.Card.Title != "House Bill · 7031234567" || hbl.Card.Layout != models.LayoutSegments {
		t.Fatalf("unexpected HBL card: title=%q layout=%q", hbl.Card.Title, hbl.Card.Layout)
	}
	if err := models.ValidateAndNormalizeCard(&hbl.Card, "patterns"); err != nil {
		t.Fatalf("HBL decoded card failed strict validation: %v", err)
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
		t.Fatalf("expected >= 2 nested Level-2 children under Seattle Gateway, got %d", len(seaChildren))
	}

	// 2. Valid IATA AWB 176-12345675 -> resolves Emirates from air-carriers facts + nests guide:mawb & tracking link child
	awb := eng.Match(ctx, "176-12345675", store, nil)
	if awb == nil {
		t.Fatalf("expected IATA AWB match for 176-12345675")
	}
	if awb.Card.Title != "Emirates MAWB · 176-12345675" || awb.Card.Layout != models.LayoutSegments {
		t.Fatalf("unexpected MAWB card: title=%q layout=%q", awb.Card.Title, awb.Card.Layout)
	}
	if err := models.ValidateAndNormalizeCard(&awb.Card, "air-freight"); err != nil {
		t.Fatalf("MAWB decoded card failed strict validation: %v", err)
	}
	awbSegs := awb.Card.Data["segments"].([]map[string]string)
	if len(awbSegs) != 3 || awbSegs[2]["meaning"] != "Valid" {
		t.Fatalf("expected Valid Mod-7 segment, got %+v", awbSegs)
	}
	if len(awb.Card.Children) < 2 {
		t.Fatalf("expected guide:mawb + tracking link children on MAWB card, got %d", len(awb.Card.Children))
	}

	// 3. Invalid IATA AWB check digit flips chip tone to red
	awbBad := eng.Match(ctx, "176-12345674", store, nil)
	if awbBad == nil {
		t.Fatalf("expected match for invalid check digit AWB")
	}
	badSegs := awbBad.Card.Data["segments"].([]map[string]string)
	if len(badSegs) != 3 || badSegs[2]["meaning"] != "Invalid" {
		t.Fatalf("expected Invalid Mod-7 segment, got %+v", badSegs)
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
	if err := models.ValidateAndNormalizeCard(&ctr.Card, "ocean-freight"); err != nil {
		t.Fatalf("ISO container decoded card failed strict validation: %v", err)
	}
	ctrSegs := ctr.Card.Data["segments"].([]map[string]string)
	if len(ctrSegs) != 3 || ctrSegs[2]["meaning"] != "Valid" {
		t.Fatalf("expected Valid ISO 6346 check digit segment, got %+v", ctrSegs)
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

	// 1. Offline fallback attaches strict facts child with helpful hint
	offline := eng.Match(ctx, "AB#48219", nil, nil)
	if offline == nil || offline.Card.Title != "ADO Work Item #48219" || !offline.Card.LocalOnly {
		t.Fatalf("unexpected offline ADO match: %+v", offline)
	}
	if err := models.ValidateAndNormalizeCard(&offline.Card, "patterns"); err != nil {
		t.Fatalf("offline ADO card failed strict validation: %v", err)
	}
	if len(offline.Card.Children) != 1 || !strings.Contains(fmt.Sprintf("%v", offline.Card.Children[0].Data["summary"]), "azure-devops") {
		t.Fatalf("expected helpful offline MCP hint child card, got %+v", offline.Card.Children)
	}

	// 2. Live MCP hit returning structured JSON fields attaches nested facts child card
	stub := &stubLivePatternExecutor{
		text:      `{"title":"Fix Customs EDI Timeout","state":"Active","assignedTo":"Alice","url":"https://dev.azure.com/org/proj/_workitems/edit/48219"}`,
		localOnly: true,
	}
	live := eng.Match(ctx, "AB#48219", nil, stub)
	if live == nil {
		t.Fatalf("expected live ADO match")
	}
	if err := models.ValidateAndNormalizeCard(&live.Card, "patterns"); err != nil {
		t.Fatalf("live ADO card failed strict validation: %v", err)
	}
	if stub.lastSrv != "azure-devops" || stub.lastTool != "wit_get_work_item" || stub.lastArgs["id"] != "48219" {
		t.Fatalf("unexpected MCP call args: srv=%q tool=%q args=%+v", stub.lastSrv, stub.lastTool, stub.lastArgs)
	}
	if live.Card.Subtitle != "Fix Customs EDI Timeout" || !live.Card.LocalOnly || len(live.Card.Children) != 1 || live.Card.Children[0].Layout != models.LayoutFacts {
		t.Fatalf("expected unpacked live ADO facts child card, got %+v", live.Card)
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

type stubLiveHTTPExecutor struct {
	calls   int
	lastReq LiveHTTPRequest
	body    string
	status  int
	err     error
}

func (s *stubLiveHTTPExecutor) ExecuteLiveHTTP(_ context.Context, req LiveHTTPRequest) ([]byte, int, error) {
	s.calls++
	s.lastReq = req
	if s.err != nil {
		return nil, s.status, s.err
	}
	if s.status == 0 {
		s.status = 200
	}
	return []byte(s.body), s.status, nil
}

func TestPatternEngine_LiveHTTPManualAutoAndFallback(t *testing.T) {
	ctx := context.Background()

	ghRule := models.PatternRule{
		ID:            "github-issue-http",
		Name:          "GitHub Issue Lookup",
		Regex:         `^(?P<owner>[A-Za-z0-9_.-]+)/(?P<repo>[A-Za-z0-9_.-]+)#(?P<num>\d+)$`,
		TitleTemplate: "GitHub · {{owner}}/{{repo}}#{{num}}",
		Layout:        models.LayoutSegments,
		Segments: []models.PatternSegmentSpec{
			{Value: "{{owner}}", Role: "OWNER", Meaning: "Org / User", Tone: "blue"},
			{Value: "{{repo}}", Role: "REPO", Meaning: "Repository", Tone: "amber"},
			{Value: "#{{num}}", Role: "ISSUE", Meaning: "Issue / PR #", Tone: "green"},
		},
		ExplanationTemplate: "GitHub issue or pull request reference.",
		Chips: []models.GlanceChip{
			{Label: "Issue / PR", Tone: "blue"},
		},
		LiveHTTP: &models.PatternLiveHTTP{
			URLTemplate: "https://api.github.com/repos/{{owner}}/{{repo}}/issues/{{num}}?ref={{0}}",
			Trigger:     "manual",
			TimeoutMs:   1500,
			CacheTTLSec: 60,
			Auth:        "gh_cli",
			Headers:     map[string]string{"Accept": "application/vnd.github+json"},
			ResultCard: models.PatternLiveResultSpec{
				Layout:          models.LayoutFacts,
				Title:           "{{json.title}}",
				DefaultExpanded: true,
				Chips:           []models.GlanceChip{{Label: "{{json.state}}", Tone: "green"}},
				Summary:         "Opened by @{{json.user.login}}",
				Facts: []models.FactItem{
					{Label: "State", Value: "{{json.state}}"},
					{Label: "Author", Value: "{{json.user.login}}", Mono: true},
					{Label: "URL", Value: "{{json.html_url}}", Copyable: true},
				},
			},
		},
	}

	stub := &stubLiveHTTPExecutor{
		body: `{"title":"Support liveHttp GET lookups","state":"open","user":{"login":"octocat"},"html_url":"https://github.com/octo/vex/issues/42"}`,
	}
	eng := NewPatternEngine([]models.PatternRule{ghRule})
	eng.SetLiveHTTPExecutor(stub)

	// 1. Manual trigger: does NOT call HTTP during typing; emits Fetch Live Data action
	manualMatch := eng.Match(ctx, "octo/vex#42", nil, nil)
	if manualMatch == nil {
		t.Fatal("expected pattern match for octo/vex#42")
	}
	if stub.calls != 0 {
		t.Fatalf("expected 0 HTTP calls for manual trigger, got %d", stub.calls)
	}
	var fetchAct *models.CardAction
	for i := range manualMatch.Card.Actions {
		if manualMatch.Card.Actions[i].ID == LiveHTTPActionID {
			fetchAct = &manualMatch.Card.Actions[i]
			break
		}
	}
	if fetchAct == nil || fetchAct.Label != "Fetch Live Data" {
		t.Fatalf("expected Fetch Live Data action on manual card, got actions=%+v", manualMatch.Card.Actions)
	}
	if !strings.Contains(fetchAct.Target, "ref=octo%2Fvex%2342") {
		t.Fatalf("expected query param {{0}} to be QueryEscaped, got target=%q", fetchAct.Target)
	}

	// 2. Manual FetchLiveHTTP hydrates nested facts child card and marks localOnly=true
	hydrated, err := eng.FetchLiveHTTP(ctx, "github-issue-http", "octo/vex#42", nil, nil, nil)
	if err != nil {
		t.Fatalf("FetchLiveHTTP failed: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("expected 1 HTTP call after FetchLiveHTTP, got %d", stub.calls)
	}
	if !hydrated.Card.LocalOnly {
		t.Fatal("expected authenticated liveHttp match to set LocalOnly=true")
	}
	if len(hydrated.Card.Children) != 1 || hydrated.Card.Children[0].Layout != models.LayoutFacts {
		t.Fatalf("expected 1 nested facts child card, got %+v", hydrated.Card.Children)
	}
	child := hydrated.Card.Children[0]
	if child.Title != "Support liveHttp GET lookups" {
		t.Fatalf("unexpected child title: %q", child.Title)
	}
	if err := models.ValidateAndNormalizeCard(&hydrated.Card, "patterns"); err != nil {
		t.Fatalf("hydrated card failed strict validation: %v", err)
	}

	// 3. Offline / timeout error keeps local segments decode intact and appends muted chip
	errStub := &stubLiveHTTPExecutor{err: context.DeadlineExceeded}
	failed, err := eng.FetchLiveHTTP(ctx, "github-issue-http", "octo/vex#42", nil, nil, errStub)
	if err != nil {
		t.Fatalf("FetchLiveHTTP should gracefully degrade on HTTP error, got err: %v", err)
	}
	if len(failed.Card.Children) != 0 {
		t.Fatalf("expected no child card on error, got %d", len(failed.Card.Children))
	}
	if segs, ok := failed.Card.Data["segments"].([]map[string]string); !ok || len(segs) != 3 {
		t.Fatalf("expected local segments decode to remain intact, got %+v", failed.Card.Data["segments"])
	}
	foundUnavailable := false
	for _, ch := range failed.Card.Chips {
		if ch.Label == LiveLookupUnavailableChip && ch.Tone == "neutral" {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatalf("expected muted %q chip, got chips=%+v", LiveLookupUnavailableChip, failed.Card.Chips)
	}
}

func TestPatternEngine_LiveHTTPAutoProjectionsTableStepsLinkAndDottedKeys(t *testing.T) {
	ctx := context.Background()

	// ADO dotted field key projection into facts + link/table/steps projections
	adoHTTPRule := models.PatternRule{
		ID:            "ado-dotted",
		Name:          "ADO Dotted",
		Regex:         `^AB#(?P<id>\d+)$`,
		TitleTemplate: "ADO #{{id}}",
		LiveHTTP: &models.PatternLiveHTTP{
			URLTemplate: "https://dev.azure.com/org/proj/_apis/wit/workitems/{{id}}?api-version=7.1",
			Trigger:     "auto",
			Auth:        "az_devops",
			ResultCard: models.PatternLiveResultSpec{
				Layout:  models.LayoutFacts,
				Title:   "{{json.fields.System.Title}}",
				Summary: "{{json.fields.System.WorkItemType}} · {{json.fields.System.State}}",
				Facts: []models.FactItem{
					{Label: "Assigned To", Value: "{{json.fields.System.AssignedTo.displayName}}"},
				},
			},
		},
	}

	tableRule := models.PatternRule{
		ID:            "table-live",
		Name:          "Table Live",
		Regex:         `^TBL-(?P<id>\d+)$`,
		TitleTemplate: "Table {{id}}",
		LiveHTTP: &models.PatternLiveHTTP{
			URLTemplate: "https://api.example.com/tables/{{id}}",
			Trigger:     "auto",
			ResultCard: models.PatternLiveResultSpec{
				Layout:    models.LayoutTable,
				Title:     "Events",
				Columns:   []string{"Port", "Status"},
				RowsPath:  "events",
				RowFields: []string{"port", "status"},
			},
		},
	}

	stepsRule := models.PatternRule{
		ID:            "steps-live",
		Name:          "Steps Live",
		Regex:         `^STP-(?P<id>\d+)$`,
		TitleTemplate: "Steps {{id}}",
		LiveHTTP: &models.PatternLiveHTTP{
			URLTemplate: "https://api.example.com/steps/{{id}}",
			Trigger:     "auto",
			ResultCard: models.PatternLiveResultSpec{
				Layout:         models.LayoutSteps,
				Title:          "Milestones",
				Summary:        "Current: {{json.current}}",
				StepsPath:      "stages",
				StepLabel:      "name",
				StepCaption:    "date",
				ActiveStepPath: "activeIndex",
			},
		},
	}

	linkRule := models.PatternRule{
		ID:            "link-live",
		Name:          "Link Live",
		Regex:         `^LNK-(?P<id>\d+)$`,
		TitleTemplate: "Link {{id}}",
		LiveHTTP: &models.PatternLiveHTTP{
			URLTemplate: "https://api.example.com/links/{{id}}",
			Trigger:     "auto",
			ResultCard: models.PatternLiveResultSpec{
				Layout: models.LayoutLink,
				Title:  "Open {{json.name}}",
				URL:    "{{json.links[0].href}}",
			},
		},
	}

	eng := NewPatternEngine([]models.PatternRule{adoHTTPRule, tableRule, stepsRule, linkRule})

	// 1. ADO dotted keys
	eng.SetLiveHTTPExecutor(&stubLiveHTTPExecutor{
		body: `{"id":12345,"fields":{"System.Title":"Fix Retry","System.State":"Active","System.WorkItemType":"Bug","System.AssignedTo":{"displayName":"Alice"}}}`,
	})
	m1 := eng.Match(ctx, "AB#12345", nil, nil)
	if m1 == nil || len(m1.Card.Children) != 1 || m1.Card.Children[0].Title != "Fix Retry" {
		t.Fatalf("unexpected ADO dotted key projection: %+v", m1)
	}

	// 2. Table projection
	eng.SetLiveHTTPExecutor(&stubLiveHTTPExecutor{
		body: `{"events":[{"port":"NLAMS","status":"Departed"},{"port":"USSEA","status":"Arrived"}]}`,
	})
	m2 := eng.Match(ctx, "TBL-1", nil, nil)
	if m2 == nil || len(m2.Card.Children) != 1 || m2.Card.Children[0].Layout != models.LayoutTable {
		t.Fatalf("unexpected table projection: %+v", m2)
	}
	if err := models.ValidateAndNormalizeCard(&m2.Card, "patterns"); err != nil {
		t.Fatalf("table child card failed strict validation: %v", err)
	}

	// 3. Steps projection
	eng.SetLiveHTTPExecutor(&stubLiveHTTPExecutor{
		body: `{"current":"In Transit","activeIndex":2,"stages":[{"name":"Booked","date":"Nov 20"},{"name":"In Transit","date":"Nov 22"}]}`,
	})
	m3 := eng.Match(ctx, "STP-1", nil, nil)
	if m3 == nil || len(m3.Card.Children) != 1 || m3.Card.Children[0].Layout != models.LayoutSteps {
		t.Fatalf("unexpected steps projection: %+v", m3)
	}
	if err := models.ValidateAndNormalizeCard(&m3.Card, "patterns"); err != nil {
		t.Fatalf("steps child card failed strict validation: %v", err)
	}

	// 4. Link projection with array indexing
	eng.SetLiveHTTPExecutor(&stubLiveHTTPExecutor{
		body: `{"name":"Tracker","links":[{"href":"https://track.example.com/123"}]}`,
	})
	m4 := eng.Match(ctx, "LNK-1", nil, nil)
	if m4 == nil || len(m4.Card.Children) != 1 || m4.Card.Children[0].Layout != models.LayoutLink {
		t.Fatalf("unexpected link projection: %+v", m4)
	}
	if err := models.ValidateAndNormalizeCard(&m4.Card, "patterns"); err != nil {
		t.Fatalf("link child card failed strict validation: %v", err)
	}
}
