package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vex/internal/indexer"
	"vex/internal/models"
	"vex/internal/plugins"
	"vex/internal/search"

	"github.com/mark3labs/mcp-go/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setupHubForTest(t *testing.T) (*HubServer, *indexer.Store, *search.Service, string, *string) {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()

	if err := indexer.SeedDefaultCollections(dataDir); err != nil {
		t.Fatalf("SeedDefaultCollections: %v", err)
	}
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Load seeded JSON files into store
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			full := filepath.Join(dataDir, e.Name())
			col, cards, err := indexer.LoadCollectionFile(full)
			if err != nil {
				t.Fatalf("LoadCollectionFile %s: %v", full, err)
			}
			if err := store.ReplaceCollection(ctx, col, cards, full); err != nil {
				t.Fatalf("ReplaceCollection %s: %v", col, err)
			}
		}
	}

	pluginMgr := plugins.NewManager()
	if err := pluginMgr.Register(plugins.PluginManifest{
		ID:          "jira",
		Name:        "Jira Quick Actions",
		Description: "Search and open Jira issues",
		Version:     "1.0.0",
		Modifiers:   []string{"jira", "j"},
	}); err != nil {
		t.Fatalf("Register plugin: %v", err)
	}

	var copiedText string
	runner := plugins.NewRunner()
	runner.ClipboardFunc = func(_ context.Context, text string) error {
		copiedText = text
		return nil
	}

	searchSvc := search.NewService(store)
	searchSvc.SetPluginProvider(pluginMgr)

	hub, err := NewHubServer(ServerDeps{
		Store:   store,
		Search:  searchSvc,
		Plugins: pluginMgr,
		Runner:  runner,
		DataDir: dataDir,
		Version: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewHubServer: %v", err)
	}
	return hub, store, searchSvc, dataDir, &copiedText
}

func extractToolText(t *testing.T, res *mcplib.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatalf("nil CallToolResult")
	}
	if res.IsError {
		t.Fatalf("unexpected tool error result: %+v", res.Content)
	}
	for _, c := range res.Content {
		if tc, ok := mcplib.AsTextContent(c); ok {
			return tc.Text
		}
	}
	t.Fatalf("no TextContent in CallToolResult: %+v", res.Content)
	return ""
}

