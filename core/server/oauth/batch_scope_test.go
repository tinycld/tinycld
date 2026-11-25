package oauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// batchToken returns an access token of a grant with scopes, for a new user of app.
func batchToken(t *testing.T, app *tests.TestApp, scopes ...string) string {
	t.Helper()
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, scopes, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	token, err := MintAccessToken(app, user, grant, AccessTokenTTL)
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	return token
}

// batchEvent is a POST /api/batch with a JSON body, as enforceGrant sees it.
func batchEvent(app core.App, bearer, body string) *core.RequestEvent {
	req := httptest.NewRequest(http.MethodPost, batchPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	return re
}

func TestEnforceGrantChecksEachBatchOperation(t *testing.T) {
	const create = `{"method":"POST","url":"/api/collections/notes_items/records","body":{"title":"a"}}`
	cases := []struct {
		name, body string
		scopes     []string
		wantStatus int // 0: allowed
		wantText   string
	}{
		{"every operation in scope", `{"requests":[` + create + `,{"method":"PATCH","url":"/api/collections/notes_items/records/abc?expand=x"},{"method":"DELETE","url":"/api/collections/labels/records/abc"},{"method":"PUT","url":"/api/collections/notes_items/records"}]}`,
			[]string{scopeNotesWrite}, 0, ""},
		{"read scope only", `{"requests":[` + create + `]}`,
			[]string{scopeNotesRead}, 403, `Operation 1 of the batch requires the "notes:write" scope`},
		{"second operation is another package's", `{"requests":[` + create + `,{"method":"POST","url":"/api/collections/widgets_items/records"}]}`,
			[]string{scopeNotesWrite}, 403, `Operation 2 of the batch requires the "widgets:write" scope`},
		{"a read-only collection", `{"requests":[{"method":"POST","url":"/api/collections/notes_folder_counts/records"}]}`,
			[]string{scopeNotesRead, scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		{"a collection no package registered", `{"requests":[{"method":"POST","url":"/api/collections/_superusers/records"}]}`,
			[]string{scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		{"a route that is not a record write", `{"requests":[{"method":"POST","url":"/api/notes/send"}]}`,
			[]string{scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		{"a read", `{"requests":[{"method":"GET","url":"/api/collections/notes_items/records"}]}`,
			[]string{scopeNotesRead, scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		{"a lower-case method", `{"requests":[{"method":"post","url":"/api/collections/notes_items/records"}]}`,
			[]string{scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		{"an encoded collection name", `{"requests":[{"method":"POST","url":"/api/collections/notes%5Fitems/records"}]}`,
			[]string{scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		{"a path below a record", `{"requests":[{"method":"PATCH","url":"/api/collections/notes_items/records/abc/extra"}]}`,
			[]string{scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		{"a null operation", `{"requests":[null]}`,
			[]string{scopeNotesWrite}, 403, "Operation 1 of the batch is not available to API tokens"},
		// The key PocketBase's reader takes is the key that is checked: it
		// matches names without case, so "Requests" is the operations too.
		{"the operations under another case", `{"Requests":[{"method":"POST","url":"/api/collections/widgets_items/records"}]}`,
			[]string{scopeNotesWrite}, 403, `Operation 1 of the batch requires the "widgets:write" scope`},
		{"not JSON", `{"requests":`, []string{scopeNotesWrite}, 400, "Failed to read the submitted batch data."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := newSchemaApp(t)
			re := batchEvent(app, batchToken(t, app, c.scopes...), c.body)
			err := enforceGrant(re)
			if c.wantStatus == 0 {
				if err != nil {
					t.Fatalf("enforceGrant refused a batch in scope: %v", err)
				}
				if re.Auth == nil {
					t.Fatal("enforceGrant must set e.Auth for an allowed batch")
				}
				return
			}
			if err == nil {
				t.Fatal("enforceGrant allowed the batch")
			}
			if got := apiStatus(err); got != c.wantStatus {
				t.Fatalf("status = %d, want %d (%v)", got, c.wantStatus, err)
			}
			if !strings.Contains(err.Error(), c.wantText) {
				t.Fatalf("error = %q, want it to hold %q", err.Error(), c.wantText)
			}
			if re.Auth != nil {
				t.Fatal("enforceGrant must not set e.Auth when it refuses")
			}
		})
	}
}

func TestEnforceGrantRefusesBatchItCannotRead(t *testing.T) {
	app := newSchemaApp(t)
	token := batchToken(t, app, scopeNotesWrite)

	multipart := batchEvent(app, token, "x")
	multipart.Request.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	if err := enforceGrant(multipart); apiStatus(err) != 403 || !strings.Contains(err.Error(), "must be JSON") {
		t.Fatalf("multipart batch: %v, want 403 must be JSON", err)
	}

	chunked := batchEvent(app, token, `{"requests":[]}`)
	chunked.Request.ContentLength = -1
	if err := enforceGrant(chunked); apiStatus(err) != 403 || !strings.Contains(err.Error(), "must state its length") {
		t.Fatalf("batch with no length: %v, want 403 must state its length", err)
	}

	app.Settings().Batch.MaxBodySize = 10
	large := batchEvent(app, token, `{"requests":[{"method":"POST","url":"/api/collections/notes_items/records"}]}`)
	if err := enforceGrant(large); apiStatus(err) != http.StatusRequestEntityTooLarge {
		t.Fatalf("batch over the limit: %v, want 413", err)
	}

	// Only the exact route is a batch. Any other path keeps the default deny.
	other := batchEvent(app, token, `{"requests":[]}`)
	other.Request.URL.Path = batchPath + "/"
	if err := enforceGrant(other); apiStatus(err) != 403 || !strings.Contains(err.Error(), "not available to API tokens") {
		t.Fatalf("POST /api/batch/: %v, want the default deny", err)
	}
}

// notesBatchApp is a served app with the grant middleware, the batch API on,
// and the fixture collection notes_items, which any signed-in user may write.
func notesBatchApp(t *testing.T) *tests.TestApp {
	app := newSchemaApp(t)
	notes := core.NewBaseCollection("notes_items")
	notes.Fields.Add(&core.TextField{Name: "title"})
	rule := "@request.auth.id != ''"
	notes.ListRule, notes.ViewRule, notes.CreateRule, notes.UpdateRule, notes.DeleteRule = &rule, &rule, &rule, &rule, &rule
	if err := app.Save(notes); err != nil {
		t.Fatalf("save notes_items: %v", err)
	}
	app.Settings().Batch.Enabled = true
	app.Settings().Batch.MaxRequests = 50
	bindGrantEnforcement(app)
	return app
}

// Through the whole API: the body the middleware read is the body the batch
// handler runs, and a refused batch writes nothing.
func TestBatchWithATokenRunsThroughTheAPI(t *testing.T) {
	const body = `{"requests":[{"method":"POST","url":"/api/collections/notes_items/records","body":{"title":"one"}},{"method":"POST","url":"/api/collections/notes_items/records","body":{"title":"two"}}]}`
	count := func(t testing.TB, app *tests.TestApp) int64 {
		n, err := app.CountRecords("notes_items")
		if err != nil {
			t.Fatalf("count notes_items: %v", err)
		}
		return n
	}

	allowed := notesBatchApp(t)
	(&tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   batchPath,
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": "Bearer " + batchToken(t, allowed, scopeNotesWrite), "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"title":"one"`, `"title":"two"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return allowed },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			if got := count(t, app); got != 2 {
				t.Fatalf("notes_items rows = %d, want 2", got)
			}
		},
	}).Test(t)

	refused := notesBatchApp(t)
	(&tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   batchPath,
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": "Bearer " + batchToken(t, refused, scopeNotesRead), "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`Operation 1 of the batch requires the \"notes:write\" scope`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return refused },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			if got := count(t, app); got != 0 {
				t.Fatalf("notes_items rows = %d after a refused batch, want 0", got)
			}
		},
	}).Test(t)
}
