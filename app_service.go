package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vex/internal/ai"
	"vex/internal/autostart"
	"vex/internal/indexer"
	"vex/internal/logging"
	"vex/internal/mcp"
	"vex/internal/models"
	"vex/internal/plugins"
	"vex/internal/search"
	"vex/internal/secrets"
	"vex/internal/updater"
)

const (
	secretServiceName     = "vex"
	secretAccountAIAPIKey = "ai_api_key"
)

// UserPrefs stores human-friendly UI and onboarding preferences in config.json.
type UserPrefs struct {
	OnboardingCompleted bool     `json:"onboardingCompleted"`
	PowerUserMode       bool     `json:"powerUserMode"`
	AIEnabled           bool     `json:"aiEnabled"`
	LaunchAtLogin       bool     `json:"launchAtLogin,omitempty"`
	LayoutMode          string   `json:"layoutMode,omitempty"` // "compact" (hero + runner-ups, default) or "split"
	SelectedGoals       []string `json:"selectedGoals,omitempty"`
	EnabledModules      []string `json:"enabledModules,omitempty"`
	PinnedCategories    []string `json:"pinnedCategories,omitempty"`
}

// RuntimeConfig is persisted at <baseDir>/config.json.
type RuntimeConfig struct {
	AI            ai.Config                  `json:"ai"`
	MCPAddr       string                     `json:"mcpAddr"`
	Hotkey        string                     `json:"hotkey"`
	LogLevel      string                     `json:"logLevel,omitempty"`      // "debug" | "info" | "warn" | "error"
	UpdateFeedURL string                     `json:"updateFeedUrl,omitempty"` // No-login manifest.json URL or shared folder path
	Prefs         UserPrefs                  `json:"prefs"`
	MCPServers    []mcp.ExternalServerConfig `json:"mcpServers,omitempty"`
}

func defaultHotkey() string {
	return "Ctrl+Shift+B"
}

func defaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		AI:            ai.DefaultConfig(),
		MCPAddr:       "127.0.0.1:17380",
		Hotkey:        defaultHotkey(),
		LogLevel:      "info",
		UpdateFeedURL: strings.TrimSpace(UpdateFeedURL),
	}
}

// ImportResult summarizes a completed ETL import for the Card Studio UI.
type ImportResult struct {
	Collection    string `json:"collection"`
	ImportedCount int    `json:"importedCount"`
	FilePath      string `json:"filePath"`
}

// Backend bundles all domain services for both GUI and --mcp-stdio modes.
type Backend struct {
	BaseDir     string
	DataDir     string
	ModulesDir  string
	PatternsDir string
	PluginsDir  string
	ConfigPath  string

	Store      *indexer.Store
	Watcher    *indexer.Watcher
	Search     *search.Service
	Plugins    *plugins.Manager
	Runner     *plugins.Runner
	AI         *ai.Service
	MCPHub     *mcp.HubServer
	MCPClients *mcp.ClientManager
	Logger     *logging.Manager
	Updater     *updater.Checker
	Secrets     secrets.Store
	Autostart   autostart.Manager
	AppService  *AppService

	mu  sync.RWMutex
	cfg RuntimeConfig
}

// InitBackend initializes storage, watcher, search, plugins, AI, and MCP services under baseDir.
func InitBackend(baseDir string, startWatcher bool) (*Backend, error) {
	return InitBackendWithSecrets(baseDir, startWatcher, nil)
}

