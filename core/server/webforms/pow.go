package webforms

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	powMaxNumber = 50000
	powTTL       = 10 * time.Minute
	powAlgorithm = "SHA-256"
)

var errPow = errors.New("invalid proof of work")

// Challenge is the ALTCHA-style puzzle: find n in [0, MaxNumber] with
// sha256(Salt + n) == Challenge. The server signs Challenge so it stores
// nothing until a solution comes back.
type Challenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	MaxNumber int    `json:"maxnumber"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

type Solution struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Number    int    `json:"number"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func newChallenge(key []byte, pubID string, now time.Time) (Challenge, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(powMaxNumber+1))
	if err != nil {
		return Challenge{}, err
	}
	q := url.Values{"pub": {pubID}, "expires": {strconv.FormatInt(now.Add(powTTL).Unix(), 10)}}
	salt := security.RandomString(16) + "?" + q.Encode()
	ch := sha256Hex(salt + strconv.FormatInt(n.Int64(), 10))
	return Challenge{Algorithm: powAlgorithm, Challenge: ch, MaxNumber: powMaxNumber, Salt: salt, Signature: sign(key, ch)}, nil
}

// powLedger remembers spent signatures until they expire, so one solution
// cannot be replayed. verify reserves a signature; a submit refused after
// that releases it, so the visitor can fix the form and send the same
// solution again. Process-local like ratelimit: a second instance would
// accept a replay once, which the rate limit still bounds.
type powLedger struct {
	mu   sync.Mutex
	used map[string]time.Time
}

func newPowLedger() *powLedger { return &powLedger{used: map[string]time.Time{}} }

func (l *powLedger) verify(key []byte, s Solution, pubID string, now time.Time) error {
	if s.Algorithm != powAlgorithm || s.Number < 0 || s.Number > powMaxNumber {
		return errPow
	}
	if !hmac.Equal([]byte(s.Signature), []byte(sign(key, s.Challenge))) {
		return errPow
	}
	if sha256Hex(s.Salt+strconv.Itoa(s.Number)) != s.Challenge {
		return errPow
	}
	i := strings.IndexByte(s.Salt, '?')
	if i < 0 {
		return errPow
	}
	q, err := url.ParseQuery(s.Salt[i+1:])
	if err != nil || q.Get("pub") != pubID {
		return errPow
	}
	exp, err := strconv.ParseInt(q.Get("expires"), 10, 64)
	if err != nil || now.Unix() > exp {
		return errPow
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for sig, until := range l.used {
		if now.After(until) {
			delete(l.used, sig)
		}
	}
	if _, spent := l.used[s.Signature]; spent {
		return errPow
	}
	l.used[s.Signature] = time.Unix(exp, 0)
	return nil
}

// release returns a reserved signature, for a submit refused after verify.
func (l *powLedger) release(signature string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.used, signature)
}
