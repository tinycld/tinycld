package coreserver

import (
	"log"
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailer"
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

// PublicValues returns a copy of every NON-secret key→value pair. This is the
// only set of values allowed to be injected into the web HTML. Secret values
// (tokens, the VAPID private key, IMAP password) are never included here.
//
// NOTE: this gates HTML INJECTION only. It is NOT a confidentiality boundary for
// the secret values themselves — those still live in the super-admin-readable
// system_settings collection, so a super admin's client reads them over the wire
// (the admin UI just renders them write-only). The trust boundary is "super
// admin"; "secret" here means "never embedded in the public page served to every
// visitor".
func (c *SystemConfig) PublicValues() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string)
	for k, v := range c.values {
		if !c.secret[k] {
			out[k] = v
		}
	}
	return out
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
		log.Printf("system_config: load skipped: %v", err)
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

// RegisterSystemConfig constructs the system config lifecycle: load all values
// once the server starts, init the consumers that depend on them, and keep the
// in-memory map in sync as rows are created or updated. Edits take effect without
// a restart — re-init handlers registered via OnChange run on each change.
func RegisterSystemConfig(app *pocketbase.PocketBase) {
	// Point the core transactional mailer at system config. The resolver reads
	// lazily (per-send), so it's safe to set here before the OnServe load —
	// Get returns "" until then, and sends only happen well after boot. This is
	// the single seam that keeps the mailer out of an import cycle with this
	// package (mailer can't import coreserver).
	mailer.ConfigResolver = systemConfig.Get

	// Re-init Sentry whenever a sentry.* value changes. Registered before the
	// initial load so no early change is missed. Sentry is the only stateful
	// consumer; push reads VAPID per-send and mail builds its provider per-call,
	// so both pick up changes via Get without a re-init handler.
	systemConfig.OnChange(func(key, _ string) {
		if strings.HasPrefix(key, "sentry.") {
			initSentryFromConfig(systemConfig)
		}
	})

	// Load after bootstrap/migrations, before the server begins handling
	// requests, then perform the initial Sentry init from the loaded values.
	// OnServe fires once per boot at that point. The Sentry middleware is already
	// bound (RegisterSentry); this supplies the client the middleware reports to.
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		systemConfig.load(app)
		initSentryFromConfig(systemConfig)
		return e.Next()
	})

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
