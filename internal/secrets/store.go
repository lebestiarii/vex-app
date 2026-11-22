package secrets

import (
	"errors"
	"strings"
	"sync"
)

// ErrNotFound is returned when a requested secret does not exist in the store.
var ErrNotFound = errors.New("secret not found")

// Store abstracts OS-native secret persistence (macOS Keychain, Windows Credential Manager)
// and in-memory test doubles.
type Store interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

// MemoryStore is a thread-safe in-memory Store used for sandbox-safe unit tests
// and platforms without a native keyring backend.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]string
}

// NewMemoryStore constructs an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]string),
	}
}

func memoryKey(service, account string) string {
	return strings.TrimSpace(service) + "\x00" + strings.TrimSpace(account)
}

// Get retrieves a secret from memory.
func (m *MemoryStore) Get(service, account string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[memoryKey(service, account)]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set stores or updates a secret in memory. Passing an empty secret deletes it.
func (m *MemoryStore) Set(service, account, secret string) error {
	if strings.TrimSpace(service) == "" || strings.TrimSpace(account) == "" {
		return errors.New("service and account are required")
	}
	if secret == "" {
		return m.Delete(service, account)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[memoryKey(service, account)] = secret
	return nil
}

// Delete removes a secret from memory; missing entries are ignored.
func (m *MemoryStore) Delete(service, account string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, memoryKey(service, account))
	return nil
}
