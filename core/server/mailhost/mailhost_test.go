package mailhost

import (
	"testing"

	"tinycld.org/core/syscfg"
)

func configure(t *testing.T, value string) {
	t.Helper()
	syscfg.ResetForTesting()
	t.Cleanup(syscfg.ResetForTesting)
	syscfg.SetResolver(func(key string) string {
		if key == Key {
			return value
		}
		return ""
	})
}

func TestConfiguredNormalizesTheSetting(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"mail.example.com", "mail.example.com"},
		{"  Mail.Example.COM.  ", "mail.example.com"},
		{"https://Mail.Example.com:443/", "mail.example.com"},
		{"mail.example.com:993", "mail.example.com"},
		{"localhost", "localhost"},
		{"mail example.com", ""},
		{"/x", ""},
		{"mail..example.com", ""},
		{"-mail.example.com", ""},
		{"[::1]:8090", ""},
	} {
		configure(t, tc.in)
		if got := Configured(); got != tc.want {
			t.Errorf("Configured() with %q = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHostPrefersTheSetting(t *testing.T) {
	configure(t, "mail.example.com")
	if got := Host("https://web.example.com"); got != "mail.example.com" {
		t.Fatalf("Host = %q, want mail.example.com", got)
	}
}

func TestHostFallsBackToTheWebHost(t *testing.T) {
	configure(t, "")
	for _, tc := range []struct{ in, want string }{
		{"https://Web.Example.com", "web.example.com"},
		{"http://localhost:8090", "localhost"},
		{"web.example.com:443", "web.example.com"},
		{"", ""},
		{"not a host", ""},
	} {
		if got := Host(tc.in); got != tc.want {
			t.Errorf("Host(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHostIgnoresAnInvalidSetting(t *testing.T) {
	configure(t, "mail example.com")
	if got := Host("https://web.example.com"); got != "web.example.com" {
		t.Fatalf("Host = %q, want web.example.com", got)
	}
}
