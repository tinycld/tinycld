package webforms

import "github.com/pocketbase/pocketbase/core"

// Register binds the publication hooks, the public routes and the embed
// policy. Called from coreserver.RegisterSharedCore.
func Register(app core.App) {
	bindPublicationHooks(app)
	registerEmbedPolicy(app)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/api/forms/public/{token}", func(re *core.RequestEvent) error {
			return handlePublicForm(app, re)
		})
		se.Router.GET("/api/forms/public/{token}/challenge", func(re *core.RequestEvent) error {
			return handleChallenge(app, re)
		})
		bindSubmitRoute(app, se)
		return se.Next()
	})
}
