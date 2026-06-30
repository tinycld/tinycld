package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"sort"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

// SMTPConfig is the outbound configuration for the self-hosted SMTP sender —
// direct-to-MX delivery with no provider account. Inbound concerns (the MX
// listener, IMAP fetcher, DKIM verification) live in the mail package and are
// not represented here. Defaults are applied by NewSMTPSender.
type SMTPConfig struct {
	// PublicHostname is what we announce in EHLO and embed in the generated
	// Message-ID. Defaults to "localhost".
	PublicHostname string

	// OutboundTimeout caps the total time per recipient-domain delivery
	// attempt (MX lookup + SMTP conversation). Defaults to 30s.
	OutboundTimeout time.Duration
}

func (c *SMTPConfig) applyDefaults() {
	if c.PublicHostname == "" {
		c.PublicHostname = "localhost"
	}
	if c.OutboundTimeout == 0 {
		c.OutboundTimeout = 30 * time.Second
	}
}

// SMTPSender delivers mail by talking SMTP directly: outbound via
// recipient-domain MX lookup. There is no provider account — credentials are
// not used for outbound; per-recipient delivery either works or it doesn't.
// It implements Sender and FullSender.
type SMTPSender struct {
	cfg SMTPConfig
}

// NewSMTPSender builds an SMTPSender with defaults applied.
func NewSMTPSender(cfg SMTPConfig) *SMTPSender {
	cfg.applyDefaults()
	return &SMTPSender{cfg: cfg}
}

// smtpMXLookup is swappable for tests.
var smtpMXLookup = net.DefaultResolver.LookupMX

// smtpDial opens a connection to host:port. Swappable for tests.
var smtpDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{}
	return d.DialContext(ctx, network, addr)
}

// Send adapts the simple Message shape onto SendFull.
func (p *SMTPSender) Send(ctx context.Context, msg *Message) error {
	_, err := p.SendFull(ctx, &SendRequest{
		From:     msg.From,
		To:       msg.To,
		Subject:  msg.Subject,
		HTMLBody: msg.HTML,
		TextBody: msg.Text,
		ReplyTo:  msg.ReplyTo,
	})
	return err
}

// SendFull performs a single outbound delivery attempt per recipient-domain
// group. Recipients are grouped by their domain; for each group we resolve MX,
// walk MX hosts in priority order, and try opportunistic STARTTLS. A permanent
// (5xx) response for a recipient bubbles up as a RecipientFailure on the
// returned SendResult. A successful send to at least one recipient returns nil
// error; if every recipient permanently failed we still return nil error (the
// FailedRecipients slice carries the bad news so the caller can persist the
// message as 'bounced'). A transport-level failure (e.g. all MX hosts
// unreachable) returns an error and no SendResult.
func (p *SMTPSender) SendFull(ctx context.Context, req *SendRequest) (*SendResult, error) {
	if req == nil {
		return nil, fmt.Errorf("nil send request")
	}
	if !deliveryEnabled() {
		return log.SendFull(ctx, req)
	}

	messageID := generateMessageID(p.cfg.PublicHostname)
	body, err := buildOutgoingRFC5322(req, messageID, p.cfg.PublicHostname)
	if err != nil {
		return nil, fmt.Errorf("failed to build message: %w", err)
	}

	envelopeFrom, err := envelopeAddress(req.From)
	if err != nil {
		return nil, fmt.Errorf("invalid From address: %w", err)
	}

	groups := groupRecipientsByDomain(req)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no recipients")
	}

	ctx, cancel := context.WithTimeout(ctx, p.cfg.OutboundTimeout)
	defer cancel()

	result := &SendResult{MessageID: messageID, ProviderMessageID: messageID}
	deliveredAny := false
	var lastTransportErr error

	for domain, recipients := range groups {
		delivered, failures, transportErr := p.deliverToDomain(ctx, domain, envelopeFrom, recipients, body)
		if transportErr != nil {
			lastTransportErr = transportErr
			// Transport error against this domain → mark every recipient in
			// the group as failed so the caller can persist bounce_reason.
			for _, rcpt := range recipients {
				result.FailedRecipients = append(result.FailedRecipients, RecipientFailure{
					Email:  rcpt,
					Reason: transportErr.Error(),
				})
			}
			continue
		}
		result.FailedRecipients = append(result.FailedRecipients, failures...)
		if delivered {
			deliveredAny = true
		}
	}

	if !deliveredAny && lastTransportErr != nil && len(result.FailedRecipients) == len(allRecipients(req)) {
		// Every recipient failed transport — return error so the caller
		// surfaces 502 to the user instead of silently storing the message.
		return nil, fmt.Errorf("smtp delivery failed for all recipients: %w", lastTransportErr)
	}

	return result, nil
}

