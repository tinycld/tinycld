package coreserver

import (
	"log"
	"reflect"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// selfEditableUserFields lists the fields the record's own user is allowed
// to change via a direct API write. Profile-style fields (`name`, `avatar`)
// plus `password` — an authenticated self password change is safe because
// PocketBase's own auth-record validation requires and verifies `oldPassword`
// for a non-superuser password change (forms.RecordUpsert.checkOldPassword),
// so allowing the field here defers to that check rather than bypassing it.
// `email` still flows through PB's confirmation endpoint (requestEmailChange);
// `verified` is set by confirmVerification.
//
// `is_demo` is intentionally absent: a sandboxed user must not be able to
// lift their own restrictions.
var selfEditableUserFields = map[string]bool{
	"name":     true,
	"avatar":   true,
	"password": true,
}

// adminEditableUserFields lists the fields owners/admins are allowed to modify
// on other users via the relaxed users.updateRule.
//
// We use an allowlist rather than a denylist so that future additions to the
// users collection (PB upgrades, new auth hooks) default to "rejected"
// instead of silently becoming admin-writable. `role` is admin-editable so an
// owner/admin can promote or demote a member; `disabled` so they can suspend
// and restore an account.
//
// `disabled` is deliberately absent from selfEditableUserFields, for the same
// reason as `is_demo`: a suspended user must not be able to lift their own
// suspension. Self-disable goes through POST /api/account/disable, which
// re-verifies identity by email confirmation.
var adminEditableUserFields = map[string]bool{
	"name":     true,
	"avatar":   true,
	"is_demo":  true,
	"role":     true,
	"disabled": true,
}

// revokeSessionsOnDisable rotates the auth token key when an admin flips
// `disabled` on, so every JWT already issued to that account stops working.
//
// Without it a suspension is advisory until the last token expires — hours in
// which a compromised account an admin has just locked keeps its live
// sessions, which is the case suspension exists for. Self-disable already does
// this (see the /api/account/disable handler); this is the admin-side copy.
//
// The cost is that re-enabling forces a fresh sign-in on every device. That
// trade is already accepted for self-disable, and it is the right one: a
// suspension you can wait out is not a suspension.
//
// Only the false→true edge rotates. Re-enabling, and any other admin edit,
// must leave sessions alone — otherwise a name change signs the user out.
func revokeSessionsOnDisable(record *core.Record, original *core.Record) {
	if record.GetBool("disabled") && !original.GetBool("disabled") {
		record.RefreshTokenKey()
	}
}

// RegisterUsersDemoAuditHook writes an audit_logs entry every time the
// is_demo flag flips on a user record. Demo state changes are
// operationally interesting (App Review setup, prospect demos, accidental
// flips) and worth a forensic trail. We write directly rather than using
// the generic audit.RegisterCollection because we don't want every name
// or avatar tweak to spam the audit log — this captures only the demo
// transition.
func RegisterUsersDemoAuditHook(app *pocketbase.PocketBase) {
	registerUsersDemoAuditHookCore(app)
}

func registerUsersDemoAuditHookCore(app core.App) {
	app.OnRecordUpdateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		original := e.Record.Original()
		wasDemo := original.GetBool("is_demo")
		nextDemo := e.Record.GetBool("is_demo")

		// Run the rest of the chain first so audit only fires on a
		// successful update (rejections shouldn't leave a phantom log).
		if err := e.Next(); err != nil {
			return err
		}
		if wasDemo == nextDemo {
			return nil
		}

		auditCol, err := e.App.FindCollectionByNameOrId("audit_logs")
		if err != nil {
			log.Printf("[demo-audit] missing audit_logs collection: %v", err)
			return nil
		}
		auditRec := core.NewRecord(auditCol)
		auditRec.Set("action", "users.demo_changed")
		auditRec.Set("resource_type", "users")
		auditRec.Set("resource_id", e.Record.Id)
		auditRec.Set("resource_label", e.Record.GetString("email"))
		auditRec.Set("metadata", map[string]any{
			"from": wasDemo,
			"to":   nextDemo,
		})
		if e.Auth != nil {
			auditRec.Set("actor", e.Auth.Id)
		}
		if err := e.App.Save(auditRec); err != nil {
			log.Printf("[demo-audit] failed to write audit entry: %v", err)
		}
		return nil
	})
}

