package readonly

import (
	"net/http"
	"strings"
	"testing"

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
				for _, p := range []string{"/api/x", "/elsewhere"} {
					e.Router.Any(p, ok)
				}
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

func TestMiddlewareOnlyCoversAPI(t *testing.T) {
	scenario(t, http.MethodPost, "/elsewhere", true, http.StatusOK, nil)
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
