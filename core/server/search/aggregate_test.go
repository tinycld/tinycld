package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// testApp gives Aggregate a real core.App, which it needs only for its logger.
func testApp(t *testing.T) core.App {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })
	return app
}

// rowSource returns a source that answers with the given rows.
func rowSource(slug string, order int, total int, titles ...string) Source {
	rows := make([]Row, len(titles))
	for i, title := range titles {
		rows[i] = Row{ID: slug + "-" + title, Title: title}
	}
	if total == 0 {
		total = len(rows)
	}
	return Source{
		Slug: slug, Label: slug, Order: order,
		Search: func(core.App, string, Query) (Result, error) {
			return Result{Rows: rows, Total: total}, nil
		},
	}
}

func titlesOf(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Title
	}
	return out
}

func TestAggregateMergesAndOrdersAcrossPackages(t *testing.T) {
	// The case that motivates scoring at all: without it, nav.order alone would
	// put mail's weak hit above drive's exact title match.
	app := testApp(t)
	sources := []Source{
		{
			Slug: "mail", Order: 5,
			Search: func(core.App, string, Query) (Result, error) {
				return Result{Rows: []Row{{ID: "m1", Title: "Q3 approval", Subtitle: "budget team"}}, Total: 1}, nil
			},
		},
		rowSource("drive", 12, 1, "budget"),
	}

	resp := Aggregate(context.Background(), app, "u1", Query{Include: []string{"budget"}}, sources)
	if got := titlesOf(resp.Rows); len(got) != 2 || got[0] != "budget" {
		t.Fatalf("rows = %v, want drive's exact match first", got)
	}
	if resp.Counts["mail"] != 1 || resp.Counts["drive"] != 1 {
		t.Fatalf("counts = %v", resp.Counts)
	}
}

func TestAggregateStampsSlugFromTheSource(t *testing.T) {
	// A source must not be able to label rows as another package's.
	app := testApp(t)
	liar := Source{
		Slug: "boards", Order: 25,
		Search: func(core.App, string, Query) (Result, error) {
			return Result{Rows: []Row{{Slug: "mail", ID: "c1", Title: "x"}}, Total: 1}, nil
		},
	}
	resp := Aggregate(context.Background(), app, "u1", Query{Include: []string{"x"}}, []Source{liar})
	if len(resp.Rows) != 1 || resp.Rows[0].Slug != "boards" {
		t.Fatalf("rows = %+v, want slug stamped as cards", resp.Rows)
	}
}

func TestAggregateIsolatesAFailingSource(t *testing.T) {
	// Contractual, inherited from the client fan-out this replaces: "one
	// backend erroring must not empty the whole palette."
	app := testApp(t)
	sources := []Source{
		{
			Slug: "mail", Order: 5,
			Search: func(core.App, string, Query) (Result, error) {
				return Result{}, errors.New("index corrupt")
			},
		},
		rowSource("drive", 12, 1, "budget"),
	}

	resp := Aggregate(context.Background(), app, "u1", Query{Include: []string{"budget"}}, sources)
	if len(resp.Rows) != 1 || resp.Rows[0].Slug != "drive" {
		t.Fatalf("rows = %+v, want drive's row despite mail failing", resp.Rows)
	}
	if len(resp.Partial) != 1 || resp.Partial[0] != "mail" {
		t.Fatalf("partial = %v, want [mail] — a dropped package must be reported", resp.Partial)
	}
	if _, counted := resp.Counts["mail"]; counted {
		t.Error("a failed source must not report a count, which would read as a real zero")
	}
}

func TestAggregateIsolatesAPanickingSource(t *testing.T) {
	app := testApp(t)
	sources := []Source{
		{
			Slug: "boards", Order: 25,
			Search: func(core.App, string, Query) (Result, error) {
				panic("nil map write")
			},
		},
		rowSource("drive", 12, 1, "budget"),
	}

	resp := Aggregate(context.Background(), app, "u1", Query{Include: []string{"budget"}}, sources)
	if len(resp.Rows) != 1 {
		t.Fatalf("a panicking package must degrade to no rows, got %+v", resp.Rows)
	}
	if len(resp.Partial) != 1 || resp.Partial[0] != "boards" {
		t.Fatalf("partial = %v, want [cards]", resp.Partial)
	}
}

