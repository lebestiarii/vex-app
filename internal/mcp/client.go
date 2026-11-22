package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"vex/internal/indexer"
	"vex/internal/models"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ExternalServerConfig defines an external MCP server that Vex connects to as a client.
type ExternalServerConfig struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Transport string            `json:"transport"` // "stdio", "sse", "http" (streamable-http), "inprocess"
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       []string          `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Enabled   bool              `json:"enabled"`
	LocalOnly bool              `json:"localOnly,omitempty"` // Mark all indexed cards & live pattern results localOnly
}

// ConnectedServerStatus summarizes a connected external MCP server and its indexed cards.
type ConnectedServerStatus struct {
	Config        ExternalServerConfig `json:"config"`
	Connected     bool                 `json:"connected"`
	ToolCount     int                  `json:"toolCount"`
	ResourceCount int                  `json:"resourceCount"`
	CardCount     int                  `json:"cardCount"`
	LastSynced    time.Time            `json:"lastSynced,omitempty"`
	LastError     string               `json:"lastError,omitempty"`
}

type managedClient struct {
	cfg           ExternalServerConfig
	client        *client.Client
	toolCount     int
	resourceCount int
	cardCount     int
	lastSynced    time.Time
	lastError     string
}

// ClientManager connects to external MCP servers, discovers their tools and resources,
// and indexes them as virtual SearchCards under @mcp:<server-id> (collection "mcp-<server-id>").
type ClientManager struct {
	store *indexer.Store

	mu      sync.RWMutex
	clients map[string]*managedClient
	order   []string
}

// NewClientManager creates an External MCP Client Manager backed by the hybrid SQLite Store.
func NewClientManager(store *indexer.Store) *ClientManager {
	return &ClientManager{
		store:   store,
		clients: make(map[string]*managedClient),
	}
}

// Connect establishes a connection to an external MCP server, initializes the protocol,
// and indexes its tools and resources as searchable cards.
func (m *ClientManager) Connect(ctx context.Context, cfg ExternalServerConfig) (*ConnectedServerStatus, error) {
	cfg, err := normalizeExternalConfig(cfg)
	if err != nil {
		return nil, err
	}

	var c *client.Client
	switch strings.ToLower(cfg.Transport) {
	case "stdio":
		if strings.TrimSpace(cfg.Command) == "" {
			return nil, errors.New("stdio MCP server requires command")
		}
		c, err = client.NewStdioMCPClient(cfg.Command, cfg.Env, cfg.Args...)
	case "sse":
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, errors.New("sse MCP server requires url")
		}
		var opts []transport.ClientOption
		if len(cfg.Headers) > 0 {
			opts = append(opts, client.WithHeaders(cfg.Headers))
		}
		c, err = client.NewSSEMCPClient(cfg.URL, opts...)
	case "http", "streamable-http":
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, errors.New("http MCP server requires url")
		}
		var opts []transport.StreamableHTTPCOption
		if len(cfg.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(cfg.Headers))
		}
		c, err = client.NewStreamableHttpClient(cfg.URL, opts...)
	default:
		return nil, fmt.Errorf("unsupported MCP client transport %q (supported: stdio, sse, http)", cfg.Transport)
	}
	if err != nil {
		return nil, fmt.Errorf("create MCP client %q: %w", cfg.ID, err)
	}

	if err := c.Start(ctx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("start MCP client %q: %w", cfg.ID, err)
	}

	return m.attachClient(ctx, cfg, c)
}

// AttachInProcess connects directly to an in-process MCPServer without binding any ports.
// Useful for embedded Wails/MCP workflows and sandbox-safe integration tests.
func (m *ClientManager) AttachInProcess(ctx context.Context, cfg ExternalServerConfig, srv *server.MCPServer) (*ConnectedServerStatus, error) {
	if srv == nil {
		return nil, errors.New("in-process MCP server is nil")
	}
	cfg.Transport = "inprocess"
	cfg, err := normalizeExternalConfig(cfg)
	if err != nil {
		return nil, err
	}
	c, err := client.NewInProcessClient(srv)
	if err != nil {
		return nil, fmt.Errorf("create in-process MCP client %q: %w", cfg.ID, err)
	}
	if err := c.Start(ctx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("start in-process MCP client %q: %w", cfg.ID, err)
	}
	return m.attachClient(ctx, cfg, c)
}

func (m *ClientManager) attachClient(ctx context.Context, cfg ExternalServerConfig, c *client.Client) (*ConnectedServerStatus, error) {
	initReq := mcplib.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcplib.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcplib.Implementation{
		Name:    "Vex",
		Version: "0.1.0",
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("initialize MCP client %q: %w", cfg.ID, err)
	}

	mc := &managedClient{
		cfg:    cfg,
		client: c,
	}

	m.mu.Lock()
	if prev, exists := m.clients[cfg.ID]; exists {
		_ = prev.client.Close()
	} else {
		m.order = append(m.order, cfg.ID)
	}
	m.clients[cfg.ID] = mc
	m.mu.Unlock()

	return m.SyncServer(ctx, cfg.ID)
}

// SyncServer discovers tools and resources from a connected MCP server and indexes them as SearchCards.
func (m *ClientManager) SyncServer(ctx context.Context, serverID string) (*ConnectedServerStatus, error) {
	id := models.Slugify(serverID)

	m.mu.RLock()
	mc, ok := m.clients[id]
	m.mu.RUnlock()
	if !ok || mc == nil || mc.client == nil {
		return nil, fmt.Errorf("MCP server %q is not connected", id)
	}

	var tools []mcplib.Tool
	if toolsRes, err := mc.client.ListTools(ctx, mcplib.ListToolsRequest{}); err == nil && toolsRes != nil {
		tools = toolsRes.Tools
	}

	var resources []mcplib.Resource
	if resRes, err := mc.client.ListResources(ctx, mcplib.ListResourcesRequest{}); err == nil && resRes != nil {
		resources = resRes.Resources
	}

	cards := buildVirtualCards(mc.cfg, tools, resources)
	if m.store != nil {
		collection := "mcp-" + id
		sourceKey := "mcp://" + id
		if err := m.store.ReplaceCollection(ctx, collection, cards, sourceKey); err != nil {
			m.mu.Lock()
			mc.lastError = err.Error()
			m.mu.Unlock()
			return nil, fmt.Errorf("index virtual MCP cards for %q: %w", id, err)
		}
	}

	now := time.Now().UTC()
	m.mu.Lock()
	mc.toolCount = len(tools)
	mc.resourceCount = len(resources)
	mc.cardCount = len(cards)
	mc.lastSynced = now
	mc.lastError = ""
	status := mc.toStatus()
	m.mu.Unlock()

	return &status, nil
}

// CallTool invokes a tool on a connected external MCP server.
func (m *ClientManager) CallTool(ctx context.Context, serverID, toolName string, args map[string]any) (*mcplib.CallToolResult, error) {
	id := models.Slugify(serverID)
	m.mu.RLock()
	mc, ok := m.clients[id]
	m.mu.RUnlock()
	if !ok || mc == nil || mc.client == nil {
		return nil, fmt.Errorf("MCP server %q is not connected", id)
	}

	req := mcplib.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args
	return mc.client.CallTool(ctx, req)
}

// ExecutePatternMCP satisfies search.LivePatternExecutor by calling a tool on a
// connected MCP server and returning the concatenated text response alongside
// the server's LocalOnly guardrail flag.
func (m *ClientManager) ExecutePatternMCP(ctx context.Context, serverID, toolName string, args map[string]any) (string, bool, error) {
	id := models.Slugify(serverID)
	m.mu.RLock()
	mc, ok := m.clients[id]
	m.mu.RUnlock()
	if !ok || mc == nil || mc.client == nil {
		return "", false, fmt.Errorf("server %q not connected", id)
	}

	req := mcplib.CallToolRequest{}
	req.Params.Name = toolName
	req.Params.Arguments = args
	res, err := mc.client.CallTool(ctx, req)
	if err != nil {
		return "", mc.cfg.LocalOnly, err
	}
	if res == nil {
		return "", mc.cfg.LocalOnly, nil
	}

	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok && strings.TrimSpace(tc.Text) != "" {
			parts = append(parts, tc.Text)
		}
	}
	out := strings.TrimSpace(strings.Join(parts, "\n"))
	if res.IsError {
		if out == "" {
			out = "MCP tool returned error"
		}
		return "", mc.cfg.LocalOnly, errors.New(out)
	}
	return out, mc.cfg.LocalOnly, nil
}

// ReadResource reads a resource URI from a connected external MCP server.
func (m *ClientManager) ReadResource(ctx context.Context, serverID, uri string) (*mcplib.ReadResourceResult, error) {
	id := models.Slugify(serverID)
	m.mu.RLock()
	mc, ok := m.clients[id]
	m.mu.RUnlock()
	if !ok || mc == nil || mc.client == nil {
		return nil, fmt.Errorf("MCP server %q is not connected", id)
	}

	req := mcplib.ReadResourceRequest{}
	req.Params.URI = uri
	return mc.client.ReadResource(ctx, req)
}

// Disconnect closes a connected MCP server and removes its virtual cards from the index.
func (m *ClientManager) Disconnect(ctx context.Context, serverID string) error {
	id := models.Slugify(serverID)
	m.mu.Lock()
	mc, ok := m.clients[id]
	if ok {
		delete(m.clients, id)
		filtered := m.order[:0]
		for _, k := range m.order {
			if k != id {
				filtered = append(filtered, k)
			}
		}
		m.order = filtered
	}
	m.mu.Unlock()

	var closeErr error
	if ok && mc != nil && mc.client != nil {
		closeErr = mc.client.Close()
	}
	if m.store != nil {
		_ = m.store.DeleteBySourceFile(ctx, "mcp://"+id)
	}
	return closeErr
}

// CloseAll disconnects all external MCP clients cleanly.
func (m *ClientManager) CloseAll(ctx context.Context) {
	m.mu.RLock()
	ids := append([]string(nil), m.order...)
	m.mu.RUnlock()
	for _, id := range ids {
		_ = m.Disconnect(ctx, id)
	}
}

// ListServers returns status summaries for all connected external MCP servers.
func (m *ClientManager) ListServers() []ConnectedServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ConnectedServerStatus, 0, len(m.order))
	for _, id := range m.order {
		if mc, ok := m.clients[id]; ok {
			out = append(out, mc.toStatus())
		}
	}
	return out
}

func (mc *managedClient) toStatus() ConnectedServerStatus {
	return ConnectedServerStatus{
		Config:        mc.cfg,
		Connected:     mc.client != nil,
		ToolCount:     mc.toolCount,
		ResourceCount: mc.resourceCount,
		CardCount:     mc.cardCount,
		LastSynced:    mc.lastSynced,
		LastError:     mc.lastError,
	}
}

func normalizeExternalConfig(cfg ExternalServerConfig) (ExternalServerConfig, error) {
	cfg.ID = models.Slugify(cfg.ID)
	if cfg.ID == "" || cfg.ID == "item" {
		if strings.TrimSpace(cfg.Name) != "" {
			cfg.ID = models.Slugify(cfg.Name)
		} else {
			return cfg, errors.New("external MCP server config requires id or name")
		}
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = cfg.ID
	}
	if strings.TrimSpace(cfg.Transport) == "" {
		if strings.TrimSpace(cfg.URL) != "" {
			cfg.Transport = "http"
		} else {
			cfg.Transport = "stdio"
		}
	}
	return cfg, nil
}

func buildVirtualCards(cfg ExternalServerConfig, tools []mcplib.Tool, resources []mcplib.Resource) []models.SearchCard {
	now := time.Now().UTC()
	collection := "mcp-" + cfg.ID // Matches @mcp:<id> and @mcp-<id> via Slugify
	cards := make([]models.SearchCard, 0, len(tools)+len(resources))

	for _, tool := range tools {
		desc := strings.TrimSpace(tool.Description)
		if desc == "" {
			desc = fmt.Sprintf("External MCP tool %q provided by %s.", tool.Name, cfg.Name)
		}
		card := models.SearchCard{
			ID:          fmt.Sprintf("mcp-%s-tool-%s", cfg.ID, models.Slugify(tool.Name)),
			Title:       tool.Name,
			Subtitle:    fmt.Sprintf("MCP Tool • %s", cfg.Name),
			Description: desc,
			Collection:  collection,
			Template:    models.TemplateCommand,
			Icon:        "plug",
			Tags:        []string{"mcp", cfg.ID, "mcp-tool"},
			Modifiers:   []string{"mcp", cfg.ID},
			Data: map[string]any{
				"mcpServerId": cfg.ID,
				"mcpServer":   cfg.Name,
				"mcpKind":     "tool",
				"toolName":    tool.Name,
				"inputSchema": tool.InputSchema,
			},
			Actions: []models.CardAction{
				{
					ID:       "ask-tool",
					Label:    "Use Tool with AI",
					Shortcut: "Enter",
					Type:     "ai_prompt",
					Target:   fmt.Sprintf("Use MCP tool %s from server %s for: {{query}}", tool.Name, cfg.ID),
				},
				{
					ID:       "copy-tool",
					Label:    "Copy Tool Name",
					Shortcut: "Cmd+C",
					Type:     "copy",
					Target:   tool.Name,
				},
			},
			LocalOnly: cfg.LocalOnly,
			Provenance: &models.Provenance{
				SourceName: cfg.Name + " (MCP)",
				Authority:  "automated-sync",
				LocalOnly:  cfg.LocalOnly,
			},
			UpdatedAt: now,
		}
		if err := models.ValidateAndNormalizeCard(&card, collection); err == nil {
			cards = append(cards, card)
		}
	}

	for _, res := range resources {
		title := strings.TrimSpace(res.Name)
		if title == "" {
			title = res.URI
		}
		desc := strings.TrimSpace(res.Description)
		if desc == "" {
			desc = fmt.Sprintf("External MCP resource %s provided by %s.", res.URI, cfg.Name)
		}
		card := models.SearchCard{
			ID:          fmt.Sprintf("mcp-%s-res-%s", cfg.ID, models.Slugify(res.URI)),
			Title:       title,
			Subtitle:    res.URI,
			Description: desc,
			Collection:  collection,
			Template:    models.TemplateKnowledge,
			Icon:        "database",
			Tags:        []string{"mcp", cfg.ID, "mcp-resource"},
			Modifiers:   []string{"mcp", cfg.ID},
			Data: map[string]any{
				"mcpServerId": cfg.ID,
				"mcpServer":   cfg.Name,
				"mcpKind":     "resource",
				"uri":         res.URI,
				"mimeType":    res.MIMEType,
			},
			Actions: []models.CardAction{
				{
					ID:       "ask-res",
					Label:    "Summarize Resource with AI",
					Shortcut: "Enter",
					Type:     "ai_prompt",
					Target:   fmt.Sprintf("Summarize MCP resource %s from %s", res.URI, cfg.Name),
				},
				{
					ID:       "copy-uri",
					Label:    "Copy Resource URI",
					Shortcut: "Cmd+C",
					Type:     "copy",
					Target:   res.URI,
				},
			},
			LocalOnly: cfg.LocalOnly,
			Provenance: &models.Provenance{
				SourceName: cfg.Name + " (MCP)",
				Authority:  "automated-sync",
				LocalOnly:  cfg.LocalOnly,
			},
			UpdatedAt: now,
		}
		if err := models.ValidateAndNormalizeCard(&card, collection); err == nil {
			cards = append(cards, card)
		}
	}

	return cards
}