// InitBackendWithSecrets initializes the backend with an explicit secrets.Store (useful for tests).
func InitBackendWithSecrets(baseDir string, startWatcher bool, secretStore secrets.Store) (*Backend, error) {
	if secretStore == nil {
		secretStore = secrets.NewDefaultStore()
	}
	if strings.TrimSpace(baseDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".vex")
	}
	dataDir := filepath.Join(baseDir, "data")
	modulesDir := filepath.Join(baseDir, "modules")
	patternsDir := filepath.Join(baseDir, "patterns")
	pluginsDir := filepath.Join(baseDir, "plugins")
	configPath := filepath.Join(baseDir, "config.json")
	dbPath := filepath.Join(baseDir, "index.db")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(modulesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create modules dir: %w", err)
	}
	if err := os.MkdirAll(patternsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create patterns dir: %w", err)
	}
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create plugins dir: %w", err)
	}
	if err := indexer.EnsureDataDir(dataDir); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}

	cfg := loadRuntimeConfig(configPath)

	logMgr, err := logging.NewManager(filepath.Join(baseDir, "logs"), cfg.LogLevel, logging.DefaultMaxBytes, logging.DefaultMaxBackups)
	if err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}
	slog.SetDefault(logMgr.Logger())
	logMgr.Component("app").Info("backend_initializing", "base_dir", baseDir, "log_level", logMgr.Level())

	// Migrate any legacy plaintext AI API key from config.json into the OS secret store,
	// or hydrate the in-memory AI config from the secret store.
	if legacyKey := strings.TrimSpace(cfg.AI.APIKey); legacyKey != "" {
		if err := secretStore.Set(secretServiceName, secretAccountAIAPIKey, legacyKey); err != nil {
			logMgr.Component("app").Warn("ai_api_key_secret_migration_failed", "err", err)
		} else {
			cfg.AI.APIKey = ""
			if saveErr := saveRuntimeConfig(configPath, cfg); saveErr != nil {
				logMgr.Component("app").Warn("ai_api_key_config_scrub_failed", "err", saveErr)
			} else {
				logMgr.Component("app").Info("ai_api_key_migrated_to_secret_store")
			}
			cfg.AI.APIKey = legacyKey
		}
	} else if storedKey, err := secretStore.Get(secretServiceName, secretAccountAIAPIKey); err == nil && strings.TrimSpace(storedKey) != "" {
		cfg.AI.APIKey = storedKey
	}

	store, err := indexer.OpenStore(dbPath)
	if err != nil {
		_ = logMgr.Close()
		return nil, fmt.Errorf("open store: %w", err)
	}

	// Perform initial full sync of JSON collections on disk into SQLite.
	ctx := context.Background()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		full := filepath.Join(dataDir, e.Name())
		col, cards, loadErr := indexer.LoadCollectionFile(full)
		if loadErr == nil {
			_ = store.ReplaceCollection(ctx, col, cards, full)
		}
	}

	// Compile and index active composable modules and card overlays.
	modStatuses, err := indexer.SyncModules(ctx, store, modulesDir, cfg.Prefs.EnabledModules)
	if err != nil {
		_ = store.Close()
		_ = logMgr.Close()
		return nil, fmt.Errorf("sync modules: %w", err)
	}
	updateInventoryGauges(ctx, store, logMgr.Metrics, modStatuses)

	pluginMgr := plugins.NewManager()
	_ = pluginMgr.LoadDir(pluginsDir)
	runner := plugins.NewRunner()

	searchSvc := search.NewService(store)
	searchSvc.SetPluginProvider(pluginMgr)

	aiSvc := ai.NewService(searchSvc, store, cfg.AI)
	if strings.TrimSpace(cfg.AI.EmbeddingModel) != "" {
		searchSvc.SetEmbedder(func(ctx context.Context, text string) ([]float32, string, error) {
			embedCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			return aiSvc.Embed(embedCtx, text)
		})
	}

	var b *Backend
	reloadPatterns := func() {
		var enabledMods []string
		if b != nil {
			enabledMods = b.Config().Prefs.EnabledModules
		} else {
			enabledMods = cfg.Prefs.EnabledModules
		}
		searchSvc.SetPatternRules(indexer.LoadActivePatternRules(patternsDir, modulesDir, enabledMods))
	}
	reloadModules := func() {
		var enabledMods []string
		if b != nil {
			enabledMods = b.Config().Prefs.EnabledModules
		} else {
			enabledMods = cfg.Prefs.EnabledModules
		}
		if statuses, syncErr := indexer.SyncModules(context.Background(), store, modulesDir, enabledMods); syncErr == nil {
			updateInventoryGauges(context.Background(), store, logMgr.Metrics, statuses)
		}
		reloadPatterns()
	}

	skillsDirs := []string{filepath.Join(baseDir, "skills")}
	if wd, wdErr := os.Getwd(); wdErr == nil {
		skillsDirs = append(skillsDirs, filepath.Join(wd, ".agent", "skills"))
	}

	mcpHub, err := mcp.NewHubServer(mcp.ServerDeps{
		Store:             store,
		Search:            searchSvc,
		Plugins:           pluginMgr,
		Runner:            runner,
		Metrics:           logMgr.Metrics,
		DataDir:           dataDir,
		ModulesDir:        modulesDir,
		PatternsDir:       patternsDir,
		SkillsDirs:        skillsDirs,
		OnModulesChanged:  reloadModules,
		OnPatternsChanged: reloadPatterns,
		Version:           currentAppInfo().Version,
	})
	if err != nil {
		_ = store.Close()
		_ = logMgr.Close()
		return nil, fmt.Errorf("init MCP hub: %w", err)
	}

	mcpClients := mcp.NewClientManager(store)
	searchSvc.SetLivePatternExecutor(mcpClients)
	reloadPatterns()

	var watcher *indexer.Watcher
	if startWatcher {
		watcher = indexer.NewWatcher(store, dataDir, nil)
		watcher.SetModulesWatcher(modulesDir, reloadModules)
		watcher.SetPatternsWatcher(patternsDir, reloadPatterns)
		_ = watcher.Start(ctx)
	}

	b = &Backend{
		BaseDir:     baseDir,
		DataDir:     dataDir,
		ModulesDir:  modulesDir,
		PatternsDir: patternsDir,
		PluginsDir:  pluginsDir,
		ConfigPath:  configPath,
		Store:       store,
		Watcher:     watcher,
		Search:      searchSvc,
		Plugins:     pluginMgr,
		Runner:      runner,
		AI:          aiSvc,
		MCPHub:      mcpHub,
		MCPClients:  mcpClients,
		Logger:      logMgr,
		Updater:     updater.NewChecker(),
		Secrets:     secretStore,
		Autostart:   autostart.NewDefaultManager(currentAppInfo().Name),
		cfg:         scrubConfigSecrets(cfg),
	}
	b.AppService = &AppService{backend: b}
	logMgr.Component("app").Info("backend_ready", "active_cards", logMgr.Metrics.ActiveCards.Load(), "enabled_modules", logMgr.Metrics.EnabledModules.Load())

	// Best-effort connect to any enabled external MCP servers configured in config.json.
	for _, srvCfg := range cfg.MCPServers {
		if srvCfg.Enabled {
			connCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, _ = mcpClients.Connect(connCtx, srvCfg)
			cancel()
		}
	}

	return b, nil
}