func TestHubServer_AllResourcesToolsAndPrompts(t *testing.T) {
	ctx := context.Background()
	hub, _, _, dataDir, copiedText := setupHubForTest(t)

	cli, err := client.NewInProcessClient(hub.MCPServer())
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("client Start: %v", err)
	}
	defer cli.Close()

	initReq := mcplib.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcplib.Implementation{Name: "test-agent", Version: "1.0"}
	initRes, err := cli.Initialize(ctx, initReq)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initRes.ServerInfo.Name != "Vex" {
		t.Fatalf("expected server name Vex, got %q", initRes.ServerInfo.Name)
	}

	// 1. Resources: search://collections & schema://search-card
	resList, err := cli.ListResources(ctx, mcplib.ListResourcesRequest{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resList.Resources) < 2 {
		t.Fatalf("expected at least 2 static resources, got %d", len(resList.Resources))
	}

	colRes, err := cli.ReadResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceParams{URI: "search://collections"},
	})
	if err != nil {
		t.Fatalf("ReadResource search://collections: %v", err)
	}
	colText, ok := mcplib.AsTextResourceContents(colRes.Contents[0])
	if !ok || !strings.Contains(colText.Text, "devops") || !strings.Contains(colText.Text, "shortcuts") {
		t.Fatalf("unexpected search://collections payload: %+v", colRes.Contents)
	}

	schemaRes, err := cli.ReadResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceParams{URI: "schema://search-card"},
	})
	if err != nil {
		t.Fatalf("ReadResource schema://search-card: %v", err)
	}
	schemaText, ok := mcplib.AsTextResourceContents(schemaRes.Contents[0])
	if !ok || !strings.Contains(schemaText.Text, "ETLMappingRule") {
		t.Fatalf("schema://search-card missing ETLMappingRule")
	}

	// 2. Resource template: card://{collection}/{id}
	cardRes, err := cli.ReadResource(ctx, mcplib.ReadResourceRequest{
		Params: mcplib.ReadResourceParams{URI: "card://devops/devops-env-cheatsheet"},
	})
	if err != nil {
		t.Fatalf("ReadResource card://devops/devops-env-cheatsheet: %v", err)
	}
	cardText, ok := mcplib.AsTextResourceContents(cardRes.Contents[0])
	if !ok || !strings.Contains(cardText.Text, "staging-us-west-2") {
		t.Fatalf("unexpected card resource content: %+v", cardRes.Contents)
	}

	// 3. Tools inventory
	toolsRes, err := cli.ListTools(ctx, mcplib.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsRes.Tools) != 10 {
		t.Fatalf("expected 10 MCP tools, got %d", len(toolsRes.Tools))
	}

	// 4. get_card_schema
	callTool := func(name string, args map[string]any) *mcplib.CallToolResult {
		req := mcplib.CallToolRequest{}
		req.Params.Name = name
		req.Params.Arguments = args
		res, err := cli.CallTool(ctx, req)
		if err != nil {
			t.Fatalf("CallTool %s failed: %v", name, err)
		}
		return res
	}

	schemaToolOut := extractToolText(t, callTool("get_card_schema", nil))
	if !strings.Contains(schemaToolOut, "exampleETLRule") {
		t.Fatalf("get_card_schema missing exampleETLRule: %s", schemaToolOut)
	}

	// 5. search_knowledge
	searchOut := extractToolText(t, callTool("search_knowledge", map[string]any{
		"query":      "staging",
		"collection": "devops",
		"limit":      5,
	}))
	if !strings.Contains(searchOut, "card://devops/devops-env-cheatsheet") {
		t.Fatalf("search_knowledge did not return staging card URI: %s", searchOut)
	}

	// 6. preview_raw_data_sample + transform_and_import_data (CSV file)
	csvPath := filepath.Join(t.TempDir(), "services.csv")
	csvData := "Service,Owner,Dashboard,Tags\nBilling API,Payments,https://billing.internal,\"finance,prod\"\nAuth Gateway,Identity,https://auth.internal,\"security,prod\"\n"
	if err := os.WriteFile(csvPath, []byte(csvData), 0o644); err != nil {
		t.Fatalf("WriteFile csv: %v", err)
	}

	sampleOut := extractToolText(t, callTool("preview_raw_data_sample", map[string]any{
		"file_path": csvPath,
	}))
	if !strings.Contains(sampleOut, `"format": "csv"`) || !strings.Contains(sampleOut, "Billing API") {
		t.Fatalf("unexpected preview_raw_data_sample output: %s", sampleOut)
	}

	importOut := extractToolText(t, callTool("transform_and_import_data", map[string]any{
		"file_path": csvPath,
		"rule": map[string]any{
			"name":             "Services CSV",
			"sourceFormat":     "csv",
			"targetCollection": "services",
			"defaultTemplate":  "shortcut",
			"titleField":       "{{Service}}",
			"subtitleField":    "{{Owner}}",
			"tagsField":        "Tags",
			"actionType":       "open_url",
			"actionLabel":      "Open Dashboard",
			"actionTarget":     "{{Dashboard}}",
		},
	}))
	if !strings.Contains(importOut, `"importedCount": 2`) {
		t.Fatalf("unexpected transform_and_import_data output: %s", importOut)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "services.json")); err != nil {
		t.Fatalf("expected services.json persisted to dataDir: %v", err)
	}

	// Verify immediately searchable after import
	importedSearch := extractToolText(t, callTool("search_knowledge", map[string]any{
		"query":      "Billing",
		"collection": "services",
	}))
	if !strings.Contains(importedSearch, "https://billing.internal") {
		t.Fatalf("imported CSV card not immediately searchable: %s", importedSearch)
	}

	// 7. upsert_card + execute_card_action
	upsertOut := extractToolText(t, callTool("upsert_card", map[string]any{
		"card": map[string]any{
			"id":          "custom-ssh-prod",
			"title":       "SSH Production Bastion",
			"collection":  "devops",
			"template":    "command",
			"description": "Connect to prod bastion",
			"tags":        []string{"ssh", "prod"},
			"data":        map[string]any{"host": "bastion.prod.internal"},
			"actions": []map[string]any{
				{"id": "copy-ssh", "label": "Copy SSH", "type": "copy", "target": "ssh {{user}}@{{data.host}}"},
			},
		},
	}))
	if !strings.Contains(upsertOut, "card://devops/custom-ssh-prod") {
		t.Fatalf("unexpected upsert_card output: %s", upsertOut)
	}

	execOut := extractToolText(t, callTool("execute_card_action", map[string]any{
		"card_id":   "custom-ssh-prod",
		"action_id": "copy-ssh",
		"args":      map[string]any{"user": "alice"},
	}))
	if !strings.Contains(execOut, `"success": true`) || *copiedText != "ssh alice@bastion.prod.internal" {
		t.Fatalf("execute_card_action failed: out=%s copied=%q", execOut, *copiedText)
	}

	// 8. batch_upsert_cards
	batchOut := extractToolText(t, callTool("batch_upsert_cards", map[string]any{
		"collection": "notes",
		"cards": []map[string]any{
			{"title": "Incident Playbook", "template": "knowledge", "description": "Step 1: check Grafana"},
			{"title": "Release Checklist", "template": "knowledge", "description": "Run go test -race"},
		},
	}))
	if !strings.Contains(batchOut, `"upsertedCount": 2`) {
		t.Fatalf("unexpected batch_upsert_cards output: %s", batchOut)
	}

	// 9. list_plugins
	pluginsOut := extractToolText(t, callTool("list_plugins", nil))
	if !strings.Contains(pluginsOut, "Jira Quick Actions") {
		t.Fatalf("unexpected list_plugins output: %s", pluginsOut)
	}

	// 10. rag_context_prompt
	promptRes, err := cli.GetPrompt(ctx, mcplib.GetPromptRequest{
		Params: mcplib.GetPromptParams{
			Name:      "rag_context_prompt",
			Arguments: map[string]string{"topic": "staging", "collection": "devops"},
		},
	})
	if err != nil {
		t.Fatalf("GetPrompt rag_context_prompt: %v", err)
	}
	if len(promptRes.Messages) == 0 {
		t.Fatalf("expected prompt message")
	}
	pmText, ok := mcplib.AsTextContent(promptRes.Messages[0].Content)
	if !ok || !strings.Contains(pmText.Text, "Staging Cluster Endpoints") {
		t.Fatalf("expected staging card in rag_context_prompt, got: %+v", promptRes.Messages[0])
	}
}

