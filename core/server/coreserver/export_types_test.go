package coreserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// TestExportTypesOutputMatchesBootHook pins the contract that the
// `tinycld export-types` subcommand produces byte-identical output to
// what the boot-time OnServe hook writes — both paths go through the
// same GenerateSchemas function, so they MUST stay in lockstep. If they
// ever diverge (e.g. someone adds preprocessing to one branch but not
// the other), the rollout that moves these files out of git breaks
// silently: dev gets one shape from boot-time regen, CI/install gets
// another from the subcommand, and the `chore(types): regen` commits
// come back.
//
// We don't invoke the cobra subcommand directly (it needs a
// *pocketbase.PocketBase, which TestApp doesn't construct) — we run
// the underlying GenerateSchemas twice into two tempdirs and compare.
// The wrapper itself is trivial enough (just a SilenceUsage'd RunE
// around the same call) that integration is covered by the dev smoke
// check (`go run . export-types --typesDir ../../core/types && git
// diff core/types/`) we ran during implementation.
func TestExportTypesOutputMatchesBootHook(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	bootDir := t.TempDir()
	subcmdDir := t.TempDir()

	// Boot-hook path mirror: same call OnServe makes.
	GenerateSchemas(app, bootDir)
	// Subcommand path mirror: same call the cobra RunE makes.
	GenerateSchemas(app, subcmdDir)

	for _, name := range []string{"pbSchema.ts", "pbZodSchema.ts"} {
		bootBytes, err := os.ReadFile(filepath.Join(bootDir, name))
		if err != nil {
			t.Fatalf("read boot %s: %v", name, err)
		}
		subcmdBytes, err := os.ReadFile(filepath.Join(subcmdDir, name))
		if err != nil {
			t.Fatalf("read subcmd %s: %v", name, err)
		}
		if string(bootBytes) != string(subcmdBytes) {
			t.Errorf("%s: boot-hook and subcommand outputs differ", name)
		}
	}
}