// Config returns a snapshot of the runtime configuration.
func (b *Backend) Config() RuntimeConfig {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cfg
}

// Close shuts down watcher, MCP servers/clients, and SQLite cleanly.
func (b *Backend) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if b.Watcher != nil {
		_ = b.Watcher.Stop()
	}
	if b.MCPHub != nil {
		_ = b.MCPHub.ShutdownHTTP(ctx)
	}
	if b.MCPClients != nil {
		b.MCPClients.CloseAll(ctx)
	}
	if b.Store != nil {
		_ = b.Store.Close()
	}
	if b.Logger != nil {
		b.Logger.Component("app").Info("backend_shutdown")
		_ = b.Logger.Close()
	}
}

func updateInventoryGauges(ctx context.Context, store *indexer.Store, m *logging.Metrics, statuses []models.ModuleStatus) {
	if m == nil {
		return
	}
	if statuses != nil {
		var enabled int64
		for _, s := range statuses {
			if s.Enabled {
				enabled++
			}
		}
		m.EnabledModules.Store(enabled)
	}
	if store != nil {
		if cols, err := store.ListCollections(ctx); err == nil {
			var total int64
			for _, c := range cols {
				total += int64(c.CardCount)
			}
			m.ActiveCards.Store(total)
		}
	}
}

func loadRuntimeConfig(path string) RuntimeConfig {
	cfg := defaultRuntimeConfig()
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(raw, &cfg)
	if strings.TrimSpace(cfg.MCPAddr) == "" {
		cfg.MCPAddr = "127.0.0.1:17380"
	}
	if strings.TrimSpace(cfg.Hotkey) == "" {
		cfg.Hotkey = defaultHotkey()
	}
	return cfg
}

func scrubConfigSecrets(cfg RuntimeConfig) RuntimeConfig {
	cfg.AI.APIKey = ""
	return cfg
}

func saveRuntimeConfig(path string, cfg RuntimeConfig) error {
	cfg = scrubConfigSecrets(cfg)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WeatherInfo holds lightweight live weather data for location/port cards.
type WeatherInfo struct {
	TempC     float64 `json:"tempC"`
	TempF     float64 `json:"tempF"`
	Condition string  `json:"condition"`
	CachedAt  string  `json:"cachedAt"`
}

type weatherCacheEntry struct {
	info      WeatherInfo
	fetchedAt time.Time
}

// AppService is bound to the Wails v3 frontend via application.NewService.
type AppService struct {
	backend               *Backend
	mu                    sync.RWMutex
	onHide                func()
	onHotkeyChange        func(oldKey, newKey string) error
	onLaunchAtLoginChange func(enabled bool)
	weatherCache          map[string]weatherCacheEntry
	anchorDirection       AnchorDirection
}

// SetAnchorDirection stores the current viewport-aware anchor direction ("down" or "up").
func (s *AppService) SetAnchorDirection(dir AnchorDirection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.anchorDirection = NormalizeAnchor(string(dir))
}

// GetAnchorDirection returns the current viewport-aware anchor direction ("down" or "up").
func (s *AppService) GetAnchorDirection() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.anchorDirection == "" {
		return string(AnchorDown)
	}
	return string(s.anchorDirection)
}

// SetHideHandler registers the Wails window hide callback.
func (s *AppService) SetHideHandler(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onHide = fn
}

