package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vex/internal/autostart"
	"vex/internal/mcp"
	"vex/internal/models"
)

func TestAppServiceEndToEnd(t *testing.T) {
	baseDir := t.TempDir()
	backend, err := InitBackend(baseDir, false)
	if err != nil {
		t.Fatalf("InitBackend: %v", err)
	}
	defer backend.Close()

	svc := backend.AppService

	// 1. Clean start: no hardcoded test data or forced built-in modules
	initialMods, err := svc.ListModules()
	if err != nil {
		t.Fatalf("ListModules: %v", err)
	}
	if len(initialMods) != 0 {
		t.Fatalf("expected 0 installed modules on fresh start, got %d", len(initialMods))
	}
	starters := svc.ListStarterModules()
	if len(starters) != 5 {
		t.Fatalf("expected 5 optional starter modules (team-templates removed), got %d", len(starters))
	}

	// 2. Onboarding installs all 5 optional starter modules as real files in ~/.vex/modules/
	statuses, err := svc.InstallStarterModules([]string{
		"air-carriers",
		"ocean-carriers",
		"containers",
		"logistics-dictionary",
		"reference-patterns",
	})
	if err != nil || len(statuses) != 5 {
		t.Fatalf("InstallStarterModules failed: count=%d err=%v", len(statuses), err)
	}

	// 3. Rolodex search & pattern decoding: HBL 7031234567 -> 3-segment "segments" card
	hblNodes, err := svc.SearchRolodex("7031234567", 5)
	if err != nil || len(hblNodes) == 0 {
		t.Fatalf("SearchRolodex 7031234567 failed: nodes=%+v err=%v", hblNodes, err)
	}
	if hblNodes[0].Layout != models.LayoutSegments {
		t.Fatalf("unexpected HBL Rolodex tree: %+v", hblNodes[0])
	}

	// 4. MAWB 176-12345675 -> Emirates MAWB with Mod-7 ✓ + nested guide & SkyCargo tracking link
	awbNodes, err := svc.SearchRolodex("176-12345675", 5)
	if err != nil || len(awbNodes) == 0 || !strings.Contains(awbNodes[0].Title, "Emirates MAWB") {
		t.Fatalf("SearchRolodex 176-12345675 failed: nodes=%+v err=%v", awbNodes, err)
	}

	// 5. Card Recents Wails RPC (RecordCardUsed + ListRecentCards)
	if err := svc.RecordCardUsed("container-40hc"); err != nil {
		t.Fatalf("RecordCardUsed: %v", err)
	}
	recents, err := svc.ListRecentCards(6)
	if err != nil || len(recents) != 1 || recents[0].ID != "container-40hc" || len(recents[0].Children) != 2 {
		t.Fatalf("ListRecentCards unexpected: recents=%+v err=%v", recents, err)
	}

	// 6. Deleting a packaged starter module removes it from disk and index permanently
	afterDel, err := svc.DeleteModule("containers")
	if err != nil || len(afterDel) != 4 {
		t.Fatalf("DeleteModule(containers) failed: count=%d err=%v", len(afterDel), err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "modules", "containers.json")); !os.IsNotExist(err) {
		t.Fatalf("expected containers.json to be deleted from modules dir")
	}

	// 7. UpsertCard writes to JSON and indexes immediately
	created, err := svc.UpsertCard(models.SearchCard{
		Title:      "Internal Wiki",
		Collection: "docs",
		Layout:     models.LayoutLink,
		Tags:       []string{"wiki"},
		Data:       map[string]any{"url": "https://wiki.internal"},
		Actions: []models.CardAction{
			{ID: "open", Label: "Open", Type: "open_url", Target: "https://wiki.internal"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertCard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "data", "docs.json")); err != nil {
		t.Fatalf("expected docs.json on disk: %v", err)
	}

	resp2, err := svc.Search("@docs wiki", 10)
	if err != nil || resp2.Total != 1 || resp2.Results[0].Card.ID != created.ID {
		t.Fatalf("expected upserted card in @docs search, got %+v (err=%v)", resp2, err)
	}

	// 8. CSV SampleRawData + ImportWithRule
	csvRaw := "Tool,Link\nLinear,https://linear.app\nVercel,https://vercel.com\n"
	sample, err := svc.SampleRawData(csvRaw, "tools.csv")
	if err != nil || sample.Format != "csv" || sample.TotalRows != 2 {
		t.Fatalf("SampleRawData failed: %+v err=%v", sample, err)
	}

	imp, err := svc.ImportWithRule(csvRaw, models.ETLMappingRule{
		Name:             "Dev Tools",
		SourceFormat:     "csv",
		TargetCollection: "tools",
		DefaultLayout:    models.LayoutLink,
		TitleField:       "{{Tool}}",
		ActionType:       "open_url",
		ActionTarget:     "{{Link}}",
	})
	if err != nil || imp.ImportedCount != 2 {
		t.Fatalf("ImportWithRule failed: %+v err=%v", imp, err)
	}

	// 9. Custom Pattern Rule CRUD & TestPatternRule
	customRule := models.PatternRule{
		ID:            "po-ref",
		Name:          "Purchase Order",
		Priority:      50,
		Normalize:     []string{"trim", "uppercase"},
		Regex:         `^PO-(?P<num>\d{6})$`,
		TitleTemplate: "Purchase Order PO-{{num}}",
		Layout:        models.LayoutSegments,
	}
	testedNode, err := svc.TestPatternRule(customRule, "po-123456")
	if err != nil || testedNode == nil || testedNode.Title != "Purchase Order PO-123456" || testedNode.Layout != models.LayoutSegments {
		t.Fatalf("TestPatternRule failed: node=%+v err=%v", testedNode, err)
	}
	if _, err := svc.UpsertPatternRule(customRule); err != nil {
		t.Fatalf("UpsertPatternRule: %v", err)
	}
	if _, err := svc.DeletePatternRule("po-ref"); err != nil {
		t.Fatalf("DeletePatternRule: %v", err)
	}

	// 10. Hotkey default + live reconfiguration & persistence
	if got := svc.GetHotkey(); got != "Ctrl+Shift+B" {
		t.Fatalf("expected default hotkey Ctrl+Shift+B, got %q", got)
	}
	var reboundFrom, reboundTo string
	svc.SetHotkeyChangeHandler(func(oldKey, newKey string) error {
		reboundFrom, reboundTo = oldKey, newKey
		return nil
	})
	if applied, err := svc.SetHotkey("Ctrl+Alt+P"); err != nil || applied != "Ctrl+Alt+P" {
		t.Fatalf("SetHotkey failed: applied=%q err=%v", applied, err)
	}
	if reboundFrom != "Ctrl+Shift+B" || reboundTo != "Ctrl+Alt+P" {
		t.Fatalf("expected rebind callback Ctrl+Shift+B -> Ctrl+Alt+P, got %q -> %q", reboundFrom, reboundTo)
	}

	// 11. Instant Unit Conversion & 3D Cargo Volumetric Weight Calculator
	convResp, err := svc.Search("1588 kg to lbs", 5)
	if err != nil || len(convResp.Results) == 0 || convResp.Results[0].Card.ID != "builtin-unit-convert" || convResp.Results[0].Card.Layout != models.LayoutConvert {
		t.Fatalf("expected builtin-unit-convert (convert) for '1588 kg to lbs', got %+v err=%v", convResp, err)
	}

	// 12. Shareable custom module import & export (strict 9-layout schema)
	customJSON := `{"id":"accounting-fx","name":"Accounting & FX Codes","category":"Finance","description":"Billing codes","version":"1.0.0","cards":[{"title":"FX Spot Rate Lookup","collection":"accounting","layout":"link","data":{"url":"https://fx.internal"}}]}`
	importedMod, err := svc.ImportModuleJSON(customJSON)
	if err != nil || importedMod == nil || !importedMod.Enabled {
		t.Fatalf("ImportModuleJSON failed: mod=%+v err=%v", importedMod, err)
	}
	exportedJSON, err := svc.ExportModuleJSON("accounting-fx")
	if err != nil || exportedJSON == "" {
		t.Fatalf("ExportModuleJSON failed: err=%v", err)
	}

	// 13. Logging & Update Checker & Anchor & Launch-at-Login
	if lvl, err := svc.SetLogLevel("debug"); err != nil || lvl != "debug" {
		t.Fatalf("SetLogLevel failed: lvl=%q err=%v", lvl, err)
	}
	manifestPath := filepath.Join(baseDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version":"0.9.0","releasedAt":"2026-11-25","notes":"Test release"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := svc.SetUpdateFeedURL(manifestPath); err != nil {
		t.Fatalf("SetUpdateFeedURL: %v", err)
	}
	updRes, err := svc.CheckForUpdates()
	if err != nil || !updRes.UpdateAvailable || updRes.LatestVersion != "0.9.0" {
		t.Fatalf("CheckForUpdates failed: res=%+v err=%v", updRes, err)
	}

	backend.Autostart = autostart.NewMemoryManager(false)
	var traySynced bool
	svc.SetLaunchAtLoginChangeHandler(func(enabled bool) {
		traySynced = enabled
	})
	if applied, err := svc.SetLaunchAtLogin(true); err != nil || !applied || !traySynced {
		t.Fatalf("SetLaunchAtLogin(true) failed: applied=%v traySynced=%v err=%v", applied, traySynced, err)
	}
}

func TestRunValidateCLI(t *testing.T) {
	dir := t.TempDir()
	validFile := filepath.Join(dir, "valid.json")
	invalidFile := filepath.Join(dir, "invalid.json")

	if err := os.WriteFile(validFile, []byte(`{"id":"ok","name":"OK","version":"1.0.0","cards":[{"title":"Docs","collection":"docs","layout":"link","data":{"url":"https://docs.example"}}]}`), 0o644); err != nil {
		t.Fatalf("write valid.json: %v", err)
	}
	if err := os.WriteFile(invalidFile, []byte(`{"id":"bad","name":"Bad","cards":[{"title":"Legacy","collection":"docs","template":"kv-grid","data":{"a":"b"}}]}`), 0o644); err != nil {
		t.Fatalf("write invalid.json: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	if code := runValidateCLI([]string{validFile}, &outBuf, &errBuf); code != 0 || !strings.Contains(outBuf.String(), "OK") {
		t.Fatalf("expected exit 0 for valid module, got code=%d stdout=%q stderr=%q", code, outBuf.String(), errBuf.String())
	}

	outBuf.Reset()
	errBuf.Reset()
	if code := runValidateCLI([]string{invalidFile}, &outBuf, &errBuf); code == 0 || !strings.Contains(errBuf.String(), "FAIL") {
		t.Fatalf("expected non-zero exit for invalid module, got code=%d stdout=%q stderr=%q", code, outBuf.String(), errBuf.String())
	}
}

func TestBuildInfoUnified(t *testing.T) {
	origName, origVersion, origFeed := AppName, AppVersion, UpdateFeedURL
	t.Cleanup(func() {
		AppName, AppVersion, UpdateFeedURL = origName, origVersion, origFeed
	})

	info := currentAppInfo()
	if info.Name != "Vex" || info.Version != "0.2.0" {
		t.Fatalf("unexpected default AppInfo: %+v", info)
	}

	AppName = "Vex"
	AppVersion = "0.3.0"
	UpdateFeedURL = "https://updates.example.com/vex/manifest.json"

	info2 := currentAppInfo()
	if info2.Name != "Vex" || info2.Version != "0.3.0" {
		t.Fatalf("unexpected overridden AppInfo: %+v", info2)
	}
	cfg := defaultRuntimeConfig()
	if cfg.UpdateFeedURL != "https://updates.example.com/vex/manifest.json" {
		t.Fatalf("expected UpdateFeedURL fallback in defaultRuntimeConfig, got %q", cfg.UpdateFeedURL)
	}
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestAppService_FetchPatternLiveHTTPAndMCPPresets(t *testing.T) {
	baseDir := t.TempDir()
	backend, err := InitBackend(baseDir, false)
	if err != nil {
		t.Fatalf("InitBackend: %v", err)
	}
	defer backend.Close()

	// Replace default LiveHTTP client with mocked RoundTripper + mocked gh CLI token runner
	mockLive := mcp.NewLiveHTTPClient(
		mcp.WithCommandRunner(func(_ context.Context, name string, args ...string) (string, string, error) {
			return "mock-gh-token", "", nil
		}),
		mcp.WithHTTPTransport(testRoundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"title":"Hydrated Issue","state":"open","comments":3,"user":{"login":"octocat"},"html_url":"https://github.com/octocat/vex/issues/99"}`)),
			}, nil
		})),
	)
	backend.LiveHTTP = mockLive
	backend.Search.SetLiveHTTPExecutor(mockLive)

	svc := backend.AppService

	// 1. Recommended MCP presets exposed cleanly via GetConfiguredMCPServers
	presets := svc.GetConfiguredMCPServers()
	if len(presets) < 2 {
		t.Fatalf("expected >=2 configured/preset MCP servers, got %d", len(presets))
	}

	// 2. Install reference-patterns starter pack (includes disabled-by-default github-issue-pr & ado-work-item)
	if _, err := svc.InstallStarterModules([]string{"reference-patterns"}); err != nil {
		t.Fatalf("InstallStarterModules: %v", err)
	}
	rules := svc.ListPatternRules()
	foundGH := false
	foundADO := false
	for _, r := range rules {
		if r.ID == "github-issue-pr" {
			foundGH = true
			if r.IsEnabled() {
				t.Fatal("expected github-issue-pr starter pattern to be disabled by default")
			}
		}
		if r.ID == "ado-work-item" {
			foundADO = true
			if r.IsEnabled() {
				t.Fatal("expected ado-work-item starter pattern to be disabled by default")
			}
		}
	}
	if !foundGH || !foundADO {
		t.Fatalf("expected github-issue-pr and ado-work-item starter patterns, foundGH=%v foundADO=%v", foundGH, foundADO)
	}

	// 3. On-demand FetchPatternLiveHTTP hydrates github-issue-pr child card
	node, err := svc.FetchPatternLiveHTTP("github-issue-pr", "octocat/vex#99")
	if err != nil {
		t.Fatalf("FetchPatternLiveHTTP: %v", err)
	}
	if node == nil || len(node.Children) != 1 || node.Children[0].Title != "Hydrated Issue" {
		t.Fatalf("expected hydrated child card 'Hydrated Issue', got %+v", node)
	}
}

func TestUserPrefsAndStayAwakePersistence(t *testing.T) {
	baseDir := t.TempDir()
	backend, err := InitBackend(baseDir, false)
	if err != nil {
		t.Fatalf("InitBackend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	svc := backend.AppService

	// Defaults: uiSize=480, theme=system, animationsEnabled=true, stayAwake=false
	defaults := svc.GetUserPrefs()
	if defaults.UISize != 480 || defaults.Theme != "system" || defaults.AnimationsEnabled == nil || !*defaults.AnimationsEnabled || defaults.StayAwake {
		t.Fatalf("unexpected default UserPrefs: %+v", defaults)
	}
	if svc.GetStayAwake() {
		t.Fatal("expected GetStayAwake() to be false by default")
	}

	// SetStayAwake triggers handler and persists
	var handlerCalls []bool
	svc.SetStayAwakeHandler(func(enabled bool) {
		handlerCalls = append(handlerCalls, enabled)
	})
	if err := svc.SetStayAwake(true); err != nil {
		t.Fatalf("SetStayAwake(true): %v", err)
	}
	if !svc.GetStayAwake() || len(handlerCalls) != 1 || !handlerCalls[0] {
		t.Fatalf("expected stayAwake=true and handler called once, got awake=%v calls=%v", svc.GetStayAwake(), handlerCalls)
	}

	// Update UserPrefs with uiSize=560, theme=dark, animationsEnabled=false, stayAwake=false
	animOff := false
	updated := UserPrefs{
		OnboardingCompleted: true,
		UISize:              560,
		Theme:               "dark",
		AnimationsEnabled:   &animOff,
		StayAwake:           false,
	}
	if err := svc.SetUserPrefs(updated); err != nil {
		t.Fatalf("SetUserPrefs: %v", err)
	}
	if svc.GetStayAwake() || len(handlerCalls) != 2 || handlerCalls[1] {
		t.Fatalf("expected stayAwake=false and handler called twice, got awake=%v calls=%v", svc.GetStayAwake(), handlerCalls)
	}

	// Re-open backend from same baseDir to verify config.json persistence
	backend.Close()
	backend2, err := InitBackend(baseDir, false)
	if err != nil {
		t.Fatalf("reopen InitBackend: %v", err)
	}
	t.Cleanup(func() { backend2.Close() })

	reloaded := backend2.AppService.GetUserPrefs()
	if reloaded.UISize != 560 || reloaded.Theme != "dark" || reloaded.AnimationsEnabled == nil || *reloaded.AnimationsEnabled || reloaded.StayAwake {
		t.Fatalf("unexpected reloaded UserPrefs: %+v", reloaded)
	}
}
