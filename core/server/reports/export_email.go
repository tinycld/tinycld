package reports

import (
	"encoding/base64"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/mailer"
	"tinycld.org/core/ratelimit"
)

// POST /api/reports/email sends a saved report's file as an attachment of one
// message from the server's sender, in the reader's name, with Reply-To the
// reader's own address. The file is made as a download makes it (buildExport),
// with the same limits. GET /api/reports/email-status says whether the server
// can send. Both routes take a signed-in session only, as the export route
// does.

// The refusals of a report email.
const (
	emailNotSetUp     = "Email sending is not set up."
	emailBadForm      = "The email is not valid."
	emailTooMany      = "A report email goes to at most 10 addresses."
	emailNoTo         = "Enter at least one address."
	emailNoSubject    = "Enter a subject."
	emailLongSubject  = "The subject is longer than 200 characters."
	emailSubjectBreak = "The subject cannot hold a line break."
	emailLongMessage  = "The message is longer than 5,000 characters."
	emailTooLarge     = "The file is too large to email. Download it instead."
	emailRateLimited  = "You have sent 20 report emails in the last hour. Try again later."
	emailNotSent      = "The email could not be sent."
)

// The bounds of a report email. The client's zod schema
// (reports/lib/export-email-form.ts) has the same ones.
const (
	emailMaxAddresses = 10
	emailMaxSubject   = 200
	emailMaxMessage   = 5000
)

// emailMaxBytes is the largest file a report email attaches (10 MB). A
// variable so a test can shrink it.
var emailMaxBytes = 10 << 20

// emailSender is the sender of a report email: the server's own. A variable so
// a test can put a failing provider in its place.
var emailSender = mailer.DefaultSender

// emailLimiter allows 20 report emails per user an hour.
var emailLimiter = ratelimit.NewPerApp("reports.email", 20, time.Hour)

// emailBody is an export body plus the message.
type emailBody struct {
	exportBody
	To      []string `json:"to"`
	Cc      []string `json:"cc"`
	Subject string   `json:"subject"`
	Message string   `json:"message"`
}

// canSendEmail: a real provider can deliver, or the capture log takes the
// message (dev and e2e).
func canSendEmail(app core.App) bool {
	return mailer.CanDeliver(app) || mailer.CaptureLogOn()
}

func handleEmailStatus(re *core.RequestEvent) error {
	if re.Auth.GetBool("disabled") {
		return re.ForbiddenError(exportDisabled, nil)
	}
	return re.JSON(http.StatusOK, map[string]bool{"canSend": canSendEmail(re.App)})
}

func handleEmail(re *core.RequestEvent) error {
	if re.Auth.GetBool("disabled") {
		return re.ForbiddenError(exportDisabled, nil)
	}
	var body emailBody
	if err := decodeStrict(re, &body, "a report email"); err != nil {
		return err
	}
	if !canSendEmail(re.App) {
		return re.BadRequestError(emailNotSetUp, nil)
	}
	to, cc, subject, err := checkEmail(re, &body)
	if err != nil {
		return err
	}
	// After the form check, so a refused form does not count.
	if !emailLimiter.For(re.App).AllowOrLog(re.Auth.Id, "reports.email") {
		return re.TooManyRequestsError(emailRateLimited, nil)
	}
	file, err := buildExport(re, body.exportBody)
	if err != nil {
		return err
	}
	if len(file.Bytes) > emailMaxBytes {
		return re.Error(http.StatusUnprocessableEntity, emailTooLarge, nil)
	}
	fullSender, ok := emailSender(re.App).(mailer.FullSender)
	if !ok {
		return re.BadRequestError(emailNotSetUp, nil)
	}
	ctx := re.Request.Context()
	res, err := fullSender.SendFull(ctx, &mailer.SendRequest{
		From:     (&mail.Address{Name: senderName(re.Auth) + " via " + re.App.Settings().Meta.AppName, Address: mailer.FromAddress(re.App)}).String(),
		ReplyTo:  re.Auth.Email(),
		To:       recipientsOf(to),
		Cc:       recipientsOf(cc),
		Subject:  subject,
		TextBody: body.Message,
		Attachments: []mailer.Attachment{{
			Name:        file.Name,
			ContentType: file.Mime,
			Content:     base64.StdEncoding.EncodeToString(file.Bytes),
		}},
	})
	all := append(append([]string{}, to...), cc...)
	if err != nil {
		log.ErrorContext(ctx, "report email: not sent", "reportId", file.ReportID, "err", err)
		return re.Error(http.StatusBadGateway, emailNotSent, nil)
	}
	if res != nil && everyRecipientFailed(all, res.FailedRecipients) {
		log.ErrorContext(ctx, "report email: every recipient failed", "reportId", file.ReportID, "failed", res.FailedRecipients)
		return re.Error(http.StatusBadGateway, emailNotSent, nil)
	}
	// The message is out, so a missing audit row is logged, not answered.
	if err := writeExportAudit(re, "report.email", file, map[string]any{"recipients": all}); err != nil {
		log.ErrorContext(ctx, "report email: audit row not written", "reportId", file.ReportID, "err", err)
	}
	return re.JSON(http.StatusOK, map[string]any{"sent": true, "recipients": len(all)})
}

