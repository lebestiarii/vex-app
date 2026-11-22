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

	// 1. Seeded collections should be immediately searchable
	resp, err := svc.Search("staging", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if resp.Total == 0 {
		t.Fatalf("expected seeded staging card to match")
	}

	// 2. UpsertCard writes to JSON and indexes immediately
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

	// 3. CSV SampleRawData + ImportWithRule
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

	// 4. Config persistence via SetAIConfig
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

	// 5. Hotkey default + live reconfiguration & persistence
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
	reloaded2 := loadRuntimeConfig(filepath.Join(baseDir, "config.json"))
	if reloaded2.Hotkey != "Ctrl+Alt+P" {
		t.Fatalf("expected persisted hotkey Ctrl+Alt+P, got %q", reloaded2.Hotkey)
	}

	// 6. UserPrefs defaults & persistence
	defaultPrefs := svc.GetUserPrefs()
	if defaultPrefs.OnboardingCompleted || defaultPrefs.PowerUserMode || defaultPrefs.AIEnabled {
		t.Fatalf("expected clean default user prefs, got %+v", defaultPrefs)
	}
	updatedPrefs := UserPrefs{
		OnboardingCompleted: true,
		PowerUserMode:       false,
		AIEnabled:           true,
		SelectedGoals:       []string{"links", "notes", "ai"},
		PinnedCategories:    []string{"ports", "customs", "global-tech"},
	}
	if err := svc.SetUserPrefs(updatedPrefs); err != nil {
		t.Fatalf("SetUserPrefs: %v", err)
	}
	reloaded3 := loadRuntimeConfig(filepath.Join(baseDir, "config.json"))
	if len(reloaded3.Prefs.PinnedCategories) != 3 || reloaded3.Prefs.PinnedCategories[0] != "ports" {
		t.Fatalf("expected persisted PinnedCategories, got %+v", reloaded3.Prefs.PinnedCategories)
	}
	if !reloaded3.Prefs.OnboardingCompleted || !reloaded3.Prefs.AIEnabled || len(reloaded3.Prefs.SelectedGoals) != 3 {
		t.Fatalf("expected persisted user prefs, got %+v", reloaded3.Prefs)
	}

	// 7. Logistics multi-facet "AMS" lookup (Port, Acronym, Event Code, Ocean Portal)
	amsResp, err := svc.Search("AMS", 10)
	if err != nil {
		t.Fatalf("Search AMS: %v", err)
	}
	if amsResp.Total < 4 {
		t.Fatalf("expected at least 4 multi-facet AMS matches (port, acronym, event, portal), got %d", amsResp.Total)
	}

	// 8. Composable Module & Overlay Engine:
	// By default, Global Technology is OFF, so searching internal hostname "ams-prd-edi01" returns 0 results.
	techOffResp, err := svc.Search("ams-prd-edi01", 10)
	if err != nil {
		t.Fatalf("Search ams-prd-edi01 (off): %v", err)
	}
	if techOffResp.Total != 0 {
		t.Fatalf("expected 0 matches for ams-prd-edi01 when global-tech is disabled, got %d", techOffResp.Total)
	}

	// Verify port-nlams has 3 composed sections (Air, Ocean, Customs), glance pills, official provenance, and related synergy cards.
	amsCard, err := backend.Store.GetCard(t.Context(), "port-nlams")
	if err != nil || amsCard == nil {
		t.Fatalf("GetCard port-nlams: card=%v err=%v", amsCard, err)
	}
	if len(amsCard.Sections) != 3 {
		t.Fatalf("expected 3 overlay sections (Air, Ocean, Customs) on port-nlams, got %d", len(amsCard.Sections))
	}
	if len(amsCard.GlancePills) == 0 || amsCard.GlancePills[0] != "NLAMS · AMS" {
		t.Fatalf("expected GlancePills starting with 'NLAMS · AMS', got %+v", amsCard.GlancePills)
	}
	if amsCard.Provenance == nil || amsCard.Provenance.Authority != "official" {
		t.Fatalf("expected official provenance on port-nlams, got %+v", amsCard.Provenance)
	}
	related, err := svc.GetRelatedCards("port-nlams", 5)
	if err != nil || len(related) < 3 {
		t.Fatalf("expected at least 3 related synergy cards for port-nlams, got %d (err=%v)", len(related), err)
	}

	// 9. Instant Unit Conversion & 3D Cargo Volumetric Weight Calculator
	convResp, err := svc.Search("1588 kg to lbs", 5)
	if err != nil || len(convResp.Results) == 0 || convResp.Results[0].Card.ID != "builtin-unit-convert" {
		t.Fatalf("expected builtin-unit-convert for '1588 kg to lbs', got %+v err=%v", convResp, err)
	}
	dimResp, err := svc.Search("120x80x100 cm", 5)
	if err != nil || len(dimResp.Results) == 0 || dimResp.Results[0].Card.ID != "builtin-unit-convert" {
		t.Fatalf("expected builtin-unit-convert for '120x80x100 cm', got %+v err=%v", dimResp, err)
	}

	// 10. GitHub Engineering Catalog Module & gt-portfolio-team-product-component Taxonomy
	if _, err := svc.SetEnabledModules([]string{"geographies", "air-freight", "ocean-freight", "customs", "github-catalog"}); err != nil {
		t.Fatalf("SetEnabledModules (enable github-catalog): %v", err)
	}
	ghResp, err := svc.Search("gt-customs-brokerage-declaration-api", 5)
	if err != nil || ghResp.Total == 0 {
		t.Fatalf("expected github-catalog repo match, got %+v err=%v", ghResp, err)
	}
	ghImport, err := svc.ImportGitHubReposJSON(`[{"name":"gt-accounting-treasury-hedging-api","description":"FX hedging microservice","html_url":"https://github.internal/gt/gt-accounting-treasury-hedging-api","language":"Go"}]`)
	if err != nil || ghImport == nil || !ghImport.Enabled {
		t.Fatalf("ImportGitHubReposJSON failed: %+v err=%v", ghImport, err)
	}
	hedgeResp, err := svc.Search("hedging", 5)
	if err != nil || hedgeResp.Total == 0 {
		t.Fatalf("expected imported gt-accounting-treasury-hedging-api repo in search, got %+v err=%v", hedgeResp, err)
	}

	// Toggle Global Technology ON -> port-nlams gains 4th section and "ams-prd-edi01" becomes searchable!
	if _, err := svc.SetEnabledModules([]string{"geographies", "air-freight", "ocean-freight", "customs", "global-tech"}); err != nil {
		t.Fatalf("SetEnabledModules (enable global-tech): %v", err)
	}
	techOnResp, err := svc.Search("ams-prd-edi01", 10)
	if err != nil || techOnResp.Total == 0 {
		t.Fatalf("expected ams-prd-edi01 to match enriched AMS card when global-tech is enabled, got %+v err=%v", techOnResp, err)
	}
	amsCardWithTech, err := backend.Store.GetCard(t.Context(), "port-nlams")
	if err != nil || amsCardWithTech == nil || len(amsCardWithTech.Sections) != 4 {
		t.Fatalf("expected 4 sections on port-nlams with global-tech enabled, got %+v err=%v", amsCardWithTech, err)
	}

	// Standalone promotion: disable "geographies" and keep only "air-freight" -> AMS air overlay promotes to standalone card!
	if _, err := svc.SetEnabledModules([]string{"air-freight"}); err != nil {
		t.Fatalf("SetEnabledModules (air-freight only): %v", err)
	}
	airOnlyCard, err := backend.Store.GetCard(t.Context(), "port-nlams")
	if err != nil || airOnlyCard == nil || airOnlyCard.ModuleID != "air-freight" {
		t.Fatalf("expected promoted standalone air-freight card for port-nlams, got %+v err=%v", airOnlyCard, err)
	}

	// Shareable custom module import & export
	customJSON := `{"id":"accounting-fx","name":"Accounting & FX Codes","category":"Finance","description":"Billing codes","version":"1.0.0","cards":[{"title":"FX Spot Rate Lookup","collection":"accounting","template":"shortcut","actions":[{"id":"open","label":"Open FX","type":"open_url","target":"https://fx.internal"}]}]}`
	importedMod, err := svc.ImportModuleJSON(customJSON)
	if err != nil || importedMod == nil || !importedMod.Enabled {
		t.Fatalf("ImportModuleJSON failed: mod=%+v err=%v", importedMod, err)
	}
	exportedJSON, err := svc.ExportModuleJSON("accounting-fx")
	if err != nil || exportedJSON == "" {
		t.Fatalf("ExportModuleJSON failed: err=%v", err)
	}

	// 11. Local Structured JSONL Logging & Diagnostics
	logInfo := svc.GetLogInfo()
	if logInfo.FilePath == "" || logInfo.Level != "info" {
		t.Fatalf("unexpected GetLogInfo: %+v", logInfo)
	}
	if lvl, err := svc.SetLogLevel("debug"); err != nil || lvl != "debug" {
		t.Fatalf("SetLogLevel failed: lvl=%q err=%v", lvl, err)
	}
	recentLogs, err := svc.GetRecentLogs(20)
	if err != nil || len(recentLogs) == 0 {
		t.Fatalf("expected structured log entries, got %d (err=%v)", len(recentLogs), err)
	}

	// 12. No-Login Update Checker (local/shared folder manifest.json)
	manifestPath := filepath.Join(baseDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version":"0.2.0","releasedAt":"2026-11-25","notes":"Test release"}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := svc.SetUpdateFeedURL(manifestPath); err != nil {
		t.Fatalf("SetUpdateFeedURL: %v", err)
	}
	updRes, err := svc.CheckForUpdates()
	if err != nil || !updRes.UpdateAvailable || updRes.LatestVersion != "0.2.0" {
		t.Fatalf("CheckForUpdates failed: res=%+v err=%v", updRes, err)
	}

	// 13. Viewport-aware anchor direction RPC
	if got := svc.GetAnchorDirection(); got != "down" {
		t.Fatalf("expected default anchor 'down', got %q", got)
	}
	svc.SetAnchorDirection(AnchorUp)
	if got := svc.GetAnchorDirection(); got != "up" {
		t.Fatalf("expected anchor 'up', got %q", got)
	}
	svc.SetAnchorDirection(AnchorDirection("invalid"))
	if got := svc.GetAnchorDirection(); got != "down" {
		t.Fatalf("expected normalized anchor 'down', got %q", got)
	}

	// 14. Launch-at-Login RPC + persistence + tray sync callback
	backend.Autostart = autostart.NewMemoryManager(false)
	var traySynced bool
	svc.SetLaunchAtLoginChangeHandler(func(enabled bool) {
		traySynced = enabled
	})
	if svc.GetLaunchAtLogin() {
		t.Fatalf("expected launchAtLogin false initially")
	}
	applied, err := svc.SetLaunchAtLogin(true)
	if err != nil || !applied || !svc.GetLaunchAtLogin() || !traySynced {
		t.Fatalf("SetLaunchAtLogin(true) failed: applied=%v traySynced=%v err=%v", applied, traySynced, err)
	}
	reloadedAuto := loadRuntimeConfig(filepath.Join(baseDir, "config.json"))
	if !reloadedAuto.Prefs.LaunchAtLogin {
		t.Fatalf("expected LaunchAtLogin persisted in config.json")
	}
}

