// Package mailer provides the shared outbound email stack for all packages —
// a provider registry (Postmark, self-hosted SMTP) selected by configuration,
// so any package can send transactional email without depending on the mail
// feature package. Configuration is read from the system_settings collection
// via ConfigResolver (the mail.* keys), per-send, so /admin edits apply live.
//
// Delivery gating: PocketBase processes started with --dev (dev/test/seed) log
// emails to stdout instead of delivering. In production, delivery is on unless
// the mail.delivery_enabled setting is "false". See deliveryEnabled.
package mailer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
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

// Config keys read from the system_settings collection (via ConfigResolver).
// They live in the shared `mail.*` namespace so the core transactional mailer
// and the mail feature package read one set of credentials.
const (
	keyProvider        = "mail.provider" // "postmark" | "smtp" (default postmark)
	keyPostmarkToken   = "mail.postmark_server_token"
	keyPostmarkAccount = "mail.postmark_account_token"
	keyFromAddress     = "mail.from_address"     // default From; falls back to defaultFromAddress
	keyDeliveryEnabled = "mail.delivery_enabled" // "false" disables delivery (logs instead)
	keySMTPPublicHost  = "mail.smtp_public_hostname"
)

const defaultFromAddress = "noreply@tinycld.org"

// ConfigResolver is the seam through which the mailer reads its configuration.
// coreserver populates it at startup to read from the in-memory SystemConfig
// (backed by the system_settings collection). The default returns "" so the
// package still builds and works standalone (tests, the bootstrap binary) —
// with no token configured it logs instead of delivering.
//
// Reads happen per-send (no caching), so an /admin edit to a mail.* setting
// takes effect on the next send without a restart — matching how the mail
// feature package and Sentry/VAPID consume system config.
var ConfigResolver = func(string) string { return "" }

// devAutoLog is true when this process was started with --dev (dev/test/seed).
// Such processes log emails instead of delivering by default, regardless of
// the mail.delivery_enabled setting, so local/CI runs never send real mail.
// Captured once at init since it's a fixed process property.
var devAutoLog = slices.Contains(os.Args, "--dev")

// deliveryEnabled reports whether outbound delivery is on. Delivery is off in
// --dev processes (auto-log) and when mail.delivery_enabled is explicitly
// "false". Otherwise it is on (the production default).
func deliveryEnabled() bool {
	if devAutoLog {
		return false
	}
	return !strings.EqualFold(ConfigResolver(keyDeliveryEnabled), "false")
}

// fromAddress returns the configured default From, or the package default.
func fromAddress() string {
	if v := ConfigResolver(keyFromAddress); v != "" {
		return v
	}
	return defaultFromAddress
}

// buildSender constructs the configured outbound sender from current config,
// or nil when nothing is configured (e.g. postmark selected but no token).
// Built per-call so config changes apply immediately.
func buildSender() Sender {
	switch strings.ToLower(ConfigResolver(keyProvider)) {
	case "smtp":
		return NewSMTPSender(SMTPConfig{PublicHostname: ConfigResolver(keySMTPPublicHost)})
	case "", "postmark":
		token := ConfigResolver(keyPostmarkToken)
		if token == "" {
			return nil
		}
		return NewPostmarkSender(token, ConfigResolver(keyPostmarkAccount), fromAddress())
	default:
		return nil
	}
}

// DefaultSender returns the Sender to use for a send. When delivery is disabled
// (dev/test or mail.delivery_enabled=false) or no provider is configured, it
// returns the log-only LogSender (which also writes TINYCLD_EMAIL_LOG for e2e).
func DefaultSender() Sender {
	if !deliveryEnabled() {
		return &LogSender{}
	}
	if s := buildSender(); s != nil {
		return s
	}
	return &LogSender{}
}

// CanDeliver reports whether a real outbound provider is configured AND
// delivery is enabled — i.e. whether DefaultSender().Send would actually
// deliver rather than log.
func CanDeliver() bool {
	return deliveryEnabled() && buildSender() != nil
}

// NoopSender silently discards messages.
type NoopSender struct{}

func (n *NoopSender) Send(_ context.Context, _ *Message) error { return nil }
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
	fmt.Println("╭──────────────────────────────────────────────────────────╮")
	fmt.Println("│  EMAIL (not delivered — delivery is disabled)       │")
	fmt.Println("├──────────────────────────────────────────────────────────┤")
	fmt.Printf("│  To:      %s\n", FormatRecipients(msg.To))
	if msg.From != "" {
		fmt.Printf("│  From:    %s\n", msg.From)
	}
	fmt.Printf("│  Subject: %s\n", msg.Subject)
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

	appendToEmailLog(loggedEmail{
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		To:          req.To,
		Cc:          req.Cc,
		Bcc:         req.Bcc,
		From:        req.From,
		Subject:     req.Subject,
		Text:        req.TextBody,
		HTML:        req.HTMLBody,
		Attachments: len(req.Attachments),
	})
	return &SendResult{MessageID: "dev-logged"}, nil
}
