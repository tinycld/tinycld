package changesignal

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// createIndex creates a fictional FTS5 index, as a package's migration
// would. Its rows hold zoo_keepers' names, so a test maps it to zoo_keepers.
func createIndex(t *testing.T, app core.App, sql string) {
	t.Helper()
	if _, err := app.DB().NewQuery(sql).Execute(); err != nil {
		t.Fatalf("create index: %v", err)
	}
}

func indexApp(t *testing.T) core.App {
	t.Helper()
	app := newZooApp(t, time.Second)
	createIndex(t, app, "CREATE VIRTUAL TABLE fts_zoo USING fts5(record_id UNINDEXED, name)")
	return app
}

func requireTopics(t *testing.T, app core.App, query string, params dbx.Params, want ...string) {
	t.Helper()
	got, err := TopicsRead(context.Background(), app, query, params)
	if err != nil {
		t.Fatalf("TopicsRead(%s): %v", query, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopicsRead(%s) = %v, want %v", query, got, want)
	}
}

func requireRefused(t *testing.T, app core.App, query string, want string) {
	t.Helper()
	_, err := TopicsRead(context.Background(), app, query, nil)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("TopicsRead(%s) = %v, want an error with %q", query, err, want)
	}
}

const unmappedFTS = `reads virtual table "fts_zoo", which RegisterVirtualTable does not map`

// A table-valued function holds no stored rows, so it adds no topic, even
// beside a stored virtual table that a query does not name.
func TestTopicsReadAllowsATableValuedFunction(t *testing.T) {
	app := indexApp(t)
	requireTopics(t, app, `SELECT a.id FROM zoo_animals a
		WHERE a.name IN (SELECT value FROM json_each({:names}))
		AND EXISTS (SELECT 1 FROM json_tree({:names}))`,
		dbx.Params{"names": `["Leo"]`}, "collection/zoo_animals")
}

func TestTopicsReadRefusesAnUnmappedVirtualTable(t *testing.T) {
	app := indexApp(t)
	requireRefused(t, app, "SELECT record_id FROM fts_zoo WHERE fts_zoo MATCH 'leo'", unmappedFTS)
}

func TestTopicsReadMapsARegisteredVirtualTableToItsCollections(t *testing.T) {
	app := indexApp(t)
	RegisterVirtualTable("fts_zoo", "zoo_keepers")
	requireTopics(t, app, `SELECT a.id FROM zoo_animals a
		JOIN fts_zoo f ON f.record_id = a.id WHERE fts_zoo MATCH {:q}`,
		dbx.Params{"q": "leo"}, "collection/zoo_animals", "collection/zoo_keepers")
}

func TestTopicsReadRefusesAMappingToAnUnknownCollection(t *testing.T) {
	app := indexApp(t)
	RegisterVirtualTable("fts_zoo", "zoo_typo")
	requireRefused(t, app, "SELECT record_id FROM fts_zoo", `virtual table "fts_zoo" maps "zoo_typo", which is not a collection`)
}

// SQLite inlines a view, so its bytecode opens the virtual table while the
// query names only the view: the names of each view's SQL count, through
// views of views.
func TestTopicsReadFindsAVirtualTableThroughAView(t *testing.T) {
	app := indexApp(t)
	saveView(t, app, "zoo_found", `SELECT a.id, a.name FROM zoo_animals a WHERE a.id IN (SELECT record_id FROM fts_zoo)`)
	saveView(t, app, "zoo_found_named", `SELECT f.id, f.name AS zoo_found_named FROM zoo_found f`)
	requireRefused(t, app, "SELECT id FROM zoo_found_named", unmappedFTS)
	RegisterVirtualTable("fts_zoo", "zoo_keepers")
	requireTopics(t, app, "SELECT id FROM zoo_found_named", nil, "collection/zoo_animals", "collection/zoo_keepers")
}

// The name scan is conservative: when a query opens a virtual table, any
// name spelled like a stored virtual table counts as a read of it, even a
// column alias. Unmapped, the query is refused; mapped, it carries the
// mapped topics. A query that opens no virtual table is never scanned.
func TestTopicsReadCountsANameSpelledLikeAVirtualTable(t *testing.T) {
	app := indexApp(t)
	requireTopics(t, app, "SELECT id AS fts_zoo FROM zoo_animals", nil, "collection/zoo_animals")
	aliased := "SELECT value AS fts_zoo FROM json_each('[1]')"
	requireRefused(t, app, aliased, unmappedFTS)
	RegisterVirtualTable("fts_zoo", "zoo_keepers")
	requireTopics(t, app, aliased, nil, "collection/zoo_keepers")
}

