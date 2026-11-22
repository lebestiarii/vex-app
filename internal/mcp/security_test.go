package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoopbackGuardBlocksCrossOriginAndBadHost(t *testing.T) {
	hub, _, _, _, _ := setupHubForTest(t)
	handler := hub.HTTPHandler()

	// 1. Valid loopback request to /health succeeds
	reqOK := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17380/health", nil)
	reqOK.Host = "127.0.0.1:17380"
	recOK := httptest.NewRecorder()
	handler.ServeHTTP(recOK, reqOK)
	if recOK.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for loopback request, got %d", recOK.Code)
	}

	// 2. Non-loopback Host header (DNS rebinding) is rejected with 403
	reqBadHost := httptest.NewRequest(http.MethodGet, "http://evil.example.com/health", nil)
	reqBadHost.Host = "evil.example.com"
	recBadHost := httptest.NewRecorder()
	handler.ServeHTTP(recBadHost, reqBadHost)
	if recBadHost.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-loopback Host, got %d", recBadHost.Code)
	}

	// 3. Cross-origin browser request (CSRF) is rejected with 403
	reqCSRF := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17380/mcp", nil)
	reqCSRF.Host = "127.0.0.1:17380"
	reqCSRF.Header.Set("Origin", "https://attacker.example.com")
	recCSRF := httptest.NewRecorder()
	handler.ServeHTTP(recCSRF, reqCSRF)
	if recCSRF.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-origin Origin header, got %d", recCSRF.Code)
	}

	// 4. Loopback /metrics endpoint returns 200 OK in Prometheus text format
	reqMetrics := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17380/metrics", nil)
	reqMetrics.Host = "127.0.0.1:17380"
	recMetrics := httptest.NewRecorder()
	handler.ServeHTTP(recMetrics, reqMetrics)
	if recMetrics.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for loopback /metrics, got %d", recMetrics.Code)
	}
}
