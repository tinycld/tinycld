package readonly

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// scenario runs one request against a test app with the middleware bound and
// routes that answer 200 for any method, so a 503 can only come from the mode.
func scenario(t *testing.T, method, url string, active bool, wantStatus int, wantBody []string) {
	t.Helper()
	if active {
		Enter()
	} else {
		Leave()
	}
	t.Cleanup(Leave)
	s := &tests.ApiScenario{
		Method:          method,
		URL:             url,
		ExpectedStatus:  wantStatus,
		ExpectedContent: wantBody,
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			app, err := tests.NewTestApp()
			if err != nil {
				t.Fatal(err)
			}
			Register(app)
			app.OnServe().BindFunc(func(e *core.ServeEvent) error {
				ok := func(re *core.RequestEvent) error { return re.String(http.StatusOK, "handled") }
				e.Router.Any("/{path...}", ok)
				return e.Next()
			})
			return app
		},
	}
	if wantStatus == http.StatusOK {
		s.ExpectedContent = []string{"handled"}
	}
	s.Test(t)
}

func TestMiddlewareRefusesWritesWhileActive(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		scenario(t, m, "/api/x", true, http.StatusServiceUnavailable, []string{`"code":"read_only"`, `"message":"The server is updating. Try again in a moment."`})
	}
}

func TestMiddlewareLetsReadsThrough(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		scenario(t, m, "/api/x", true, http.StatusOK, nil)
	}
}

func TestMiddlewareInactivePassesWrites(t *testing.T) {
	scenario(t, http.MethodPost, "/api/x", false, http.StatusOK, nil)
}

// DAV writes (/caldav, /carddav, /dav/drive, a package prefix) are not under
// /api/ but still write, so the mode must refuse them on every path.
func TestMiddlewareRefusesDAVWritesWhileActive(t *testing.T) {
	cases := []struct {
		method string
		url    string
	}{
		{http.MethodPut, "/caldav/x.ics"},
		{"MKCOL", "/dav/drive/a"},
		{"PROPPATCH", "/caldav/x.ics"},
		{"MOVE", "/carddav/a.vcf"},
		{http.MethodDelete, "/carddav/a.vcf"},
	}
	for _, c := range cases {
		scenario(t, c.method, c.url, true, http.StatusServiceUnavailable, []string{`"code":"read_only"`})
	}
}

func TestMiddlewareLetsDAVWritesThroughWhenInactive(t *testing.T) {
	cases := []struct {
		method string
		url    string
	}{
		{http.MethodPut, "/caldav/x.ics"},
		{"MKCOL", "/dav/drive/a"},
		{"PROPPATCH", "/caldav/x.ics"},
		{"MOVE", "/carddav/a.vcf"},
		{http.MethodDelete, "/carddav/a.vcf"},
	}
	for _, c := range cases {
		scenario(t, c.method, c.url, false, http.StatusOK, nil)
	}
}

// PROPFIND and REPORT are DAV read methods: they must pass like GET.
func TestMiddlewareLetsDAVReadsThroughWhileActive(t *testing.T) {
	scenario(t, "PROPFIND", "/caldav/x.ics", true, http.StatusOK, nil)
	scenario(t, "REPORT", "/carddav/a.vcf", true, http.StatusOK, nil)
}

func TestMiddlewareCoversEveryPath(t *testing.T) {
	scenario(t, http.MethodPost, "/elsewhere", true, http.StatusServiceUnavailable, []string{`"code":"read_only"`})
}

func TestMiddlewareLetsPlainGETThroughWhileActive(t *testing.T) {
	scenario(t, http.MethodGet, "/", true, http.StatusOK, nil)
}

