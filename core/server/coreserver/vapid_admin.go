package coreserver

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/push"
)

// RegisterVapidAdminEndpoints exposes "generate a VAPID keypair" for the /admin
// Settings console. Generation AND persistence happen server-side: the operator
// never needs to read the keys (the server signs with the private key; the
// browser receives the public key through the normal push-subscribe flow), so
// the secret private key never travels to the client at all. The endpoint mints
// the pair, writes both into system_settings, and returns only the public key
// for display. Bring-your-own keys are still possible via the normal
// system_settings write (the panel's "use existing keys" path). AuthZ:
// requireAdmin, same as the other /api/admin endpoints.
func RegisterVapidAdminEndpoints(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/admin/vapid/generate", func(re *core.RequestEvent) error {
			return handleGenerateVapid(app, re)
		}).BindFunc(requireAdmin)
		return e.Next()
	})
}

// handleGenerateVapid mints a keypair, persists both keys to system_settings, and
// returns ONLY the public key. Split out so it's unit-testable against a
// tests.TestApp without standing up the router.
func handleGenerateVapid(app core.App, re *core.RequestEvent) error {
	privateKey, publicKey, err := push.GenerateVAPIDKeys()
	if err != nil {
		return re.InternalServerError("Failed to generate VAPID keys", err)
	}
	if err := upsertSystemSetting(app, "vapid.public_key", publicKey, false); err != nil {
		return re.InternalServerError("Failed to save VAPID public key", err)
	}
	if err := upsertSystemSetting(app, "vapid.private_key", privateKey, true); err != nil {
		return re.InternalServerError("Failed to save VAPID private key", err)
	}
	// Return only the public key — the private key stays server-side.
	return re.JSON(http.StatusOK, map[string]string{"publicKey": publicKey})
}

// upsertSystemSetting writes a single key into the system_settings collection,
// updating the existing row or creating one. Saving fires the
// OnRecordAfter*Success hooks, so the in-memory SystemConfig refreshes too.
func upsertSystemSetting(app core.App, key, value string, isSecret bool) error {
	existing, err := app.FindFirstRecordByFilter(
		"system_settings", "key = {:key}", map[string]any{"key": key},
	)
	if err == nil {
		existing.Set("value", value)
		return app.Save(existing)
	}
	col, err := app.FindCollectionByNameOrId("system_settings")
	if err != nil {
		return fmt.Errorf("find system_settings collection: %w", err)
	}
	rec := core.NewRecord(col)
	rec.Set("key", key)
	rec.Set("value", value)
	rec.Set("is_secret", isSecret)
	return app.Save(rec)
}
