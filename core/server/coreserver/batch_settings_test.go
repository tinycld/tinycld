package coreserver

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestApplyBatchSettings_EnablesBatchWithCoreLimits(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	// A deployment that stored other values must not keep them.
	stored := app.Settings()
	stored.Batch = core.BatchConfig{Enabled: false, MaxRequests: 5, Timeout: 1}
	if err := app.Save(stored); err != nil {
		t.Fatal(err)
	}

	if err := applyBatchSettings(app); err != nil {
		t.Fatalf("applyBatchSettings: %v", err)
	}

	want := core.BatchConfig{Enabled: true, MaxRequests: 1100, Timeout: 30, MaxBodySize: 8 << 20}
	if got := app.Settings().Batch; got != want {
		t.Fatalf("batch settings = %+v, want %+v", got, want)
	}
	// Applying again changes nothing and must not fail.
	if err := applyBatchSettings(app); err != nil {
		t.Fatalf("second applyBatchSettings: %v", err)
	}
}

// The largest batch core allows must complete well inside the timeout on a
// collection with no hooks. A package with slow hooks measures its own.
func TestBatch_LargestAllowedBatchCompletes(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	if err := applyBatchSettings(app); err != nil {
		t.Fatal(err)
	}

	widgets := core.NewBaseCollection("widgets")
	widgets.Fields.Add(&core.TextField{Name: "name"})
	if err := app.Save(widgets); err != nil {
		t.Fatal(err)
	}
	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(superusers)
	su.SetEmail("batch-su@test.local")
	su.SetPassword("1234567890")
	if err := app.Save(su); err != nil {
		t.Fatal(err)
	}
	token, err := su.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	const count = 1100
	requests := make([]string, count)
	for i := range requests {
		requests[i] = fmt.Sprintf(`{"method":"POST","url":"/api/collections/widgets/records","body":{"name":"w%d"}}`, i)
	}
	body := `{"requests":[` + strings.Join(requests, ",") + `]}`

	started := time.Now()
	(&tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/batch",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": token, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"name":"w1099"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}).Test(t)
	took := time.Since(started)
	t.Logf("1,100 creates in one batch took %s", took)
	if took > 15*time.Second {
		t.Fatalf("1,100 creates took %s; the batch timeout is 30s", took)
	}

	total, err := app.CountRecords("widgets")
	if err != nil {
		t.Fatal(err)
	}
	if total != count {
		t.Fatalf("widgets = %d, want %d", total, count)
	}
}
