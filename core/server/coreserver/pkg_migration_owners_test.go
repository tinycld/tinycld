package coreserver

import (
	"reflect"
	"testing"
)

func TestParseMigrationOwners(t *testing.T) {
	data := []byte(`{
		"1700000000_create_core.js": "core",
		"1713000000_create_gizmos.js": "gizmos",
		"1713000001_add_gizmos_field.js": "gizmos",
		"1716000000_create_cogs.js": "cogs"
	}`)
	owners, ok := parseMigrationOwners(data)
	if !ok {
		t.Fatal("parseMigrationOwners returned ok=false for valid JSON")
	}
	if owners["1713000001_add_gizmos_field.js"] != "gizmos" {
		t.Errorf("gizmos field migration owner = %q, want gizmos", owners["1713000001_add_gizmos_field.js"])
	}

	if _, ok := parseMigrationOwners([]byte("not json")); ok {
		t.Error("parseMigrationOwners returned ok=true for invalid JSON")
	}
}

func TestQueryMigrationsForPackage(t *testing.T) {
	owners := map[string]string{
		"1713000000_create_gizmos.js":    "gizmos",
		"1713000001_add_gizmos_field.js": "gizmos",
		"1716000000_create_cogs.js":      "cogs",
		"1700000000_create_core.js":      "core",
	}

	gizmos := queryMigrationsForPackage(owners, "gizmos")
	want := []string{"1713000000_create_gizmos.js", "1713000001_add_gizmos_field.js"}
	if !reflect.DeepEqual(gizmos, want) {
		t.Errorf("gizmos migrations = %v, want %v (sorted ascending)", gizmos, want)
	}

	if got := queryMigrationsForPackage(owners, "nonexistent"); len(got) != 0 {
		t.Errorf("unknown slug returned %v, want empty", got)
	}
}
