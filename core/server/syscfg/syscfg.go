// Package syscfg is the indirection through which core reads system-wide
// configuration — the "use-your-own-service" values (Sentry, web push, mail
// provider credentials) stored in the system_settings collection.
//
// By default it reads that collection, exactly as the code did before this
// package existed, so a standalone deployment behaves identically. The reason
// it is an indirection at all is that a SUPERVISING composition — a process
// that owns this app's runtime wiring rather than discovering it — may own
// these values instead, and must be able to say so without core knowing
// anything about what that supervisor is.
//
// This is the same seam quota already has: SettingsLimits reads the org's own
// database, FixedLimits takes the ceiling from whoever composed the app. The
// reason is identical, and worth restating because it is the whole point —
// every deployment has its own superusers, so a value stored inside the
// deployment can be changed by that deployment. A value its operator must own
// therefore cannot live there.
//
// It is a package of its own rather than a member of coreserver so that push,
// mailer and feature packages can read through it without an import cycle.
// Nothing here imports coreserver.
package syscfg

import (
	"strings"
	"sync"
)

// Provider resolves a system-settings key and declares which key namespaces
// its supplier owns.
//
// ManagedPrefixes returns dotted key prefixes ("sentry.", "vapid.", "mail.").
// A managed namespace is one this deployment must not edit for itself: core
// refuses writes to those keys and hides the UI that would edit them. The
// default provider manages nothing, which is what makes a standalone
// deployment fully self-administered.
type Provider interface {
	Get(key string) string
	ManagedPrefixes() []string
}

var (
	mu      sync.RWMutex
	current Provider = unmanaged{}
)

// unmanaged is the zero provider: it resolves nothing and manages nothing. It
// stands in before SetProvider or SetResolver runs so that a Get during boot
// returns "" rather than panicking, matching SystemConfig's own behaviour
// before its first load.
type unmanaged struct{}

func (unmanaged) Get(string) string         { return "" }
func (unmanaged) ManagedPrefixes() []string { return nil }

// resolverProvider adapts a plain lookup func to Provider, managing nothing.
// This is the standalone shape: core's own SystemConfig.Get supplies values and
// the deployment administers every one of them itself.
type resolverProvider struct{ get func(string) string }

func (r resolverProvider) Get(key string) string   { return r.get(key) }
func (resolverProvider) ManagedPrefixes() []string { return nil }

// SetResolver installs a plain value lookup that manages no namespace — what
// coreserver calls with SystemConfig.Get once the collection is loadable.
func SetResolver(get func(key string) string) {
	if get == nil {
		return
	}
	SetProvider(resolverProvider{get: get})
}

// SetProvider installs the process-wide provider. A supervising composition
// calls this BEFORE any package registers, so that every consumer's first read
// already resolves through it.
//
// A nil provider is ignored rather than installed: losing the resolver would
// turn every subsequent read into "" — silently disabling mail, push and error
// reporting — and a caller that passes nil has a bug, not an intent.
func SetProvider(p Provider) {
	if p == nil {
		return
	}
	mu.Lock()
	current = p
	mu.Unlock()
}

// Get resolves a key through the current provider. Unset keys return "".
func Get(key string) string {
	mu.RLock()
	p := current
	mu.RUnlock()
	return p.Get(key)
}

// ManagedPrefixes reports the key namespaces the current provider owns.
// Empty on a standalone deployment.
func ManagedPrefixes() []string {
	mu.RLock()
	p := current
	mu.RUnlock()
	return p.ManagedPrefixes()
}

// IsManaged reports whether key falls in a namespace the provider owns, and so
// must not be written or edited from inside this deployment.
//
// An empty prefix is ignored rather than treated as "everything": it would
// match every key and lock the deployment out of its own settings, which is
// never what a supplier of an empty list meant.
func IsManaged(key string) bool {
	for _, prefix := range ManagedPrefixes() {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
