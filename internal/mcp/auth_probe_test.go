package mcp

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/server"

	"vex/internal/indexer"
)

func TestProbeCLIAuthReadOnlyAndSafe(t *testing.T) {
	ctx := context.Background()

	// 1. Both CLIs installed and signed in; verify token commands are NEVER invoked.
	statuses := ProbeCLIAuth(ctx, func(_ context.Context, name string, args ...string) (string, string, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "token") {
			t.Fatalf("security violation: ProbeCLIAuth must never request tokens (%s %s)", name, joined)
		}
		switch name {
		case "gh":
			return "github.com\n  ✓ Logged in to github.com account octocat (keyring)\n", "", nil
		case "az":
			return `{"name":"Enterprise-Prod","tenantId":"00000000-1111-2222-3333-444444444444","user":{"name":"alice@example.com"}}`, "", nil
		}
		return "", "", exec.ErrNotFound
	})

	if len(statuses) != 2 {
		t.Fatalf("expected 2 CLI statuses, got %d", len(statuses))
	}
	if !statuses[0].Authenticated || statuses[0].Account != "octocat" || statuses[0].HostOrTenant != "github.com" {
		t.Fatalf("unexpected gh status: %+v", statuses[0])
	}
	if !statuses[1].Authenticated || statuses[1].Account != "alice@example.com" || statuses[1].HostOrTenant != "00000000-1111-2222-3333-444444444444" {
		t.Fatalf("unexpected az status: %+v", statuses[1])
	}

	// 2. Missing gh + unauthenticated az
	statuses2 := ProbeCLIAuth(ctx, func(_ context.Context, name string, _ ...string) (string, string, error) {
		if name == "gh" {
			return "", "", exec.ErrNotFound
		}
		return "", "Please run 'az login' to setup account.", errors.New("exit status 1")
	})
	if statuses2[0].Installed || statuses2[0].Authenticated {
		t.Fatalf("expected gh not installed, got %+v", statuses2[0])
	}
	if !statuses2[1].Installed || statuses2[1].Authenticated {
		t.Fatalf("expected az installed but unauthenticated, got %+v", statuses2[1])
	}

	// 3. Recommended MCP presets default to opt-in (Enabled=false) and LocalOnly=true
	presets := RecommendedMCPPresets()
	if len(presets) != 2 {
		t.Fatalf("expected 2 recommended MCP presets, got %d", len(presets))
	}
	for _, p := range presets {
		if p.Enabled || !p.LocalOnly {
			t.Fatalf("preset %q must default to Enabled=false and LocalOnly=true: %+v", p.ID, p)
		}
	}
}

func TestExecutePatternMCPAndLocalOnlyPropagation(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	mockSrv := server.NewMCPServer("MockADO", "1.0.0", server.WithToolCapabilities(true))
	mockSrv.AddTool(
		mcplib.NewTool("wit_get_work_item",
			mcplib.WithDescription("Fetch an Azure DevOps work item by ID"),
			mcplib.WithString("id", mcplib.Required()),
		),
		func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			id := req.GetString("id", "")
			return mcplib.NewToolResultText(`{"id":"` + id + `","title":"EDI Retry Policy","state":"Active"}`), nil
		},
	)

	mgr := NewClientManager(store)
	ctx := context.Background()
	_, err = mgr.AttachInProcess(ctx, ExternalServerConfig{
		ID:        "azure-devops",
		Name:      "Azure DevOps MCP",
		Enabled:   true,
		LocalOnly: true,
	}, mockSrv)
	if err != nil {
		t.Fatalf("AttachInProcess: %v", err)
	}
	defer mgr.CloseAll(ctx)

	// Verify indexed tool card inherited LocalOnly=true
	cards, err := store.SearchHybrid(ctx, indexer.SearchOptions{Query: "wit_get_work_item", Limit: 5})
	if err != nil || len(cards) == 0 || !cards[0].Card.LocalOnly {
		t.Fatalf("expected indexed MCP tool card with LocalOnly=true, got %+v (err=%v)", cards, err)
	}

	// Verify ExecutePatternMCP returns tool text and localOnly=true
	text, localOnly, err := mgr.ExecutePatternMCP(ctx, "azure-devops", "wit_get_work_item", map[string]any{"id": "12345"})
	if err != nil {
		t.Fatalf("ExecutePatternMCP: %v", err)
	}
	if !localOnly || !strings.Contains(text, "EDI Retry Policy") {
		t.Fatalf("unexpected ExecutePatternMCP response: localOnly=%v text=%q", localOnly, text)
	}
}

