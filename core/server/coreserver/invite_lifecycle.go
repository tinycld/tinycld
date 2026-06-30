package coreserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailer"
)

const (
	inviteTokenTTL  = 7 * 24 * time.Hour
	inviteTokenSize = 32 // bytes; hex-encoded to 64 chars
)

// RegisterInviteLifecycle wires a hook that emails invited users after
// a user_org membership row is created. For brand-new users (verified=false),
// it mints an invite_tokens row and emails a password-set link. For existing
// users, it emails a simple "you've been added" notice linking to the org.
func RegisterInviteLifecycle(app *pocketbase.PocketBase) {
	registerInviteLifecycleCore(app)
}

func registerInviteLifecycleCore(app core.App) {
	app.OnRecordAfterCreateSuccess("user_org").BindFunc(func(e *core.RecordEvent) error {
		userOrg := e.Record
		go handleUserOrgInvite(app, userOrg)
		return e.Next()
	})
}

func handleUserOrgInvite(app core.App, userOrg *core.Record) {
	userID := userOrg.GetString("user")
	orgID := userOrg.GetString("org")
	role := userOrg.GetString("role")
	inviterID := userOrg.GetString("created_by")

	user, err := app.FindRecordById("users", userID)
	if err != nil {
		app.Logger().Warn("invite lifecycle: failed to find user",
			"userID", userID, "error", err)
		return
	}
	org, err := app.FindRecordById("orgs", orgID)
	if err != nil {
		app.Logger().Warn("invite lifecycle: failed to find org",
			"orgID", orgID, "error", err)
		return
	}

	// Demo inviters: skip the outbound email but still mint the token so the
	// invited record exists and the demo flow looks complete from the UI.
	suppressEmail := IsDemoUser(app, inviterID)

	if user.GetBool("verified") {
		if !suppressEmail {
			sendExistingMemberEmail(app, user, org, role)
		}
		return
	}

	// Brand-new (unverified) users are handled by the admin-delivered invite flow:
	// POST /api/invite-member mints the token and returns the URL in its response.
	// The admin shares it manually, or via POST /api/invite-link/:userOrgId/send.
	// This hook used to also mint a token + email here; both responsibilities now
	// belong to the endpoint.
}

// invalidateExistingTokens marks all unused invite tokens for a user+org as used.
func invalidateExistingTokens(app core.App, userID, orgID string) error {
	tokens, err := app.FindRecordsByFilter(
		"invite_tokens",
		"user = {:userId} && org = {:orgId} && used_at = ''",
		"",
		0,
		0,
		map[string]any{"userId": userID, "orgId": orgID},
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range tokens {
		t.Set("used_at", now)
		if err := app.Save(t); err != nil {
			return err
		}
	}
	return nil
}

func mintInviteToken(app core.App, user *core.Record, org *core.Record, role string) (string, error) {
	tokenBytes := make([]byte, inviteTokenSize)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	col, err := app.FindCollectionByNameOrId("invite_tokens")
	if err != nil {
		return "", fmt.Errorf("find invite_tokens collection: %w", err)
	}

	record := core.NewRecord(col)
	record.Set("token", token)
	record.Set("user", user.Id)
	record.Set("org", org.Id)
	record.Set("role", role)
	record.Set("expires_at", time.Now().Add(inviteTokenTTL).UTC().Format(time.RFC3339))

	if err := app.Save(record); err != nil {
		return "", fmt.Errorf("save invite token: %w", err)
	}
	return token, nil
}

func sendExistingMemberEmail(app core.App, user *core.Record, org *core.Record, role string) {
	appURL := strings.TrimRight(app.Settings().Meta.AppURL, "/")
	slug := org.GetString("slug")
	link := fmt.Sprintf("%s/a/%s", appURL, slug)

	orgName := org.GetString("name")
	userEmail := user.GetString("email")
	userName := user.GetString("name")
	if userName == "" {
		userName = userEmail
	}

	subject := fmt.Sprintf("You've been added to %s", orgName)
	htmlBody, text := mailer.RenderTransactionalEmail(mailer.TransactionalEmail{
		Eyebrow:  "Invitation to " + orgName,
		Greeting: mailer.Greeting(userName),
		BodyHTML: fmt.Sprintf("You've been added to <strong>%s</strong> as a <strong>%s</strong>. Sign in with your existing account to get started.", mailer.EscapeHTML(orgName), mailer.EscapeHTML(role)),
		BodyText: fmt.Sprintf("You've been added to %s as a %s. Sign in with your existing account to get started.", orgName, role),
		CTALabel: fmt.Sprintf("Open %s", orgName),
		CTALink:  link,
		Footer:   "If you didn't expect to join this organization, please contact your admin.",
	})

	send(app, userName, userEmail, subject, htmlBody, text)
}

func send(app core.App, toName, toEmail, subject, htmlBody, textBody string) {
	msg := &mailer.Message{
		To:      []mailer.Recipient{{Name: toName, Email: toEmail}},
		Subject: subject,
		HTML:    htmlBody,
		Text:    textBody,
	}
	if err := mailer.DefaultSender().Send(context.Background(), msg); err != nil {
		app.Logger().Error("invite lifecycle: failed to send email",
			"to", toEmail, "subject", subject, "error", err)
	}
}
