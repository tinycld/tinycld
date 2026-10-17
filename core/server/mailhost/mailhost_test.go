package mailhost

import (
	"context"
	"log/slog"
	"sync"
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

type recordSink struct {
	mu      sync.Mutex
	records []slog.Record
}

func (s *recordSink) Enabled(context.Context, slog.Level) bool { return true }
func (s *recordSink) Handle(_ context.Context, r slog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
	return nil
}
func (s *recordSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *recordSink) WithGroup(string) slog.Handler      { return s }

func (s *recordSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

// Tests that use this must not run in parallel: they swap the global slog.Default.
func captureWarnings(t *testing.T) *recordSink {
	t.Helper()
	sink := &recordSink{}
	prev := slog.Default()
	slog.SetDefault(slog.New(sink))
	resetWarned()
	t.Cleanup(func() {
		slog.SetDefault(prev)
		resetWarned()
	})
	return sink
}

func TestConfiguredWarnsOncePerInvalidValue(t *testing.T) {
	sink := captureWarnings(t)

	configure(t, "mail.example.com/")
	Configured()
	if got := sink.count(); got != 1 {
		t.Fatalf("invalid value: %d warnings, want 1", got)
	}
	Configured()
	if got := sink.count(); got != 1 {
		t.Fatalf("same invalid value again: %d warnings, want 1", got)
	}

	configure(t, "mail example.com")
	Configured()
	if got := sink.count(); got != 2 {
		t.Fatalf("different invalid value: %d warnings, want 2", got)
	}

	sink.mu.Lock()
	rec := sink.records[0]
	sink.mu.Unlock()
	if rec.Level != slog.LevelWarn || rec.Message != "ignoring a mail host setting that is not a host name; mail apps get the web host" {
		t.Errorf("unexpected record: %v %q", rec.Level, rec.Message)
	}
	attrs := map[string]string{}
	rec.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.String(); return true })
	if attrs["key"] != Key || attrs["value"] != "mail.example.com/" {
		t.Errorf("unexpected attrs: %v", attrs)
	}
}

func TestConfiguredNeverWarnsForValidOrBlank(t *testing.T) {
	sink := captureWarnings(t)
	for _, v := range []string{"", "   ", "mail.example.com", "https://mail.example.com:443/"} {
		configure(t, v)
		Configured()
		Host("https://app.example.com")
	}
	if got := sink.count(); got != 0 {
		t.Fatalf("%d warnings, want 0", got)
	}
}
