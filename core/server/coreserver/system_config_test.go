package coreserver

import (
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// createSystemSettingsCollection mirrors the runtime shape of
// pb_migrations/1910000010_create_system_settings.js for unit tests (the JS
// migration's env-seed path is exercised end-to-end by the export-types replay).
func createSystemSettingsCollection(t *testing.T, app core.App) {
	t.Helper()
	c := core.NewBaseCollection("system_settings")
	c.Id = "pbc_system_settings"
	c.Fields.Add(&core.TextField{Name: "key", Required: true, Max: 100})
	c.Fields.Add(&core.TextField{Name: "value", Max: 5000})
	c.Fields.Add(&core.BoolField{Name: "is_secret"})
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	c.AddIndex("idx_system_settings_key", true, "key", "")
	if err := app.Save(c); err != nil {
		t.Fatalf("save system_settings collection: %v", err)
	}
}

func saveSetting(t *testing.T, app core.App, key, value string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("system_settings")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("key", key)
	rec.Set("value", value)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save setting %q: %v", key, err)
	}
	return rec
}

// Get returns "" before anything is loaded, and the stored value after load().
func TestSystemConfigLoadAndGet(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	createSystemSettingsCollection(t, app)
	saveSetting(t, app, "sentry.dsn", "https://example.dsn")

	cfg := &SystemConfig{values: map[string]string{}}
	if got := cfg.Get("sentry.dsn"); got != "" {
		t.Errorf("before load: want empty, got %q", got)
	}
	cfg.load(app)
	if got := cfg.Get("sentry.dsn"); got != "https://example.dsn" {
		t.Errorf("after load: want stored value, got %q", got)
	}
	if got := cfg.Get("missing.key"); got != "" {
		t.Errorf("unset key: want empty, got %q", got)
	}
}

// set updates the value live and fires OnChange callbacks (so a stateful consumer
// like Sentry can re-init without a restart).
func TestSystemConfigSetFiresOnChange(t *testing.T) {
	cfg := &SystemConfig{values: map[string]string{}}

	var (
		mu      sync.Mutex
		gotKey  string
		gotVal  string
		fireCnt int
	)
	cfg.OnChange(func(key, value string) {
		mu.Lock()
		defer mu.Unlock()
		gotKey, gotVal, fireCnt = key, value, fireCnt+1
	})

	cfg.set("sentry.dsn", "https://new.dsn")

	if got := cfg.Get("sentry.dsn"); got != "https://new.dsn" {
		t.Errorf("set did not update value, got %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if fireCnt != 1 || gotKey != "sentry.dsn" || gotVal != "https://new.dsn" {
		t.Errorf("OnChange fired wrong: cnt=%d key=%q val=%q", fireCnt, gotKey, gotVal)
	}
}

// An OnChange handler may call Get without deadlocking (callbacks run outside the
// lock) — this is exactly what reinitSentry will do.
func TestSystemConfigOnChangeCanRead(t *testing.T) {
	cfg := &SystemConfig{values: map[string]string{}}
	var observed string
	cfg.OnChange(func(key, _ string) {
		observed = cfg.Get(key) // must not deadlock
	})
	cfg.set("sentry.dsn", "https://read.dsn")
	if observed != "https://read.dsn" {
		t.Errorf("OnChange could not read updated value, got %q", observed)
	}
}

// The record hooks wired by RegisterSystemConfig keep the in-memory map in sync
// when a system_settings row is created or updated.
func TestSystemConfigRecordHooksSync(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	createSystemSettingsCollection(t, app)

	// Point the package-global at a fresh instance and wire the hooks to it.
	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}}

	syncRow := func(e *core.RecordEvent) error {
		systemConfig.set(e.Record.GetString("key"), e.Record.GetString("value"))
		return e.Next()
	}
	app.OnRecordAfterCreateSuccess("system_settings").BindFunc(syncRow)
	app.OnRecordAfterUpdateSuccess("system_settings").BindFunc(syncRow)

	rec := saveSetting(t, app, "vapid.subject", "mailto:admin@example.com")
	if got := systemConfig.Get("vapid.subject"); got != "mailto:admin@example.com" {
		t.Errorf("after create hook: got %q", got)
	}

	rec.Set("value", "mailto:ops@example.com")
	if err := app.Save(rec); err != nil {
		t.Fatalf("update setting: %v", err)
	}
	if got := systemConfig.Get("vapid.subject"); got != "mailto:ops@example.com" {
		t.Errorf("after update hook: got %q", got)
	}
}
