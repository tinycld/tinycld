package coreserver

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
)

// captureSentry installs a Sentry client that records events into a slice
// instead of sending them. Returns a getter for the captured events and a
// cleanup func that re-initializes Sentry with the empty default state so
// later tests are not contaminated.
func captureSentry(t *testing.T) (func() []*sentry.Event, func()) {
	t.Helper()

	var (
		mu     sync.Mutex
		events []*sentry.Event
	)

	err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://public@example.com/1",
		BeforeSend: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
			return nil // dropping the event prevents the HTTP transport from firing
		},
	})
	if err != nil {
		t.Fatalf("sentry.Init: %v", err)
	}

	get := func() []*sentry.Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]*sentry.Event, len(events))
		copy(out, events)
		return out
	}
	cleanup := func() {
		_ = sentry.Init(sentry.ClientOptions{})
	}
	return get, cleanup
}

// runScenario boots a TestApp with the Sentry middleware bound and a single
// test route, then drives it with the provided ApiScenario.
func runScenario(t *testing.T, mountRoute func(g *router.RouterGroup[*core.RequestEvent]), scenario *tests.ApiScenario) {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	registerSentryMiddlewareCore(app)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		mountRoute(e.Router.RouterGroup)
		return e.Next()
	})

	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

func TestSentryMiddlewareCapturesReturnedError(t *testing.T) {
	get, cleanup := captureSentry(t)
	defer cleanup()

	mount := func(g *router.RouterGroup[*core.RequestEvent]) {
		g.GET("/test/boom", func(re *core.RequestEvent) error {
			return router.NewInternalServerError("boom", errors.New("underlying boom"))
		})
	}
	runScenario(t, mount, &tests.ApiScenario{
		Name:            "500 from returned ApiError",
		Method:          http.MethodGet,
		URL:             "/test/boom",
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{`"status":500`},
	})

	events := get()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	if events[0].Level != sentry.LevelError {
		t.Errorf("expected LevelError, got %s", events[0].Level)
	}
	if len(events[0].Exception) == 0 || events[0].Exception[0].Value != "underlying boom" {
		t.Errorf("expected captured exception 'underlying boom', got %+v", events[0].Exception)
	}
}

func TestSentryMiddlewareSkipsClientErrors(t *testing.T) {
	get, cleanup := captureSentry(t)
	defer cleanup()

	mount := func(g *router.RouterGroup[*core.RequestEvent]) {
		g.GET("/test/forbidden", func(re *core.RequestEvent) error {
			return router.NewForbiddenError("nope", nil)
		})
	}
	runScenario(t, mount, &tests.ApiScenario{
		Name:            "403 should not capture",
		Method:          http.MethodGet,
		URL:             "/test/forbidden",
		ExpectedStatus:  http.StatusForbidden,
		ExpectedContent: []string{`"status":403`},
	})

	if events := get(); len(events) != 0 {
		t.Fatalf("expected zero captured events for a 4xx, got %d: %+v", len(events), events)
	}
}

func TestSentryMiddlewareCapturesPanic(t *testing.T) {
	get, cleanup := captureSentry(t)
	defer cleanup()

	mount := func(g *router.RouterGroup[*core.RequestEvent]) {
		g.GET("/test/panic", func(re *core.RequestEvent) error {
			panic("kaboom")
		})
	}
	runScenario(t, mount, &tests.ApiScenario{
		Name:            "panicking handler",
		Method:          http.MethodGet,
		URL:             "/test/panic",
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{`"status":500`},
	})

	events := get()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	if events[0].Level != sentry.LevelFatal {
		t.Errorf("expected LevelFatal for panic, got %s", events[0].Level)
	}
}

// TestSentryMiddlewareSkipsHealthEndpoint confirms that paths in the skip
// set (such as /api/health) are fully excluded from capture even when they
// return a 5xx — flapping dependencies or mid-rollout deploys would
// otherwise flood Sentry with noise that adds no diagnostic value over
// what k8s/uptime checks already surface. We use a non-conflicting custom
// path because PB's default router already binds /api/health.
func TestSentryMiddlewareSkipsHealthEndpoint(t *testing.T) {
	get, cleanup := captureSentry(t)
	defer cleanup()

	const probePath = "/api/_test_probe"
	sentrySkipPaths[probePath] = true
	defer delete(sentrySkipPaths, probePath)

	mount := func(g *router.RouterGroup[*core.RequestEvent]) {
		g.GET(probePath, func(re *core.RequestEvent) error {
			return router.NewInternalServerError("probe broken", errors.New("db down"))
		})
	}
	runScenario(t, mount, &tests.ApiScenario{
		Name:            "failing probe in skip set should not capture",
		Method:          http.MethodGet,
		URL:             probePath,
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{`"status":500`},
	})

	if events := get(); len(events) != 0 {
		t.Fatalf("expected zero captured events for %s, got %d: %+v", probePath, len(events), events)
	}
}

