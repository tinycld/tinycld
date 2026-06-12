package coreserver

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Per-package, named-migration apply/revert.
//
// PocketBase's built-in `migrate` / `migrate down N` operate on the GLOBAL
// migration history by COUNT: `down N` reverses the last N applied migrations in
// reverse order, regardless of which package they belong to. That is exactly
// wrong for per-package version changes — every package's migrations interleave
// by timestamp in one _migrations table, so a count-based step would tear down
// unrelated packages' schema.
//
// Instead we drive a NAMED subset of migrations. core.AppMigrations holds every
// registered migration (Go and JS alike — the jsvm plugin registers each .js
// `migrate(up, down)` into the same global list as callable Go closures) as
// *core.Migration{File, Up, Down, ReapplyCondition}. We look each target file up
// by name, run its own Up/Down inside the same Aux+main transaction nesting the
// stock runner uses, and replicate the _migrations bookkeeping
// (insert {file, applied} / delete by file) ourselves. Because we only ever
// touch the named files for one package, no other package's history or schema is
// affected — there is no count, so there is no interleave hazard.
//
// See core/migrations_runner.go (Up/Down/saveAppliedMigration/saveRevertedMigration)
// in the pinned PocketBase for the patterns mirrored here.

// pkgMigrationByFile returns the registered migration with the given filename.
func pkgMigrationByFile(file string) (*core.Migration, bool) {
	for _, m := range core.AppMigrations.Items() {
		if m.File == file {
			return m, true
		}
	}
	return nil, false
}

// migrationApplied reports whether a migration filename is recorded in the
// _migrations history table.
func migrationApplied(app core.App, file string) (bool, error) {
	var exists int
	err := app.DB().Select("(1)").
		From(core.DefaultMigrationsTable).
		Where(dbx.HashExp{"file": file}).
		Limit(1).
		Row(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		// No matching row → not applied. This is the only error we treat as a
		// negative result.
		return false, nil
	}
	if err != nil {
		// A real failure (locked/closed DB, cancellation, bad SQL) must surface —
		// swallowing it would make a revert silently skip an applied migration or
		// make an apply double-run.
		return false, fmt.Errorf("check migration %s applied: %w", file, err)
	}
	return exists > 0, nil
}

