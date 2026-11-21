package reports

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// beginRead opens the one read transaction a run reads its statements in, so
// they all see one snapshot of the database: a write saved between two of
// them is in neither. It opens on the concurrent (read) pool, never the
// single writer pool, so a run does not queue behind writes. When the app
// already runs in a transaction (a txApp), the run reads in that transaction,
// and end leaves it open for its owner. end rolls the read transaction back;
// a run calls it on every return, or the transaction keeps the WAL from
// resetting.
func beginRead(ctx context.Context, app core.App) (db dbx.Builder, end func(), err error) {
	switch pool := app.ConcurrentDB().(type) {
	case *dbx.Tx:
		return pool, func() {}, nil
	case *dbx.DB:
		tx, err := pool.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, nil, mapRunError(err)
		}
		end := func() {
			// The transaction only read, so a failed rollback loses no
			// write. ErrTxDone: database/sql already rolled it back when
			// the run's context ended.
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.WarnContext(ctx, "ending a run's read transaction", "error", err)
			}
		}
		return tx, end, nil
	}
	return nil, nil, fmt.Errorf("reports: the read pool is a %T, not a database or a transaction", app.ConcurrentDB())
}
