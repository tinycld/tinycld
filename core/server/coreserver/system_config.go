package coreserver

import (
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailer"
	"tinycld.org/core/syscfg"
)

// SystemConfig is the single source of truth for system-wide configuration —
// the "use-your-own-service" values (Sentry, web-push, mail provider creds) that
// a third-party host owns. Values live in the `system_settings` collection and
// are configured from the /admin Settings console; this struct holds the current
// values in memory so consumers read them without a DB hit on every use, and so
// stateful consumers (Sentry) can be re-initialized the moment a value changes.
//
// There is NO os.Getenv anywhere in the read path: the environment is consulted
// exactly once, by the create_system_settings migration's one-time seed. After
// that, the collection is authoritative.
type SystemConfig struct {
	mu       sync.RWMutex
	values   map[string]string
	secret   map[string]bool // key → is_secret; gates what may be exposed to clients
	onChange []func(key, value string)
}

// systemConfig is the process-wide instance, constructed by RegisterSystemConfig
// and read by subsystems (sentry.go, push, mail) via SystemSettings().
var systemConfig = &SystemConfig{values: map[string]string{}, secret: map[string]bool{}}

// SystemSettings returns the process-wide SystemConfig. Always non-nil; before
// load() runs (i.e. before the DB is ready) every Get returns "".
func SystemSettings() *SystemConfig { return systemConfig }

// Get returns the current value for key, or "" if unset. Pure in-memory read.
func (c *SystemConfig) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.values[key]
}

// publicValue returns the value for key ONLY if it is non-secret; a secret (or
// unset) key returns "". Per-key gate for the HTML injector so a single
// whitelisted key can't leak even if a row were mis-flagged.
func (c *SystemConfig) publicValue(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.secret[key] {
		return ""
	}
	return c.values[key]
}

// isSecret reports whether a LOCALLY STORED row for key is flagged secret. A
// key with no local row is not secret by omission — it simply isn't ours to
// judge, which is the case for every value a supervisor supplies. Callers that
// publish anything must therefore establish publishability by whitelist and use
// this only to honour a local row's flag.
func (c *SystemConfig) isSecret(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.secret[key]
}

