package hold

import (
	"sync/atomic"
	"testing"
	"time"

	"tinycld.org/core/readonly"
)

// The lease must keep renewing while the server is read-only: if it lapsed,
// deletes held for a backup still reading the files would go through. This
// guards against a future read-only check reaching renew.
func TestLeaseKeepsRenewingWhileReadOnly(t *testing.T) {
	prev := renewInterval
	renewInterval = 10 * time.Millisecond
	t.Cleanup(func() { renewInterval = prev })
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	base := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	var calls atomic.Int64
	now := func() time.Time { return base.Add(time.Duration(calls.Add(1)) * time.Minute) }

	dir := t.TempDir()
	h, err := Acquire(dir, "engine", now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Release() })
	first, ok, err := Read(dir)
	if err != nil || !ok {
		t.Fatalf("read: %v %v", ok, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, ok, err := Read(dir)
		if err == nil && ok && st.Expires.After(first.Expires) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the hold lease was not renewed while the server was read-only")
}
