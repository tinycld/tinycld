package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestEmbeddedContext_RoundTripAndAbsence(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if _, ok := GetEmbeddedContext(app); ok {
		t.Fatal("GetEmbeddedContext on a bare app must report absence")
	}

	SetEmbeddedContext(app, EmbeddedContext{InstanceID: "acme", ControlSocket: "/run/ctl.sock"})

	ec, ok := GetEmbeddedContext(app)
	if !ok || ec.InstanceID != "acme" || ec.ControlSocket != "/run/ctl.sock" {
		t.Fatalf("GetEmbeddedContext = %+v, %v", ec, ok)
	}
}

// The deprecated aliases must read what the new names stamped — they are the
// same store entry, not a parallel one, which is what lets mail and
// hosting/limits migrate on their own commits rather than in lockstep.
func TestTenantContextAlias_SeesTheEmbeddedStamp(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	SetEmbeddedContext(app, EmbeddedContext{InstanceID: "acme"})

	tc, ok := GetTenantContext(app)
	if !ok || tc.InstanceID != "acme" {
		t.Fatalf("GetTenantContext = %+v, %v", tc, ok)
	}
}
