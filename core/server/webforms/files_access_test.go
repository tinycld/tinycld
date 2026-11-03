package webforms

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// A submission file downloads with an admin's ordinary bearer token, as the
// CLI sends it; the fork's view-rule check still keeps it from a member and
// from an anonymous caller.
func TestSubmissionFileDownloadsWithABearerToken(t *testing.T) {
	f := newFixture(t, contactDef)
	sub := storeForTest(t, f, map[string]any{"name": "Ada"},
		upload{"files.cv", "cv.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n")})
	file, err := f.app.FindFirstRecordByData("form_submission_files", "submission", sub.Id)
	if err != nil {
		t.Fatal(err)
	}
	url := "/api/files/form_submission_files/" + file.Id + "/" + file.GetString("file")
	admin := rlsUser(t, f.app, "files-admin@test.local", "admin")
	member := rlsUser(t, f.app, "files-member@test.local", "member")
	factory := func(testing.TB) *tests.TestApp { return f.app }
	for _, sc := range []tests.ApiScenario{
		{Name: "admin bearer", Method: http.MethodGet, URL: url, Headers: map[string]string{"Authorization": admin}, ExpectedStatus: 200, ExpectedContent: []string{"%PDF-1.4"}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "member bearer", Method: http.MethodGet, URL: url, Headers: map[string]string{"Authorization": member}, ExpectedStatus: 404, ExpectedContent: []string{`"status":404`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "anonymous", Method: http.MethodGet, URL: url, ExpectedStatus: 404, ExpectedContent: []string{`"status":404`}, TestAppFactory: factory, DisableTestAppCleanup: true},
	} {
		sc.Test(t)
	}
}
