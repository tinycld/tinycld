package coreserver

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/offboard"
)

// adminOffboardRequest names the user to offboard plus what happens to the
// content they authored.
type adminOffboardRequest struct {
	UserID string       `json:"user_id"`
	Plan   offboard.Plan `json:"plan"`
}

// RegisterAdminOffboard wires POST /api/admin/users/offboard.
//
// Separate from /api/account/delete because that route is self-only by
// construction: it identifies the subject as re.Auth and confirms by the
// caller's own email. An admin removing someone else can do neither — they
// cannot type the target's email, and the subject is a parameter. Folding both
// into one handler would mean an endpoint whose subject is sometimes the caller
// and sometimes not, which is exactly the shape authorization bugs like.
func RegisterAdminOffboard(app *pocketbase.PocketBase) {
	registerAdminOffboardCore(app)
}

func registerAdminOffboardCore(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/admin/users/offboard", func(re *core.RequestEvent) error {
			return handleAdminOffboard(app, re)
		}).BindFunc(requireAuthCore)
		return e.Next()
	})
}

func handleAdminOffboard(app core.App, re *core.RequestEvent) error {
	authRecord := re.Auth
	if authRecord == nil || authRecord.Collection().Name != "users" {
		return re.UnauthorizedError("Authentication required", nil)
	}
	if !isOrgAdmin(authRecord) {
		return re.ForbiddenError("Admin access required", nil)
	}

	var req adminOffboardRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if strings.TrimSpace(req.UserID) == "" {
		return re.BadRequestError("user_id is required", nil)
	}
	// Self-offboard goes through /api/account/delete, which confirms by email.
	// Blocking it here stops an admin from bypassing that confirmation on
	// themselves — and from an accidental click removing the last owner.
	if req.UserID == authRecord.Id {
		return re.BadRequestError("use /api/account/delete to remove your own account", nil)
	}

	// actorUserID is the admin, recorded so the offboard is attributable.
	result, err := offboard.OffboardUser(app, req.UserID, req.Plan, authRecord.Id)
	if err != nil {
		if errors.Is(err, offboard.ErrInvalidPlan) {
			return re.BadRequestError(err.Error(), err)
		}
		return re.InternalServerError("offboard", err)
	}
	return re.JSON(200, result)
}
