package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vex/internal/ai"
	"vex/internal/autostart"
	"vex/internal/models"
	"vex/internal/secrets"
)

func TestAppServiceEndToEnd(t *testing.T) {
	baseDir := t.TempDir()
	backend, err := InitBackendWithSecrets(baseDir, false, secrets.NewMemoryStore())
	if err != nil {
		t.Fatalf("InitBackendWithSecrets: %v", err)
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
	if len(starters) != 6 {
		t.Fatalf("expected 6 optional starter modules available for onboarding, got %d", len(starters))
	}

	// 2. Onboarding installs selected optional starter modules as real files in ~/.vex/modules/
	statuses, err := svc.InstallStarterModules([]string{
		"air-carriers",
		"ocean-carriers",
		"containers",
		"logistics-dictionary",
		"reference-patterns",
		"team-templates",
	})
	if err != nil || len(statuses) != 6 {
		t.Fatalf("InstallStarterModules failed: count=%d err=%v", len(statuses), err)
	}

	// 3. Rolodex search & pattern decoding: HBL 7031234567 -> 3-segment pattern-breakdown card
	hblNodes, err := svc.SearchRolodex("7031234567", 5)
	if err != nil || len(hblNodes) == 0 {
		t.Fatalf("SearchRolodex 7031234567 failed: nodes=%+v err=%v", hblNodes, err)
	}
	if hblNodes[0].Template != models.TemplatePatternBreakdown {
		t.Fatalf("unexpected HBL Rolodex tree: %+v", hblNodes[0])
	}

	// 4. MAWB 176-12345675 -> Emirates MAWB with Mod-7 ✓ + nested guide & SkyCargo tracking links
	awbNodes, err := svc.SearchRolodex("176-12345675", 5)
	if err != nil || len(awbNodes) == 0 || !strings.Contains(awbNodes[0].Title, "Emirates MAWB") {
		t.Fatalf("SearchRolodex 176-12345675 failed: nodes=%+v err=%v", awbNodes, err)
	}

	// 5. Deleting a packaged starter module removes it from disk and index permanently
	afterDel, err := svc.DeleteModule("containers")
	if err != nil || len(afterDel) != 5 {
		t.Fatalf("DeleteModule(containers) failed: count=%d err=%v", len(afterDel), err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "modules", "containers.json")); !os.IsNotExist(err) {
		t.Fatalf("expected containers.json to be deleted from modules dir")
	}

	// 6. UpsertCard writes to JSON and indexes immediately
	created, err := svc.UpsertCard(models.SearchCard{
		Title:      "Internal Wiki",
		Collection: "docs",
		Template:   models.TemplateAppShortcut,
		Tags:       []string{"wiki"},
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

	// 7. CSV SampleRawData + ImportWithRule
	csvRaw := "Tool,Link\nLinear,https://linear.app\nVercel,https://vercel.com\n"
	sample, err := svc.SampleRawData(csvRaw, "tools.csv")
	if err != nil || sample.Format != "csv" || sample.TotalRows != 2 {
		t.Fatalf("SampleRawData failed: %+v err=%v", sample, err)
	}

	imp, err := svc.ImportWithRule(csvRaw, models.ETLMappingRule{
		Name:             "Dev Tools",
		SourceFormat:     "csv",
		TargetCollection: "tools",
		DefaultTemplate:  models.TemplateAppShortcut,
		TitleField:       "{{Tool}}",
		ActionType:       "open_url",
		ActionTarget:     "{{Link}}",
	})
	if err != nil || imp.ImportedCount != 2 {
		t.Fatalf("ImportWithRule failed: %+v err=%v", imp, err)
	}

	// 8. Custom Pattern Rule CRUD & TestPatternRule
	customRule := models.PatternRule{
		ID:            "po-ref",
		Name:          "Purchase Order",
		Priority:      50,
		Normalize:     []string{"trim", "uppercase"},
		Regex:         `^PO-(?P<num>\d{6})$`,
		TitleTemplate: "Purchase Order PO-{{num}}",
		Template:      models.TemplateReferenceDecoder,
	}
	testedNode, err := svc.TestPatternRule(customRule, "po-123456")
	if err != nil || testedNode == nil || testedNode.Title != "Purchase Order PO-123456" {
		t.Fatalf("TestPatternRule failed: node=%+v err=%v", testedNode, err)
	}
	if _, err := svc.UpsertPatternRule(customRule); err != nil {
		t.Fatalf("UpsertPatternRule: %v", err)
	}
	if _, err := svc.DeletePatternRule("po-ref"); err != nil {
		t.Fatalf("DeletePatternRule: %v", err)
	}

	// 9. Config persistence via SetAIConfig
	if err := svc.SetAIConfig(ai.Config{
		Provider:  ai.ProviderOpenAI,
		BaseURL:   "https://models.github.ai/inference",
		ChatModel: "openai/gpt-4o-mini",
	}); err != nil {
		t.Fatalf("SetAIConfig: %v", err)
	}
	reloaded := loadRuntimeConfig(filepath.Join(baseDir, "config.json"))
	if reloaded.AI.Provider != ai.ProviderOpenAI || reloaded.AI.ChatModel != "openai/gpt-4o-mini" {
		t.Fatalf("expected persisted AI config, got %+v", reloaded.AI)
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
	if err != nil || len(convResp.Results) == 0 || convResp.Results[0].Card.ID != "builtin-unit-convert" {
		t.Fatalf("expected builtin-unit-convert for '1588 kg to lbs', got %+v err=%v", convResp, err)
	}

	// 12. Shareable custom module import & export
	customJSON := `{"id":"accounting-fx","name":"Accounting & FX Codes","category":"Finance","description":"Billing codes","version":"1.0.0","cards":[{"title":"FX Spot Rate Lookup","collection":"accounting","template":"shortcut","actions":[{"id":"open","label":"Open FX","type":"open_url","target":"https://fx.internal"}]}]}`
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

func TestSecretStoreAPIKeyPersistenceAndMigration(t *testing.T) {
	baseDir := t.TempDir()
	memSecrets := secrets.NewMemoryStore()

	legacyJSON := `{
  "ai": {
    "provider": "openai",
    "baseUrl": "https://api.openai.com/v1",
    "apiKey": "sk-legacy-plaintext",
    "chatModel": "gpt-4o-mini"
  },
  "mcpAddr": "127.0.0.1:17380",
  "hotkey": "Ctrl+Shift+B"
}
`
	if err := os.WriteFile(filepath.Join(baseDir, "config.json"), []byte(legacyJSON), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	backend, err := InitBackendWithSecrets(baseDir, false, memSecrets)
	if err != nil {
		t.Fatalf("InitBackendWithSecrets: %v", err)
	}
	defer backend.Close()

	if got := backend.AI.GetConfig().APIKey; got != "sk-legacy-plaintext" {
		t.Fatalf("expected migrated APIKey in memory, got %q", got)
	}
	stored, err := memSecrets.Get(secretServiceName, secretAccountAIAPIKey)
	if err != nil || stored != "sk-legacy-plaintext" {
		t.Fatalf("expected migrated key in secret store, got %q (err=%v)", stored, err)
	}

	rawAfterMigration, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if strings.Contains(string(rawAfterMigration), "sk-legacy-plaintext") {
		t.Fatalf("config.json still contains legacy plaintext key: %s", string(rawAfterMigration))
	}

	if err := backend.AppService.SetAIConfig(ai.Config{
		Provider:  ai.ProviderOpenAI,
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "sk-updated-secret",
		ChatModel: "gpt-4o-mini",
	}); err != nil {
		t.Fatalf("SetAIConfig: %v", err)
	}
	stored2, err := memSecrets.Get(secretServiceName, secretAccountAIAPIKey)
	if err != nil || stored2 != "sk-updated-secret" {
		t.Fatalf("expected updated key in secret store, got %q (err=%v)", stored2, err)
	}
}
