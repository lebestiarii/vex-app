package search

import (
	"strings"
	"unicode"

	"vex/internal/models"
)

// QueryMode indicates the primary operational mode of the omnibar.
type QueryMode string

const (
	ModeSearch QueryMode = "search"
	ModeAI     QueryMode = "ai"
)

// ParsedQuery is the structured representation of a Raycast-style search string.
type ParsedQuery struct {
	Raw        string                  `json:"raw"`
	Mode       QueryMode               `json:"mode"`
	Modifier   string                  `json:"modifier,omitempty"`
	Collection string                  `json:"collection,omitempty"`
	Tags       []string                `json:"tags,omitempty"`
	Template   models.CardTemplateType `json:"template,omitempty"`
	Params     map[string]string       `json:"params,omitempty"`
	Args       []string                `json:"args,omitempty"`
	Text       string                  `json:"text"`
}

// Parse tokenizes a raw omnibar query into modifiers (/cmd), collections (@col),
// tags (#tag), AI RAG mode (?question), key:value params, and free text.
func Parse(raw string) ParsedQuery {
	trimmed := strings.TrimSpace(raw)
	pq := ParsedQuery{
		Raw:    raw,
		Mode:   ModeSearch,
		Params: make(map[string]string),
	}
	if trimmed == "" {
		return pq
	}

	// AI RAG Mode: "? @devops #staging how do I deploy?"
	rest := trimmed
	if strings.HasPrefix(rest, "?") {
		pq.Mode = ModeAI
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "?"))
	}

	tokens := splitTokens(rest)
	seenTags := make(map[string]struct{})
	var textParts []string

	for _, tok := range tokens {
		if tok == "" {
			continue
		}

		// Modifier: /alias (only take the first modifier as active scope; subsequent /paths stay as text)
		if strings.HasPrefix(tok, "/") && len(tok) > 1 && pq.Modifier == "" && !strings.Contains(tok[1:], "/") {
			pq.Modifier = strings.ToLower(strings.TrimPrefix(tok, "/"))
			continue
		}

		// Collection filter: @collection or @mcp:github
		if strings.HasPrefix(tok, "@") && len(tok) > 1 && pq.Collection == "" {
			pq.Collection = strings.ToLower(strings.TrimPrefix(tok, "@"))
			continue
		}

		// Tag filter: #tag
		if strings.HasPrefix(tok, "#") && len(tok) > 1 {
			tag := strings.ToLower(strings.TrimPrefix(tok, "#"))
			if _, ok := seenTags[tag]; !ok {
				seenTags[tag] = struct{}{}
				pq.Tags = append(pq.Tags, tag)
			}
			continue
		}

		// Inline key:value parameter (avoid matching URLs like http:// or https://)
		if idx := strings.IndexByte(tok, ':'); idx > 0 && idx < len(tok)-1 && !strings.Contains(tok, "://") {
			key := strings.ToLower(tok[:idx])
			val := strings.Trim(tok[idx+1:], `"'`)
			if key == "type" || key == "tpl" || key == "template" {
				cand := models.CardTemplateType(strings.ToLower(val))
				if models.ValidTemplates[cand] {
					pq.Template = cand
					continue
				}
			}
			if isValidParamKey(key) {
				pq.Params[key] = val
				continue
			}
		}

		unquoted := stripOuterQuotes(tok)
		textParts = append(textParts, unquoted)
		pq.Args = append(pq.Args, unquoted)
	}

	pq.Text = strings.Join(textParts, " ")
	return pq
}

func isValidParamKey(k string) bool {
	for _, r := range k {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func stripOuterQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// splitTokens splits input on whitespace while keeping quoted segments together (including key:"val with spaces").
func splitTokens(s string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune

	for _, r := range s {
		switch {
		case quote != 0:
			cur.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
			cur.WriteRune(r)
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