// RegisterUsersFieldGuard rejects update requests on the users collection
// that fall outside two narrow paths:
//   - Self-edits: the record owner can change anything (PB's normal auth).
//   - Admin-edits: a caller who is an owner/admin (users.role) can change
//     ONLY the allowlisted fields above.
//
// The relaxed users.updateRule lets any authenticated user attempt an update so
// client code can use pbtsdb mutations directly; this hook narrows that to
// "owner/admin, allowlisted field only". PocketBase's collection rules can't
// constrain which fields a write touches, so per-field policy lives here.
func RegisterUsersFieldGuard(app *pocketbase.PocketBase) {
	registerUsersFieldGuardCore(app)
}

// registerUsersFieldGuardCore is the core.App-typed body so tests can wire
// the hook into a *tests.TestApp directly. Callers in production go through
// RegisterUsersFieldGuard which takes the concrete *pocketbase.PocketBase.
func registerUsersFieldGuardCore(app core.App) {
	app.OnRecordUpdateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("Authentication required", nil)
		}

		original := e.Record.Original()

		// Superusers bypass the field allowlist. The guard exists to constrain
		// regular API clients (self-edits, org-admin edits); superusers already
		// bypass collection rules and are trusted with full administrative
		// writes (seed/reset scripts overwrite email/password on the demo
		// account, operators reset locked-out users, etc.). Suspension still
		// ends sessions on this path: an operator locking a compromised
		// account has the same expectation an org admin does.
		if e.Auth.IsSuperuser() {
			revokeSessionsOnDisable(e.Record, original)
			return e.Next()
		}

		isSelf := e.Auth.Id == e.Record.Id

		// Demo accounts are shared across anonymous visitors via /api/demo/start.
		// Letting one visitor self-edit the profile (name, avatar, ...) leaves
		// the change visible to every subsequent visitor until the nightly
		// reset wipes it. Reject self-edits outright; admin edits are still
		// allowed so an operator can flip is_demo back off if needed.
		if isSelf && original.GetBool("is_demo") {
			return e.ForbiddenError("Demo accounts are read-only", nil)
		}

		allowed := selfEditableUserFields
		if !isSelf {
			allowed = adminEditableUserFields
		}

		// Walk every field; reject the request if any non-allowed field
		// changed. reflect.DeepEqual handles every field type (string, bool,
		// *PasswordFieldValue, etc.) without us enumerating each.
		for _, field := range e.Record.Collection().Fields {
			name := field.GetName()
			if reflect.DeepEqual(e.Record.GetRaw(name), original.GetRaw(name)) {
				continue
			}
			if !allowed[name] {
				msg := "Only the record owner can change this field"
				if isSelf {
					// Sensitive fields (password, email, verified) go through
					// PB's dedicated confirmation endpoints, not direct
					// updates. is_demo is admin-only by design.
					msg = "This field cannot be changed via a direct update"
				}
				return e.ForbiddenError(msg, map[string]any{"field": name})
			}
		}

		// Self-edits don't need the admin check below.
		if isSelf {
			return e.Next()
		}

		// Single-org: the caller must hold an owner/admin role on their own
		// users record to edit another user.
		if !isOrgAdmin(e.Auth) {
			return e.ForbiddenError("Org admin role required", nil)
		}

		// After the allowlist walk, not before: rotating the key changes
		// `tokenKey`, which the walk would then reject as an unpermitted field.
		revokeSessionsOnDisable(e.Record, original)
		return e.Next()
	})
}
