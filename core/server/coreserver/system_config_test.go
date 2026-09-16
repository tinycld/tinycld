package coreserver

import (
	"strings"
	"sync"
	"testing"
	"tinycld.org/core/syscfg"

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

	cfg.set("sentry.dsn", "https://new.dsn", false)

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
	cfg.set("sentry.dsn", "https://read.dsn", false)
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
		systemConfig.set(
			e.Record.GetString("key"),
			e.Record.GetString("value"),
			e.Record.GetBool("is_secret"),
		)
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

// PublicValues exposes non-secret keys and withholds secret ones — the gate that
// keeps tokens/private keys out of anything sent to a client.
func TestSystemConfigPublicValues(t *testing.T) {
	cfg := &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	cfg.set("sentry.dsn", "https://public.dsn", false)
	cfg.set("sentry.auth_token", "secret-token", true)

	pub := cfg.PublicValues()
	if pub["sentry.dsn"] != "https://public.dsn" {
		t.Errorf("non-secret value missing from PublicValues: %v", pub)
	}
	if _, present := pub["sentry.auth_token"]; present {
		t.Error("secret value must NOT appear in PublicValues")
	}
}

// injectPublicConfig publishes only non-secret values into the HTML, before
// </head>, and never leaks a secret. Drives the package-global systemConfig.
func TestInjectPublicConfig(t *testing.T) {
	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	// The injector resolves values through the syscfg seam, so point it at the
	// config this test drives.
	syscfg.SetResolver(systemConfig.Get)
	t.Cleanup(func() { syscfg.SetResolver(prev.Get) })
	systemConfig.set("sentry.dsn", "https://abc@o1.ingest.sentry.io/1", false)
	systemConfig.set("sentry.auth_token", "super-secret", true)

	html := []byte("<html><head><title>x</title></head><body></body></html>")
	out := string(injectPublicConfig(html))

	if !strings.Contains(out, "window.__TINYCLD_PUBLIC_CONFIG__=") {
		t.Fatalf("injection missing: %s", out)
	}
	if !strings.Contains(out, "https://abc@o1.ingest.sentry.io/1") {
		t.Error("non-secret DSN should be injected")
	}
	if !strings.Contains(out, `"sentryDsn"`) {
		t.Error("DSN should be published under the client field name sentryDsn")
	}
	if strings.Contains(out, "super-secret") || strings.Contains(out, "auth_token") {
		t.Error("secret value/key must NEVER be injected into the HTML")
	}
	// Injected before </head> so it runs before the deferred bundle.
	if strings.Index(out, "window.__TINYCLD_PUBLIC_CONFIG__") > strings.Index(out, "</head>") {
		t.Error("script must be injected before </head>")
	}
}

// A value containing "</script>" must not terminate the tag early.
func TestInjectPublicConfigEscapesScriptClose(t *testing.T) {
	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	systemConfig.set("sentry.dsn", "https://x/</script><script>alert(1)</script>", false)

	out := string(injectPublicConfig([]byte("<head></head>")))
	if strings.Contains(out, "</script><script>alert(1)") {
		t.Errorf("unescaped </script> breakout in injected config: %s", out)
	}
}

// Nothing to publish (no non-secret values) → HTML unchanged.
func TestInjectPublicConfigNoop(t *testing.T) {
	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	systemConfig.set("sentry.auth_token", "only-a-secret", true)

	html := []byte("<head></head>")
	if got := string(injectPublicConfig(html)); got != string(html) {
		t.Errorf("expected HTML unchanged when nothing public, got %q", got)
	}
}

// publicValue (the per-key injection gate) returns a non-secret value but BLANKS
// a secret one — so even a whitelisted key can't leak a credential into the HTML
// if it were mis-flagged secret.
func TestSystemConfigPublicValue(t *testing.T) {
	cfg := &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	cfg.set("sentry.dsn", "https://public.dsn", false)
	if got := cfg.publicValue("sentry.dsn"); got != "https://public.dsn" {
		t.Errorf("non-secret publicValue = %q, want the value", got)
	}
	// Flip the same key to secret → must blank.
	cfg.set("sentry.dsn", "https://now.secret", true)
	if got := cfg.publicValue("sentry.dsn"); got != "" {
		t.Errorf("secret publicValue must be blank, got %q", got)
	}
	if got := cfg.publicValue("missing"); got != "" {
		t.Errorf("unset publicValue must be blank, got %q", got)
	}
}

// The injector relies on publicValue: a sentry.dsn mis-flagged secret must NOT
// appear in the injected HTML.
func TestInjectPublicConfigSkipsSecretSentryDsn(t *testing.T) {
	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	systemConfig.set("sentry.dsn", "https://leak.dsn", true) // mis-flagged secret

	out := string(injectPublicConfig([]byte("<head></head>")))
	if out != "<head></head>" {
		t.Errorf("a secret-flagged sentry.dsn must not be injected, got %q", out)
	}
}

// The web client needs the VAPID PUBLIC key as pushManager.subscribe's
// applicationServerKey, so it is whitelisted into the injected config. Before
// this, the key reached the browser by no route at all and web push could never
// subscribe. The PRIVATE key must never follow it out.
func TestInjectPublicConfigPublishesVapidPublicKey(t *testing.T) {
	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	// The injector resolves values through the syscfg seam, so point it at the
	// config this test drives.
	syscfg.SetResolver(systemConfig.Get)
	t.Cleanup(func() { syscfg.SetResolver(prev.Get) })
	systemConfig.set("vapid.public_key", "BPublicKey123", false)
	systemConfig.set("vapid.private_key", "PrivateKeyNeverLeak", true)

	out := string(injectPublicConfig([]byte("<head></head>")))

	if !strings.Contains(out, `"vapidPublicKey"`) {
		t.Errorf("public key should be published as vapidPublicKey, got %q", out)
	}
	if !strings.Contains(out, "BPublicKey123") {
		t.Errorf("public key value missing from injected config: %q", out)
	}
	if strings.Contains(out, "PrivateKeyNeverLeak") || strings.Contains(out, "private_key") {
		t.Fatalf("VAPID PRIVATE key must NEVER be injected into the HTML: %q", out)
	}
}

// A VAPID public key mis-flagged is_secret=true must be blanked by the
// publicValue gate rather than injected.
func TestInjectPublicConfigSkipsSecretVapidPublicKey(t *testing.T) {
	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	systemConfig.set("vapid.public_key", "BLeakKey", true) // mis-flagged secret

	if out := string(injectPublicConfig([]byte("<head></head>"))); out != "<head></head>" {
		t.Errorf("a secret-flagged vapid.public_key must not be injected, got %q", out)
	}
}

// managedProvider is a supervising composition's provider: it owns the listed
// namespaces and supplies their values from memory, never from a row here.
type managedProvider struct{ prefixes []string }

func (managedProvider) Get(string) string           { return "" }
func (m managedProvider) ManagedPrefixes() []string { return m.prefixes }

// updateSettingAsRequest drives the OnRecordUpdateRequest chain — the path a
// client save takes. app.Save alone bypasses request hooks entirely, so a test
// that used it would pass no matter what the guard does.
func updateSettingAsRequest(t *testing.T, app core.App, rec *core.Record, value string) error {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("system_settings")
	if err != nil {
		t.Fatal(err)
	}
	rec.Set("value", value)
	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{App: app},
		Record:       rec,
	}
	e.Collection = col
	return app.OnRecordUpdateRequest("system_settings").Trigger(e, func(_ *core.RecordRequestEvent) error {
		return app.Save(rec)
	})
}

