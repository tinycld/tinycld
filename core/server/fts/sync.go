package fts

import (
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("fts")

// RegisterSync binds the FTS index-sync record hooks for one collection. On
// every create/update it does an idempotent delete-then-insert; on delete it
// removes the row. Core registers this itself (from the manifest `fts` block) so
// the sync never depends on tenant TS and can't be skipped.
func RegisterSync(app *pocketbase.PocketBase, cfg Config) {
	app.OnRecordAfterCreateSuccess(cfg.Collection).BindFunc(func(e *core.RecordEvent) error {
		syncRecord(app, cfg, e.Record, false)
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess(cfg.Collection).BindFunc(func(e *core.RecordEvent) error {
		syncRecord(app, cfg, e.Record, false)
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess(cfg.Collection).BindFunc(func(e *core.RecordEvent) error {
		syncRecord(app, cfg, e.Record, true)
		return e.Next()
	})
}

// syncRecord upserts (or, when remove is true, deletes) a single record's FTS
// row. Failures are logged, not returned: an index hiccup must not fail the
// underlying write (the after-success hook has already committed the record).
func syncRecord(app *pocketbase.PocketBase, cfg Config, record *core.Record, remove bool) {
	db := app.NonconcurrentDB()

	// Always delete first — makes create/update idempotent and handles delete.
	if _, err := db.NewQuery("DELETE FROM " + cfg.Table + " WHERE record_id = {:id}").
		Bind(map[string]any{"id": record.Id}).Execute(); err != nil {
		log.Warn("delete from index failed",
			"table", cfg.Table, "recordID", record.Id, "err", err)
	}

	if remove {
		return
	}

	cols := make([]string, 0, len(cfg.Columns)+1)
	placeholders := make([]string, 0, len(cfg.Columns)+1)
	params := map[string]any{"record_id": record.Id}
	cols = append(cols, "record_id")
	placeholders = append(placeholders, "{:record_id}")

	for _, c := range cfg.Columns {
		val := record.GetString(c.Field)
		if c.Strip {
			val = stripHTML(val)
		}
		cols = append(cols, c.FTS)
		placeholders = append(placeholders, "{:"+c.FTS+"}")
		params[c.FTS] = val
	}

	q := "INSERT INTO " + cfg.Table +
		" (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	if _, err := db.NewQuery(q).Bind(params).Execute(); err != nil {
		log.Warn("index insert failed",
			"table", cfg.Table, "recordID", record.Id, "err", err)
	}
}
