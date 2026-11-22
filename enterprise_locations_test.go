package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"vex/internal/ai"
	"vex/internal/indexer"
	"vex/internal/models"
	"vex/internal/search"
	"vex/internal/secrets"
)

var (
	reIATA     = regexp.MustCompile(`^[A-Z]{3}$`)
	reUNLOCODE = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{3}$`)
	reISO2     = regexp.MustCompile(`^[A-Z]{2}$`)
)

type locationsRoundTripFunc func(req *http.Request) (*http.Response, error)

func (f locationsRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func makeMockHTTPResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func validateLocationsManifest(t *testing.T, mod models.ModuleManifest, checkTargetRange bool) {
	t.Helper()

	if strings.TrimSpace(mod.ID) == "" {
		t.Fatalf("module id is required")
	}
	if mod.DefaultEnabled {
		t.Fatalf("enterprise module %q must set defaultEnabled=false", mod.ID)
	}
	if !mod.LocalOnly {
		t.Fatalf("enterprise module %q must set localOnly=true", mod.ID)
	}
	if checkTargetRange && (len(mod.Cards) < 180 || len(mod.Cards) > 200) {
		t.Fatalf("expected consolidated card count in [180, 200], got %d", len(mod.Cards))
	}

	seenIDs := make(map[string]struct{}, len(mod.Cards))
	seenEntityKeys := make(map[string]struct{}, len(mod.Cards))
	seenIATA := make(map[string]struct{}, len(mod.Cards))

	for i, orig := range mod.Cards {
		card := orig
		if err := models.ValidateAndNormalizeCard(&card, "ports"); err != nil {
			t.Fatalf("card %d (%q) failed ValidateAndNormalizeCard: %v", i, orig.ID, err)
		}
		if _, dup := seenIDs[card.ID]; dup {
			t.Fatalf("duplicate card id %q", card.ID)
		}
		seenIDs[card.ID] = struct{}{}

		ekLower := strings.ToLower(strings.TrimSpace(card.EntityKey))
		if !strings.HasPrefix(ekLower, "loc:") {
			t.Fatalf("card %q expected entityKey prefix loc:, got %q", card.ID, card.EntityKey)
		}
		if _, dup := seenEntityKeys[ekLower]; dup {
			t.Fatalf("duplicate entityKey %q on card %q", card.EntityKey, card.ID)
		}
		seenEntityKeys[ekLower] = struct{}{}

		if !card.LocalOnly {
			t.Fatalf("card %q must set localOnly=true", card.ID)
		}
		if card.Provenance == nil {
			t.Fatalf("card %q missing provenance block", card.ID)
		}
		if !card.Provenance.LocalOnly || card.Provenance.Authority != "official" || card.Provenance.SourceName == "" || card.Provenance.SourceURL == "" || card.Provenance.VerifiedAt == "" {
			t.Fatalf("card %q has incomplete or non-localOnly provenance: %+v", card.ID, card.Provenance)
		}

		// Live weather, local timezone, and GlancePills data contract checks
		iata, _ := card.Data["iata"].(string)
		unlocode, _ := card.Data["unlocode"].(string)
		cc, _ := card.Data["country_code"].(string)
		country, _ := card.Data["country"].(string)
		currency, _ := card.Data["currency"].(string)
		tz, _ := card.Data["timezone"].(string)
		address, _ := card.Data["address"].(string)

		if !reIATA.MatchString(iata) {
			t.Fatalf("card %q invalid IATA %q", card.ID, iata)
		}
		if card.EntityKey != "loc:"+iata {
			t.Fatalf("card %q entityKey %q does not match IATA %q", card.ID, card.EntityKey, iata)
		}
		if _, dup := seenIATA[iata]; dup {
			t.Fatalf("duplicate IATA code %q on card %q", iata, card.ID)
		}
		seenIATA[iata] = struct{}{}

		if !reUNLOCODE.MatchString(unlocode) {
			t.Fatalf("card %q invalid UN/LOCODE %q", card.ID, unlocode)
		}
		if !reISO2.MatchString(cc) {
			t.Fatalf("card %q invalid country_code %q", card.ID, cc)
		}
		if !strings.HasPrefix(unlocode, cc) {
			t.Fatalf("card %q UN/LOCODE %q does not match country_code %q", card.ID, unlocode, cc)
		}
		if strings.TrimSpace(country) == "" || strings.TrimSpace(currency) == "" || strings.TrimSpace(address) == "" {
			t.Fatalf("card %q missing required country/currency/address metadata", card.ID)
		}

		if _, err := time.LoadLocation(tz); err != nil {
			t.Fatalf("card %q invalid IANA timezone %q: %v", card.ID, tz, err)
		}

		lat, okLat := card.Data["lat"].(float64)
		lon, okLon := card.Data["lon"].(float64)
		if !okLat || !okLon || lat < -90 || lat > 90 || lon < -180 || lon > 180 || (lat == 0 && lon == 0) {
			t.Fatalf("card %q invalid numeric coordinates lat=%v lon=%v", card.ID, card.Data["lat"], card.Data["lon"])
		}

		expectedPill := unlocode + " · " + iata
		if len(card.GlancePills) == 0 || card.GlancePills[0] != expectedPill {
			t.Fatalf("card %q expected first GlancePill %q, got %v", card.ID, expectedPill, card.GlancePills)
		}
	}
}

func verifyOverlaysAndCloudRAGExclusion(t *testing.T, modulesDir, moduleID, amsSecretToken string) {
	t.Helper()

	all, isBuiltIn, err := indexer.LoadAllModules(modulesDir)
	if err != nil {
		t.Fatalf("LoadAllModules: %v", err)
	}
	if isBuiltIn[moduleID] {
		t.Fatalf("external module %q must not be marked builtIn", moduleID)
	}

	// Disable geographies; enable external locations + air-freight + ocean-freight
	compiled := indexer.CompileActiveModules(all, []string{moduleID, "air-freight", "ocean-freight"})

	checkGateway := func(entityKey string) {
		var found *models.SearchCard
		for i := range compiled {
			if strings.EqualFold(compiled[i].EntityKey, entityKey) && compiled[i].ModuleID == moduleID {
				found = &compiled[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected compiled card for %s from module %q", entityKey, moduleID)
		}
		if !found.LocalOnly || found.Provenance == nil || !found.Provenance.LocalOnly {
			t.Fatalf("expected localOnly=true to propagate on %s card and provenance", entityKey)
		}
		hasAir, hasOcean := false, false
		for _, sec := range found.Sections {
			if sec.ModuleID == "air-freight" {
				hasAir = true
			}
			if sec.ModuleID == "ocean-freight" {
				hasOcean = true
			}
		}
		if !hasAir || !hasOcean {
			t.Fatalf("expected air-freight and ocean-freight overlays on %s (got air=%v ocean=%v sections=%+v)", entityKey, hasAir, hasOcean, found.Sections)
		}
	}

	checkGateway("loc:AMS")
	checkGateway("loc:SEA")

	// Index into in-memory SQLite and verify cloud LLM RAG strips localOnly cards while Ollama retains them.
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.ReplaceSourceFile(ctx, indexer.CompiledModulesSource, compiled); err != nil {
		t.Fatalf("ReplaceSourceFile: %v", err)
	}
	searchSvc := search.NewService(store)

	// Verify local search still finds the localOnly card
	localRes, err := searchSvc.Search(ctx, "AMS", 10)
	if err != nil || localRes == nil {
		t.Fatalf("local search failed: %v", err)
	}
	foundLocal := false
	for _, r := range localRes.Results {
		if r.Card.ModuleID == moduleID && strings.EqualFold(r.Card.EntityKey, "loc:AMS") {
			foundLocal = true
			break
		}
	}
	if !foundLocal {
		t.Fatalf("expected local search to return loc:AMS from %q", moduleID)
	}

	// Verify cloud LLM providers (OpenAI, Anthropic, Gemini) exclude localOnly cards from RAG context
	cloudProviders := []struct {
		provider ai.ProviderType
		respJSON string
	}{
		{ai.ProviderOpenAI, `{"choices":[{"message":{"content":"No private context."}}]}`},
		{ai.ProviderAnthropic, `{"content":[{"type":"text","text":"No private context."}]}`},
		{ai.ProviderGemini, `{"candidates":[{"content":{"parts":[{"text":"No private context."}]}}]}`},
	}

	for _, tc := range cloudProviders {
		var captured string
		aiSvc := ai.NewService(searchSvc, store, ai.Config{
			Provider: tc.provider,
			APIKey:   "test-key",
		})
		aiSvc.SetHTTPClient(&http.Client{
			Transport: locationsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(req.Body)
				captured = string(b)
				return makeMockHTTPResp(200, tc.respJSON), nil
			}),
		})

		resp, err := aiSvc.AskRAG(ctx, ai.RAGRequest{Query: "AMS NLAMS", Collection: "ports", Limit: 5}, nil)
		if err != nil {
			t.Fatalf("AskRAG (%s): %v", tc.provider, err)
		}
		for _, src := range resp.Sources {
			if strings.HasPrefix(src.CardID, "exp-loc-") || strings.HasPrefix(src.CardID, "fixture-loc-") {
				t.Fatalf("cloud provider %s leaked localOnly location card %q in sources", tc.provider, src.CardID)
			}
		}
		if amsSecretToken != "" && strings.Contains(captured, amsSecretToken) {
			t.Fatalf("cloud provider %s leaked localOnly branch address token %q into prompt", tc.provider, amsSecretToken)
		}
	}

	// Verify local Ollama retains full access to localOnly location cards
	var ollamaPrompt string
	ollamaSvc := ai.NewService(searchSvc, store, ai.Config{
		Provider:  ai.ProviderOllama,
		ChatModel: "llama3.2",
	})
	ollamaSvc.SetHTTPClient(&http.Client{
		Transport: locationsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(req.Body)
			ollamaPrompt = string(b)
			return makeMockHTTPResp(200, `{"message":{"role":"assistant","content":"Local AMS answer [1]."}}`), nil
		}),
	})
	ollamaResp, err := ollamaSvc.AskRAG(ctx, ai.RAGRequest{Query: "AMS NLAMS", Collection: "ports", Limit: 5}, nil)
	if err != nil {
		t.Fatalf("AskRAG (ollama): %v", err)
	}
	if len(ollamaResp.Sources) == 0 {
		t.Fatalf("expected local Ollama RAG to retain localOnly location card sources")
	}
	if amsSecretToken != "" && !strings.Contains(ollamaPrompt, amsSecretToken) {
		t.Fatalf("expected local Ollama prompt to contain %q", amsSecretToken)
	}
}

func TestExternalLocationsModuleFixtureContract(t *testing.T) {
	fixture := models.ModuleManifest{
		ID:             "enterprise-locations-fixture",
		Name:           "Enterprise Locations Fixture",
		Category:       "Core & Locations",
		Description:    "Generic fixture testing external enterprise locations contract, overlays, and localOnly isolation.",
		Icon:           "globe",
		Version:        "1.0.0",
		DefaultEnabled: false,
		LocalOnly:      true,
		Cards: []models.SearchCard{
			{
				ID:          "fixture-loc-ams",
				EntityKey:   "loc:AMS",
				ModuleID:    "enterprise-locations-fixture",
				Title:       "Amsterdam — Enterprise Gateway (AMS / NLAMS)",
				Subtitle:    "Netherlands (NL) • UN/LOCODE NLAMS • IATA AMS • EUR (€)",
				Description: "Generic fixture card for Amsterdam gateway.",
				Collection:  "ports",
				Template:    models.TemplateKeyValue,
				Icon:        "globe",
				Tags:        []string{"ams", "nlams", "amsterdam", "netherlands"},
				Modifiers:   []string{"port", "iata"},
				RelatedKeys: []string{"AMS", "NLAMS", "NL"},
				LocalOnly:   true,
				Data: map[string]any{
					"iata":         "AMS",
					"unlocode":     "NLAMS",
					"country":      "Netherlands",
					"country_code": "NL",
					"currency":     "EUR (€)",
					"timezone":     "Europe/Amsterdam",
					"address":      "FIXTURE_AMS_SCHIPHOL_ADDR_991",
					"lat":          52.3105,
					"lon":          4.7683,
				},
				Actions: []models.CardAction{
					{ID: "copy-iata", Label: "Copy AMS", Shortcut: "Enter", Type: "copy", Target: "AMS"},
				},
				Provenance: &models.Provenance{
					SourceName: "Enterprise Locations Fixture",
					SourceURL:  "https://example.com/locations",
					VerifiedAt: "2026-11-22",
					Authority:  "official",
					LocalOnly:  true,
				},
			},
			{
				ID:          "fixture-loc-sea",
				EntityKey:   "loc:SEA",
				ModuleID:    "enterprise-locations-fixture",
				Title:       "Seattle — Enterprise Gateway (SEA / USSEA)",
				Subtitle:    "United States (US) • UN/LOCODE USSEA • IATA SEA • USD ($)",
				Description: "Generic fixture card for Seattle gateway.",
				Collection:  "ports",
				Template:    models.TemplateKeyValue,
				Icon:        "globe",
				Tags:        []string{"sea", "ussea", "seattle", "united states"},
				Modifiers:   []string{"port", "iata"},
				RelatedKeys: []string{"SEA", "USSEA", "US"},
				LocalOnly:   true,
				Data: map[string]any{
					"iata":         "SEA",
					"unlocode":     "USSEA",
					"country":      "United States",
					"country_code": "US",
					"currency":     "USD ($)",
					"timezone":     "America/Los_Angeles",
					"address":      "FIXTURE_SEA_GATEWAY_ADDR_992",
					"lat":          47.4502,
					"lon":          -122.3088,
				},
				Actions: []models.CardAction{
					{ID: "copy-iata", Label: "Copy SEA", Shortcut: "Enter", Type: "copy", Target: "SEA"},
				},
				Provenance: &models.Provenance{
					SourceName: "Enterprise Locations Fixture",
					SourceURL:  "https://example.com/locations",
					VerifiedAt: "2026-11-22",
					Authority:  "official",
					LocalOnly:  true,
				},
			},
		},
	}

	validateLocationsManifest(t, fixture, false)

	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	modulesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modulesDir, fixture.ID+".json"), raw, 0o644); err != nil {
		t.Fatalf("write fixture module: %v", err)
	}

	verifyOverlaysAndCloudRAGExclusion(t, modulesDir, fixture.ID, "FIXTURE_AMS_SCHIPHOL_ADDR_991")

	// Also verify AppService.ImportModuleJSON end-to-end with zero core code changes.
	backend, err := InitBackendWithSecrets(t.TempDir(), false, secrets.NewMemoryStore())
	if err != nil {
		t.Fatalf("InitBackendWithSecrets: %v", err)
	}
	defer backend.Close()

	status, err := backend.AppService.ImportModuleJSON(string(raw))
	if err != nil {
		t.Fatalf("ImportModuleJSON: %v", err)
	}
	if status.ID != fixture.ID || !status.Enabled || !status.LocalOnly || status.CardCount != 2 {
		t.Fatalf("unexpected imported module status: %+v", status)
	}
}

func TestEnterpriseLocationsModuleJSON(t *testing.T) {
	modulePath := filepath.Join("modules-enterprise", "expeditors-locations.json")
	raw, err := os.ReadFile(modulePath)
	if os.IsNotExist(err) {
		t.Skipf("external enterprise module %s not present (git-ignored in public/CI builds)", modulePath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", modulePath, err)
	}

	var manifest models.ModuleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("unmarshal %s: %v", modulePath, err)
	}

	validateLocationsManifest(t, manifest, true)

	// Extract the AMS card's street address token to assert zero leakage to cloud LLMs.
	var amsAddr string
	for _, c := range manifest.Cards {
		if strings.EqualFold(c.EntityKey, "loc:AMS") {
			amsAddr, _ = c.Data["address"].(string)
			break
		}
	}
	if strings.TrimSpace(amsAddr) == "" {
		t.Fatalf("expected non-empty street address on loc:AMS card")
	}

	modulesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modulesDir, manifest.ID+".json"), raw, 0o644); err != nil {
		t.Fatalf("stage module in temp dir: %v", err)
	}

	verifyOverlaysAndCloudRAGExclusion(t, modulesDir, manifest.ID, amsAddr)

	// Verify AppService.ImportModuleJSON loads, validates, saves, and indexes all consolidated cards cleanly.
	backend, err := InitBackendWithSecrets(t.TempDir(), false, secrets.NewMemoryStore())
	if err != nil {
		t.Fatalf("InitBackendWithSecrets: %v", err)
	}
	defer backend.Close()

	status, err := backend.AppService.ImportModuleJSON(string(raw))
	if err != nil {
		t.Fatalf("AppService.ImportModuleJSON: %v", err)
	}
	if status.ID != "expeditors-locations" || !status.LocalOnly || status.DefaultEnabled || status.CardCount != len(manifest.Cards) {
		t.Fatalf("unexpected imported status: %+v", status)
	}
}
