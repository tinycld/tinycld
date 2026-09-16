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

// isSecret reports a locally-stored row's flag — the second gate before a
// whitelisted key is injected into the page. It speaks only for rows THIS
// deployment stores; see publicConfigScript for why the whitelist, not this
// flag, is what actually keeps the page safe.
func TestSystemConfigIsSecret(t *testing.T) {
	cfg := &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}
	cfg.set("sentry.dsn", "https://public.dsn", false)
	cfg.set("sentry.auth_token", "secret-token", true)

	// isSecret is the gate publishableValue consults before injecting a
	// whitelisted key into the page.
	if cfg.isSecret("sentry.dsn") {
		t.Error("a non-secret row was reported secret; its value would be withheld from the page")
	}
	if !cfg.isSecret("sentry.auth_token") {
		t.Error("a secret row was not reported secret; were it ever whitelisted it would reach the page")
	}
	// A key with no local row — every value a supervisor supplies — is not
	// secret by omission. It is not ours to judge, which is exactly why the
	// whitelist in publicConfigScript, not this flag, is what keeps the page
	// safe.
	if cfg.isSecret("vapid.private_key") {
		t.Error("a key with no local row should not be reported secret")
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
	syscfg.ResetForTesting()
	syscfg.SetResolver(systemConfig.Get)
	t.Cleanup(syscfg.ResetForTesting)
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
	syscfg.ResetForTesting()
	syscfg.SetResolver(systemConfig.Get)
	t.Cleanup(syscfg.ResetForTesting)
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

// updateSetting saves through app.Save — deliberately the LOWEST-level write
// path. The guard is bound on the model hooks, which fire here; binding only the
// request hooks would leave this path (and every other in-process writer) open,
// so this is the case worth asserting.
func updateSetting(t *testing.T, app core.App, rec *core.Record, value string) error {
	t.Helper()
	rec.Set("value", value)
	return app.Save(rec)
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
	app.OnRecordUpdate("system_settings").BindFunc(refuseManagedWrite)

	syscfg.SetProvider(managedProvider{prefixes: []string{"mail.", "vapid.", "sentry."}})
	t.Cleanup(syscfg.ResetForTesting)

	if err := updateSetting(t, app, managed, "smtp"); err == nil {
		t.Error("a write to a managed key was permitted; it must be refused")
	}
	// A key outside every managed namespace stays this deployment's own.
	if err := updateSetting(t, app, ownKey, "200"); err != nil {
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
	syscfg.ResetForTesting()
	syscfg.SetResolver(systemConfig.Get)
	t.Cleanup(syscfg.ResetForTesting)

	app.OnRecordUpdate("system_settings").BindFunc(refuseManagedWrite)

	rec := saveSetting(t, app, "mail.provider", "postmark")
	if err := updateSetting(t, app, rec, "smtp"); err != nil {
		t.Errorf("standalone deployment could not edit its own mail settings: %v", err)
	}
}
