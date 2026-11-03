package webforms

import (
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
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

func TestPruneUsesEachPublicationsOwnRetention(t *testing.T) {
	f := newFixture(t, contactDef) // 90 days
	submit(t, f, "a old", base(t, f, "keep90"), 200, []string{`"ok":true`})
	backdate(t, f, "keep90", 40*24*time.Hour)

	second := secondPublication(t, f, 30)
	submit(t, second, "b old", base(t, second, "drop30"), 200, []string{`"ok":true`})
	backdate(t, second, "drop30", 40*24*time.Hour)

	n, err := pruneSubmissions(f.app, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("only the 30-day publication's submission goes, got %d %v", n, err)
	}
	if _, err := f.app.FindFirstRecordByData("form_submissions", "client_id", "keep90"); err != nil {
		t.Fatal("a 40-day-old submission of a 90-day publication stays")
	}
}

func TestPruneContinuesAfterAFailedDelete(t *testing.T) {
	f := newFixture(t, contactDef)
	submit(t, f, "stuck", base(t, f, "stuck"), 200, []string{`"ok":true`})
	submit(t, f, "free", base(t, f, "free"), 200, []string{`"ok":true`})
	backdate(t, f, "stuck", 92*24*time.Hour)
	backdate(t, f, "free", 91*24*time.Hour)
	f.app.OnRecordDelete("form_submissions").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("client_id") == "stuck" {
			return errors.New("locked")
		}
		return e.Next()
	})

	n, err := pruneSubmissions(f.app, time.Now())
	if n != 1 || err == nil {
		t.Fatalf("one delete fails, the other still happens and the error is reported: %d %v", n, err)
	}
}