func TestHubServer_StreamableHTTPHandlerOffline(t *testing.T) {
	hub, _, _, _, _ := setupHubForTest(t)
	handler := hub.HTTPHandler()

	// 1. Health endpoint
	hReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17380/health", nil)
	hReq.Host = "127.0.0.1:17380"
	hRec := httptest.NewRecorder()
	handler.ServeHTTP(hRec, hReq)
	if hRec.Code != http.StatusOK || !strings.Contains(hRec.Body.String(), "vex-mcp") {
		t.Fatalf("unexpected /health response: code=%d body=%s", hRec.Code, hRec.Body.String())
	}

	// 2. MCP JSON-RPC initialize over Streamable HTTP (no loopback socket needed!)
	initBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcplib.LATEST_PROTOCOL_VERSION,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "http-test", "version": "1.0"},
		},
	}
	raw, _ := json.Marshal(initBody)
	mcpReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17380/mcp", bytes.NewReader(raw))
	mcpReq.Host = "127.0.0.1:17380"
	mcpReq.Header.Set("Content-Type", "application/json")
	mcpReq.Header.Set("Accept", "application/json, text/event-stream")
	mcpRec := httptest.NewRecorder()
	handler.ServeHTTP(mcpRec, mcpReq)

	if mcpRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from POST /mcp, got %d: %s", mcpRec.Code, mcpRec.Body.String())
	}
	if !strings.Contains(mcpRec.Body.String(), "Vex") {
		t.Fatalf("expected Vex in initialize response, got: %s", mcpRec.Body.String())
	}
}

