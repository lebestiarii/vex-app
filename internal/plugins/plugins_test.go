package plugins

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vex/internal/models"
	"vex/internal/search"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPluginManagerLoadAndDynamicSources(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "sysinfo")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifestJSON := `{
		"id": "sysinfo",
		"name": "System Echo Plugin",
		"description": "Returns dynamic cards from a local endpoint",
		"modifiers": ["sys", "echo"],
		"icon": "activity",
		"staticCards": [
			{
				"id": "sys-static-1",
				"title": "System Overview",
				"subtitle": "Static plugin card",
				"layout": "facts",
				"data": {"summary": "Static system overview."}
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr := NewManager()
	if err := mgr.LoadDir(tmpDir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	loaded := mgr.ListPlugins()
	if len(loaded) != 1 || len(loaded[0].StaticCards) != 1 || loaded[0].StaticCards[0].ID != "sys-static-1" {
		t.Fatalf("expected 1 loaded plugin with sys-static-1, got %+v", loaded)
	}

	// Test dynamic HTTP plugin returning arbitrary JSON wrapped as facts card (in-memory RoundTripper)
	mgr.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body := `{"status":"healthy","latency_ms":14,"env":"` + r.URL.Query().Get("env") + `"}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	if err := mgr.Register(PluginManifest{
		ID:        "health",
		Name:      "Cluster Health",
		Modifiers: []string{"health"},
		DynamicSource: &DynamicSourceConfig{
			Type:           "http",
			URL:            "http://health.local/status?env={{arg1}}",
			ResponseFormat: "facts",
		},
	}); err != nil {
		t.Fatalf("Register HTTP plugin: %v", err)
	}

	pqHTTP := search.Parse("/health staging")
	httpCards, err := mgr.QueryPlugins(context.Background(), pqHTTP)
	if err != nil {
		t.Fatalf("QueryPlugins http: %v", err)
	}
	if len(httpCards) != 1 || httpCards[0].Layout != models.LayoutFacts {
		t.Fatalf("expected 1 facts card, got %+v", httpCards)
	}
	body := httpCards[0].SearchableBody()
	if !strings.Contains(body, "staging") || !strings.Contains(body, "healthy") {
		t.Errorf("unexpected HTTP facts body: %q", body)
	}
}

func TestInterpolateAndActionRunner(t *testing.T) {
	card := models.SearchCard{
		ID:         "deploy-card",
		Title:      "Deploy Service",
		Collection: "devops",
		Layout:     models.LayoutSnippet,
		Data:       map[string]any{"body": "ssh ops@10.0.0.9", "language": "bash"},
		Actions: []models.CardAction{
			{ID: "shell-act", Type: "shell", Target: `printf '{"env":"{{env}}","arg":"{{arg1}}"}'`},
			{ID: "open-act", Type: "open_url", Target: "https://example.com/{{arg1}}?env={{env}}"},
			{ID: "copy-act", Type: "copy", Target: "ops@{{arg1}}"},
			{ID: "ai-act", Type: "ai_prompt", Target: "Explain deployment for {{arg1}}"},
		},
	}

	pq := search.Parse("/deploy env:prod api-gateway")

	var openedKind, openedTarget, copiedText string
	runner := NewRunner()
	runner.OpenFunc = func(_ context.Context, kind, target string) error {
		openedKind = kind
		openedTarget = target
		return nil
	}
	runner.ClipboardFunc = func(_ context.Context, text string) error {
		copiedText = text
		return nil
	}

	ctx := context.Background()

	// 1. Shell action type is rejected by Runner.Execute
	res := runner.Execute(ctx, card, "shell-act", pq, nil)
	if res.Success || !strings.Contains(res.Error, "unsupported action type") {
		t.Fatalf("expected shell action to be rejected, got: %+v", res)
	}

	// 2. Open URL with extraArgs override
	resOpen := runner.Execute(ctx, card, "open-act", pq, map[string]string{"env": "canary"})
	if !resOpen.Success || openedKind != "open_url" || openedTarget != "https://example.com/api-gateway?env=canary" {
		t.Fatalf("unexpected open_url result: %+v (kind=%q target=%q)", resOpen, openedKind, openedTarget)
	}

	// 3. Copy action
	resCopy := runner.Execute(ctx, card, "copy-act", pq, nil)
	if !resCopy.Success || copiedText != "ops@api-gateway" {
		t.Fatalf("unexpected copy result: %+v (copied=%q)", resCopy, copiedText)
	}

	// 4. Removed ai_prompt action type is rejected
	resAI := runner.Execute(ctx, card, "ai-act", pq, nil)
	if resAI.Success || !strings.Contains(resAI.Error, "unsupported action type") {
		t.Fatalf("expected removed ai_prompt action to be rejected, got: %+v", resAI)
	}

	// 5. HTTP action (in-memory RoundTripper)
	runner.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	}

	httpCard := models.SearchCard{
		ID:      "webhook-card",
		Title:   "Trigger Webhook",
		Layout:  models.LayoutGroup,
		Actions: []models.CardAction{{ID: "hook", Type: "http", Target: "POST http://webhook.local/trigger"}},
	}
	resHTTP := runner.Execute(ctx, httpCard, "hook", search.Parse(""), nil)
	if !resHTTP.Success || resHTTP.Data["ok"] != true {
		t.Fatalf("unexpected http action result: %+v", resHTTP)
	}
}
