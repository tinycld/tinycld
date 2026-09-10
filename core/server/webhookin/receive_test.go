package webhookin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"tinycld.org/core/ratelimit"
)

func TestVerifySignature_AcceptsAMatchingDigest(t *testing.T) {
	secret := "s3cret"
	body := []byte(`{"hello":"world"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifySignature(secret, body, header) {
		t.Error("a correct signature was rejected")
	}
}

func TestVerifySignature_RejectsTampering(t *testing.T) {
	secret := "s3cret"
	body := []byte(`{"hello":"world"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if verifySignature(secret, []byte(`{"hello":"tampered"}`), header) {
		t.Error("a body that did not match its signature was accepted")
	}
	if verifySignature("wrong-secret", body, header) {
		t.Error("a signature under the wrong secret was accepted")
	}
}

func TestVerifySignature_RejectsMalformedHeaders(t *testing.T) {
	body := []byte(`{}`)
	for _, header := range []string{
		"",                  // absent
		"deadbeef",          // no algorithm prefix
		"sha1=deadbeef",     // wrong algorithm
		"sha256=",           // empty digest
		"sha256=not-hex-!!", // undecodable
	} {
		if verifySignature("s3cret", body, header) {
			t.Errorf("malformed header %q was accepted", header)
		}
	}
}

func TestVerifySignature_RejectsWhenNoSecretIsConfigured(t *testing.T) {
	// An unconfigured source must not fall open to "no secret, no check".
	body := []byte(`{}`)
	mac := hmac.New(sha256.New, []byte(""))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if verifySignature("", body, header) {
		t.Error("a blank secret was treated as valid configuration")
	}
}

func TestDeliveryLimiter_MetersPerSource(t *testing.T) {
	limiter := ratelimit.New(2, time.Minute)

	if !limiter.Allow("acme") || !limiter.Allow("acme") {
		t.Fatal("the first two deliveries were rejected")
	}
	if limiter.Allow("acme") {
		t.Error("a third delivery slipped past the ceiling")
	}
	if !limiter.Allow("widgets") {
		t.Error("one source's burst consumed another's budget")
	}
}
