package models

// PatternMCPLookup optionally routes a matched pattern (such as an Azure DevOps
// work item ID or GitHub issue reference) to a connected external MCP server tool.
type PatternMCPLookup struct {
	ServerID  string            `json:"serverId"`            // Connected MCP server ID, e.g. "azure-devops" or "github"
	ToolName  string            `json:"toolName"`            // Tool name on that MCP server, e.g. "wit_get_work_item"
	Arguments map[string]string `json:"arguments,omitempty"` // Map of tool argument -> template using {{match}} or {{captureName}}
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
