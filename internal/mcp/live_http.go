package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"vex/internal/models"
	"vex/internal/search"
)

const (
	azDevOpsResourceID = "499b84ac-1321-427f-aa17-267ca6975798"
	maxLiveHTTPBody    = 1 << 20 // 1 MiB safety cap
	defaultTokenTTL    = 5 * time.Minute
	maxRespCacheSize   = 128
)

var liveHTTPEnvVarRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

var liveHTTPForbiddenHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"api-key":             true,
	"x-auth-token":        true,
	"x-access-token":      true,
	"private-token":       true,
	"x-github-token":      true,
	"x-token":             true,
}

type liveHTTPCacheEntry struct {
	body       []byte
	statusCode int
	expiresAt  time.Time
}

type liveTokenCacheEntry struct {
	token     string
	expiresAt time.Time
}

// LiveHTTPClient executes strictly read-only HTTP GET lookups for declarative PatternRules,
// with in-memory TTL response caching and cached OS CLI / environment variable authentication.
// Tokens are held strictly in memory for short TTLs and never persisted or logged.
type LiveHTTPClient struct {
	transport http.RoundTripper
	cmdRunner CommandRunner
	envLookup func(string) string
	now       func() time.Time
	tokenTTL  time.Duration

	respMu    sync.RWMutex
	respCache map[string]liveHTTPCacheEntry

	tokenMu    sync.Mutex
	tokenCache map[string]liveTokenCacheEntry
}

// LiveHTTPOption customizes a LiveHTTPClient (primarily for sandbox-safe unit tests).
type LiveHTTPOption func(*LiveHTTPClient)

// WithHTTPTransport injects a custom http.RoundTripper (no live network in tests).
func WithHTTPTransport(rt http.RoundTripper) LiveHTTPOption {
	return func(c *LiveHTTPClient) {
		c.transport = rt
	}
}

// WithCommandRunner injects a custom CLI runner for gh_cli / az_devops token acquisition.
func WithCommandRunner(runner CommandRunner) LiveHTTPOption {
	return func(c *LiveHTTPClient) {
		c.cmdRunner = runner
	}
}

// WithEnvLookup injects an environment variable reader for env:VAR_NAME auth.
func WithEnvLookup(fn func(string) string) LiveHTTPOption {
	return func(c *LiveHTTPClient) {
		c.envLookup = fn
	}
}

// WithClock injects a custom clock for deterministic cache TTL tests.
func WithClock(fn func() time.Time) LiveHTTPOption {
	return func(c *LiveHTTPClient) {
		c.now = fn
	}
}

