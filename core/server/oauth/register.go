package oauth

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Register wires the OAuth authorization server into an app.
//
// Called from coreserver.registerSharedCore, so a single-org deployment and a
// multi-org tenant get exactly the same endpoints — an org hosted on the
// router must be able to authorize a CLI or a Zapier connection just like a
// self-hosted box.
func Register(app *pocketbase.PocketBase) {
	bindGrantEnforcement(app)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Discovery. Unauthenticated by design: a client reads this before it
		// has any credential.
		e.Router.GET("/.well-known/oauth-authorization-server",
			func(re *core.RequestEvent) error { return handleMetadata(app, re) })

		// Device Authorization Grant (RFC 8628). Unauthenticated: the whole
		// point is that the device has no credential yet.
		e.Router.POST("/oauth/device",
			func(re *core.RequestEvent) error { return handleDeviceAuthorization(app, re) })

		// Token endpoint. Unauthenticated at the HTTP layer; the grant itself
		// (device_code / auth code + PKCE / refresh token) is the credential.
		e.Router.POST("/oauth/token",
			func(re *core.RequestEvent) error { return handleToken(app, re) })

		// Revocation (RFC 7009). Unauthenticated per spec — presenting the
		// token is sufficient authority to destroy it.
		e.Router.POST("/oauth/revoke",
			func(re *core.RequestEvent) error { return handleRevoke(app, re) })

		// Consent surfaces. These require a signed-in user: approval binds the
		// grant to whoever is authenticated in the browser.
		e.Router.GET("/oauth/authorize/info",
			func(re *core.RequestEvent) error { return handleAuthorizeInfo(app, re) })
		e.Router.POST("/oauth/authorize/approve",
			func(re *core.RequestEvent) error { return handleApproveDevice(app, re) })
		e.Router.POST("/oauth/authorize/deny",
			func(re *core.RequestEvent) error { return handleDenyDevice(app, re) })
		e.Router.POST("/oauth/authorize",
			func(re *core.RequestEvent) error { return handleAuthorize(app, re) })

		// Identity for integrations.
		e.Router.GET("/oauth/userinfo",
			func(re *core.RequestEvent) error { return handleUserinfo(app, re) })

		return e.Next()
	})
}