// TestSentryMiddlewareSkipsDirectWriteClientError mirrors the direct-write
// 5xx case for a 4xx response — confirming the status-based capture path
// only fires on server errors, not client errors written through the same
// nil-returning handler shape.
func TestSentryMiddlewareSkipsDirectWriteClientError(t *testing.T) {
	get, cleanup := captureSentry(t)
	defer cleanup()

	mount := func(g *router.RouterGroup[*core.RequestEvent]) {
		g.GET("/test/raw400", func(re *core.RequestEvent) error {
			http.Error(re.Response, "raw 400", http.StatusBadRequest)
			return nil
		})
	}
	runScenario(t, mount, &tests.ApiScenario{
		Name:            "raw 400 written to ResponseWriter",
		Method:          http.MethodGet,
		URL:             "/test/raw400",
		ExpectedStatus:  http.StatusBadRequest,
		ExpectedContent: []string{"raw 400"},
	})

	if events := get(); len(events) != 0 {
		t.Fatalf("expected zero captured events for a direct-write 4xx, got %d: %+v", len(events), events)
	}
}

// TestSentryMiddlewareCapturesDirectWriteServerError covers the case that
// motivated this middleware: a handler that writes a 5xx directly to the
// ResponseWriter (e.g. the go-webdav CalDAV library), returns nil, and so
// produces no error value for PB's normal path to capture. The captured
// body should appear under the "response" context so the Sentry event
// surfaces the actual error message that http.Error wrote, not just the
// status code.
func TestSentryMiddlewareCapturesDirectWriteServerError(t *testing.T) {
	get, cleanup := captureSentry(t)
	defer cleanup()

	mount := func(g *router.RouterGroup[*core.RequestEvent]) {
		g.GET("/test/raw500", func(re *core.RequestEvent) error {
			http.Error(re.Response, "underlying db connection refused", http.StatusInternalServerError)
			return nil
		})
	}
	runScenario(t, mount, &tests.ApiScenario{
		Name:            "raw 500 written to ResponseWriter",
		Method:          http.MethodGet,
		URL:             "/test/raw500",
		ExpectedStatus:  http.StatusInternalServerError,
		ExpectedContent: []string{"underlying db connection refused"},
	})

	events := get()
	if len(events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(events))
	}
	if events[0].Message == "" {
		t.Errorf("expected a non-empty message for the captured 500, got %+v", events[0])
	}
	respCtx, ok := events[0].Contexts["response"]
	if !ok {
		t.Fatalf("expected a 'response' context with the captured body, got contexts=%+v", events[0].Contexts)
	}
	body, ok := respCtx["body"].(string)
	if !ok {
		t.Fatalf("expected response.body to be a string, got %T (%+v)", respCtx["body"], respCtx["body"])
	}
	if !strings.Contains(body, "underlying db connection refused") {
		t.Errorf("expected captured body to contain the http.Error message, got %q", body)
	}
}

// initSentryFromConfig is safe to call with an empty or a set DSN — empty leaves
// the SDK a no-op, a value swaps the active client. It must never panic; the boot
// path and the on-change re-init both call it unconditionally.
func TestInitSentryFromConfig(t *testing.T) {
	t.Cleanup(func() { _ = sentry.Init(sentry.ClientOptions{}) }) // reset global client

	cfg := &SystemConfig{values: map[string]string{}}
	initSentryFromConfig(cfg) // empty DSN — no-op, no panic

	cfg.set("sentry.dsn", "https://abc@o1.ingest.sentry.io/1")
	initSentryFromConfig(cfg) // valid-shaped DSN — swaps the client, no panic
}

// The re-init wiring fires ONLY for sentry.* keys: a sentry.dsn change re-inits,
// an unrelated key (e.g. vapid.subject) does not. This mirrors the prefix filter
// RegisterSystemConfig installs; push/mail read per-use and must not trigger a
// Sentry re-init.
func TestSentryReinitOnlyForSentryKeys(t *testing.T) {
	cfg := &SystemConfig{values: map[string]string{}}
	var reinits int
	cfg.OnChange(func(key, _ string) {
		if strings.HasPrefix(key, "sentry.") {
			reinits++
		}
	})

	cfg.set("vapid.subject", "mailto:a@b.c")
	cfg.set("mail.postmark_server_token", "tok")
	if reinits != 0 {
		t.Errorf("non-sentry keys must not trigger reinit, got %d", reinits)
	}

	cfg.set("sentry.dsn", "https://x@o1.ingest.sentry.io/2")
	cfg.set("sentry.org", "myorg")
	if reinits != 2 {
		t.Errorf("each sentry.* change should reinit once, got %d", reinits)
	}
}
