package search

import (
	"reflect"
	"testing"

	"vex/internal/models"
)

func TestParseQueryModifiers(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantMode       QueryMode
		wantModifier   string
		wantCollection string
		wantTags       []string
		wantTemplate   models.CardTemplateType
		wantParams     map[string]string
		wantArgs       []string
		wantText       string
	}{
		{
			name:           "full modifier, collection, tags, template, params and quoted args",
			input:          `/gh @devops #prod #k8s type:command env:staging branch:"release 2.0" deploy`,
			wantMode:       ModeSearch,
			wantModifier:   "gh",
			wantCollection: "devops",
			wantTags:       []string{"prod", "k8s"},
			wantTemplate:   models.TemplateCommand,
			wantParams:     map[string]string{"env": "staging", "branch": "release 2.0"},
			wantArgs:       []string{"deploy"},
			wantText:       "deploy",
		},
		{
			name:           "AI RAG mode with collection and tag scoping",
			input:          `? @knowledge #ops how do I rollback staging?`,
			wantMode:       ModeAI,
			wantCollection: "knowledge",
			wantTags:       []string{"ops"},
			wantArgs:       []string{"how", "do", "I", "rollback", "staging?"},
			wantText:       "how do I rollback staging?",
		},
		{
			name:     "URL is not misparsed as key:value param",
			input:    `https://localhost:3000/api/v1`,
			wantMode: ModeSearch,
			wantArgs: []string{"https://localhost:3000/api/v1"},
			wantText: "https://localhost:3000/api/v1",
		},
		{
			name:     "deduplicates repeated tags",
			input:    `#Dev #dev #ops notes`,
			wantMode: ModeSearch,
			wantTags: []string{"dev", "ops"},
			wantArgs: []string{"notes"},
			wantText: "notes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.input)
			if got.Mode != tc.wantMode {
				t.Errorf("Mode: got %q, want %q", got.Mode, tc.wantMode)
			}
			if got.Modifier != tc.wantModifier {
				t.Errorf("Modifier: got %q, want %q", got.Modifier, tc.wantModifier)
			}
			if got.Collection != tc.wantCollection {
				t.Errorf("Collection: got %q, want %q", got.Collection, tc.wantCollection)
			}
			if !reflect.DeepEqual(got.Tags, tc.wantTags) {
				t.Errorf("Tags: got %v, want %v", got.Tags, tc.wantTags)
			}
			if got.Template != tc.wantTemplate {
				t.Errorf("Template: got %q, want %q", got.Template, tc.wantTemplate)
			}
			wantParams := tc.wantParams
			if wantParams == nil {
				wantParams = map[string]string{}
			}
			if !reflect.DeepEqual(got.Params, wantParams) {
				t.Errorf("Params: got %v, want %v", got.Params, wantParams)
			}
			if !reflect.DeepEqual(got.Args, tc.wantArgs) {
				t.Errorf("Args: got %v, want %v", got.Args, tc.wantArgs)
			}
			if got.Text != tc.wantText {
				t.Errorf("Text: got %q, want %q", got.Text, tc.wantText)
			}
		})
	}
}
