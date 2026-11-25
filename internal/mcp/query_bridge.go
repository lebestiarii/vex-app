package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vex/internal/models"
)

var (
	ghIssueRefRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)#(\d+)$`)
	adoIDRefRe   = regexp.MustCompile(`^(?:(?i:AB)#)?(\d{1,9})$`)
)

// QueryExternalMCP satisfies search.LiveMCPQueryProvider, routing /gh and /ado
// queries to connected external MCP servers ("github" and "azure-devops") and
// mapping responses strictly onto Vex's generic link, facts, action-list, and table layouts.
func (m *ClientManager) QueryExternalMCP(ctx context.Context, modifier, query string, limit int) ([]models.SearchCard, error) {
	if limit <= 0 {
		limit = 8
	}
	if limit > 8 {
		limit = 8
	}

	mod := strings.ToLower(strings.TrimSpace(modifier))
	q := strings.TrimSpace(query)

	switch mod {
	case "gh", "github":
		return m.queryGitHubMCP(ctx, q, limit)
	case "ado", "azure-devops":
		return m.queryAzureDevOpsMCP(ctx, q, limit)
	default:
		return nil, nil
	}
}

func (m *ClientManager) getManagedServer(serverID string) (ExternalServerConfig, []string, bool) {
	id := models.Slugify(serverID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	mc, ok := m.clients[id]
	if !ok || mc == nil || mc.client == nil {
		return ExternalServerConfig{}, nil, false
	}
	names := append([]string(nil), mc.toolNames...)
	return mc.cfg, names, true
}

func pickToolName(available []string, preferred ...string) string {
	availSet := make(map[string]string, len(available))
	for _, n := range available {
		availSet[strings.ToLower(strings.TrimSpace(n))] = n
	}
	for _, want := range preferred {
		if actual, ok := availSet[strings.ToLower(want)]; ok {
			return actual
		}
	}
	return ""
}

func (m *ClientManager) queryGitHubMCP(ctx context.Context, query string, limit int) ([]models.SearchCard, error) {
	cfg, tools, connected := m.getManagedServer("github")
	if !connected {
		return []models.SearchCard{disconnectedServerHintCard("github", "GitHub MCP Server", "/gh <query>", "gh auth login")}, nil
	}

	if query == "" {
		return []models.SearchCard{connectedUsageGuideCard(
			"github",
			"GitHub MCP Connected",
			"Type `/gh <repo>`, `/gh owner/repo#123`, `/gh pr:<term>`, or `/gh issue:<term>` to query live GitHub data.",
			tools,
			cfg.LocalOnly,
		)}, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	// 1. Direct owner/repo#num lookup
	if sub := ghIssueRefRe.FindStringSubmatch(query); len(sub) == 4 {
		owner, repo, numStr := sub[1], sub[2], sub[3]
		tool := pickToolName(tools, "get_issue", "get_pull_request", "issue_read")
		if tool != "" {
			numInt, _ := strconv.Atoi(numStr)
			args := map[string]any{
				"owner":        owner,
				"repo":         repo,
				"issue_number": numInt,
				"pull_number":  numInt,
			}
			text, localOnly, err := m.ExecutePatternMCP(reqCtx, "github", tool, args)
			if err == nil && strings.TrimSpace(text) != "" {
				return mapGitHubPayloadToCards(text, query, localOnly || cfg.LocalOnly, limit), nil
			}
		}
	}

	// 2. Search PRs / Issues / Repositories
	lowerQ := strings.ToLower(query)
	var tool string
	var searchText string
	switch {
	case strings.HasPrefix(lowerQ, "pr:") || strings.HasPrefix(lowerQ, "pull:"):
		searchText = strings.TrimSpace(query[strings.IndexByte(query, ':')+1:])
		tool = pickToolName(tools, "search_pull_requests", "search_issues", "list_pull_requests")
	case strings.HasPrefix(lowerQ, "issue:"):
		searchText = strings.TrimSpace(query[strings.IndexByte(query, ':')+1:])
		tool = pickToolName(tools, "search_issues", "list_issues")
	default:
		searchText = query
		tool = pickToolName(tools, "search_repositories", "search_issues", "search_pull_requests", "search_code")
	}

	if tool == "" {
		return []models.SearchCard{connectedUsageGuideCard(
			"github",
			"GitHub MCP — No Search Tool Found",
			"Connected server does not advertise search_repositories, search_issues, or get_issue.",
			tools,
			cfg.LocalOnly,
		)}, nil
	}

	text, localOnly, err := m.ExecutePatternMCP(reqCtx, "github", tool, map[string]any{
		"query": searchText,
		"q":     searchText,
	})
	if err != nil {
		return []models.SearchCard{liveQueryErrorCard("github", "GitHub Lookup Unavailable", err.Error(), cfg.LocalOnly)}, nil
	}
	cards := mapGitHubPayloadToCards(text, query, localOnly || cfg.LocalOnly, limit)
	if len(cards) == 0 {
		return []models.SearchCard{liveQueryErrorCard("github", "No GitHub Matches", fmt.Sprintf("No GitHub items matched %q.", query), cfg.LocalOnly)}, nil
	}
	return cards, nil
}

func (m *ClientManager) queryAzureDevOpsMCP(ctx context.Context, query string, limit int) ([]models.SearchCard, error) {
	cfg, tools, connected := m.getManagedServer("azure-devops")
	if !connected {
		return []models.SearchCard{disconnectedServerHintCard("azure-devops", "Azure DevOps MCP Server", "/ado <id or query>", "az login")}, nil
	}

	if query == "" {
		return []models.SearchCard{connectedUsageGuideCard(
			"azure-devops",
			"Azure DevOps MCP Connected",
			"Type `/ado 12345`, `/ado AB#12345`, or `/ado <search text>` to query live work items.",
			tools,
			cfg.LocalOnly,
		)}, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	// 1. Direct numeric / AB#12345 work item lookup
	if sub := adoIDRefRe.FindStringSubmatch(query); len(sub) == 2 {
		idStr := sub[1]
		tool := pickToolName(tools, "wit_get_work_item", "get_work_item")
		if tool != "" {
			text, localOnly, err := m.ExecutePatternMCP(reqCtx, "azure-devops", tool, map[string]any{
				"id": idStr,
			})
			if err != nil {
				return []models.SearchCard{liveQueryErrorCard("azure-devops", "Azure DevOps Lookup Unavailable", err.Error(), cfg.LocalOnly)}, nil
			}
			cards := mapADOPayloadToCards(text, query, localOnly || cfg.LocalOnly, limit)
			if len(cards) > 0 {
				return cards, nil
			}
		}
	}

	// 2. Search work items
	tool := pickToolName(tools, "wit_search_work_items", "search_workitem", "wit_my_work_items", "search_work_items")
	if tool == "" {
		return []models.SearchCard{connectedUsageGuideCard(
			"azure-devops",
			"Azure DevOps MCP — No Search Tool Found",
			"Connected server does not advertise wit_search_work_items or wit_get_work_item.",
			tools,
			cfg.LocalOnly,
		)}, nil
	}

	text, localOnly, err := m.ExecutePatternMCP(reqCtx, "azure-devops", tool, map[string]any{
		"query":      query,
		"searchText": query,
	})
	if err != nil {
		return []models.SearchCard{liveQueryErrorCard("azure-devops", "Azure DevOps Lookup Unavailable", err.Error(), cfg.LocalOnly)}, nil
	}
	cards := mapADOPayloadToCards(text, query, localOnly || cfg.LocalOnly, limit)
	if len(cards) == 0 {
		return []models.SearchCard{liveQueryErrorCard("azure-devops", "No Azure DevOps Matches", fmt.Sprintf("No work items matched %q.", query), cfg.LocalOnly)}, nil
	}
	return cards, nil
}

// mapGitHubPayloadToCards projects GitHub MCP JSON payloads (repos, PRs, issues)
// strictly onto Vex's generic link, facts, action-list, and table layouts.
func mapGitHubPayloadToCards(rawText, query string, localOnly bool, limit int) []models.SearchCard {
	now := time.Now().UTC()
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return nil
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		card := models.SearchCard{
			ID:         "mcp-gh-text-" + models.Slugify(query),
			Title:      "GitHub · " + query,
			Collection: "mcp-github",
			Layout:     models.LayoutFacts,
			Icon:       "🐙",
			Data:       map[string]any{"summary": trimmed},
			LocalOnly:  localOnly,
			UpdatedAt:  now,
		}
		if models.ValidateAndNormalizeCard(&card, "mcp-github") == nil {
			return []models.SearchCard{card}
		}
		return nil
	}

	items := extractJSONObjectSlice(parsed, "items", "repositories", "pull_requests", "issues", "results")
	if len(items) == 0 {
		if obj, ok := parsed.(map[string]any); ok {
			items = []map[string]any{obj}
		}
	}
	if len(items) == 0 {
		return nil
	}

	if len(items) > limit {
		items = items[:limit]
	}

	// Multi-item repository list -> action-list card with Open & Clone/Copy actions
	if len(items) > 1 && isGitHubRepoItem(items[0]) {
		rows := make([]map[string]any, 0, len(items))
		for _, item := range items {
			name := firstNonEmptyStr(stringField(item, "full_name"), stringField(item, "name"))
			if name == "" {
				continue
			}
			htmlURL := firstValidHTTPURL(stringField(item, "html_url"), stringField(item, "url"))
			cloneURL := firstNonEmptyStr(stringField(item, "clone_url"), stringField(item, "ssh_url"), htmlURL)
			lang := stringField(item, "language")
			desc := stringField(item, "description")

			row := map[string]any{
				"primary": name,
				"mono":    true,
			}
			if desc != "" {
				row["subtitle"] = desc
			}
			if lang != "" {
				row["badge"] = map[string]any{"label": lang, "tone": "blue"}
			}
			var acts []map[string]any
			if htmlURL != "" {
				acts = append(acts, map[string]any{"label": "Open", "type": "open_url", "value": htmlURL, "tone": "blue"})
			}
			if cloneURL != "" {
				acts = append(acts, map[string]any{"label": "Copy", "type": "copy", "value": cloneURL})
			}
			if len(acts) == 0 {
				acts = append(acts, map[string]any{"label": "Copy", "type": "copy", "value": name})
			}
			row["actions"] = acts
			rows = append(rows, row)
		}
		if len(rows) > 0 {
			card := models.SearchCard{
				ID:              "mcp-gh-repos-" + models.Slugify(query),
				Title:           fmt.Sprintf("GitHub Repositories · %s", query),
				Collection:      "mcp-github",
				Layout:          models.LayoutActionList,
				Icon:            "🐙",
				Chips:           []models.GlanceChip{{Label: fmt.Sprintf("%d repos", len(rows)), Tone: "blue"}},
				Data:            map[string]any{"rows": rows},
				DefaultExpanded: true,
				LocalOnly:       localOnly,
				UpdatedAt:       now,
			}
			if models.ValidateAndNormalizeCard(&card, "mcp-github") == nil {
				return []models.SearchCard{card}
			}
		}
	}

	// Multi-item issues/PRs -> summary table card + individual link cards
	if len(items) > 1 {
		var cards []models.SearchCard
		tableRows := make([][]string, 0, len(items))
		for _, item := range items {
			num := stringField(item, "number")
			if num != "" {
				num = "#" + num
			} else {
				num = firstNonEmptyStr(stringField(item, "id"), "—")
			}
			title := firstNonEmptyStr(stringField(item, "title"), stringField(item, "full_name"), stringField(item, "name"), "Item")
			state := firstNonEmptyStr(stringField(item, "state"), "open")
			tableRows = append(tableRows, []string{num, title, state})
		}
		tbl := models.SearchCard{
			ID:              "mcp-gh-table-" + models.Slugify(query),
			Title:           fmt.Sprintf("GitHub Results · %s", query),
			Collection:      "mcp-github",
			Layout:          models.LayoutTable,
			Icon:            "🐙",
			Chips:           []models.GlanceChip{{Label: fmt.Sprintf("%d items", len(tableRows)), Tone: "blue"}},
			Data:            map[string]any{"columns": []string{"#", "Title", "State"}, "rows": tableRows, "monoColumns": []int{0}},
			DefaultExpanded: true,
			LocalOnly:       localOnly,
			UpdatedAt:       now,
		}
		if models.ValidateAndNormalizeCard(&tbl, "mcp-github") == nil {
			cards = append(cards, tbl)
		}
		for i, item := range items {
			if linkCard, ok := buildSingleLinkCard("mcp-gh-link", i, item, "mcp-github", "🐙", localOnly, now); ok {
				cards = append(cards, linkCard)
			}
		}
		return cards
	}

	// Single GitHub item (Repo, PR, or Issue)
	item := items[0]
	htmlURL := firstValidHTTPURL(stringField(item, "html_url"), stringField(item, "url"))
	title := firstNonEmptyStr(stringField(item, "title"), stringField(item, "full_name"), stringField(item, "name"), query)
	state := stringField(item, "state")
	author := nestedStringField(item, "user", "login")
	lang := stringField(item, "language")
	desc := firstNonEmptyStr(stringField(item, "description"), stringField(item, "body"))
	if len(desc) > 220 {
		desc = desc[:217] + "..."
	}

	var facts []map[string]any
	if num := stringField(item, "number"); num != "" {
		facts = append(facts, map[string]any{"label": "Number", "value": "#" + num, "mono": true})
	}
	if state != "" {
		facts = append(facts, map[string]any{"label": "State", "value": state})
	}
	if author != "" {
		facts = append(facts, map[string]any{"label": "Author", "value": "@" + author, "mono": true})
	}
	if lang != "" {
		facts = append(facts, map[string]any{"label": "Language", "value": lang})
	}
	if stars := stringField(item, "stargazers_count"); stars != "" {
		facts = append(facts, map[string]any{"label": "Stars", "value": stars, "mono": true})
	}
	if htmlURL != "" {
		facts = append(facts, map[string]any{"label": "URL", "value": htmlURL, "mono": true, "copyable": true})
	}

	// Pure link with minimal metadata -> single-button link layout
	if len(facts) <= 1 && desc == "" && htmlURL != "" {
		card := models.SearchCard{
			ID:         "mcp-gh-link-" + models.Slugify(title),
			Title:      title,
			Collection: "mcp-github",
			Layout:     models.LayoutLink,
			Icon:       "🐙",
			Data:       map[string]any{"url": htmlURL},
			Actions:    []models.CardAction{{ID: "open", Label: "Open in Browser", Shortcut: "Enter", Type: "open_url", Target: htmlURL}},
			LocalOnly:  localOnly,
			UpdatedAt:  now,
		}
		if models.ValidateAndNormalizeCard(&card, "mcp-github") == nil {
			return []models.SearchCard{card}
		}
	}

	data := map[string]any{}
	if desc != "" {
		data["summary"] = desc
	}
	if len(facts) > 0 {
		data["facts"] = facts
	}
	if len(data) == 0 {
		data["summary"] = title
	}

	var chips []models.GlanceChip
	if state != "" {
		tone := "green"
		if strings.EqualFold(state, "closed") {
			tone = "red"
		}
		chips = append(chips, models.GlanceChip{Label: strings.ToUpper(state), Tone: tone})
	}

	var children []models.SearchCard
	if htmlURL != "" {
		children = append(children, models.SearchCard{
			ID:         "mcp-gh-open-" + models.Slugify(title),
			Title:      "Open on GitHub",
			Collection: "mcp-github",
			Layout:     models.LayoutLink,
			Icon:       "🌐",
			Data:       map[string]any{"url": htmlURL},
			Actions:    []models.CardAction{{ID: "open", Label: "Open on GitHub", Shortcut: "Enter", Type: "open_url", Target: htmlURL}},
			LocalOnly:  localOnly,
			UpdatedAt:  now,
		})
	}

	card := models.SearchCard{
		ID:              "mcp-gh-item-" + models.Slugify(query+"-"+title),
		Title:           title,
		Collection:      "mcp-github",
		Layout:          models.LayoutFacts,
		Icon:            "🐙",
		Chips:           chips,
		Data:            data,
		Children:        children,
		DefaultExpanded: true,
		LocalOnly:       localOnly,
		UpdatedAt:       now,
	}
	if models.ValidateAndNormalizeCard(&card, "mcp-github") == nil {
		return []models.SearchCard{card}
	}
	return nil
}

// mapADOPayloadToCards projects Azure DevOps MCP work item payloads strictly onto
// Vex's generic facts, link, and table layouts with LocalOnly preserved.
func mapADOPayloadToCards(rawText, query string, localOnly bool, limit int) []models.SearchCard {
	now := time.Now().UTC()
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return nil
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		card := models.SearchCard{
			ID:         "mcp-ado-text-" + models.Slugify(query),
			Title:      "Azure DevOps · " + query,
			Collection: "mcp-azure-devops",
			Layout:     models.LayoutFacts,
			Icon:       "🔷",
			Data:       map[string]any{"summary": trimmed},
			LocalOnly:  true,
			UpdatedAt:  now,
		}
		if models.ValidateAndNormalizeCard(&card, "mcp-azure-devops") == nil {
			return []models.SearchCard{card}
		}
		return nil
	}

	items := extractJSONObjectSlice(parsed, "value", "workItems", "items", "results")
	if len(items) == 0 {
		if obj, ok := parsed.(map[string]any); ok {
			items = []map[string]any{obj}
		}
	}
	if len(items) == 0 {
		return nil
	}
	if len(items) > limit {
		items = items[:limit]
	}

	// Multiple work items -> generic table card
	if len(items) > 1 {
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			wi := extractADOWorkItemFields(item)
			rows = append(rows, []string{
				firstNonEmptyStr(wi.id, "—"),
				firstNonEmptyStr(wi.wiType, "Item"),
				firstNonEmptyStr(wi.title, "Untitled"),
				firstNonEmptyStr(wi.state, "Unknown"),
			})
		}
		tbl := models.SearchCard{
			ID:              "mcp-ado-table-" + models.Slugify(query),
			Title:           fmt.Sprintf("Azure DevOps Work Items · %s", query),
			Collection:      "mcp-azure-devops",
			Layout:          models.LayoutTable,
			Icon:            "🔷",
			Chips:           []models.GlanceChip{{Label: fmt.Sprintf("%d items", len(rows)), Tone: "blue"}},
			Data:            map[string]any{"columns": []string{"ID", "Type", "Title", "State"}, "rows": rows, "monoColumns": []int{0}},
			DefaultExpanded: true,
			LocalOnly:       localOnly || true,
			UpdatedAt:       now,
		}
		if models.ValidateAndNormalizeCard(&tbl, "mcp-azure-devops") == nil {
			return []models.SearchCard{tbl}
		}
		return nil
	}

	// Single work item -> generic facts card + optional nested link child card
	wi := extractADOWorkItemFields(items[0])
	title := firstNonEmptyStr(wi.title, "Work Item "+wi.id, query)
	if wi.id != "" && !strings.Contains(title, wi.id) {
		title = fmt.Sprintf("AB#%s · %s", wi.id, title)
	}

	var facts []map[string]any
	if wi.id != "" {
		facts = append(facts, map[string]any{"label": "Work Item ID", "value": "AB#" + wi.id, "mono": true, "copyable": true})
	}
	if wi.wiType != "" {
		facts = append(facts, map[string]any{"label": "Type", "value": wi.wiType})
	}
	if wi.state != "" {
		facts = append(facts, map[string]any{"label": "State", "value": wi.state})
	}
	if wi.assignedTo != "" {
		facts = append(facts, map[string]any{"label": "Assigned To", "value": wi.assignedTo})
	}
	if wi.areaPath != "" {
		facts = append(facts, map[string]any{"label": "Area Path", "value": wi.areaPath, "mono": true})
	}
	if wi.url != "" {
		facts = append(facts, map[string]any{"label": "URL", "value": wi.url, "mono": true, "copyable": true})
	}

	data := map[string]any{}
	if wi.summary != "" {
		data["summary"] = wi.summary
	}
	if len(facts) > 0 {
		data["facts"] = facts
	}
	if len(data) == 0 {
		data["summary"] = title
	}

	var chips []models.GlanceChip
	if wi.state != "" {
		chips = append(chips, models.GlanceChip{Label: wi.state, Tone: "blue"})
	}
	if wi.wiType != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(wi.wiType)) {
		chips = append(chips, models.GlanceChip{Label: wi.wiType, Tone: "neutral"})
	}

	var children []models.SearchCard
	if wi.url != "" {
		children = append(children, models.SearchCard{
			ID:         "mcp-ado-open-" + models.Slugify(wi.id+title),
			Title:      "Open in Azure DevOps",
			Collection: "mcp-azure-devops",
			Layout:     models.LayoutLink,
			Icon:       "🌐",
			Data:       map[string]any{"url": wi.url},
			Actions:    []models.CardAction{{ID: "open", Label: "Open in Azure DevOps", Shortcut: "Enter", Type: "open_url", Target: wi.url}},
			LocalOnly:  true,
			UpdatedAt:  now,
		})
	}

	card := models.SearchCard{
		ID:              "mcp-ado-item-" + models.Slugify(firstNonEmptyStr(wi.id, query)),
		Title:           title,
		Collection:      "mcp-azure-devops",
		Layout:          models.LayoutFacts,
		Icon:            "🔷",
		Chips:           chips,
		Data:            data,
		Children:        children,
		DefaultExpanded: true,
		LocalOnly:       localOnly || true,
		UpdatedAt:       now,
	}
	if models.ValidateAndNormalizeCard(&card, "mcp-azure-devops") == nil {
		return []models.SearchCard{card}
	}
	return nil
}

