package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCompareSemver(t *testing.T) {
	if CompareSemver("0.1.0", "0.2.0") != -1 {
		t.Fatalf("expected 0.1.0 < 0.2.0")
	}
	if CompareSemver("v0.2.0", "0.10.0") != -1 {
		t.Fatalf("expected v0.2.0 < 0.10.0")
	}
	if CompareSemver("v1.2.3", "1.2.3") != 0 {
		t.Fatalf("expected v1.2.3 == 1.2.3")
	}
	if CompareSemver("1.2.4", "1.2.3") != 1 {
		t.Fatalf("expected 1.2.4 > 1.2.3")
	}
}

func TestCheckerHTTPAndLocalFile(t *testing.T) {
	plat := runtime.GOOS + "-" + runtime.GOARCH
	manifestJSON := `{"version":"0.2.0","releasedAt":"2026-11-25","notes":"New modules & update checker","downloads":{"` + plat + `":{"url":"https://releases.internal/vex-0.2.0","sha256":"abc123"}}}`

	// 1. In-memory HTTP manifest check (no loopback socket needed)
	chk := NewChecker()
	chk.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(manifestJSON)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	res, err := chk.Check(context.Background(), "0.1.0", "https://releases.internal/manifest.json")
	if err != nil {
		t.Fatalf("HTTP Check: %v", err)
	}
	if !res.UpdateAvailable || res.LatestVersion != "0.2.0" || res.DownloadURL != "https://releases.internal/vex-0.2.0" {
		t.Fatalf("unexpected HTTP CheckResult: %+v", res)
	}

	// 2. Local / shared folder manifest check (already up to date)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	resUpToDate, err := chk.Check(context.Background(), "0.2.0", manifestPath)
	if err != nil {
		t.Fatalf("Local file Check: %v", err)
	}
	if resUpToDate.UpdateAvailable {
		t.Fatalf("expected UpdateAvailable=false when already at 0.2.0, got %+v", resUpToDate)
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vex.bin")
	payload := []byte("vex-binary-payload")
	if err := os.WriteFile(p, payload, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sum := sha256.Sum256(payload)
	if err := VerifyFileSHA256(p, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("expected SHA-256 verification to succeed: %v", err)
	}
	if err := VerifyFileSHA256(p, "deadbeef"); err == nil {
		t.Fatalf("expected SHA-256 mismatch error")
	}
}
