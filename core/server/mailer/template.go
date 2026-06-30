package mailer

import (
	"fmt"
	htmltpl "html"
	"strings"
)

// BrandColor is the accent color used across transactional emails (CTA button,
// top border, link color). Kept here so every email rendered through
// RenderTransactionalEmail shares one source of truth.
const BrandColor = "#0d9488"

// TransactionalEmail describes a single transactional message in the shared
// house style: an uppercase eyebrow, a greeting, a body, a primary call-to-action
// button (with a copy-paste link fallback), and a muted footer note.
//
// BodyHTML is inserted verbatim into the HTML body, so callers that interpolate
// user-controlled values into it MUST escape them first (use EscapeHTML). BodyText
// is the plain-text equivalent for the text/plain MIME part. Eyebrow, Greeting,
// CTALabel, and Footer are escaped by the renderer.
type TransactionalEmail struct {
	Eyebrow  string // uppercase label above the greeting, e.g. "Password reset · Acme"
	Greeting string // e.g. "Hi Alice" (no trailing comma — the template adds it)
	BodyHTML string // HTML body paragraph(s); caller pre-escapes interpolated values
	BodyText string // plain-text body for the text/plain part
	CTALabel string // button text, e.g. "Reset password"
	CTALink  string // button + copy-link target URL
	Footer   string // muted note at the bottom
}

// EscapeHTML escapes a string for safe interpolation into an email's BodyHTML.
func EscapeHTML(s string) string {
	return htmltpl.EscapeString(s)
}

// Greeting builds a "Hi <name>" greeting, falling back to a bare "Hi" when the
// name is empty. Shared so every transactional email greets consistently.
func Greeting(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Hi"
	}
	return "Hi " + name
}

// RenderTransactionalEmail renders the shared house-style email and returns the
// HTML and plain-text bodies. The two share the same CTA link; the text body is
// the caller-provided BodyText followed by the link.
func RenderTransactionalEmail(d TransactionalEmail) (html, text string) {
	html = fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>%s</title>
</head>
<body style="margin:0;padding:0;background:#f5f5f4;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#1c1917;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="background:#f5f5f4;padding:40px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="560" cellpadding="0" cellspacing="0" border="0" style="max-width:560px;width:100%%;background:#ffffff;border-radius:12px;box-shadow:0 1px 3px rgba(0,0,0,0.08);overflow:hidden;">
          <tr>
            <td style="padding:32px 40px 16px 40px;border-top:4px solid %s;">
              <p style="margin:0 0 8px 0;font-size:14px;color:#78716c;letter-spacing:0.5px;text-transform:uppercase;font-weight:600;">%s</p>
              <h1 style="margin:0;font-size:24px;line-height:1.3;font-weight:600;color:#1c1917;">%s,</h1>
            </td>
          </tr>
          <tr>
            <td style="padding:8px 40px 8px 40px;">
              <p style="margin:0;font-size:16px;line-height:1.6;color:#44403c;">%s</p>
            </td>
          </tr>
          <tr>
            <td style="padding:24px 40px 8px 40px;">
              <a href="%s" style="display:inline-block;padding:12px 24px;background:%s;color:#ffffff;text-decoration:none;border-radius:8px;font-weight:600;font-size:15px;">%s</a>
            </td>
          </tr>
          <tr>
            <td style="padding:8px 40px 32px 40px;">
              <p style="margin:16px 0 4px 0;font-size:13px;color:#78716c;">Or copy this link into your browser:</p>
              <p style="margin:0;font-size:13px;color:#44403c;word-break:break-all;"><a href="%s" style="color:%s;text-decoration:none;">%s</a></p>
            </td>
          </tr>
          <tr>
            <td style="padding:20px 40px;border-top:1px solid #e7e5e4;background:#fafaf9;">
              <p style="margin:0;font-size:12px;line-height:1.6;color:#78716c;">%s</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`,
		htmltpl.EscapeString(d.Eyebrow),
		BrandColor,
		htmltpl.EscapeString(d.Eyebrow),
		htmltpl.EscapeString(d.Greeting),
		d.BodyHTML,
		htmltpl.EscapeString(d.CTALink),
		BrandColor,
		htmltpl.EscapeString(d.CTALabel),
		htmltpl.EscapeString(d.CTALink),
		BrandColor,
		htmltpl.EscapeString(d.CTALink),
		htmltpl.EscapeString(d.Footer),
	)

	text = fmt.Sprintf("%s,\n\n%s\n\n%s\n", d.Greeting, d.BodyText, d.CTALink)
	return html, text
}
