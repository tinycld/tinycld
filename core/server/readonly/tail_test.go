package readonly

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"
)

func registeredTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	Register(app)
	return app
}

// A supervised upgrade keeps the mode on until the process exits, so a tail
// parked in the wait must be ended by the app's terminate, without its write.
func TestTailParkedInReadOnlyEndsAtTerminate(t *testing.T) {
	t.Cleanup(Leave)
	app := registeredTestApp(t)
	Enter()

	ctx, release := TailContext(app)
	ran := false
	result := make(chan error, 1)
	go func() {
		defer release()
		result <- WhenWritable(ctx, func() error {
			ran = true
			return nil
		})
	}()

	app.Cleanup()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want the terminate's cancel, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the tail was still waiting after the app terminated")
	}
	if ran {
		t.Fatal("a tail ended by terminate must not write")
	}
}

// The terminate waits for a tail already writing, so its save lands before
// the database closes.
func TestTerminateWaitsForATailInFlight(t *testing.T) {
	app := registeredTestApp(t)
	ctx, release := TailContext(app)

	cleaned := make(chan struct{})
	go func() {
		app.Cleanup()
		close(cleaned)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the terminate did not end the tail's context")
	}
	select {
	case <-cleaned:
		t.Fatal("the app closed while a tail was still in flight")
	default:
	}

	release()
	select {
	case <-cleaned:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not close after the tail finished")
	}
}

func TestTailContextAfterTerminateIsDone(t *testing.T) {
	app := registeredTestApp(t)
	app.Cleanup()
	ctx, release := TailContext(app)
	defer release()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("a tail that starts after the app terminated must not wait")
	}
}

// A test app that never called Register still gets a bounded, live context.
func TestTailContextWithoutRegister(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	ctx, release := TailContext(app)
	defer release()
	if ctx.Err() != nil {
		t.Fatalf("ctx already done: %v", ctx.Err())
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > TailWait {
		t.Fatalf("want a deadline within TailWait, got %v (%v)", deadline, ok)
	}
}
