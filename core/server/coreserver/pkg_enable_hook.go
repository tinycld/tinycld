package coreserver

import "github.com/pocketbase/pocketbase/core"

// RegisterPkgEnableHook makes the server, not the client, choose a package's
// status when it is turned back on. "disabled" is shared by "the owner hid
// it" and "it left the build", so the client cannot tell which source to
// restore; bundled-packages.json can.
//
// Model-level hook so it covers app.Save() as well as API writes.
func RegisterPkgEnableHook(app core.App) {
	app.OnRecordUpdate("pkg_registry").BindFunc(func(e *core.RecordEvent) error {
		wasDisabled := e.Record.Original().GetString("status") == "disabled"
		next := e.Record.GetString("status")
		if wasDisabled && next != "disabled" && next != "available" {
			if bundledSlugSet()[e.Record.GetString("slug")] {
				e.Record.Set("status", "bundled")
			} else {
				e.Record.Set("status", "installed")
			}
		}
		return e.Next()
	})
}
