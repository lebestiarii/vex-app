//go:build darwin

package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinLaunchAgentPlistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := NewDarwinManager("com.vex.test", "/Applications/Vex.app/Contents/MacOS/vex", dir)

	if enabled, err := m.IsEnabled(); err != nil || enabled {
		t.Fatalf("expected disabled initially, got %v (%v)", enabled, err)
	}
	if err := m.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if enabled, err := m.IsEnabled(); err != nil || !enabled {
		t.Fatalf("expected enabled after SetEnabled(true), got %v (%v)", enabled, err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "com.vex.test.plist"))
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "<string>com.vex.test</string>") || !strings.Contains(text, "/Applications/Vex.app/Contents/MacOS/vex") {
		t.Fatalf("unexpected plist contents:\n%s", text)
	}

	if err := m.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if enabled, err := m.IsEnabled(); err != nil || enabled {
		t.Fatalf("expected disabled after SetEnabled(false), got %v (%v)", enabled, err)
	}
}
