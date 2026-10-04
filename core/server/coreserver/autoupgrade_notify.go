package coreserver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/approutes"
	"tinycld.org/core/mailer"
)

type mailFn func(toName, toEmail, subject, htmlBody, textBody string) error

func pauseNotice(wanted map[string]string, reason string, reminder bool) notice {
	subject := "Automatic updates are paused"
	if reminder {
		subject = "Reminder: automatic updates are still paused"
	}
	body := fmt.Sprintf("New versions are available (%s), but they do not work with the packages you have installed:\n\n%s\n\nUpdates stay paused until this is resolved. You can update packages by hand on Settings → Packages.",
		formatTarget(wanted), reason)
	return notice{Subject: subject, BodyText: body}
}

func blockedNotice(target map[string]string, reason string, reminder bool) notice {
	subject := "An automatic update was rolled back"
	if reminder {
		subject = "Reminder: an automatic update is still blocked"
	}
	body := fmt.Sprintf("The update to %s was rolled back:\n\n%s\n\nIt will not be tried again until you clear it on Settings → Packages.",
		formatTarget(target), reason)
	return notice{Subject: subject, BodyText: body}
}

// notifyAdmins emails n to every owner and admin. The error joins the
// failures; a caller that only informs may drop it, since send logs each one.
// No recipient is an error too: a caller that records delivery (a notice
// sent once) must not count a boot before the first owner as having told
// anyone.
func notifyAdmins(app core.App, send mailFn, n notice) error {
	users, err := app.FindRecordsByFilter("users",
		"(role = 'owner' || role = 'admin') && disabled != true", "", 0, 0)
	if err != nil {
		srvLog.Warn("auto-upgrade: cannot list recipients", "err", err)
		return fmt.Errorf("coreserver: list the administrators: %w", err)
	}
	if len(users) == 0 {
		return errors.New("coreserver: email the administrators: no enabled owner or admin to email")
	}
	ctaLabel, ctaPath := n.CTALabel, n.CTAPath
	if ctaLabel == "" {
		ctaLabel, ctaPath = "Open Packages", "settings/packages"
	}
	link := strings.TrimRight(app.Settings().Meta.AppURL, "/") + approutes.Href(ctaPath)
	var errs []error
	for _, u := range users {
		html, text := mailer.RenderTransactionalEmail(mailer.TransactionalEmail{
			Eyebrow:  "Automatic updates",
			Greeting: mailer.Greeting(u.GetString("name")),
			BodyHTML: strings.ReplaceAll(mailer.EscapeHTML(n.BodyText), "\n", "<br>"),
			BodyText: n.BodyText,
			CTALabel: ctaLabel,
			CTALink:  link,
		})
		if err := send(u.GetString("name"), u.Email(), n.Subject, html, text); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
