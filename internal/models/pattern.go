package models

// PatternMCPLookup optionally routes a matched pattern (such as an Azure DevOps
// work item ID or GitHub issue reference) to a connected external MCP server tool.
type PatternMCPLookup struct {
	ServerID  string            `json:"serverId"`            // Connected MCP server ID, e.g. "azure-devops" or "github"
	ToolName  string            `json:"toolName"`            // Tool name on that MCP server, e.g. "wit_get_work_item"
	Arguments map[string]string `json:"arguments,omitempty"` // Map of tool argument -> template using {{match}} or {{captureName}}
}

// PatternLiveHTTP declaratively configures an optional, strictly read-only HTTP GET
// lookup that enriches a matched PatternRule with a nested child card.
type PatternLiveHTTP struct {
	URLTemplate string                `json:"urlTemplate"`           // Supports {{0}}/{{match}} and named captures, safely URL-escaped
	Trigger     string                `json:"trigger,omitempty"`     // "auto" | "manual" (default "manual")
	TimeoutMs   int                   `json:"timeoutMs,omitempty"`   // Default 1500, max 5000
	CacheTTLSec int                   `json:"cacheTtlSec,omitempty"` // Default 60
	Auth        string                `json:"auth,omitempty"`        // "none" | "gh_cli" | "az_devops" | "env:VAR_NAME" (zero inline secrets)
	Headers     map[string]string     `json:"headers,omitempty"`     // Optional non-auth headers (e.g. Accept)
	ResultCard  PatternLiveResultSpec `json:"resultCard"`            // Projects {{json.<dot.path>}} into a nested facts/table/steps/link child card
}

// EffectiveTrigger returns the normalized trigger mode ("auto" or "manual", defaulting to "manual").
func (h PatternLiveHTTP) EffectiveTrigger() string {
	if h.Trigger == "auto" {
		return "auto"
	}
	return "manual"
}

// EffectiveTimeoutMs returns the bounded request timeout in milliseconds (default 1500, max 5000).
func (h PatternLiveHTTP) EffectiveTimeoutMs() int {
	if h.TimeoutMs <= 0 {
		return 1500
	}
	if h.TimeoutMs > 5000 {
		return 5000
	}
	return h.TimeoutMs
}

// EffectiveCacheTTLSec returns the response cache TTL in seconds (default 60).
func (h PatternLiveHTTP) EffectiveCacheTTLSec() int {
	if h.CacheTTLSec <= 0 {
		return 60
	}
	return h.CacheTTLSec
}

// EffectiveAuth returns the normalized auth mode ("none" when omitted).
func (h PatternLiveHTTP) EffectiveAuth() string {
	if h.Auth == "" {
		return "none"
	}
	return h.Auth
}

// PatternLiveResultSpec declaratively projects JSON response fields (via {{json.<dot.path>}})
// into a nested facts, table, steps, or link child card under the decoded pattern card.
type PatternLiveResultSpec struct {
	Layout          CardLayoutType `json:"layout"`                    // "facts" | "table" | "steps" | "link"
	Title           string         `json:"title,omitempty"`           // Optional child card title template
	Icon            string         `json:"icon,omitempty"`            // Optional child card icon
	DefaultExpanded bool           `json:"defaultExpanded,omitempty"` // Whether hydrated child starts expanded
	Chips           []GlanceChip   `json:"chips,omitempty"`           // Optional glance chips (max 4, supports {{json.*}})

	// Facts layout projection
	Summary string     `json:"summary,omitempty"`
	Facts   []FactItem `json:"facts,omitempty"`

	// Link layout projection
	URL string `json:"url,omitempty"`

	// Table layout projection
	Columns     []string   `json:"columns,omitempty"`     // 2..4 column headers
	RowsPath    string     `json:"rowsPath,omitempty"`    // Optional JSON array dot-path (e.g. "items")
	RowFields   []string   `json:"rowFields,omitempty"`   // Per-column dot-path/template for each element in RowsPath
	Rows        [][]string `json:"rows,omitempty"`        // Static/template rows when RowsPath is omitted
	MonoColumns []int      `json:"monoColumns,omitempty"`
	SourceURL   string     `json:"sourceUrl,omitempty"`

	// Steps layout projection
	StepsPath      string     `json:"stepsPath,omitempty"`      // Optional JSON array dot-path for steps
	StepLabel      string     `json:"stepLabel,omitempty"`      // Field/template for each step label
	StepCaption    string     `json:"stepCaption,omitempty"`    // Field/template for each step caption
	Steps          []StepItem `json:"steps,omitempty"`          // Static/template steps when StepsPath is omitted
	ActiveStep     int        `json:"activeStep,omitempty"`
	ActiveStepPath string     `json:"activeStepPath,omitempty"` // Optional JSON path/template resolving to 1-based step index
	Bullets        []string   `json:"bullets,omitempty"`
}

// PatternDataLookup enriches pattern captures by looking up an indexed card in SQLite
// (for example, resolving AWB prefix "176" via entityKey "air-carrier:176" or SCAC "MAEU"
// via "ocean-carrier:MAEU") so updating a carrier module automatically updates pattern decoding.
type PatternDataLookup struct {
	Alias           string            `json:"alias"`                    // Capture namespace prefix, e.g. "carrier" -> {{carrier.name}}
	TargetEntityKey string            `json:"targetEntityKey"`          // Template e.g. "air-carrier:{{prefix}}"
	ExtractFields   map[string]string `json:"extractFields,omitempty"`  // outputKey -> source field ("title", "subtitle", "fact.<Label>", "meta.<k>", "data.<k>")
	FallbackValues  map[string]string `json:"fallbackValues,omitempty"` // outputKey -> fallback if target card not indexed
}

