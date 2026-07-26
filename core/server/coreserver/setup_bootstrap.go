package coreserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

var (
	setupToken   string
	setupTokenMu sync.Mutex
)

func RegisterSetupBootstrap(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Sync AppURL from TINYCLD_PUBLIC_URL on every boot so all server-side
		// URL builders (email links, share URLs, webhook URLs) use the correct
		// public address instead of PocketBase's bind address default.
		if publicURL := strings.TrimRight(os.Getenv("TINYCLD_PUBLIC_URL"), "/"); publicURL != "" {
			app.Settings().Meta.AppURL = publicURL
			if err := app.Save(app.Settings()); err != nil {
				log.Printf("setup bootstrap: failed to persist AppURL from TINYCLD_PUBLIC_URL: %v", err)
			}
		}

		// Replace PB's default installer with our own setup URL
		e.InstallerFunc = func(_ core.App, _ *core.Record, baseURL string) error {
			token, err := generateToken(32)
			if err != nil {
				return fmt.Errorf("setup bootstrap: failed to generate token: %w", err)
			}

			setupTokenMu.Lock()
			setupToken = token
			setupTokenMu.Unlock()

			// Prefer TINYCLD_PUBLIC_URL when set so the printed URL matches
			// where the user actually browses (dev: scripts/dev.ts proxy on
			// 7100; prod: whatever public URL fronts PB). PB's baseURL is
			// the bind address, which is wrong whenever a reverse proxy
			// sits in front of the server.
			publicURL := strings.TrimRight(os.Getenv("TINYCLD_PUBLIC_URL"), "/")
			if publicURL == "" {
				publicURL = strings.TrimRight(baseURL, "/")
			}
			setupURL := fmt.Sprintf("%s/setup?token=%s", publicURL, token)
			printBoxed("First run setup, visit below URL to configure tinycld:", setupURL)
			return nil
		}

		e.Router.GET("/api/setup/check", func(re *core.RequestEvent) error {
			setupTokenMu.Lock()
			hasToken := setupToken != ""
			setupTokenMu.Unlock()
			return re.JSON(http.StatusOK, map[string]bool{"needsSetup": hasToken})
		})

		e.Router.POST("/api/setup/init", func(re *core.RequestEvent) error {
			return handleSetupInit(app, re)
		})

		return e.Next()
	})
}

