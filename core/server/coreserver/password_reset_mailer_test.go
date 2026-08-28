package coreserver

import (
	"strings"
	"testing"
)

func TestBuildPasswordResetMessage_LinkPointsAtAppResetScreen(t *testing.T) {
	subject, html, text := buildPasswordResetMessage(
		"https://app.example.com/",
		"Example",
		"Alice",
		"tok-123",
	)

	wantLink := "https://app.example.com/a/reset-password/tok-123"

	if subject != "Reset your password" {
		t.Errorf("subject = %q, want %q", subject, "Reset your password")
	}
	// Trailing slash on AppURL must be trimmed (no double slash before /reset-password).
	if strings.Contains(html, "//a/reset-password") {
		t.Errorf("html link has double slash: %q", html)
	}
	if !strings.Contains(html, wantLink) {
		t.Errorf("html missing reset link %q", wantLink)
	}
	if !strings.Contains(text, wantLink) {
		t.Errorf("text missing reset link %q", wantLink)
	}
	// Must NOT link at PocketBase's admin confirm UI.
	if strings.Contains(html, "/_/#/auth/confirm-password-reset") {
		t.Errorf("html links at PB admin UI instead of app screen: %q", html)
	}
	if !strings.Contains(html, "Hi Alice") {
		t.Errorf("html missing personalized greeting, got %q", html)
	}
	// The eyebrow names the app.
	if !strings.Contains(html, "Password reset · Example") {
		t.Errorf("html missing app-named eyebrow, got %q", html)
	}
}

func TestBuildPasswordResetMessage_EmptyNameUsesGenericGreeting(t *testing.T) {
	_, html, _ := buildPasswordResetMessage("https://app.example.com", "Example", "", "tok-9")
	if !strings.Contains(html, "Hi,") {
		t.Errorf("expected generic 'Hi,' greeting when name empty, got %q", html)
	}
}

func TestBuildPasswordResetMessage_EmptyAppNameDropsSeparator(t *testing.T) {
	_, html, _ := buildPasswordResetMessage("https://app.example.com", "", "Bob", "tok-1")
	// With no app name the eyebrow is the bare "Password reset" (no trailing " · ").
	if strings.Contains(html, "Password reset ·") {
		t.Errorf("expected bare 'Password reset' eyebrow when AppName empty, got %q", html)
	}
	if !strings.Contains(html, "Password reset") {
		t.Errorf("expected 'Password reset' eyebrow, got %q", html)
	}
}