type adoWorkItemFields struct {
	id         string
	title      string
	wiType     string
	state      string
	assignedTo string
	areaPath   string
	summary    string
	url        string
}

func extractADOWorkItemFields(item map[string]any) adoWorkItemFields {
	fields, _ := item["fields"].(map[string]any)
	pick := func(keys ...string) string {
		for _, k := range keys {
			if fields != nil {
				if v, ok := fields[k]; ok {
					if s := stringifySimple(v); s != "" {
						return s
					}
				}
			}
			if v, ok := item[k]; ok {
				if s := stringifySimple(v); s != "" {
					return s
				}
			}
		}
		return ""
	}

	assigned := pick("System.AssignedTo", "assignedTo")
	if fields != nil {
		if m, ok := fields["System.AssignedTo"].(map[string]any); ok {
			assigned = firstNonEmptyStr(stringField(m, "displayName"), stringField(m, "uniqueName"), assigned)
		}
	}

	htmlURL := firstValidHTTPURL(
		nestedStringField(item, "_links", "html", "href"),
		stringField(item, "html_url"),
		stringField(item, "url"),
	)

	return adoWorkItemFields{
		id:         pick("id", "System.Id"),
		title:      pick("System.Title", "title"),
		wiType:     pick("System.WorkItemType", "workItemType", "type"),
		state:      pick("System.State", "state"),
		assignedTo: assigned,
		areaPath:   pick("System.AreaPath", "areaPath"),
		summary:    pick("System.Description", "description"),
		url:        htmlURL,
	}
}

