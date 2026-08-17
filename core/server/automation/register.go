// tinycld/core/server/automation/register.go
package automation

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
	"tinycld.org/core/notify"
)

var log = logging.ForPackage("automation")

type Options struct {
	DefsPath string
}

// Register wires the rules engine into an app. With no materialized defs the
// engine is inert: no hooks bound, no endpoints — a workspace without
// automation packages pays one file-stat.
func Register(app *pocketbase.PocketBase, opts Options) {
	defs, err := LoadDefs(opts.DefsPath)
	if err != nil {
		log.Error("defs unreadable, engine disabled", "err", err)
		return
	}
	if len(defs.Packages) == 0 {
		log.Info("no definitions, engine inert")
		return
	}

	registerCoreNativeActions()

	engine := NewEngine(app, defs)
	registerEndpoints(app, engine)
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		engine.Start()
		// Native action availability depends on which packages' Go registered
		// their RegisterAction/RegisterExtras handlers — that happens before
		// OnServe, so the catalog snapshot taken here is accurate.
		engine.syncCatalog()
		return se.Next()
	})
}

// registerCoreNativeActions installs the Go handlers for core's own native
// action refs. Factored out of Register so a test can install them without
// the full app-bootstrap path (LoadDefs + OnServe binding).
func registerCoreNativeActions() {
	registerCoreEmailAction()

	// core:apply-label's `label` param is a caller-supplied record id, so it
	// needs a registered authorizer like any other relation param. The check
	// itself is deliberately a pass: labels are org-wide by design — any
	// non-guest may view, apply, and edit any label (the labels collection's
	// own rules say so, and the engine's floor has already held the rule
	// owner to them). If labels ever become per-user, this is the line that
	// changes.
	RegisterRelationAuthorizer("core:apply-label", "label", func(core.App, ActionRequest, string) error {
		return nil
	})

	// core:notify is fire-and-forget: the handler returns nil immediately and
	// the actual push happens on a detached goroutine. The engine records this
	// action's ActionResult as "ok" the instant the handler returns, before
	// notifyUser has run (or even before its liveness check) — a delivery
	// failure inside the goroutine is invisible to rule_runs.results.
	RegisterAction("core:notify", func(a core.App, req ActionRequest) error {
		go func() {
			if !appIsLive(a) {
				return
			}
			// notifyUser is runs.go's package-level seam over notify.NotifyUser —
			// tests intercept it to assert on the notification without racing the
			// real push I/O against app teardown (same rationale as the
			// auto-disable notification's use of the seam).
			notifyUser(a, notify.NotifyParams{
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
}
