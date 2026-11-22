package models

// PatternMCPLookup optionally routes a matched pattern (such as an Azure DevOps
// work item ID or GitHub issue reference) to a connected external MCP server tool.
type PatternMCPLookup struct {
	ServerID  string            `json:"serverId"`            // Connected MCP server ID, e.g. "azure-devops" or "github"
	ToolName  string            `json:"toolName"`            // Tool name on that MCP server, e.g. "wit_get_work_item"
	Arguments map[string]string `json:"arguments,omitempty"` // Map of tool argument -> template using {{match}} or {{captureName}}
}

// PatternRule declaratively defines a reference-number or identifier recognizer
// (e.g., IATA Air Waybill, ISO 6346 container, House/Master Bill, or ADO Work Item).
// Custom rules can be placed in ~/.vex/patterns/*.json or bundled inside ModuleManifest.Patterns.
type PatternRule struct {
	ID                  string                       `json:"id"`
	Name                string                       `json:"name"`
	Category            string                       `json:"category,omitempty"` // e.g. "logistics", "work-items", "engineering"
	Description         string                       `json:"description,omitempty"`
	Regex               string                       `json:"regex"`                 // Go RE2 regex with optional named capture groups (?P<name>...)
	CaseInsensitive     bool                         `json:"caseInsensitive,omitempty"`
	Checksum            string                       `json:"checksum,omitempty"`    // Optional built-in validator: "iata-mod7" | "iso6346"
	Lookups             map[string]map[string]string `json:"lookups,omitempty"`     // captureName -> (rawValue -> decoded label)
	TitleTemplate       string                       `json:"titleTemplate"`         // e.g. "{{lookup.mode}} · {{match}}"
	SubtitleTemplate    string                       `json:"subtitleTemplate,omitempty"`
	DescriptionTemplate string                       `json:"descriptionTemplate,omitempty"`
	Collection          string                       `json:"collection,omitempty"` // Defaults to "patterns"
	Template            CardTemplateType             `json:"template,omitempty"`   // Defaults to "kv-grid"
	Icon                string                       `json:"icon,omitempty"`
	Fields              map[string]string            `json:"fields,omitempty"`         // Structured kv-grid fields resolved from captures/lookups
	GlancePills         []string                     `json:"glancePills,omitempty"`    // Up to 3 summary pill templates
	LinkEntityKeys      []string                     `json:"linkEntityKeys,omitempty"` // Templates resolving to canonical EntityKeys (e.g. "loc:SEA") for cross-card enrichment
	Actions             []CardAction                 `json:"actions,omitempty"`        // Target templates interpolated with captures/lookups
	LiveMCP             *PatternMCPLookup            `json:"liveMcp,omitempty"`        // Optional live MCP tool query when the target server is connected
	LocalOnly           bool                         `json:"localOnly,omitempty"`      // Exclude synthesized card from external cloud LLM RAG
	Provenance          *Provenance                  `json:"provenance,omitempty"`
}