func TestAggregateTimesOutASlowSourceWithoutBlockingTheRest(t *testing.T) {
	app := testApp(t)
	slow := Source{
		Slug: "mail", Order: 5,
		Search: func(core.App, string, Query) (Result, error) {
			time.Sleep(2 * sourceTimeout)
			return Result{}, nil
		},
	}
	sources := []Source{slow, rowSource("drive", 12, 1, "budget")}

	start := time.Now()
	resp := Aggregate(context.Background(), app, "u1", Query{Include: []string{"budget"}}, sources)
	elapsed := time.Since(start)

	if elapsed >= 2*sourceTimeout {
		t.Fatalf("waited %v — a slow source must be cut off, not awaited", elapsed)
	}
	if len(resp.Rows) != 1 || resp.Rows[0].Slug != "drive" {
		t.Fatalf("rows = %+v, want drive's row", resp.Rows)
	}
	if len(resp.Partial) != 1 || resp.Partial[0] != "mail" {
		t.Fatalf("partial = %v, want [mail]", resp.Partial)
	}
}

func TestAggregateRequiresAPositiveTerm(t *testing.T) {
	// FTS5 errors on a NOT-only MATCH — there is nothing to subtract from — so
	// an exclude-only query must yield empty rather than reach any source.
	app := testApp(t)
	called := false
	src := Source{
		Slug: "mail", Order: 5,
		Search: func(core.App, string, Query) (Result, error) {
			called = true
			return Result{}, nil
		},
	}

	resp := Aggregate(context.Background(), app, "u1", Query{Exclude: []string{"draft"}}, []Source{src})
	if called {
		t.Error("an exclude-only query must not reach a source")
	}
	if len(resp.Rows) != 0 {
		t.Errorf("rows = %+v, want none", resp.Rows)
	}
}

func TestAggregateRefusesAnUnauthenticatedCaller(t *testing.T) {
	app := testApp(t)
	called := false
	src := Source{
		Slug: "mail", Order: 5,
		Search: func(core.App, string, Query) (Result, error) {
			called = true
			return Result{}, nil
		},
	}
	Aggregate(context.Background(), app, "", Query{Include: []string{"budget"}}, []Source{src})
	if called {
		t.Error("no userID must mean no source call — scoping depends on it")
	}
}

func TestAggregateReportsTruncation(t *testing.T) {
	// A client that cannot tell "5 results" from "5 of 500" will imply
	// completeness it was never given.
	app := testApp(t)
	src := rowSource("mail", 5, 500, "budget a", "budget b")

	resp := Aggregate(context.Background(), app, "u1", Query{Include: []string{"budget"}}, []Source{src})
	if len(resp.Truncated) != 1 || resp.Truncated[0] != "mail" {
		t.Fatalf("truncated = %v, want [mail]", resp.Truncated)
	}
	if resp.Counts["mail"] != 500 {
		t.Fatalf("count = %d, want the source's full total", resp.Counts["mail"])
	}
}

func TestAggregatePagesTheMergedSet(t *testing.T) {
	// Paging must happen after the merge: forwarding Offset to each source
	// would page every package independently and silently drop rows.
	app := testApp(t)
	var gotOffset int
	src := Source{
		Slug: "mail", Order: 5,
		Search: func(_ core.App, _ string, q Query) (Result, error) {
			gotOffset = q.Offset
			return Result{Rows: []Row{
				{ID: "1", Title: "budget one"},
				{ID: "2", Title: "budget two"},
				{ID: "3", Title: "budget three"},
			}, Total: 3}, nil
		},
	}

	resp := Aggregate(context.Background(), app, "u1",
		Query{Include: []string{"budget"}, Limit: 1, Offset: 1}, []Source{src})
	if gotOffset != 0 {
		t.Errorf("source saw offset %d — it must always report from its first hit", gotOffset)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("rows = %+v, want one", resp.Rows)
	}

	// An offset past the end is an empty page, not an error.
	resp = Aggregate(context.Background(), app, "u1",
		Query{Include: []string{"budget"}, Limit: 5, Offset: 99}, []Source{src})
	if len(resp.Rows) != 0 {
		t.Fatalf("rows = %+v, want none past the end", resp.Rows)
	}
}

func TestAggregateOverFetchesSoRankingCanChoose(t *testing.T) {
	// Each source must be asked for more than the page size, or a package whose
	// hits all score poorly still occupies its share of the merged page.
	app := testApp(t)
	var gotLimit int
	src := Source{
		Slug: "mail", Order: 5,
		Search: func(_ core.App, _ string, q Query) (Result, error) {
			gotLimit = q.Limit
			return Result{}, nil
		},
	}
	Aggregate(context.Background(), app, "u1", Query{Include: []string{"x"}, Limit: 10}, []Source{src})
	if gotLimit <= 10 {
		t.Fatalf("source limit = %d, want more than the page size", gotLimit)
	}
}
