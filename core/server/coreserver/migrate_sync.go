package coreserver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// SyncResult records what a migration sync did.
type SyncResult struct {
	Reverted []string // DOWN migrations run against the outgoing binary
	Pending  []string // UP migrations the NEW binary will apply on boot
	// SkippedUnregistered are migrations the diff flagged for DOWN but that aren't
	// registered in the running binary's core.AppMigrations, so they could NOT be
	// reverted (their Down never ran). For core's compiled-in Go migrations this is
	// expected; for a uninstalled PACKAGE's JS migrations it means its tables were
	// left behind — the symptom this surfaces. Logged for diagnosis.
	SkippedUnregistered []string
}

// logSyncResult records the migration diff into the durable job log: exactly
// which migrations were reverted (DOWN, run now against the outgoing binary) and
// which are pending (UP, applied by the new binary on its post-swap boot). When a
// schema-state bug appears post-upgrade, this is the line that says what the
// rebuild changed about the schema.
func logSyncResult(job *installJob, res SyncResult) {
	// Surface skipped (unregistered) DOWN candidates regardless of the rest: for an
	// uninstall these are the dropped package's migrations whose Down never ran, so
	// its tables/data persist — the cause of "I uninstalled it but the tables are
	// still there." Expected for core's compiled-in Go migrations; a problem for a
	// package's JS migrations.
	if len(res.SkippedUnregistered) > 0 {
		jobLogf(job, "migrations SKIPPED (DOWN candidate but not registered in the running binary, %d): %s — their schema is NOT reverted",
			len(res.SkippedUnregistered), strings.Join(res.SkippedUnregistered, ", "))
	}
	switch {
	case len(res.Reverted) == 0 && len(res.Pending) == 0:
		jobLogf(job, "migrations: no schema change (applied set matches the build)")
		return
	default:
		if len(res.Reverted) > 0 {
			jobLogf(job, "migrations DOWN (reverted now, %d): %s", len(res.Reverted), strings.Join(res.Reverted, ", "))
		}
		if len(res.Pending) > 0 {
			jobLogf(job, "migrations UP (applied by the new binary on boot, %d): %s", len(res.Pending), strings.Join(res.Pending, ", "))
		}
	}
}

// syncMigrations brings the live DB toward newSet by running DOWN for every
// migration the new build drops. UP migrations (present in newSet, not yet
// applied) are NOT run here — the freshly-built binary applies them on its
// post-swap boot (PocketBase auto-migrates on start). They are returned in
// Pending for logging/verification.
//
// applied is the current _migrations file set; newSet is the build's set
// (buildMigrationFiles). DOWN runs newest-first; the caller must have taken a
// pb_data backup first so a failure can be rolled back.
//
// The DOWN set is filtered to migrations REGISTERED in the currently-running
// binary (pkgMigrationByFile). The applied set legitimately contains migrations
// this binary can't revert — core's compiled-in Go migrations (e.g.
// normalize_indexes.go) live in core.AppMigrations but are NOT files in
// pb_migrations/, so a raw applied−newSet diff would flag every Go migration as
// "dropped." Those persist across every build and are never reverted by a
// package operation; a file-based newSet can't list them, so we exclude any
// unregistered file from DOWN rather than error on it. (A genuine package
// downgrade's reverted migrations ARE registered — the running binary still has
// their Down closures — so they pass the filter.)
func syncMigrations(app core.App, applied, newSet []string) (SyncResult, error) {
	// A real build always carries core's migrations, so an empty newSet means the
	// build dir's pb_migrations wasn't populated (generator didn't run / wrong
	// path). Treat it as a build failure — NOT a signal to revert every applied
	// migration, which would tear down the whole schema.
	if len(applied) > 0 && len(newSet) == 0 {
		return SyncResult{}, fmt.Errorf("new build carries no migrations (empty pb_migrations) — refusing to revert %d applied migrations", len(applied))
	}
	candidates := migrationsToRevert(applied, newSet)
	down := registeredOnly(candidates)
	skipped := unregisteredOnly(candidates)
	up := migrationsToApply(applied, newSet)

	var reverted []string
	if len(down) > 0 {
		r, err := revertNamedMigrations(app, down)
		if err != nil {
			return SyncResult{Reverted: r, Pending: up, SkippedUnregistered: skipped}, err
		}
		reverted = r
	}
	return SyncResult{Reverted: reverted, Pending: up, SkippedUnregistered: skipped}, nil
}

