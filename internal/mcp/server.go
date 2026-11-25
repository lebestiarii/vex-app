package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vex/internal/indexer"
	"vex/internal/logging"
	"vex/internal/models"
	"vex/internal/plugins"
	"vex/internal/search"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ServerDeps wires Vex's core backend services into the Domain MCP Server.
type ServerDeps struct {
	Store             *indexer.Store
	Search            *search.Service
	Plugins           *plugins.Manager
	Runner            *plugins.Runner
	Metrics           *logging.Metrics
	DataDir           string
	ModulesDir        string
	PatternsDir       string
	SkillsDirs        []string // Optional directories containing <skill>/SKILL.md files for MCP agent skill sharing
	OnModulesChanged  func()
	OnPatternsChanged func()
	Version           string
}

// HubServer exposes Vex's search index, schemas, ETL pipeline, and actions
// over the Model Context Protocol (both stdio and Streamable HTTP).
type HubServer struct {
	deps      ServerDeps
	mcpServer *server.MCPServer

	mu         sync.Mutex
	httpServer *http.Server
	streamSrv  *server.StreamableHTTPServer
	httpAddr   string
}

// NewHubServer creates and registers all Vex MCP resources, tools, and prompts.
func NewHubServer(deps ServerDeps) (*HubServer, error) {
	if deps.Store == nil {
		return nil, errors.New("mcp server requires non-nil Store")
	}
	if deps.Search == nil {
		deps.Search = search.NewService(deps.Store)
	}
	if deps.Plugins == nil {
		deps.Plugins = plugins.NewManager()
	}
	if deps.Runner == nil {
		deps.Runner = plugins.NewRunner()
	}
	if strings.TrimSpace(deps.Version) == "" {
		deps.Version = "0.1.0"
	}

	s := &HubServer{deps: deps}

	mcpSrv := server.NewMCPServer(
		"Vex",
		deps.Version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, true),
		server.WithRecovery(),
		server.WithInstructions("Vex local knowledge Rolodex, configurable pattern decoder, and CSV/JSON SearchCard ETL hub. Use schema://search-card or get_card_schema first, validate manifests with validate_module, then publish with upsert_module."),
	)
	s.mcpServer = mcpSrv

	s.registerResources()
	s.registerTools()

	return s, nil
}

// MCPServer returns the underlying mcp-go server (for Wails integration or in-process testing).
func (s *HubServer) MCPServer() *server.MCPServer {
	return s.mcpServer
}

// ServeStdio blocks serving MCP JSON-RPC requests over stdin/stdout (used by --mcp-stdio).
func (s *HubServer) ServeStdio() error {
	return server.ServeStdio(s.mcpServer, server.WithWorkerPoolSize(1))
}

// HTTPHandler returns an http.Handler mounting the MCP Streamable HTTP endpoint at /mcp, /health, and /metrics,
// protected by a strict loopback Host & Origin guard to prevent browser CSRF and DNS rebinding.
func (s *HubServer) HTTPHandler() http.Handler {
	streamSrv := server.NewStreamableHTTPServer(
		s.mcpServer,
		server.WithEndpointPath("/mcp"),
	)
	return s.buildHTTPMux(streamSrv)
}

func (s *HubServer) buildHTTPMux(streamSrv *server.StreamableHTTPServer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Metrics != nil {
			s.deps.Metrics.MCPRequestsTotal.Add(1)
		}
		streamSrv.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"vex-mcp"}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if s.deps.Metrics != nil {
			s.deps.Metrics.WritePrometheus(w)
		}
	})
	return loopbackGuardMiddleware(mux)
}

func loopbackGuardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "" && !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden: non-loopback Host header", http.StatusForbidden)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLoopbackHost(u.Host) {
				http.Error(w, "forbidden: cross-origin requests are not allowed on local MCP endpoint", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// StartHTTP starts the Streamable HTTP MCP server on addr (default "127.0.0.1:17380").
func (s *HubServer) StartHTTP(addr string) error {
	if strings.TrimSpace(addr) == "" {
		addr = "127.0.0.1:17380"
	}

	s.mu.Lock()
	if s.httpServer != nil {
		s.mu.Unlock()
		return nil
	}
	streamSrv := server.NewStreamableHTTPServer(
		s.mcpServer,
		server.WithEndpointPath("/mcp"),
	)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.buildHTTPMux(streamSrv),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.streamSrv = streamSrv
	s.httpServer = srv
	s.httpAddr = addr
	s.mu.Unlock()

	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// ShutdownHTTP gracefully stops the HTTP MCP server if running.
func (s *HubServer) ShutdownHTTP(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpServer
	streamSrv := s.streamSrv
	s.httpServer = nil
	s.streamSrv = nil
	s.mu.Unlock()

	if streamSrv != nil {
		streamSrv.CloseSessions(ctx)
	}
	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

// --- Resources ---

func (s *HubServer) registerResources() {
	// 1. search://collections
	s.mcpServer.AddResource(
		mcplib.NewResource(
			"search://collections",
			"Indexed SearchCard Collections",
			mcplib.WithResourceDescription("Lists all indexed JSON collections, card counts, and available tags."),
			mcplib.WithMIMEType("application/json"),
		),
		func(ctx context.Context, request mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			stats, err := s.deps.Store.ListCollections(ctx)
			if err != nil {
				return nil, err
			}
			raw, err := json.MarshalIndent(stats, "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "application/json",
					Text:     string(raw),
				},
			}, nil
		},
	)

	// 2. schema://search-card
	s.mcpServer.AddResource(
		mcplib.NewResource(
			"schema://search-card",
			"SearchCard & ETLMappingRule JSON Schema",
			mcplib.WithResourceDescription("Canonical JSON Schema for Vex SearchCard items and ETLMappingRule CSV/JSON import definitions."),
			mcplib.WithMIMEType("application/schema+json"),
		),
		func(ctx context.Context, request mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "application/schema+json",
					Text:     models.CardJSONSchema(),
				},
			}, nil
		},
	)

	// 3. card://{collection}/{id}
	s.mcpServer.AddResourceTemplate(
		mcplib.NewResourceTemplate(
			"card://{collection}/{id}",
			"SearchCard Document",
			mcplib.WithTemplateDescription("Retrieves a full SearchCard JSON document and flattened RAG body by collection and ID."),
			mcplib.WithTemplateMIMEType("application/json"),
		),
		func(ctx context.Context, request mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			uri := request.Params.URI
			trimmed := strings.TrimPrefix(uri, "card://")
			parts := strings.SplitN(trimmed, "/", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
				return nil, fmt.Errorf("invalid card URI %q (expected card://{collection}/{id})", uri)
			}
			reqCol := models.Slugify(parts[0])
			cardID := parts[1]

			card, err := s.deps.Store.GetCard(ctx, cardID)
			if err != nil {
				return nil, err
			}
			if card == nil || (reqCol != "" && card.Collection != reqCol) {
				return nil, fmt.Errorf("card %q not found in collection %q", cardID, reqCol)
			}

			payload := map[string]any{
				"card":           card,
				"searchableBody": card.SearchableBody(),
			}
			raw, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      uri,
					MIMEType: "application/json",
					Text:     string(raw),
				},
			}, nil
		},
	)

	// 4. search://modules
	s.mcpServer.AddResource(
		mcplib.NewResource(
			"search://modules",
			"Installed Optional Modules",
			mcplib.WithResourceDescription("Lists all installed optional ModuleManifest files in ~/.vex/modules/ and available starter modules."),
			mcplib.WithMIMEType("application/json"),
		),
		func(_ context.Context, request mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			mods, paths, err := indexer.LoadAllModules(s.deps.ModulesDir)
			if err != nil {
				return nil, err
			}
			payload := map[string]any{
				"installed": mods,
				"paths":     paths,
				"starters":  indexer.ListStarterModules(s.deps.ModulesDir),
			}
			raw, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "application/json",
					Text:     string(raw),
				},
			}, nil
		},
	)

	// 5. search://patterns
	s.mcpServer.AddResource(
		mcplib.NewResource(
			"search://patterns",
			"Active Pattern Matching Rules",
			mcplib.WithResourceDescription("Lists all currently loaded PatternRules from enabled modules and ~/.vex/patterns/."),
			mcplib.WithMIMEType("application/json"),
		),
		func(_ context.Context, request mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			raw, err := json.MarshalIndent(s.deps.Search.PatternRules(), "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "application/json",
					Text:     string(raw),
				},
			}, nil
		},
	)

	// 6. search://skills (Agent Skill Sharing)
	s.mcpServer.AddResource(
		mcplib.NewResource(
			"search://skills",
			"Shared Agent Skills Index",
			mcplib.WithResourceDescription("Lists available Vex agent skills (UI design system, generic template authoring, architecture, doc drift) shareable across MCP clients."),
			mcplib.WithMIMEType("application/json"),
		),
		func(_ context.Context, request mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			raw, err := json.MarshalIndent(s.discoverSkills(), "", "  ")
			if err != nil {
				return nil, err
			}
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "application/json",
					Text:     string(raw),
				},
			}, nil
		},
	)

	// 7. skill://{name}
	s.mcpServer.AddResourceTemplate(
		mcplib.NewResourceTemplate(
			"skill://{name}",
			"Agent Skill Markdown Document",
			mcplib.WithTemplateDescription("Reads a shared SKILL.md instruction document by skill slug (e.g. skill://vex-rolodex-ui, skill://vex-template-authoring)."),
			mcplib.WithTemplateMIMEType("text/markdown"),
		),
		func(_ context.Context, request mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
			name := models.Slugify(strings.TrimPrefix(request.Params.URI, "skill://"))
			content, err := s.readSkillMarkdown(name)
			if err != nil {
				return nil, err
			}
			return []mcplib.ResourceContents{
				mcplib.TextResourceContents{
					URI:      request.Params.URI,
					MIMEType: "text/markdown",
					Text:     content,
				},
			}, nil
		},
	)
}

// --- Tools ---

