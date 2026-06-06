package sharelink

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"

	"github.com/pocketbase/pocketbase/tools/security"
)

// anonIDAlphabet is the alphabet for minted anon ids — URL/cookie safe.
const anonIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// anonIDLen is the random portion length of a minted anon id.
const anonIDLen = 22

// NewAnonID mints a fresh stable anonymous id. The client persists this
// in a long-lived cookie so the same visitor keeps the same identity
// across visits and across links. Prefixed so it's recognizable in logs.
func NewAnonID() string {
	return "anon_" + security.RandomStringWithAlphabet(anonIDLen, anonIDAlphabet)
}

// DisplayName derives a stable "Anon <Animal>" name from an anon id.
// Deterministic (no storage) so the same anon id always renders the same
// name everywhere — preview, comments, and editor presence. Uses a hash
// of the id to pick an adjective + animal, giving enough combinations to
// keep collisions rare within a single document's set of visitors.
func DisplayName(anonID string) string {
	sum := sha256.Sum256([]byte("tinycld:anon-name:" + anonID))
	adjIdx := binary.BigEndian.Uint32(sum[0:4]) % uint32(len(anonAdjectives))
	animIdx := binary.BigEndian.Uint32(sum[4:8]) % uint32(len(anonAnimals))
	return "Anon " + anonAdjectives[adjIdx] + " " + anonAnimals[animIdx]
}

// IsValidAnonID does a cheap shape check on a client-supplied anon id
// before we trust it as a returning identity. We don't store anon ids,
// so this is just defense against absurd input, not authentication.
func IsValidAnonID(id string) bool {
	if len(id) != len("anon_")+anonIDLen {
		return false
	}
	if id[:5] != "anon_" {
		return false
	}
	for _, r := range id[5:] {
		if !isAlnum(byte(r)) {
			return false
		}
	}
	return true
}

func isAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// ensure crypto/rand is linked even if security swaps its source; the
// import documents that anon ids are unguessable.
var _ = rand.Reader

var anonAdjectives = []string{
	"Brave", "Calm", "Clever", "Cosmic", "Curious", "Eager", "Gentle",
	"Happy", "Jolly", "Keen", "Lively", "Lucky", "Mellow", "Nimble",
	"Plucky", "Quiet", "Rapid", "Sandy", "Sunny", "Swift", "Witty", "Zesty",
}

var anonAnimals = []string{
	"Otter", "Falcon", "Badger", "Heron", "Lynx", "Marmot", "Narwhal",
	"Ocelot", "Panda", "Quokka", "Raccoon", "Salmon", "Tapir", "Urchin",
	"Vole", "Walrus", "Yak", "Zebra", "Beaver", "Cheetah", "Dolphin", "Egret",
}