func TestClientManager_ExternalMCPBridgeIndexesVirtualCards(t *testing.T) {
	ctx := context.Background()
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	searchSvc := search.NewService(store)
	mgr := NewClientManager(store)
	defer mgr.CloseAll(ctx)

	// Build a mock external MCP server representing GitHub
	extSrv := server.NewMCPServer("MockGitHub", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, true),
	)
	extSrv.AddTool(
		mcplib.NewTool("search_issues",
			mcplib.WithDescription("Search GitHub issues by query"),
			mcplib.WithString("q", mcplib.Required()),
		),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			q := req.GetString("q", "")
			return mcplib.NewToolResultText("Matched issue #42 for query: " + q), nil
		},
	)
	extSrv.AddTool(
		mcplib.NewTool("create_pr", mcplib.WithDescription("Create a GitHub pull request")),
		func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText("PR created"), nil
		},
	)
	extSrv.AddResource(
		mcplib.NewResource("github://repos/vex", "Vex Repo",
			mcplib.WithResourceDescription("Repository metadata for vex"),
			mcplib.WithMIMEType("application/json"),
		),
		func(ctx context.Context, req mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      req.Params.URI,
					MIMEType: "application/json",
					Text:     `{"repo":"vex","stars":100}`,
				},
			}, nil
		},
	)

	status, err := mgr.AttachInProcess(ctx, ExternalServerConfig{
		ID:      "github",
		Name:    "GitHub MCP",
		Enabled: true,
	}, extSrv)
	if err != nil {
		t.Fatalf("AttachInProcess: %v", err)
	}
	if !status.Connected || status.ToolCount != 2 || status.ResourceCount != 1 || status.CardCount != 3 {
		t.Fatalf("unexpected external server status: %+v", status)
	}

	// Verify searchable via @mcp:github modifier!
	res, err := searchSvc.Search(ctx, "@mcp:github issues", 10)
	if err != nil {
		t.Fatalf("Search @mcp:github: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].Card.Title != "search_issues" {
		t.Fatalf("expected search_issues card under @mcp:github, got %+v", res.Results)
	}
	if res.Results[0].Card.Template != models.TemplateCommand {
		t.Fatalf("expected command template for MCP tool card, got %q", res.Results[0].Card.Template)
	}

	// Verify searchable via /mcp modifier!
	res2, err := searchSvc.Search(ctx, "/mcp vex", 10)
	if err != nil {
		t.Fatalf("Search /mcp: %v", err)
	}
	if len(res2.Results) != 1 || res2.Results[0].Card.Subtitle != "github://repos/vex" {
		t.Fatalf("expected repo resource card under /mcp, got %+v", res2.Results)
	}

	// Direct tool invocation through ClientManager
	toolRes, err := mgr.CallTool(ctx, "github", "search_issues", map[string]any{"q": "wails"})
	if err != nil {
		t.Fatalf("CallTool via ClientManager: %v", err)
	}
	if text := extractToolText(t, toolRes); !strings.Contains(text, "issue #42 for query: wails") {
		t.Fatalf("unexpected remote tool output: %q", text)
	}

	// Direct resource read through ClientManager
	readRes, err := mgr.ReadResource(ctx, "github", "github://repos/vex")
	if err != nil {
		t.Fatalf("ReadResource via ClientManager: %v", err)
	}
	tc, ok := mcplib.AsTextResourceContents(readRes.Contents[0])
	if !ok || !strings.Contains(tc.Text, `"stars":100`) {
		t.Fatalf("unexpected remote resource content: %+v", readRes.Contents)
	}

	// Disconnect removes virtual cards from SQLite
	if err := mgr.Disconnect(ctx, "github"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	after, err := searchSvc.Search(ctx, "@mcp:github", 10)
	if err != nil || len(after.Results) != 0 {
		t.Fatalf("expected 0 cards after Disconnect, got %d (err=%v)", len(after.Results), err)
	}
}
