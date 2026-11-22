package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"vex/internal/indexer"
	"vex/internal/models"
	"vex/internal/search"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func makeResponse(status int, contentType, body string) *http.Response {
	h := make(http.Header)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func setupTestStore(t *testing.T) (*indexer.Store, *search.Service) {
	t.Helper()
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cards := []models.SearchCard{
		{
			ID:          "devops-staging-k8s",
			Title:       "Staging Cluster Endpoints",
			Subtitle:    "Kubernetes staging details",
			Description: "Staging API lives at https://api.staging.internal in namespace search-platform on port 17380.",
			Collection:  "devops",
			Template:    models.TemplateKeyValue,
			Tags:        []string{"staging", "k8s"},
			Data:        map[string]any{"api_url": "https://api.staging.internal", "namespace": "search-platform"},
		},
		{
			ID:          "shortcuts-github",
			Title:       "GitHub Dashboard",
			Subtitle:    "Code hosting",
			Description: "Open GitHub repositories and pull requests.",
			Collection:  "shortcuts",
			Template:    models.TemplateAppShortcut,
			Tags:        []string{"dev", "git"},
		},
	}
	for _, c := range cards {
		if err := store.UpsertCard(context.Background(), c, "test.json"); err != nil {
			t.Fatalf("UpsertCard: %v", err)
		}
	}
	return store, search.NewService(store)
}

func TestAskRAG_OllamaStreamingAndCitations(t *testing.T) {
	store, searchSvc := setupTestStore(t)

	var capturedPrompt string
	cfg := Config{
		Provider:        ProviderOllama,
		BaseURL:         "http://mock-ollama.local",
		ChatModel:       "llama3.2",
		EmbeddingModel:  "", // FTS5 only
		MaxContextCards: 5,
	}
	aiSvc := NewService(searchSvc, store, cfg)
	aiSvc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/chat" {
				t.Fatalf("unexpected path %s", req.URL.Path)
			}
			bodyBytes, _ := io.ReadAll(req.Body)
			capturedPrompt = string(bodyBytes)
			ndjson := strings.Join([]string{
				`{"message":{"role":"assistant","content":"The staging API "},"done":false}`,
				`{"message":{"role":"assistant","content":"is at https://api.staging.internal [1]."},"done":true}`,
			}, "\n")
			return makeResponse(200, "application/x-ndjson", ndjson), nil
		}),
	})

	var streamed []string
	resp, err := aiSvc.AskRAG(context.Background(), RAGRequest{
		Query: "? @devops staging api url",
	}, func(chunk string) {
		streamed = append(streamed, chunk)
	})
	if err != nil {
		t.Fatalf("AskRAG: %v", err)
	}
	if len(streamed) != 2 {
		t.Fatalf("expected 2 streamed chunks, got %d (%v)", len(streamed), streamed)
	}
	if !strings.Contains(resp.Answer, "https://api.staging.internal [1]") {
		t.Fatalf("unexpected answer: %q", resp.Answer)
	}
	if len(resp.Sources) != 1 || resp.Sources[0].CardID != "devops-staging-k8s" {
		t.Fatalf("expected staging card as source [1], got %+v", resp.Sources)
	}
	if !strings.Contains(capturedPrompt, "search-platform") {
		t.Fatalf("expected RAG system prompt to include card data, got: %s", capturedPrompt)
	}
}

