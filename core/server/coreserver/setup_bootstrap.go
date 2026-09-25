package coreserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"tinycld.org/core/approutes"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

var setupState = newSetupGuard(time.Now, announceSetupCode)

// announceSetupCode prints the code on its own line and inside the link, so
// a person can either click the link or type the code into any client
// (a native app has no link to click).
func announceSetupCode(code string) {
	printBoxed(
		"Finish setup in your browser:",
		setupPublicURL()+approutes.Href("setup")+"?code="+code,
		"Setup code: "+formatSetupCode(code),
	)
}

var setupBaseURL string

// setupPublicURL prefers TINYCLD_PUBLIC_URL so the printed link matches where
// the person browses; PocketBase's baseURL is the bind address, which is
// wrong behind a reverse proxy.
func setupPublicURL() string {
	if publicURL := strings.TrimRight(os.Getenv("TINYCLD_PUBLIC_URL"), "/"); publicURL != "" {
		return publicURL
	}
	return strings.TrimRight(setupBaseURL, "/")
}

func RegisterSetupBootstrap(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Sync AppURL from TINYCLD_PUBLIC_URL on every boot so all server-side
		// URL builders (email links, share URLs, webhook URLs) use the correct
		// public address instead of PocketBase's bind address default.
		if publicURL := strings.TrimRight(os.Getenv("TINYCLD_PUBLIC_URL"), "/"); publicURL != "" {
			app.Settings().Meta.AppURL = publicURL
			if err := app.Save(app.Settings()); err != nil {
				srvLog.Error("setup bootstrap: failed to persist AppURL from TINYCLD_PUBLIC_URL", "err", err)
			}
		}

		// setupBaseURL is written once here, at boot, before any request is
		// served, then only ever read — no concurrent access to guard.
		e.InstallerFunc = func(_ core.App, _ *core.Record, baseURL string) error {
			setupBaseURL = baseURL
			return setupState.Issue()
		}

		e.Router.GET("/api/setup/check", func(re *core.RequestEvent) error {
			return re.JSON(http.StatusOK, map[string]bool{"needsSetup": setupState.NeedsSetup()})
		})
		e.Router.POST("/api/setup/verify", func(re *core.RequestEvent) error {
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
				return re.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
			}
			if result := setupState.Check(re.RealIP(), body.Code); result != checkOK {
				return setupRefusal(re, result)
			}
			return re.JSON(http.StatusOK, map[string]any{})
		})
		e.Router.POST("/api/setup/init", func(re *core.RequestEvent) error {
			var req setupInitRequest
			if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
				return re.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
			}
			body, status := runSetupInit(app, setupState, re.RealIP(), req)
			return re.JSON(status, body)
		})
		return e.Next()
	})
}

type setupInitRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AppURL   string `json:"appUrl"`
}

var setupRefusalBodies = map[checkResult]map[string]string{
	checkMismatch: {"error": "That code does not match. Check the server log for the latest code.", "reason": "mismatch"},
	checkLocked:   {"error": "Too many tries. Wait 10 minutes, or restart the server for a new code.", "reason": "locked"},
	checkNoSetup:  {"error": "This server is already set up.", "reason": "done"},
}

func setupRefusal(re *core.RequestEvent, result checkResult) error {
	return re.JSON(http.StatusForbidden, setupRefusalBodies[result])
}

// runSetupInit is the handler body, split out so tests drive it without HTTP.
func runSetupInit(app core.App, guard *setupGuard, ip string, req setupInitRequest) (any, int) {
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		return map[string]string{"error": "Email and password are required."}, http.StatusBadRequest
	}
	var operator *core.Record
	result, err := guard.Consume(ip, req.Code, func() error {
		created, cerr := createSetupOwner(app, req)
		operator = created
		return cerr
	})
	if result != checkOK {
		return setupRefusalBodies[result], http.StatusForbidden
	}
	if err != nil {
		srvLog.Error("setup: owner creation failed", "err", err)
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}
	authToken, err := operator.NewAuthToken()
	if err != nil {
		return map[string]string{"error": "Owner created but failed to generate auth token."}, http.StatusInternalServerError
	}
	return map[string]string{"authToken": authToken, "email": req.Email, "userId": operator.Id}, http.StatusOK
}

// createSetupOwner mints both identities (see createOperatorIdentities for
// why there are two), saves the app URL and starts the wizard. It runs inside
// the guard's lock.
//
// Bootstrap two identities for the first operator:
//  1. a PocketBase _superusers record — keeps PB's installer satisfied (so
//     the setup token isn't re-printed on every reboot), backs the sharelink
//     signing key, and remains a recovery login.
//  2. a regular `users` record with role=owner — this is the identity the
//     /admin console actually runs as. The console writes through the
//     app's pbtsdb stores (the shared app pb client), and those writes
//     must carry an auth that satisfies the users `manageRule` (the
//     owner/admin clause) to set managed fields like `verified` when
//     creating a pre-verified user. A raw _superusers token on a
//     throwaway client never reached those stores, which is why org creation
//     failed with a 400 on `verified`. We therefore mint the returned auth
//     token from the `users` record and the client saves it onto the shared
//     pb instance.
func createSetupOwner(app core.App, req setupInitRequest) (*core.Record, error) {
	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return nil, fmt.Errorf("find superusers collection: %w", err)
	}
	superuser := core.NewRecord(superusers)
	superuser.SetEmail(req.Email)
	superuser.SetPassword(req.Password)
	superuser.SetVerified(true)
	if err := app.Save(superuser); err != nil {
		return nil, fmt.Errorf("create superuser: %w", err)
	}
	operator, err := createOwnerOperator(app, req.Email, req.Name, req.Password)
	if err != nil {
		return nil, fmt.Errorf("create owner: %w", err)
	}
	if req.AppURL != "" {
		app.Settings().Meta.AppURL = req.AppURL
		if err := app.Save(app.Settings()); err != nil {
			srvLog.Warn("setup: failed to save app URL", "err", err)
		}
	}
	if err := MarkSetupWizardStarted(app); err != nil {
		srvLog.Warn("setup: could not start the setup wizard", "err", err)
	}
	return operator, nil
}

