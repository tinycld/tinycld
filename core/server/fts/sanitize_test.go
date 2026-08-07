package fts

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// liveFTSTokenizer mirrors the tokenizer string used by feature FTS migrations
// (e.g. contacts' pb-migrations/..._create_fts_contacts.js). Keep them in sync:
// these tests are the guard that the tokenizer + SanitizeQuery actually match the
// email-bearing search terms a real bug showed were being missed.
const liveFTSTokenizer = "porter unicode61"

type seedRow struct {
	id, first, last, email, company, phone, notes string
}

// newContactsShapedFTS builds an in-memory FTS5 index with the contacts column
// shape — the reference collection for the shared capability's query behavior.
func newContactsShapedFTS(t *testing.T, tokenizer string, rows ...seedRow) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE fts_contacts USING fts5(
			record_id UNINDEXED, first_name, last_name, email, company, phone, notes,
			tokenize='` + tokenizer + `'
		)`); err != nil {
		t.Fatalf("create fts (%q): %v", tokenizer, err)
	}

	for _, r := range rows {
		if _, err := db.Exec(`
			INSERT INTO fts_contacts
				(record_id, first_name, last_name, email, company, phone, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.id, r.first, r.last, r.email, r.company, r.phone, r.notes,
		); err != nil {
			t.Fatalf("seed %q: %v", r.id, err)
		}
	}
	return db
}

