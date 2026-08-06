package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// challengeFor builds the S256 challenge for a verifier the way a conforming
// client does: BASE64URL(SHA256(ASCII(verifier))), no padding.
func challengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestVerifyPKCEAcceptsMatchingVerifier(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	if !VerifyPKCE(challengeFor(verifier), verifier) {
		t.Fatal("a correct verifier must validate")
	}
}

func TestVerifyPKCERejectsWrongVerifier(t *testing.T) {
	challenge := challengeFor("the-real-verifier")
	if VerifyPKCE(challenge, "an-attackers-guess") {
		t.Fatal("a wrong verifier must not validate — this is the whole point of PKCE")
	}
}

func TestVerifyPKCERejectsEmptyInput(t *testing.T) {
	// Empty inputs must never validate. Note this holds because the hash
	// comparison below cannot match an empty challenge — SHA256("") encodes to
	// a 43-char digest — not because of the explicit guard in VerifyPKCE. The
	// guard is defense-in-depth; this test would pass without it.
	if VerifyPKCE("", "") {
		t.Fatal("empty challenge+verifier must not validate")
	}
	if VerifyPKCE(challengeFor("x"), "") {
		t.Fatal("empty verifier must not validate")
	}
	if VerifyPKCE("", "x") {
		t.Fatal("empty challenge must not validate")
	}
}

func TestVerifyPKCERejectsPlainMethod(t *testing.T) {
	// OAuth 2.1 removes the `plain` method. Passing the verifier as its own
	// challenge must fail.
	verifier := "some-verifier-value"
	if VerifyPKCE(verifier, verifier) {
		t.Fatal("plain-style challenge (verifier == challenge) must not validate")
	}
}