func TestBuildInfoEditions(t *testing.T) {
	origName, origEdition, origVersion, origFeed := AppName, Edition, AppVersion, UpdateFeedURL
	t.Cleanup(func() {
		AppName, Edition, AppVersion, UpdateFeedURL = origName, origEdition, origVersion, origFeed
	})

	info := currentAppInfo()
	if info.Name != "Vex" || info.Edition != "community" || info.Version != "0.1.0" {
		t.Fatalf("unexpected default AppInfo: %+v", info)
	}

	AppName = "Vex"
	Edition = "enterprise"
	AppVersion = "0.2.0"
	UpdateFeedURL = "https://intranet.example.com/vex/manifest.json"

	info2 := currentAppInfo()
	if info2.Name != "Vex" || info2.Edition != "enterprise" || info2.Version != "0.2.0" {
		t.Fatalf("unexpected overridden AppInfo: %+v", info2)
	}
	cfg := defaultRuntimeConfig()
	if cfg.UpdateFeedURL != "https://intranet.example.com/vex/manifest.json" {
		t.Fatalf("expected UpdateFeedURL fallback in defaultRuntimeConfig, got %q", cfg.UpdateFeedURL)
	}
}

func TestSecretStoreAPIKeyPersistenceAndMigration(t *testing.T) {
	baseDir := t.TempDir()
	memSecrets := secrets.NewMemoryStore()

	// 1. Seed legacy plaintext config.json containing an API key
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

	// Legacy key must be hydrated in AI service and migrated into the secret store
	if got := backend.AI.GetConfig().APIKey; got != "sk-legacy-plaintext" {
		t.Fatalf("expected migrated APIKey in memory, got %q", got)
	}
	stored, err := memSecrets.Get(secretServiceName, secretAccountAIAPIKey)
	if err != nil || stored != "sk-legacy-plaintext" {
		t.Fatalf("expected migrated key in secret store, got %q (err=%v)", stored, err)
	}

	// config.json on disk must have been scrubbed
	rawAfterMigration, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if strings.Contains(string(rawAfterMigration), "sk-legacy-plaintext") {
		t.Fatalf("config.json still contains legacy plaintext key: %s", string(rawAfterMigration))
	}

	// 2. Updating AI config stores new key in secret store and keeps config.json clean
	if err := backend.AppService.SetAIConfig(ai.Config{
		Provider:  ai.ProviderOpenAI,
		ChatModel: "gpt-4o-mini",
		APIKey:    "sk-new-secret-key",
	}); err != nil {
		t.Fatalf("SetAIConfig: %v", err)
	}
	stored2, err := memSecrets.Get(secretServiceName, secretAccountAIAPIKey)
	if err != nil || stored2 != "sk-new-secret-key" {
		t.Fatalf("expected updated key in secret store, got %q (err=%v)", stored2, err)
	}
	rawAfterSet, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json after SetAIConfig: %v", err)
	}
	if strings.Contains(string(rawAfterSet), "sk-new-secret-key") || strings.Contains(string(rawAfterSet), `"apiKey"`) {
		t.Fatalf("config.json leaked apiKey after SetAIConfig: %s", string(rawAfterSet))
	}
	if backend.Config().AI.APIKey != "" {
		t.Fatalf("Backend.Config() should not expose raw AI APIKey")
	}

	// 3. Restarting backend hydrates API key from secret store
	backend.Close()
	backend2, err := InitBackendWithSecrets(baseDir, false, memSecrets)
	if err != nil {
		t.Fatalf("InitBackendWithSecrets restart: %v", err)
	}
	defer backend2.Close()
	if got := backend2.AI.GetConfig().APIKey; got != "sk-new-secret-key" {
		t.Fatalf("expected hydrated APIKey after restart, got %q", got)
	}
}
