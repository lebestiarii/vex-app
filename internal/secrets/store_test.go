package secrets

import (
	"errors"
	"testing"
)

func TestMemoryStoreLifecycle(t *testing.T) {
	store := NewMemoryStore()

	if _, err := store.Get("vex", "ai_api_key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := store.Set("vex", "ai_api_key", "sk-test-123"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := store.Get("vex", "ai_api_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != "sk-test-123" {
		t.Fatalf("got %q, want %q", got, "sk-test-123")
	}

	// Overwrite
	if err := store.Set("vex", "ai_api_key", "sk-test-456"); err != nil {
		t.Fatalf("Set overwrite failed: %v", err)
	}
	got, err = store.Get("vex", "ai_api_key")
	if err != nil || got != "sk-test-456" {
		t.Fatalf("got %q (%v), want %q", got, err, "sk-test-456")
	}

	// Setting empty string deletes the secret
	if err := store.Set("vex", "ai_api_key", ""); err != nil {
		t.Fatalf("Set empty failed: %v", err)
	}
	if _, err := store.Get("vex", "ai_api_key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after empty Set, got %v", err)
	}

	// Deleting a missing key is a no-op
	if err := store.Delete("vex", "ai_api_key"); err != nil {
		t.Fatalf("Delete missing failed: %v", err)
	}
}