// Names compare as SQLite compares them: quoted or not, in any case, with a
// schema prefix, as a dbx placeholder, and after a quote in a comment.
func TestTopicsReadMatchesNamesAsSQLiteDoes(t *testing.T) {
	app := newZooApp(t, time.Second)
	createIndex(t, app, "create virtual   table Zoo_Index using fts5(record_id UNINDEXED, name)")
	RegisterVirtualTable("ZOO_INDEX", "zoo_keepers")
	for _, query := range []string{
		"SELECT record_id FROM zoo_index",
		`SELECT record_id FROM "ZOO_Index"`,
		"SELECT record_id FROM [zoo_index]",
		"SELECT record_id FROM `zoo_index`",
		"SELECT record_id FROM 'zoo_index'",
		"SELECT record_id FROM main.zoo_index",
		"SELECT record_id FROM {{zoo_index}}",
		"SELECT [[i.record_id]] FROM zoo_index i",
		"SELECT record_id /* it's */ FROM -- don't\n zoo_index",
	} {
		requireTopics(t, app, query, nil, "collection/zoo_keepers")
	}
}

func TestIsVirtualTableSQL(t *testing.T) {
	for sql, want := range map[string]bool{
		"CREATE VIRTUAL TABLE fts_zoo USING fts5(name)":      true,
		"create\n\tvirtual  Table fts_zoo using fts5(name)":  true,
		"  CREATE VIRTUAL TABLE x USING rtree(id, a, b)":     true,
		"CREATE TABLE fts_zoo_data(id INTEGER PRIMARY KEY)":  false,
		"CREATE TABLE virtual_things(id)":                    false,
		"CREATE VIEW zoo_v AS SELECT 'CREATE VIRTUAL TABLE'": false,
		"": false,
	} {
		if got := isVirtualTableSQL(sql); got != want {
			t.Errorf("isVirtualTableSQL(%q) = %v, want %v", sql, got, want)
		}
	}
}

func TestIdentifiersReadsEveryWayToNameATable(t *testing.T) {
	got := identifiers("SELECT \"A b\".\"C\"\"d\", [e f], `G`, 'h''i', {{j_k}}, [[l.m]] FROM main.n -- o p'q\n /* r */ s")
	for _, want := range []string{"a b", `c"d`, "e f", "g", "h'i", "j_k", "l", "m", "main", "n", "o", "p", "q", "r", "s", "a", "b", "c", "d"} {
		if !got[want] {
			t.Errorf("identifiers has no %q: %v", want, got)
		}
	}
}

// dbx quotes the whole text of {{name}} and each dot-separated part of
// [[alias.column]], so a name with a character a bare word cannot hold is
// read whole.
func TestIdentifiersReadDbxMarkersWhole(t *testing.T) {
	got := identifiers("SELECT [[fts-zoo.record-id]] FROM {{FTS-Zoo}}")
	for _, want := range []string{"fts-zoo", "record-id"} {
		if !got[want] {
			t.Errorf("identifiers has no %q: %v", want, got)
		}
	}
}

func TestTopicsReadFindsAVirtualTableNamedInADbxMarker(t *testing.T) {
	app := newZooApp(t, time.Second)
	createIndex(t, app, `CREATE VIRTUAL TABLE "fts-zoo" USING fts5(record_id UNINDEXED, name)`)
	query := "SELECT [[f.record_id]] FROM {{fts-zoo}} f"
	requireRefused(t, app, query, `reads virtual table "fts-zoo", which RegisterVirtualTable does not map`)
	RegisterVirtualTable("fts-zoo", "zoo_keepers")
	requireTopics(t, app, query, nil, "collection/zoo_keepers")
}

