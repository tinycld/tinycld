package textpreview

import (
	"crypto/sha256"
	"encoding/hex"
)

// FirstRegionText returns the leading region of plaintext (up to 3600 runes,
// rune-aware so multibyte characters are never split) along with its hash.
func FirstRegionText(plaintext string) (region, hash string) {
	runes := []rune(plaintext)
	if len(runes) > 3600 {
		runes = runes[:3600]
	}
	region = string(runes)
	hash = Hash(region)
	return region, hash
}

// Hash returns the hex-encoded SHA-256 of s.
func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
