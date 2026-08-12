// tinycld/core/server/automation/register.go
package automation

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/notify"
)

type Options struct {
	DefsPath string
}

// Register wires the rules engine into an app. With no materialized defs the
// engine is inert: no hooks bound, no endpoints — a workspace without
// automation packages pays one file-stat.
func Register(app *pocketbase.PocketBase, opts Options) {
	defs, err := LoadDefs(opts.DefsPath)
	if err != nil {
		app.Logger().Error("automation: defs unreadable, engine disabled", "err", err)
		return
	}
	if len(defs.Packages) == 0 {
		app.Logger().Info("automation: no definitions, engine inert")
		return
	}

	RegisterAction("core:notify", func(a core.App, req ActionRequest) error {
		go func() {
			if !appIsLive(a) {
				return
			}
			notify.NotifyUser(a, notify.NotifyParams{
				UserID:  req.OwnerID,
				Type:    "automation",
				Package: "core",
				Title:   req.Params["title"],
				Body:    req.Params["body"],
				URL:     req.Params["url"],
			})
		}()
		return nil
	})

	engine := NewEngine(app, defs)
	registerEndpoints(app, engine)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		engine.Start()
		return se.Next()
	})
}
