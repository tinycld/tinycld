// Package webforms serves published web forms to visitors with no session:
// the form definition, a proof-of-work challenge and the submit endpoint.
// See docs/superpowers/specs/2026-11-02-web-forms-design.md.
package webforms

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

const (
	stampLabel = "tinycld:webforms:stamp:v1"
	powLabel   = "tinycld:webforms:pow:v1"
	ipLabel    = "tinycld:webforms:ip:v1"
)

// subkey derives a domain-separated key from the superusers auth-token secret,
// as sharelink and oauth do; the label's :v1 suffix is the rotation seam.
func subkey(app core.App, label string) ([]byte, error) {
	col, err := app.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return nil, fmt.Errorf("webforms: load superusers collection: %w", err)
	}
	base := col.AuthToken.Secret
	if base == "" {
		return nil, errors.New("webforms: superusers auth token secret is empty")
	}
	mac := hmac.New(sha256.New, []byte(base))
	mac.Write([]byte(label))
	return mac.Sum(nil), nil
}

func sign(key []byte, s string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(s))
	return hex.EncodeToString(mac.Sum(nil))
}

// hashIP stores a keyed hash, never the address: enough to see one source
// flooding a form, not enough to identify a visitor.
func hashIP(app core.App, ip string) (string, error) {
	key, err := subkey(app, ipLabel)
	if err != nil {
		return "", err
	}
	return sign(key, ip), nil
}
