package coreserver

import (
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// errLastOwner is the shared refusal for any path that would leave the org
// with zero active owners. Non-owners cannot assign the owner role (users
// field guard + updateRule), so a zero-owner org is unrecoverable without a
// superuser — the state must be unreachable, not merely discouraged.
var errLastOwner = errors.New(
	"this is the last owner — promote another member to owner first")

// hasOtherActiveOwner reports whether any enabled owner exists besides
// excludeID. "Active" is load-bearing: a disabled owner cannot log in to
// promote anyone, so counting one would let the last usable owner be removed.
func hasOtherActiveOwner(app core.App, excludeID string) (bool, error) {
	n, err := app.CountRecords("users",
		dbx.HashExp{"role": "owner", "disabled": false},
		dbx.Not(dbx.HashExp{"id": excludeID}),
	)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ensureNotLastActiveOwner returns errLastOwner when removing `user` from the
// owner pool (demote, disable, delete, offboard) would leave zero active
// owners. The record-request hooks below cover direct API writes; the
// self-service endpoints (/api/account/disable, /api/account/delete) and the
// admin offboard endpoint mutate through model-level saves that never pass
// those hooks, so they call this directly.
func ensureNotLastActiveOwner(app core.App, user *core.Record) error {
	if user.GetString("role") != "owner" || user.GetBool("disabled") {
		return nil
	}
	other, err := hasOtherActiveOwner(app, user.Id)
	if err != nil {
		return err
	}
	if !other {
		return errLastOwner
	}
	return nil
}

// requireNotLastActiveOwner adapts ensureNotLastActiveOwner to an endpoint
// handler: the last-owner refusal becomes a 400 with the message the client
// can show verbatim; anything else is a 500.
func requireNotLastActiveOwner(app core.App, re *core.RequestEvent, user *core.Record) error {
	if err := ensureNotLastActiveOwner(app, user); err != nil {
		if errors.Is(err, errLastOwner) {
			return re.BadRequestError(err.Error(), nil)
		}
		return re.InternalServerError("last-owner check failed", err)
	}
	return nil
}

// RegisterLastOwnerGuard rejects users-collection API writes that would leave
// the org with zero active owners: demoting the last owner, disabling them,
// or deleting them. The Members drawer computes isLastOwner client-side, but
// that only drives helper text — this is the server backstop.
func RegisterLastOwnerGuard(app *pocketbase.PocketBase) {
	registerLastOwnerGuardCore(app)
}

// registerLastOwnerGuardCore is the core.App-typed body so tests can wire the
// hook into a *tests.TestApp directly, mirroring registerUsersFieldGuardCore.
func registerLastOwnerGuardCore(app core.App) {
	app.OnRecordUpdateRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		original := e.Record.Original()
		if original.GetString("role") != "owner" || original.GetBool("disabled") {
			return e.Next()
		}
		losesOwnership := e.Record.GetString("role") != "owner" || e.Record.GetBool("disabled")
		if !losesOwnership {
			return e.Next()
		}
		// Operators can always re-promote someone from the superuser console,
		// so the guard steps aside for them — consistent with the field-guard
		// bypass, and it keeps decommissioning flows possible.
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		if err := ensureNotLastActiveOwner(e.App, original); err != nil {
			if errors.Is(err, errLastOwner) {
				return e.ForbiddenError(err.Error(), nil)
			}
			return e.InternalServerError("last-owner check failed", err)
		}
		return e.Next()
	})

	app.OnRecordDeleteRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		if err := ensureNotLastActiveOwner(e.App, e.Record); err != nil {
			if errors.Is(err, errLastOwner) {
				return e.ForbiddenError(err.Error(), nil)
			}
			return e.InternalServerError("last-owner check failed", err)
		}
		return e.Next()
	})
}
