package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// touchInterval throttles last_used_at writes. Without it every authenticated
// request would issue a write, turning a read-mostly workload into a
// write-heavy one for no user-visible gain — the field only drives the
// "last used" line on the Connected apps screen.
const touchInterval = 5 * time.Minute

// usersCollection is the app's regular (non-superuser) auth collection.
// Named here rather than imported, matching driveshare's usersCollection —
// the two packages don't share a module.
const usersCollection = "users"

// userCodeAlphabet omits 0/O/1/I/L. A user reads this code off a terminal and
// types it into a browser, so ambiguous glyphs are a real support cost.
const userCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// randomToken returns nBytes of crypto/rand as unpadded base64url.
func randomToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("oauth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashSecret returns the hex SHA-256 of s. Used for refresh tokens, client
// secrets, and authorization codes: we store only the hash, so a database read
// never yields a usable credential.
func hashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// newUserCode returns a human-transcribable code shaped XXXX-XXXX.
//
// No collision retry against the (now partial-UNIQUE, see the migration's
// idx_oauth_grants_user_code comment) index: the alphabet gives 32^8 ≈ 2^40
// combinations, so even thousands of simultaneously pending device logins
// leave a collision probability far below other failure modes this endpoint
// already doesn't retry for (e.g. a transient DB write error). A collision
// surfaces as handleDeviceAuthorization's ordinary app.Save error path — a
// 500 the CLI already treats as "request device authorization again," which
// mints a fresh code — so a bespoke retry loop here would add complexity to
// handle a case indistinguishable, from the caller's perspective, from any
// other save failure.
func newUserCode() (string, error) {
	out := make([]byte, 0, 9)
	for i := 0; i < 8; i++ {
		if i == 4 {
			out = append(out, '-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(userCodeAlphabet))))
		if err != nil {
			return "", fmt.Errorf("oauth: read random: %w", err)
		}
		out = append(out, userCodeAlphabet[n.Int64()])
	}
	return string(out), nil
}

// NewGrant creates a grant row with a fresh jti. status is "pending" for a
// device flow awaiting approval, or "active" for an already-authorized grant.
func NewGrant(
	app core.App,
	userID, clientRecID string,
	scopes []string,
	status string,
) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId(grantsCollection)
	if err != nil {
		return nil, fmt.Errorf("oauth: find %s: %w", grantsCollection, err)
	}
	jti, err := randomToken(24)
	if err != nil {
		return nil, err
	}

	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("client", clientRecID)
	rec.Set("jti", jti)
	rec.Set("scopes", strings.Join(scopes, " "))
	rec.Set("status", status)
	if err := app.Save(rec); err != nil {
		return nil, fmt.Errorf("oauth: save grant: %w", err)
	}
	return rec, nil
}

// FindGrantByJTI looks up a grant by its token id.
func FindGrantByJTI(app core.App, jti string) (*core.Record, error) {
	if jti == "" {
		return nil, ErrInvalidGrant
	}
	rec, err := app.FindFirstRecordByFilter(
		grantsCollection, "jti = {:jti}", map[string]any{"jti": jti},
	)
	if err != nil || rec == nil {
		return nil, ErrInvalidGrant
	}
	return rec, nil
}

// FindGrantByUserCode looks up a pending device-flow grant by the code the
// user types into the browser.
func FindGrantByUserCode(app core.App, userCode string) (*core.Record, error) {
	if userCode == "" {
		return nil, ErrInvalidGrant
	}
	rec, err := app.FindFirstRecordByFilter(
		grantsCollection, "user_code = {:c}", map[string]any{"c": userCode},
	)
	if err != nil || rec == nil {
		return nil, ErrInvalidGrant
	}
	return rec, nil
}

// VerifyGrant is the check every authenticated request runs. A valid signature
// is never sufficient: the row is re-read so a revocation takes effect on the
// next request, and status, expiry, and the owning user's disabled flag are
// re-checked against current state. Mirrors sharelink.VerifyAndResolve.
func VerifyGrant(app core.App, jti string) (*core.Record, error) {
	rec, err := FindGrantByJTI(app, jti)
	if err != nil {
		return nil, err
	}
	switch rec.GetString("status") {
	case "revoked":
		return nil, ErrGrantRevoked
	case "active":
		// ok
	default:
		// "pending" — a device grant nobody has approved yet.
		return nil, ErrInvalidGrant
	}
	if exp := rec.GetDateTime("expires_at"); !exp.IsZero() && exp.Time().Before(time.Now()) {
		return nil, ErrInvalidGrant
	}

	// PocketBase's per-request auth middleware never re-runs the disabled-user
	// hook (that only guards token ISSUANCE — see disabled_guard.go), so an
	// already-issued OAuth access token would otherwise keep authenticating
	// forever after the account is disabled. This is the per-request cutoff,
	// same role as davauth's disabled check for protocol logins. An "active"
	// grant always has a user (only "pending" device grants can lack one, and
	// that status is rejected above), but userID is re-validated defensively
	// rather than assumed. Reported as ErrInvalidGrant, not a distinct error —
	// a caller must not be able to tell "disabled" apart from "revoked".
	userID := rec.GetString("user")
	if userID == "" {
		return nil, ErrInvalidGrant
	}
	user, err := app.FindRecordById(usersCollection, userID)
	if err != nil {
		// Deleted user: cascade delete should have taken the grant with it,
		// but do not treat "can't confirm the user" as "assume fine".
		return nil, ErrInvalidGrant
	}
	if user.GetBool("disabled") {
		return nil, ErrInvalidGrant
	}

	return rec, nil
}

// RevokeGrant marks a grant revoked. Revoking an already-revoked grant is not
// an error; revoking a nonexistent id is, via FindRecordById.
func RevokeGrant(app core.App, grantID string) error {
	rec, err := app.FindRecordById(grantsCollection, grantID)
	if err != nil {
		return fmt.Errorf("oauth: find grant %s: %w", grantID, err)
	}
	rec.Set("status", "revoked")
	// Clear every field that could still be used as a credential, however the
	// grant got here — including mid-flight (e.g. a user denying a pending
	// device request), which is why device_code/user_code are included and
	// not just the token-exchange fields.
	rec.Set("refresh_token_hash", "")
	rec.Set("auth_code_hash", "")
	rec.Set("device_code", "")
	rec.Set("user_code", "")
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("oauth: save revoked grant: %w", err)
	}
	return nil
}

// TouchGrant stamps last_used_at, at most once per touchInterval.
func TouchGrant(app core.App, grant *core.Record) error {
	last := grant.GetDateTime("last_used_at")
	if !last.IsZero() && time.Since(last.Time()) < touchInterval {
		return nil
	}
	grant.Set("last_used_at", time.Now())
	if err := app.Save(grant); err != nil {
		return fmt.Errorf("oauth: touch grant: %w", err)
	}
	return nil
}
