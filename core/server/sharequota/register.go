package sharequota

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// BindMeters binds the file-download hook for every registered collection.
//
// The hook is TAGGED by collection, so a package's meter is only ever invoked
// for that package's files and core compares no names to anything. That is
// what keeps this file free of any package's vocabulary — the tag does the
// dispatch that a switch statement would otherwise have to do here.
//
// Core binds rather than the package, for the reason quota's hooks are bound
// the same way: a composition may link a feature whose own registration ran
// somewhere core cannot see, and one place that binds them all is easier to
// reason about than N places that each might not have.
//
// The hook fires AFTER the file route has already checked the collection's
// view rule and BEFORE the bytes are served, which is the only window where a
// refusal both knows the caller was allowed in and can still refuse cheaply.
//
// A meter that returns an error refuses the download. A meter that cannot tell
// which link to charge must serve it: this hook meters, it never authorizes.
// The access decision was made before it ran.
func BindMeters(app *pocketbase.PocketBase) {
	for _, c := range RegisteredMetered() {
		c := c
		app.OnFileDownloadRequest(c.Collection).BindFunc(func(e *core.FileDownloadRequestEvent) error {
			return c.Meter(e.App, e)
		})
	}
}