func TestUpsertPatternRuleMCPToolAndLiveReload(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	patternsDir := t.TempDir()
	reloaded := false
	hub, err := NewHubServer(ServerDeps{
		Store:       store,
		PatternsDir: patternsDir,
		OnPatternsChanged: func() {
			reloaded = true
		},
	})
	if err != nil {
		t.Fatalf("NewHubServer: %v", err)
	}

	c, err := client.NewInProcessClient(hub.MCPServer())
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("client Start: %v", err)
	}
	defer c.Close()

	initReq := mcplib.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcplib.Implementation{Name: "test", Version: "1.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// 1. Valid rule saves to disk and triggers live reload
	callReq := mcplib.CallToolRequest{}
	callReq.Params.Name = "upsert_pattern_rule"
	callReq.Params.Arguments = map[string]any{
		"rule_json": `{"id":"snow-inc","name":"ServiceNow Incident","regex":"^(?P<inc>INC\\d{7})$","titleTemplate":"ServiceNow {{inc}}","localOnly":true}`,
	}
	res, err := c.CallTool(ctx, callReq)
	if err != nil || res.IsError {
		t.Fatalf("upsert_pattern_rule failed: res=%+v err=%v", res, err)
	}
	if !reloaded {
		t.Fatalf("expected OnPatternsChanged callback to fire after upsert_pattern_rule")
	}

	loaded, err := indexer.LoadPatternRulesDir(patternsDir)
	if err != nil || len(loaded) != 1 || loaded[0].ID != "snow-inc" {
		t.Fatalf("expected saved pattern rule in %s, got %+v (err=%v)", patternsDir, loaded, err)
	}

	// 2. Invalid regex returns a clean MCP tool error
	badReq := mcplib.CallToolRequest{}
	badReq.Params.Name = "upsert_pattern_rule"
	badReq.Params.Arguments = map[string]any{
		"rule_json": `{"id":"bad-re","name":"Broken","regex":"[unclosed","titleTemplate":"Broken"}`,
	}
	badRes, err := c.CallTool(ctx, badReq)
	if err != nil || !badRes.IsError {
		t.Fatalf("expected MCP tool error for invalid regex, got res=%+v err=%v", badRes, err)
	}
}

func TestModuleManagementMCPTools(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	modulesDir := t.TempDir()
	syncCount := 0
	hub, err := NewHubServer(ServerDeps{
		Store:      store,
		ModulesDir: modulesDir,
		OnModulesChanged: func() {
			syncCount++
		},
	})
	if err != nil {
		t.Fatalf("NewHubServer: %v", err)
	}

	c, err := client.NewInProcessClient(hub.MCPServer())
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	initReq := mcplib.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcplib.Implementation{Name: "test", Version: "1.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	call := func(name string, args map[string]any) *mcplib.CallToolResult {
		req := mcplib.CallToolRequest{}
		req.Params.Name = name
		req.Params.Arguments = args
		res, err := c.CallTool(ctx, req)
		if err != nil || res.IsError {
			t.Fatalf("CallTool %s failed: res=%+v err=%v", name, res, err)
		}
		return res
	}

	// 1. Install starter module via MCP
	call("install_starter_module", map[string]any{"module_id": "team-templates"})
	// 2. Get full module JSON via MCP
	getRes := call("get_module", map[string]any{"module_id": "team-templates"})
	if len(getRes.Content) == 0 {
		t.Fatalf("expected module JSON content from get_module")
	}
	// 3. Upsert custom module via MCP
	call("upsert_module", map[string]any{
		"module_json": `{"id":"my-team-edi","name":"My Team EDI","category":"Templates","version":"1.0.0","cards":[{"id":"my-edi-1","title":"My Team EDI Servers","collection":"templates","template":"server-matrix","icon":"🖥️","chips":[{"label":"PROD","tone":"red","dot":"red"}],"data":{"servers":[{"env":"PROD","tone":"red","host":"team-prd-edi01.internal"}]}}]}`,
	})
	// 4. Delete installed module via MCP
	call("delete_module", map[string]any{"module_id": "team-templates"})

	if syncCount != 3 {
		t.Fatalf("expected 3 OnModulesChanged callbacks (install, upsert, delete), got %d", syncCount)
	}
}
