//go:build unix

package readonly

import (
	"syscall"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"
)

func TestSIGUSR2Enters(t *testing.T) {
	Leave()
	t.Cleanup(Leave)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	Register(app)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !Active() {
		if time.Now().After(deadline) {
			t.Fatal("SIGUSR2 did not enter read-only mode")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