func (s *HubServer) registerTools() {
	// 1. search_knowledge
	s.mcpServer.AddTool(
		mcplib.NewTool("search_knowledge",
			mcplib.WithDescription("Execute 4-tier exact/contextual/FTS5/guarded-fuzzy search across all Vex JSON card collections and dynamic plugins."),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithString("query", mcplib.Required(), mcplib.Description("Search query (supports Raycast modifiers like /cmd, @collection, #tag, layout:facts).")),
			mcplib.WithString("collection", mcplib.Description("Optional collection name filter (e.g. devops, air-carriers, containers).")),
			mcplib.WithArray("tags", mcplib.Description("Optional tag filters."), mcplib.WithStringItems()),
			mcplib.WithString("layout", mcplib.Description("Optional layout filter (group, link, action-list, facts, segments, steps, convert, table, snippet).")),
			mcplib.WithNumber("limit", mcplib.Description("Maximum results to return (default 10).")),
		),
		s.handleSearchKnowledge,
	)

	// 2. get_card_schema
	s.mcpServer.AddTool(
		mcplib.NewTool("get_card_schema",
			mcplib.WithDescription("Return the canonical SearchCard and ETLMappingRule JSON Schema plus example payloads so an AI can author valid cards or CSV/JSON import rules."),
			mcplib.WithReadOnlyHintAnnotation(true),
		),
		s.handleGetCardSchema,
	)

	// 3. preview_raw_data_sample
	s.mcpServer.AddTool(
		mcplib.NewTool("preview_raw_data_sample",
			mcplib.WithDescription("Inspect a local CSV, TSV, JSON, or JSONL file (or raw string) and return detected format, rootPath, fields/headers, total row count, and up to 5 sample rows."),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithString("file_path", mcplib.Description("Absolute path to a local .csv, .tsv, .json, or .jsonl file.")),
			mcplib.WithString("raw_content", mcplib.Description("Raw CSV/TSV/JSON/JSONL content string (used when file_path is omitted).")),
			mcplib.WithString("filename_hint", mcplib.Description("Optional filename hint such as bookmarks.csv or export.json.")),
		),
		s.handlePreviewRawDataSample,
	)

	// 4. transform_and_import_data
	s.mcpServer.AddTool(
		mcplib.NewTool("transform_and_import_data",
			mcplib.WithDescription("Apply an ETLMappingRule to transform a CSV/TSV/JSON/JSONL file or string into validated SearchCards, persist them to disk, and immediately index them."),
			mcplib.WithIdempotentHintAnnotation(true),
			mcplib.WithString("file_path", mcplib.Description("Absolute path to local raw data file.")),
			mcplib.WithString("raw_content", mcplib.Description("Raw CSV/TSV/JSON/JSONL payload (used when file_path is omitted).")),
			mcplib.WithObject("rule", mcplib.Required(), mcplib.Description("ETLMappingRule JSON object mapping source columns/keys to SearchCard fields.")),
			mcplib.WithBoolean("replace_collection", mcplib.Description("If true, overwrite the target collection instead of merging by card ID (default false).")),
		),
		s.handleTransformAndImportData,
	)

	// 5. upsert_card
	s.mcpServer.AddTool(
		mcplib.NewTool("upsert_card",
			mcplib.WithDescription("Create or update a single SearchCard in its target JSON collection and immediately index it."),
			mcplib.WithIdempotentHintAnnotation(true),
			mcplib.WithObject("card", mcplib.Required(), mcplib.Description("SearchCard object conforming to schema://search-card.")),
		),
		s.handleUpsertCard,
	)

	// 6. batch_upsert_cards
	s.mcpServer.AddTool(
		mcplib.NewTool("batch_upsert_cards",
			mcplib.WithDescription("Create or update multiple SearchCards in a target JSON collection atomically and immediately index them."),
			mcplib.WithIdempotentHintAnnotation(true),
			mcplib.WithString("collection", mcplib.Required(), mcplib.Description("Target collection name (e.g. shortcuts, devops, contacts).")),
			mcplib.WithArray("cards", mcplib.Required(), mcplib.Description("Array of SearchCard objects."), mcplib.Items(map[string]any{"type": "object"})),
		),
		s.handleBatchUpsertCards,
	)

	// 7. list_plugins
	s.mcpServer.AddTool(
		mcplib.NewTool("list_plugins",
			mcplib.WithDescription("List all registered declarative Vex plugins, their /modifiers, static cards, and dynamic source capabilities."),
			mcplib.WithReadOnlyHintAnnotation(true),
		),
		s.handleListPlugins,
	)

	// 8. execute_card_action
	s.mcpServer.AddTool(
		mcplib.NewTool("execute_card_action",
			mcplib.WithDescription("Execute an action attached to an indexed SearchCard with optional template arguments."),
			mcplib.WithString("card_id", mcplib.Required(), mcplib.Description("ID of the SearchCard to execute.")),
			mcplib.WithString("action_id", mcplib.Description("Optional specific action ID (defaults to the card's primary action).")),
			mcplib.WithString("query", mcplib.Description("Optional query string providing {{arg1}}, {{query}}, or key:value parameters.")),
			mcplib.WithObject("args", mcplib.Description("Optional key-value map overriding template placeholders.")),
		),
		s.handleExecuteCardAction,
	)

	// 9. get_related_cards
	s.mcpServer.AddTool(
		mcplib.NewTool("get_related_cards",
			mcplib.WithDescription("Find up to limit related SearchCards across enabled modules that share synergy with card_id via entityKey, relatedKeys, or domain tags."),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithString("card_id", mcplib.Required(), mcplib.Description("ID of the anchor SearchCard.")),
			mcplib.WithNumber("limit", mcplib.Description("Maximum number of related cards to return (default 5).")),
		),
		s.handleGetRelatedCards,
	)

	// 10. upsert_pattern_rule
	s.mcpServer.AddTool(
		mcplib.NewTool("upsert_pattern_rule",
			mcplib.WithDescription("Validate and save a declarative PatternRule JSON file to ~/.vex/patterns/<id>.json and immediately reload Vex's live PatternEngine. Call get_card_schema first to inspect the PatternRule schema."),
			mcplib.WithObject("rule", mcplib.Description("PatternRule object matching schema://search-card#/definitions/PatternRule.")),
			mcplib.WithString("rule_json", mcplib.Description("Optional JSON string representation of a PatternRule (alternative to rule object).")),
		),
		s.handleUpsertPatternRule,
	)

	// 11. list_pattern_rules
	s.mcpServer.AddTool(
		mcplib.NewTool("list_pattern_rules",
			mcplib.WithDescription("List all active PatternRules loaded from enabled modules and ~/.vex/patterns/."),
			mcplib.WithReadOnlyHintAnnotation(true),
		),
		s.handleListPatternRules,
	)

	// 12. delete_pattern_rule
	s.mcpServer.AddTool(
		mcplib.NewTool("delete_pattern_rule",
			mcplib.WithDescription("Delete a standalone pattern rule file from ~/.vex/patterns/<id>.json and reload the live PatternEngine."),
			mcplib.WithString("rule_id", mcplib.Required(), mcplib.Description("ID of the pattern rule to delete.")),
		),
		s.handleDeletePatternRule,
	)

	// 13. list_modules
	s.mcpServer.AddTool(
		mcplib.NewTool("list_modules",
			mcplib.WithDescription("List installed user modules in ~/.vex/modules/ and optional packaged starter modules available for installation."),
			mcplib.WithReadOnlyHintAnnotation(true),
		),
		s.handleListModules,
	)

	// 14. get_module
	s.mcpServer.AddTool(
		mcplib.NewTool("get_module",
			mcplib.WithDescription("Return the full ModuleManifest JSON (including cards, nested children, overlays, and patterns) for an installed module ID so you can inspect and update it."),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithString("module_id", mcplib.Required(), mcplib.Description("Installed module ID (e.g. gateways-starter, air-carriers, containers, reference-patterns).")),
		),
		s.handleGetModule,
	)

	// 15. validate_module
	s.mcpServer.AddTool(
		mcplib.NewTool("validate_module",
			mcplib.WithDescription("Strictly validate a ModuleManifest JSON object or string against Vex's zero-alias 9-layout schema, chip/echo invariants, and depth <= 2 nesting rules without writing to disk."),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithObject("module", mcplib.Description("ModuleManifest JSON object to validate.")),
			mcplib.WithString("module_json", mcplib.Description("Optional raw JSON string representation of a ModuleManifest.")),
		),
		s.handleValidateModule,
	)

	// 16. upsert_module
	s.mcpServer.AddTool(
		mcplib.NewTool("upsert_module",
			mcplib.WithDescription("Strictly validate and save a ModuleManifest JSON file in ~/.vex/modules/<id>.json (flat-on-disk cards, overlays, and pattern rules) and immediately re-index Vex."),
			mcplib.WithIdempotentHintAnnotation(true),
			mcplib.WithObject("module", mcplib.Description("ModuleManifest JSON object conforming to schema://search-card#/$defs/ModuleManifest.")),
			mcplib.WithString("module_json", mcplib.Description("Optional raw JSON string representation of a ModuleManifest.")),
		),
		s.handleUpsertModule,
	)

	// 17. delete_module
	s.mcpServer.AddTool(
		mcplib.NewTool("delete_module",
			mcplib.WithDescription("Permanently remove an installed module JSON file from ~/.vex/modules/<id>.json and re-sync the index."),
			mcplib.WithString("module_id", mcplib.Required(), mcplib.Description("ID of the module to delete.")),
		),
		s.handleDeleteModule,
	)

	// 18. install_starter_module
	s.mcpServer.AddTool(
		mcplib.NewTool("install_starter_module",
			mcplib.WithDescription("Install an optional packaged starter module (air-carriers, ocean-carriers, containers, logistics-dictionary, reference-patterns) into ~/.vex/modules/."),
			mcplib.WithString("module_id", mcplib.Required(), mcplib.Description("Starter module ID to install.")),
		),
		s.handleInstallStarterModule,
	)

	// 18. list_skills
	s.mcpServer.AddTool(
		mcplib.NewTool("list_skills",
			mcplib.WithDescription("List shared Vex agent skills (UI layout rules, generic template authoring, architecture) available for MCP clients."),
			mcplib.WithReadOnlyHintAnnotation(true),
		),
		s.handleListSkills,
	)

	// 19. get_skill
	s.mcpServer.AddTool(
		mcplib.NewTool("get_skill",
			mcplib.WithDescription("Read the full SKILL.md markdown guide for a shared Vex agent skill (e.g. vex-rolodex-ui, vex-template-authoring, vex-architecture)."),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("Skill slug name (from list_skills).")),
		),
		s.handleGetSkill,
	)
}