// SetHotkeyChangeHandler registers a callback that live-rebinds the OS global shortcut.
func (s *AppService) SetHotkeyChangeHandler(fn func(oldKey, newKey string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onHotkeyChange = fn
}

// SetLaunchAtLoginChangeHandler registers a callback to keep the system tray menu synced.
func (s *AppService) SetLaunchAtLoginChangeHandler(fn func(enabled bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLaunchAtLoginChange = fn
}

// GetLaunchAtLogin returns whether Vex is configured to launch automatically on OS login.
func (s *AppService) GetLaunchAtLogin() bool {
	if s.backend.Autostart != nil {
		if enabled, err := s.backend.Autostart.IsEnabled(); err == nil {
			return enabled
		}
	}
	return s.backend.Config().Prefs.LaunchAtLogin
}

// SetLaunchAtLogin enables or disables OS login autostart, persists UserPrefs, and notifies the tray menu.
func (s *AppService) SetLaunchAtLogin(enabled bool) (bool, error) {
	if s.backend.Autostart != nil {
		if err := s.backend.Autostart.SetEnabled(enabled); err != nil {
			return s.GetLaunchAtLogin(), err
		}
	}

	s.backend.mu.Lock()
	s.backend.cfg.Prefs.LaunchAtLogin = enabled
	snapshot := s.backend.cfg
	s.backend.mu.Unlock()

	if err := saveRuntimeConfig(s.backend.ConfigPath, snapshot); err != nil {
		return enabled, err
	}

	s.mu.RLock()
	cb := s.onLaunchAtLoginChange
	s.mu.RUnlock()
	if cb != nil {
		cb(enabled)
	}
	return enabled, nil
}

func (s *AppService) GetHotkey() string {
	return s.backend.Config().Hotkey
}

func (s *AppService) SetHotkey(accelerator string) (string, error) {
	newKey := strings.TrimSpace(accelerator)
	if newKey == "" {
		return "", errors.New("hotkey accelerator cannot be empty")
	}

	s.backend.mu.RLock()
	oldKey := s.backend.cfg.Hotkey
	s.backend.mu.RUnlock()

	if oldKey == newKey {
		return newKey, nil
	}

	s.mu.RLock()
	cb := s.onHotkeyChange
	s.mu.RUnlock()
	if cb != nil {
		if err := cb(oldKey, newKey); err != nil {
			return oldKey, err
		}
	}

	s.backend.mu.Lock()
	s.backend.cfg.Hotkey = newKey
	snapshot := s.backend.cfg
	s.backend.mu.Unlock()

	if err := saveRuntimeConfig(s.backend.ConfigPath, snapshot); err != nil {
		return newKey, err
	}
	return newKey, nil
}

func (s *AppService) Search(rawQuery string, limit int) (*search.SearchResponse, error) {
	start := time.Now()
	resp, err := s.backend.Search.Search(context.Background(), rawQuery, limit)
	if s.backend.Logger != nil {
		s.backend.Logger.Metrics.SearchesTotal.Add(1)
		total := 0
		if resp != nil {
			total = resp.Total
		}
		if err != nil {
			s.backend.Logger.Component("search").Error("search_failed", "error", err.Error(), "duration_ms", time.Since(start).Milliseconds())
		} else {
			s.backend.Logger.Component("search").Debug("search_completed", "results", total, "duration_ms", time.Since(start).Milliseconds())
		}
	}
	return resp, err
}

func (s *AppService) ExecuteAction(cardID, actionID, rawQuery string, extraArgs map[string]string) (plugins.ActionResult, error) {
	ctx := context.Background()
	parsed := search.Parse(rawQuery)
	recordMetric := func(res plugins.ActionResult) plugins.ActionResult {
		if s.backend.Logger != nil {
			s.backend.Logger.Metrics.ActionsTotal.Add(1)
			if !res.Success {
				s.backend.Logger.Metrics.ActionErrorsTotal.Add(1)
				s.backend.Logger.Component("runner").Warn("action_failed", "card_id", cardID, "action_type", res.ActionType, "error", res.Error, "duration_ms", res.DurationMs)
			} else {
				s.backend.Logger.Component("runner").Info("action_executed", "card_id", cardID, "action_type", res.ActionType, "duration_ms", res.DurationMs)
			}
		}
		return res
	}

	// Handle synthetic unit conversion card directly when not in SQLite.
	if cardID == "builtin-unit-convert" {
		if convCard := search.TryConvertQuery(parsed.Text); convCard != nil {
			return recordMetric(s.backend.Runner.Execute(ctx, *convCard, actionID, parsed, extraArgs)), nil
		}
	}

	card, err := s.backend.Store.GetCard(ctx, cardID)
	if err != nil {
		return recordMetric(plugins.ActionResult{Success: false, Error: err.Error()}), err
	}
	if card == nil {
		// Check dynamic plugin cards if not in SQLite.
		if dynCards, dynErr := s.backend.Plugins.QueryPlugins(ctx, parsed); dynErr == nil {
			for i := range dynCards {
				if dynCards[i].ID == cardID {
					card = &dynCards[i]
					break
				}
			}
		}
	}
	if card == nil {
		err := fmt.Errorf("card %q not found", cardID)
		return recordMetric(plugins.ActionResult{Success: false, Error: err.Error()}), err
	}

	return recordMetric(s.backend.Runner.Execute(ctx, *card, actionID, parsed, extraArgs)), nil
}

func (s *AppService) UpsertCard(card models.SearchCard) (*models.SearchCard, error) {
	if err := models.ValidateAndNormalizeCard(&card, card.Collection); err != nil {
		return nil, err
	}
	savedPath, err := indexer.UpsertCardToDisk(s.backend.DataDir, card)
	if err != nil {
		return nil, err
	}
	if err := s.backend.Store.UpsertCard(context.Background(), card, savedPath); err != nil {
		return nil, err
	}
	return &card, nil
}

func (s *AppService) SampleRawData(rawContent, filenameHint string) (*models.RawDataSample, error) {
	return indexer.SampleRawData([]byte(rawContent), filenameHint)
}

func (s *AppService) GenerateETLMappingWithPreview(rawContent, filenameHint, userHint string) (*ai.ETLMappingPreview, error) {
	return s.backend.AI.GenerateETLMappingWithPreview(context.Background(), []byte(rawContent), filenameHint, userHint)
}

func (s *AppService) ImportWithRule(rawContent string, rule models.ETLMappingRule) (*ImportResult, error) {
	cards, err := indexer.TransformWithRule([]byte(rawContent), rule)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, errors.New("rule produced 0 cards")
	}
	col := cards[0].Collection
	savedPath, _, err := indexer.BatchUpsertCardsToDisk(s.backend.DataDir, col, cards)
	if err != nil {
		return nil, err
	}
	loadedCol, loadedCards, err := indexer.LoadCollectionFile(savedPath)
	if err != nil {
		return nil, err
	}
	if err := s.backend.Store.ReplaceCollection(context.Background(), loadedCol, loadedCards, savedPath); err != nil {
		return nil, err
	}
	return &ImportResult{
		Collection:    loadedCol,
		ImportedCount: len(cards),
		FilePath:      savedPath,
	}, nil
}

func (s *AppService) AskRAG(req ai.RAGRequest) (*ai.RAGResponse, error) {
	return s.backend.AI.AskRAG(context.Background(), req, nil)
}

func (s *AppService) GetAIConfig() ai.Config {
	return s.backend.AI.GetConfig()
}

func (s *AppService) GetUserPrefs() UserPrefs {
	return s.backend.Config().Prefs
}

func (s *AppService) SetUserPrefs(prefs UserPrefs) error {
	s.backend.mu.Lock()
	s.backend.cfg.Prefs = prefs
	snapshot := s.backend.cfg
	s.backend.mu.Unlock()
	if _, err := indexer.SyncModules(context.Background(), s.backend.Store, s.backend.ModulesDir, prefs.EnabledModules); err != nil {
		return err
	}
	s.backend.Search.SetPatternRules(indexer.LoadActivePatternRules(s.backend.PatternsDir, s.backend.ModulesDir, prefs.EnabledModules))
	return saveRuntimeConfig(s.backend.ConfigPath, snapshot)
}

func (s *AppService) ListModules() ([]models.ModuleStatus, error) {
	prefs := s.GetUserPrefs()
	return indexer.SyncModules(context.Background(), s.backend.Store, s.backend.ModulesDir, prefs.EnabledModules)
}

func (s *AppService) SetEnabledModules(moduleIDs []string) ([]models.ModuleStatus, error) {
	if moduleIDs == nil {
		moduleIDs = []string{}
	}
	s.backend.mu.Lock()
	s.backend.cfg.Prefs.EnabledModules = moduleIDs
	snapshot := s.backend.cfg
	s.backend.mu.Unlock()

	statuses, err := indexer.SyncModules(context.Background(), s.backend.Store, s.backend.ModulesDir, moduleIDs)
	if err != nil {
		return nil, err
	}
	s.backend.Search.SetPatternRules(indexer.LoadActivePatternRules(s.backend.PatternsDir, s.backend.ModulesDir, moduleIDs))
	if s.backend.Logger != nil {
		s.backend.Logger.Metrics.ModuleSyncsTotal.Add(1)
		updateInventoryGauges(context.Background(), s.backend.Store, s.backend.Logger.Metrics, statuses)
		s.backend.Logger.Component("modules").Info("modules_updated", "enabled_count", len(moduleIDs), "active_cards", s.backend.Logger.Metrics.ActiveCards.Load())
	}
	if err := saveRuntimeConfig(s.backend.ConfigPath, snapshot); err != nil {
		return statuses, err
	}
	return statuses, nil
}

func (s *AppService) ImportModuleJSON(rawJSON string) (*models.ModuleStatus, error) {
	var manifest models.ModuleManifest
	if err := json.Unmarshal([]byte(rawJSON), &manifest); err != nil {
		return nil, fmt.Errorf("invalid module JSON: %w", err)
	}
	saved, err := indexer.SaveCustomModule(s.backend.ModulesDir, manifest)
	if err != nil {
		return nil, err
	}

	// Automatically enable the newly imported module
	prefs := s.GetUserPrefs()
	enabled := append([]string(nil), prefs.EnabledModules...)
	if prefs.EnabledModules == nil {
		installed, _, _ := indexer.LoadAllModules(s.backend.ModulesDir)
		for _, m := range installed {
			if m.DefaultEnabled {
				enabled = append(enabled, m.ID)
			}
		}
	}
	has := false
	for _, id := range enabled {
		if id == saved.ID {
			has = true
			break
		}
	}
	if !has {
		enabled = append(enabled, saved.ID)
	}
	statuses, err := s.SetEnabledModules(enabled)
	if err != nil {
		return nil, err
	}
	for i := range statuses {
		if statuses[i].ID == saved.ID {
			return &statuses[i], nil
		}
	}
	return &models.ModuleStatus{
		ID:           saved.ID,
		Name:         saved.Name,
		Category:     saved.Category,
		Description:  saved.Description,
		Version:      saved.Version,
		Enabled:      true,
		CardCount:    len(saved.Cards),
		OverlayCount: len(saved.Overlays),
		PatternCount: len(saved.Patterns),
	}, nil
}

// ListStarterModules lists all packaged optional modules that can be installed/loaded
// at onboarding or from the Module Catalog.
func (s *AppService) ListStarterModules() []models.StarterModuleInfo {
	return indexer.ListStarterModules(s.backend.ModulesDir)
}

// InstallStarterModules installs the selected optional starter modules as editable,
// deletable JSON files in ~/.vex/modules/ and enables them.
func (s *AppService) InstallStarterModules(starterIDs []string) ([]models.ModuleStatus, error) {
	installed, err := indexer.InstallStarterModules(s.backend.ModulesDir, starterIDs)
	if err != nil {
		return nil, err
	}
	prefs := s.GetUserPrefs()
	enabled := append([]string(nil), prefs.EnabledModules...)
	if prefs.EnabledModules == nil {
		all, _, _ := indexer.LoadAllModules(s.backend.ModulesDir)
		for _, m := range all {
			if m.DefaultEnabled {
				enabled = append(enabled, m.ID)
			}
		}
	}
	seen := make(map[string]bool, len(enabled))
	for _, id := range enabled {
		seen[id] = true
	}
	for _, m := range installed {
		if !seen[m.ID] {
			seen[m.ID] = true
			enabled = append(enabled, m.ID)
		}
	}
	return s.SetEnabledModules(enabled)
}

// DeleteModule permanently removes a module JSON file from ~/.vex/modules/ and re-syncs.
func (s *AppService) DeleteModule(moduleID string) ([]models.ModuleStatus, error) {
	if err := indexer.DeleteModule(s.backend.ModulesDir, moduleID); err != nil {
		return nil, err
	}
	target := models.Slugify(moduleID)
	prefs := s.GetUserPrefs()
	var next []string
	if prefs.EnabledModules != nil {
		next = make([]string, 0, len(prefs.EnabledModules))
		for _, id := range prefs.EnabledModules {
			if models.Slugify(id) != target {
				next = append(next, id)
			}
		}
	}
	return s.SetEnabledModules(next)
}

func (s *AppService) ExportModuleJSON(moduleID string) (string, error) {
	all, _, err := indexer.LoadAllModules(s.backend.ModulesDir)
	if err != nil {
		return "", err
	}
	for _, m := range all {
		if m.ID == moduleID {
			raw, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				return "", err
			}
			return string(raw) + "\n", nil
		}
	}
	return "", fmt.Errorf("module %q not found", moduleID)
}

