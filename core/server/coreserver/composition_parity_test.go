package coreserver

import (
	"sort"
	"testing"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/logging"
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
	"OnBootstrap":                         7,
	"OnBootstrapClear":                    2, // PocketBase's own __pbCronStop__ and the fork's notify reload guard
	"OnCollectionAfterCreateError":        1,
	"OnCollectionAfterCreateSuccess":      1,
	"OnCollectionAfterDeleteError":        1,
	"OnCollectionAfterDeleteSuccess":      1,
	"OnCollectionAfterUpdateError":        1,
	"OnCollectionAfterUpdateSuccess":      1,
	"OnCollectionCreate":                  1,
	"OnCollectionCreateExecute":           2,
	"OnCollectionCreateRequest":           1,
	"OnCollectionDeleteExecute":           5,
	"OnCollectionDeleteRequest":           1,
	"OnCollectionUpdate":                  2,
	"OnCollectionUpdateExecute":           2,
	"OnCollectionUpdateRequest":           1,
	"OnCollectionValidate":                1,
	"OnMailerRecordPasswordResetSend":     2,
	"OnModelAfterCreateError":             2,
	"OnModelAfterCreateSuccess":           5,
	"OnModelAfterDeleteError":             2,
	"OnModelAfterDeleteSuccess":           4,
	"OnModelAfterUpdateError":             2,
	"OnModelAfterUpdateSuccess":           5,
	"OnModelCreate":                       2,
	"OnModelCreateExecute":                2,
	"OnModelDelete":                       3,
	"OnModelDeleteExecute":                2,
	"OnModelUpdate":                       2,
	"OnModelUpdateExecute":                2,
	"OnModelValidate":                     2,
	"OnRecordAfterCreateError":            1,
	"OnRecordAfterCreateSuccess":          6,
	"OnRecordAfterDeleteError":            1,
	"OnRecordAfterDeleteSuccess":          3,
	"OnRecordAfterUpdateError":            1,
	"OnRecordAfterUpdateSuccess":          4,
	"OnRecordAuthRequest":                 2, // the disabled-user guard and the MFA gate
	"OnRecordAuthWithPasswordRequest":     2,
	"OnRecordConfirmPasswordResetRequest": 1, // a reset ends an admin-set temporary password
	"OnRecordCreate":                      4,
	"OnRecordCreateExecute":               2,
	"OnRecordCreateRequest":               16,
	"OnRecordDelete":                      5,
	"OnRecordDeleteExecute":               5,
	"OnRecordDeleteRequest":               18,
	"OnRecordUpdate":                      8, // incl. app passwords revoked on a tokenKey change
	"OnRecordUpdateExecute":               4,
	"OnRecordUpdateRequest":               19,
	"OnRecordValidate":                    6,
	"OnServe":                             37,
	"OnSettingsReload":                    1,
	"OnTerminate":                         12, // the start of the terminate's shared deadline (shutdown.Register, shared: the first shared registration that waits binds it, readonly.Register in RegisterSharedEarly); readonly.Register ending tail waits (shared, via RegisterSharedEarly); realtime storing every collaborative document; the background runners of notify (mention notifications), coreserver (invite email + notification; boot notices and the auto-upgrade checks reuse it), audit (audit rows) and search (search sources), all shared, via RegisterSharedCore; drainhooks dropping the app from the process drain (shared, via realtime's drain hook); the package-operation stop and watcher-restart veto (host-only, rebuildJobsFor)
}

func TestRegisterBindsTheRecordedHandlerCounts(t *testing.T) {
	// Register falls back to quota.RegisteredSources() (a process global other
	// tests may have populated) when Options.QuotaSources is empty.
	quota.ResetSourcesForTesting()
	// Register names its app the process's default log owner; later tests
	// must not write their records into this test's terminated app.
	t.Cleanup(func() { logging.SetDefaultApp(nil) })

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
