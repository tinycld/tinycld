package coreserver

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
