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
// *tests.TestApp. Create/update hooks run inside the write's transaction and
// hand the handler that transactional e.App directly, so a failed derived
// write fails the client's request and nothing half-expanded is ever visible.
// Delete is different: the fork's onRecordDeleteExecute runs e.Next() inside
// its own RunInTransaction, then restores e.App to the non-transactional app
// before returning control here — so by the time a delete hook's code after
// e.Next() runs, the parent delete has already committed. The delete hooks
// below therefore open their own transaction around e.Next() plus the derived
// cleanup, so a failed cleanup rolls back the parent delete too instead of
// orphaning derived rows.
func registerCore(app core.App) {
	app.OnRecordCreate("group_members").BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return deriveForMember(e.App, e.Record.GetString("group"), e.Record.GetString("user"))
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
		if err := e.Next(); err != nil {
			return err
		}
		if e.Record.GetString("role") != "guest" || e.Record.Original().GetString("role") == "guest" {
			return nil
		}
		return RemoveUserMemberships(e.App, e.Record.Id)
	})

	for _, t := range RegisteredGrantTables() {
		bindGrantTable(app, t)
	}
}

func bindGrantTable(app core.App, t GrantTable) {
	app.OnRecordCreate(t.Collection).BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if !isGrant(e.Record) {
			return nil
		}
		return expandGrant(e.App, t, e.Record)
	})
	app.OnRecordUpdate(t.Collection).BindFunc(func(e *core.RecordEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if !isGrant(e.Record) {
			return nil
		}
		return syncDerived(e.App, t, e.Record)
	})
	app.OnRecordDelete(t.Collection).BindFunc(func(e *core.RecordEvent) error {
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			if !isGrant(e.Record) {
				return nil
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
