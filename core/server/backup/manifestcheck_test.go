package backup

import (
	"errors"
	"testing"

	"tinycld.org/core/backup/format"
)

func TestCompareEmbedded(t *testing.T) {
	m := format.Manifest{Packages: map[string]string{"widgets": "1.0.0", "gadgets": "2.0.0"}}
	cases := []struct {
		name      string
		installed map[string]string
		wantEmpty bool
		missing   int
		extra     int
		deltas    int
	}{
		{"equal", map[string]string{"widgets": "1.0.0", "gadgets": "2.0.0"}, true, 0, 0, 0},
		{"missing package", map[string]string{"widgets": "1.0.0"}, false, 1, 0, 0},
		{"extra package", map[string]string{"widgets": "1.0.0", "gadgets": "2.0.0", "sprockets": "1.0.0"}, false, 0, 1, 0},
		{"version delta", map[string]string{"widgets": "1.1.0", "gadgets": "2.0.0"}, false, 0, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := compareEmbedded(m, c.installed)
			if d.Empty() != c.wantEmpty || len(d.Missing) != c.missing || len(d.Extra) != c.extra || len(d.VersionDelta) != c.deltas {
				t.Fatalf("%+v", d)
			}
		})
	}
}

// A mismatch error has to read as ErrManifestMismatch through errors.Is, or
// every caller that routes on the sentinel would report it as an unknown
// failure instead of a package-set difference the operator can act on.
func TestMismatchErrorIsSentinelAndListsTheDiff(t *testing.T) {
	err := &MismatchError{Diff: Diff{
		Missing:      []string{"widgets"},
		Extra:        []string{"sprockets"},
		VersionDelta: map[string][2]string{"gadgets": {"1.0.0", "2.0.0"}},
	}}
	if !errors.Is(err, ErrManifestMismatch) {
		t.Fatal("a MismatchError must match ErrManifestMismatch")
	}
	msg := err.Error()
	for _, want := range []string{"widgets", "sprockets", "gadgets", "1.0.0", "2.0.0"} {
		if !contains(msg, want) {
			t.Fatalf("error %q does not mention %q", msg, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// installedPackages reads the registry the manifest check compares against, and
// must leave out core: core's version is a separate manifest field, and
// counting it as a package would report a mismatch on every core upgrade.
func TestInstalledPackagesSkipsCore(t *testing.T) {
	app := newTestApp(t)
	got, err := installedPackages(app)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["core"]; ok {
		t.Fatalf("core must not be reported as an installed package: %v", got)
	}
	if got["widgets"] != "1.0.0" {
		t.Fatalf("installed = %v", got)
	}
}
