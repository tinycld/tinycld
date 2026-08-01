package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestTenantContext_RoundTripAndAbsence(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if IsTenant(app) {
		t.Fatal("a bare app must not read as a tenant")
	}
	if _, ok := GetTenantContext(app); ok {
		t.Fatal("GetTenantContext on a bare app must report absence")
	}

	setTenantContext(app, TenantContext{Slug: "acme", ControlSocket: "/run/ctl.sock"})

	if !IsTenant(app) {
		t.Fatal("IsTenant must be true after the stamp")
	}
	tc, ok := GetTenantContext(app)
	if !ok || tc.Slug != "acme" || tc.ControlSocket != "/run/ctl.sock" {
		t.Fatalf("GetTenantContext = %+v, %v", tc, ok)
	}
}