type setupInitRequest struct {
	Token    string `json:"token"`
	AppName  string `json:"appName"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AppURL   string `json:"appUrl"`
}

func handleSetupInit(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	setupTokenMu.Lock()
	currentToken := setupToken
	setupTokenMu.Unlock()

	if currentToken == "" {
		return re.JSON(http.StatusForbidden, map[string]string{
			"error": "Setup has already been completed.",
		})
	}

	var req setupInitRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body.",
		})
	}

	if req.Token != currentToken {
		return re.JSON(http.StatusForbidden, map[string]string{
			"error": "Invalid setup token.",
		})
	}

	if req.Email == "" || req.Password == "" {
		return re.JSON(http.StatusBadRequest, map[string]string{
			"error": "Email and password are required.",
		})
	}

	setupTokenMu.Lock()
	tokenStillValid := setupToken != ""
	setupTokenMu.Unlock()
	if !tokenStillValid {
		return re.JSON(http.StatusForbidden, map[string]string{
			"error": "Setup has already been completed.",
		})
	}

	// Bootstrap two identities for the first operator:
	//   1. a PocketBase _superusers record — keeps PB's installer satisfied (so
	//      the setup token isn't re-printed on every reboot), backs the sharelink
	//      signing key, and remains a recovery login.
	//   2. a regular `users` record promoted via a `super_admins` row — this is
	//      the identity the /admin console actually runs as. The console writes
	//      through the app's pbtsdb stores (the shared app pb client), and those
	//      writes must carry an auth that satisfies the users `manageRule`
	//      (@collection.super_admins...) to set managed fields like `verified`
	//      when creating a pre-verified org owner. A raw _superusers token on a
	//      throwaway client never reached those stores, which is why org creation
	//      failed with a 400 on `verified`. We therefore mint the returned auth
	//      token from the `users` record and the client saves it onto the shared
	//      pb instance.
	superusers, err := app.FindCollectionByNameOrId("_superusers")
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to find superusers collection.",
		})
	}
	superuser := core.NewRecord(superusers)
	superuser.SetEmail(req.Email)
	superuser.SetPassword(req.Password)
	superuser.SetVerified(true)
	if err := app.Save(superuser); err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to create superuser: %v", err),
		})
	}

	operator, err := createSuperAdminOperator(app, req.Email, req.Password)
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to create admin user: %v", err),
		})
	}

	if req.AppName != "" {
		app.Settings().Meta.AppName = req.AppName
	}
	if req.AppURL != "" {
		app.Settings().Meta.AppURL = req.AppURL
	}
	if req.AppName != "" || req.AppURL != "" {
		if err := app.Save(app.Settings()); err != nil {
			log.Printf("Setup bootstrap: failed to save settings: %v", err)
		}
	}

	// Clear the token — one-time use
	setupTokenMu.Lock()
	setupToken = ""
	setupTokenMu.Unlock()

	// Mint the token from the `users` operator (not _superusers) so the console
	// runs as the super-admin app user and store writes are authorized.
	authToken, err := operator.NewAuthToken()
	if err != nil {
		return re.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Admin user created but failed to generate auth token.",
		})
	}

	return re.JSON(http.StatusOK, map[string]string{
		"authToken": authToken,
		"email":     req.Email,
		"userId":    operator.Id,
	})
}

// createSuperAdminOperator creates the first operator as a regular `users`
// record and promotes it via a `super_admins` row. Returns the users record so
// the caller can mint its auth token. The super_admins createRule is null
// (superuser-only); this runs in the app's Go context, which bypasses record
// rules, so the insert is authorized without an authenticated superuser.
func createSuperAdminOperator(app core.App, email, password string) (*core.Record, error) {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, fmt.Errorf("find users collection: %w", err)
	}
	username := DeriveUsername(email)
	operator := core.NewRecord(users)
	operator.SetEmail(email)
	operator.SetPassword(password)
	operator.SetVerified(true)
	operator.Set("emailVisibility", true)
	// `name` is required and is a human display label; seed it from the email
	// local-part (the operator can rename themselves later). `username` is the
	// unique handle, derived by the shared helper.
	operator.Set("name", strings.SplitN(email, "@", 2)[0])
	operator.Set("username", username)
	// `role` is required (1940000000_backfill_and_require_users_role). The
	// operator's real authority is its super_admins row, not this field, so it
	// takes the least-privileged non-guest value — the same one the migration
	// backfilled existing operators to, and the one that leaves every
	// @request.auth.role rule evaluating exactly as an empty role used to.
	operator.Set("role", "member")
	if err := app.Save(operator); err != nil {
		return nil, fmt.Errorf("create users record: %w", err)
	}

	superAdmins, err := app.FindCollectionByNameOrId("super_admins")
	if err != nil {
		return nil, fmt.Errorf("find super_admins collection: %w", err)
	}
	grant := core.NewRecord(superAdmins)
	grant.Set("user", operator.Id)
	if err := app.Save(grant); err != nil {
		return nil, fmt.Errorf("create super_admins grant: %w", err)
	}
	return operator, nil
}

func generateToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func printBoxed(title, url string) {
	w := len(url) + 4
	if titleW := len(title) + 4; titleW > w {
		w = titleW
	}
	h := strings.Repeat("─", w)

	pad := func(s string) string {
		gap := w - len(s) - 2
		return "│ " + s + strings.Repeat(" ", gap) + " │"
	}

	fmt.Printf("\n┌%s┐\n", h)
	fmt.Printf("%s\n", pad(title))
	fmt.Printf("│%s│\n", strings.Repeat(" ", w))
	fmt.Printf("%s\n", pad(url))
	fmt.Printf("└%s┘\n\n", h)
}