func (s *HubServer) handleSearchKnowledge(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	query := strings.TrimSpace(req.GetString("query", ""))
	collection := strings.TrimSpace(req.GetString("collection", ""))
	tags := req.GetStringSlice("tags", nil)
	layout := strings.TrimSpace(req.GetString("layout", ""))
	limit := req.GetInt("limit", 10)
	if limit <= 0 {
		limit = 10
	}

	var queryParts []string
	if collection != "" && !strings.Contains(query, "@") {
		queryParts = append(queryParts, "@"+strings.TrimPrefix(collection, "@"))
	}
	for _, t := range tags {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t != "" {
			queryParts = append(queryParts, "#"+t)
		}
	}
	if layout != "" && !strings.Contains(query, "layout:") && !strings.Contains(query, "type:") {
		queryParts = append(queryParts, "layout:"+layout)
	}
	if query != "" {
		queryParts = append(queryParts, query)
	}
	fullQuery := strings.Join(queryParts, " ")

	resp, err := s.deps.Search.Search(ctx, fullQuery, limit)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("search failed", err), nil
	}

	type mcpSearchItem struct {
		URI         string            `json:"uri"`
		Score       float64           `json:"score"`
		MatchSource string            `json:"matchSource"`
		Card        models.SearchCard `json:"card"`
		CardNode    models.CardNode   `json:"cardNode"`
	}
	items := make([]mcpSearchItem, 0, len(resp.Results))
	for _, r := range resp.Results {
		items = append(items, mcpSearchItem{
			URI:         fmt.Sprintf("card://%s/%s", r.Card.Collection, r.Card.ID),
			Score:       r.Score,
			MatchSource: r.MatchSource,
			Card:        r.Card,
			CardNode:    r.Card.ToCardNode(0),
		})
	}
	return jsonToolResult(map[string]any{
		"parsed":  resp.Parsed,
		"total":   len(items),
		"results": items,
	})
}

