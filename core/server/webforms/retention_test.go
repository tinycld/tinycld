package webforms

import (
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/tools/types"
)

func backdate(t *testing.T, f *fixture, clientID string, age time.Duration) {
	t.Helper()
	rec, err := f.app.FindFirstRecordByData("form_submissions", "client_id", clientID)
	if err != nil {
		t.Fatal(err)
	}
	when, _ := types.ParseDateTime(time.Now().Add(-age))
	if _, err := f.app.DB().Update("form_submissions", map[string]any{"created": when.String()}, dbx.HashExp{"id": rec.Id}).Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestPruneSubmissionsByPublicationRetention(t *testing.T) {
	f := newFixture(t, contactDef) // retention 90 days
	submit(t, f, "old", base(t, f, "old"), 200, []string{`"ok":true`})
	submit(t, f, "new", base(t, f, "new"), 200, []string{`"ok":true`})
	backdate(t, f, "old", 91*24*time.Hour)
	backdate(t, f, "new", 10*24*time.Hour)

	n, err := pruneSubmissions(f.app, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("one old submission deleted, got %d %v", n, err)
	}
	if _, err := f.app.FindFirstRecordByData("form_submissions", "client_id", "new"); err != nil {
		t.Fatal("a recent submission stays")
	}
}

func TestRetentionJobIsScheduled(t *testing.T) {
	f := newFixture(t, contactDef)
	found := false
	for _, j := range f.app.Cron().Jobs() {
		if j.Id() == retentionJobID {
			found = true
		}
	}
	if !found {
		t.Fatal("the retention job must be registered")
	}
}
