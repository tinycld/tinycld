package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
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
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not mention %q", msg, want)
		}
	}
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

// CheckManifest is what lets the upload branch of the restore API answer 409
// BEFORE it answers 202 and starts an asynchronous restore. It must therefore
// reach the same verdict as phase 2 while writing NOTHING: a check that staged
// or armed anything would turn a refusal into a half-started restore.
func TestCheckManifestDecidesWithoutSideEffects(t *testing.T) {
	resetRestoreState(t)
	data, id := archiveFor(t)
	app := newTestApp(t)

	m, err := CheckManifest(app, bytes.NewReader(data), id)
	if err != nil {
		t.Fatalf("an equal package set must be accepted: %v", err)
	}
	if m.Format == "" {
		t.Fatalf("the manifest must be returned: %+v", m)
	}
	if _, err := os.Stat(restoreDir(app)); !os.IsNotExist(err) {
		t.Fatalf("CheckManifest must write nothing under restore/: %v", err)
	}

	// Drop the fictional package the archive names, so this binary can no longer
	// run the archive's package set.
	regs, err := app.FindRecordsByFilter("pkg_registry", "slug = 'widgets'", "", 0, 0)
	if err != nil || len(regs) != 1 {
		t.Fatalf("registry rows: %v, %v", regs, err)
	}
	if err := app.Delete(regs[0]); err != nil {
		t.Fatal(err)
	}

	_, err = CheckManifest(app, bytes.NewReader(data), id)
	var mismatch *MismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("a missing package must be a *MismatchError, got %v", err)
	}
	if len(mismatch.Diff.Missing) != 1 || mismatch.Diff.Missing[0] != "widgets" {
		t.Fatalf("diff = %+v", mismatch.Diff)
	}
	if _, err := os.Stat(restoreDir(app)); !os.IsNotExist(err) {
		t.Fatalf("a refusal must write nothing under restore/: %v", err)
	}
}

// A deployment that can rebuild itself has nothing to refuse: it becomes whatever
// the archive names, so the check must pass a set this binary does not carry.
func TestCheckManifestAcceptsAnyoneWithARebuilder(t *testing.T) {
	resetRestoreState(t)
	data, id := archiveFor(t)
	app := newTestApp(t)
	regs, err := app.FindRecordsByFilter("pkg_registry", "slug = 'widgets'", "", 0, 0)
	if err != nil || len(regs) != 1 {
		t.Fatalf("registry rows: %v, %v", regs, err)
	}
	if err := app.Delete(regs[0]); err != nil {
		t.Fatal(err)
	}
	RegisterRebuilder(func(context.Context, *installjob.Job, format.Lockfile) error { return nil })
	if _, err := CheckManifest(app, bytes.NewReader(data), id); err != nil {
		t.Fatalf("a rebuilder must make any package set acceptable: %v", err)
	}
}
