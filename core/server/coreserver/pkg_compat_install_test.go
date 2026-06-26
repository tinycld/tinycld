package coreserver

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// installRegistryRow inserts a pkg_registry row at the given version so
// checkInstallCompat resolves it as installed.
func installRegistryRow(t *testing.T, app core.App, slug, version string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatalf("find pkg_registry: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("slug", slug)
	rec.Set("version", version)
	rec.Set("status", "installed")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save registry row %s: %v", slug, err)
	}
}

// A package with no peerVersions installs regardless of what else is present.
func TestCheckInstallCompat_NoPeerVersions(t *testing.T) {
	app := newRegistryOnlyApp(t)
	m := &parsedManifest{Slug: "todo", Version: "1.0.0"}
	if err := checkInstallCompat(app, m); err != nil {
		t.Fatalf("expected no error for peerVersions-less package, got %v", err)
	}
}

// The calendar-slots case: a peer requirement on an ABSENT package must fail the
// install up front (the bug — it used to install and roll its migration back).
func TestCheckInstallCompat_MissingDependencyFails(t *testing.T) {
	app := newRegistryOnlyApp(t)
	m := &parsedManifest{
		Slug:         "calendar-slots",
		Version:      "0.1.0",
		PeerVersions: map[string]string{"calendar": "^0.1.0"},
	}
	err := checkInstallCompat(app, m)
	if err == nil {
		t.Fatal("expected a compat error when calendar is not installed, got nil")
	}
	if !strings.Contains(err.Error(), "calendar") || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error should name the missing dependency and say it's not installed, got: %v", err)
	}
}

// With the dependency present and in range, the install passes the gate.
func TestCheckInstallCompat_SatisfiedDependencyPasses(t *testing.T) {
	app := newRegistryOnlyApp(t)
	installRegistryRow(t, app, "calendar", "0.1.0")
	m := &parsedManifest{
		Slug:         "calendar-slots",
		Version:      "0.1.0",
		PeerVersions: map[string]string{"calendar": "^0.1.0"},
	}
	if err := checkInstallCompat(app, m); err != nil {
		t.Fatalf("expected no error when calendar 0.1.0 satisfies ^0.1.0, got %v", err)
	}
}

// A present-but-out-of-range dependency must fail, and the error must report the
// version actually found (not "not installed").
func TestCheckInstallCompat_OutOfRangeDependencyFails(t *testing.T) {
	app := newRegistryOnlyApp(t)
	installRegistryRow(t, app, "calendar", "0.2.0")
	m := &parsedManifest{
		Slug:         "calendar-slots",
		Version:      "0.1.0",
		PeerVersions: map[string]string{"calendar": "^0.1.0"},
	}
	err := checkInstallCompat(app, m)
	if err == nil {
		t.Fatal("expected a compat error when calendar 0.2.0 violates ^0.1.0, got nil")
	}
	if !strings.Contains(err.Error(), "0.2.0") {
		t.Fatalf("error should report the found version 0.2.0, got: %v", err)
	}
}

func TestCompatError_NilForEmpty(t *testing.T) {
	if err := compatError(nil); err != nil {
		t.Fatalf("compatError(nil) = %v, want nil", err)
	}
	if err := compatError([]compatViolation{}); err != nil {
		t.Fatalf("compatError(empty) = %v, want nil", err)
	}
}
