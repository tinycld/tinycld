package coreserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/userorg"
)

// accountDeleteRequest is the per-call payload. plans is keyed by user_org
// record ID and lets the client send one Plan per org the user is in. A
// missing entry defaults to {mode: "reassign"} with no successor, which
// triggers the server's auto-pick (oldest owner). Sole-member orgs ignore
// the plan and force ModeDeleteOrg server-side.
type accountDeleteRequest struct {
	Email string                  `json:"email"`
	Plans map[string]userorg.Plan `json:"plans"`
}

// RegisterAccountDelete wires POST /api/account/delete onto the app. This
// endpoint is the multi-org orchestrator on top of userorg.LeaveOrg —
// account-level delete is just "leave every org I'm in, then anonymize."
func RegisterAccountDelete(app *pocketbase.PocketBase) {
	registerAccountDeleteCore(app)
}

func registerAccountDeleteCore(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/account/delete", func(re *core.RequestEvent) error {
			return handleAccountDelete(app, re)
		}).BindFunc(requireAuthCore)
		return e.Next()
	})
}

func handleAccountDelete(app core.App, re *core.RequestEvent) error {
	authRecord := re.Auth
	if authRecord == nil || authRecord.Collection().Name != "users" {
		return re.UnauthorizedError("Authentication required", nil)
	}

	var req accountDeleteRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	currentEmail := strings.ToLower(strings.TrimSpace(authRecord.GetString("email")))
	providedEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if providedEmail == "" || providedEmail != currentEmail {
		return re.BadRequestError("email confirmation does not match", nil)
	}

	memberships, err := app.FindRecordsByFilter(
		"user_org",
		"user = {:uid}",
		"-created",
		0, 0,
		map[string]any{"uid": authRecord.Id},
	)
	if err != nil {
		return re.InternalServerError("load memberships", err)
	}

	// Per-org loop. Each LeaveOrg is its own transaction — a failure on
	// org 3 of 5 leaves the first two completed and the user un-anonymized,
	// so retrying the call picks up where it left off.
	for _, m := range memberships {
		plan, ok := req.Plans[m.Id]
		if !ok {
			// No plan supplied: default to reassign with auto-picked
			// successor. Sole-member orgs are force-overridden by the
			// server to ModeDeleteOrg.
			plan = userorg.Plan{Mode: userorg.ModeReassign}
		}
		if _, err := userorg.LeaveOrgAs(app, m.Id, plan, true, authRecord.Id); err != nil {
			return re.InternalServerError(
				fmt.Sprintf("leave org %s failed", m.GetString("org")), err,
			)
		}
	}

	// Anonymization happens inside the final LeaveOrg call (the one that
	// removed the user's last user_org). If the user had zero orgs to start
	// with, the loop above ran zero times and we still need to anonymize.
	if len(memberships) == 0 {
		if err := userorg.AnonymizeUser(app, authRecord.Id); err != nil {
			return re.InternalServerError("anonymize", err)
		}
	}

	return re.NoContent(204)
}

// randomHex returns N random bytes encoded as hex. Lives here as the
// coreserver-package-local helper; demo_start.go is the actual caller. The
// userorg package has its own copy for the anonymize-user path. Both are
// trivial enough that a shared util would be more friction than the
// duplication is worth — if a third package needs it, lift to tools/.
func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