func (s *HubServer) handleGetCardSchema(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	var schemaObj any
	_ = json.Unmarshal([]byte(models.CardJSONSchema()), &schemaObj)

	exampleRule := models.ETLMappingRule{
		Name:             "CSV Bookmarks Import",
		SourceFormat:     "csv",
		TargetCollection: "bookmarks",
		DefaultLayout:    models.LayoutLink,
		TitleField:       "{{Name}}",
		SubtitleField:    "{{Category}}",
		TagsField:        "Tags",
		ActionType:       "open_url",
		ActionLabel:      "Open Link",
		ActionTarget:     "{{URL}}",
	}
	return jsonToolResult(map[string]any{
		"schema":         schemaObj,
		"exampleETLRule": exampleRule,
		"layouts":        models.LayoutNames(),
		"actionTypes":    []string{"open_url", "open_pwa", "launch_app", "copy", "http"},
	})
}

func (s *HubServer) handlePreviewRawDataSample(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	rawBytes, hint, err := readRawInput(req)
	if err != nil {
		return mcplib.NewToolResultError(err.Error()), nil
	}
	sample, err := indexer.SampleRawData(rawBytes, hint)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to sample raw data", err), nil
	}
	return jsonToolResult(sample)
}

func (s *HubServer) handleTransformAndImportData(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	rawBytes, _, err := readRawInput(req)
	if err != nil {
		return mcplib.NewToolResultError(err.Error()), nil
	}
	args := req.GetArguments()
	rawRule, ok := args["rule"]
	if !ok || rawRule == nil {
		return mcplib.NewToolResultError("rule parameter is required"), nil
	}
	var rule models.ETLMappingRule
	if err := decodeArg(rawRule, &rule); err != nil {
		return mcplib.NewToolResultErrorFromErr("invalid ETLMappingRule", err), nil
	}

	cards, err := indexer.TransformWithRule(rawBytes, rule)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("ETL transformation failed", err), nil
	}

	col := cards[0].Collection
	replace := req.GetBool("replace_collection", false)

	var savedPath string
	if s.deps.DataDir != "" {
		if replace {
			savedPath, err = indexer.SaveCollectionFile(s.deps.DataDir, col, cards)
		} else {
			savedPath, _, err = indexer.BatchUpsertCardsToDisk(s.deps.DataDir, col, cards)
		}
		if err != nil {
			return mcplib.NewToolResultErrorFromErr("failed to write JSON collection to disk", err), nil
		}
	}

	// Sync SQLite store immediately so subsequent searches see the imported cards without waiting for fsnotify.
	if savedPath != "" {
		loadedCol, loadedCards, loadErr := indexer.LoadCollectionFile(savedPath)
		if loadErr != nil {
			return mcplib.NewToolResultErrorFromErr("failed to reload saved collection", loadErr), nil
		}
		if err := s.deps.Store.ReplaceCollection(ctx, loadedCol, loadedCards, savedPath); err != nil {
			return mcplib.NewToolResultErrorFromErr("failed to index imported cards", err), nil
		}
	} else {
		if replace {
			if err := s.deps.Store.ReplaceCollection(ctx, col, cards, ""); err != nil {
				return mcplib.NewToolResultErrorFromErr("failed to index imported cards", err), nil
			}
		} else {
			for _, c := range cards {
				if err := s.deps.Store.UpsertCard(ctx, c, ""); err != nil {
					return mcplib.NewToolResultErrorFromErr("failed to upsert imported card", err), nil
				}
			}
		}
	}

	sampleIDs := make([]string, 0, min(5, len(cards)))
	for i := 0; i < len(cards) && i < 5; i++ {
		sampleIDs = append(sampleIDs, cards[i].ID)
	}
	return jsonToolResult(map[string]any{
		"collection":    col,
		"importedCount": len(cards),
		"filePath":      savedPath,
		"sampleCardIds": sampleIDs,
	})
}

