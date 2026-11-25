package mcp

import (
	"context"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"vex/internal/indexer"
	"vex/internal/models"
	"vex/internal/search"
)

func TestQueryExternalMCP_GitHubAndADOGenericLayouts(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	mgr := NewClientManager(store)
	defer mgr.CloseAll(ctx)

	svc := search.NewService(store)
	svc.SetLiveMCPQueryProvider(mgr)

	// 1. Disconnected /gh returns clean generic facts setup hint
	respDisconnected, err := svc.Search(ctx, "/gh search-widget", 10)
	if err != nil {
		t.Fatalf("Search /gh disconnected: %v", err)
	}
	if len(respDisconnected.Results) == 0 || respDisconnected.Results[0].Card.Layout != models.LayoutFacts {
		t.Fatalf("expected disconnected hint facts card, got %+v", respDisconnected.Results)
	}

	// 2. Attach in-process GitHub MCP server
	ghSrv := server.NewMCPServer("MockGitHub", "1.0.0", server.WithToolCapabilities(true))
	ghSrv.AddTool(
		mcplib.NewTool("search_repositories",
			mcplib.WithDescription("Search GitHub repositories"),
			mcplib.WithString("query"),
		),
		func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText(`{"items":[
				{"full_name":"octo/vex","description":"Desktop Rolodex","language":"Go","html_url":"https://github.com/octo/vex","clone_url":"https://github.com/octo/vex.git"},
				{"full_name":"octo/vex-docs","description":"Docs site","language":"TypeScript","html_url":"https://github.com/octo/vex-docs","clone_url":"https://github.com/octo/vex-docs.git"}
			]}`), nil
		},
	)
	ghSrv.AddTool(
		mcplib.NewTool("get_issue",
			mcplib.WithDescription("Get GitHub issue"),
			mcplib.WithString("owner"),
			mcplib.WithString("repo"),
		),
		func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText(`{"number":42,"title":"Add liveHttp GET support","state":"open","user":{"login":"octocat"},"html_url":"https://github.com/octo/vex/issues/42"}`), nil
		},
	)
	ghSrv.AddTool(
		mcplib.NewTool("search_issues",
			mcplib.WithDescription("Search GitHub issues and PRs"),
			mcplib.WithString("q"),
		),
		func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText(`{"items":[
				{"number":10,"title":"Improve ranking","state":"open","html_url":"https://github.com/octo/vex/issues/10"},
				{"number":11,"title":"Fix tray icon","state":"closed","html_url":"https://github.com/octo/vex/issues/11"}
			]}`), nil
		},
	)

	if _, err := mgr.AttachInProcess(ctx, ExternalServerConfig{
		ID:        "github",
		Name:      "GitHub MCP",
		Enabled:   true,
		LocalOnly: true,
	}, ghSrv); err != nil {
		t.Fatalf("AttachInProcess github: %v", err)
	}

	// 2a. /gh <repo query> -> action-list layout
	repoResp, err := svc.Search(ctx, "/gh vex", 10)
	if err != nil {
		t.Fatalf("Search /gh vex: %v", err)
	}
	if len(repoResp.Results) == 0 || repoResp.Results[0].Card.Layout != models.LayoutActionList {
		t.Fatalf("expected action-list card for /gh repo search, got %+v", repoResp.Results)
	}

	// 2b. /gh owner/repo#42 -> facts layout with nested link child
	issueResp, err := svc.Search(ctx, "/gh octo/vex#42", 10)
	if err != nil {
		t.Fatalf("Search /gh octo/vex#42: %v", err)
	}
	if len(issueResp.Results) == 0 || issueResp.Results[0].Card.Layout != models.LayoutFacts {
		t.Fatalf("expected facts card for /gh octo/vex#42, got %+v", issueResp.Results)
	}
	if len(issueResp.Results[0].Card.Children) != 1 || issueResp.Results[0].Card.Children[0].Layout != models.LayoutLink {
		t.Fatalf("expected nested link child card, got %+v", issueResp.Results[0].Card.Children)
	}

	// 2c. /gh issue:ranking -> table + link cards
	issuesResp, err := svc.Search(ctx, "/gh issue:ranking", 10)
	if err != nil {
		t.Fatalf("Search /gh issue:ranking: %v", err)
	}
	if len(issuesResp.Results) < 2 || issuesResp.Results[0].Card.Layout != models.LayoutTable || issuesResp.Results[1].Card.Layout != models.LayoutLink {
		t.Fatalf("expected table + link cards for /gh issue search, got %+v", issuesResp.Results)
	}

	// 3. Attach in-process Azure DevOps MCP server
	adoSrv := server.NewMCPServer("MockADO", "1.0.0", server.WithToolCapabilities(true))
	adoSrv.AddTool(
		mcplib.NewTool("wit_get_work_item",
			mcplib.WithDescription("Get ADO work item"),
			mcplib.WithString("id"),
		),
		func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText(`{
				"id": 12345,
				"fields": {
					"System.Title": "EDI Timeout Recovery",
					"System.WorkItemType": "Bug",
					"System.State": "Active",
					"System.AssignedTo": {"displayName": "Alice Smith"}
				},
				"_links": {"html": {"href": "https://dev.azure.com/contoso/ops/_workitems/edit/12345"}}
			}`), nil
		},
	)
	adoSrv.AddTool(
		mcplib.NewTool("wit_search_work_items",
			mcplib.WithDescription("Search ADO work items"),
			mcplib.WithString("searchText"),
		),
		func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return mcplib.NewToolResultText(`{"value":[
				{"id":101,"fields":{"System.Title":"Fix AWB parser","System.WorkItemType":"Task","System.State":"Active"}},
				{"id":102,"fields":{"System.Title":"Update SCAC list","System.WorkItemType":"User Story","System.State":"New"}}
			]}`), nil
		},
	)

	if _, err := mgr.AttachInProcess(ctx, ExternalServerConfig{
		ID:        "azure-devops",
		Name:      "Azure DevOps MCP",
		Enabled:   true,
		LocalOnly: true,
	}, adoSrv); err != nil {
		t.Fatalf("AttachInProcess ado: %v", err)
	}

	// 3a. /ado AB#12345 -> facts card (localOnly=true) + nested link child
	adoSingle, err := svc.Search(ctx, "/ado AB#12345", 10)
	if err != nil {
		t.Fatalf("Search /ado AB#12345: %v", err)
	}
	if len(adoSingle.Results) == 0 || adoSingle.Results[0].Card.Layout != models.LayoutFacts || !adoSingle.Results[0].Card.LocalOnly {
		t.Fatalf("expected localOnly facts card for /ado AB#12345, got %+v", adoSingle.Results)
	}
	if len(adoSingle.Results[0].Card.Children) != 1 || adoSingle.Results[0].Card.Children[0].Layout != models.LayoutLink {
		t.Fatalf("expected nested link child card for ADO work item, got %+v", adoSingle.Results[0].Card.Children)
	}

	// 3b. /ado edi -> generic table card
	adoSearch, err := svc.Search(ctx, "/ado edi", 10)
	if err != nil {
		t.Fatalf("Search /ado edi: %v", err)
	}
	if len(adoSearch.Results) == 0 || adoSearch.Results[0].Card.Layout != models.LayoutTable || !adoSearch.Results[0].Card.LocalOnly {
		t.Fatalf("expected localOnly table card for /ado search, got %+v", adoSearch.Results)
	}
}
