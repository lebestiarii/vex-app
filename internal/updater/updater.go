package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DownloadAsset describes a platform-specific release artifact and its SHA-256 digest.
type DownloadAsset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
}

// UpdateManifest is the lightweight JSON file hosted at any HTTP/HTTPS URL or shared folder path
// (no user login required).
type UpdateManifest struct {
	Version    string                   `json:"version"`
	ReleasedAt string                   `json:"releasedAt,omitempty"`
	Notes      string                   `json:"notes,omitempty"`
	Downloads  map[string]DownloadAsset `json:"downloads,omitempty"` // e.g. "darwin-arm64", "windows-amd64"
}

// CheckResult is returned to the UI after comparing the running version against UpdateManifest.
type CheckResult struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleasedAt      string `json:"releasedAt,omitempty"`
	Notes           string `json:"notes,omitempty"`
	PlatformKey     string `json:"platformKey"`
	DownloadURL     string `json:"downloadUrl,omitempty"`
	DownloadSHA256  string `json:"downloadSha256,omitempty"`
	Source          string `json:"source"`
}

// Checker fetches UpdateManifest from an HTTP/HTTPS URL or local/shared folder path without login.
type Checker struct {
	HTTPClient *http.Client
	Timeout    time.Duration
}

// NewChecker returns a Checker with a 3-second default timeout so offline checks fail fast.
func NewChecker() *Checker {
	return &Checker{
		HTTPClient: &http.Client{Timeout: 3 * time.Second},
		Timeout:    3 * time.Second,
	}
}

// Check compares currentVersion against the manifest at feedLocation (HTTP/HTTPS URL or local/UNC file path).
func (c *Checker) Check(ctx context.Context, currentVersion, feedLocation string) (*CheckResult, error) {
	loc := strings.TrimSpace(feedLocation)
	if loc == "" {
		return nil, errors.New("update manifest URL or file path is not configured")
	}

	raw, err := c.readManifestBytes(ctx, loc)
	if err != nil {
		return nil, err
	}

	var manifest UpdateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("invalid update manifest JSON: %w", err)
	}
	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Version == "" {
		return nil, errors.New("update manifest is missing a 'version' field")
	}

	platformKey := runtime.GOOS + "-" + runtime.GOARCH
	res := &CheckResult{
		CurrentVersion:  normalizeVersion(currentVersion),
		LatestVersion:   normalizeVersion(manifest.Version),
		UpdateAvailable: CompareSemver(currentVersion, manifest.Version) < 0,
		ReleasedAt:      manifest.ReleasedAt,
		Notes:           manifest.Notes,
		PlatformKey:     platformKey,
		Source:          loc,
	}
	if asset, ok := manifest.Downloads[platformKey]; ok {
		res.DownloadURL = strings.TrimSpace(asset.URL)
		res.DownloadSHA256 = strings.TrimSpace(asset.SHA256)
	}
	return res, nil
}

func (c *Checker) readManifestBytes(ctx context.Context, loc string) ([]byte, error) {
	lower := strings.ToLower(loc)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if _, err := url.Parse(loc); err != nil {
			return nil, fmt.Errorf("invalid update URL: %w", err)
		}
		timeout := c.Timeout
		if timeout <= 0 {
			timeout = 3 * time.Second
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, loc, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")

		client := c.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: timeout}
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch update manifest: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("update manifest returned HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	}

	// Local or shared network folder file path (e.g. /Volumes/Shared/vex/manifest.json or \\server\share\manifest.json)
	raw, err := os.ReadFile(loc)
	if err != nil {
		return nil, fmt.Errorf("read local update manifest: %w", err)
	}
	return raw, nil
}

// CompareSemver compares two semantic versions ("v0.1.0" vs "0.2.0").
// Returns -1 if a < b, 0 if a == b, and +1 if a > b.
func CompareSemver(a, b string) int {
	pa := parseSemverParts(a)
	pb := parseSemverParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V"))
	if v == "" {
		return "0.0.0"
	}
	return v
}

func parseSemverParts(v string) [3]int {
	v = normalizeVersion(v)
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		if n, err := strconv.Atoi(strings.TrimSpace(parts[i])); err == nil {
			out[i] = n
		}
	}
	return out
}

// VerifyFileSHA256 computes the SHA-256 checksum of filePath and compares it against expectedHex.
func VerifyFileSHA256(filePath, expectedHex string) error {
	expected := strings.ToLower(strings.TrimSpace(expectedHex))
	if expected == "" {
		return errors.New("expected SHA-256 digest is empty")
	}
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("SHA-256 mismatch: got %s, want %s", actual, expected)
	}
	return nil
}
