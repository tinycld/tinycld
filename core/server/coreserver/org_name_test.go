package coreserver

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestSetOrgName(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	if err := setOrgName(app, "  Harbor Dental  "); err != nil {
		t.Fatal(err)
	}
	if got := app.Settings().Meta.AppName; got != "Harbor Dental" {
		t.Fatalf("AppName = %q", got)
	}
	if err := setOrgName(app, "   "); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := setOrgName(app, strings.Repeat("x", 256)); err == nil {
		t.Fatal("256-char name accepted")
	}
}
