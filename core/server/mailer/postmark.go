package mailer

import (
	"context"
	"fmt"
	"strings"

	"github.com/mrz1836/postmark"
)

var log = &LogSender{}

// PostmarkSender sends email via the Postmark API.
// It implements both Sender (simple) and FullSender (rich with CC/BCC/attachments).
// When delivery is disabled (a --dev process or mail.delivery_enabled=false) it
// logs emails to stdout instead of delivering — see deliveryEnabled.
type PostmarkSender struct {
	client      *postmark.Client
	defaultFrom string
}

// NewPostmarkSender creates a Postmark-backed sender. The API root is
// Postmark's own unless mail.postmark_api_url is set. The postmark_api_url
// override is for test and staging stacks only — it redirects sends to a fake
// or staging Postmark API. It changes mail sends only. Domain and server calls
// (domain validation, webhook setup) ignore it and always use Postmark's API.
func NewPostmarkSender(serverToken, accountToken, defaultFrom string) *PostmarkSender {
	client := postmark.NewClient(serverToken, accountToken)
	if apiURL := ConfigResolver(keyPostmarkAPIURL); apiURL != "" {
		client.BaseURL = strings.TrimRight(apiURL, "/")
	}
	return &PostmarkSender{
		client:      client,
		defaultFrom: defaultFrom,
	}
}

// Client returns the underlying Postmark client for provider-specific operations
// (domain management, etc.) that only the mail package needs.
func (p *PostmarkSender) Client() *postmark.Client {
	return p.client
}

// Send sends a simple transactional email.
func (p *PostmarkSender) Send(ctx context.Context, msg *Message) error {
	if !deliveryEnabled() {
		return log.Send(ctx, msg)
	}
	from := msg.From
	if from == "" {
		from = p.defaultFrom
	}

	email := postmark.Email{
		From:     from,
		To:       FormatRecipients(msg.To),
		Subject:  msg.Subject,
		HTMLBody: msg.HTML,
		TextBody: msg.Text,
		ReplyTo:  msg.ReplyTo,
	}

	resp, err := p.client.SendEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("postmark send failed: %w", err)
	}
	if resp.ErrorCode != 0 {
		return fmt.Errorf("postmark error %d: %s", resp.ErrorCode, resp.Message)
	}
	return nil
}

// buildPostmarkEmail translates a SendRequest into the postmark.Email wire
// shape. Split out from SendFull so request-building is testable without
// the network.
func buildPostmarkEmail(req *SendRequest, defaultFrom string) postmark.Email {
	from := req.From
	if from == "" {
		from = defaultFrom
	}
	email := postmark.Email{
		From:     from,
		To:       FormatRecipients(req.To),
		Cc:       FormatRecipients(req.Cc),
		Bcc:      FormatRecipients(req.Bcc),
		Subject:  req.Subject,
		HTMLBody: req.HTMLBody,
		TextBody: req.TextBody,
		ReplyTo:  req.ReplyTo,
		Metadata: req.Metadata,
	}

	var headers []postmark.Header
	if req.MessageID != "" {
		headers = append(headers, postmark.Header{Name: "Message-ID", Value: req.MessageID})
	}
	if req.InReplyTo != "" {
		headers = append(headers, postmark.Header{Name: "In-Reply-To", Value: req.InReplyTo})
	}
	if req.References != "" {
		headers = append(headers, postmark.Header{Name: "References", Value: req.References})
	}
	for _, h := range req.Headers {
		headers = append(headers, postmark.Header{Name: h.Name, Value: h.Value})
	}
	if len(headers) > 0 {
		email.Headers = headers
	}

	for _, att := range req.Attachments {
		email.Attachments = append(email.Attachments, postmark.Attachment{
			Name:        att.Name,
			ContentType: att.ContentType,
			Content:     att.Content,
			ContentID:   att.ContentID,
		})
	}

	return email
}

// SendFull sends a rich email with CC, BCC, attachments, and threading headers.
func (p *PostmarkSender) SendFull(ctx context.Context, req *SendRequest) (*SendResult, error) {
	if !deliveryEnabled() {
		return log.SendFull(ctx, req)
	}
	email := buildPostmarkEmail(req, p.defaultFrom)

	resp, err := p.client.SendEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("postmark send failed: %w", err)
	}
	if resp.ErrorCode != 0 {
		return nil, fmt.Errorf("postmark error %d: %s", resp.ErrorCode, resp.Message)
	}

	// Postmark's MessageID is its own tracking id, not the Message-ID header
	// recipients see; it is what delivery and bounce webhooks report back.
	messageID := req.MessageID
	if messageID == "" {
		messageID = resp.MessageID
	}
	return &SendResult{
		ProviderMessageID: resp.MessageID,
		MessageID:         messageID,
	}, nil
}

// FormatRecipients formats a slice of Recipients into a comma-separated string.
func FormatRecipients(recipients []Recipient) string {
	parts := make([]string, 0, len(recipients))
	for _, r := range recipients {
		if r.Name != "" {
			parts = append(parts, fmt.Sprintf("%q <%s>", r.Name, r.Email))
		} else {
			parts = append(parts, r.Email)
		}
	}
	return strings.Join(parts, ", ")
}