func TestOpenAI_SyncMissingEmbeddingsAndSSERAG(t *testing.T) {
	store, searchSvc := setupTestStore(t)

	cfg := Config{
		Provider:       ProviderOpenAI,
		BaseURL:        "https://mock-openai.local/v1",
		APIKey:         "sk-test-123",
		ChatModel:      "gpt-4o-mini",
		EmbeddingModel: "text-embedding-3-small",
	}
	aiSvc := NewService(searchSvc, store, cfg)
	aiSvc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Authorization") != "Bearer sk-test-123" {
				t.Fatalf("missing Authorization header")
			}
			switch req.URL.Path {
			case "/v1/embeddings":
				var body map[string]any
				_ = json.NewDecoder(req.Body).Decode(&body)
				input, _ := body["input"].(string)
				vec := []float32{0.1, 0.9, 0.0}
				if strings.Contains(strings.ToLower(input), "staging") || strings.Contains(strings.ToLower(input), "cluster") {
					vec = []float32{0.95, 0.05, 0.0}
				}
				respBytes, _ := json.Marshal(map[string]any{
					"data": []map[string]any{{"embedding": vec}},
				})
				return makeResponse(200, "application/json", string(respBytes)), nil

			case "/v1/chat/completions":
				sse := "data: {\"choices\":[{\"delta\":{\"content\":\"Use \"}}]}\n\n" +
					"data: {\"choices\":[{\"delta\":{\"content\":\"Staging Cluster [1].\"}}]}\n\n" +
					"data: [DONE]\n\n"
				return makeResponse(200, "text/event-stream", sse), nil
			default:
				return makeResponse(404, "text/plain", "not found"), nil
			}
		}),
	})

	searchSvc.SetEmbedder(aiSvc.Embed)

	count, err := aiSvc.SyncMissingEmbeddings(context.Background())
	if err != nil {
		t.Fatalf("SyncMissingEmbeddings: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 cards embedded, got %d", count)
	}
	// Second sync should be a no-op because content hashes match.
	count2, err := aiSvc.SyncMissingEmbeddings(context.Background())
	if err != nil || count2 != 0 {
		t.Fatalf("expected 0 cards needing re-embedding, got count=%d err=%v", count2, err)
	}

	var chunks []string
	resp, err := aiSvc.AskRAG(context.Background(), RAGRequest{
		Query: "cluster details",
	}, func(c string) { chunks = append(chunks, c) })
	if err != nil {
		t.Fatalf("AskRAG OpenAI: %v", err)
	}
	if resp.Answer != "Use Staging Cluster [1]." || len(chunks) != 2 {
		t.Fatalf("unexpected OpenAI SSE output: answer=%q chunks=%v", resp.Answer, chunks)
	}
	if len(resp.Sources) == 0 || resp.Sources[0].CardID != "devops-staging-k8s" {
		t.Fatalf("expected vector+FTS match on staging card, got %+v", resp.Sources)
	}
}

func TestAnthropicAndGeminiProviders(t *testing.T) {
	store, searchSvc := setupTestStore(t)

	// 1. Anthropic chat + expected embedding error
	aiSvc := NewService(searchSvc, store, Config{
		Provider:       ProviderAnthropic,
		BaseURL:        "https://mock-anthropic.local/v1",
		APIKey:         "ant-key",
		ChatModel:      "claude-3-5-sonnet-latest",
		EmbeddingModel: "none",
	})
	aiSvc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("x-api-key") != "ant-key" {
				t.Fatalf("missing x-api-key")
			}
			return makeResponse(200, "application/json", `{"content":[{"type":"text","text":"Anthropic grounded answer [1]."}]}`), nil
		}),
	})

	if _, _, err := aiSvc.Embed(context.Background(), "hello"); err == nil {
		t.Fatalf("expected anthropic Embed to return descriptive error")
	}

	// Clear embeddingModel so AskRAG uses FTS5 + Anthropic chat cleanly
	cfg := aiSvc.GetConfig()
	cfg.EmbeddingModel = ""
	aiSvc.SetConfig(cfg)

	antResp, err := aiSvc.AskRAG(context.Background(), RAGRequest{Query: "github"}, nil)
	if err != nil {
		t.Fatalf("AskRAG Anthropic: %v", err)
	}
	if antResp.Answer != "Anthropic grounded answer [1]." {
		t.Fatalf("unexpected Anthropic answer: %q", antResp.Answer)
	}

	// 2. Gemini chat + embeddings
	aiSvc.SetConfig(Config{
		Provider:       ProviderGemini,
		BaseURL:        "https://mock-gemini.local/v1beta",
		APIKey:         "gem-key",
		ChatModel:      "gemini-2.5-flash",
		EmbeddingModel: "text-embedding-004",
	})
	aiSvc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("x-goog-api-key") != "gem-key" {
				t.Fatalf("missing x-goog-api-key header")
			}
			if strings.Contains(req.URL.Path, ":embedContent") {
				return makeResponse(200, "application/json", `{"embedding":{"values":[0.25,0.75]}}`), nil
			}
			return makeResponse(200, "application/json", `{"candidates":[{"content":{"parts":[{"text":"Gemini grounded answer [1]."}]}}]}`), nil
		}),
	})

	vec, modelName, err := aiSvc.Embed(context.Background(), "hello gemini")
	if err != nil || modelName != "text-embedding-004" || len(vec) != 2 {
		t.Fatalf("Gemini Embed failed: vec=%v model=%q err=%v", vec, modelName, err)
	}
	gemResp, err := aiSvc.AskRAG(context.Background(), RAGRequest{Query: "github"}, nil)
	if err != nil || gemResp.Answer != "Gemini grounded answer [1]." {
		t.Fatalf("AskRAG Gemini failed: resp=%+v err=%v", gemResp, err)
	}
}

