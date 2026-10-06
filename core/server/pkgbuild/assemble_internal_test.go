package pkgbuild

import "testing"

// Mirrors the TS tests in scripts/__tests__ for isPlainSafeYamlKey /
// renderOverridesBlock (scripts/write-workspace-root.ts): the two
// implementations must stay in sync, since both generators have to emit
// byte-identical overrides blocks.
func TestPlainSafeYamlKey(t *testing.T) {
	cases := []struct {
		key  string
		safe bool
	}{
		{"expo", true},
		{"react", true},
		{"@tanstack/db", false},
		{"minimatch@3>brace-expansion", false},
		{"true", false},
		{"false", false},
		{"null", false},
		{"yes", false},
		{"123", false},
		{"*foo", false},
		{"&anchor", false},
		{"!tag", false},
		{"#comment", false},
		{"", false},
	}
	for _, c := range cases {
		if got := plainSafeYamlKey(c.key); got != c.safe {
			t.Errorf("plainSafeYamlKey(%q) = %v, want %v", c.key, got, c.safe)
		}
	}
}

func TestRenderOverridesBlockQuotesUnsafeKeysOnly(t *testing.T) {
	block := renderOverridesBlock(map[string]string{
		"expo":                        "55.0.26",
		"react":                       "19.2.0",
		"@tanstack/db":                "0.9.0",
		"minimatch@3>brace-expansion": "1.1.21",
	})
	want := "\noverrides:\n" +
		"  '@tanstack/db': 0.9.0\n" +
		"  expo: 55.0.26\n" +
		"  'minimatch@3>brace-expansion': 1.1.21\n" +
		"  react: 19.2.0\n"
	if block != want {
		t.Errorf("renderOverridesBlock() =\n%s\nwant\n%s", block, want)
	}
}
