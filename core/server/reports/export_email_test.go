package reports

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/mailer"
	"tinycld.org/core/oauth"
)

const (
	emailURL       = "/api/reports/email"
	emailStatusURL = "/api/reports/email-status"
	// zooOrgName is the organization's name the tests give the app.
	zooOrgName = "Zoo Keepers Ltd"
)

// capturedEmail is one line of the mail capture log (TINYCLD_EMAIL_LOG).
type capturedEmail struct {
	To              []mailer.Recipient `json:"to"`
	Cc              []mailer.Recipient `json:"cc"`
	From            string             `json:"from"`
	Subject         string             `json:"subject"`
	Text            string             `json:"text"`
	Attachments     int                `json:"attachments"`
	ReplyTo         string             `json:"reply_to"`
	AttachmentNames []string           `json:"attachment_names"`
}

// captureEmails turns the mail capture log on, in a file of the test's own,
// and names the organization.
func captureEmails(t *testing.T, s *savedZoo) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "emails.jsonl")
	t.Setenv("TINYCLD_EMAIL_LOG", path)
	s.app.Settings().Meta.AppName = zooOrgName
	return path
}

// capturedEmails reads every message the capture log holds.
func capturedEmails(t *testing.T, path string) []capturedEmail {
	t.Helper()
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []capturedEmail
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		var e capturedEmail
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("log line %s: %v", scanner.Text(), err)
		}
		out = append(out, e)
	}
	return out
}

func emailsOf(rs []mailer.Recipient) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Email)
	}
	return out
}

