package indexer

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"vex/internal/models"
)

// GitHubRepoEntry represents a lightweight repository metadata record (no code clone needed).
type GitHubRepoEntry struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name,omitempty"`
	Description   string `json:"description,omitempty"`
	HTMLURL       string `json:"html_url,omitempty"`
	URL           string `json:"url,omitempty"`
	Language      string `json:"language,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// RepoTaxonomy holds the parsed segments of a standard naming convention:
// <prefix>-<portfolio>-<team>-<product>-<component...> (e.g. gt-customs-core-declaration-api).
type RepoTaxonomy struct {
	Prefix    string `json:"prefix"`
	Portfolio string `json:"portfolio"`
	Team      string `json:"team"`
	Product   string `json:"product"`
	Component string `json:"component"`
	Standard  bool   `json:"standard"`
}

// ParseRepoTaxonomy splits a repository name following <prefix>-<portfolio>-<team>-<product>-<component>.
func ParseRepoTaxonomy(repoName string) RepoTaxonomy {
	clean := strings.TrimSpace(repoName)
	if idx := strings.LastIndexByte(clean, '/'); idx >= 0 {
		clean = clean[idx+1:]
	}
	parts := strings.Split(clean, "-")
	if len(parts) >= 5 {
		return RepoTaxonomy{
			Prefix:    strings.ToLower(parts[0]),
			Portfolio: titleWord(parts[1]),
			Team:      titleWord(parts[2]),
			Product:   titleWord(parts[3]),
			Component: strings.Join(parts[4:], "-"),
			Standard:  true,
		}
	}
	if len(parts) == 4 {
		return RepoTaxonomy{
			Prefix:    strings.ToLower(parts[0]),
			Portfolio: titleWord(parts[1]),
			Team:      titleWord(parts[2]),
			Product:   titleWord(parts[3]),
			Component: "core",
			Standard:  true,
		}
	}
	return RepoTaxonomy{
		Prefix:    "repo",
		Portfolio: "General",
		Team:      "Engineering",
		Product:   clean,
		Component: "service",
		Standard:  false,
	}
}

func titleWord(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	switch lower {
	case "gt", "api", "ui", "edi", "wms", "ams", "isf", "fx", "uld", "noc", "sre", "ai", "mcp":
		return strings.ToUpper(lower)
	default:
		return strings.ToUpper(lower[:1]) + lower[1:]
	}
}

// DefaultGitHubCatalogRepos returns a curated sample set of standard gt-<portfolio>-<team>-<product>-<component>
// repositories across Geographies, Air, Ocean, Customs, Accounting, Transcon, and Platform portfolios.
func DefaultGitHubCatalogRepos() []GitHubRepoEntry {
	return []GitHubRepoEntry{
		{
			Name:          "gt-customs-brokerage-declaration-api",
			Description:   "Core customs declaration & ACE / EU ICS2 electronic filing microservice.",
			HTMLURL:       "https://github.corp.internal/gt/gt-customs-brokerage-declaration-api",
			Language:      "Go",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-customs-compliance-ofacscreen-service",
			Description:   "Real-time OFAC SDN, BIS Entity List, and denied-party compliance screening engine.",
			HTMLURL:       "https://github.corp.internal/gt/gt-customs-compliance-ofacscreen-service",
			Language:      "Go",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-customs-tariff-htsclassifier-ui",
			Description:   "Harmonized Tariff Schedule (HS / HTS) classification workbench and duty calculator UI.",
			HTMLURL:       "https://github.corp.internal/gt/gt-customs-tariff-htsclassifier-ui",
			Language:      "TypeScript",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-air-gateway-uldplanner-ui",
			Description:   "3D ULD contour & load planning tool for AKE (LD3), PMC (P6P), AMJ, and B747-8F / B777F main decks.",
			HTMLURL:       "https://github.corp.internal/gt/gt-air-gateway-uldplanner-ui",
			Language:      "TypeScript",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-air-export-easawb-service",
			Description:   "Electronic Air Waybill (e-AWB / Cargo-IMP / Cargo-XML) consolidation and carrier messaging service.",
			HTMLURL:       "https://github.corp.internal/gt/gt-air-export-easawb-service",
			Language:      "Java",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-ocean-manifest-amsportal-service",
			Description:   "Backend service powering the AMS Ocean Shipment Portal, 24-hour CBP manifest filings, and EDI 315 milestones.",
			HTMLURL:       "https://github.corp.internal/gt/gt-ocean-manifest-amsportal-service",
			Language:      "Go",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-ocean-fcl-containeralloc-api",
			Description:   "Ocean FCL/LCL container allocation, 20GP/40GP/40HC/40RF equipment tracking, and carrier booking API.",
			HTMLURL:       "https://github.corp.internal/gt/gt-ocean-fcl-containeralloc-api",
			Language:      "Go",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-geographies-masterdata-locode-indexer",
			Description:   "Canonical UN/LOCODE, IATA airport code, timezone, and branch office master data synchronization pipeline.",
			HTMLURL:       "https://github.corp.internal/gt/gt-geographies-masterdata-locode-indexer",
			Language:      "Go",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-accounting-billing-fxengine-worker",
			Description:   "Multi-currency freight billing, VAT deferment (Art. 23), and daily FX spot rate settlement worker.",
			HTMLURL:       "https://github.corp.internal/gt/gt-accounting-billing-fxengine-worker",
			Language:      "C#",
			DefaultBranch: "main",
		},
		{
			Name:          "gt-transcon-dispatch-rfsrouter-api",
			Description:   "North America 53' FTL/LTL and European Road Feeder Service (RFS) linehaul routing & dispatch API.",
			HTMLURL:       "https://github.corp.internal/gt/gt-transcon-dispatch-rfsrouter-api",
			Language:      "Go",
			DefaultBranch: "main",
		},
	}
}

// BuildGitHubCatalogModule builds the toggleable github-catalog ModuleManifest
// with individual repository cards, portfolio roll-up directory cards, and cross-module overlays.
func BuildGitHubCatalogModule(repos []GitHubRepoEntry) models.ModuleManifest {
	if len(repos) == 0 {
		repos = DefaultGitHubCatalogRepos()
	}

	prov := &models.Provenance{
		SourceName: "GitHub Engineering Catalog (gt-*)",
		SourceURL:  "https://github.corp.internal/gt",
		VerifiedAt: "2026-11-21",
		Authority:  "automated-sync",
	}

	cards := make([]models.SearchCard, 0, len(repos)+8)
	type portfolioSummary struct {
		teams    map[string]struct{}
		products map[string]struct{}
		repos    []string
	}
	portfolios := make(map[string]*portfolioSummary)

	for _, r := range repos {
		tax := ParseRepoTaxonomy(r.Name)
		repoURL := strings.TrimSpace(r.HTMLURL)
		if repoURL == "" {
			repoURL = strings.TrimSpace(r.URL)
		}
		if repoURL == "" {
			repoURL = "https://github.corp.internal/gt/" + r.Name
		}

		pKey := tax.Portfolio
		ps, ok := portfolios[pKey]
		if !ok {
			ps = &portfolioSummary{
				teams:    make(map[string]struct{}),
				products: make(map[string]struct{}),
			}
			portfolios[pKey] = ps
		}
		ps.teams[tax.Team] = struct{}{}
		ps.products[tax.Product] = struct{}{}
		ps.repos = append(ps.repos, r.Name)

		tags := []string{
			"github", "repo", "gt",
			strings.ToLower(tax.Portfolio),
			strings.ToLower(tax.Team),
			strings.ToLower(tax.Product),
			strings.ToLower(tax.Component),
		}
		if r.Language != "" {
			tags = append(tags, strings.ToLower(r.Language))
		}

		cards = append(cards, models.SearchCard{
			ID:          "repo-" + models.Slugify(r.Name),
			EntityKey:   "repo:" + r.Name,
			ModuleID:    "github-catalog",
			Title:       r.Name,
			Subtitle:    fmt.Sprintf("Portfolio: %s • Team: %s • Product: %s (%s)", tax.Portfolio, tax.Team, tax.Product, tax.Component),
			Description: r.Description,
			Collection:  "github-catalog",
			Template:    models.TemplateKeyValue,
			Icon:        "terminal",
			Tags:        tags,
			Modifiers:   []string{"repo", "gh", "gt"},
			GlancePills: []string{"Portfolio: " + tax.Portfolio, "Team: " + tax.Team, tax.Product},
			RelatedKeys: []string{"portfolio:" + strings.ToLower(tax.Portfolio), "team:" + strings.ToLower(tax.Team)},
			Provenance:  prov,
			Data: map[string]any{
				"repository":     r.Name,
				"portfolio":      tax.Portfolio,
				"team":           tax.Team,
				"product":        tax.Product,
				"component":      tax.Component,
				"language":       r.Language,
				"default_branch": nonEmpty(r.DefaultBranch, "main"),
				"url":            repoURL,
			},
			Actions: []models.CardAction{
				{ID: "open-repo", Label: "Open Repository", Shortcut: "Enter", Type: "open_url", Target: repoURL},
				{ID: "copy-name", Label: "Copy Repo Name", Type: "copy", Target: r.Name},
				{ID: "copy-clone", Label: "Copy git clone", Type: "copy", Target: "git clone " + repoURL + ".git"},
			},
		})
	}

	// Build Portfolio Roll-Up Directory Cards so searching a portfolio lists its teams, products, and repos
	pNames := make([]string, 0, len(portfolios))
	for k := range portfolios {
		pNames = append(pNames, k)
	}
	sort.Strings(pNames)

	for _, pName := range pNames {
		ps := portfolios[pName]
		teamsList := sortedKeys(ps.teams)
		productsList := sortedKeys(ps.products)
		sort.Strings(ps.repos)

		cards = append(cards, models.SearchCard{
			ID:          "portfolio-dir-" + models.Slugify(pName),
			EntityKey:   "portfolio:" + strings.ToLower(pName),
			ModuleID:    "github-catalog",
			Title:       fmt.Sprintf("Portfolio Directory: %s (%d Repos)", pName, len(ps.repos)),
			Subtitle:    fmt.Sprintf("Teams: %s • Products: %s", strings.Join(teamsList, ", "), strings.Join(productsList, ", ")),
			Description: fmt.Sprintf("Engineering portfolio summary for %s derived from standard gt-<portfolio>-<team>-<product>-<component> repositories.", pName),
			Collection:  "github-catalog",
			Template:    models.TemplateKeyValue,
			Icon:        "book-open",
			Tags:        append([]string{"portfolio", "directory", "teams", "products", "github", strings.ToLower(pName)}, strings.ToLower(pName)),
			Modifiers:   []string{"portfolio", "team", "gt"},
			GlancePills: []string{fmt.Sprintf("%d Teams", len(teamsList)), fmt.Sprintf("%d Products", len(productsList)), fmt.Sprintf("%d Repos", len(ps.repos))},
			RelatedKeys: []string{"portfolio:" + strings.ToLower(pName)},
			Provenance:  prov,
			Data: map[string]any{
				"portfolio":    pName,
				"teams":        strings.Join(teamsList, ", "),
				"products":     strings.Join(productsList, ", "),
				"repositories": strings.Join(ps.repos, ", "),
			},
			Actions: []models.CardAction{
				{ID: "copy-repos", Label: "Copy Repo List", Shortcut: "Enter", Type: "copy", Target: strings.Join(ps.repos, "\n")},
			},
		})
	}

	// Add an overlay enriching the AMS Ocean Shipment Portal card with its backing microservice repo
	overlays := []models.CardOverlay{
		{
			TargetCardID:  "portal-ams-ocean",
			FallbackTitle: "AMS Ocean Shipment Portal — Engineering Repo",
			FallbackCol:   "github-catalog",
			AddTags:       []string{"gt-ocean-manifest-amsportal-service"},
			Section: models.CardSection{
				Title:      "GitHub Engineering — Backing Repository",
				Badge:      "GitHub Catalog",
				Notes:      "Standard gt-<portfolio>-<team>-<product>-<component> service powering this portal.",
				Provenance: prov,
				Fields: map[string]any{
					"repository": "gt-ocean-manifest-amsportal-service",
					"portfolio":  "Ocean",
					"team":       "Manifest",
					"product":    "Amsportal",
				},
				Actions: []models.CardAction{
					{
						ID:     "open-ams-repo",
						Label:  "Open Repo in GitHub",
						Type:   "open_url",
						Target: "https://github.corp.internal/gt/gt-ocean-manifest-amsportal-service",
					},
				},
			},
		},
	}

	return models.ModuleManifest{
		ID:             "github-catalog",
		Name:           "GitHub Engineering Catalog (Portfolios, Teams & Repos)",
		Category:       "Technology",
		Description:    "Zero-bloat index of gt-<portfolio>-<team>-<product>-<component> repositories with automatic Portfolio, Team, and Product roll-up directories. Off by default.",
		Icon:           "terminal",
		Version:        "1.0.0",
		DefaultEnabled: false,
		Cards:          cards,
		Overlays:       overlays,
	}
}

// ParseGitHubRepoListJSON parses a lightweight JSON array of repositories (from GitHub REST API or `gh repo list --json`).
func ParseGitHubRepoListJSON(raw []byte) ([]GitHubRepoEntry, error) {
	var rawList []map[string]any
	if err := json.Unmarshal(raw, &rawList); err != nil {
		return nil, fmt.Errorf("expected JSON array of repositories: %w", err)
	}
	if len(rawList) == 0 {
		return nil, errors.New("repository list is empty")
	}

	entries := make([]GitHubRepoEntry, 0, len(rawList))
	for _, item := range rawList {
		name := asString(item["name"])
		if name == "" {
			name = asString(item["full_name"])
		}
		if name == "" {
			continue
		}
		lang := asString(item["language"])
		if lang == "" {
			if pl, ok := item["primaryLanguage"].(map[string]any); ok {
				lang = asString(pl["name"])
			}
		}
		entries = append(entries, GitHubRepoEntry{
			Name:          name,
			FullName:      asString(item["full_name"]),
			Description:   asString(item["description"]),
			HTMLURL:       asString(item["html_url"]),
			URL:           asString(item["url"]),
			Language:      lang,
			DefaultBranch: asString(item["default_branch"]),
			UpdatedAt:     asString(item["updated_at"]),
		})
	}
	if len(entries) == 0 {
		return nil, errors.New("no valid repository entries found in JSON")
	}
	return entries, nil
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func nonEmpty(val, fallback string) string {
	if strings.TrimSpace(val) != "" {
		return val
	}
	return fallback
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