func (s *HubServer) handleUpsertCard(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	rawCard, ok := args["card"]
	if !ok || rawCard == nil {
		return mcplib.NewToolResultError("card parameter is required"), nil
	}
	var card models.SearchCard
	if err := decodeArg(rawCard, &card); err != nil {
		return mcplib.NewToolResultErrorFromErr("invalid SearchCard payload", err), nil
	}
	if err := models.ValidateAndNormalizeCard(&card, card.Collection); err != nil {
		return mcplib.NewToolResultErrorFromErr("card validation failed", err), nil
	}

	var savedPath string
	if s.deps.DataDir != "" {
		var err error
		savedPath, err = indexer.UpsertCardToDisk(s.deps.DataDir, card)
		if err != nil {
			return mcplib.NewToolResultErrorFromErr("failed to persist card to disk", err), nil
		}
	}
	if err := s.deps.Store.UpsertCard(ctx, card, savedPath); err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to index card", err), nil
	}

	return jsonToolResult(map[string]any{
		"uri":      fmt.Sprintf("card://%s/%s", card.Collection, card.ID),
		"filePath": savedPath,
		"card":     card,
	})
}

func (s *HubServer) handleBatchUpsertCards(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	collection := strings.TrimSpace(req.GetString("collection", ""))
	if collection == "" {
		return mcplib.NewToolResultError("collection parameter is required"), nil
	}
	args := req.GetArguments()
	rawCards, ok := args["cards"]
	if !ok || rawCards == nil {
		return mcplib.NewToolResultError("cards parameter is required"), nil
	}
	var cards []models.SearchCard
	if err := decodeArg(rawCards, &cards); err != nil {
		return mcplib.NewToolResultErrorFromErr("invalid cards array", err), nil
	}
	if len(cards) == 0 {
		return mcplib.NewToolResultError("cards array is empty"), nil
	}

	col := models.Slugify(collection)
	for i := range cards {
		if err := models.ValidateAndNormalizeCard(&cards[i], col); err != nil {
			return mcplib.NewToolResultErrorFromErr(fmt.Sprintf("card %d invalid", i), err), nil
		}
	}

	var savedPath string
	if s.deps.DataDir != "" {
		var err error
		savedPath, _, err = indexer.BatchUpsertCardsToDisk(s.deps.DataDir, col, cards)
		if err != nil {
			return mcplib.NewToolResultErrorFromErr("failed to persist cards to disk", err), nil
		}
		loadedCol, loadedCards, loadErr := indexer.LoadCollectionFile(savedPath)
		if loadErr != nil {
			return mcplib.NewToolResultErrorFromErr("failed to reload saved collection", loadErr), nil
		}
		if err := s.deps.Store.ReplaceCollection(ctx, loadedCol, loadedCards, savedPath); err != nil {
			return mcplib.NewToolResultErrorFromErr("failed to index collection", err), nil
		}
	} else {
		for _, c := range cards {
			if err := s.deps.Store.UpsertCard(ctx, c, ""); err != nil {
				return mcplib.NewToolResultErrorFromErr("failed to index card", err), nil
			}
		}
	}

	return jsonToolResult(map[string]any{
		"collection":    col,
		"upsertedCount": len(cards),
		"filePath":      savedPath,
	})
}

func (s *HubServer) handleListPlugins(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	list := s.deps.Plugins.ListPlugins()
	type pluginSummary struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		Description      string   `json:"description,omitempty"`
		Version          string   `json:"version,omitempty"`
		Modifiers        []string `json:"modifiers,omitempty"`
		StaticCardCount  int      `json:"staticCardCount"`
		HasDynamicSource bool     `json:"hasDynamicSource"`
	}
	out := make([]pluginSummary, 0, len(list))
	for _, p := range list {
		out = append(out, pluginSummary{
			ID:               p.ID,
			Name:             p.Name,
			Description:      p.Description,
			Version:          p.Version,
			Modifiers:        p.Modifiers,
			StaticCardCount:  len(p.StaticCards),
			HasDynamicSource: p.DynamicSource != nil,
		})
	}
	return jsonToolResult(map[string]any{
		"count":   len(out),
		"plugins": out,
	})
}

