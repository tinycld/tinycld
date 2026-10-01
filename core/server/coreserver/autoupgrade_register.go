package coreserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/autoupgrade"
	"tinycld.org/core/syscfg"
)

// RegisterAutoUpgrade binds the same hooks in every composition; which
// Delegate (if any) answers them is decided by whoever composed the app.
func RegisterAutoUpgrade(app *pocketbase.PocketBase) {
	registerAutoUpgradeOn(app)
}

func registerAutoUpgradeOn(app core.App) {
	app.OnRecordCreateRequest("system_settings").BindFunc(guardAutoUpgradeWrite)
	app.OnRecordUpdateRequest("system_settings").BindFunc(guardAutoUpgradeWrite)
	app.OnRecordDeleteRequest("system_settings").BindFunc(guardAutoUpgradeWrite)
	app.OnRecordAfterCreateSuccess("system_settings").BindFunc(notifyPolicy)
	app.OnRecordAfterUpdateSuccess("system_settings").BindFunc(notifyPolicy)

	ctx, cancel := context.WithCancel(context.Background())
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		cancel()
		return e.Next()
	})
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/admin/packages/auto-upgrade/status", func(re *core.RequestEvent) error {
			return handleAutoUpgradeStatus(re)
		}).BindFunc(requireOwner)

		if d := autoupgrade.Current(); d != nil {
			enabled := readSystemSetting(e.App, autoupgrade.KeyEnabled) == "true"
			if err := d.PolicyChanged(ctx, enabled); err != nil {
				srvLog.Warn("auto-upgrade: boot policy push failed", "err", err)
			}
			if s, ok := d.(autoupgrade.Starter); ok {
				s.Start(ctx)
			}
		}
		return e.Next()
	})
}

// guardAutoUpgradeWrite narrows the admin-wide system_settings rule: the
// upgrade policy is the owner's decision, as package installs are.
func guardAutoUpgradeWrite(e *core.RecordRequestEvent) error {
	if !strings.HasPrefix(e.Record.GetString("key"), "autoupgrade.") {
		return e.Next()
	}
	if e.HasSuperuserAuth() || isOwner(e.Auth) {
		return e.Next()
	}
	return e.ForbiddenError("Only the owner can change automatic updates.", nil)
}

func notifyPolicy(e *core.RecordEvent) error {
	if e.Record.GetString("key") == autoupgrade.KeyEnabled {
		if d := autoupgrade.Current(); d != nil {
			if err := d.PolicyChanged(context.Background(), e.Record.GetString("value") == "true"); err != nil {
				srvLog.Warn("auto-upgrade: policy push failed", "err", err)
			}
		}
	}
	return e.Next()
}

func handleAutoUpgradeStatus(re *core.RequestEvent) error {
	st := autoupgrade.Unavailable("This build cannot update itself. Download a new release to update.")
	if d := autoupgrade.Current(); d != nil {
		s, err := d.Status(re.Request.Context())
		if err != nil {
			return re.InternalServerError("Failed to read update status", err)
		}
		st = s
	}
	return re.JSON(http.StatusOK, map[string]any{
		"windowManaged": syscfg.IsManaged(autoupgrade.KeyWindow),
		"status":        st,
	})
}
