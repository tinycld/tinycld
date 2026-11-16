// Package mailer provides the shared outbound email stack for all packages —
// a provider registry (Postmark, self-hosted SMTP) selected by configuration,
// so any package can send transactional email without depending on the mail
// feature package. Configuration is read per send through core's syscfg seam
// for the app the send belongs to (the mail.* keys), so /admin edits apply live.
//
// Delivery gating: PocketBase processes started with --dev (dev/test/seed) log
// emails to stdout instead of delivering. In production, delivery is on unless
// the mail.delivery_enabled setting is "false". See deliveryEnabled.
package mailer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pocketbase/pocketbase/core"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"tinycld.org/core/maildomains"
	"tinycld.org/core/syscfg"
)

// Recipient is an email address with an optional display name.
type Recipient struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Header is a custom email header.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Attachment is a base64-encoded file attachment.
type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"` // base64 encoded
	ContentID   string `json:"content_id,omitempty"`
}

// SendRequest is a full email send request with CC, BCC, attachments, and threading headers.
type SendRequest struct {
	From        string       `json:"from"`
	To          []Recipient  `json:"to"`
	Cc          []Recipient  `json:"cc,omitempty"`
	Bcc         []Recipient  `json:"bcc,omitempty"`
	Subject     string       `json:"subject"`
	HTMLBody    string       `json:"html_body"`
	TextBody    string       `json:"text_body"`
	ReplyTo     string       `json:"reply_to,omitempty"`
	InReplyTo   string       `json:"in_reply_to,omitempty"`
	References  string       `json:"references,omitempty"`
	Headers     []Header     `json:"headers,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`

	// MessageID is the Message-ID header the provider sends, in "<id@host>"
	// form (see GenerateMessageID). A caller that stores its own copy of the
	// message sets it, so the stored copy carries the ID the recipients see and
	// an IMAP client's copy of the same message can be matched to it. Empty
	// lets the provider choose one.
	MessageID string `json:"message_id,omitempty"`

	// Metadata is passed to providers that echo it back on their delivery
	// and bounce notifications. A notification is often received by
	// something that did not send the message and must work out who did.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SendResult is the response from a successful send.
//
// FailedRecipients carries per-recipient permanent failures from providers that
// surface them at submit time (notably the self-hosted SMTP provider, which
// sees 5xx responses inline during the SMTP conversation). For providers where
// bounces are only known asynchronously via webhook (e.g. Postmark), this slice
// is always empty and the bounce webhook continues to drive delivery_status.
// Callers that store messages locally should mark delivery_status='bounced'
// when this slice contains every original recipient.
type SendResult struct {
	ProviderMessageID string             `json:"provider_message_id"`
	MessageID         string             `json:"message_id"`
	FailedRecipients  []RecipientFailure `json:"failed_recipients,omitempty"`
}

// RecipientFailure is a single recipient that the provider couldn't deliver
// to at submit time. Reason includes the SMTP code+message when available.
type RecipientFailure struct {
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// Message is a simplified email for transactional sends (notifications, invites, etc.).
type Message struct {
	From    string      `json:"from"`
	To      []Recipient `json:"to"`
	Subject string      `json:"subject"`
	HTML    string      `json:"html"`
	Text    string      `json:"text"`
	ReplyTo string      `json:"reply_to,omitempty"`

	// Headers are extra header fields, by name. Every Sender writes them.
	Headers map[string]string `json:"headers,omitempty"`
}

// sendRequest is the message in the SendRequest shape, so the providers send
// a Message and a SendRequest through one path. Headers are sorted by name:
// a map has no order, and the same message must give the same wire bytes.
// A header the sender writes itself, or one that could inject a header line,
// is refused (see checkHeader).
func (m *Message) sendRequest() (*SendRequest, error) {
	names := make([]string, 0, len(m.Headers))
	for name := range m.Headers {
		names = append(names, name)
	}
	slices.Sort(names)
	var headers []Header
	for _, name := range names {
		if err := checkHeader(name, m.Headers[name]); err != nil {
			return nil, err
		}
		headers = append(headers, Header{Name: name, Value: m.Headers[name]})
	}
	return &SendRequest{
		From:     m.From,
		To:       m.To,
		Subject:  m.Subject,
		HTMLBody: m.HTML,
		TextBody: m.Text,
		ReplyTo:  m.ReplyTo,
		Headers:  headers,
	}, nil
}

// reservedHeaders are the fields a sender writes from the Message itself, or
// that describe the body. A custom header with one of these names would
// replace them (SMTP) or duplicate them (Postmark).
var reservedHeaders = map[string]bool{
	"from": true, "to": true, "cc": true, "bcc": true, "reply-to": true, "sender": true,
	"subject": true, "date": true, "message-id": true, "mime-version": true,
}

// checkHeader refuses a reserved name (any Content-* name too), a CR or LF in
// the name or value (it would start a new header line), and a name with a
// character outside the RFC 5322 field-name set: printable US-ASCII except
// the colon.
func checkHeader(name, value string) error {
	lower := strings.ToLower(name)
	if reservedHeaders[lower] || strings.HasPrefix(lower, "content-") {
		return fmt.Errorf("header %q is reserved", name)
	}
	if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("header %q contains a line break", name)
	}
	if name == "" {
		return fmt.Errorf("header name is empty")
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; c < 33 || c > 126 || c == ':' {
			return fmt.Errorf("header name %q has a character not allowed in a field name", name)
		}
	}
	return nil
}

// Sender can send simple transactional emails.
type Sender interface {
	Send(ctx context.Context, msg *Message) error
}

// FullSender can send rich emails with CC, BCC, attachments, and threading headers.
type FullSender interface {
	SendFull(ctx context.Context, req *SendRequest) (*SendResult, error)
}

// --- Configuration ---

// Config keys read from the system_settings collection (via the app's syscfg seam).
// They live in the shared `mail.*` namespace so the core transactional mailer
// and the mail feature package read one set of credentials.
const (
	keyProvider        = "mail.provider" // "postmark" | "smtp" (default postmark)
	keyPostmarkToken   = "mail.postmark_server_token"
	keyPostmarkAccount = "mail.postmark_account_token"
	keyFromAddress     = "mail.from_address"     // default From; falls back to defaultFromAddress
	keyDeliveryEnabled = "mail.delivery_enabled" // "false" disables delivery (logs instead)
	keySMTPPublicHost  = "mail.smtp_public_hostname"
	// keyPostmarkAPIURL replaces Postmark's API root. It is unset in
	// production; a test stack sets it to point sends at a stand-in server.
	keyPostmarkAPIURL = "mail.postmark_api_url"
)

const defaultFromAddress = "noreply@tinycld.org"

// tokenResolveTimeout bounds the first derivation's Postmark calls; later
// calls are answered from the resolver's cache.
const tokenResolveTimeout = 15 * time.Second

// postmarkServerToken is the server token app sends with. It asks app's
// maildomains token source, which derives the token from the account token
// when no server token is configured; an app whose composition installed no
// source (a bare test app, the bootstrap binary) still sends with a
// configured server token.
func postmarkServerToken(app core.App) string {
	ctx, cancel := context.WithTimeout(context.Background(), tokenResolveTimeout)
	defer cancel()
	token, err := maildomains.ServerToken(ctx, app)
	if err == nil {
		return token
	}
	if configured := syscfg.Get(app, keyPostmarkToken); configured != "" {
		return configured
	}
	fmt.Fprintf(os.Stderr, "[mailer] no postmark server token: %v — mail will be logged, not delivered\n", err)
	return ""
}

// devAutoLog is true when this process was started with --dev (dev/test/seed).
// Such processes log emails instead of delivering by default, regardless of
// the mail.delivery_enabled setting, so local/CI runs never send real mail.
// Captured once at init since it's a fixed process property.
var devAutoLog = slices.Contains(os.Args, "--dev")

// deliveryEnabled reports whether app's outbound delivery is on. Delivery is
// off in --dev processes (auto-log) and when mail.delivery_enabled is
// explicitly "false". Otherwise it is on (the production default).
func deliveryEnabled(app core.App) bool {
	if devAutoLog {
		return false
	}
	return !strings.EqualFold(syscfg.Get(app, keyDeliveryEnabled), "false")
}

// CaptureLogOn reports whether outbound email is captured instead of sent:
// TINYCLD_EMAIL_LOG names a log file (e2e), or the process runs with --dev,
// where DefaultSender is a LogSender.
func CaptureLogOn() bool {
	return os.Getenv("TINYCLD_EMAIL_LOG") != "" || devAutoLog
}

// FromAddress returns app's configured default From address, or the package
// default: the address a send on behalf of a user goes out from.
func FromAddress(app core.App) string {
	return fromAddress(app)
}

// fromAddress returns app's configured default From, or the package default.
func fromAddress(app core.App) string {
	if v := syscfg.Get(app, keyFromAddress); v != "" {
		return v
	}
	return defaultFromAddress
}

// buildSender constructs app's configured outbound sender from current
// config, or nil when nothing is configured (e.g. postmark selected but no
// token). Built per call so config changes apply immediately.
func buildSender(app core.App) Sender {
	switch strings.ToLower(syscfg.Get(app, keyProvider)) {
	case "smtp":
		return NewSMTPSender(app, SMTPConfig{PublicHostname: syscfg.Get(app, keySMTPPublicHost)})
	case "", "postmark":
		token := postmarkServerToken(app)
		if token == "" {
			return nil
		}
		return NewPostmarkSender(app, token, syscfg.Get(app, keyPostmarkAccount), fromAddress(app))
	default:
		// An unrecognized provider would silently degrade to log-only, which
		// looks to an operator like mail is configured when it isn't. Surface it.
		fmt.Fprintf(os.Stderr, "[mailer] unknown %s=%q — mail will be logged, not delivered\n",
			keyProvider, syscfg.Get(app, keyProvider))
		return nil
	}
}

// DefaultSender returns the Sender for a send on behalf of app. When delivery
// is disabled (dev/test or mail.delivery_enabled=false) or no provider is
// configured, it returns the log-only LogSender (which also writes
// TINYCLD_EMAIL_LOG for e2e).
func DefaultSender(app core.App) Sender {
	if !deliveryEnabled(app) {
		return &LogSender{}
	}
	if s := buildSender(app); s != nil {
		return s
	}
	return &LogSender{}
}

// CanDeliver reports whether app has a real outbound provider configured AND
// delivery enabled — i.e. whether DefaultSender(app).Send would actually
// deliver rather than log.
func CanDeliver(app core.App) bool {
	return deliveryEnabled(app) && buildSender(app) != nil
}

// NoopSender silently discards messages.
type NoopSender struct{}

// Send discards the message, but refuses unsafe headers as every Sender does,
// so a bad message fails the same way whichever sender is configured.
func (n *NoopSender) Send(_ context.Context, msg *Message) error {
	_, err := msg.sendRequest()
	return err
}
func (n *NoopSender) SendFull(_ context.Context, _ *SendRequest) (*SendResult, error) {
	return &SendResult{}, nil
}

// LogSender prints emails to stdout instead of delivering them. If the
// TINYCLD_EMAIL_LOG env var is set, each email is also appended as a JSON
// line to that file — used by e2e tests to assert on outbound email.
type LogSender struct{}

// loggedEmail is the JSON shape written to TINYCLD_EMAIL_LOG. One JSON
// object per line (JSONL), so tests can read the file lazily.
type loggedEmail struct {
	Timestamp   string      `json:"timestamp"`
	To          []Recipient `json:"to"`
	Cc          []Recipient `json:"cc,omitempty"`
	Bcc         []Recipient `json:"bcc,omitempty"`
	From        string      `json:"from,omitempty"`
	Subject     string      `json:"subject"`
	Text        string      `json:"text,omitempty"`
	HTML        string      `json:"html,omitempty"`
	Attachments int         `json:"attachments,omitempty"`
	ReplyTo     string      `json:"reply_to,omitempty"`
	// AttachmentNames are the file names of the attachments, in order.
	AttachmentNames []string `json:"attachment_names,omitempty"`

	Headers map[string]string `json:"headers,omitempty"`
}

// fileLogMu serializes concurrent writes so JSONL lines never interleave.
var fileLogMu sync.Mutex

func appendToEmailLog(entry loggedEmail) {
	path := os.Getenv("TINYCLD_EMAIL_LOG")
	if path == "" {
		return
	}
	fileLogMu.Lock()
	defer fileLogMu.Unlock()

	// Ensure the parent dir exists: O_CREATE makes the file but not the
	// directory. In e2e the webServer (and thus PB) can boot and send the
	// first email before globalSetup has created tmp/, so create it here.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[mailer] failed to create email log dir %q: %v\n", filepath.Dir(path), err)
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mailer] failed to open email log %q: %v\n", path, err)
		return
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mailer] failed to marshal email log entry: %v\n", err)
		return
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "[mailer] failed to write email log: %v\n", err)
	}
}