func (s *AppService) GetRelatedCards(cardID string, limit int) ([]models.SearchCard, error) {
	return s.backend.Store.FindRelatedCards(context.Background(), cardID, limit)
}

// ImportGitHubReposJSON parses a lightweight GitHub repository metadata JSON array
// (from GitHub REST API or `gh repo list --json name,description,url,primaryLanguage`),
// builds the gt-<portfolio>-<team>-<product>-<component> catalog module, saves it to modulesDir, and enables it.
func (s *AppService) ImportGitHubReposJSON(rawJSON string) (*models.ModuleStatus, error) {
	entries, err := indexer.ParseGitHubRepoListJSON([]byte(rawJSON))
	if err != nil {
		return nil, err
	}
	mod := indexer.BuildGitHubCatalogModule(entries)
	rawMod, err := json.Marshal(mod)
	if err != nil {
		return nil, err
	}
	return s.ImportModuleJSON(string(rawMod))
}

func (s *AppService) SetAIConfig(cfg ai.Config) error {
	s.backend.AI.SetConfig(cfg)
	normalized := s.backend.AI.GetConfig()

	if s.backend.Secrets != nil {
		if key := strings.TrimSpace(normalized.APIKey); key != "" {
			if err := s.backend.Secrets.Set(secretServiceName, secretAccountAIAPIKey, key); err != nil {
				return fmt.Errorf("store AI API key: %w", err)
			}
		} else {
			if err := s.backend.Secrets.Delete(secretServiceName, secretAccountAIAPIKey); err != nil {
				return fmt.Errorf("delete AI API key: %w", err)
			}
		}
	}

	if strings.TrimSpace(normalized.EmbeddingModel) != "" {
		s.backend.Search.SetEmbedder(func(ctx context.Context, text string) ([]float32, string, error) {
			embedCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			return s.backend.AI.Embed(embedCtx, text)
		})
	} else {
		s.backend.Search.SetEmbedder(nil)
	}

	s.backend.mu.Lock()
	scrubbedAI := normalized
	scrubbedAI.APIKey = ""
	s.backend.cfg.AI = scrubbedAI
	snapshot := s.backend.cfg
	s.backend.mu.Unlock()
	return saveRuntimeConfig(s.backend.ConfigPath, snapshot)
}

