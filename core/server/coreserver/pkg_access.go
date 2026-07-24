package coreserver

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterOrgPkgEnabledHooks adds authorization checks that PocketBase RQL
// alone cannot express: the requesting user must hold an owner/admin role.
func RegisterOrgPkgEnabledHooks(app *pocketbase.PocketBase) {
	check := func(e *core.RecordRequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("Authentication required", nil)
		}
		if !isOrgAdmin(e.Auth) {
			return e.ForbiddenError("Org admin role required", nil)
		}
		return e.Next()
	}

	app.OnRecordCreateRequest("org_pkg_enabled").BindFunc(check)
	app.OnRecordUpdateRequest("org_pkg_enabled").BindFunc(check)
	app.OnRecordDeleteRequest("org_pkg_enabled").BindFunc(check)
}