func (s *HubServer) handleExecuteCardAction(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	cardID := strings.TrimSpace(req.GetString("card_id", ""))
	if cardID == "" {
		return mcplib.NewToolResultError("card_id parameter is required"), nil
	}
	card, err := s.deps.Store.GetCard(ctx, cardID)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to fetch card", err), nil
	}
	if card == nil {
		return mcplib.NewToolResultErrorf("card %q not found", cardID), nil
	}

	actionID := strings.TrimSpace(req.GetString("action_id", ""))
	query := req.GetString("query", "")
	parsed := search.Parse(query)

	var extraArgs map[string]string
	if rawArgs, ok := req.GetArguments()["args"]; ok && rawArgs != nil {
		var tmp map[string]any
		if err := decodeArg(rawArgs, &tmp); err == nil && len(tmp) > 0 {
			extraArgs = make(map[string]string, len(tmp))
			for k, v := range tmp {
				extraArgs[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	result := s.deps.Runner.Execute(ctx, *card, actionID, parsed, extraArgs)
	return jsonToolResult(result)
}

func (s *HubServer) handleGetRelatedCards(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	cardID := strings.TrimSpace(req.GetString("card_id", ""))
	if cardID == "" {
		return mcplib.NewToolResultError("card_id parameter is required"), nil
	}
	limit := req.GetInt("limit", 5)
	related, err := s.deps.Store.FindRelatedCards(ctx, cardID, limit)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("failed to find related cards", err), nil
	}
	return jsonToolResult(related)
}

func (s *HubServer) handleUpsertPatternRule(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if strings.TrimSpace(s.deps.PatternsDir) == "" {
		return mcplib.NewToolResultError("patterns directory is not configured on this MCP server"), nil
	}

	var rule models.PatternRule
	if rawStr := strings.TrimSpace(req.GetString("rule_json", "")); rawStr != "" {
		if err := json.Unmarshal([]byte(rawStr), &rule); err != nil {
			return mcplib.NewToolResultErrorFromErr("invalid rule_json", err), nil
		}
	} else if args := req.GetArguments(); args != nil {
		if rawObj, ok := args["rule"]; ok && rawObj != nil {
			b, err := json.Marshal(rawObj)
			if err != nil {
				return mcplib.NewToolResultErrorFromErr("marshal rule object", err), nil
			}
			if err := json.Unmarshal(b, &rule); err != nil {
				return mcplib.NewToolResultErrorFromErr("decode PatternRule", err), nil
			}
		}
	}

	savedPath, err := indexer.SavePatternRule(s.deps.PatternsDir, rule)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("save pattern rule", err), nil
	}
	if s.deps.OnPatternsChanged != nil {
		s.deps.OnPatternsChanged()
	}
	return jsonToolResult(map[string]any{
		"status":   "saved_and_reloaded",
		"ruleId":   models.Slugify(rule.ID),
		"filePath": savedPath,
	})
}

func (s *HubServer) handleListPatternRules(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return jsonToolResult(s.deps.Search.PatternRules())
}

func (s *HubServer) handleDeletePatternRule(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if strings.TrimSpace(s.deps.PatternsDir) == "" {
		return mcplib.NewToolResultError("patterns directory is not configured on this MCP server"), nil
	}
	ruleID := strings.TrimSpace(req.GetString("rule_id", ""))
	if ruleID == "" {
		return mcplib.NewToolResultError("rule_id is required"), nil
	}
	if err := indexer.DeletePatternRule(s.deps.PatternsDir, ruleID); err != nil {
		return mcplib.NewToolResultErrorFromErr("delete pattern rule", err), nil
	}
	if s.deps.OnPatternsChanged != nil {
		s.deps.OnPatternsChanged()
	}
	return jsonToolResult(map[string]any{
		"status": "deleted_and_reloaded",
		"ruleId": models.Slugify(ruleID),
	})
}

func (s *HubServer) handleListModules(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	mods, paths, err := indexer.LoadAllModules(s.deps.ModulesDir)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("list modules", err), nil
	}
	summaries := make([]map[string]any, 0, len(mods))
	for _, m := range mods {
		summaries = append(summaries, map[string]any{
			"id":           m.ID,
			"name":         m.Name,
			"category":     m.Category,
			"version":      m.Version,
			"cardCount":    len(m.Cards),
			"overlayCount": len(m.Overlays),
			"patternCount": len(m.Patterns),
			"filePath":     paths[m.ID],
		})
	}
	return jsonToolResult(map[string]any{
		"installed": summaries,
		"starters":  indexer.ListStarterModules(s.deps.ModulesDir),
	})
}

func (s *HubServer) handleGetModule(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	moduleID := models.Slugify(req.GetString("module_id", ""))
	if moduleID == "" || moduleID == "item" {
		return mcplib.NewToolResultError("module_id is required"), nil
	}
	mods, _, err := indexer.LoadAllModules(s.deps.ModulesDir)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("load modules", err), nil
	}
	for _, m := range mods {
		if m.ID == moduleID {
			return jsonToolResult(m)
		}
	}
	return mcplib.NewToolResultError(fmt.Sprintf("module %q is not installed", moduleID)), nil
}

func extractRawModuleJSON(req mcplib.CallToolRequest) ([]byte, error) {
	if rawStr := strings.TrimSpace(req.GetString("module_json", "")); rawStr != "" {
		return []byte(rawStr), nil
	}
	if args := req.GetArguments(); args != nil {
		if rawObj, ok := args["module"]; ok && rawObj != nil {
			if str, isStr := rawObj.(string); isStr {
				return []byte(str), nil
			}
			b, err := json.Marshal(rawObj)
			if err != nil {
				return nil, fmt.Errorf("marshal module object: %w", err)
			}
			return b, nil
		}
	}
	return nil, errors.New("either module or module_json is required")
}

func (s *HubServer) handleValidateModule(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	raw, err := extractRawModuleJSON(req)
	if err != nil {
		return mcplib.NewToolResultError(err.Error()), nil
	}
	mod, res := models.ValidateModuleJSON(raw)
	payload := map[string]any{
		"valid":  res.Valid,
		"errors": res.Errors,
	}
	if mod != nil {
		payload["moduleId"] = mod.ID
		payload["cardCount"] = len(mod.Cards)
		payload["patternCount"] = len(mod.Patterns)
	}
	return jsonToolResult(payload)
}