// purgeUnregisteredPackageRows deletes the _migrations history rows for an
// uninstalled package's migrations that syncMigrations could NOT revert because
// they weren't registered in the running binary (the SkippedUnregistered set).
//
// Why this is needed: an uninstall runs DOWN only for migrations still registered
// in core.AppMigrations (registeredOnly). When the active build no longer carries
// a package's JS migrations (e.g. after a prior failed-install rollback), they're
// unregistered, so their Down is skipped AND their _migrations rows are left
// behind. A later REINSTALL then sees those rows as "already applied" and skips
// the Up entirely — so the package's tables are never (re)created even though the
// install reports success. Clearing the stranded rows lets a reinstall re-run the
// Up cleanly. We only touch rows OWNED by the uninstalled slug (per the migration
// owner map) AND in the skipped set, so no other package's (or core's) history is
// affected. Schema the skipped Down would have dropped may persist (we have no
// Down to run) — that's logged separately; the row purge is what unblocks
// reinstall, which is the reported symptom.
func purgeUnregisteredPackageRows(app core.App, slug string, skipped []string) ([]string, error) {
	if slug == "" || len(skipped) == 0 {
		return nil, nil
	}
	owned := make(map[string]bool)
	for _, f := range migrationsForPackage(slug) {
		owned[f] = true
	}
	purged := []string{}
	for _, f := range skipped {
		// Prefer the authoritative owner map; fall back to a slug-substring match on
		// the filename when the map is stale/absent (e.g. the active build already
		// dropped this package), so a deeply-stranded row is still recoverable. The
		// convention is `<ts>_<...slug...>.js`, so requiring the slug in the name
		// avoids touching unrelated files.
		if !owned[f] && !strings.Contains(f, slug) {
			continue
		}
		applied, err := migrationApplied(app, f)
		if err != nil {
			return purged, err
		}
		if !applied {
			continue
		}
		if err := deleteMigrationRow(app, f); err != nil {
			return purged, err
		}
		purged = append(purged, f)
	}
	sort.Strings(purged)
	return purged, nil
}

// unregisteredOnly is the complement of registeredOnly: the files NOT resolvable
// in the running binary's core.AppMigrations. For an uninstall these are the
// dropped package's migrations whose Down can't run — diagnostic only.
func unregisteredOnly(files []string) []string {
	var out []string
	for _, f := range files {
		if _, ok := pkgMigrationByFile(f); !ok {
			out = append(out, f)
		}
	}
	return out
}

// registeredOnly keeps only the migration files registered in the running
// binary (core.AppMigrations via pkgMigrationByFile). Unregistered files can't
// be reverted by this binary and are not this operation's concern.
func registeredOnly(files []string) []string {
	out := files[:0:0]
	for _, f := range files {
		if _, ok := pkgMigrationByFile(f); ok {
			out = append(out, f)
		}
	}
	return out
}

// buildMigrationFiles returns the sorted *.js migration filenames a built
// workspace carries, read from <buildDir>/tinycld/server/pb_migrations.
func buildMigrationFiles(buildDir string) ([]string, error) {
	migDir := filepath.Join(buildDir, "tinycld", "server", "pb_migrations")
	entries, err := os.ReadDir(migDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// migrationsToApply returns files present in newSet but not in applied,
// sorted ascending (oldest-first) so UP migrations run in timestamp order.
func migrationsToApply(applied, newSet []string) []string {
	have := make(map[string]bool, len(applied))
	for _, f := range applied {
		have[f] = true
	}
	var out []string
	for _, f := range newSet {
		if !have[f] {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// migrationsToRevert returns files present in applied but absent from newSet,
// sorted descending (newest-first) so DOWN migrations tear down in reverse
// dependency order.
func migrationsToRevert(applied, newSet []string) []string {
	keep := make(map[string]bool, len(newSet))
	for _, f := range newSet {
		keep[f] = true
	}
	var out []string
	for _, f := range applied {
		if !keep[f] {
			out = append(out, f)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}
