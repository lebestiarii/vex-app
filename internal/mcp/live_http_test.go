package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vex/internal/search"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func makeJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestLiveHTTPClient_GETCachingAndAuthModes(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)

	var httpCalls atomic.Int32
	var ghCalls atomic.Int32
	var azCalls atomic.Int32
	var lastAuthHeader string
	var lastAcceptHeader string
	var lastMethod string

	client := NewLiveHTTPClient(
		WithClock(func() time.Time { return now }),
		WithEnvLookup(func(k string) string {
			if k == "MY_API_TOKEN" {
				return "env-secret-val"
			}
			return ""
		}),
		WithCommandRunner(func(_ context.Context, name string, args ...string) (string, string, error) {
			joined := strings.Join(args, " ")
			switch name {
			case "gh":
				if joined != "auth token" {
					t.Fatalf("unexpected gh args: %q", joined)
				}
				ghCalls.Add(1)
				return "gh-cli-token-123\n", "", nil
			case "az":
				want := "account get-access-token --resource 499b84ac-1321-427f-aa17-267ca6975798 --query accessToken -o tsv"
				if joined != want {
					t.Fatalf("unexpected az args: %q, want %q", joined, want)
				}
				azCalls.Add(1)
				return "az-devops-token-456\n", "", nil
			default:
				return "", "", errors.New("unexpected command: " + name)
			}
		}),
		WithHTTPTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			httpCalls.Add(1)
			lastMethod = r.Method
			lastAuthHeader = r.Header.Get("Authorization")
			lastAcceptHeader = r.Header.Get("Accept")
			return makeJSONResponse(http.StatusOK, `{"state":"open","title":"Fix LiveHTTP"}`), nil
		})),
	)

	// 1. gh_cli auth + response cache hit within TTL
	req := search.LiveHTTPRequest{
		URL:         "https://api.github.com/repos/octo/vex/issues/42",
		Auth:        "gh_cli",
		Headers:     map[string]string{"Accept": "application/vnd.github+json"},
		TimeoutMs:   1500,
		CacheTTLSec: 60,
	}
	body, status, err := client.ExecuteLiveHTTP(ctx, req)
	if err != nil || status != 200 || !strings.Contains(string(body), "Fix LiveHTTP") {
		t.Fatalf("unexpected first liveHttp result: status=%d body=%q err=%v", status, string(body), err)
	}
	if lastMethod != http.MethodGet {
		t.Fatalf("expected GET method, got %q", lastMethod)
	}
	if lastAuthHeader != "Bearer gh-cli-token-123" {
		t.Fatalf("expected gh_cli Bearer token, got %q", lastAuthHeader)
	}
	if lastAcceptHeader != "application/vnd.github+json" {
		t.Fatalf("expected custom Accept header, got %q", lastAcceptHeader)
	}

	// Second call within 60s hits response cache (no additional HTTP or CLI call)
	now = now.Add(30 * time.Second)
	_, _, err = client.ExecuteLiveHTTP(ctx, req)
	if err != nil {
		t.Fatalf("cached ExecuteLiveHTTP failed: %v", err)
	}
	if httpCalls.Load() != 1 || ghCalls.Load() != 1 {
		t.Fatalf("expected 1 HTTP call and 1 gh CLI call (cached), got http=%d gh=%d", httpCalls.Load(), ghCalls.Load())
	}

	// Advance past 60s response TTL (still within 5m token TTL): HTTP refetches, gh CLI remains cached!
	now = now.Add(45 * time.Second)
	_, _, err = client.ExecuteLiveHTTP(ctx, req)
	if err != nil {
		t.Fatalf("post-TTL ExecuteLiveHTTP failed: %v", err)
	}
	if httpCalls.Load() != 2 || ghCalls.Load() != 1 {
		t.Fatalf("expected 2 HTTP calls and 1 cached gh CLI call, got http=%d gh=%d", httpCalls.Load(), ghCalls.Load())
	}

	// 2. az_devops auth invokes exact hardcoded resource args
	_, _, err = client.ExecuteLiveHTTP(ctx, search.LiveHTTPRequest{
		URL:         "https://dev.azure.com/contoso/proj/_apis/wit/workitems/12345?api-version=7.1",
		Auth:        "az_devops",
		TimeoutMs:   1500,
		CacheTTLSec: 60,
	})
	if err != nil {
		t.Fatalf("az_devops liveHttp failed: %v", err)
	}
	if lastAuthHeader != "Bearer az-devops-token-456" || azCalls.Load() != 1 {
		t.Fatalf("unexpected az_devops auth=%q azCalls=%d", lastAuthHeader, azCalls.Load())
	}

	// 3. env:VAR_NAME auth
	_, _, err = client.ExecuteLiveHTTP(ctx, search.LiveHTTPRequest{
		URL:  "https://api.example.com/status/1",
		Auth: "env:MY_API_TOKEN",
	})
	if err != nil || lastAuthHeader != "Bearer env-secret-val" {
		t.Fatalf("expected env auth Bearer header, got auth=%q err=%v", lastAuthHeader, err)
	}

	// Missing env var fails cleanly
	if _, _, err := client.ExecuteLiveHTTP(ctx, search.LiveHTTPRequest{
		URL:  "https://api.example.com/status/2",
		Auth: "env:UNSET_VAR",
	}); err == nil {
		t.Fatal("expected error for unset environment variable")
	}
}

func TestLiveHTTPClient_SecurityGuardsAndTimeout(t *testing.T) {
	ctx := context.Background()
	client := NewLiveHTTPClient(
		WithHTTPTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "slow.example.com" {
				select {
				case <-r.Context().Done():
					return nil, r.Context().Err()
				case <-time.After(2 * time.Second):
					return makeJSONResponse(http.StatusOK, `{}`), nil
				}
			}
			return makeJSONResponse(http.StatusOK, `{}`), nil
		})),
	)

	// Non-http scheme rejected
	if _, _, err := client.ExecuteLiveHTTP(ctx, search.LiveHTTPRequest{URL: "file:///etc/passwd"}); err == nil {
		t.Fatal("expected file:// URL to be rejected")
	}
	// Embedded credentials rejected
	if _, _, err := client.ExecuteLiveHTTP(ctx, search.LiveHTTPRequest{URL: "https://user:pass@api.example.com/x"}); err == nil {
		t.Fatal("expected embedded URL credentials to be rejected")
	}
	// Forbidden auth header rejected
	if _, _, err := client.ExecuteLiveHTTP(ctx, search.LiveHTTPRequest{
		URL:     "https://api.example.com/x",
		Headers: map[string]string{"Authorization": "Bearer leaked"},
	}); err == nil {
		t.Fatal("expected Authorization header in liveHttp.headers to be rejected")
	}
	// Timeout enforced
	if _, _, err := client.ExecuteLiveHTTP(ctx, search.LiveHTTPRequest{
		URL:       "https://slow.example.com/item",
		TimeoutMs: 50,
	}); err == nil {
		t.Fatal("expected timeout error on slow endpoint")
	}
}