func (s *HubServer) handleUpsertModule(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if strings.TrimSpace(s.deps.ModulesDir) == "" {
		return mcplib.NewToolResultError("modules directory is not configured on this MCP server"), nil
	}
	raw, err := extractRawModuleJSON(req)
	if err != nil {
		return mcplib.NewToolResultError(err.Error()), nil
	}
	mod, valRes := models.ValidateModuleJSON(raw)
	if !valRes.Valid || mod == nil {
		return mcplib.NewToolResultError("module validation failed: " + valRes.Err().Error()), nil
	}
	saved, err := indexer.SaveCustomModule(s.deps.ModulesDir, *mod)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("save module", err), nil
	}
	if s.deps.OnModulesChanged != nil {
		s.deps.OnModulesChanged()
	}
	return jsonToolResult(map[string]any{
		"status":       "saved_and_reindexed",
		"moduleId":     saved.ID,
		"cardCount":    len(saved.Cards),
		"patternCount": len(saved.Patterns),
	})
}

func (s *HubServer) handleDeleteModule(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if strings.TrimSpace(s.deps.ModulesDir) == "" {
		return mcplib.NewToolResultError("modules directory is not configured on this MCP server"), nil
	}
	moduleID := strings.TrimSpace(req.GetString("module_id", ""))
	if moduleID == "" {
		return mcplib.NewToolResultError("module_id is required"), nil
	}
	if err := indexer.DeleteModule(s.deps.ModulesDir, moduleID); err != nil {
		return mcplib.NewToolResultErrorFromErr("delete module", err), nil
	}
	if s.deps.OnModulesChanged != nil {
		s.deps.OnModulesChanged()
	}
	return jsonToolResult(map[string]any{
		"status":   "deleted_and_reindexed",
		"moduleId": models.Slugify(moduleID),
	})
}

func (s *HubServer) handleInstallStarterModule(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	if strings.TrimSpace(s.deps.ModulesDir) == "" {
		return mcplib.NewToolResultError("modules directory is not configured on this MCP server"), nil
	}
	moduleID := strings.TrimSpace(req.GetString("module_id", ""))
	if moduleID == "" {
		return mcplib.NewToolResultError("module_id is required"), nil
	}
	saved, err := indexer.InstallStarterModule(s.deps.ModulesDir, moduleID)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("install starter module", err), nil
	}
	if s.deps.OnModulesChanged != nil {
		s.deps.OnModulesChanged()
	}
	return jsonToolResult(map[string]any{
		"status":       "installed_and_indexed",
		"moduleId":     saved.ID,
		"cardCount":    len(saved.Cards),
		"patternCount": len(saved.Patterns),
	})
}

type sharedSkillSummary struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
}

func (s *HubServer) discoverSkills() []sharedSkillSummary {
	seen := make(map[string]bool)
	var out []sharedSkillSummary
	for _, dir := range s.deps.SkillsDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			slug := models.Slugify(e.Name())
			if slug == "" || seen[slug] {
				continue
			}
			skillPath := filepath.Join(dir, e.Name(), "SKILL.md")
			raw, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}
			seen[slug] = true
			out = append(out, sharedSkillSummary{
				Name:        slug,
				URI:         "skill://" + slug,
				Description: extractSkillFrontmatterDesc(string(raw)),
			})
		}
	}
	return out
}

func (s *HubServer) readSkillMarkdown(slug string) (string, error) {
	clean := models.Slugify(slug)
	if clean == "" || clean == "item" {
		return "", errors.New("valid skill name is required")
	}
	for _, dir := range s.deps.SkillsDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && models.Slugify(e.Name()) == clean {
				raw, readErr := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md"))
				if readErr == nil {
					return string(raw), nil
				}
			}
		}
	}
	return "", fmt.Errorf("skill %q not found", clean)
}

func extractSkillFrontmatterDesc(md string) string {
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "description:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "description:")), `"'`)
		}
	}
	return ""
}

func (s *HubServer) handleListSkills(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	return jsonToolResult(s.discoverSkills())
}

func (s *HubServer) handleGetSkill(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	name := strings.TrimSpace(req.GetString("name", ""))
	if name == "" {
		return mcplib.NewToolResultError("name is required"), nil
	}
	md, err := s.readSkillMarkdown(name)
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("read skill", err), nil
	}
	return mcplib.NewToolResultText(md), nil
}

// --- Helpers ---

func readRawInput(req mcplib.CallToolRequest) ([]byte, string, error) {
	filePath := strings.TrimSpace(req.GetString("file_path", ""))
	rawContent := req.GetString("raw_content", "")
	hint := strings.TrimSpace(req.GetString("filename_hint", ""))

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, "", fmt.Errorf("read file_path %q: %w", filePath, err)
		}
		if hint == "" {
			hint = filepath.Base(filePath)
		}
		return data, hint, nil
	}
	if strings.TrimSpace(rawContent) != "" {
		return []byte(rawContent), hint, nil
	}
	return nil, "", errors.New("either file_path or raw_content is required")
}

func decodeArg(raw any, dst any) error {
	if str, ok := raw.(string); ok {
		return json.Unmarshal([]byte(str), dst)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func jsonToolResult(v any) (*mcplib.CallToolResult, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcplib.NewToolResultErrorFromErr("marshal result", err), nil
	}
	return mcplib.NewToolResultText(string(raw)), nil
}
