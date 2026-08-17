package coreserver

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailer"
)

// RegisterPasswordResetMailer overrides PocketBase's built-in password-reset
// email for the app's `users` collection so the link points at the app's own
// reset screen and the message is delivered through the shared core mailer
// (the same path invites use) rather than PocketBase's native SMTP.
//
// PocketBase still owns the reset token lifecycle (mint, expiry, single-use) —
// we only replace the email. The token is available on the event as
// e.Meta["token"]; we build a link to /reset-password/<token>, which the client
// route app/reset-password/[token].tsx consumes via confirmPasswordReset.
//
// The send always goes through mailer.DefaultSender() (like invites): it
// delivers via the configured provider when delivery is enabled, and otherwise
// logs (dev/test, or no provider configured), so we never depend on PB's
// native SMTP. We return without calling e.Next() to suppress PB's own send.
func RegisterPasswordResetMailer(app *pocketbase.PocketBase) {
	registerPasswordResetMailerCore(app)
}

func registerPasswordResetMailerCore(app core.App) {
	// Tagged to "users" so superusers keep PocketBase's default reset path.
	app.OnMailerRecordPasswordResetSend("users").BindFunc(func(e *core.MailerRecordEvent) error {
		token, _ := e.Meta["token"].(string)
		if token == "" {
			// Without a token we can't build a working link; let PB send its
			// default email rather than a broken one.
			srvLog.Warn("password reset: missing token in mailer event; deferring to PB")
			return e.Next()
		}

		toEmail := e.Record.Email()
		toName := e.Record.GetString("name")
		settings := app.Settings().Meta
		subject, htmlBody, text := buildPasswordResetMessage(settings.AppURL, settings.AppName, toName, token)

		send(app, toName, toEmail, subject, htmlBody, text)

		// Suppress PocketBase's native send by returning without calling e.Next().
		return nil
	})
}

// buildPasswordResetMessage assembles the subject, HTML, and text bodies of the
// reset email via the shared transactional-email template. Pulled out of the
// hook so it's unit-testable without PocketBase's trigger machinery. toName may
// be empty (falls back to a generic greeting); the link always points at the
// app's own reset screen.
func buildPasswordResetMessage(appURL, appName, toName, token string) (subject, html, text string) {
	base := strings.TrimRight(appURL, "/")
	link := fmt.Sprintf("%s/reset-password/%s", base, token)

	eyebrow := "Password reset"
	if appName != "" {
		eyebrow = "Password reset · " + appName
	}

	subject = "Reset your password"
	html, text = mailer.RenderTransactionalEmail(mailer.TransactionalEmail{
		Eyebrow:  eyebrow,
		Greeting: mailer.Greeting(toName),
		BodyHTML: "We received a request to reset your password. Click the button below to choose a new one. This link will expire soon.",
		BodyText: "We received a request to reset your password. Open the link below to choose a new one. If you didn't request this, you can safely ignore this email.",
		CTALabel: "Reset password",
		CTALink:  link,
		Footer:   "If you didn't request a password reset, no action is needed — your password stays the same.",
	})
	return subject, html, text
}
