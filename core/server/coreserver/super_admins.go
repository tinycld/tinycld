package coreserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

var errUserNotFound = errors.New("no user found matching that id or email")

// Super-admin roster management. These endpoints back the "Super Admins" panel
// in the admin console: list current super admins, grant a user, revoke a user.
//
// AuthZ: guarded by requireAdmin (a PB superuser OR an existing super-admin app
// user). That means an existing super admin can mint another — acceptable for
// v1; the alternative (superuser-only grants) would force every grant through
// the raw PocketBase admin UI. Revocation is likewise admin-guarded; there is
// deliberately no "you can't revoke yourself" guard — locking yourself out is
// recoverable via the PB superuser.
//
// The list endpoint runs in the app's Go context (bypasses the collection's
// self-only read rule), so it can return the full roster with each user's
// name/email expanded — something the client can't read directly.

type grantSuperAdminRequest struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
}

type superAdminRow struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

func RegisterSuperAdminEndpoints(app *pocketbase.PocketBase) {
	registerSuperAdminEndpointsCore(app)
}

func registerSuperAdminEndpointsCore(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/admin/super-admins")

		adminGuard := func(re *core.RequestEvent) error {
			return requireAdmin(app, re)
		}

		g.GET("", func(re *core.RequestEvent) error {
			return handleListSuperAdmins(app, re)
		}).BindFunc(adminGuard)

		g.POST("", func(re *core.RequestEvent) error {
			return handleGrantSuperAdmin(app, re)
		}).BindFunc(adminGuard)

		g.DELETE("/{userId}", func(re *core.RequestEvent) error {
			return handleRevokeSuperAdmin(app, re)
		}).BindFunc(adminGuard)

		return e.Next()
	})
}

func handleListSuperAdmins(app core.App, re *core.RequestEvent) error {
	records, err := app.FindRecordsByFilter("super_admins", "", "-created", 0, 0)
	if err != nil {
		return re.InternalServerError("Failed to list super admins", err)
	}

	rows := make([]superAdminRow, 0, len(records))
	for _, rec := range records {
		userID := rec.GetString("user")
		row := superAdminRow{ID: rec.Id, UserID: userID}
		if user, err := app.FindRecordById("users", userID); err == nil {
			row.Name = user.GetString("name")
			row.Email = user.GetString("email")
		}
		rows = append(rows, row)
	}

	return re.JSON(http.StatusOK, map[string]any{"superAdmins": rows})
}

func handleGrantSuperAdmin(app core.App, re *core.RequestEvent) error {
	var req grantSuperAdminRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	user, err := resolveGrantTarget(app, req)
	if err != nil {
		return re.BadRequestError(err.Error(), nil)
	}

	if isSuperAdmin(app, user.Id) {
		return re.BadRequestError("User is already a super admin", nil)
	}

	collection, err := app.FindCollectionByNameOrId("super_admins")
	if err != nil {
		return re.InternalServerError("Failed to find super_admins collection", err)
	}

	record := core.NewRecord(collection)
	record.Set("user", user.Id)
	// created_by is a relation into the users collection. A PB-superuser request
	// still carries a non-nil re.Auth (its id lives in _superusers, not users),
	// so recording it here fails the relation validation with a 500. Only stamp
	// created_by for an app-user grantor; leave it empty for a superuser.
	if re.Auth != nil && !re.Auth.IsSuperuser() {
		record.Set("created_by", re.Auth.Id)
	}
	if err := app.Save(record); err != nil {
		return re.InternalServerError("Failed to grant super admin", err)
	}

	return re.JSON(http.StatusOK, superAdminRow{
		ID:     record.Id,
		UserID: user.Id,
		Name:   user.GetString("name"),
		Email:  user.GetString("email"),
	})
}

func handleRevokeSuperAdmin(app core.App, re *core.RequestEvent) error {
	userID := re.Request.PathValue("userId")
	if userID == "" {
		return re.BadRequestError("userId is required", nil)
	}

	record, err := app.FindFirstRecordByFilter(
		"super_admins", "user = {:user}", map[string]any{"user": userID})
	if err != nil {
		return re.NotFoundError("User is not a super admin", nil)
	}

	if err := app.Delete(record); err != nil {
		return re.InternalServerError("Failed to revoke super admin", err)
	}

	return re.JSON(http.StatusOK, map[string]any{"ok": true})
}

// resolveGrantTarget finds the users record for a grant request by id or email.
func resolveGrantTarget(app core.App, req grantSuperAdminRequest) (*core.Record, error) {
	if id := strings.TrimSpace(req.UserID); id != "" {
		user, err := app.FindRecordById("users", id)
		if err != nil {
			return nil, errUserNotFound
		}
		return user, nil
	}
	if email := strings.TrimSpace(req.Email); email != "" {
		user, err := app.FindFirstRecordByFilter(
			"users", "email = {:email}", map[string]any{"email": email})
		if err != nil {
			return nil, errUserNotFound
		}
		return user, nil
	}
	return nil, errUserNotFound
}
