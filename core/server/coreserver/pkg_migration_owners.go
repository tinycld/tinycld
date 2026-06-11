package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Migration → owning-package attribution.
//
// The generator (app/scripts/generate.ts symlinkServerArtifacts) flattens every
// package's pb-migrations/ into one directory and emits pb_migrations_owner.json
// recording each migration filename's owning package slug (core migrations are
// owned by 'core'). The per-package named-migration runner (pkg_migrate.go) uses
// this to resolve "which migration files belong to package X" so it can apply or
// revert just one package's schema without touching any other's.

const migrationOwnersFile = "pb_migrations_owner.json"

var (
	migrationOwnersMu     sync.Mutex
	migrationOwnersMap    map[string]string // file → slug
	migrationOwnersLoaded bool
)

// loadMigrationOwners reads and caches the file→slug map. A missing file yields
// an empty map (dev layouts / partial assemblies); callers degrade gracefully.
// The cache is reloadable via resetMigrationOwnersCache — the version-change
// pipeline regenerates the map mid-run, so a one-shot cache would go stale.
func loadMigrationOwners() map[string]string {
	migrationOwnersMu.Lock()
	defer migrationOwnersMu.Unlock()
	if migrationOwnersLoaded {
		return migrationOwnersMap
	}
	migrationOwnersMap = map[string]string{}
	migrationOwnersLoaded = true
	path := findMigrationOwnersJSON()
	if path == "" {
		return migrationOwnersMap
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return migrationOwnersMap
	}
	if parsed, ok := parseMigrationOwners(data); ok {
		migrationOwnersMap = parsed
	}
	return migrationOwnersMap
}

// resetMigrationOwnersCache forces the next loadMigrationOwners to re-read the
// owner map from disk. Called after the generator rewrites it during a version
// change so subsequent migrationsForPackage lookups reflect the new file set.

// parseMigrationOwners decodes the file→slug JSON, returning ok=false on invalid
// JSON. Pure (no I/O) so the query helpers can be exercised in tests.
func parseMigrationOwners(data []byte) (map[string]string, bool) {
	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

// queryMigrationsForPackage / queryPackageForMigration are the pure cores of the
// exported helpers, taking an explicit owner map so tests don't depend on the
// process-global cache.
func queryMigrationsForPackage(owners map[string]string, slug string) []string {
	out := make([]string, 0)
	for file, owner := range owners {
		if owner == slug {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

func findMigrationOwnersJSON() string {
	// The generator writes the owner map to the Go server dir, which is
	// <appDir>/server/ (SERVER_DIR in scripts/paths.ts). Post-merge the running
	// binary lives at <appDir>/tinycld, so binaryDir()/resolveServerDir() == appDir
	// and the file is one level deeper at <appDir>/server/. Earlier candidates
	// assumed the OLD layout (binary at app/server/, cwd app/server) where
	// `../server` or the binary dir itself held the file; keep them for back-compat
	// but ALSO check <appDir>/server/ and <cwd>/server/ for the merged layout.
	binDir := filepath.Dir(os.Args[0])
	srvDir := resolveServerDir()
	candidates := []string{
		migrationOwnersFile,                                  // <cwd>/
		filepath.Join("server", migrationOwnersFile),         // <cwd>/server/ (merged: cwd==appDir)
		filepath.Join("..", "server", migrationOwnersFile),   // <cwd>/../server/ (old layout)
		filepath.Join(binDir, migrationOwnersFile),           // <binaryDir>/
		filepath.Join(binDir, "server", migrationOwnersFile), // <binaryDir>/server/ (merged)
		filepath.Join(srvDir, migrationOwnersFile),           // resolveServerDir()/
		filepath.Join(srvDir, "server", migrationOwnersFile), // resolveServerDir()/server/ (merged)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// packageForMigration returns the owning package slug for a migration filename,
// or "" if unknown.

// migrationsForPackage returns the migration filenames owned by the given slug,
// sorted ascending (timestamp order). Empty if the slug owns none or the map is
// unavailable.
func migrationsForPackage(slug string) []string {
	return queryMigrationsForPackage(loadMigrationOwners(), slug)
}
