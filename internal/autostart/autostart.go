package autostart

import (
	"os"
	"strings"
	"sync"
)

// Manager controls whether Vex launches automatically when the user logs in.
type Manager interface {
	IsEnabled() (bool, error)
	SetEnabled(enabled bool) error
}

// MemoryManager is a thread-safe in-memory Manager for sandbox-safe unit tests.
type MemoryManager struct {
	mu      sync.RWMutex
	enabled bool
}

// NewMemoryManager constructs a MemoryManager with an initial enabled state.
func NewMemoryManager(initial bool) *MemoryManager {
	return &MemoryManager{enabled: initial}
}

func (m *MemoryManager) IsEnabled() (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled, nil
}

func (m *MemoryManager) SetEnabled(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
	return nil
}

func currentExecutablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(p), nil
}

func sanitizeIdentifier(appName string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(appName)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "vex"
	}
	return b.String()
}
