package groups

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("groups")

// Register binds the expansion hooks for group_members, users and every
// registered grant table, plus the boot reconcile. Packages must have called
// RegisterGrantTable before this runs, which RegisterSharedCore's position
// after RegisterExtras guarantees.
func Register(app *pocketbase.PocketBase) {
	registerCore(app)
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := Reconcile(e.App); err != nil {
			log.Error("boot reconcile failed", "err", err)
		}
		return e.Next()
	})
}

// registerCore is the core.App-typed body so tests can bind it on a
// *tests.TestApp. None of the model save/delete hooks give the handler a
// transaction the caller did not already open — the REST save path
// (apis/record_crud.go, forms/record_upsert.go) calls SaveWithContext on the
// plain app with no wrapping transaction, and core.App's onRecordSaveExecute
// opens none either. So every handler below that writes derived rows after
// e.Next() opens its own transaction around e.Next() plus that write:
// RunInTransaction nests onto an outer transaction when the caller happens to
// already be in one (see third_party/pocketbase/core/db_tx.go), and starts a
// fresh one otherwise. Either way, a failed derived write rolls back the
// parent row's save/delete too, instead of leaving it committed with nothing
// (or half of the expansion) expanded.
//
// A handler opens that transaction ONLY when it has derived work to do. A
// transaction opened inside a hook is invisible to the app that started the
// save, so PocketBase fires the row's after-success hooks — realtime among
// them — before the transaction commits. The realtime access check then
// cannot see the row and drops the event. A direct membership row, or a user
// update that is not a demotion to guest, has nothing to expand and must not
// pay that price.
func registerCore(app core.App) {
	app.OnRecordCreate("group_members").BindFunc(func(e *core.RecordEvent) error {
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			return deriveForMember(txApp, e.Record.GetString("group"), e.Record.GetString("user"))
		})
	})
	app.OnRecordDelete("group_members").BindFunc(func(e *core.RecordEvent) error {
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			return removeDerivedForMember(txApp, e.Record.GetString("group"), e.Record.GetString("user"))
		})
	})

	// Listeners run after commit so a package side effect (mail provisioning)
	// never sees an uncommitted membership.
	app.OnRecordAfterCreateSuccess("group_members").BindFunc(func(e *core.RecordEvent) error {
		notify(e.Record, true)
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess("group_members").BindFunc(func(e *core.RecordEvent) error {
		notify(e.Record, false)
		return e.Next()
	})

	// A user demoted to guest leaves every group: guests are never members.
	app.OnRecordUpdate("users").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("role") != "guest" || e.Record.Original().GetString("role") == "guest" {
			return e.Next()
		}
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			return RemoveUserMemberships(txApp, e.Record.Id)
		})
	})

	for _, t := range RegisteredGrantTables() {
		bindGrantTable(app, t)
	}
}

func bindGrantTable(app core.App, t GrantTable) {
	app.OnRecordCreate(t.Collection).BindFunc(func(e *core.RecordEvent) error {
		if !isGrant(e.Record) {
			return e.Next()
		}
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			return expandGrant(txApp, t, e.Record)
		})
	})
	app.OnRecordUpdate(t.Collection).BindFunc(func(e *core.RecordEvent) error {
		if !isGrant(e.Record) {
			return e.Next()
		}
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			return syncDerived(txApp, t, e.Record)
		})
	})
	app.OnRecordDelete(t.Collection).BindFunc(func(e *core.RecordEvent) error {
		if !isGrant(e.Record) {
			return e.Next()
		}
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			return removeDerivedForGrant(txApp, t, e.Record)
		})
	})
}

func notify(member *core.Record, joined bool) {
	err := notifyMembership(MembershipEvent{
		UserID:  member.GetString("user"),
		GroupID: member.GetString("group"),
		Joined:  joined,
	})
	if err != nil {
		log.Error("membership listener failed", "user", member.GetString("user"), "group", member.GetString("group"), "joined", joined, "err", err)
	}
}
