package mailer

import (
	"testing"
)

// withConfig swaps ConfigResolver to read from a static map for the duration of
// a test, and forces devAutoLog off so delivery decisions reflect config alone.
func withConfig(t *testing.T, values map[string]string) {
	t.Helper()
	origResolver := ConfigResolver
	origDev := devAutoLog
	ConfigResolver = func(key string) string { return values[key] }
	devAutoLog = false
	t.Cleanup(func() {
		ConfigResolver = origResolver
		devAutoLog = origDev
	})
}

func TestDefaultSender_LogsWhenUnconfigured(t *testing.T) {
	withConfig(t, map[string]string{})
	if _, ok := DefaultSender().(*LogSender); !ok {
		t.Errorf("expected LogSender when no provider configured, got %T", DefaultSender())
	}
	if CanDeliver() {
		t.Errorf("CanDeliver should be false when no provider configured")
	}
}

func TestDefaultSender_PostmarkWhenTokenSet(t *testing.T) {
	withConfig(t, map[string]string{
		keyPostmarkToken: "tok-123",
		keyFromAddress:   "hello@example.com",
	})
	s, ok := DefaultSender().(*PostmarkSender)
	if !ok {
		t.Fatalf("expected PostmarkSender, got %T", DefaultSender())
	}
	if s.defaultFrom != "hello@example.com" {
		t.Errorf("from: got %q, want hello@example.com", s.defaultFrom)
	}
	if !CanDeliver() {
		t.Errorf("CanDeliver should be true with a token set")
	}
}

func TestDefaultSender_SMTPWhenProviderSMTP(t *testing.T) {
	withConfig(t, map[string]string{
		keyProvider:       "smtp",
		keySMTPPublicHost: "mx.example.com",
	})
	if _, ok := DefaultSender().(*SMTPSender); !ok {
		t.Errorf("expected SMTPSender when provider=smtp, got %T", DefaultSender())
	}
	if !CanDeliver() {
		t.Errorf("CanDeliver should be true for smtp provider")
	}
}

func TestDefaultSender_DeliveryDisabledLogs(t *testing.T) {
	withConfig(t, map[string]string{
		keyPostmarkToken:   "tok-123",
		keyDeliveryEnabled: "false",
	})
	if _, ok := DefaultSender().(*LogSender); !ok {
		t.Errorf("expected LogSender when delivery disabled, got %T", DefaultSender())
	}
	if CanDeliver() {
		t.Errorf("CanDeliver should be false when delivery disabled")
	}
}

func TestDefaultSender_DevAutoLog(t *testing.T) {
	withConfig(t, map[string]string{keyPostmarkToken: "tok-123"})
	devAutoLog = true // simulate a --dev process
	if _, ok := DefaultSender().(*LogSender); !ok {
		t.Errorf("expected LogSender in --dev process, got %T", DefaultSender())
	}
}

func TestFromAddress_DefaultsWhenUnset(t *testing.T) {
	withConfig(t, map[string]string{})
	if got := fromAddress(); got != defaultFromAddress {
		t.Errorf("fromAddress: got %q, want %q", got, defaultFromAddress)
	}
}
