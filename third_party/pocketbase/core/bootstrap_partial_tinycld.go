package core

import (
	"errors"

	"github.com/pocketbase/dbx"
)

// Fork: a failed Bootstrap leaves no database pool open.
//
// Bootstrap opens four pools: the data pools, then the auxiliary pools. When
// an open fails after an earlier one succeeded, upstream keeps the pools that
// opened. ClearBootstrap returned at once, because IsBootstrapped needs both
// the data and the auxiliary pool, and a pool that opened as the first of a
// pair was never stored at all. A process that opens and closes many apps
// leaked a set of pools for each failed open. See FORK.md.

// closePartialBootstrap closes the pools that a failed Bootstrap stored on
// app. No OnBootstrapClear hook runs: the app never finished a bootstrap.
func (app *BaseApp) closePartialBootstrap() error {
	var errs []error
	for _, db := range []*dbx.Builder{
		&app.concurrentDB,
		&app.nonconcurrentDB,
		&app.auxConcurrentDB,
		&app.auxNonconcurrentDB,
	} {
		if *db == nil {
			continue
		}
		if c, ok := (*db).(interface{ Close() error }); ok {
			if err := c.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		*db = nil
	}
	return errors.Join(errs...)
}

// closeOpened closes the pools that opened before err, and returns err, so a
// failed pool init leaves none of its pools open.
func closeOpened(err error, opened ...*dbx.DB) error {
	for _, db := range opened {
		_ = db.Close()
	}
	return err
}