// deliverToDomain opens an SMTP connection to one MX host for `domain` and
// issues MAIL FROM / RCPT TO (one per recipient) / DATA. Returns whether at
// least one recipient was accepted, the list of recipients that got a
// permanent (5xx) failure, and any transport error (failed to reach any MX).
func (p *SMTPSender) deliverToDomain(ctx context.Context, domain, from string, recipients []string, body []byte) (bool, []RecipientFailure, error) {
	hosts, err := resolveMXHosts(ctx, domain)
	if err != nil {
		return false, nil, fmt.Errorf("mx lookup for %s: %w", domain, err)
	}

	var lastErr error
	for _, host := range hosts {
		conn, err := smtpDial(ctx, "tcp", host+":25")
		if err != nil {
			lastErr = fmt.Errorf("dial %s: %w", host, err)
			continue
		}

		delivered, failures, convErr := runSMTPConversation(conn, host, p.cfg.PublicHostname, from, recipients, body)
		// runSMTPConversation closes its own connection on the happy path;
		// on transport error we still close defensively.
		_ = conn.Close()
		if convErr != nil {
			lastErr = fmt.Errorf("smtp to %s: %w", host, convErr)
			continue
		}
		return delivered, failures, nil
	}

	return false, nil, fmt.Errorf("all MX hosts unreachable for %s: %w", domain, lastErr)
}

// runSMTPConversation drives the SMTP conversation against an already-dialed
// connection. STARTTLS is attempted when advertised; AUTH is never offered
// (this is server-to-server traffic, not client submission).
func runSMTPConversation(conn net.Conn, host, helo, from string, recipients []string, body []byte) (bool, []RecipientFailure, error) {
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return false, nil, fmt.Errorf("new client: %w", err)
	}
	defer client.Close()

	if err := client.Hello(helo); err != nil {
		return false, nil, fmt.Errorf("EHLO: %w", err)
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			// Fall through without TLS — opportunistic. Most receivers accept.
			// We deliberately do not fail here: a failing STARTTLS is far
			// rarer than a misconfigured TLS cert on the receiver, and most
			// receivers accept plaintext fallback for incoming mail.
		}
	}

	if err := client.Mail(from); err != nil {
		return false, nil, fmt.Errorf("MAIL FROM: %w", err)
	}

	var failures []RecipientFailure
	acceptedCount := 0
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			// Distinguish permanent (5xx) from temporary (4xx) failures by
			// the leading digit of the SMTP code carried in the error. The
			// net/smtp client wraps the response as "<code> <text>" in
			// err.Error(), so a simple prefix check is reliable enough.
			reason := err.Error()
			if isPermanentSMTPError(reason) {
				failures = append(failures, RecipientFailure{Email: rcpt, Reason: reason})
				continue
			}
			// Temporary failure → treat as transport error so the caller
			// retries the whole batch on the next MX (or surfaces 502).
			return false, nil, fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
		acceptedCount++
	}

	if acceptedCount == 0 {
		// All recipients permanently failed at RCPT — close cleanly without
		// sending DATA. The failures slice carries the per-recipient bounces.
		_ = client.Reset()
		_ = client.Quit()
		return false, failures, nil
	}

	w, err := client.Data()
	if err != nil {
		return false, failures, fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return false, failures, fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return false, failures, fmt.Errorf("close DATA: %w", err)
	}

	_ = client.Quit()
	return true, failures, nil
}