// A deployment must not edit a value its operator owns. Hiding the UI is not
// enough: the collection's rules authorize any owner or admin, a superuser
// bypasses them, and — because reads resolve through syscfg — a permitted write
// would be silently inert rather than effective.
func TestManagedKeysRefuseWrites(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	createSystemSettingsCollection(t, app)

	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}

	managed := saveSetting(t, app, "mail.provider", "postmark")
	ownKey := saveSetting(t, app, "storage_limit_bytes", "100")

	// Bind the REAL guard RegisterSystemConfig installs, not a copy of it.
	app.OnRecordUpdateRequest("system_settings").BindFunc(refuseManagedWrite)

	syscfg.SetProvider(managedProvider{prefixes: []string{"mail.", "vapid.", "sentry."}})
	t.Cleanup(func() { syscfg.SetResolver(prev.Get) })

	if err := updateSettingAsRequest(t, app, managed, "smtp"); err == nil {
		t.Error("a write to a managed key was permitted; it must be refused")
	}
	// A key outside every managed namespace stays this deployment's own.
	if err := updateSettingAsRequest(t, app, ownKey, "200"); err != nil {
		t.Errorf("a write to an unmanaged key was refused: %v", err)
	}
}

// The same collection stays fully editable where nothing is managed — the
// standalone guarantee.
func TestUnmanagedDeploymentKeepsWritingEverySetting(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	createSystemSettingsCollection(t, app)

	prev := systemConfig
	t.Cleanup(func() { systemConfig = prev })
	systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	syscfg.SetResolver(systemConfig.Get)
	t.Cleanup(func() { syscfg.SetResolver(prev.Get) })

	app.OnRecordUpdateRequest("system_settings").BindFunc(refuseManagedWrite)

	rec := saveSetting(t, app, "mail.provider", "postmark")
	if err := updateSettingAsRequest(t, app, rec, "smtp"); err != nil {
		t.Errorf("standalone deployment could not edit its own mail settings: %v", err)
	}
}