func (s *AppService) SyncEmbeddings() (int, error) {
	return s.backend.AI.SyncMissingEmbeddings(context.Background())
}

func (s *AppService) ListCollections() ([]indexer.CollectionStats, error) {
	return s.backend.Store.ListCollections(context.Background())
}

func (s *AppService) ListMCPServers() []mcp.ConnectedServerStatus {
	return s.backend.MCPClients.ListServers()
}

// ListPatternRules returns all active reference-decoding and live-lookup PatternRules.
func (s *AppService) ListPatternRules() []models.PatternRule {
	return s.backend.Search.PatternRules()
}

// UpsertPatternRule validates, saves to ~/.vex/patterns/<id>.json, and hot-reloads the live PatternEngine.
func (s *AppService) UpsertPatternRule(rule models.PatternRule) ([]models.PatternRule, error) {
	if _, err := indexer.SavePatternRule(s.backend.PatternsDir, rule); err != nil {
		return nil, err
	}
	prefs := s.GetUserPrefs()
	s.backend.Search.SetPatternRules(indexer.LoadActivePatternRules(s.backend.PatternsDir, s.backend.ModulesDir, prefs.EnabledModules))
	return s.backend.Search.PatternRules(), nil
}

// DeletePatternRule removes a standalone pattern rule from ~/.vex/patterns/ and reloads the PatternEngine.
func (s *AppService) DeletePatternRule(ruleID string) ([]models.PatternRule, error) {
	if err := indexer.DeletePatternRule(s.backend.PatternsDir, ruleID); err != nil {
		return nil, err
	}
	prefs := s.GetUserPrefs()
	s.backend.Search.SetPatternRules(indexer.LoadActivePatternRules(s.backend.PatternsDir, s.backend.ModulesDir, prefs.EnabledModules))
	return s.backend.Search.PatternRules(), nil
}