// inTx runs fn on one connection: a temp table, a temp view and an attached
// database live on the connection that made them, and a transaction keeps
// the check's reads on that connection.
func inTx(t *testing.T, app core.App, fn func(tx core.App)) {
	t.Helper()
	err := app.RunInTransaction(func(tx core.App) error {
		fn(tx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func txRefused(t *testing.T, tx core.App, query, want string) {
	t.Helper()
	_, err := TopicsRead(context.Background(), tx, query, nil)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("TopicsRead(%s) = %v, want an error with %q", query, err, want)
	}
}

func txTopics(t *testing.T, tx core.App, query string, want ...string) {
	t.Helper()
	got, err := TopicsRead(context.Background(), tx, query, nil)
	if err != nil {
		t.Fatalf("TopicsRead(%s): %v", query, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopicsRead(%s) = %v, want %v", query, got, want)
	}
}

func TestTopicsReadFindsATempVirtualTable(t *testing.T) {
	app := newZooApp(t, time.Second)
	inTx(t, app, func(tx core.App) {
		createIndex(t, tx, "CREATE VIRTUAL TABLE temp.fts_scratch USING fts5(record_id UNINDEXED, name)")
		txRefused(t, tx, "SELECT record_id FROM fts_scratch", `reads virtual table "fts_scratch"`)
		RegisterVirtualTable("fts_scratch", "zoo_keepers")
		txTopics(t, tx, "SELECT record_id FROM fts_scratch", "collection/zoo_keepers")
	})
}

func TestTopicsReadFindsAVirtualTableThroughATempView(t *testing.T) {
	app := indexApp(t)
	inTx(t, app, func(tx core.App) {
		createIndex(t, tx, "CREATE TEMP VIEW zoo_scratch AS SELECT record_id FROM fts_zoo")
		txRefused(t, tx, "SELECT record_id FROM zoo_scratch", unmappedFTS)
		RegisterVirtualTable("fts_zoo", "zoo_keepers")
		txTopics(t, tx, "SELECT record_id FROM zoo_scratch", "collection/zoo_keepers")
	})
}

func TestTopicsReadFindsAnAttachedVirtualTable(t *testing.T) {
	app := newZooApp(t, time.Second)
	// A transaction runs on the one connection of NonconcurrentDB, which
	// attaches the database (ATTACH is refused inside a transaction).
	if _, err := app.NonconcurrentDB().NewQuery("ATTACH DATABASE ':memory:' AS zoo_side").Execute(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = app.NonconcurrentDB().NewQuery("DETACH DATABASE zoo_side").Execute() })
	inTx(t, app, func(tx core.App) {
		createIndex(t, tx, "CREATE VIRTUAL TABLE zoo_side.fts_side USING fts5(record_id UNINDEXED, name)")
		txRefused(t, tx, "SELECT record_id FROM zoo_side.fts_side", `reads virtual table "fts_side"`)
		RegisterVirtualTable("fts_side", "zoo_keepers")
		txTopics(t, tx, "SELECT record_id FROM zoo_side.fts_side", "collection/zoo_keepers")
	})
}

func TestCheckViewsCountsTheCollectionsOfAMappedVirtualTable(t *testing.T) {
	app := indexApp(t)
	saveView(t, app, "zoo_found", `SELECT a.id, a.name FROM zoo_animals a WHERE a.id IN (SELECT record_id FROM fts_zoo)`)
	RegisterVirtualTable("fts_zoo", "zoo_keepers")
	RegisterView("zoo_found", "zoo_animals")
	err := CheckViews(app)
	if err == nil || !strings.Contains(err.Error(), `view "zoo_found" reads "zoo_keepers", which RegisterView does not declare`) {
		t.Fatalf("CheckViews = %v", err)
	}
	RegisterView("zoo_found", "zoo_keepers")
	if err := CheckViews(app); err != nil {
		t.Fatalf("CheckViews: %v", err)
	}
}

func TestCheckViewsRefusesAViewOverAnUnmappedVirtualTable(t *testing.T) {
	app := indexApp(t)
	saveView(t, app, "zoo_found", `SELECT a.id, a.name FROM zoo_animals a WHERE a.id IN (SELECT record_id FROM fts_zoo)`)
	RegisterView("zoo_found", "zoo_animals", "zoo_keepers")
	err := CheckViews(app)
	if err == nil || !strings.Contains(err.Error(), `view "zoo_found": `+unmappedFTS) {
		t.Fatalf("CheckViews = %v", err)
	}
}
