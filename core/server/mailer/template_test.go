package mailer

import (
	"strings"
	"testing"
)

func TestRenderTransactionalEmail_EscapesAndLinks(t *testing.T) {
	html, text := RenderTransactionalEmail(TransactionalEmail{
		Eyebrow:  "Password reset · Acme",
		Greeting: Greeting("Alice"),
		BodyHTML: "Reset your <strong>password</strong>.",
		BodyText: "Reset your password.",
		CTALabel: "Reset password",
		CTALink:  "https://app.example.com/reset-password/abc",
	})

	if !strings.Contains(html, "https://app.example.com/reset-password/abc") {
		t.Errorf("html missing CTA link: %q", html)
	}
	// BodyHTML is inserted verbatim (caller-escaped), so <strong> survives.
	if !strings.Contains(html, "<strong>password</strong>") {
		t.Errorf("html should pass BodyHTML through verbatim, got %q", html)
	}
	// The eyebrow is escaped by the renderer; the dot separator is plain text.
	if !strings.Contains(html, "Password reset · Acme") {
		t.Errorf("html missing eyebrow, got %q", html)
	}
	if !strings.Contains(text, "Hi Alice,") {
		t.Errorf("text missing greeting, got %q", text)
	}
	if !strings.Contains(text, "https://app.example.com/reset-password/abc") {
		t.Errorf("text missing link, got %q", text)
	}
}

func TestRenderTransactionalEmail_EscapesEyebrowAndCTALabel(t *testing.T) {
	html, _ := RenderTransactionalEmail(TransactionalEmail{
		Eyebrow:  "<x>",
		Greeting: Greeting(""),
		CTALabel: "<y>",
		CTALink:  "https://e/x",
	})
	if strings.Contains(html, "<x>") || strings.Contains(html, "<y>") {
		t.Errorf("eyebrow/CTALabel must be HTML-escaped, got %q", html)
	}
}

func TestGreeting(t *testing.T) {
	if got := Greeting("  Bob  "); got != "Hi Bob" {
		t.Errorf("Greeting trims and prefixes; got %q", got)
	}
	if got := Greeting(""); got != "Hi" {
		t.Errorf("empty name → bare 'Hi'; got %q", got)
	}
}
