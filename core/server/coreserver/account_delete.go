package coreserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/userorg"
)

// accountDeleteRequest is the per-call payload. The caller confirms by
// re-typing their own email.
type accountDeleteRequest struct {
	Email string `json:"email"`
}

// RegisterAccountDelete wires POST /api/account/delete onto the app. Single-org:
// account delete just anonymizes the caller's users record — there is no org
// junction to unwind.
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

	if err := userorg.AnonymizeUser(app, authRecord.Id); err != nil {
		return re.InternalServerError("anonymize", err)
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