// emailRequest is the export body of a saved report on the reader's day
// 2026-11-16 plus the message fields.
func emailRequest(reportID, format string, to, cc []string, subject, message string) string {
	raw, err := json.Marshal(map[string]any{
		"reportId": reportID,
		"params":   map[string]any{},
		"options":  map[string]any{"today": "2026-11-16", "timeZone": "UTC"},
		"format":   format,
		"hour12":   false,
		"to":       to,
		"cc":       cc,
		"subject":  subject,
		"message":  message,
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// plainEmail is a valid send of format to one address.
func plainEmail(reportID, format, to string) string {
	return emailRequest(reportID, format, []string{to}, []string{}, "Feeding Costs", "Hello")
}

// emailAs sends one email request through the real router as user ("" =
// signed out).
func (s *savedZoo) emailAs(t *testing.T, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	return s.serve(t, user, http.MethodPost, emailURL, body)
}

func (s *savedZoo) emailStatusAs(t *testing.T, user string) *httptest.ResponseRecorder {
	t.Helper()
	return s.serve(t, user, http.MethodGet, emailStatusURL, "")
}

func (s *savedZoo) serve(t *testing.T, user, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("Authorization", s.token(t, user))
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func auditRowsOf(t *testing.T, app core.App, action string) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("audit_logs", "action = {:action}", "", 0, 0, map[string]any{"action": action})
	if err != nil {
		t.Fatalf("find audit rows: %v", err)
	}
	return rows
}

// requireNothingSent fails when the log holds a message or a report.email
// audit row was written.
func requireNothingSent(t *testing.T, s *savedZoo, path string) {
	t.Helper()
	if got := capturedEmails(t, path); len(got) != 0 {
		t.Fatalf("%d messages were sent: %+v", len(got), got)
	}
	if n := len(auditRowsOf(t, s.app, "report.email")); n != 0 {
		t.Fatalf("%d report.email audit rows were written", n)
	}
}

// One send is one message from the server's sender, in the reader's name, with
// the file attached under the download's name, and one report.email audit row.
func TestEmailSendsTheFileAsOneMessage(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	path := captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	to := []string{"lee@zoo.test", "max@zoo.test"}
	cc := []string{"kim@zoo.test"}
	message := "Hello,\n\nAttached is the Feeding Costs report.\n\nann"

	rec := s.emailAs(t, "ann", emailRequest(rep.Id, "pdf", to, cc, "Feeding Costs — March", message))
	requireStatus(t, rec, http.StatusOK)
	var answer struct {
		Sent       bool `json:"sent"`
		Recipients int  `json:"recipients"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("answer %s: %v", rec.Body.String(), err)
	}
	if !answer.Sent || answer.Recipients != 3 {
		t.Fatalf("answer = %s, want {sent: true, recipients: 3}", rec.Body.String())
	}

	sent := capturedEmails(t, path)
	if len(sent) != 1 {
		t.Fatalf("%d messages logged, want 1: %+v", len(sent), sent)
	}
	got := sent[0]
	if !slices.Equal(emailsOf(got.To), to) || !slices.Equal(emailsOf(got.Cc), cc) {
		t.Fatalf("to %q cc %q, want %q and %q", emailsOf(got.To), emailsOf(got.Cc), to, cc)
	}
	if got.Subject != "Feeding Costs — March" || got.Text != message {
		t.Fatalf("subject %q text %q, want the ones sent", got.Subject, got.Text)
	}
	from := (&mail.Address{Name: "ann via " + zooOrgName, Address: mailer.FromAddress(s.app)}).String()
	if got.From != from {
		t.Fatalf("from = %q, want %q", got.From, from)
	}
	if got.ReplyTo != "ann@zoo.test" {
		t.Fatalf("reply_to = %q, want the sender's own address", got.ReplyTo)
	}
	if got.Attachments != 1 || !slices.Equal(got.AttachmentNames, []string{"feeding-costs-2026-11-16.pdf"}) {
		t.Fatalf("attachments %d named %q, want the one file feeding-costs-2026-11-16.pdf", got.Attachments, got.AttachmentNames)
	}

	rows := auditRowsOf(t, s.app, "report.email")
	if len(rows) != 1 {
		t.Fatalf("%d report.email audit rows, want 1", len(rows))
	}
	row := rows[0]
	if row.GetString("resource_id") != rep.Id || row.GetString("resource_label") != "Feeding Costs" || row.GetString("actor") != s.id("ann") {
		t.Fatalf("audit row %v, want the report's id and title, and ann", row.FieldsData())
	}
	var meta struct {
		Format     string   `json:"format"`
		Rows       float64  `json:"rows"`
		Recipients []string `json:"recipients"`
	}
	if err := json.Unmarshal([]byte(row.GetString("metadata")), &meta); err != nil {
		t.Fatalf("metadata %s: %v", row.GetString("metadata"), err)
	}
	if meta.Format != "pdf" || meta.Rows != 5 || !slices.Equal(meta.Recipients, append(slices.Clone(to), cc...)) {
		t.Fatalf("metadata = %+v, want pdf, 5 rows and the three addresses", meta)
	}
	if n := len(auditRowsOf(t, s.app, "report.export")); n != 0 {
		t.Fatalf("a sent email also wrote %d report.export rows", n)
	}
}

func TestEmailAttachesTheChosenFormat(t *testing.T) {
	s := newSavedZoo(t)
	path := captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	for _, format := range []string{"xlsx", "csv"} {
		requireStatus(t, s.emailAs(t, "ann", plainEmail(rep.Id, format, format+"@zoo.test")), http.StatusOK)
	}
	sent := capturedEmails(t, path)
	if len(sent) != 2 {
		t.Fatalf("%d messages logged, want 2", len(sent))
	}
	for i, want := range []string{"feeding-costs-2026-11-16.xlsx", "feeding-costs-2026-11-16.csv"} {
		if !slices.Equal(sent[i].AttachmentNames, []string{want}) {
			t.Fatalf("message %d attaches %q, want %q", i, sent[i].AttachmentNames, want)
		}
	}
}

func TestEmailRefusesABadForm(t *testing.T) {
	s := newSavedZoo(t)
	path := captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	eleven := make([]string, 0, 11)
	for i := range 11 {
		eleven = append(eleven, fmt.Sprintf("p%d@zoo.test", i))
	}
	ok := []string{"lee@zoo.test"}
	for name, c := range map[string]struct {
		body  string
		parts []string
	}{
		"no To address":   {body: emailRequest(rep.Id, "csv", []string{}, []string{}, "S", "M")},
		"a bad address":   {body: emailRequest(rep.Id, "csv", []string{"lee@zoo.test", "not-an-address"}, []string{}, "S", "M")},
		"11 addresses":    {body: emailRequest(rep.Id, "csv", eleven[:6], eleven[6:], "S", "M"), parts: []string{`"message":"A report email goes to at most 10 addresses."`}},
		"an empty subj":   {body: emailRequest(rep.Id, "csv", ok, []string{}, "", "M")},
		"a blank subj":    {body: emailRequest(rep.Id, "csv", ok, []string{}, "   ", "M")},
		"subject of 201":  {body: emailRequest(rep.Id, "csv", ok, []string{}, strings.Repeat("s", 201), "M")},
		"message of 5001": {body: emailRequest(rep.Id, "csv", ok, []string{}, "S", strings.Repeat("m", 5001))},
		"CR in subject":   {body: emailRequest(rep.Id, "csv", ok, []string{}, "S\rBcc: spy@zoo.test", "M")},
		"LF in subject":   {body: emailRequest(rep.Id, "csv", ok, []string{}, "S\nBcc: spy@zoo.test", "M")},
	} {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, s.emailAs(t, "ann", c.body), http.StatusBadRequest, c.parts...)
		})
	}
	// 200 characters and 5 000 characters are still accepted.
	requireStatus(t, s.emailAs(t, "ann", emailRequest(rep.Id, "csv", ok, []string{}, strings.Repeat("s", 200), strings.Repeat("m", 5000))), http.StatusOK)
	if got := capturedEmails(t, path); len(got) != 1 {
		t.Fatalf("%d messages logged, want only the valid one", len(got))
	}
}

func TestEmailRefusesAFileTooLargeToEmail(t *testing.T) {
	s := newSavedZoo(t)
	path := captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	saved := emailMaxBytes
	emailMaxBytes = 10
	t.Cleanup(func() { emailMaxBytes = saved })
	requireStatus(t, s.emailAs(t, "ann", plainEmail(rep.Id, "csv", "lee@zoo.test")), http.StatusUnprocessableEntity,
		`"message":"The file is too large to email. Download it instead."`)
	requireNothingSent(t, s, path)
}

// 20 report emails per user an hour; a refused form does not count, and
// another user has a count of their own.
func TestEmailLimitsSendsPerUser(t *testing.T) {
	s := newSavedZoo(t)
	path := captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, true)
	for range 3 {
		requireStatus(t, s.emailAs(t, "ann", emailRequest(rep.Id, "csv", []string{"not-an-address"}, []string{}, "S", "M")), http.StatusBadRequest)
	}
	for i := range 20 {
		rec := s.emailAs(t, "ann", plainEmail(rep.Id, "csv", fmt.Sprintf("r%d@zoo.test", i)))
		if rec.Code != http.StatusOK {
			t.Fatalf("send %d: status %d, want 200\n%s", i+1, rec.Code, rec.Body.String())
		}
	}
	requireStatus(t, s.emailAs(t, "ann", plainEmail(rep.Id, "csv", "late@zoo.test")), http.StatusTooManyRequests,
		`"message":"You have sent 20 report emails in the last hour. Try again later."`)
	requireStatus(t, s.emailAs(t, "bob", plainEmail(rep.Id, "csv", "bob-to@zoo.test")), http.StatusOK)
	if n := len(capturedEmails(t, path)); n != 21 {
		t.Fatalf("%d messages logged, want 21", n)
	}
}

func TestEmailKeepsTheDownloadLimits(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	path := captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	s.feeding(t, "f7", s.recs["leo"], 400, "2026-04-01", 10)
	// ann's report is now five feedings and a total: six rows.
	keepExportLimits(t)
	exportLimits.pdfRows = 5
	requireStatus(t, s.emailAs(t, "ann", plainEmail(rep.Id, "pdf", "lee@zoo.test")), http.StatusUnprocessableEntity,
		`"message":"This report has more than 5,000 rows. Narrow the period or download CSV."`)
	requireNothingSent(t, s, path)
}

func TestEmailRefusesWhoCannotSend(t *testing.T) {
	s := newSavedZoo(t)
	path := captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)

	requireStatus(t, s.emailAs(t, "ot", plainEmail(rep.Id, "csv", "lee@zoo.test")), http.StatusForbidden,
		`"message":"You no longer have access to this report."`)
	requireStatus(t, s.emailAs(t, "", plainEmail(rep.Id, "csv", "lee@zoo.test")), http.StatusUnauthorized)
	requireStatus(t, s.emailStatusAs(t, ""), http.StatusUnauthorized)

	ed := s.recs["ed"]
	ed.Set("disabled", true)
	if err := s.app.Save(ed); err != nil {
		t.Fatal(err)
	}
	// ed holds an editor's share of R; a disabled account still sends nothing.
	requireStatus(t, s.emailAs(t, "ed", plainEmail(s.id("R"), "csv", "lee@zoo.test")), http.StatusForbidden,
		`"message":"Your account is disabled."`)
	requireStatus(t, s.emailStatusAs(t, "ed"), http.StatusForbidden)
	requireNothingSent(t, s, path)
}

// Both routes take a signed-in session only, as the export route does.
func TestEmailRoutesRefuseAnOAuthToken(t *testing.T) {
	z := newZoo(t)
	t.Setenv("TINYCLD_EMAIL_LOG", filepath.Join(t.TempDir(), "emails.jsonl"))
	oauth.Register(z.app)
	Register(z.app)
	rep := z.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	client, err := oauth.FindClientByClientID(z.app, "tinycld-cli")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := oauth.NewGrant(z.app, z.id("ann"), client.Id, oauth.AllScopes(), "active")
	if err != nil {
		t.Fatal(err)
	}
	token, err := oauth.MintAccessToken(z.app, z.recs["ann"], grant, oauth.AccessTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range []tests.ApiScenario{
		{Name: "send with an OAuth token", Method: http.MethodPost, URL: emailURL,
			Body:           strings.NewReader(plainEmail(rep.Id, "csv", "lee@zoo.test")),
			Headers:        map[string]string{"Authorization": "Bearer " + token},
			ExpectedStatus: http.StatusForbidden, ExpectedContent: []string{`not available to API tokens`}},
		{Name: "status with an OAuth token", Method: http.MethodGet, URL: emailStatusURL,
			Headers:        map[string]string{"Authorization": "Bearer " + token},
			ExpectedStatus: http.StatusForbidden, ExpectedContent: []string{`not available to API tokens`}},
		// The same reader's session gets the status.
		{Name: "status with a session", Method: http.MethodGet, URL: emailStatusURL,
			Headers:        map[string]string{"Authorization": z.token(t, "ann")},
			ExpectedStatus: http.StatusOK, ExpectedContent: []string{`"canSend":true`}},
	} {
		sc.TestAppFactory = func(testing.TB) *tests.TestApp { return z.app }
		sc.DisableTestAppCleanup = true
		sc.Test(t)
	}
}

func TestEmailStatusSaysWhetherTheServerCanSend(t *testing.T) {
	s := newSavedZoo(t)
	captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	on := s.emailStatusAs(t, "ann")
	requireStatus(t, on, http.StatusOK)
	if got := strings.TrimSpace(on.Body.String()); got != `{"canSend":true}` {
		t.Fatalf("status with the capture log on = %s", got)
	}

	// No capture log and no provider: nothing can send.
	t.Setenv("TINYCLD_EMAIL_LOG", "")
	off := s.emailStatusAs(t, "ann")
	requireStatus(t, off, http.StatusOK)
	if got := strings.TrimSpace(off.Body.String()); got != `{"canSend":false}` {
		t.Fatalf("status with nothing to send by = %s", got)
	}
	requireStatus(t, s.emailAs(t, "ann", plainEmail(rep.Id, "csv", "lee@zoo.test")), http.StatusBadRequest,
		`"message":"Email sending is not set up."`)
	if n := len(auditRowsOf(t, s.app, "report.email")); n != 0 {
		t.Fatalf("a refused send wrote %d audit rows", n)
	}
}

// failingSender is a provider that refuses a send, or accepts it and fails
// every recipient.
type failingSender struct {
	result *mailer.SendResult
	err    error
}

func (f failingSender) Send(context.Context, *mailer.Message) error { return f.err }
func (f failingSender) SendFull(context.Context, *mailer.SendRequest) (*mailer.SendResult, error) {
	return f.result, f.err
}

func TestEmailThatCannotBeSentIsAServerError(t *testing.T) {
	s := newSavedZoo(t)
	captureEmails(t, s)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	saved := emailSender
	t.Cleanup(func() { emailSender = saved })
	body := emailRequest(rep.Id, "csv", []string{"lee@zoo.test", "max@zoo.test"}, []string{"kim@zoo.test"}, "S", "M")

	for name, sender := range map[string]failingSender{
		"the provider refuses": {err: errors.New("provider down")},
		"every recipient fails": {result: &mailer.SendResult{FailedRecipients: []mailer.RecipientFailure{
			{Email: "lee@zoo.test", Reason: "550"}, {Email: "max@zoo.test", Reason: "550"}, {Email: "kim@zoo.test", Reason: "550"},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			emailSender = func(core.App) mailer.Sender { return sender }
			requireStatus(t, s.emailAs(t, "ann", body), http.StatusBadGateway, `"message":"The email could not be sent."`)
			if n := len(auditRowsOf(t, s.app, "report.email")); n != 0 {
				t.Fatalf("an unsent email wrote %d audit rows", n)
			}
		})
	}
}