func disconnectedServerHintCard(serverID, name, usage, authCmd string) models.SearchCard {
	card := models.SearchCard{
		ID:         "mcp-disconnected-" + serverID,
		Title:      name + " Not Connected",
		Collection: "system",
		Layout:     models.LayoutFacts,
		Icon:       "🔌",
		Chips:      []models.GlanceChip{{Label: "Offline", Tone: "neutral"}},
		Data: map[string]any{
			"summary": fmt.Sprintf("Enable and connect %s in Modules & MCP to run %s queries live.", name, usage),
			"facts": []map[string]any{
				{"label": "CLI Auth Check", "value": authCmd, "mono": true, "copyable": true},
				{"label": "Preset ID", "value": serverID, "mono": true},
			},
		},
		DefaultExpanded: true,
		LocalOnly:       true,
		UpdatedAt:       time.Now().UTC(),
	}
	_ = models.ValidateAndNormalizeCard(&card, "system")
	return card
}

func connectedUsageGuideCard(serverID, title, summary string, tools []string, localOnly bool) models.SearchCard {
	facts := []map[string]any{
		{"label": "Server ID", "value": serverID, "mono": true},
		{"label": "Discovered Tools", "value": strconv.Itoa(len(tools)), "mono": true},
	}
	if len(tools) > 0 {
		preview := tools
		if len(preview) > 5 {
			preview = preview[:5]
		}
		facts = append(facts, map[string]any{"label": "Sample Tools", "value": strings.Join(preview, ", "), "mono": true})
	}
	card := models.SearchCard{
		ID:              "mcp-guide-" + serverID,
		Title:           title,
		Collection:      "mcp-" + serverID,
		Layout:          models.LayoutFacts,
		Icon:            "⚡",
		Chips:           []models.GlanceChip{{Label: "Connected", Tone: "green", Dot: "green"}},
		Data:            map[string]any{"summary": summary, "facts": facts},
		DefaultExpanded: true,
		LocalOnly:       localOnly,
		UpdatedAt:       time.Now().UTC(),
	}
	_ = models.ValidateAndNormalizeCard(&card, "mcp-"+serverID)
	return card
}

