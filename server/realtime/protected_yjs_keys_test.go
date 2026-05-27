package realtime

import "testing"

func TestProtectedYjsRootKeys(t *testing.T) {
	// The text package depends on these exact names; renaming requires
	// a coordinated migration. This test pins them.
	expected := map[string]bool{
		"clientAuthors":   true,
		"clientFirstSeen": true,
		"editEvents":      true,
	}
	seen := map[string]int{}
	for _, k := range ProtectedYjsRootKeys {
		seen[k]++
	}
	for k := range expected {
		if seen[k] != 1 {
			t.Errorf("key %q appeared %d times, want exactly 1", k, seen[k])
		}
	}
	for k := range seen {
		if !expected[k] {
			t.Errorf("unexpected protected key %q", k)
		}
	}
}
