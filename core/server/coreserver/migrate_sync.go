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
	down := registeredOnly(migrationsToRevert(applied, newSet))
	up := migrationsToApply(applied, newSet)

	var reverted []string
	if len(down) > 0 {
		r, err := revertNamedMigrations(app, down)
		if err != nil {
			return SyncResult{Reverted: r, Pending: up}, err
		}
		reverted = r
	}
	return SyncResult{Reverted: reverted, Pending: up}, nil
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
