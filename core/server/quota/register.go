package quota

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// Register binds the enforcement hooks for the given sources.
//
// Every source gets OnRecordCreate and OnRecordUpdate. That is what makes the
// ceiling unskippable: any code path that persists a row — REST, WebDAV, IMAP,
// a package's own endpoint — goes through app.Save and therefore through these.
//
// A no-op when no sources are configured or limits resolves to unlimited.
func Register(app *pocketbase.PocketBase, sources []Source, limits LimitsFunc) error {
	if len(sources) == 0 || limits == nil {
		return nil
	}
	for _, src := range sources {
		if err := ValidateSource(src); err != nil {
			return err
		}
	}

	for _, src := range sources {
		bindCreate(app, sources, src, limits)
		bindUpdate(app, sources, src, limits)
	}
	return nil
}

func bindCreate(app *pocketbase.PocketBase, all []Source, src Source, limits LimitsFunc) {
	app.OnRecordCreate(src.Collection).BindFunc(func(e *core.RecordEvent) error {
		delta := int64(e.Record.GetInt(src.SizeField))
		if delta <= 0 {
			return e.Next()
		}
		if err := check(e.App, all, src, e.Record, delta, limits); err != nil {
			return err
		}
		return e.Next()
	})
}

func bindUpdate(app *pocketbase.PocketBase, all []Source, src Source, limits LimitsFunc) {
	app.OnRecordUpdate(src.Collection).BindFunc(func(e *core.RecordEvent) error {
		// Only the growth counts. Replacing a 10 MB file with a 12 MB one
		// consumes 2 MB, not 12 — charging the full size would refuse writes
		// that free space or leave it unchanged.
		newSize := int64(e.Record.GetInt(src.SizeField))
		oldSize := int64(e.Record.Original().GetInt(src.SizeField))
		delta := newSize - oldSize
		if delta <= 0 {
			return e.Next()
		}
		if err := check(e.App, all, src, e.Record, delta, limits); err != nil {
			return err
		}
		return e.Next()
	})
}

// check applies both ceilings to a pending write of `delta` bytes.
//
// The org ceiling is evaluated first: it is the one the operator sold, and
// reporting it is more actionable than telling a user they are personally out of
// space when the whole deployment is.
func check(app core.App, all []Source, src Source, record *core.Record, delta int64, limits LimitsFunc) error {
	lim := limits(app)
	if lim.PerOrg <= 0 && lim.PerUser <= 0 {
		return nil
	}

	if lim.PerOrg > 0 {
		used, err := OrgUsage(app, all)
		if err != nil {
			return err
		}
		if used+delta > lim.PerOrg {
			return apiErr(&ExceededError{
				Scope: "organization", Used: used, Limit: lim.PerOrg, Requested: delta,
			})
		}
	}

	// Per-user only applies to owned data; shared rows have no one to charge.
	if lim.PerUser > 0 && src.OwnerField != "" {
		owner := record.GetString(src.OwnerField)
		if owner == "" {
			return nil
		}
		used, err := UserUsage(app, all, owner)
		if err != nil {
			return err
		}
		if used+delta > lim.PerUser {
			return apiErr(&ExceededError{
				Scope: "user", Used: used, Limit: lim.PerUser, Requested: delta,
			})
		}
	}

	return nil
}

// apiErr surfaces a refusal as 413, which is the status both the REST API and
// x/net/webdav's PUT path map sensibly.
func apiErr(err *ExceededError) error {
	return router.NewApiError(http.StatusRequestEntityTooLarge, err.Error(), nil)
}
