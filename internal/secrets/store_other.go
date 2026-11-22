//go:build !darwin && !windows

package secrets

// NewDefaultStore returns an in-memory fallback Store on unsupported platforms.
func NewDefaultStore() Store {
	return NewMemoryStore()
}
