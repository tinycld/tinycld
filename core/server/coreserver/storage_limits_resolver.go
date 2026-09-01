package coreserver

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/quota"
)

// A storage ceiling can change while a tenant is running, and TenantOptions
// cannot express that: the caller must hand RegisterTenant a QuotaLimits value
// before the app exists, so anything it resolves is fixed at boot and a new
// ceiling only takes effect on the next spawn.
//
// This is the seam that closes that gap. A package registered through
// TenantOptions.RegisterExtras runs BEFORE quota.Register binds its hooks, so
// it can leave a resolver here and RegisterTenant will bind that instead of
// opts.QuotaLimits. A package holding a live source for the ceiling — one it
// refreshes out of band — therefore gets consulted on every write rather than
// once at boot.
//
// The resolver is called per record save, so it must be cheap: a cached struct
// read, never a query or a round trip.

// storageLimitsResolverKey namespaces the store entry; the app store is shared
// with PocketBase internals and package code.
const storageLimitsResolverKey = "tinycld.storageLimitsResolver"

// SetStorageLimitsResolver registers the resolver core consults for storage
// ceilings, replacing any previously set. Call it from a package's Register
// during TenantOptions.RegisterExtras — that is the only window early enough,
// since quota.Register binds no hooks at all against a nil resolver and a
// resolver supplied after it would leave enforcement silently off.
func SetStorageLimitsResolver(app core.App, limits quota.LimitsFunc) {
	app.Store().Set(storageLimitsResolverKey, limits)
}

// StorageLimitsResolver reports the registered resolver, and whether one was
// registered at all. A nil resolver stored under the key reads as absent, so a
// package that computed nothing cannot silently disable enforcement that
// TenantOptions.QuotaLimits would otherwise have provided.
func StorageLimitsResolver(app core.App) (quota.LimitsFunc, bool) {
	limits, ok := app.Store().Get(storageLimitsResolverKey).(quota.LimitsFunc)
	if !ok || limits == nil {
		return nil, false
	}
	return limits, true
}