// TestPatternRule evaluates a draft PatternRule against sampleInput and returns the resulting Rolodex CardNode.
func (s *AppService) TestPatternRule(rule models.PatternRule, sampleInput string) (*models.CardNode, error) {
	res, err := search.TestRule(context.Background(), rule, sampleInput, s.backend.Store, s.backend.MCPClients)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	node := res.Card.ToCardNode(0)
	return &node, nil
}

// SearchRolodex executes search + pattern decoding and projects results directly into Rolodex CardNodes.
func (s *AppService) SearchRolodex(rawQuery string, limit int) ([]models.CardNode, error) {
	resp, err := s.Search(rawQuery, limit)
	if err != nil {
		return nil, err
	}
	nodes := make([]models.CardNode, 0, len(resp.Results))
	for _, r := range resp.Results {
		nodes = append(nodes, r.Card.ToCardNode(0))
	}
	return nodes, nil
}

// ProbeCLIAuth runs read-only identity checks (`gh auth status`, `az account show`)
// without ever requesting or caching bearer tokens.
func (s *AppService) ProbeCLIAuth() []mcp.CLIAuthStatus {
	return mcp.ProbeCLIAuth(context.Background(), nil)
}

// GetRecommendedMCPPresets returns opt-in, localOnly-enforced MCP server config templates
// for official GitHub and Azure DevOps MCP servers.
func (s *AppService) GetRecommendedMCPPresets() []mcp.ExternalServerConfig {
	return mcp.RecommendedMCPPresets()
}

