package coreserver

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// RegisterOrgInfoEndpoint serves the deployment's branding to the client,
// unauthenticated — the client wants it before login (document title, org
// avatar). The name is Settings().Meta.AppName: the setup wizard's app name in
// a standalone deployment, or the org's display_name in a router-managed
// tenant (adopted from .runtime/app.json at boot, serve-org). Nothing else
// from settings is exposed: PB's own /api/settings is superuser-only, and this
// endpoint publishes exactly one deliberately-public field.
func RegisterOrgInfoEndpoint(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/org-info", func(re *core.RequestEvent) error {
			return re.JSON(http.StatusOK, map[string]string{
				"name": app.Settings().Meta.AppName,
			})
		})
		return e.Next()
	})
}
