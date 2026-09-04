// Package guestauth provisions the `users` row behind an email-OTP sign-in.
//
// A share-link visitor arrives with no account. Proving they control an email
// address is what turns them into someone a membership row can point at — and
// every content relation in this codebase (a comment's author, a card's
// created_by) is a required relation to `users`, so there is no writing
// anything until that row exists.
//
// Promoted from drive/server, which had the only implementation, when boards'
// public boards needed the same flow. Everything here is package-agnostic: what
// a redeemed link GRANTS differs per package (drive writes a drive_shares row,
// cards a boards_project_members row) and stays in the package. What is shared
// is the account-and-code machinery, which is fiddly, security-sensitive, and
// was identical in both.
package guestauth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
	"tinycld.org/core/coreserver"
	"tinycld.org/core/mailer"
)

// CodeLength is the number of digits in an emailed OTP.
const CodeLength = 6

// CodeTTL is how long a minted OTP stays valid. PocketBase's stock OTP flow
// uses 15 minutes and this mirrors it: short enough that a leaked email is not
// a long-lived credential, long enough that a person can actually open their
// inbox.
const CodeTTL = 15 * time.Minute

// GuestRole is the role a share-link visitor's account carries.
const GuestRole = "guest"

// ParseEmail normalizes and validates an address, returning it trimmed with the
// local part's case preserved.
func ParseEmail(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty email")
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", err
	}
	if addr.Address == "" {
		return "", errors.New("empty email")
	}
	return addr.Address, nil
}

// NewCode returns a fresh numeric OTP.
//
// Six digits is only 10^6, so the code is not the security boundary on its own:
// what bounds a brute force is the caller's rate limit, single-use deletion on
// success, and the TTL above. A caller that skips those has a guessable code.
func NewCode() string {
	return security.RandomStringWithAlphabet(CodeLength, "1234567890")
}

// FindOrCreateUser returns the `users` row for an email, creating one if
// needed. A created row is unverified, carries a random password and the guest
// role, and is useless until a verified code upgrades it.
//
// Matching is case-insensitive (PocketBase's FindAuthRecordByEmail).
func FindOrCreateUser(app core.App, email string) (*core.Record, error) {
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, fmt.Errorf("users collection: %w", err)
	}

	existing, err := app.FindAuthRecordByEmail(usersCol, email)
	if err == nil && existing != nil {
		return existing, nil
	}
	// FindAuthRecordByEmail returns sql.ErrNoRows when missing; tolerate that
	// and fall through to create. Any OTHER error (a transient DB failure) is
	// fatal — without this guard a flaky connection silently double-creates.
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no rows") {
		return nil, fmt.Errorf("lookup user by email: %w", err)
	}

	displayName := email
	if idx := strings.IndexByte(email, '@'); idx > 0 {
		displayName = email[:idx]
	}

	username, err := PickAvailableUsername(app, coreserver.DeriveUsername(email))
	if err != nil {
		return nil, fmt.Errorf("derive username: %w", err)
	}

	rec := core.NewRecord(usersCol)
	rec.SetEmail(email)
	rec.Set("emailVisibility", true)
	rec.Set("name", displayName)
	rec.Set("username", username)
	// `role` is required, so it must be set at creation rather than left to
	// EnsureGuestRole further down the flow — the save fails validation
	// otherwise and no visitor can ever sign in. Guest is right for a brand-new
	// account reached through a link; EnsureGuestRole's job is the different
	// one of never DOWNGRADING an existing member with the same address.
	rec.Set("role", GuestRole)
	rec.SetVerified(false)
	rec.SetPassword(security.RandomString(32))
	if err := app.Save(rec); err != nil {
		// Race: another request created the same email between the lookup and
		// this save. Re-fetch and return that row.
		if again, ferr := app.FindAuthRecordByEmail(usersCol, email); ferr == nil && again != nil {
			return again, nil
		}
		return nil, fmt.Errorf("create guest user: %w", err)
	}
	return rec, nil
}

