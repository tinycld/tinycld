package coreserver

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestWriteRetryBindsAndWrapsTheServerAddress(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	registerWriteRetry(app)

	e := &core.ServeEvent{App: app, Server: &http.Server{Addr: "127.0.0.1:0"}}
	if err := app.OnServe().Trigger(e, func(*core.ServeEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if e.Listener == nil {
		t.Fatal("OnServe left no listener")
	}
	defer e.Listener.Close()
	if _, ok := e.Listener.(*writeRetryListener); !ok {
		t.Fatalf("e.Listener is %T, want a *writeRetryListener", e.Listener)
	}
}

func TestWriteRetryLeavesADefaultAddressToPocketBase(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	registerWriteRetry(app)

	e := &core.ServeEvent{App: app, Server: &http.Server{}}
	if err := app.OnServe().Trigger(e, func(*core.ServeEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if e.Listener != nil {
		t.Fatalf("e.Listener = %v, want nil so PocketBase binds its default address", e.Listener)
	}
}
