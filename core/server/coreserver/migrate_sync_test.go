package coreserver

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMigrationsToApply(t *testing.T) {
	applied := []string{"100_a.js", "200_b.js"}
	newSet := []string{"100_a.js", "200_b.js", "300_c.js", "400_d.js"}
	got := migrationsToApply(applied, newSet)
	want := []string{"300_c.js", "400_d.js"} // oldest-first
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migrationsToApply = %v, want %v", got, want)
	}
}

func TestMigrationsToRevert(t *testing.T) {
	applied := []string{"100_a.js", "200_b.js", "300_c.js"}
	newSet := []string{"100_a.js"}
	got := migrationsToRevert(applied, newSet)
	want := []string{"300_c.js", "200_b.js"} // newest-first (reverse)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migrationsToRevert = %v, want %v", got, want)
	}
}

func TestMigrationDiff_NoChange(t *testing.T) {
	set := []string{"100_a.js", "200_b.js"}
	if got := migrationsToApply(set, set); len(got) != 0 {
		t.Fatalf("expected no applies, got %v", got)
	}
	if got := migrationsToRevert(set, set); len(got) != 0 {
		t.Fatalf("expected no reverts, got %v", got)
	}
}

func TestBuildMigrationFiles(t *testing.T) {
	dir := t.TempDir()
	mig := filepath.Join(dir, "tinycld", "server", "pb_migrations")
	if err := os.MkdirAll(mig, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"200_b.js", "100_a.js", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(mig, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := buildMigrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"100_a.js", "200_b.js"} // sorted, .js only
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMigrationFiles = %v, want %v", got, want)
	}
}