// OnChange registers a callback fired whenever a key's value changes (via the
// system_settings record hooks). Used by stateful consumers that must re-init —
// e.g. Sentry re-runs sentry.Init on a sentry.* change. Consumers that read
// per-use (push reads VAPID per send; mail builds its provider per call) don't
// need this; they just call Get at use time.
func (c *SystemConfig) OnChange(fn func(key, value string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChange = append(c.onChange, fn)
}

// load replaces the in-memory map with every row currently in system_settings.
// Run once after the DB is ready (an OnServe hook, post-migration).
func (c *SystemConfig) load(app core.App) {
	recs, err := app.FindRecordsByFilter("system_settings", "id != ''", "", 0, 0)
	if err != nil {
		// The collection may not exist yet on a brand-new DB whose migrations
		// haven't run; treat as empty rather than failing the boot.
		srvLog.Info("system config load skipped", "err", err)
		return
	}
	next := make(map[string]string, len(recs))
	nextSecret := make(map[string]bool, len(recs))
	for _, r := range recs {
		key := r.GetString("key")
		next[key] = r.GetString("value")
		nextSecret[key] = r.GetBool("is_secret")
	}
	c.mu.Lock()
	c.values = next
	c.secret = nextSecret
	c.mu.Unlock()
}

// set updates a single key (value + secret flag) and fires change callbacks.
// Called by the record hooks; the callbacks run OUTSIDE the lock so a reinit
// handler can call Get without deadlocking.
func (c *SystemConfig) set(key, value string, isSecret bool) {
	c.mu.Lock()
	if c.values == nil {
		c.values = map[string]string{}
	}
	if c.secret == nil {
		c.secret = map[string]bool{}
	}
	c.values[key] = value
	c.secret[key] = isSecret
	cbs := make([]func(string, string), len(c.onChange))
	copy(cbs, c.onChange)
	c.mu.Unlock()
	for _, fn := range cbs {
		fn(key, value)
	}
}

// refuseManagedWrite rejects a write to any key a supervising composition owns.
// A package-level function, not a closure, so tests bind exactly what production
// binds — a copy in a test would keep passing after this guard was deleted.
//
// Bound on the MODEL-level hooks, which fire for app.Save() as well as for a
// request. The request hooks alone left every in-process writer unguarded —
// upsertSystemSetting in vapid_admin.go is one, and any future one would inherit
// no protection at all. A hand-placed check at each call site is not a boundary;
// this is.
func refuseManagedWrite(e *core.RecordEvent) error {
	if syscfg.IsManaged(e.Record.GetString("key")) {
		return apis.NewForbiddenError(
			"This setting is managed by your hosting provider and cannot be changed here.", nil)
	}
	return e.Next()
}

// RegisterSystemConfig constructs the system config lifecycle: load all values
// once the server starts, init the consumers that depend on them, and keep the
// in-memory map in sync as rows are created or updated. Edits take effect without
// a restart — re-init handlers registered via OnChange run on each change.
func RegisterSystemConfig(app *pocketbase.PocketBase) {
	// Point the syscfg seam at this collection, and the mailer at the seam.
	// The resolver reads lazily (per-send), so it's safe to set here before the
	// OnServe load — Get returns "" until then, and sends only happen well
	// after boot.
	//
	// SetResolver is inert once a supervising composition has claimed the seam,
	// so this cannot hand administration back to a deployment whose operator
	// owns these values. Called unconditionally because the claim, not the
	// caller, is what decides.
	//
	// Do NOT reintroduce a "does it manage anything" check here. A supervisor
	// whose config failed to load claims the seam managing nothing yet, and
	// treating that as unclaimed would silently restore the org's own
	// collection — the precise fallback the supervisor exists to prevent.
	syscfg.SetResolver(systemConfig.Get)
	// syscfg, not systemConfig.Get, is what keeps the mailer out of an import
	// cycle with this package (mailer can't import coreserver).
	mailer.ConfigResolver = syscfg.Get

	// Re-init Sentry whenever a sentry.* value changes. Registered before the
	// initial load so no early change is missed. Sentry is the only stateful
	// consumer; push reads VAPID per-send and mail builds its provider per-call,
	// so both pick up changes via Get without a re-init handler.
	systemConfig.OnChange(func(key, _ string) {
		if strings.HasPrefix(key, "sentry.") {
			initSentryFromConfig()
		}
	})

	// Load after bootstrap/migrations, before the server begins handling
	// requests, then perform the initial Sentry init from the loaded values.
	// OnServe fires once per boot at that point. The Sentry middleware is already
	// bound (RegisterSentry); this supplies the client the middleware reports to.
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		systemConfig.load(app)
		initSentryFromConfig()
		return e.Next()
	})

	// Refuse writes to keys a supervising composition owns.
	//
	// Without this the hidden UI would be the only thing stopping a write, and
	// hiding a form is not access control: the collection's rules authorize any
	// owner or admin, and a PB superuser bypasses them entirely — and every
	// deployment has its own superusers. Worse, a permitted write would be
	// SILENTLY INERT, because reads resolve through syscfg and would never look
	// at the stored row: the form would save, report success, and change
	// nothing. Refusing is both safer and more honest.
	//
	// On the create/update/delete hooks rather than the *Success ones below:
	// this must run before the row is persisted, not react to it afterwards.
	app.OnRecordCreate("system_settings").BindFunc(refuseManagedWrite)
	app.OnRecordUpdate("system_settings").BindFunc(refuseManagedWrite)
	// Delete too: removing a managed row cannot unmanage the value (reads never
	// consult the row) but it would desynchronize the collection from what the
	// deployment actually runs on, which is how a confusing support case starts.
	app.OnRecordDelete("system_settings").BindFunc(refuseManagedWrite)

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
}