func TestMiddlewareSetsRetryAfter(t *testing.T) {
	Enter()
	t.Cleanup(Leave)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	Register(app)
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/x", func(re *core.RequestEvent) error { return re.NoContent(http.StatusOK) })
		return e.Next()
	})
	(&tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/x",
		ExpectedStatus:        http.StatusServiceUnavailable,
		ExpectedContent:       []string{`"code":"read_only"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			if got := res.Header.Get("Retry-After"); got != "2" {
				t.Fatalf("Retry-After = %q, want 2", got)
			}
		},
	}).Test(t)
}

// POST /api/realtime only sets which topics an open stream carries; a client
// that reconnects during the pause must be able to subscribe again.
func TestMiddlewareLetsRealtimeSubscribe(t *testing.T) {
	Enter()
	t.Cleanup(Leave)
	(&tests.ApiScenario{
		Method:          http.MethodPost,
		URL:             "/api/realtime",
		Body:            strings.NewReader(`{"clientId":"missing","subscriptions":[]}`),
		ExpectedStatus:  http.StatusNotFound, // PocketBase's own answer for an unknown client: not 503
		ExpectedContent: []string{`"data":{}`},
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			app, err := tests.NewTestApp()
			if err != nil {
				t.Fatal(err)
			}
			Register(app)
			return app
		},
	}).Test(t)
}

func TestEnterLeave(t *testing.T) {
	Leave()
	Enter()
	Enter()
	if !Active() {
		t.Fatal("Enter did not switch the mode on")
	}
	Leave()
	if Active() {
		t.Fatal("Leave did not switch the mode off")
	}
}

// OnEnter fns run once per false->true transition, in their own goroutine, and
// must not run again on a second Enter while already active.
func TestOnEnterRunsOnEachTransitionToActive(t *testing.T) {
	t.Cleanup(resetOnEnterForTest)
	t.Cleanup(Leave)
	Leave()

	var calls atomic.Int32
	done := make(chan struct{}, 10)
	OnEnter(func() {
		calls.Add(1)
		done <- struct{}{}
	})

	Enter()
	waitOrFatal(t, done, "OnEnter fn did not run on Enter")
	Enter() // already active: must not run again
	select {
	case <-done:
		t.Fatal("OnEnter fn ran again on a repeated Enter while already active")
	case <-time.After(100 * time.Millisecond):
	}

	Leave()
	Enter()
	waitOrFatal(t, done, "OnEnter fn did not run on the next false->true transition")

	if got := calls.Load(); got != 2 {
		t.Fatalf("OnEnter fn ran %d times, want 2", got)
	}
}

// OnEnter must not block Enter: a slow or blocked fn runs in its own
// goroutine, so Enter returns immediately regardless.
func TestOnEnterDoesNotBlockEnter(t *testing.T) {
	t.Cleanup(resetOnEnterForTest)
	t.Cleanup(Leave)
	Leave()

	started := make(chan struct{})
	release := make(chan struct{})
	OnEnter(func() {
		close(started)
		<-release
	})

	enterDone := make(chan struct{})
	go func() {
		Enter()
		close(enterDone)
	}()

	select {
	case <-enterDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Enter blocked on a slow OnEnter fn")
	}
	<-started
	close(release)
}

func waitOrFatal(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(msg)
	}
}

func TestWhenWritableRunsAtOnceWhenInactive(t *testing.T) {
	t.Cleanup(Leave)
	Leave()
	var ran bool
	err := WhenWritable(context.Background(), func() error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("WhenWritable = %v, want nil", err)
	}
	if !ran {
		t.Fatal("fn did not run while inactive")
	}
}

func TestWhenWritableWaitsAndRunsAfterLeave(t *testing.T) {
	t.Cleanup(Leave)
	Enter()

	done := make(chan error, 1)
	ran := make(chan struct{}, 1)
	go func() {
		done <- WhenWritable(context.Background(), func() error {
			ran <- struct{}{}
			return nil
		})
	}()

	select {
	case <-ran:
		t.Fatal("fn ran while still read-only")
	case <-time.After(100 * time.Millisecond):
	}

	Leave()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WhenWritable = %v after Leave, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WhenWritable still waits after Leave")
	}
}

func TestWhenWritableReturnsCtxErrWithoutRunningFn(t *testing.T) {
	t.Cleanup(Leave)
	Enter()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var ran bool
	err := WhenWritable(ctx, func() error {
		ran = true
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WhenWritable = %v, want context.DeadlineExceeded", err)
	}
	if ran {
		t.Fatal("fn ran even though ctx ended first")
	}
}
