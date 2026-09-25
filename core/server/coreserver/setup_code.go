package coreserver

import (
	"crypto/rand"
	"crypto/subtle"
	"math/big"
	"strings"
	"sync"
	"time"
)

// setupCodeAlphabet leaves out 0/O, 1/I/L so a code read off a terminal and
// typed by hand cannot be misread. 31^8 is about 40 bits; with the lockout
// below that is far past what an online guesser can reach.
const setupCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

const (
	setupCodeLength     = 8
	setupIPFailureLimit = 5
	setupIPWindow       = 10 * time.Minute
	setupTotalFailLimit = 20
)

func generateSetupCode() (string, error) {
	max := big.NewInt(int64(len(setupCodeAlphabet)))
	b := make([]byte, setupCodeLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = setupCodeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// normalizeSetupCode accepts what a person types or pastes: any case, with
// the display dash or spaces.
func normalizeSetupCode(s string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func formatSetupCode(code string) string {
	if len(code) != setupCodeLength {
		return code
	}
	return code[:4] + "-" + code[4:]
}

type checkResult int

const (
	checkOK checkResult = iota
	checkMismatch
	checkLocked
	checkNoSetup
)

// setupGuard owns the first-run code and the lockout. One mutex covers the
// code, the counters AND the owner creation in Consume, so a check and the
// clearing of the code can never interleave with a second request.
type setupGuard struct {
	mu            sync.Mutex
	code          string
	failures      map[string][]time.Time
	totalFailures int
	now           func() time.Time
	announce      func(code string)
}

func newSetupGuard(now func() time.Time, announce func(code string)) *setupGuard {
	return &setupGuard{failures: map[string][]time.Time{}, now: now, announce: announce}
}

// Issue makes a new code and announces it. Called at boot while no owner
// exists, and again when the total failure limit is reached.
func (g *setupGuard) Issue() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.issueLocked()
}

func (g *setupGuard) issueLocked() error {
	code, err := generateSetupCode()
	if err != nil {
		return err
	}
	g.code = code
	g.totalFailures = 0
	g.failures = map[string][]time.Time{}
	g.announce(code)
	return nil
}

func (g *setupGuard) NeedsSetup() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.code != ""
}

func (g *setupGuard) Check(ip, input string) checkResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.checkLocked(ip, input)
}

func (g *setupGuard) checkLocked(ip, input string) checkResult {
	if g.code == "" {
		return checkNoSetup
	}
	if g.isLockedLocked(ip) {
		return checkLocked
	}
	if subtle.ConstantTimeCompare([]byte(normalizeSetupCode(input)), []byte(g.code)) == 1 {
		return checkOK
	}
	g.failures[ip] = append(g.recentLocked(ip), g.now())
	g.totalFailures++
	if g.totalFailures >= setupTotalFailLimit {
		if err := g.issueLocked(); err != nil {
			srvLog.Error("setup: failed to regenerate the setup code", "err", err)
		}
	}
	return checkMismatch
}

func (g *setupGuard) recentLocked(ip string) []time.Time {
	cutoff := g.now().Add(-setupIPWindow)
	var kept []time.Time
	for _, at := range g.failures[ip] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	return kept
}

func (g *setupGuard) isLockedLocked(ip string) bool {
	return len(g.recentLocked(ip)) >= setupIPFailureLimit
}

// Consume checks the code and, on a match, runs create while still holding
// the lock. The code is cleared only when create succeeds, so a failed
// create (bad email, weak password) can be retried with the same code.
func (g *setupGuard) Consume(ip, input string, create func() error) (checkResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	result := g.checkLocked(ip, input)
	if result != checkOK {
		return result, nil
	}
	if err := create(); err != nil {
		return checkOK, err
	}
	g.code = ""
	return checkOK, nil
}
