package coreserver

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGenerateSetupCodeShape(t *testing.T) {
	for i := 0; i < 200; i++ {
		code, err := generateSetupCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 8 {
			t.Fatalf("len(%q) = %d, want 8", code, len(code))
		}
		for _, r := range code {
			if !strings.ContainsRune(setupCodeAlphabet, r) {
				t.Fatalf("%q contains %q, which is not in the alphabet", code, r)
			}
		}
	}
}

func TestNormalizeAndFormatSetupCode(t *testing.T) {
	if got := normalizeSetupCode(" k7qm-3xpd "); got != "K7QM3XPD" {
		t.Errorf("normalize = %q", got)
	}
	if got := formatSetupCode("K7QM3XPD"); got != "K7QM-3XPD" {
		t.Errorf("format = %q", got)
	}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestGuard(t *testing.T) (*setupGuard, *fakeClock, *[]string) {
	t.Helper()
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	var announced []string
	g := newSetupGuard(clock.now, func(code string) { announced = append(announced, code) })
	if err := g.Issue(); err != nil {
		t.Fatal(err)
	}
	return g, clock, &announced
}

func TestSetupGuardCheck(t *testing.T) {
	g, _, announced := newTestGuard(t)
	code := (*announced)[0]
	if got := g.Check("1.1.1.1", strings.ToLower(formatSetupCode(code))); got != checkOK {
		t.Fatalf("correct code = %v, want checkOK", got)
	}
	if got := g.Check("1.1.1.1", "AAAAAAAA"); got != checkMismatch {
		t.Fatalf("wrong code = %v, want checkMismatch", got)
	}
}

func TestSetupGuardLocksAnIPAfterFiveFailures(t *testing.T) {
	g, clock, announced := newTestGuard(t)
	code := (*announced)[0]
	for i := 0; i < 5; i++ {
		g.Check("1.1.1.1", "AAAAAAAA")
	}
	if got := g.Check("1.1.1.1", code); got != checkLocked {
		t.Fatalf("after 5 failures = %v, want checkLocked", got)
	}
	if got := g.Check("2.2.2.2", code); got != checkOK {
		t.Fatalf("other IP = %v, want checkOK", got)
	}
	clock.t = clock.t.Add(10*time.Minute + time.Second)
	if got := g.Check("1.1.1.1", code); got != checkOK {
		t.Fatalf("after lockout expiry = %v, want checkOK", got)
	}
}

func TestSetupGuardRegeneratesAfterTwentyFailures(t *testing.T) {
	g, _, announced := newTestGuard(t)
	first := (*announced)[0]
	for i := 0; i < 20; i++ {
		// A different IP each time so no single-IP lockout masks the total.
		g.Check(strings.Repeat("9", i+1), "AAAAAAAA")
	}
	if len(*announced) != 2 {
		t.Fatalf("announced %d codes, want 2", len(*announced))
	}
	if got := g.Check("3.3.3.3", first); got != checkMismatch {
		t.Fatalf("old code after regeneration = %v, want checkMismatch", got)
	}
}

// Two concurrent inits with the right code: exactly one creates the owner.
func TestSetupGuardConsumeIsSingleUse(t *testing.T) {
	g, _, announced := newTestGuard(t)
	code := (*announced)[0]
	var mu sync.Mutex
	created := 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = g.Consume("1.1.1.1", code, func() error {
				mu.Lock()
				created++
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if created != 1 {
		t.Fatalf("created %d owners, want 1", created)
	}
	if g.NeedsSetup() {
		t.Fatal("NeedsSetup after a successful consume")
	}
}

// A failed create keeps the code, so the person can fix the input and retry.
func TestSetupGuardConsumeKeepsCodeOnCreateError(t *testing.T) {
	g, _, announced := newTestGuard(t)
	code := (*announced)[0]
	_, err := g.Consume("1.1.1.1", code, func() error { return errors.New("boom") })
	if err == nil {
		t.Fatal("want the create error back")
	}
	if !g.NeedsSetup() {
		t.Fatal("code was cleared by a failed create")
	}
}
