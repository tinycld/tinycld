package coreserver

import (
	"sort"
	"testing"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/quota"
	"tinycld.org/core/rlstest"
)

// The tripwire for the composition gap (a hand-rolled second composition once
// drifted from Register, silently missed the users field guard, and let any
// member PATCH their own role to owner).
//
// The full comparison needs BOTH arms — Register and the composition layered on
// top of it — and only a repo that imports both can run it. That one lives with
// the embedder. This is the half that can live here, and it is deliberately
// blunt: a golden count per hook of what Register itself binds.
//
// It exists so a change made in THIS repo fails in THIS repo. Without it, a
// registration added to Register lands green here and only breaks in a
// downstream repo whose checkout the author may not even have.
//
// WHEN THIS FAILS: you changed what Register binds. That is allowed — update
// the number here, and then decide whether the other composition should get the
// same registration. If it should, it belongs in RegisterSharedEarly or
// RegisterSharedCore instead of Register's own tail. If it should not, record
// the divergence in the embedder's parity allowlist with a reason.
var hostHookCounts = map[string]int{
	"OnBootstrap":                     6,
	"OnCollectionAfterCreateError":    1,
	"OnCollectionAfterCreateSuccess":  1,
	"OnCollectionAfterDeleteError":    1,
	"OnCollectionAfterDeleteSuccess":  1,
	"OnCollectionAfterUpdateError":    1,
	"OnCollectionAfterUpdateSuccess":  1,
	"OnCollectionCreate":              1,
	"OnCollectionCreateExecute":       2,
	"OnCollectionCreateRequest":       1,
	"OnCollectionDeleteExecute":       5,
	"OnCollectionDeleteRequest":       1,
	"OnCollectionUpdate":              2,
	"OnCollectionUpdateExecute":       2,
	"OnCollectionUpdateRequest":       1,
	"OnCollectionValidate":            1,
	"OnMailerRecordPasswordResetSend": 2,
	"OnModelAfterCreateError":         2,
	"OnModelAfterCreateSuccess":       4,
	"OnModelAfterDeleteError":         2,
	"OnModelAfterDeleteSuccess":       3,
	"OnModelAfterUpdateError":         2,
	"OnModelAfterUpdateSuccess":       4,
	"OnModelCreate":                   2,
	"OnModelCreateExecute":            2,
	"OnModelDelete":                   3,
	"OnModelDeleteExecute":            2,
	"OnModelUpdate":                   2,
	"OnModelUpdateExecute":            2,
	"OnModelValidate":                 2,
	"OnRecordAfterCreateError":        1,
	"OnRecordAfterCreateSuccess":      3,
	"OnRecordAfterDeleteError":        1,
	"OnRecordAfterDeleteSuccess":      1,
	"OnRecordAfterUpdateError":        1,
	"OnRecordAfterUpdateSuccess":      2,
	"OnRecordAuthRequest":             1,
	"OnRecordAuthWithPasswordRequest": 1,
	"OnRecordCreate":                  1,
	"OnRecordCreateExecute":           2,
	"OnRecordCreateRequest":           6,
	"OnRecordDelete":                  2,
	"OnRecordDeleteExecute":           5,
	"OnRecordDeleteRequest":           7,
	"OnRecordUpdate":                  3,
	"OnRecordUpdateExecute":           4,
	"OnRecordUpdateRequest":           9,
	"OnRecordValidate":                6,
	"OnServe":                         23,
	"OnSettingsReload":                1,
	"OnTerminate":                     2,
}

func TestRegisterBindsTheRecordedHandlerCounts(t *testing.T) {
	// Register falls back to quota.RegisteredSources() (a process global other
	// tests may have populated) when Options.QuotaSources is empty.
	quota.ResetSourcesForTesting()

	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	Register(app, Options{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		TypesDir:      t.TempDir(),
		PublicDir:     t.TempDir(),
		HooksPoolSize: 1,
	})

	counts := rlstest.HookHandlerCounts(t, app)

	names := make([]string, 0, len(counts)+len(hostHookCounts))
	seen := map[string]bool{}
	for n := range counts {
		if !seen[n] {
			names, seen[n] = append(names, n), true
		}
	}
	for n := range hostHookCounts {
		if !seen[n] {
			names, seen[n] = append(names, n), true
		}
	}
	sort.Strings(names)

	for _, name := range names {
		got, want := counts[name], hostHookCounts[name]
		if got != want {
			t.Errorf("%s: Register binds %d handler(s), recorded %d. "+
				"If this is intended, update hostHookCounts — and decide whether the "+
				"embedded composition needs the same registration (shared set) or not "+
				"(its parity allowlist, with a reason).", name, got, want)
		}
	}
}
