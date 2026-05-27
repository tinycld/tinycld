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
	if len(ProtectedYjsRootKeys) != len(expected) {
		t.Fatalf("expected %d protected keys, got %d", len(expected), len(ProtectedYjsRootKeys))
	}
	for _, k := range ProtectedYjsRootKeys {
		if !expected[k] {
			t.Errorf("unexpected protected key %q", k)
		}
	}
}
