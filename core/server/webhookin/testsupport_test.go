package webhookin

import (
	"errors"
	"testing"
)

var errSentinel = errors.New("sentinel")

// resetRegistry clears global registry state so each test starts clean.
func resetRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	sources = map[string]Source{}
	registryMu.Unlock()
}
