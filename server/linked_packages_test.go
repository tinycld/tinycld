package main

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// CI assembles the lean shell -- app + core, no `--with <feature>` flags (see
// .github/workflows/ci.yml). The only packages linked there are the E2E stubs
// the workflow scaffolds. This asserts that stays true.
//
// Why CI-only: a developer's workspace is assembled from whichever members they
// chose, so locally this set is legitimately mail, drive, calendar and the rest.
// There is no violation to detect on a dev machine -- only in the one
// environment whose linked set is supposed to be fixed.
//
// What it catches: a dependency added to core or the app shell that drags a
// feature package into the lean assembly, breaking the lean-shell guarantee (a
// feature-less workspace must typecheck, boot and pass its tests). Otherwise
// invisible -- whoever adds it has that package linked locally and sees green.
//
// It lives here, not in core, because bundled-packages.json is generated into
// this directory (and gitignored). This is also the module CI runs `go test`
// in. The file is the same one coreserver.SyncBundledPackages reads at boot, so
// this checks the set the server actually loads.

// Scaffolded by the CI workflow for the keyboard-shortcut and search E2E specs
// (tests/scripts/scaffold-shortcut-stub.ts, scaffold-search-stubs.ts). Fixtures,
// not features -- gitignored at the workspace root, generated per run.
var stubSlugs = []string{"shortcut-stub", "search-alpha", "search-beta"}

func TestLeanShellLinksOnlyCoreAndStubs(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("dev workspaces legitimately link features; only CI's set is fixed")
	}

	const path = "bundled-packages.json"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var packages []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(data, &packages); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var unexpected []string
	var sawCore bool
	for _, pkg := range packages {
		switch {
		case pkg.Slug == "core":
			sawCore = true
		case slices.Contains(stubSlugs, pkg.Slug):
		default:
			unexpected = append(unexpected, pkg.Slug)
		}
	}

	if len(unexpected) > 0 {
		// A feature package here means something now depends on it in an
		// assembly meant to have none. Remove the dependency -- do not add the
		// slug to stubSlugs.
		t.Errorf("lean shell links unexpected feature packages: %v", unexpected)
	}

	// Guards the parse: a moved file or changed shape would otherwise yield an
	// empty list and pass vacuously. Core is the one package present in EVERY
	// assembly -- the stubs are not, since only the E2E job scaffolds them and
	// this test also runs in the Go job, whose set is core alone.
	if !sawCore {
		t.Errorf("core not found in %s (parsed %d entries) -- the check would pass vacuously", path, len(packages))
	}
}