// FetchWeather fetches lightweight current weather from Open-Meteo (no API key required)
// with a 10-minute in-memory cache and 2.5s timeout.
func (s *AppService) FetchWeather(lat, lon float64) (*WeatherInfo, error) {
	cacheKey := fmt.Sprintf("%.2f,%.2f", lat, lon)
	s.mu.RLock()
	if s.weatherCache != nil {
		if entry, ok := s.weatherCache[cacheKey]; ok && time.Since(entry.fetchedAt) < 10*time.Minute {
			info := entry.info
			s.mu.RUnlock()
			return &info, nil
		}
	}
	s.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current=temperature_2m,weather_code",
		lat, lon,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather service status %d", resp.StatusCode)
	}

	var payload struct {
		Current struct {
			Temperature2m float64 `json:"temperature_2m"`
			WeatherCode   int     `json:"weather_code"`
		} `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	tempC := payload.Current.Temperature2m
	info := WeatherInfo{
		TempC:     tempC,
		TempF:     tempC*9.0/5.0 + 32.0,
		Condition: wmoWeatherLabel(payload.Current.WeatherCode),
		CachedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	s.mu.Lock()
	if s.weatherCache == nil {
		s.weatherCache = make(map[string]weatherCacheEntry)
	}
	s.weatherCache[cacheKey] = weatherCacheEntry{info: info, fetchedAt: time.Now()}
	s.mu.Unlock()

	return &info, nil
}

func wmoWeatherLabel(code int) string {
	switch {
	case code == 0:
		return "Clear"
	case code <= 3:
		return "Partly Cloudy"
	case code == 45 || code == 48:
		return "Foggy"
	case code >= 51 && code <= 67:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Showers"
	case code >= 95:
		return "Thunderstorm"
	default:
		return "Cloudy"
	}
}

func (s *AppService) GetLogInfo() logging.LogDiagnostics {
	if s.backend.Logger == nil {
		return logging.LogDiagnostics{}
	}
	cfg := s.backend.Config()
	metricsAddr := ""
	if cfg.MCPAddr != "" && !strings.EqualFold(cfg.MCPAddr, "off") {
		metricsAddr = "http://" + cfg.MCPAddr + "/metrics"
	}
	return s.backend.Logger.Diagnostics(metricsAddr)
}

func (s *AppService) GetRecentLogs(limit int) ([]logging.LogEntry, error) {
	if s.backend.Logger == nil {
		return []logging.LogEntry{}, nil
	}
	return s.backend.Logger.ReadRecentEntries(limit)
}

func (s *AppService) SetLogLevel(level string) (string, error) {
	if s.backend.Logger == nil {
		return "info", nil
	}
	applied := s.backend.Logger.SetLevel(level)
	s.backend.mu.Lock()
	s.backend.cfg.LogLevel = applied
	snapshot := s.backend.cfg
	s.backend.mu.Unlock()
	s.backend.Logger.Component("app").Info("log_level_changed", "level", applied)
	if err := saveRuntimeConfig(s.backend.ConfigPath, snapshot); err != nil {
		return applied, err
	}
	return applied, nil
}

func (s *AppService) ClearLogs() error {
	if s.backend.Logger == nil {
		return nil
	}
	return s.backend.Logger.Clear()
}

func (s *AppService) GetAppInfo() AppInfo {
	return currentAppInfo()
}

func (s *AppService) GetAppVersion() string {
	return currentAppInfo().Version
}

func (s *AppService) GetUpdateFeedURL() string {
	if u := strings.TrimSpace(s.backend.Config().UpdateFeedURL); u != "" {
		return u
	}
	return strings.TrimSpace(UpdateFeedURL)
}

func (s *AppService) SetUpdateFeedURL(feedURL string) error {
	cleaned := strings.TrimSpace(feedURL)
	s.backend.mu.Lock()
	s.backend.cfg.UpdateFeedURL = cleaned
	snapshot := s.backend.cfg
	s.backend.mu.Unlock()
	if s.backend.Logger != nil {
		s.backend.Logger.Component("updater").Info("update_feed_updated", "configured", cleaned != "")
	}
	return saveRuntimeConfig(s.backend.ConfigPath, snapshot)
}

func (s *AppService) CheckForUpdates() (*updater.CheckResult, error) {
	feed := s.GetUpdateFeedURL()
	chk := s.backend.Updater
	if chk == nil {
		chk = updater.NewChecker()
	}
	res, err := chk.Check(context.Background(), currentAppInfo().Version, feed)
	if s.backend.Logger != nil {
		if err != nil {
			s.backend.Logger.Component("updater").Warn("update_check_failed", "error", err.Error())
		} else {
			s.backend.Logger.Component("updater").Info("update_check_completed", "current", res.CurrentVersion, "latest", res.LatestVersion, "available", res.UpdateAvailable)
		}
	}
	return res, err
}

func (s *AppService) HideWindow() {
	s.mu.RLock()
	fn := s.onHide
	s.mu.RUnlock()
	if fn != nil {
		fn()
	}
}