// IsBcryptHash reports whether v looks like a bcrypt hash ("$2" prefix, the
// same check PocketBase's own password field uses to recognise one). Callers
// that will write a hash into more than one record (a superuser and a users
// record sharing one credential) must validate it once, up front, before
// touching either — writing an unchecked value into a raw password field
// succeeds (ValidateValue skips its checks when Plain is empty), so the
// invalid value would otherwise land permanently in whichever record is
// written first.
func IsBcryptHash(v string) bool {
	return strings.HasPrefix(v, "$2")
}

// CreateOwnerAccountWithHash mints the owner from an already-computed bcrypt
// hash instead of a plaintext password. A caller that holds only the hash of
// a password chosen elsewhere — never the plaintext itself — uses this, so
// the plaintext never crosses a process boundary.
// name is the display name; empty falls back to the email local-part, the
// same default the plaintext path uses.
func CreateOwnerAccountWithHash(app core.App, email, name, passwordHash string) (*core.Record, error) {
	if !IsBcryptHash(passwordHash) {
		return nil, fmt.Errorf("password hash must be a bcrypt hash")
	}
	operator, err := newOwnerRecord(app, email, name)
	if err != nil {
		return nil, err
	}
	// The password field's setter (SetPassword) hashes a PLAIN value; a
	// precomputed hash goes in raw via PasswordFieldValue so it is stored
	// verbatim instead of being re-hashed.
	operator.SetRaw(core.FieldNamePassword, &core.PasswordFieldValue{Hash: passwordHash})
	operator.RefreshTokenKey()
	if err := app.Save(operator); err != nil {
		return nil, fmt.Errorf("create users record: %w", err)
	}
	return operator, nil
}

// Exported as CreateOwnerAccount for callers outside the setup wizard — a
// hosted org is provisioned by a router, which has no wizard to run (that
// route is bound only in the host composition) and would otherwise leave the
// org with zero users. Both paths must mint the SAME shape of account, so
// they share this one implementation rather than each assembling the record.
func CreateOwnerAccount(app core.App, email, password string) (*core.Record, error) {
	return createOwnerOperator(app, email, "", password)
}

// CreateOwnerAccountNamed is CreateOwnerAccount with an explicit display
// name. Kept as a separate export, rather than changing CreateOwnerAccount's
// signature, so existing callers of the two-arg form keep compiling.
func CreateOwnerAccountNamed(app core.App, email, name, password string) (*core.Record, error) {
	return createOwnerOperator(app, email, name, password)
}

// createOwnerOperator creates the first operator as a regular `users` record
// with role=owner. Returns the users record so the caller can mint its auth
// token. This runs in the app's Go context, which bypasses record rules, so
// the insert is authorized without an authenticated superuser.
func createOwnerOperator(app core.App, email, name, password string) (*core.Record, error) {
	operator, err := newOwnerRecord(app, email, name)
	if err != nil {
		return nil, err
	}
	operator.SetPassword(password)
	if err := app.Save(operator); err != nil {
		return nil, fmt.Errorf("create users record: %w", err)
	}
	return operator, nil
}

// newOwnerRecord assembles the owner record every path shares: verified,
// visible email, username from the shared helper, role=owner. Only the
// password is left to the caller.
func newOwnerRecord(app core.App, email, name string) (*core.Record, error) {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, fmt.Errorf("find users collection: %w", err)
	}
	if strings.TrimSpace(name) == "" {
		// `name` is required and is a human display label; seed it from the
		// email local-part (the operator can rename themselves later).
		name = strings.SplitN(email, "@", 2)[0]
	}
	operator := core.NewRecord(users)
	operator.SetEmail(email)
	operator.SetVerified(true)
	operator.Set("emailVisibility", true)
	operator.Set("name", name)
	// `username` is the unique handle, derived by the shared helper.
	operator.Set("username", DeriveUsername(email))
	// `role` is required (1940000000_backfill_and_require_users_role).
	//
	// owner is the whole of the operator's authority: it gates the /admin
	// console (requireAdmin admits owner/admin) and package management within
	// it (requireOwner), as well as Settings > Members and /api/invite-member.
	// Anything less leaves the wizard-runner unable to invite anyone or to
	// promote themselves, and the deployment permanently single-user.
	//
	// The person who ran the setup wizard is the deployment's owner, so this is
	// also the honest value. Standalone-only: RegisterSetupBootstrap is bound
	// in the host composition, never in a tenant.
	operator.Set("role", "owner")
	return operator, nil
}

func printBoxed(title string, lines ...string) {
	w := len(title) + 4
	for _, l := range lines {
		if lineW := len(l) + 4; lineW > w {
			w = lineW
		}
	}
	h := strings.Repeat("─", w)

	pad := func(s string) string {
		gap := w - len(s) - 2
		return "│ " + s + strings.Repeat(" ", gap) + " │"
	}

	fmt.Printf("\n┌%s┐\n", h)
	fmt.Printf("%s\n", pad(title))
	fmt.Printf("│%s│\n", strings.Repeat(" ", w))
	for _, l := range lines {
		fmt.Printf("%s\n", pad(l))
	}
	fmt.Printf("└%s┘\n\n", h)
}
