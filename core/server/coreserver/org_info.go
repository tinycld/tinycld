package coreserver

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// RegisterOrgInfoEndpoint serves the deployment's branding to the client,
// unauthenticated — the client wants it before login (document title, org
// avatar). The name is Settings().Meta.AppName: the setup wizard's app name in
// a standalone deployment, or the org's display_name in a router-managed
// tenant (adopted from .runtime/app.json at boot, serve-org). The logo URL and
// crop come from org_branding, whose viewRule is deliberately public for the
// same pre-login reason. Nothing else from settings is exposed: PB's own
// /api/settings is superuser-only, and this endpoint publishes only these
// deliberately-public fields.
func RegisterOrgInfoEndpoint(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/org-info", func(re *core.RequestEvent) error {
			logoURL, logoCrop := orgLogo(app)
			return re.JSON(http.StatusOK, map[string]string{
				"name":     app.Settings().Meta.AppName,
				"logoUrl":  logoURL,
				"logoCrop": logoCrop,
			})
		})
		return e.Next()
	})
}

// orgLogo returns the deployment logo's public URL and stored crop, or empty
// strings when none is set. Every failure path is "no logo": this endpoint is
// unauthenticated and on the pre-login path, so it must never 500 because
// branding is absent or the collection has not been migrated yet.
func orgLogo(app core.App) (string, string) {
	record, err := app.FindFirstRecordByFilter("org_branding", "id != ''")
	if err != nil || record == nil {
		return "", ""
	}
	filename := record.GetString("logo")
	if filename == "" {
		return "", ""
	}
	return "/api/files/org_branding/" + record.Id + "/" + filename, record.GetString("logo_crop")
}