// PatternLinkRule declaratively controls how a matched pattern links and nests enriched cards
// from the local knowledge index (e.g., nesting a Gateway card with its EDI Servers & Customs Links,
// or nesting a Dictionary Guide card).
type PatternLinkRule struct {
	TargetEntityKey  string   `json:"targetEntityKey,omitempty"`  // Template e.g. "loc:{{lookup.branch}}" or "guide:hbl"
	TargetCollection string   `json:"targetCollection,omitempty"` // Optional collection filter when linking by tag/query
	TargetTag        string   `json:"targetTag,omitempty"`        // Optional tag template to match linked cards
	RelationLabel    string   `json:"relationLabel,omitempty"`    // Optional human label describing the relationship
	AutoExpand       bool     `json:"autoExpand,omitempty"`       // Whether the linked child card starts expanded in Rolodex UI
	NestChildren     bool     `json:"nestChildren,omitempty"`     // Whether to recursively include the linked card's own children
	CopyChildrenOnly bool     `json:"copyChildrenOnly,omitempty"` // Attach only the linked card's children (e.g., Airline Tracking Links)
	AttachUnder      string   `json:"attachUnder,omitempty"`      // Optional entityKey template of a previously linked child card to nest inside (falls back to top-level child)
	MaxDepth         int      `json:"maxDepth,omitempty"`         // Max nesting depth (default 2)
	IncludeFields    []string `json:"includeFields,omitempty"`    // Optional subset of fact labels or data fields to include in section summary
}

// PatternSegmentSpec declaratively defines one tile in the segment breakdown grid
// rendered by the "segments" layout (e.g. MODE / BRANCH / SEQUENCE).
type PatternSegmentSpec struct {
	Value   string `json:"value"`          // Template e.g. "{{mode}}"
	Role    string `json:"role"`           // Uppercase micro label e.g. "MODE"
	Meaning string `json:"meaning"`        // Decoded label template e.g. "{{lookup.mode}}"
	Tone    string `json:"tone,omitempty"` // "blue" | "amber" | "green" | "neutral"
}

// PatternRule declaratively defines a reference-number or identifier recognizer
// (e.g., House Bill of Lading, IATA Air Waybill, ISO 6346 container, SCAC, or Work Item).
// All pattern rules are loadable and configurable via ~/.vex/patterns/*.json or optional modules.
type PatternRule struct {
	ID                  string                       `json:"id"`
	Name                string                       `json:"name"`
	Category            string                       `json:"category,omitempty"`  // e.g. "logistics", "air", "ocean", "engineering"
	Description         string                       `json:"description,omitempty"`
	Example             string                       `json:"example,omitempty"`   // Optional sample input shown in Patterns UI
	Enabled             *bool                        `json:"enabled,omitempty"`   // Defaults to true when omitted
	Priority            int                          `json:"priority,omitempty"`  // Higher priority rules evaluate first
	Normalize           []string                     `json:"normalize,omitempty"` // Pre-match transforms: "trim", "uppercase", "lowercase", "strip-spaces", "strip-dashes"
	Regex               string                       `json:"regex"`               // Go RE2 regex with optional named capture groups (?P<name>...)
	CaseInsensitive     bool                         `json:"caseInsensitive,omitempty"`
	Checksum            string                       `json:"checksum,omitempty"` // Optional validator: "iata-mod7" | "iso6346" | "luhn"
	Lookups             map[string]map[string]string `json:"lookups,omitempty"`  // captureName -> (rawValue -> decoded label)
	DataLookups         []PatternDataLookup          `json:"dataLookups,omitempty"`
	TitleTemplate       string                       `json:"titleTemplate"` // e.g. "House Bill · {{match}}"
	SubtitleTemplate    string                       `json:"subtitleTemplate,omitempty"`
	DescriptionTemplate string                       `json:"descriptionTemplate,omitempty"`
	ExplanationTemplate string                       `json:"explanationTemplate,omitempty"` // 1-2 sentence plain-English explanation below segment tiles
	Segments            []PatternSegmentSpec         `json:"segments,omitempty"`            // Breakdown tiles rendered by layout "segments"
	Collection          string                       `json:"collection,omitempty"`          // Defaults to "patterns"
	Layout              CardLayoutType               `json:"layout,omitempty"`              // Defaults to "segments"
	Icon                string                       `json:"icon,omitempty"`
	DefaultExpanded     bool                         `json:"defaultExpanded,omitempty"`
	Fields              map[string]string            `json:"fields,omitempty"`         // Optional structured fact templates for non-segments layouts
	Chips               []GlanceChip                 `json:"chips,omitempty"`          // Typed glance chips (label supports {{templates}})
	LinkEntityKeys      []string                     `json:"linkEntityKeys,omitempty"` // Shorthand templates resolving to EntityKeys for cross-card enrichment
	LinkRules           []PatternLinkRule            `json:"linkRules,omitempty"`      // Rich declarative cross-card linking & nesting rules
	Actions             []CardAction                 `json:"actions,omitempty"`        // Target templates interpolated with captures/lookups
	LiveHTTP            *PatternLiveHTTP             `json:"liveHttp,omitempty"`       // Optional read-only HTTP GET enrichment
	LiveMCP             *PatternMCPLookup            `json:"liveMcp,omitempty"`        // Optional live MCP tool query when target server is connected
	LocalOnly           bool                         `json:"localOnly,omitempty"`      // Marks synthesized card as localOnly
	Provenance          *Provenance                  `json:"provenance,omitempty"`
}

// IsEnabled reports whether the pattern rule is active (defaults to true when Enabled is nil).
func (r PatternRule) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}
