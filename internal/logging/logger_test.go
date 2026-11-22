package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestStructuredJSONLLoggingAndRotation(t *testing.T) {
	logDir := t.TempDir()
	// Tiny 350-byte max size to force rotation deterministically
	mgr, err := NewManager(logDir, "info", 350, 2)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer mgr.Close()

	// Debug messages should be suppressed at "info" level
	mgr.Component("search").Debug("suppressed_debug", "q", "AMS")
	entries, err := mgr.ReadRecentEntries(10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("expected 0 entries at info level for debug log, got %d (err=%v)", len(entries), err)
	}

	// Switch level dynamically to "debug"
	if lvl := mgr.SetLevel("debug"); lvl != "debug" {
		t.Fatalf("expected debug level, got %q", lvl)
	}
	for i := 0; i < 6; i++ {
		mgr.Component("search").Info("query_completed", "index", i, "duration_ms", 3)
	}

	// Verify rotated backup .1 exists
	if _, err := os.Stat(mgr.FilePath() + ".1"); err != nil {
		t.Fatalf("expected rotated log file %s.1 to exist: %v", mgr.FilePath(), err)
	}

	recent, err := mgr.ReadRecentEntries(5)
	if err != nil || len(recent) == 0 {
		t.Fatalf("expected recent entries, got %d (err=%v)", len(recent), err)
	}
	if recent[0].Service != "vex" || recent[0].Component != "search" || recent[0].Msg != "query_completed" {
		t.Fatalf("unexpected parsed log entry: %+v", recent[0])
	}

	// Clear logs
	if err := mgr.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	afterClear, err := mgr.ReadRecentEntries(5)
	if err != nil || len(afterClear) != 0 {
		t.Fatalf("expected 0 entries after Clear, got %d (err=%v)", len(afterClear), err)
	}
}

func TestPrometheusMetricsOutput(t *testing.T) {
	m := &Metrics{}
	m.SearchesTotal.Add(12)
	m.ActionsTotal.Add(4)
	m.ActiveCards.Store(42)
	m.EnabledModules.Store(5)

	var buf bytes.Buffer
	m.WritePrometheus(&buf)
	out := buf.String()

	for _, want := range []string{
		"vex_searches_total 12",
		"vex_actions_total 4",
		"vex_active_cards 42",
		"vex_enabled_modules 5",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected Prometheus output to contain %q, got:\n%s", want, out)
		}
	}
}
