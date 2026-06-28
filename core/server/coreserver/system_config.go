package coreserver

import (
	"log"
	"sync"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
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
	onChange []func(key, value string)
}

// systemConfig is the process-wide instance, constructed by RegisterSystemConfig
// and read by subsystems (sentry.go, push, mail) via SystemSettings().
var systemConfig = &SystemConfig{values: map[string]string{}}

// SystemSettings returns the process-wide SystemConfig. Always non-nil; before
// load() runs (i.e. before the DB is ready) every Get returns "".
func SystemSettings() *SystemConfig { return systemConfig }

// Get returns the current value for key, or "" if unset. Pure in-memory read.
func (c *SystemConfig) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
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
	for _, r := range recs {
		next[r.GetString("key")] = r.GetString("value")
	}
	c.mu.Lock()
	c.values = next
	c.mu.Unlock()
}

// set updates a single key in the map and fires change callbacks. Called by the
// record hooks; the callbacks run OUTSIDE the lock so a reinit handler can call
// Get without deadlocking.
func (c *SystemConfig) set(key, value string) {
	c.mu.Lock()
	if c.values == nil {
		c.values = map[string]string{}
	}
	c.values[key] = value
	cbs := make([]func(string, string), len(c.onChange))
	copy(cbs, c.onChange)
	c.mu.Unlock()
	for _, fn := range cbs {
		fn(key, value)
	}
}

// RegisterSystemConfig constructs the system config lifecycle: load all values
// once the server starts, and keep the in-memory map in sync as rows are created
// or updated. Edits take effect without a restart — re-init handlers registered
// via OnChange run on each change.
func RegisterSystemConfig(app *pocketbase.PocketBase) {
	// Load after bootstrap/migrations, before the server begins handling
	// requests. OnServe fires once per boot at that point.
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		systemConfig.load(app)
		return e.Next()
	})

	syncRow := func(e *core.RecordEvent) error {
		systemConfig.set(e.Record.GetString("key"), e.Record.GetString("value"))
		return e.Next()
	}
	app.OnRecordAfterCreateSuccess("system_settings").BindFunc(syncRow)
	app.OnRecordAfterUpdateSuccess("system_settings").BindFunc(syncRow)
}