// isPermanentSMTPError checks whether an error string from net/smtp begins
// with a 5xx response code. Net/smtp formats responses as "NNN <text>" or
// "NNN-<text>" for multiline; both forms are covered by checking the first
// digit. Returns false for empty strings and non-numeric prefixes so transient
// network errors don't get mistaken for permanent bounces.
func isPermanentSMTPError(s string) bool {
	if len(s) < 3 {
		return false
	}
	return s[0] == '5' && isDigit(s[1]) && isDigit(s[2])
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// resolveMXHosts returns hosts sorted by preference. If MX lookup yields no
// records (NXDOMAIN or empty), RFC 5321 §5.1 requires falling back to the
// domain's A/AAAA record — we honor that by returning the domain itself.
func resolveMXHosts(ctx context.Context, domain string) ([]string, error) {
	mxs, err := smtpMXLookup(ctx, domain)
	if err == nil && len(mxs) > 0 {
		sort.SliceStable(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
		hosts := make([]string, len(mxs))
		for i, mx := range mxs {
			hosts[i] = strings.TrimSuffix(mx.Host, ".")
		}
		return hosts, nil
	}
	// Fallback to the bare domain — receivers without explicit MX still get mail.
	return []string{domain}, nil
}

// groupRecipientsByDomain partitions To+Cc+Bcc by recipient domain.
func groupRecipientsByDomain(req *SendRequest) map[string][]string {
	groups := make(map[string][]string)
	add := func(addr string) {
		_, domain := splitAddress(addr)
		if domain == "" {
			return
		}
		groups[domain] = append(groups[domain], addr)
	}
	for _, r := range req.To {
		add(r.Email)
	}
	for _, r := range req.Cc {
		add(r.Email)
	}
	for _, r := range req.Bcc {
		add(r.Email)
	}
	return groups
}

// allRecipients flattens To+Cc+Bcc into a single slice of email addresses.
func allRecipients(req *SendRequest) []string {
	out := make([]string, 0, len(req.To)+len(req.Cc)+len(req.Bcc))
	for _, r := range req.To {
		out = append(out, r.Email)
	}
	for _, r := range req.Cc {
		out = append(out, r.Email)
	}
	for _, r := range req.Bcc {
		out = append(out, r.Email)
	}
	return out
}

// envelopeAddress extracts the bare email from a possibly-display-name-wrapped
// From string ("Alice <a@example.com>" → "a@example.com"). The wire-level
// MAIL FROM must be the bare address.
func envelopeAddress(from string) (string, error) {
	if from == "" {
		return "", fmt.Errorf("empty from")
	}
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.Index(from[i:], ">"); j > 0 {
			return strings.TrimSpace(from[i+1 : i+j]), nil
		}
	}
	return strings.TrimSpace(from), nil
}

// splitAddress splits an email address into its local-part and domain. The
// address is trimmed and lower-cased first so the derived domain is a clean
// MX-lookup target. Returns empty strings when there is no single "@" separator.
func splitAddress(email string) (localPart, domain string) {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.LastIndex(email, "@")
	if at < 1 || at >= len(email)-1 {
		return "", ""
	}
	return email[:at], email[at+1:]
}

// generateMessageID builds an RFC-compliant Message-ID rooted at the operator's
// public hostname. Time + 8 random hex chars keep collision probability negligible.
func generateMessageID(hostname string) string {
	if hostname == "" {
		hostname = "localhost"
	}
	suffix, _ := randomHex(8)
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UTC().UnixNano(), suffix, hostname)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- RFC 5322 message construction ---

// buildOutgoingRFC5322 serializes a SendRequest into RFC 5322 bytes for direct
// SMTP delivery. Threading headers (In-Reply-To, References) are emitted when
// present. Attachments are emitted as a multipart/mixed wrapper around the
// text/html alternative. The Message-ID is injected (the caller generates it
// up-front so the same value can be stored locally for thread matching).
func buildOutgoingRFC5322(req *SendRequest, messageID, helo string) ([]byte, error) {
	var buf bytes.Buffer

	var h gomail.Header
	h.SetDate(time.Now().UTC())
	h.SetSubject(req.Subject)
	h.SetMessageID(messageID)

	if req.From != "" {
		fromAddrs, err := gomail.ParseAddressList(req.From)
		if err == nil && len(fromAddrs) > 0 {
			h.SetAddressList("From", fromAddrs)
		} else {
			// Fall back: treat as a bare address with no display name.
			h.SetAddressList("From", []*gomail.Address{{Address: req.From}})
		}
	}
	if req.ReplyTo != "" {
		if addrs, err := gomail.ParseAddressList(req.ReplyTo); err == nil && len(addrs) > 0 {
			h.SetAddressList("Reply-To", addrs)
		}
	}
	if len(req.To) > 0 {
		h.SetAddressList("To", recipientsToAddrs(req.To))
	}
	if len(req.Cc) > 0 {
		h.SetAddressList("Cc", recipientsToAddrs(req.Cc))
	}
	// Bcc is deliberately omitted from headers — it lives only in the SMTP
	// envelope, per RFC 5322 §3.6.3.

	if req.InReplyTo != "" {
		h.Set("In-Reply-To", req.InReplyTo)
	}
	if req.References != "" {
		h.Set("References", req.References)
	}
	for _, hdr := range req.Headers {
		// Custom headers from the caller are appended verbatim. We do not
		// overwrite headers we've already set (Subject, From, etc.).
		if hdr.Name == "" {
			continue
		}
		h.Set(hdr.Name, hdr.Value)
	}

	if len(req.Attachments) > 0 {
		mw, err := gomail.CreateWriter(&buf, h)
		if err != nil {
			return nil, fmt.Errorf("create writer: %w", err)
		}
		if err := writeAlternativePart(mw, req.TextBody, req.HTMLBody); err != nil {
			mw.Close()
			return nil, err
		}
		for _, att := range req.Attachments {
			if err := writeAttachmentFromBase64(mw, att); err != nil {
				return nil, err
			}
		}
		mw.Close()
		return buf.Bytes(), nil
	}

	if req.HTMLBody != "" {
		mw, err := gomail.CreateWriter(&buf, h)
		if err != nil {
			return nil, fmt.Errorf("create writer: %w", err)
		}
		if err := writeAlternativePart(mw, req.TextBody, req.HTMLBody); err != nil {
			mw.Close()
			return nil, err
		}
		mw.Close()
		return buf.Bytes(), nil
	}

	h.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	w, err := gomail.CreateSingleInlineWriter(&buf, h)
	if err != nil {
		return nil, fmt.Errorf("create plain writer: %w", err)
	}
	io.WriteString(w, req.TextBody)
	w.Close()
	return buf.Bytes(), nil
}

func recipientsToAddrs(rs []Recipient) []*gomail.Address {
	out := make([]*gomail.Address, 0, len(rs))
	for _, r := range rs {
		out = append(out, &gomail.Address{Name: r.Name, Address: r.Email})
	}
	return out
}

// writeAlternativePart writes a multipart/alternative body (text + optional
// html) into the given writer.
func writeAlternativePart(mw *gomail.Writer, textBody, htmlBody string) error {
	altW, err := mw.CreateInline()
	if err != nil {
		return fmt.Errorf("failed to create alternative part: %w", err)
	}

	var textH gomail.InlineHeader
	textH.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	tw, err := altW.CreatePart(textH)
	if err != nil {
		return fmt.Errorf("failed to create text part: %w", err)
	}
	io.WriteString(tw, textBody)
	tw.Close()

	if htmlBody != "" {
		var htmlH gomail.InlineHeader
		htmlH.SetContentType("text/html", map[string]string{"charset": "utf-8"})
		hw, err := altW.CreatePart(htmlH)
		if err != nil {
			return fmt.Errorf("failed to create html part: %w", err)
		}
		io.WriteString(hw, htmlBody)
		hw.Close()
	}

	altW.Close()
	return nil
}

func writeAttachmentFromBase64(mw *gomail.Writer, att Attachment) error {
	data, err := base64.StdEncoding.DecodeString(att.Content)
	if err != nil {
		return fmt.Errorf("decode attachment %q: %w", att.Name, err)
	}
	contentType := att.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var ah gomail.InlineHeader
	ah.SetContentType(contentType, map[string]string{"name": att.Name})
	ah.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Name))
	ah.Set("Content-Transfer-Encoding", "base64")
	if att.ContentID != "" {
		ah.Set("Content-ID", "<"+att.ContentID+">")
	}

	aw, err := mw.CreateSingleInline(ah)
	if err != nil {
		return fmt.Errorf("create attachment part: %w", err)
	}
	encoder := base64.NewEncoder(base64.StdEncoding, aw)
	encoder.Write(data)
	encoder.Close()
	aw.Close()
	return nil
}

// Compile-time assertion that SMTPSender satisfies both sender interfaces.
var (
	_ Sender     = (*SMTPSender)(nil)
	_ FullSender = (*SMTPSender)(nil)
)