func (l *LogSender) Send(_ context.Context, msg *Message) error {
	// Checked before anything prints, so a refused message prints nothing.
	req, err := msg.sendRequest()
	if err != nil {
		return err
	}
	fmt.Println("╭──────────────────────────────────────────────────────────╮")
	fmt.Println("│  EMAIL (not delivered — delivery is disabled)       │")
	fmt.Println("├──────────────────────────────────────────────────────────┤")
	fmt.Printf("│  To:      %s\n", FormatRecipients(msg.To))
	if msg.From != "" {
		fmt.Printf("│  From:    %s\n", msg.From)
	}
	fmt.Printf("│  Subject: %s\n", msg.Subject)
	for _, h := range req.Headers {
		fmt.Printf("│  %s: %s\n", h.Name, h.Value)
	}
	fmt.Println("├──────────────────────────────────────────────────────────┤")
	if msg.Text != "" {
		fmt.Println(msg.Text)
	} else {
		fmt.Println("(HTML only — no text body)")
	}
	fmt.Println("╰──────────────────────────────────────────────────────────╯")

	appendToEmailLog(loggedEmail{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		To:        msg.To,
		From:      msg.From,
		Subject:   msg.Subject,
		Text:      msg.Text,
		HTML:      msg.HTML,
		ReplyTo:   msg.ReplyTo,
		Headers:   msg.Headers,
	})
	return nil
}

