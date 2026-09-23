package quota

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("quota")

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
			return apiErr(src.Collection, record.GetString(src.OwnerField), &ExceededError{
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
			return apiErr(src.Collection, owner, &ExceededError{
				Scope: "user", Used: used, Limit: lim.PerUser, Requested: delta,
			})
		}
	}

	return nil
}

// apiErr surfaces a refusal as 413, which is the status both the REST API and
// x/net/webdav's PUT path map sensibly.
//
// It also records the refusal. A refusal is the clearest abuse signal there
// is — an account repeatedly hitting a ceiling looks nothing like one working
// normally — and until now it was returned to the caller and forgotten, so
// there was no way to answer "who has been hammering this limit?" after the
// fact. Warn (not Info) so it reaches Sentry, where a spike is visible
// without anyone querying _logs.
//
// The enforcement hooks are OnRecordCreate/OnRecordUpdate, not their
// ...Request variants — that is what makes the ceiling unskippable, and it is
// also why there is no request here to name the acting user or IP. Owner is
// the best attribution available; for a shared row there is none.
func apiErr(collection string, owner string, err *ExceededError) error {
	log.Warn("quota refused a write",
		"collection", collection,
		"scope", err.Scope,
		"owner", owner,
		"used", err.Used,
		"limit", err.Limit,
		"requested", err.Requested)
	return router.NewApiError(http.StatusRequestEntityTooLarge, err.Error(), nil)
}
