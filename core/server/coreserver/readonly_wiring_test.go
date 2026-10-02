package coreserver

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/readonly"
)

// A write refused during a pause is expected, not an error: Sentry must not
// hear about it. registerSharedMiddleware binds read-only first for that.
func TestReadOnlyRefusesBeforeSentry(t *testing.T) {
	get, cleanup := captureSentry(t)
	defer cleanup()
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	registerSharedMiddleware(app)
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
	}).Test(t)
	if events := get(); len(events) != 0 {
		t.Fatalf("Sentry got %d events for a refused write", len(events))
	}
}
