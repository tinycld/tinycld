package oauth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// seedUserAndClient creates one user and one public client, returning their ids.
func seedUserAndClient(t *testing.T, app *tests.TestApp) (userID, clientRecID string) {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	u := core.NewRecord(users)
	u.Set("email", "alice@example.com")
	u.Set("password", "s3cret-password")
	if err := app.Save(u); err != nil {
		t.Fatalf("save user: %v", err)
	}

	clients, err := app.FindCollectionByNameOrId(clientsCollection)
	if err != nil {
		t.Fatalf("find clients: %v", err)
	}
	c := core.NewRecord(clients)
	c.Set("client_id", "tinycld-cli")
	c.Set("name", "TinyCld CLI")
	c.Set("type", "public")
	c.Set("is_first_party", true)
	if err := app.Save(c); err != nil {
		t.Fatalf("save client: %v", err)
	}
	return u.Id, c.Id
}

func TestNewGrantIsFindableByJTI(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	jti := grant.GetString("jti")
	if jti == "" {
		t.Fatal("NewGrant must assign a jti")
	}

	found, err := FindGrantByJTI(app, jti)
	if err != nil {
		t.Fatalf("FindGrantByJTI: %v", err)
	}
	if found.Id != grant.Id {
		t.Fatalf("found grant %s, want %s", found.Id, grant.Id)
	}
}

func TestVerifyGrantRejectsRevoked(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	jti := grant.GetString("jti")

	// Sanity: valid before revocation.
	if _, err := VerifyGrant(app, jti); err != nil {
		t.Fatalf("VerifyGrant before revoke: %v", err)
	}

	if err := RevokeGrant(app, grant.Id); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}

	// This is the property the whole design exists for: revoking one grant
	// must take effect on the very next request.
	if _, err := VerifyGrant(app, jti); !errors.Is(err, ErrGrantRevoked) {
		t.Fatalf("VerifyGrant after revoke = %v, want ErrGrantRevoked", err)
	}
}

func TestVerifyGrantRejectsExpired(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	grant.Set("expires_at", time.Now().Add(-time.Hour))
	if err := app.Save(grant); err != nil {
		t.Fatalf("save expired grant: %v", err)
	}

	if _, err := VerifyGrant(app, grant.GetString("jti")); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("VerifyGrant on expired = %v, want ErrInvalidGrant", err)
	}
}

func TestVerifyGrantRejectsPending(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	// A device-flow grant awaiting approval must not authorize anything.
	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "pending")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	if _, err := VerifyGrant(app, grant.GetString("jti")); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("VerifyGrant on pending = %v, want ErrInvalidGrant", err)
	}
}

func TestVerifyGrantRejectsUnknownJTI(t *testing.T) {
	app := newSchemaApp(t)
	if _, err := VerifyGrant(app, "no-such-jti"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("VerifyGrant on unknown jti = %v, want ErrInvalidGrant", err)
	}
}

func TestNewUserCodeIsReadable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		code, err := newUserCode()
		if err != nil {
			t.Fatalf("newUserCode: %v", err)
		}
		// Format WDJB-MJHT: two groups of four, one dash.
		if len(code) != 9 || code[4] != '-' {
			t.Fatalf("newUserCode() = %q, want XXXX-XXXX", code)
		}
		// Ambiguous glyphs must be absent — users read these aloud and retype them.
		for _, bad := range []string{"0", "O", "1", "I", "L"} {
			if strings.Contains(code, bad) {
				t.Fatalf("user code %q contains ambiguous character %q", code, bad)
			}
		}
		if seen[code] {
			t.Fatalf("newUserCode produced a duplicate within 50 draws: %q", code)
		}
		seen[code] = true
	}
}

func TestHashSecretIsStableAndNotPlaintext(t *testing.T) {
	h1 := hashSecret("super-secret")
	h2 := hashSecret("super-secret")
	if h1 != h2 {
		t.Fatal("hashSecret must be deterministic")
	}
	if strings.Contains(h1, "super-secret") {
		t.Fatal("hashSecret must not embed the plaintext")
	}
	if h1 == hashSecret("different") {
		t.Fatal("hashSecret must differ for different inputs")
	}
}
