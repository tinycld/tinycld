package coreserver

import (
	"context"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"tinycld.org/core/autoupgrade"
	"tinycld.org/core/installjob"
	"tinycld.org/core/rlstest"
)

// startingDelegate records what the boot path does with a Delegate that runs
// its own loop.
type startingDelegate struct {
	recordingDelegate
	started []context.Context
}

func (d *startingDelegate) Start(ctx context.Context) { d.started = append(d.started, ctx) }

func serve(t *testing.T, app core.App) {
	t.Helper()
	e := new(core.ServeEvent)
	e.App = app
	e.Router = router.NewRouter[*core.RequestEvent](nil)
	if err := app.OnServe().Trigger(e); err != nil {
		t.Fatalf("OnServe: %v", err)
	}
}

func installDelegate(t *testing.T, d autoupgrade.Delegate) {
	t.Helper()
	autoupgrade.SetDelegate(d)
	t.Cleanup(func() { autoupgrade.SetDelegate(nil) })
}

// The production boot path: serving pushes the STORED flag once and starts the
// Delegate's loop with a context that ends when the app terminates.
func TestServePushesStoredPolicyAndStartsDelegate(t *testing.T) {
	app := adminConsoleTestApp(t)
	row, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": autoupgrade.KeyEnabled})
	mustNil(t, err)
	row.Set("value", "false")
	mustNil(t, app.Save(row))

	d := &startingDelegate{}
	installDelegate(t, d)
	registerAutoUpgradeOn(app)
	serve(t, app)

	if len(d.calls) != 1 || d.calls[0] != false {
		t.Fatalf("boot push: calls %v, want [false]", d.calls)
	}
	if len(d.started) != 1 {
		t.Fatalf("Start called %d times, want 1", len(d.started))
	}
	ctx := d.started[0]
	if ctx.Err() != nil {
		t.Fatal("Start got a context that had already ended")
	}
	mustNil(t, app.OnTerminate().Trigger(&core.TerminateEvent{App: app}))
	if ctx.Err() == nil {
		t.Fatal("the Delegate's context must end when the app terminates")
	}
}

// An unreadable flag is not pushed as "off"; the loop still starts, and its
// own tick reports the read failure.
func TestServeSkipsBootPushWhenFlagUnreadable(t *testing.T) {
	app := adminConsoleTestApp(t)
	d := &startingDelegate{}
	installDelegate(t, d)
	registerAutoUpgradeOn(app)
	dropSystemSettings(t, app)
	serve(t, app)

	if len(d.calls) != 0 {
		t.Fatalf("pushed %v from an unreadable flag", d.calls)
	}
	if len(d.started) != 1 {
		t.Fatalf("Start called %d times, want 1", len(d.started))
	}
}

// bootedPocketBase is a real *pocketbase.PocketBase (what newLocalScheduler
// takes) on a fresh data dir with this repo's migrations applied.
func bootedPocketBase(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir(), HideStartBanner: true})
	mustNil(t, app.Bootstrap())
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))
	return app
}

// The scheduler's real apply closure starts a version change through the same
// path as a person, and the install log row says the job was automatic.
//
// The registry row has no install spec, so the job fails right after it writes
// the log row: specForVersion refuses an unrecognized source before any
// workspace, network or toolchain step. Nothing else is stubbed — the compat
// gate, the job claim, and the rebuild goroutine are the production ones.
func TestLocalSchedulerApplyStartsAnAutoVersionChange(t *testing.T) {
	app := bootedPocketBase(t)
	saveRegistryRow(t, app, "widgets", "installed")

	s := newLocalScheduler(app)
	mustNil(t, s.apply([]installjob.VersionChange{{Slug: "widgets", TargetVersion: "0.6.0"}}))

	var row *core.Record
	deadline := time.Now().Add(10 * time.Second)
	for {
		rows, err := app.FindRecordsByFilter("pkg_install_log", "status != 'running'", "", 0, 0)
		mustNil(t, err)
		if len(rows) == 1 && !installjob.Running() {
			row = rows[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the version-change job did not finish; rows=%d running=%v", len(rows), installjob.Running())
		}
		time.Sleep(20 * time.Millisecond)
	}

	if row.GetString("trigger") != "auto" || row.GetString("action") != "version_change" {
		t.Fatalf("log row trigger=%q action=%q", row.GetString("trigger"), row.GetString("action"))
	}
	var changes []installjob.VersionChange
	mustNil(t, row.UnmarshalJSONField("changes", &changes))
	if len(changes) != 1 || changes[0].Slug != "widgets" || changes[0].TargetVersion != "0.6.0" {
		t.Fatalf("changes %+v", changes)
	}
	if row.GetString("status") != "failed" {
		t.Fatalf("status %q: the fixture must make the job stop before a rebuild", row.GetString("status"))
	}
}
