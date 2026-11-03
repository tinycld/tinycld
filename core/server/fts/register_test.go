package fts

import (
	"context"
	"reflect"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

// newZooIndexApp has the fictional zoo_animals collection and its FTS5
// index, created as a package's migration would create it.
func newZooIndexApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	changesignal.ResetForTesting()
	t.Cleanup(changesignal.ResetForTesting)
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir(), HideStartBanner: true})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	animals := core.NewBaseCollection("zoo_animals")
	animals.Fields.Add(&core.TextField{Name: "name"})
	if err := app.Save(animals); err != nil {
		t.Fatalf("save collection: %v", err)
	}
	if _, err := app.NonconcurrentDB().NewQuery(
		"CREATE VIRTUAL TABLE fts_zoo_animals USING fts5(record_id UNINDEXED, name)",
	).Execute(); err != nil {
		t.Fatalf("create fts table: %v", err)
	}
	return app
}

var zooAnimalsIndex = Config{
	Slug: "zoo", Collection: "zoo_animals", Table: "fts_zoo_animals",
	Columns: []Column{{FTS: "name", Field: "name"}},
}

// The sync keeps the index's rows in step with its collection, so a report
// or a view over the index reads that collection: its topic tells the reader
// when to run again.
func TestRegisterSyncMapsTheIndexToItsCollection(t *testing.T) {
	app := newZooIndexApp(t)
	RegisterSync(app, zooAnimalsIndex)
	got, err := changesignal.TopicsRead(context.Background(), app,
		"SELECT record_id FROM fts_zoo_animals WHERE fts_zoo_animals MATCH {:q}", dbx.Params{"q": "leo"})
	if err != nil {
		t.Fatalf("TopicsRead: %v", err)
	}
	if want := []string{"collection/zoo_animals"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TopicsRead = %v, want %v", got, want)
	}
}

func TestRegisterMapsEachConfigsIndex(t *testing.T) {
	app := newZooIndexApp(t)
	Register(app, []Config{zooAnimalsIndex})
	want := map[string][]string{"fts_zoo_animals": {"zoo_animals"}}
	if got := changesignal.RegisteredVirtualTables(); !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredVirtualTables = %v, want %v", got, want)
	}
}
