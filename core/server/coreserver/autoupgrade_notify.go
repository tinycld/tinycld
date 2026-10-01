package coreserver

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/approutes"
	"tinycld.org/core/mailer"
)

type mailFn func(toName, toEmail, subject, htmlBody, textBody string)

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

func notifyAdmins(app core.App, send mailFn, n notice) {
	users, err := app.FindRecordsByFilter("users",
		"(role = 'owner' || role = 'admin') && disabled != true", "", 0, 0)
	if err != nil {
		srvLog.Warn("auto-upgrade: cannot list recipients", "err", err)
		return
	}
	link := strings.TrimRight(app.Settings().Meta.AppURL, "/") + approutes.Href("settings/packages")
	for _, u := range users {
		html, text := mailer.RenderTransactionalEmail(mailer.TransactionalEmail{
			Eyebrow:  "Automatic updates",
			Greeting: mailer.Greeting(u.GetString("name")),
			BodyHTML: strings.ReplaceAll(mailer.EscapeHTML(n.BodyText), "\n", "<br>"),
			BodyText: n.BodyText,
			CTALabel: "Open Packages",
			CTALink:  link,
		})
		send(u.GetString("name"), u.Email(), n.Subject, html, text)
	}
}
