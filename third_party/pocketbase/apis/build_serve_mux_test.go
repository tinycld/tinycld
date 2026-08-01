package apis_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/tests"
)

func TestBuildServeMux_ReturnsWorkingHandlerWithoutServer(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	handler, err := apis.BuildServeMux(app, apis.ServeConfig{})
	if err != nil {
		t.Fatalf("BuildServeMux error: %v", err)
	}
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	// /api/health is always registered by NewRouter -> bindHealthApi.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}
