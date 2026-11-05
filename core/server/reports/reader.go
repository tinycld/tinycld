package reports

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// FindViewableRecord returns the record of collection with id when the
// reader's view rule allows it. A record that does not exist, or that the
// reader may not view (a null rule included), returns sql.ErrNoRows, as
// PocketBase's view API answers 404 for both, so a caller cannot tell them
// apart. Any other error (a rule that cannot be built, a database fault) is
// returned as is.
func FindViewableRecord(app core.App, reader *core.RequestInfo, collection, id string) (*core.Record, error) {
	col, err := app.FindCachedCollectionByNameOrId(collection)
	if errors.Is(err, sql.ErrNoRows) {
		// PocketBase answers sql.ErrNoRows for an unknown collection too; a
		// caller would read that as a hidden record and silently fall back,
		// though it is a wiring mistake in the package.
		return nil, fmt.Errorf("reports: collection %q does not exist", collection)
	}
	if err != nil {
		return nil, err
	}
	rec, err := app.FindRecordById(col, id)
	if err != nil {
		return nil, err
	}
	ok, err := app.CanAccessRecord(rec, reader, col.ViewRule)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, sql.ErrNoRows
	}
	return rec, nil
}