// PickAvailableUsername returns `base`, or base+"2", base+"3", … until one is
// free. The unique index on users.username is the DB-level net if a race slips
// past this read-then-write check.
//
// Bases shorter than three characters are padded: PocketBase's stock users
// collection enforces min length 3, and a short email prefix ("ok@example.com"
// → "ok") would otherwise fail validation at save. This codebase's migration
// relaxes that to 1, but the PB test fixture still ships with 3, so padding
// keeps both working.
func PickAvailableUsername(app core.App, base string) (string, error) {
	for len(base) < 3 {
		base += "0"
	}
	candidate := base
	for i := 2; i < 1000; i++ {
		existing, err := app.FindFirstRecordByFilter(
			"users", "username = {:u}", map[string]any{"u": candidate})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("lookup username %q: %w", candidate, err)
		}
		if existing == nil {
			return candidate, nil
		}
		candidate = base + strconv.Itoa(i)
	}
	return "", fmt.Errorf("could not find free username for %q", base)
}

// EnsureGuestRole stamps role=guest on a user that has no role yet.
//
// The rule that matters is the one it does NOT do: a user who already carries
// any role keeps it, so an owner or admin who happens to open a share link is
// never demoted to guest.
func EnsureGuestRole(app core.App, user *core.Record) error {
	if user.GetString("role") != "" {
		return nil
	}
	user.Set("role", GuestRole)
	if err := app.Save(user); err != nil {
		return fmt.Errorf("set guest role: %w", err)
	}
	return nil
}

// MintOTP creates a one-time code anchored to a user and returns the OTP record
// alongside the plaintext code. The code is stored hashed, so this return value
// is the ONLY chance to send it — there is no reading it back.
func MintOTP(app core.App, user *core.Record, email string) (*core.OTP, string, error) {
	code := NewCode()
	otp := core.NewOTP(app)
	otp.SetCollectionRef(user.Collection().Id)
	otp.SetRecordRef(user.Id)
	otp.SetSentTo(email)
	otp.SetPassword(code)
	if err := app.Save(otp); err != nil {
		return nil, "", fmt.Errorf("mint OTP: %w", err)
	}
	return otp, code, nil
}

// SendCode emails an OTP.
//
// Goes through the project's own mailer (LogSender in dev and test, Postmark in
// production) rather than PocketBase's NewMailClient, so every outbound email in
// the codebase shares one delivery gate, one dev log path and one config.
//
// `subject` and `intro` are the caller's, because only they know what the
// recipient is joining — a file, a board — and a code with no context in an
// inbox reads like phishing. The code itself is escaped and formatted here so
// every caller renders it the same way.
func SendCode(ctx context.Context, subject, intro, toEmail, code string) error {
	html := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 480px;">
<p>%s</p>
<p style="font-size: 28px; font-weight: 700; letter-spacing: 4px;">%s</p>
<p style="color: #555;">The code expires in 15 minutes. If you didn't request it, you can ignore this email.</p>
</div>`, escapeHTML(intro), escapeHTML(code))
	text := fmt.Sprintf("%s %s\n\nThe code expires in 15 minutes.\n", intro, code)

	return mailer.DefaultSender().Send(ctx, &mailer.Message{
		To:      []mailer.Recipient{{Email: toEmail}},
		Subject: subject,
		HTML:    html,
		Text:    text,
	})
}

// escapeHTML guards the interpolations above. The code alphabet is numeric so
// escaping it is a defensive no-op today; it stays so a future alphanumeric
// alphabet cannot open an injection. The caller-supplied intro is the half that
// genuinely needs it — a board name is user input.
func escapeHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	).Replace(s)
}

// ErrBadCode is the single failure every rejection collapses into.
//
// Uniform on purpose: an attacker must not be able to tell "no such otp_id"
// from "wrong code" from "expired" by the status or the message, or the opaque
// otp_id becomes an oracle.
var ErrBadCode = errors.New("invalid or expired code")

// ConsumeOTP validates a code against an otp_id and email, and returns the OTP
// so the caller can delete it inside their provisioning transaction.
//
// Deleting on the wrong-email and expired paths is deliberate: an attacker who
// guessed or intercepted the opaque otp_id must not be able to keep retrying it
// against different addresses.
func ConsumeOTP(app core.App, otpID, code, email string) (*core.OTP, error) {
	otp, err := app.FindOTPById(otpID)
	if err != nil {
		return nil, ErrBadCode
	}
	if otp.HasExpired(CodeTTL) {
		_ = app.Delete(otp)
		return nil, ErrBadCode
	}
	if !strings.EqualFold(otp.SentTo(), email) {
		_ = app.Delete(otp)
		return nil, ErrBadCode
	}
	if !otp.ValidatePassword(code) {
		// NOT deleted: a legitimate person fat-fingering a digit must be able
		// to try again within the TTL. The rate limit is what bounds guessing.
		return nil, ErrBadCode
	}
	return otp, nil
}
