package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// MethodS256 is the only code_challenge_method we accept. OAuth 2.1 removes
// `plain`, and supporting it would defeat the purpose: an attacker who
// intercepts the authorization request could replay the challenge verbatim.
const MethodS256 = "S256"

// VerifyPKCE reports whether verifier hashes to challenge under S256:
// BASE64URL-ENCODE(SHA256(ASCII(verifier))) == challenge, unpadded.
//
// Empty inputs always fail — a stripped PKCE parameter must never read as a
// successful verification.
func VerifyPKCE(challenge, verifier string) bool {
	if challenge == "" || verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}