func TestGenerateETLMappingWithPreview_CSVAndNestedJSON(t *testing.T) {
	store, searchSvc := setupTestStore(t)

	aiSvc := NewService(searchSvc, store, Config{
		Provider:  ProviderOllama,
		BaseURL:   "http://mock-ollama.local",
		ChatModel: "llama3.2",
	})

	t.Run("CSV with fenced markdown LLM response", func(t *testing.T) {
		aiSvc.SetHTTPClient(&http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				// Simulate a chatty local model wrapping JSON in markdown fences
				llmText := "Sure! Here is the mapping rule:\n```json\n" + `{
					"name": "Bookmarks CSV",
					"sourceFormat": "csv",
					"targetCollection": "Team Bookmarks",
					"defaultTemplate": "shortcut",
					"titleField": "{{SiteName}}",
					"subtitleField": "{{Category}}",
					"tagsField": "Labels",
					"actionType": "open_url",
					"actionLabel": "Open Site",
					"actionTarget": "{{Link}}"
				}` + "\n```"
				payload, _ := json.Marshal(map[string]any{
					"message": map[string]any{"role": "assistant", "content": llmText},
				})
				return makeResponse(200, "application/json", string(payload)), nil
			}),
		})

		rawCSV := []byte("SiteName,Category,Link,Labels\nGrafana,Observability,https://grafana.local,\"ops,metrics\"\nPagerDuty,Incidents,https://pagerduty.local,\"oncall,ops\"\n")
		prev, err := aiSvc.GenerateETLMappingWithPreview(context.Background(), rawCSV, "bookmarks.csv", "Make them clickable browser shortcuts")
		if err != nil {
			t.Fatalf("GenerateETLMappingWithPreview CSV: %v", err)
		}
		if prev.Rule.TargetCollection != "team-bookmarks" {
			t.Fatalf("expected slugified collection team-bookmarks, got %q", prev.Rule.TargetCollection)
		}
		if prev.TotalCards != 2 || len(prev.PreviewCards) != 2 {
			t.Fatalf("expected 2 preview cards, got total=%d preview=%d", prev.TotalCards, len(prev.PreviewCards))
		}
		if prev.PreviewCards[0].Title != "Grafana" || prev.PreviewCards[0].Actions[0].Target != "https://grafana.local" {
			t.Fatalf("unexpected first preview card: %+v", prev.PreviewCards[0])
		}
	})

	t.Run("Nested incompatible JSON auto-detected rootPath", func(t *testing.T) {
		aiSvc.SetHTTPClient(&http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				// Model omits rootPath; normalizeETLRule should inherit sample.DetectedRootPath!
				llmText := `{
					"name": "Servers JSON",
					"targetCollection": "servers",
					"defaultTemplate": "kv-grid",
					"titleField": "hostname",
					"subtitleField": "region",
					"actionType": "copy",
					"actionTarget": "ssh {{user}}@{{ip}}"
				}`
				payload, _ := json.Marshal(map[string]any{
					"message": map[string]any{"role": "assistant", "content": llmText},
				})
				return makeResponse(200, "application/json", string(payload)), nil
			}),
		})

		rawJSON := []byte(`{"meta":{"version":1},"inventory":{"hosts":[{"hostname":"web-01","region":"us-west","user":"ubuntu","ip":"10.0.1.5"}]}}`)
		prev, err := aiSvc.GenerateETLMappingWithPreview(context.Background(), rawJSON, "inventory.json", "")
		if err != nil {
			t.Fatalf("GenerateETLMappingWithPreview JSON: %v", err)
		}
		if prev.Rule.RootPath != "inventory.hosts" {
			t.Fatalf("expected inherited rootPath inventory.hosts, got %q", prev.Rule.RootPath)
		}
		if prev.TotalCards != 1 || prev.PreviewCards[0].Actions[0].Target != "ssh ubuntu@10.0.1.5" {
			t.Fatalf("unexpected nested JSON preview card: %+v", prev.PreviewCards)
		}
	})
}

