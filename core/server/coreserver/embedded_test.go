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