// applyNamedMigrations runs the Up function of each given migration file that is
// not already applied, in ascending filename order, recording each in the
// _migrations table. It mirrors MigrationsRunner.Up but over a named subset:
// honoring ReapplyCondition and wrapping the whole batch in the aux+main
// transaction nesting so a failure rolls back both the schema change and the
// bookkeeping together. Files with no registered migration are an error — a
// version's recorded migration set must always be resolvable.
func applyNamedMigrations(app core.App, files []string) ([]string, error) {
	ordered := sortedCopy(files)
	applied := []string{}

	err := app.AuxRunInTransaction(func(txApp core.App) error {
		return txApp.RunInTransaction(func(txApp core.App) error {
			for _, file := range ordered {
				m, ok := pkgMigrationByFile(file)
				if !ok {
					return fmt.Errorf("apply: migration %q is not registered", file)
				}

				already, err := migrationApplied(txApp, file)
				if err != nil {
					return err
				}
				if already {
					if m.ReapplyCondition == nil {
						continue
					}
					shouldReapply, condErr := m.ReapplyCondition(txApp, nil, file)
					if condErr != nil {
						return condErr
					}
					if !shouldReapply {
						continue
					}
					if delErr := deleteMigrationRow(txApp, file); delErr != nil {
						return delErr
					}
				}

				if m.Up != nil {
					if upErr := m.Up(txApp); upErr != nil {
						return fmt.Errorf("apply migration %s: %w", file, upErr)
					}
				}
				if insErr := insertMigrationRow(txApp, file); insErr != nil {
					return insErr
				}
				applied = append(applied, file)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return applied, nil
}

// revertNamedMigrations runs the Down function of each given migration file that
// is currently applied, in DESCENDING filename order (newest first, so a column
// added by a later migration is dropped before the table an earlier one created),
// deleting each from the _migrations table. Mirrors MigrationsRunner.Down over a
// named subset. An applied file with no registered migration is an error — we
// never silently drop a history row while leaving its schema behind.
func revertNamedMigrations(app core.App, files []string) ([]string, error) {
	ordered := sortedCopy(files)
	// descending: revert newest first
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}
	reverted := []string{}

	err := app.AuxRunInTransaction(func(txApp core.App) error {
		return txApp.RunInTransaction(func(txApp core.App) error {
			for _, file := range ordered {
				applied, err := migrationApplied(txApp, file)
				if err != nil {
					return err
				}
				if !applied {
					continue // nothing to revert for this file
				}
				m, ok := pkgMigrationByFile(file)
				if !ok {
					return fmt.Errorf("revert: migration %q is not registered", file)
				}
				if m.Down != nil {
					if downErr := m.Down(txApp); downErr != nil {
						return fmt.Errorf("revert migration %s: %w", file, downErr)
					}
				}
				if delErr := deleteMigrationRow(txApp, file); delErr != nil {
					return delErr
				}
				reverted = append(reverted, file)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return reverted, nil
}

// ---------- _migrations bookkeeping (replicates the stock runner) ----------

func insertMigrationRow(txApp core.App, file string) error {
	_, err := txApp.DB().Insert(core.DefaultMigrationsTable, dbx.Params{
		"file":    file,
		"applied": time.Now().UnixMicro(),
	}).Execute()
	if err != nil {
		return fmt.Errorf("record applied migration %s: %w", file, err)
	}
	return nil
}

func deleteMigrationRow(txApp core.App, file string) error {
	_, err := txApp.DB().Delete(core.DefaultMigrationsTable, dbx.HashExp{"file": file}).Execute()
	if err != nil {
		return fmt.Errorf("record reverted migration %s: %w", file, err)
	}
	return nil
}

func sortedCopy(files []string) []string {
	out := make([]string, len(files))
	copy(out, files)
	sort.Strings(out)
	return out
}

// ---------- downgrade drop report ----------

// DropReport describes the schema a downgrade would destroy, so the UI can warn
// the operator before they confirm. droppedCollections are collections that
// exist now but would not after the revert; droppedFields are fields removed
// from collections that survive.
type DropReport struct {
	DroppedCollections []string       `json:"droppedCollections"`
	DroppedFields      []DroppedField `json:"droppedFields"`
}

type DroppedField struct {
	Collection string `json:"collection"`
	Field      string `json:"field"`
}

// errDryRollback is the sentinel that unwinds the dry-run transaction after the
// schema has been stepped down and snapshotted, so the revert is never committed.
var errDryRollback = errors.New("pkg_migrate: dry-run rollback")

// dryRevertNamedMigrations reports what a real revert of the given files would
// drop, WITHOUT committing anything. It snapshots the current collection/field
// shape, runs the Down functions inside a transaction, snapshots again, diffs,
// then forces the transaction to roll back via errDryRollback. The live schema
// is unchanged on return.
//
// Diffing the whole collection set (rather than only slug-prefixed collections)
// keeps the report correct for packages whose collections don't share a naming
// prefix — we compare the actual before/after worlds.
func dryRevertNamedMigrations(app core.App, files []string) (DropReport, error) {
	var report DropReport

	ordered := sortedCopy(files)
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}

	txErr := app.AuxRunInTransaction(func(txApp core.App) error {
		return txApp.RunInTransaction(func(txApp core.App) error {
			before, err := collectionFieldSnapshot(txApp)
			if err != nil {
				return err
			}

			for _, file := range ordered {
				applied, err := migrationApplied(txApp, file)
				if err != nil {
					return err
				}
				if !applied {
					continue
				}
				m, ok := pkgMigrationByFile(file)
				if !ok {
					return fmt.Errorf("dry-revert: migration %q is not registered", file)
				}
				if m.Down != nil {
					if downErr := m.Down(txApp); downErr != nil {
						return fmt.Errorf("dry-revert migration %s: %w", file, downErr)
					}
				}
			}

			after, err := collectionFieldSnapshot(txApp)
			if err != nil {
				return err
			}
			report = diffSnapshots(before, after)

			// Never commit a dry run.
			return errDryRollback
		})
	})
	if txErr != nil && !errors.Is(txErr, errDryRollback) {
		return DropReport{}, txErr
	}
	return report, nil
}

// collectionFieldSnapshot maps each base/auth/view collection name to the set of
// its field names. System collections (e.g. _migrations, _superusers) are
// excluded — a package downgrade is never expected to touch them and they would
// only add noise to the report.
func collectionFieldSnapshot(app core.App) (map[string]map[string]bool, error) {
	cols, err := app.FindAllCollections()
	if err != nil {
		return nil, err
	}
	snap := make(map[string]map[string]bool, len(cols))
	for _, c := range cols {
		if c.System {
			continue
		}
		fields := make(map[string]bool, len(c.Fields))
		for _, f := range c.Fields {
			fields[f.GetName()] = true
		}
		snap[c.Name] = fields
	}
	return snap, nil
}

// diffSnapshots reports what exists in before but not after: whole collections
// gone, and fields gone from surviving collections.
func diffSnapshots(before, after map[string]map[string]bool) DropReport {
	report := DropReport{
		DroppedCollections: []string{},
		DroppedFields:      []DroppedField{},
	}
	beforeNames := make([]string, 0, len(before))
	for name := range before {
		beforeNames = append(beforeNames, name)
	}
	sort.Strings(beforeNames)

	for _, name := range beforeNames {
		afterFields, stillExists := after[name]
		if !stillExists {
			report.DroppedCollections = append(report.DroppedCollections, name)
			continue
		}
		fieldNames := make([]string, 0, len(before[name]))
		for f := range before[name] {
			fieldNames = append(fieldNames, f)
		}
		sort.Strings(fieldNames)
		for _, f := range fieldNames {
			if !afterFields[f] {
				report.DroppedFields = append(report.DroppedFields, DroppedField{Collection: name, Field: f})
			}
		}
	}
	return report
}
