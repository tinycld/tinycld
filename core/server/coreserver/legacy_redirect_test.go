package coreserver

import "testing"

func TestLegacyAppRedirect(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		// The motivating case: links minted before the move are still in inboxes.
		{"emailed invite", "accept-invite/tok-123", "/a/accept-invite/tok-123"},
		{"emailed reset", "reset-password/tok-123", "/a/reset-password/tok-123"},
		{"bookmarked settings", "settings/personal", "/a/settings/personal"},
		{"bare pre-auth route", "connect", "/a/connect"},

		// Already migrated — must not double-prefix.
		{"already prefixed", "a/mail", ""},
		{"bare prefix", "a", ""},

		// Not app routes. Rewriting any of these would break them.
		{"public share", "p/drive/share/tok", ""},
		{"public demo", "p/demo", ""},
		{"api", "api/health", ""},
		{"webdav", "dav/drive/", ""},
		{"caldav", "caldav/u/cal/", ""},
		{"asset miss", "favicon.ico", ""},
		{"well-known", ".well-known/caldav", ""},
		{"empty", "", ""},

		// A package route that merely starts with an allowlisted substring must
		// not match — the check is per-segment, not a prefix match.
		{"segment prefix only", "settingsomething", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := legacyAppRedirect(c.path); got != c.want {
				t.Errorf("legacyAppRedirect(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}
