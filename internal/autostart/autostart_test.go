package autostart

import "testing"

func TestMemoryManager(t *testing.T) {
	m := NewMemoryManager(false)
	if enabled, err := m.IsEnabled(); err != nil || enabled {
		t.Fatalf("expected disabled initially, got enabled=%v err=%v", enabled, err)
	}
	if err := m.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if enabled, err := m.IsEnabled(); err != nil || !enabled {
		t.Fatalf("expected enabled=true, got enabled=%v err=%v", enabled, err)
	}
	if err := m.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if enabled, err := m.IsEnabled(); err != nil || enabled {
		t.Fatalf("expected enabled=false, got enabled=%v err=%v", enabled, err)
	}
}