// matchIDs runs the raw user query through SanitizeQuery (the production path)
// and MATCHes the index.
func matchIDs(t *testing.T, db *sql.DB, userQuery string) []string {
	t.Helper()
	q := SanitizeQuery(userQuery)
	if q == "" {
		return nil
	}
	rows, err := db.Query(
		`SELECT record_id FROM fts_contacts WHERE fts_contacts MATCH ? ORDER BY rank`, q,
	)
	if err != nil {
		t.Fatalf("match %q (fts=%q): %v", userQuery, q, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func assertMatches(t *testing.T, db *sql.DB, userQuery string, wantIDs ...string) {
	t.Helper()
	got := matchIDs(t, db, userQuery)
	gotSet := map[string]bool{}
	for _, id := range got {
		gotSet[id] = true
	}
	if len(got) != len(wantIDs) {
		t.Errorf("query %q: got %v, want exactly %v", userQuery, got, wantIDs)
		return
	}
	for _, want := range wantIDs {
		if !gotSet[want] {
			t.Errorf("query %q: missing %q (got %v)", userQuery, want, got)
		}
	}
}

func assertNoMatch(t *testing.T, db *sql.DB, userQuery string) {
	t.Helper()
	if got := matchIDs(t, db, userQuery); len(got) != 0 {
		t.Errorf("query %q: expected no matches, got %v", userQuery, got)
	}
}

var (
	alice = seedRow{"alice", "Alice", "Smith", "zephyrqa.test@example.com", "Acme", "", ""}
	bob   = seedRow{"bob", "Bob", "Jones", "bob@globex.io", "Globex", "555-1212", ""}
	carol = seedRow{"carol", "Carol", "Nguyen", "c.nguyen+sales@sub.example.org", "Initech", "", "VIP client"}
)

// THE BUG: a partial email address that skips an interior token must still match.
// Typing "zephyrqa@example.com" (omitting the ".test" middle token) used to be
// wrapped as a single ordered phrase and matched nothing.
func TestFTSPartialEmailAddressMatches(t *testing.T) {
	db := newContactsShapedFTS(t, liveFTSTokenizer, alice, bob, carol)

	assertMatches(t, db, "zephyrqa@example.com", "alice")
	assertMatches(t, db, "zephyrqa.test@example.com", "alice")
	assertMatches(t, db, "bob@globex.io", "bob")
	assertMatches(t, db, "c.nguyen@example.org", "carol")
}

func TestFTSEmailLocalPartTokens(t *testing.T) {
	db := newContactsShapedFTS(t, liveFTSTokenizer, alice, bob, carol)

	assertMatches(t, db, "zephyr", "alice")
	assertMatches(t, db, "zephyrqa", "alice")
	assertMatches(t, db, "nguyen", "carol")
	assertMatches(t, db, "sales", "carol")
}

func TestFTSEmailDomainTokens(t *testing.T) {
	db := newContactsShapedFTS(t, liveFTSTokenizer, alice, bob, carol)

	assertMatches(t, db, "example", "alice", "carol")
	assertMatches(t, db, "globex", "bob")
	assertMatches(t, db, "globex.io", "bob")
	assertMatches(t, db, "sub.example.org", "carol")
}

func TestFTSNameAndCompanyStillWork(t *testing.T) {
	db := newContactsShapedFTS(t, liveFTSTokenizer, alice, bob, carol)

	assertMatches(t, db, "alice", "alice")
	assertMatches(t, db, "smith", "alice")
	assertMatches(t, db, "acme", "alice")
	assertMatches(t, db, "Carol Nguyen", "carol")
}

func TestFTSNonMatchingQueryReturnsNothing(t *testing.T) {
	db := newContactsShapedFTS(t, liveFTSTokenizer, alice, bob, carol)

	assertNoMatch(t, db, "nonexistentxyz")
	assertNoMatch(t, db, "zephyr globex")
}

// SanitizeQuery must neutralize FTS5 operator characters without throwing, and
// must split email punctuation into separate prefix terms.
func TestSanitizeQuery(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"john", `"john"*`},
		{"john doe", `"john"* "doe"*`},
		{"a@b.com", `"a"* "b"* "com"*`},
		{"a.b@example.com", `"a"* "b"* "example"* "com"*`},
		{"*", ""},
		{"()", ""},
		{`"`, ""},
		{"-", ""},
		{"foo*", `"foo"*`},
		{"(bar)", `"bar"*`},
	}
	for _, tc := range cases {
		if got := SanitizeQuery(tc.in); got != tc.want {
			t.Errorf("SanitizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// stripHTML must remove markup so HTML in editor fields doesn't pollute the index.
func TestStripHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"plain text", "plain text"},
		{"<p>hello</p>", "hello"},
		{"<div><b>VIP</b> client</div>", "VIP  client"},
		{"  <span>trim me</span>  ", "trim me"},
	}
	for _, tc := range cases {
		if got := stripHTML(tc.in); got != tc.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFTSNotesAreSearchableAfterStrip(t *testing.T) {
	withNotes := carol
	withNotes.notes = stripHTML("<p>loves <b>kayaking</b></p>")
	db := newContactsShapedFTS(t, liveFTSTokenizer, alice, withNotes)

	assertMatches(t, db, "kayaking", "carol")
}

func TestSanitizeQueryWithExclusions(t *testing.T) {
	cases := []struct {
		name             string
		include, exclude string
		want             string
	}{
		{"no exclusions", "budget", "", `"budget"*`},
		{"one exclusion", "budget", "draft", `"budget"* NOT "draft"*`},
		{"two exclusions", "budget", "draft old", `"budget"* NOT "draft"* NOT "old"*`},
		{"exclude only yields nothing", "", "draft", ""},
		{"blank both", "", "", ""},
	}
	for _, tc := range cases {
		if got := SanitizeQueryWithExclusions(tc.include, tc.exclude); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// An exclusion must actually remove the row, not merely fail to boost it.
func TestExclusionRemovesMatchingRow(t *testing.T) {
	db := newContactsShapedFTS(t, liveFTSTokenizer,
		seedRow{id: "1", first: "Ada", notes: "budget planning"},
		seedRow{id: "2", first: "Grace", notes: "budget draft"},
	)
	q := SanitizeQueryWithExclusions("budget", "draft")
	rows, err := db.Query(`SELECT record_id FROM fts_contacts WHERE fts_contacts MATCH ?`, q)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 1 || ids[0] != "1" {
		t.Errorf("got %v, want [1]", ids)
	}
}
