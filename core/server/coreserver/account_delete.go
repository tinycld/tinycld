package coreserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/offboard"
)

// accountDeleteRequest is the per-call payload. The caller confirms by
// re-typing their own email, and chooses what happens to the content they
// authored.
type accountDeleteRequest struct {
	Email string        `json:"email"`
	Plan  offboard.Plan `json:"plan"`
}

// accountDisableRequest confirms a self-disable by re-typed email — the same
// shape as delete, so the client has one confirmation pattern.
type accountDisableRequest struct {
	Email string `json:"email"`
}

// accountEnableRequest names the user to restore. Admin-only.
type accountEnableRequest struct {
	UserID string `json:"user_id"`
}

// RegisterAccountDelete wires the self-service account routes:
//
//	POST /api/account/delete   — irreversible: settle authored content, then anonymize
//	POST /api/account/disable  — reversible: suspend the caller's own account
//	POST /api/account/enable   — admin-only: restore a suspended account
//
// Single-org: these replace the former /api/account/leave-org endpoints, which
// were pure cross-org membership management. "Leaving" is meaningless when the
// deployment IS the org, so the choice is suspend or delete.
func RegisterAccountDelete(app *pocketbase.PocketBase) {
	registerAccountDeleteCore(app)
}

func registerAccountDeleteCore(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/account/delete", func(re *core.RequestEvent) error {
			return handleAccountDelete(app, re)
		}).BindFunc(requireAuthCore)

		e.Router.POST("/api/account/disable", func(re *core.RequestEvent) error {
			return handleAccountDisable(app, re)
		}).BindFunc(requireAuthCore)

		e.Router.POST("/api/account/enable", func(re *core.RequestEvent) error {
			return handleAccountEnable(app, re)
		}).BindFunc(requireAuthCore)
		return e.Next()
	})
}

// requireSelfUser returns the caller's users record, rejecting superusers and
// any other auth collection. These endpoints act on the CALLER, so the subject
// is never a request parameter.
func requireSelfUser(re *core.RequestEvent) (*core.Record, error) {
	authRecord := re.Auth
	if authRecord == nil || authRecord.Collection().Name != "users" {
		return nil, re.UnauthorizedError("Authentication required", nil)
	}
	return authRecord, nil
}

// confirmEmailMatches enforces the type-your-own-email confirmation both
// destructive endpoints share.
func confirmEmailMatches(re *core.RequestEvent, authRecord *core.Record, provided string) error {
	currentEmail := strings.ToLower(strings.TrimSpace(authRecord.GetString("email")))
	providedEmail := strings.ToLower(strings.TrimSpace(provided))
	if providedEmail == "" || providedEmail != currentEmail {
		return re.BadRequestError("email confirmation does not match", nil)
	}
	return nil
}

func handleAccountDelete(app core.App, re *core.RequestEvent) error {
	authRecord, err := requireSelfUser(re)
	if err != nil {
		return err
	}

	var req accountDeleteRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if err := confirmEmailMatches(re, authRecord, req.Email); err != nil {
		return err
	}
	if err := requireNotLastActiveOwner(app, re, authRecord); err != nil {
		return err
	}

	// An omitted plan means "just anonymize me, leave what I authored" — the
	// long-standing behavior, and the safe default: a malformed or truncated
	// request must never be read as "delete everything I ever wrote".
	// Reassign and delete_my_data stay explicit opt-ins.
	if strings.TrimSpace(string(req.Plan.Mode)) == "" {
		if err := offboard.AnonymizeUser(app, authRecord.Id); err != nil {
			return re.InternalServerError("anonymize", err)
		}
		return re.NoContent(204)
	}

	// Route through OffboardUser rather than anonymizing directly: it settles
	// authored content (reassign to a successor, or delete it) AND anonymizes
	// in one transaction, so a failure part-way leaves no orphaned rows. The
	// previous implementation called anonymizeUser unconditionally, so a
	// caller's choice was silently ignored and content always stayed behind.
	result, err := offboard.OffboardUser(app, authRecord.Id, req.Plan, authRecord.Id)
	if err != nil {
		if errors.Is(err, offboard.ErrInvalidPlan) {
			return re.BadRequestError(err.Error(), err)
		}
		return re.InternalServerError("offboard", err)
	}

	return re.JSON(200, result)
}

func handleAccountDisable(app core.App, re *core.RequestEvent) error {
	authRecord, err := requireSelfUser(re)
	if err != nil {
		return err
	}

	var req accountDisableRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if err := confirmEmailMatches(re, authRecord, req.Email); err != nil {
		return err
	}
	if err := requireNotLastActiveOwner(app, re, authRecord); err != nil {
		return err
	}

	// This endpoint exists (rather than a plain record update) because the
	// users field guard keeps `disabled` out of selfEditableUserFields — a
	// suspended user must not be able to clear their own flag. Setting it here
	// re-verifies identity by email confirmation first.
	authRecord.Set("disabled", true)

	// Invalidate every issued token. Without this the caller's current session
	// — and any other signed-in device — keeps working until the JWT expires,
	// so the suspension wouldn't take effect for hours. The cost is that
	// restoring the account requires a fresh sign-in, which is the right trade.
	authRecord.RefreshTokenKey()

	if err := app.Save(authRecord); err != nil {
		return re.InternalServerError("disable account", err)
	}
	return re.NoContent(204)
}

func handleAccountEnable(app core.App, re *core.RequestEvent) error {
	authRecord := re.Auth
	if authRecord == nil || authRecord.Collection().Name != "users" {
		return re.UnauthorizedError("Authentication required", nil)
	}
	// Restoring an account is an administrative act: the subject cannot ask for
	// it themselves (they can no longer authenticate), and a peer must not be
	// able to undo a suspension.
	if !isOrgAdmin(authRecord) {
		return re.ForbiddenError("Admin access required", nil)
	}

	var req accountEnableRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if strings.TrimSpace(req.UserID) == "" {
		return re.BadRequestError("user_id is required", nil)
	}

	target, err := app.FindRecordById("users", req.UserID)
	if err != nil {
		return re.NotFoundError("user not found", err)
	}
	target.Set("disabled", false)
	if err := app.Save(target); err != nil {
		return re.InternalServerError("enable account", err)
	}
	return re.NoContent(204)
}

// randomHex returns N random bytes encoded as hex. Lives here as the
// coreserver-package-local helper; demo_start.go is the actual caller.
func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