func liveQueryErrorCard(serverID, title, detail string, localOnly bool) models.SearchCard {
	card := models.SearchCard{
		ID:              "mcp-error-" + serverID,
		Title:           title,
		Collection:      "mcp-" + serverID,
		Layout:          models.LayoutFacts,
		Icon:            "⚠️",
		Chips:           []models.GlanceChip{{Label: "Live lookup unavailable", Tone: "neutral"}},
		Data:            map[string]any{"summary": strings.TrimSpace(detail)},
		DefaultExpanded: true,
		LocalOnly:       localOnly,
		UpdatedAt:       time.Now().UTC(),
	}
	_ = models.ValidateAndNormalizeCard(&card, "mcp-"+serverID)
	return card
}

func buildSingleLinkCard(prefix string, idx int, item map[string]any, collection, icon string, localOnly bool, now time.Time) (models.SearchCard, bool) {
	htmlURL := firstValidHTTPURL(stringField(item, "html_url"), stringField(item, "url"))
	if htmlURL == "" {
		return models.SearchCard{}, false
	}
	title := firstNonEmptyStr(stringField(item, "title"), stringField(item, "full_name"), stringField(item, "name"))
	if title == "" {
		return models.SearchCard{}, false
	}
	if num := stringField(item, "number"); num != "" {
		title = fmt.Sprintf("#%s · %s", num, title)
	}
	var chips []models.GlanceChip
	if state := stringField(item, "state"); state != "" {
		tone := "green"
		if strings.EqualFold(state, "closed") {
			tone = "red"
		}
		chips = append(chips, models.GlanceChip{Label: strings.ToUpper(state), Tone: tone})
	}
	card := models.SearchCard{
		ID:         fmt.Sprintf("%s-%d-%s", prefix, idx, models.Slugify(title)),
		Title:      title,
		Collection: collection,
		Layout:     models.LayoutLink,
		Icon:       icon,
		Chips:      chips,
		Data:       map[string]any{"url": htmlURL},
		Actions:    []models.CardAction{{ID: "open", Label: "Open in Browser", Shortcut: "Enter", Type: "open_url", Target: htmlURL}},
		LocalOnly:  localOnly,
		UpdatedAt:  now,
	}
	if models.ValidateAndNormalizeCard(&card, collection) != nil {
		return models.SearchCard{}, false
	}
	return card, true
}

