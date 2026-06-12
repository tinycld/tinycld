package coreserver

import (
	"reflect"
	"testing"
)

func TestParseMigrationOwners(t *testing.T) {
	data := []byte(`{
		"1700000000_create_core.js": "core",
		"1713000000_create_mail.js": "mail",
		"1713000001_add_mail_field.js": "mail",
		"1716000000_create_drive.js": "drive"
	}`)
	owners, ok := parseMigrationOwners(data)
	if !ok {
		t.Fatal("parseMigrationOwners returned ok=false for valid JSON")
	}
	if owners["1713000001_add_mail_field.js"] != "mail" {
		t.Errorf("mail field migration owner = %q, want mail", owners["1713000001_add_mail_field.js"])
	}

	if _, ok := parseMigrationOwners([]byte("not json")); ok {
		t.Error("parseMigrationOwners returned ok=true for invalid JSON")
	}
}

func TestQueryMigrationsForPackage(t *testing.T) {
	owners := map[string]string{
		"1713000000_create_mail.js":    "mail",
		"1713000001_add_mail_field.js": "mail",
		"1716000000_create_drive.js":   "drive",
		"1700000000_create_core.js":    "core",
	}

	mail := queryMigrationsForPackage(owners, "mail")
	want := []string{"1713000000_create_mail.js", "1713000001_add_mail_field.js"}
	if !reflect.DeepEqual(mail, want) {
		t.Errorf("mail migrations = %v, want %v (sorted ascending)", mail, want)
	}

	if got := queryMigrationsForPackage(owners, "nonexistent"); len(got) != 0 {
		t.Errorf("unknown slug returned %v, want empty", got)
	}
}