func TestAskRAGLocalOnlyGuardrail(t *testing.T) {
	store, err := indexer.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	cards := []models.SearchCard{
		{
			ID:          "ams-secret",
			Title:       "AMS Internal Vault Credentials",
			Collection:  "vault",
			Template:    models.TemplateKnowledge,
			Description: "TOP_SECRET_CLOUD_FORBIDDEN",
			LocalOnly:   true,
			UpdatedAt:   time.Now().UTC(),
		},
		{
			ID:          "ams-public",
			Title:       "AMS Public Gateway",
			Collection:  "ports",
			Template:    models.TemplateKnowledge,
			Description: "PUBLIC_GATEWAY_INFO",
			Sections: []models.CardSection{
				{
					ModuleID:   "air-freight",
					ModuleName: "Air Freight",
					Title:      "Air Cargo",
					Notes:      "PUBLIC_AIR_NOTES",
				},
				{
					ModuleID:   "internal-tech",
					ModuleName: "Internal Tech",
					Title:      "Private Subnet",
					Notes:      "PRIVATE_SUBNET_FORBIDDEN",
					LocalOnly:  true,
				},
			},
			UpdatedAt: time.Now().UTC(),
		},
	}
	if err := store.ReplaceCollection(ctx, "mixed", cards, "mixed.json"); err != nil {
		t.Fatalf("ReplaceCollection: %v", err)
	}

	searchSvc := search.NewService(store)

	t.Run("Cloud provider excludes localOnly card and localOnly section", func(t *testing.T) {
		var capturedPrompt string
		aiSvc := NewService(searchSvc, store, Config{
			Provider:  ProviderOpenAI,
			APIKey:    "sk-test",
			ChatModel: "gpt-4o-mini",
		})
		aiSvc.SetHTTPClient(&http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				bodyBytes, _ := io.ReadAll(req.Body)
				capturedPrompt = string(bodyBytes)
				respJSON := `{"choices":[{"message":{"content":"Answer [1]"}}]}`
				return makeResponse(200, "application/json", respJSON), nil
			}),
		})

		resp, err := aiSvc.AskRAG(ctx, RAGRequest{Query: "AMS"}, nil)
		if err != nil {
			t.Fatalf("AskRAG OpenAI: %v", err)
		}
		if len(resp.Sources) != 1 || resp.Sources[0].CardID != "ams-public" {
			t.Fatalf("expected only ams-public in cloud RAG sources, got %+v", resp.Sources)
		}
		if strings.Contains(capturedPrompt, "TOP_SECRET_CLOUD_FORBIDDEN") {
			t.Fatalf("cloud RAG prompt leaked localOnly card content")
		}
		if strings.Contains(capturedPrompt, "PRIVATE_SUBNET_FORBIDDEN") {
			t.Fatalf("cloud RAG prompt leaked localOnly section content")
		}
		if !strings.Contains(capturedPrompt, "PUBLIC_GATEWAY_INFO") || !strings.Contains(capturedPrompt, "PUBLIC_AIR_NOTES") {
			t.Fatalf("cloud RAG prompt missing public card/section content: %s", capturedPrompt)
		}
	})

	t.Run("Ollama retains localOnly card and localOnly section", func(t *testing.T) {
		var capturedPrompt string
		aiSvc := NewService(searchSvc, store, Config{
			Provider:  ProviderOllama,
			ChatModel: "llama3.2",
		})
		aiSvc.SetHTTPClient(&http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				bodyBytes, _ := io.ReadAll(req.Body)
				capturedPrompt = string(bodyBytes)
				respJSON := `{"message":{"role":"assistant","content":"Local answer [1] [2]"}}`
				return makeResponse(200, "application/json", respJSON), nil
			}),
		})

		resp, err := aiSvc.AskRAG(ctx, RAGRequest{Query: "AMS"}, nil)
		if err != nil {
			t.Fatalf("AskRAG Ollama: %v", err)
		}
		if len(resp.Sources) != 2 {
			t.Fatalf("expected 2 sources for local Ollama RAG, got %d", len(resp.Sources))
		}
		if !strings.Contains(capturedPrompt, "TOP_SECRET_CLOUD_FORBIDDEN") || !strings.Contains(capturedPrompt, "PRIVATE_SUBNET_FORBIDDEN") {
			t.Fatalf("Ollama RAG prompt should include localOnly content: %s", capturedPrompt)
		}
	})
}