// checkEmail checks the message fields and gives the bare addresses and the
// trimmed subject, or a 400 with an error per field.
func checkEmail(re *core.RequestEvent, body *emailBody) (to, cc []string, subject string, err error) {
	errs := map[string]validation.Error{}
	message := emailBadForm
	to, toBad := parseEmailAddresses(body.To)
	cc, ccBad := parseEmailAddresses(body.Cc)
	if toBad != "" {
		errs["to"] = validation.NewError("invalid_email", toBad+" is not an email address.")
	}
	if ccBad != "" {
		errs["cc"] = validation.NewError("invalid_email", ccBad+" is not an email address.")
	}
	switch {
	case len(body.To) == 0:
		errs["to"] = validation.NewError("required", emailNoTo)
	case len(body.To)+len(body.Cc) > emailMaxAddresses:
		errs["to"] = validation.NewError("too_many", emailTooMany)
		message = emailTooMany
	}
	subject = strings.TrimSpace(body.Subject)
	switch {
	case subject == "":
		errs["subject"] = validation.NewError("required", emailNoSubject)
	case utf8.RuneCountInString(subject) > emailMaxSubject:
		errs["subject"] = validation.NewError("too_long", emailLongSubject)
	case strings.ContainsAny(subject, "\r\n"):
		errs["subject"] = validation.NewError("line_break", emailSubjectBreak)
	}
	if utf8.RuneCountInString(body.Message) > emailMaxMessage {
		errs["message"] = validation.NewError("too_long", emailLongMessage)
	}
	if len(errs) > 0 {
		return nil, nil, "", re.BadRequestError(message, errs)
	}
	return to, cc, subject, nil
}

// parseEmailAddresses gives the bare address of each one, or the first that
// is not an email address.
func parseEmailAddresses(raw []string) (addresses []string, bad string) {
	addresses = make([]string, 0, len(raw))
	for _, a := range raw {
		parsed, err := mail.ParseAddress(strings.TrimSpace(a))
		if err != nil {
			return nil, a
		}
		addresses = append(addresses, parsed.Address)
	}
	return addresses, ""
}

// everyRecipientFailed: the provider took the message but refused each
// address (compared without case).
func everyRecipientFailed(addresses []string, failed []mailer.RecipientFailure) bool {
	if len(failed) == 0 {
		return false
	}
	refused := make(map[string]bool, len(failed))
	for _, f := range failed {
		refused[strings.ToLower(f.Email)] = true
	}
	for _, a := range addresses {
		if !refused[strings.ToLower(a)] {
			return false
		}
	}
	return true
}

func recipientsOf(addresses []string) []mailer.Recipient {
	out := make([]mailer.Recipient, 0, len(addresses))
	for _, a := range addresses {
		out = append(out, mailer.Recipient{Email: a})
	}
	return out
}

// senderName is the reader's name, else the local part of their email.
func senderName(user *core.Record) string {
	if name := strings.TrimSpace(user.GetString("name")); name != "" {
		return name
	}
	local, _, _ := strings.Cut(user.Email(), "@")
	return local
}
