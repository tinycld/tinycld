package coreserver

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// disabledRejectionMessage is what a suspended user sees. Deliberately says
// who to contact rather than leaking whether the credentials were correct —
// the check runs AFTER PocketBase validates them, so "wrong password" and
// "disabled" stay distinguishable to the server but not to a prober.
const disabledRejectionMessage = "This account has been disabled. Contact an administrator to restore access."

// RegisterDisabledUserGuard refuses authentication for users carrying
// `disabled = true`.
//
// Two hooks, not one. OnRecordAuthWithPasswordRequest covers the password
// form; OnRecordAuthRequest is the common tail every successful auth passes
// through — OTP, OAuth2, and auth-refresh included. Binding only the password
// hook would leave a user with a live refresh token able to keep renewing a
// session indefinitely after being disabled, and would let an OAuth2 sign-in
// through untouched.
//
// This is a hook rather than a collection rule because PocketBase has no
// authRule: rules gate record reads/writes, not token issuance.
func RegisterDisabledUserGuard(app *pocketbase.PocketBase) {
	registerDisabledUserGuardCore(app)
}

// registerDisabledUserGuardCore is the core.App-typed body so tests can wire
// the hook into a *tests.TestApp directly.
func registerDisabledUserGuardCore(app core.App) {
	app.OnRecordAuthWithPasswordRequest("users").BindFunc(func(e *core.RecordAuthWithPasswordRequestEvent) error {
		// e.Record is nil when the identity didn't resolve to a user at all;
		// let PocketBase produce its own "invalid credentials" response rather
		// than turning a bad username into a different error shape.
		if e.Record != nil && e.Record.GetBool("disabled") {
			return e.ForbiddenError(disabledRejectionMessage, nil)
		}
		return e.Next()
	})

	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		if e.Record != nil && e.Record.GetBool("disabled") {
			return e.ForbiddenError(disabledRejectionMessage, nil)
		}
		return e.Next()
	})
}
