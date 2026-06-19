package coreserver

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Cross-org impersonation for the admin console's Organizations tab.
//
// Listing/creating/editing orgs is done by the client through the pbtsdb stores:
// the orgs/user_org/users/mail_domains collections carry a super_admins clause in
// their API rules (see the pkg_admin_super_admin_rules migration), so a
// super-admin's collection CRUD is authorized without a custom endpoint.
//
// Impersonation is the one action that CAN'T be a collection rule: it mints an
// auth token for ANOTHER user, a privileged server action with no record-rule
// equivalent. So it stays a requireAdmin-guarded endpoint, running in the app's
// Go context.

type orgAdminOwner struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func RegisterOrgAdminEndpoints(app *pocketbase.PocketBase) {
	registerOrgAdminEndpointsCore(app)
}

func registerOrgAdminEndpointsCore(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/admin/orgs")

		adminGuard := func(re *core.RequestEvent) error {
			return requireAdmin(app, re)
		}

		g.POST("/{orgId}/impersonate", func(re *core.RequestEvent) error {
			return handleImpersonateOwner(app, re)
		}).BindFunc(adminGuard)

		return e.Next()
	})
}

// orgOwner returns the owner user record of an org (the user_org row with
// role=owner), or nil if the org has no owner.
func orgOwner(app core.App, orgID string) *core.Record {
	membership, err := app.FindFirstRecordByFilter(
		"user_org",
		"org = {:org} && role = 'owner'",
		dbx.Params{"org": orgID},
	)
	if err != nil {
		return nil
	}
	user, err := app.FindRecordById("users", membership.GetString("user"))
	if err != nil {
		return nil
	}
	return user
}

// handleImpersonateOwner mints an auth token for the org's owner so an admin can
// enter that org's workspace. Minting a token for another user is a superuser-
// level action in PocketBase; gating it behind requireAdmin + running it in the
// app's Go context is what lets a super-admin (non-superuser) do it.
func handleImpersonateOwner(app core.App, re *core.RequestEvent) error {
	orgID := re.Request.PathValue("orgId")
	if orgID == "" {
		return re.BadRequestError("orgId is required", nil)
	}

	owner := orgOwner(app, orgID)
	if owner == nil {
		return re.NotFoundError("Organization has no owner to impersonate", nil)
	}

	token, err := owner.NewAuthToken()
	if err != nil {
		return re.InternalServerError("Failed to mint impersonation token", err)
	}

	return re.JSON(http.StatusOK, map[string]any{
		"token": token,
		"owner": orgAdminOwner{ID: owner.Id, Name: owner.GetString("name"), Email: owner.GetString("email")},
	})
}
