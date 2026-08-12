package guestauth

import (
	"strings"
	"testing"
)

// ParseEmail and the code alphabet are pure; the account and OTP helpers need a
// live app and are exercised end to end by the packages that use them
// (cards/server/endpoints_share_otp_test.go drives the whole flow).
//
// What is worth pinning here is the part a reader is most likely to get wrong
// when touching this file: WHICH rejection burns the code and which does not.

func TestParseEmailTrimsAndValidates(t *testing.T) {
	got, err := ParseEmail("  someone@test.local  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "someone@test.local" {
		t.Fatalf("ParseEmail = %q, want the trimmed address", got)
	}
}

func TestParseEmailAcceptsADisplayNameForm(t *testing.T) {
	// net/mail accepts "Name <addr>"; the address is what must come back, since
	// it is what the OTP is keyed on.
	got, err := ParseEmail("Ada <ada@test.local>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ada@test.local" {
		t.Fatalf("ParseEmail = %q, want the bare address", got)
	}
}

func TestParseEmailRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "   ", "not-an-address", "@nope", "a@"} {
		if _, err := ParseEmail(raw); err == nil {
			t.Fatalf("ParseEmail(%q) was accepted", raw)
		}
	}
}

func TestNewCodeIsSixDigits(t *testing.T) {
	// The length is not cosmetic: it is the keyspace the rate limiter's
	// arithmetic is sized against.
	code := NewCode()
	if len(code) != CodeLength {
		t.Fatalf("code %q has length %d, want %d", code, len(code), CodeLength)
	}
	if strings.Trim(code, "0123456789") != "" {
		t.Fatalf("code %q contains a non-digit", code)
	}
}

func TestNewCodeVaries(t *testing.T) {
	// A constant code would satisfy every other assertion in this file.
	seen := map[string]bool{}
	for range 50 {
		seen[NewCode()] = true
	}
	if len(seen) < 2 {
		t.Fatal("NewCode returned the same value 50 times")
	}
}