func (l *LogSender) SendFull(_ context.Context, req *SendRequest) (*SendResult, error) {
	fmt.Println("╭──────────────────────────────────────────────────────────╮")
	fmt.Println("│  EMAIL (not delivered — delivery is disabled)       │")
	fmt.Println("├──────────────────────────────────────────────────────────┤")
	fmt.Printf("│  To:      %s\n", FormatRecipients(req.To))
	if len(req.Cc) > 0 {
		fmt.Printf("│  Cc:      %s\n", FormatRecipients(req.Cc))
	}
	if len(req.Bcc) > 0 {
		fmt.Printf("│  Bcc:     %s\n", FormatRecipients(req.Bcc))
	}
	if req.From != "" {
		fmt.Printf("│  From:    %s\n", req.From)
	}
	fmt.Printf("│  Subject: %s\n", req.Subject)
	if len(req.Attachments) > 0 {
		fmt.Printf("│  Attach:  %d file(s)\n", len(req.Attachments))
	}
	fmt.Println("├──────────────────────────────────────────────────────────┤")
	if req.TextBody != "" {
		fmt.Println(req.TextBody)
	} else {
		fmt.Println("(HTML only — no text body)")
	}
	fmt.Println("╰──────────────────────────────────────────────────────────╯")

	var attachmentNames []string
	for _, a := range req.Attachments {
		attachmentNames = append(attachmentNames, a.Name)
	}
	appendToEmailLog(loggedEmail{
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
		To:              req.To,
		Cc:              req.Cc,
		Bcc:             req.Bcc,
		From:            req.From,
		Subject:         req.Subject,
		Text:            req.TextBody,
		HTML:            req.HTMLBody,
		Attachments:     len(req.Attachments),
		ReplyTo:         req.ReplyTo,
		AttachmentNames: attachmentNames,
	})
	messageID := req.MessageID
	if messageID == "" {
		messageID = "dev-logged"
	}
	return &SendResult{MessageID: messageID}, nil
}