func isGitHubRepoItem(item map[string]any) bool {
	if _, hasFullName := item["full_name"]; hasFullName {
		return true
	}
	if _, hasStars := item["stargazers_count"]; hasStars {
		return true
	}
	if _, hasClone := item["clone_url"]; hasClone {
		return true
	}
	return false
}

func extractJSONObjectSlice(parsed any, arrayKeys ...string) []map[string]any {
	if arr, ok := parsed.([]any); ok {
		return toMapSlice(arr)
	}
	if obj, ok := parsed.(map[string]any); ok {
		for _, k := range arrayKeys {
			if arr, ok := obj[k].([]any); ok {
				return toMapSlice(arr)
			}
		}
	}
	return nil
}

func toMapSlice(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, elem := range arr {
		if m, ok := elem.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	return stringifySimple(m[key])
}

func nestedStringField(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = obj[k]
	}
	return stringifySimple(cur)
}

func stringifySimple(v any) string {
	switch tv := v.(type) {
	case string:
		return strings.TrimSpace(tv)
	case float64:
		if tv == float64(int64(tv)) {
			return strconv.FormatInt(int64(tv), 10)
		}
		return strconv.FormatFloat(tv, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(tv)
	default:
		return ""
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func firstValidHTTPURL(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" && models.ValidateHTTPURLStrict(s) == nil {
			return s
		}
	}
	return ""
}