// NewLiveHTTPClient creates a read-only GET LiveHTTPExecutor with in-memory TTL caching.
func NewLiveHTTPClient(opts ...LiveHTTPOption) *LiveHTTPClient {
	c := &LiveHTTPClient{
		transport:  http.DefaultTransport,
		cmdRunner:  defaultCommandRunner,
		envLookup:  os.Getenv,
		now:        time.Now,
		tokenTTL:   defaultTokenTTL,
		respCache:  make(map[string]liveHTTPCacheEntry),
		tokenCache: make(map[string]liveTokenCacheEntry),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// ClearCache removes all cached HTTP responses and cached CLI tokens.
func (c *LiveHTTPClient) ClearCache() {
	c.respMu.Lock()
	c.respCache = make(map[string]liveHTTPCacheEntry)
	c.respMu.Unlock()

	c.tokenMu.Lock()
	c.tokenCache = make(map[string]liveTokenCacheEntry)
	c.tokenMu.Unlock()
}

// ExecuteLiveHTTP satisfies search.LiveHTTPExecutor using strictly HTTP GET,
// bounded timeout, in-memory TTL response caching, and zero inline secrets.
func (c *LiveHTTPClient) ExecuteLiveHTTP(ctx context.Context, req search.LiveHTTPRequest) ([]byte, int, error) {
	if err := models.ValidateHTTPURLStrict(req.URL); err != nil {
		return nil, 0, fmt.Errorf("invalid liveHttp URL: %w", err)
	}

	authMode := strings.TrimSpace(req.Auth)
	if authMode == "" {
		authMode = "none"
	}

	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 1500
	}
	if timeoutMs > 5000 {
		timeoutMs = 5000
	}

	ttlSec := req.CacheTTLSec
	if ttlSec <= 0 {
		ttlSec = 60
	}

	now := c.now()
	cacheKey := authMode + "\x00" + req.URL
	c.respMu.RLock()
	if entry, ok := c.respCache[cacheKey]; ok && now.Before(entry.expiresAt) {
		bodyCopy := append([]byte(nil), entry.body...)
		status := entry.statusCode
		c.respMu.RUnlock()
		return bodyCopy, status, nil
	}
	c.respMu.RUnlock()

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	bearerToken, err := c.resolveAuthToken(reqCtx, authMode)
	if err != nil {
		return nil, 0, err
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, req.URL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build liveHttp request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "Vex-LiveHTTP/1.0")

	for k, v := range req.Headers {
		keyTrim := strings.TrimSpace(k)
		if keyTrim == "" {
			continue
		}
		lowerKey := strings.ToLower(keyTrim)
		if liveHTTPForbiddenHeaders[lowerKey] || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "key") {
			return nil, 0, fmt.Errorf("forbidden auth header %q in liveHttp.headers", keyTrim)
		}
		if strings.ContainsAny(keyTrim+v, "\r\n\x00") {
			return nil, 0, fmt.Errorf("illegal control characters in header %q", keyTrim)
		}
		httpReq.Header.Set(keyTrim, v)
	}

	if bearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	client := &http.Client{
		Transport: c.transport,
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			if err := models.ValidateHTTPURLStrict(next.URL.String()); err != nil {
				return err
			}
			if len(via) > 0 && !strings.EqualFold(next.URL.Host, via[0].URL.Host) {
				next.Header.Del("Authorization")
			}
			return nil
		},
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("liveHttp GET failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLiveHTTPBody))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read liveHttp body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, resp.StatusCode, fmt.Errorf("liveHttp returned HTTP %d", resp.StatusCode)
	}

	c.respMu.Lock()
	if len(c.respCache) >= maxRespCacheSize {
		for k, v := range c.respCache {
			if !now.Before(v.expiresAt) {
				delete(c.respCache, k)
			}
		}
		if len(c.respCache) >= maxRespCacheSize {
			c.respCache = make(map[string]liveHTTPCacheEntry)
		}
	}
	c.respCache[cacheKey] = liveHTTPCacheEntry{
		body:       append([]byte(nil), body...),
		statusCode: resp.StatusCode,
		expiresAt:  c.now().Add(time.Duration(ttlSec) * time.Second),
	}
	c.respMu.Unlock()

	return body, resp.StatusCode, nil
}

func (c *LiveHTTPClient) resolveAuthToken(ctx context.Context, authMode string) (string, error) {
	switch authMode {
	case "", "none":
		return "", nil
	case "gh_cli":
		return c.cachedCLIToken(ctx, "gh_cli", "gh", "auth", "token")
	case "az_devops":
		return c.cachedCLIToken(
			ctx,
			"az_devops",
			"az",
			"account",
			"get-access-token",
			"--resource",
			azDevOpsResourceID,
			"--query",
			"accessToken",
			"-o",
			"tsv",
		)
	default:
		if strings.HasPrefix(authMode, "env:") {
			varName := strings.TrimPrefix(authMode, "env:")
			if !liveHTTPEnvVarRe.MatchString(varName) {
				return "", fmt.Errorf("invalid env auth variable %q", varName)
			}
			val := strings.TrimSpace(c.envLookup(varName))
			if val == "" {
				return "", fmt.Errorf("environment variable %s is not set", varName)
			}
			return val, nil
		}
		return "", fmt.Errorf("unsupported liveHttp auth mode %q", authMode)
	}
}

// cachedCLIToken invokes a hardcoded CLI command with zero user input and caches
// the bearer token strictly in memory for a short TTL.
func (c *LiveHTTPClient) cachedCLIToken(ctx context.Context, cacheKey, bin string, hardcodedArgs ...string) (string, error) {
	now := c.now()
	c.tokenMu.Lock()
	if entry, ok := c.tokenCache[cacheKey]; ok && now.Before(entry.expiresAt) && entry.token != "" {
		tok := entry.token
		c.tokenMu.Unlock()
		return tok, nil
	}
	c.tokenMu.Unlock()

	stdout, _, err := c.cmdRunner(ctx, bin, hardcodedArgs...)
	if err != nil {
		return "", fmt.Errorf("%s token unavailable: %w", cacheKey, err)
	}
	tok := strings.TrimSpace(stdout)
	if tok == "" {
		return "", fmt.Errorf("%s returned empty token", cacheKey)
	}
	if strings.ContainsAny(tok, "\r\n") {
		tok = strings.TrimSpace(strings.Split(tok, "\n")[0])
	}

	c.tokenMu.Lock()
	c.tokenCache[cacheKey] = liveTokenCacheEntry{
		token:     tok,
		expiresAt: c.now().Add(c.tokenTTL),
	}
	c.tokenMu.Unlock()
	return tok, nil
}
