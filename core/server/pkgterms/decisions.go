package pkgterms

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

// Register binds the owner's decisions on a package's terms and the guard of
// the registry's terms fields. Bound once by coreserver's shared
// registration. Nothing here refuses a package or its requests for its terms
// state.
func Register(app core.App) {
	registerDecisions(app)
	registerRegistryGuards(app)
}

// registerDecisions makes a pkg_terms_decisions row the one way to accept or
// decline. The create rule admits only the owner; the request hook records
// who decided; the model hook checks the row against the package and, for an
// acceptance, writes the accepted version in the same transaction. The check
// and the registry write hold when a decision is created; a superuser can
// still change a decision row through the dashboard, and that does not touch
// the registry.
func registerDecisions(app core.App) {
	app.OnRecordCreateRequest(DecisionsCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		// A second check behind the create rule, which already admits only the
		// owner.
		if !e.HasSuperuserAuth() {
			if e.Auth == nil || e.Auth.GetString("role") != "owner" {
				return e.ForbiddenError("Only the owner decides on the terms of a package.", nil)
			}
			// The client does not choose whose decision this is.
			e.Record.Set("user", e.Auth.Id)
		}
		return e.Next()
	})

	app.OnRecordCreate(DecisionsCollection).BindFunc(func(e *core.RecordEvent) error {
		slug := e.Record.GetString("pkg")
		version := e.Record.GetString("terms_version")
		decision := e.Record.GetString("decision")
		original := e.App
		defer func() { e.App = original }()
		return original.RunInTransaction(func(txApp core.App) error {
			pkg, err := txApp.FindFirstRecordByFilter(RegistryCollection, "slug = {:slug}", dbx.Params{"slug": slug})
			if errors.Is(err, sql.ErrNoRows) {
				return validation.Errors{"pkg": validation.NewError("unknown_package", "There is no such package.")}
			}
			if err != nil {
				return err
			}
			current := pkg.GetString(FieldTermsVersion)
			if current == "" {
				return validation.Errors{"pkg": validation.NewError("no_terms", "The package has no terms.")}
			}
			if version != current {
				return validation.Errors{"terms_version": validation.NewError("not_current", "This is not the current version of the terms.")}
			}
			if !Pending(pkg) {
				return validation.Errors{"decision": validation.NewError("already_accepted", "The terms are already accepted.")}
			}
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			if decision != DecisionAccepted {
				return nil
			}
			pkg.Set(FieldAcceptedVersion, version)
			return txApp.Save(pkg)
		})
	})
}

// registerRegistryGuards keeps the terms fields of pkg_registry out of the
// owner's reach through the record API: the update rule of pkg_registry
// admits the owner (the owner turns packages on and off), so without this an
// owner could write terms_accepted_version and leave no decision record. A
// version change or an acceptance stays the work of the boot sync and of a
// decision row; a superuser still repairs either field.
func registerRegistryGuards(app core.App) {
	const refusal = "The terms of a package are accepted on the terms screen."
	app.OnRecordCreateRequest(RegistryCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() && (e.Record.GetString(FieldTermsVersion) != "" || e.Record.GetString(FieldAcceptedVersion) != "") {
			return e.ForbiddenError(refusal, nil)
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest(RegistryCollection).BindFunc(func(e *core.RecordRequestEvent) error {
		if !e.HasSuperuserAuth() {
			before := e.Record.Original()
			for _, field := range []string{FieldTermsVersion, FieldAcceptedVersion} {
				if e.Record.GetString(field) != before.GetString(field) {
					return e.ForbiddenError(refusal, nil)
				}
			}
		}
		return e.Next()
	})
}
