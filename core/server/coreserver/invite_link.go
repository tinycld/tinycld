package coreserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailer"
)

// RegisterInviteLinkEndpoints wires the admin-facing endpoints that surface
// and manage invite links for pending users.
func RegisterInviteLinkEndpoints(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/invite-link/{userId}", func(re *core.RequestEvent) error {
			return handleGetInviteLink(app, re)
		}).BindFunc(requireAuthCore)

		e.Router.POST("/api/invite-link/{userId}/rotate", func(re *core.RequestEvent) error {
			return handleRotateInviteLink(app, re)
		}).BindFunc(requireAuthCore)

		e.Router.POST("/api/invite-link/{userId}/send", func(re *core.RequestEvent) error {
			return handleSendInviteLink(app, re)
		}).BindFunc(requireAuthCore)

		return e.Next()
	})
}

// resolveInvitedUserAsAdmin loads the invited user by ID and verifies the
// caller is an admin or owner. Returns the user record on success.
func resolveInvitedUserAsAdmin(app core.App, re *core.RequestEvent) (*core.Record, error) {
	userID := re.Request.PathValue("userId")
	user, err := app.FindRecordById("users", userID)
	if err != nil || user == nil {
		return nil, re.NotFoundError("user not found", err)
	}
	if re.Auth == nil {
		return nil, re.UnauthorizedError("Authentication required", nil)
	}
	if !isOrgAdmin(re.Auth) {
		return nil, re.ForbiddenError("You must be an admin or owner", nil)
	}
	return user, nil
}

// liveTokenForUser returns an unused, unexpired invite token for the user, or
// (nil, nil) if none exists. Rotation invalidates all prior tokens before
// minting a new one, so at most one live token exists at a time and ordering
// doesn't matter.
func liveTokenForUser(app core.App, user *core.Record) (*core.Record, error) {
	records, err := app.FindRecordsByFilter(
		"invite_tokens",
		"user = {:u} && used_at = ''",
		"", 0, 0,
		map[string]any{"u": user.Id},
	)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		exp := r.GetDateTime("expires_at")
		if !exp.IsZero() && exp.Time().Before(time.Now()) {
			continue
		}
		return r, nil
	}
	return nil, nil
}

func handleGetInviteLink(app core.App, re *core.RequestEvent) error {
	user, err := resolveInvitedUserAsAdmin(app, re)
	if err != nil {
		return err
	}
	token, err := liveTokenForUser(app, user)
	if err != nil {
		return re.InternalServerError("Failed to look up invite token", err)
	}
	if token == nil {
		return re.JSON(http.StatusNotFound, map[string]any{"error": "no live invite link"})
	}
	return re.JSON(http.StatusOK, map[string]any{
		"inviteUrl": buildInviteURL(app, token.GetString("token")),
		"expiresAt": token.GetString("expires_at"),
	})
}

func handleRotateInviteLink(app core.App, re *core.RequestEvent) error {
	user, err := resolveInvitedUserAsAdmin(app, re)
	if err != nil {
		return err
	}

	if err := invalidateExistingTokens(app, user.Id); err != nil {
		return re.InternalServerError("Failed to invalidate old tokens", err)
	}

	token, err := mintInviteToken(app, user, user.GetString("role"))
	if err != nil {
		return re.InternalServerError("Failed to mint invite token", err)
	}

	return re.JSON(http.StatusOK, map[string]any{
		"inviteUrl": buildInviteURL(app, token),
	})
}

func handleSendInviteLink(app core.App, re *core.RequestEvent) error {
	user, err := resolveInvitedUserAsAdmin(app, re)
	if err != nil {
		return err
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := decodeJSONBody(re, &body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if err := validateAltEmail(body.Email); err != nil {
		return re.BadRequestError(err.Error(), err)
	}

	if IsDemoUser(app, re.Auth.Id) {
		return re.JSON(http.StatusServiceUnavailable, map[string]any{
			"error": "Email sending is disabled for demo accounts; copy the link manually.",
		})
	}

	token, err := liveTokenForUser(app, user)
	if err != nil {
		return re.InternalServerError("Failed to look up invite token", err)
	}
	if token == nil {
		return re.JSON(http.StatusConflict, map[string]any{
			"error": "no live invite link, rotate first",
		})
	}

	if err := sendInviteEmailTo(app, body.Email, user, user.GetString("role"), token.GetString("token")); err != nil {
		return re.JSON(http.StatusBadGateway, map[string]any{
			"error": "failed to send invite email: " + err.Error(),
		})
	}

	return re.JSON(http.StatusOK, map[string]any{"delivered": true})
}

// validateAltEmail checks that an email looks like a real address before we
// hand it to the mailer.
func validateAltEmail(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("email is required")
	}
	if _, err := mail.ParseAddress(s); err != nil {
		return fmt.Errorf("invalid email")
	}
	return nil
}

// decodeJSONBody is a small wrapper for parsing request bodies; callers map
// errors to BadRequestError.
func decodeJSONBody(re *core.RequestEvent, dst any) error {
	return json.NewDecoder(re.Request.Body).Decode(dst)
}

// sendInviteEmailTo builds the invite email body and addresses it to an
// arbitrary email instead of the user's account email.
func sendInviteEmailTo(app core.App, toEmail string, user *core.Record, role, token string) error {
	link := buildInviteURL(app, token)

	userName := user.GetString("name")
	if userName == "" {
		userName = user.GetString("email")
	}

	subject := "You've been invited"
	htmlBody, text := mailer.RenderTransactionalEmail(mailer.TransactionalEmail{
		Eyebrow:  "Invitation",
		Greeting: mailer.Greeting(userName),
		BodyHTML: fmt.Sprintf("You've been invited to join as a <strong>%s</strong>. To get started, set a password for your account.", mailer.EscapeHTML(role)),
		BodyText: fmt.Sprintf("You've been invited to join as a %s. To get started, set a password for your account. The link expires in 7 days.", role),
		CTALabel: "Set your password",
		CTALink:  link,
		Footer:   "If you weren't expecting this invitation, you can safely ignore this email. The link expires in 7 days.",
	})

	msg := &mailer.Message{
		To:      []mailer.Recipient{{Name: userName, Email: toEmail}},
		Subject: subject,
		HTML:    htmlBody,
		Text:    text,
	}
	return mailer.DefaultSender().Send(context.Background(), msg)
}
