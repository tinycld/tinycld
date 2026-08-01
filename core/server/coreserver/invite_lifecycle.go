package coreserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailer"
)

const (
	inviteTokenTTL  = 7 * 24 * time.Hour
	inviteTokenSize = 32 // bytes; hex-encoded to 64 chars
)

// invalidateExistingTokens marks all unused invite tokens for a user as used.
func invalidateExistingTokens(app core.App, userID string) error {
	tokens, err := app.FindRecordsByFilter(
		"invite_tokens",
		"user = {:userId} && used_at = ''",
		"",
		0,
		0,
		map[string]any{"userId": userID},
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

func mintInviteToken(app core.App, user *core.Record, role string) (string, error) {
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
	record.Set("role", role)
	record.Set("expires_at", time.Now().Add(inviteTokenTTL).UTC().Format(time.RFC3339))

	if err := app.Save(record); err != nil {
		return "", fmt.Errorf("save invite token: %w", err)
	}
	return token, nil
}

func sendExistingMemberEmail(app core.App, user *core.Record, role string) {
	appURL := strings.TrimRight(app.Settings().Meta.AppURL, "/")

	userEmail := user.GetString("email")
	userName := user.GetString("name")
	if userName == "" {
		userName = userEmail
	}

	subject := "You've been added"
	htmlBody, text := mailer.RenderTransactionalEmail(mailer.TransactionalEmail{
		Eyebrow:  "Invitation",
		Greeting: mailer.Greeting(userName),
		BodyHTML: fmt.Sprintf("You've been added as a <strong>%s</strong>. Sign in with your existing account to get started.", mailer.EscapeHTML(role)),
		BodyText: fmt.Sprintf("You've been added as a %s. Sign in with your existing account to get started.", role),
		CTALabel: "Open the app",
		CTALink:  appURL,
		Footer:   "If you didn't expect to join, please contact your admin.",
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
