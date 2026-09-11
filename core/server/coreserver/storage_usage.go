package coreserver

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/quota"
)

// StorageUsageResponse is the per-user storage breakdown: who is using the
// disk, and the per-user ceiling (0 = unlimited) their usage is measured
// against.
//
// Deliberately NOT a deployment total against a plan ceiling. A single-tenant
// deployment's operator owns the disk; the only question the app can answer
// for them is which user is filling it. What an organization owes for the
// space it occupies is a commercial question belonging to whoever sells the
// hosting, and core knows nothing about plans, ceilings or billing.
type StorageUsageResponse struct {
	Users        []quota.UserBytes `json:"users"`
	LimitPerUser int64             `json:"limitPerUser"`
}

// RegisterStorageUsageEndpoint serves GET /api/storage-usage.
//
// Package-agnostic by construction: it sums whatever collections the installed
// packages registered as quota sources, so it counts drive, mail, boards and
// anything added later without naming one.
//
// Sources and the per-user ceiling are read per request rather than captured,
// because this registers from RegisterSharedCore — shared by the single-org
// app and a hosting tenant, which bind quota with different limit resolvers
// (a tenant's org ceiling comes from the router). Reading through the same
// registry and resolver enforcement uses keeps the reported numbers and the
// enforced ceiling describing one set of collections in both compositions.
func RegisterStorageUsageEndpoint(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/storage-usage", func(re *core.RequestEvent) error {
			users, err := quota.UsageByUser(re.App, quota.RegisteredSources())
			if err != nil {
				return re.InternalServerError("Failed to compute storage usage", err)
			}
			perUser := quota.SettingsLimits(re.App).PerUser
			return re.JSON(http.StatusOK, StorageUsageResponse{
				Users:        users,
				LimitPerUser: perUser,
			})
			// Admin-gated: a per-user breakdown names every member and what
			// they store, which is not a member's business to read.
		}).BindFunc(requireAdmin)
		return e.Next()
	})
}
