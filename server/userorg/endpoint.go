package userorg

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Register wires the leave-org endpoints onto the app router. Call from the
// core server's bootstrap (server.go) alongside the other Register* funcs.
func Register(app *pocketbase.PocketBase) {
	registerCore(app)
}

func registerCore(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/account/leave-org", func(re *core.RequestEvent) error {
			return handleLeaveOrg(app, re)
		}).BindFunc(requireAuth)

		e.Router.GET("/api/account/leave-org/preview", func(re *core.RequestEvent) error {
			return handlePreview(app, re)
		}).BindFunc(requireAuth)

		return e.Next()
	})
}

func requireAuth(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.Next()
}

type leaveOrgRequest struct {
	UserOrgID string `json:"user_org_id"`
	Plan      Plan   `json:"plan"`
}

func handleLeaveOrg(app core.App, re *core.RequestEvent) error {
	authUser := re.Auth
	if authUser == nil || authUser.Collection().Name != "users" {
		return re.UnauthorizedError("Authentication required", nil)
	}

	var req leaveOrgRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if req.UserOrgID == "" {
		return re.BadRequestError("user_org_id is required", nil)
	}

	leaver, err := app.FindRecordById("user_org", req.UserOrgID)
	if err != nil {
		return re.NotFoundError("user_org not found", err)
	}

	actorIsLeaver, err := authorizeLeaveOrg(app, authUser, leaver)
	if err != nil {
		return re.ForbiddenError(err.Error(), err)
	}

	result, err := LeaveOrgAs(app, req.UserOrgID, req.Plan, actorIsLeaver, authUser.Id)
	if err != nil {
		// ErrInvalidPlan is a client error; everything else is a 500.
		if errors.Is(err, ErrInvalidPlan) {
			return re.BadRequestError(err.Error(), err)
		}
		return re.InternalServerError("leave org failed", err)
	}

	return re.JSON(http.StatusOK, result)
}

// authorizeLeaveOrg decides whether authUser may call leave-org against the
// given leaver user_org. Returns actorIsLeaver=true when authUser IS the
// leaver (self-leave), false when authUser is an authorized admin/owner
// removing someone else, or an error explaining the rejection.
//
// Authorization rules:
//   - Self-leave is always allowed.
//   - For admin-driven removal:
//   - the caller must be an owner OR admin of the same org;
//   - an admin (not owner) cannot remove an owner. Owner-to-owner removal
//     is fine; the org keeps its owner pool. This blocks the
//     admin-elevates-by-removing-all-owners attack: even after removing
//     every owner, an admin can't actually trigger any of those removals.
//   - the caller can't admin-remove themselves — that's a self-leave
//     control-flow violation and probably indicates a buggy client.
func authorizeLeaveOrg(app core.App, authUser, leaver *core.Record) (bool, error) {
	leaverUserID := leaver.GetString("user")
	orgID := leaver.GetString("org")

	if authUser.Id == leaverUserID {
		return true, nil
	}

	callerMemberships, err := app.FindRecordsByFilter(
		"user_org",
		"user = {:uid} && org = {:org}",
		"", 1, 0,
		map[string]any{"uid": authUser.Id, "org": orgID},
	)
	if err != nil || len(callerMemberships) == 0 {
		return false, fmt.Errorf("only the user themselves or an org owner/admin can leave-org a member")
	}

	callerRole := callerMemberships[0].GetString("role")
	if callerRole != "owner" && callerRole != "admin" {
		return false, fmt.Errorf("only the user themselves or an org owner/admin can leave-org a member")
	}

	leaverRole := leaver.GetString("role")
	if callerRole == "admin" && leaverRole == "owner" {
		return false, fmt.Errorf("admins cannot remove owners; an owner must do it")
	}

	return false, nil
}

// previewResponse drives the UI's "what's about to happen" panel. counts is
// keyed by "<collection>.<field>" so the client can label rows ("23 Drive
// items", "7 Calendar events"). peers lists the remaining org members with
// their role and display info so the picker can render them.
type previewResponse struct {
	OrgID      string            `json:"org_id"`
	OrgName    string            `json:"org_name"`
	SoleMember bool              `json:"sole_member"`
	SoleOwner  bool              `json:"sole_owner"`
	Counts     map[string]int    `json:"counts"`
	Peers      []previewPeerInfo `json:"peers"`
}

type previewPeerInfo struct {
	UserOrgID string `json:"user_org_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
}

func handlePreview(app core.App, re *core.RequestEvent) error {
	authUser := re.Auth
	if authUser == nil || authUser.Collection().Name != "users" {
		return re.UnauthorizedError("Authentication required", nil)
	}

	userOrgID := re.Request.URL.Query().Get("user_org_id")
	if userOrgID == "" {
		return re.BadRequestError("user_org_id query param is required", nil)
	}

	leaver, err := app.FindRecordById("user_org", userOrgID)
	if err != nil {
		return re.NotFoundError("user_org not found", err)
	}
	orgID := leaver.GetString("org")

	if _, err := authorizeLeaveOrg(app, authUser, leaver); err != nil {
		return re.ForbiddenError(err.Error(), err)
	}

	peers, err := loadOrgPeers(app, orgID, userOrgID)
	if err != nil {
		return re.InternalServerError("load peers", err)
	}

	otherOwners := 0
	for _, p := range peers {
		if p.Role == "owner" {
			otherOwners++
		}
	}
	soleMember := len(peers) == 0
	soleOwner := !soleMember && leaver.GetString("role") == "owner" && otherOwners == 0

	counts := make(map[string]int)
	for _, ref := range RegisteredReassignable() {
		if !collectionExists(app, ref.Collection) {
			continue
		}
		var n int
		err := app.DB().NewQuery(fmt.Sprintf(
			"SELECT COUNT(*) AS c FROM %s WHERE %s = {:leaver}",
			ref.Collection, ref.Field,
		)).Bind(dbx.Params{"leaver": userOrgID}).Row(&n)
		if err != nil {
			// Don't fail the whole preview on one bad count; surface 0 and log.
			app.Logger().Warn("leave-org preview count failed",
				"collection", ref.Collection, "field", ref.Field, "error", err)
			continue
		}
		if n > 0 {
			counts[ref.Collection+"."+ref.Field] = n
		}
	}

	peerInfos := make([]previewPeerInfo, 0, len(peers))
	for _, p := range peers {
		info := previewPeerInfo{
			UserOrgID: p.UserOrgID,
			UserID:    p.UserID,
			Role:      p.Role,
		}
		if user, err := app.FindRecordById("users", p.UserID); err == nil {
			info.Name = user.GetString("name")
			info.Email = user.GetString("email")
		}
		peerInfos = append(peerInfos, info)
	}

	var orgName string
	if org, err := app.FindRecordById("orgs", orgID); err == nil {
		orgName = org.GetString("name")
	}

	return re.JSON(http.StatusOK, previewResponse{
		OrgID:      orgID,
		OrgName:    orgName,
		SoleMember: soleMember,
		SoleOwner:  soleOwner,
		Counts:     counts,
		Peers:      peerInfos,
	})
}
