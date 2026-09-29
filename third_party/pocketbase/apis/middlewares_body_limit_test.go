package apis_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestBodyLimitMiddleware(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}

	testHandler := func(e *core.RequestEvent) error {
		// read the body multiple times to ensure that the limited
		// reader guards and rereads are invoked
		var result any
		if err := e.BindBody(&result); err != nil {
			return err
		}

		if err := e.BindBody(&result); err != nil {
			return err
		}

		return e.JSON(200, result)
	}

	const customLimit = 20

	pbRouter.POST("/a", testHandler) // default global BodyLimit check
	pbRouter.POST("/b", testHandler).Bind(apis.BodyLimit(customLimit))
	pbRouter.POST("/iof", func(e *core.RequestEvent) error {
		// ensure that normal io methods still operate correctly
		b, err := io.ReadAll(e.Request.Body)
		if err != nil {
			return err
		}

		return e.String(http.StatusOK, string(b))
	}).Bind(apis.BodyLimit(customLimit))

	mux, err := pbRouter.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []struct {
		name              string
		url               string
		body              string
		lazyContentLength bool
		expectedStatus    int
	}{
		{
			"(eager content-length check) with body = default limit",
			"/a",
			`"` + strings.Repeat("a", int(apis.DefaultMaxBodySize-2)) + `"`,
			false,
			http.StatusOK,
		},
		{
			"(eager content-length check) with body > default limit",
			"/a",
			`"` + strings.Repeat("a", int(apis.DefaultMaxBodySize)) + `"`,
			false,
			http.StatusRequestEntityTooLarge,
		},
		{
			"(lazy content-length check) with body = default limit",
			"/a",
			`"` + strings.Repeat("a", int(apis.DefaultMaxBodySize-2)) + `"`,
			true,
			http.StatusOK,
		},
		{
			"(lazy content-length check) with body > default limit",
			"/a",
			`"` + strings.Repeat("a", int(apis.DefaultMaxBodySize)) + `"`,
			true,
			http.StatusRequestEntityTooLarge,
		},
		// ---
		{
			"(eager content-length check) with body = custom limit",
			"/b",
			`"` + strings.Repeat("a", customLimit-2) + `"`,
			false,
			http.StatusOK,
		},
		{
			"(eager content-length check) with body > custom limit",
			"/b",
			`"` + strings.Repeat("a", customLimit) + `"`,
			false,
			http.StatusRequestEntityTooLarge,
		},
		{
			"(lazy content-length check) with body = custom limit",
			"/b",
			`"` + strings.Repeat("a", customLimit-2) + `"`,
			true,
			http.StatusOK,
		},
		{
			"(lazy content-length check) with body > custom limit",
			"/b",
			`"` + strings.Repeat("a", customLimit) + `"`,
			true,
			http.StatusRequestEntityTooLarge,
		},
		// ---
		{
			"io.ReadAll io.EOF exact limit check",
			"/iof",
			`"` + strings.Repeat("a", customLimit-2) + `"`,
			true,
			http.StatusOK,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			req := httptest.NewRequest("POST", s.url, strings.NewReader(s.body))
			req.Header.Set("Content-Type", "application/json")

			if s.lazyContentLength {
				req.ContentLength = -1
			}

			mux.ServeHTTP(rec, req)

			result := rec.Result()
			defer result.Body.Close()

			if result.StatusCode != s.expectedStatus {
				t.Fatalf("Expected response status %d, got %d", s.expectedStatus, result.StatusCode)
			}
		})
	}
}
