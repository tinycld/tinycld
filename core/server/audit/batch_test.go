package audit

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// The audit hooks themselves, driven through the real /api/batch endpoint: a
// helper test cannot see a hook that asks the wrong app for its transaction.

type auditBatchEnv struct {
	app   *tests.TestApp
	token string
	// failAfterSave makes the outermost widgets create hook fail after every
	// operation of the batch was saved and every audit hook returned.
	failAfterSave atomic.Bool
}

func setupAuditBatch(t *testing.T) *auditBatchEnv {
	t.Helper()
	app, _ := auditTestApp(t)
	env := &auditBatchEnv{app: app}

	settings := app.Settings()
	settings.Batch.Enabled = true
	settings.Batch.MaxRequests = 50
	settings.Batch.Timeout = 3
	if err := app.Save(settings); err != nil {
		t.Fatal(err)
	}

	// Bound before the audit hooks, so it is the outermost hook.
	app.OnRecordCreateRequest("widgets").BindFunc(func(e *core.RecordRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if env.failAfterSave.Load() {
			return errors.New("refused after the save by the test")
		}
		return nil
	})
	bindCollection(app, "widgets", nil)

	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(superusers)
	su.SetEmail("audit-batch@test.local")
	su.SetPassword("1234567890")
	if err := app.Save(su); err != nil {
		t.Fatal(err)
	}
	if env.token, err = su.NewAuthToken(); err != nil {
		t.Fatal(err)
	}
	return env
}

func (env *auditBatchEnv) post(t *testing.T, url, body string, status int) {
	t.Helper()
	(&tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   url,
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": env.token, "Content-Type": "application/json"},
		ExpectedStatus:        status,
		ExpectedContent:       []string{`"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func (env *auditBatchEnv) createdLabels(t *testing.T) []string {
	t.Helper()
	rows, err := env.app.FindRecordsByFilter("audit_logs",
		"action = 'created' && resource_type = 'widgets'", "resource_label", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	labels := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = r.GetString("resource_label")
	}
	return labels
}

// waitForRows waits for want audit rows. Audit rows are written off the
// request, in no fixed order.
func (env *auditBatchEnv) waitForRows(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		labels := env.createdLabels(t)
		if len(labels) >= want {
			return labels
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit rows = %v, want %d", labels, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const twoWidgets = `{"requests":[
	{"method":"POST","url":"/api/collections/widgets/records","body":{"name":"batch-a"}},
	{"method":"POST","url":"/api/collections/widgets/records","body":{"name":"batch-b"}}]}`

func TestAuditBatch_CommittedBatchWritesARowForEachOperation(t *testing.T) {
	env := setupAuditBatch(t)
	env.post(t, "/api/batch", twoWidgets, http.StatusOK)

	rows := env.waitForRows(t, 2)
	if len(rows) != 2 || rows[0] != "batch-a" || rows[1] != "batch-b" {
		t.Fatalf("audit rows = %v, want batch-a and batch-b", rows)
	}
}

// The case afterCommit exists for: every audit hook returned success, and
// then the batch rolled back.
func TestAuditBatch_RolledBackBatchWritesNoRow(t *testing.T) {
	env := setupAuditBatch(t)
	env.failAfterSave.Store(true)
	env.post(t, "/api/batch", twoWidgets, http.StatusBadRequest)
	env.failAfterSave.Store(false)

	if n, err := env.app.CountRecords("widgets"); err != nil || n != 1 {
		t.Fatalf("widgets = %d (err %v), want only the fixture row: the batch must roll back", n, err)
	}
	// A later single create is the marker: its row is scheduled after any row
	// the rolled-back batch scheduled. The short hold after it only lets such
	// a row land; it cannot fail a correct build.
	env.post(t, "/api/collections/widgets/records", `{"name":"sentinel"}`, http.StatusOK)
	env.waitForRows(t, 1)
	time.Sleep(200 * time.Millisecond)
	if rows := env.createdLabels(t); len(rows) != 1 || rows[0] != "sentinel" {
		t.Fatalf("audit rows = %v, want only sentinel: the batch rolled back", rows)
	}
}
