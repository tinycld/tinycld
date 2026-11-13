package classifier

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

var verifyTaxonomy = []TaxonomyLabel{
	{Name: "greeting", AssignWhen: "A short friendly hello."},
	{Name: "other", AssignWhen: "Anything that is not a greeting."},
}

// Register binds the admin verify route. Verify bypasses the paused status
// and the budget: it is how an admin clears a pause after fixing the key.
func Register(app core.App, adminOnly func(*core.RequestEvent) error) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/classifier/verify", func(e *core.RequestEvent) error {
			cfg := LoadConfig(e.App)
			if cfg.APIKey == "" {
				return e.JSON(http.StatusOK, map[string]any{"ok": false, "error": "missing_key"})
			}
			_, err := call(e.Request.Context(), e.App, cfg, verifyTaxonomy, map[string]any{"subject": "Hello there"}, StageMetadata, false)
			if err != nil {
				return e.JSON(http.StatusOK, map[string]any{"ok": false, "error": ErrorCode(err)})
			}
			if err := setStatus(e.App, StatusOK); err != nil {
				return e.InternalServerError("", err)
			}
			return e.JSON(http.StatusOK, map[string]any{"ok": true, "error": ""})
		}).BindFunc(adminOnly)
		// Members cannot read system_settings, so this is how their
		// settings screens learn whether the feature is on. It exposes only
		// the boolean, never the key or the status.
		se.Router.GET("/api/classifier/status", func(e *core.RequestEvent) error {
			if e.Auth.GetString("role") == "guest" {
				return e.ForbiddenError("A guest cannot read the classifier status.", nil)
			}
			return e.JSON(http.StatusOK, map[string]any{"available": Available(e.App)})
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}
