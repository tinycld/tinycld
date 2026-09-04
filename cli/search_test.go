package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"tinycld.org/cli/client"
	"tinycld.org/cli/internal/config"
)

// searchServer stands in for GET /api/search, recording the query it was sent
// so the tests can assert on the wire format the grammar produced.
type searchServer struct {
	srv       *httptest.Server
	lastQuery url.Values
	response  searchResponse
}

func newSearchServer(t *testing.T) *searchServer {
	t.Helper()
	s := &searchServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		s.lastQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.response)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

// searchDeps wires deps to the fake server with a valid stored credential, so
// the command reaches the endpoint instead of stopping at authentication.
func searchDeps(t *testing.T, s *searchServer) *deps {
	t.Helper()
	d, store := testDeps(t)
	d.httpClient = s.srv.Client()
	// Fixed rather than the generated list: that one reflects whichever
	// packages the checkout assembled (empty in CI), which would make these
	// assertions pass or fail on the machine rather than on the code.
	d.slugs = []string{"mail", "drive", "boards", "contacts"}

	host := strings.TrimPrefix(s.srv.URL, "http://")
	cfg := &config.Config{
		Current:  host,
		Contexts: map[string]config.Context{host: {Origin: s.srv.URL}},
	}
	if err := cfg.Save(d.configDir); err != nil {
		t.Fatal(err)
	}
	tok, err := json.Marshal(client.TokenSet{
		AccessToken: "access-1",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(host, string(tok)); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSearchSendsParsedGrammar(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)

	// Chips scope the request, the bare word stays a term, and the hyphenated
	// term becomes an exclusion — the same split the palette would produce.
	if _, _, err := runCLI(t, d, "search", "mail: drive: budget -draft"); err != nil {
		t.Fatal(err)
	}

	if got := s.lastQuery.Get("q"); got != "budget" {
		t.Errorf("q = %q, want %q", got, "budget")
	}
	if got := s.lastQuery.Get("not"); got != "draft" {
		t.Errorf("not = %q, want %q", got, "draft")
	}
	if got := s.lastQuery["pkg"]; len(got) != 2 || got[0] != "mail" || got[1] != "drive" {
		t.Errorf("pkg = %v, want [mail drive]", got)
	}
}

func TestSearchMergesPkgFlagWithChips(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)

	if _, _, err := runCLI(t, d, "search", "mail: budget", "--pkg", "drive"); err != nil {
		t.Fatal(err)
	}

	// Additive, not overriding: both express scope, and dropping either would
	// search a package the user did not ask for (or skip one they did).
	got := s.lastQuery["pkg"]
	if len(got) != 2 || got[0] != "mail" || got[1] != "drive" {
		t.Errorf("pkg = %v, want [mail drive]", got)
	}
}

func TestSearchNotFlagAddsToExclusions(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)

	if _, _, err := runCLI(t, d, "search", "budget -draft", "--not", "archived"); err != nil {
		t.Fatal(err)
	}

	got := strings.Fields(s.lastQuery.Get("not"))
	if len(got) != 2 || got[0] != "draft" || got[1] != "archived" {
		t.Errorf("not = %v, want [draft archived]", got)
	}
}

func TestSearchTableOutput(t *testing.T) {
	s := newSearchServer(t)
	s.response = searchResponse{
		Rows: []searchRow{
			{
				Slug: "drive", ID: "d1", Title: "<mark>budget</mark>-2026.xlsx",
				Subtitle: "Spreadsheet", Meta: "2d",
			},
		},
		Counts: map[string]int{"drive": 1},
	}
	d := searchDeps(t, s)

	stdout, _, err := runCLI(t, d, "search", "budget")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stdout, "PKG") || !strings.Contains(stdout, "SUBTITLE") {
		t.Errorf("headers missing:\n%s", stdout)
	}
	// Highlight tags are the server's markup, not part of the file name.
	if strings.Contains(stdout, "<mark>") {
		t.Errorf("mark tags survived into the table:\n%s", stdout)
	}
	if !strings.Contains(stdout, "budget-2026.xlsx") {
		t.Errorf("title missing:\n%s", stdout)
	}
}

// --json must stay a clean document on stdout: Fields is the whole reason a
// script would use it, and any summary text would break `jq`.
func TestSearchJSONKeepsFieldsAndCleanStdout(t *testing.T) {
	s := newSearchServer(t)
	s.response = searchResponse{
		Rows: []searchRow{{
			Slug: "mail", ID: "m1", Title: "Q3 budget",
			Fields: map[string]any{"mailbox_id": "mb1"},
		}},
		Counts: map[string]int{"mail": 1},
	}
	d := searchDeps(t, s)

	stdout, stderr, err := runCLI(t, d, "search", "budget", "--json")
	if err != nil {
		t.Fatal(err)
	}

	var decoded searchResponse
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON (%v):\n%s", err, stdout)
	}
	if got := decoded.Rows[0].Fields["mailbox_id"]; got != "mb1" {
		t.Errorf("Fields did not survive --json: %#v", decoded.Rows[0].Fields)
	}
	if !strings.Contains(stderr, "1 result(s)") {
		t.Errorf("summary should go to stderr, got:\n%s", stderr)
	}
}

func TestSearchReportsPerPackageCounts(t *testing.T) {
	s := newSearchServer(t)
	s.response = searchResponse{
		Rows:   []searchRow{{Slug: "mail", ID: "m1", Title: "Q3 budget"}},
		Counts: map[string]int{"mail": 12, "drive": 3, "boards": 0},
	}
	d := searchDeps(t, s)

	_, stderr, err := runCLI(t, d, "search", "budget")
	if err != nil {
		t.Fatal(err)
	}

	// Ordered by count, and a package with no matches is not listed.
	if !strings.Contains(stderr, "(12 in mail, 3 in drive)") {
		t.Errorf("counts = %q", stderr)
	}
	if strings.Contains(stderr, "boards") {
		t.Errorf("zero-count package should not be listed: %q", stderr)
	}
}

// A dropped package must be named. Silently omitting it reads exactly like
// "nothing matched there", which is the more misleading of the two.
func TestSearchNamesPartialAndTruncatedPackages(t *testing.T) {
	s := newSearchServer(t)
	s.response = searchResponse{
		Rows:      []searchRow{{Slug: "mail", ID: "m1", Title: "Q3 budget"}},
		Counts:    map[string]int{"mail": 1},
		Partial:   []string{"drive"},
		Truncated: []string{"mail"},
	}
	d := searchDeps(t, s)

	_, stderr, err := runCLI(t, d, "search", "budget")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stderr, "warning") || !strings.Contains(stderr, "drive") {
		t.Errorf("partial package not reported: %q", stderr)
	}
	if !strings.Contains(stderr, "more matches available in mail") {
		t.Errorf("truncated package not reported: %q", stderr)
	}
}

// --quiet suppresses chatter, but a package that failed is a correctness
// caveat rather than chatter: hiding it would let a partial result read as
// complete in exactly the scripted runs that cannot notice.
func TestSearchQuietStillWarnsAboutPartial(t *testing.T) {
	s := newSearchServer(t)
	s.response = searchResponse{
		Rows:    []searchRow{{Slug: "mail", ID: "m1", Title: "Q3 budget"}},
		Counts:  map[string]int{"mail": 1},
		Partial: []string{"drive"},
	}
	d := searchDeps(t, s)

	_, stderr, err := runCLI(t, d, "search", "budget", "--quiet")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(stderr, "result(s)") {
		t.Errorf("--quiet should suppress the count line: %q", stderr)
	}
	if !strings.Contains(stderr, "drive") {
		t.Errorf("--quiet must not hide a dropped package: %q", stderr)
	}
}

func TestSearchRejectsExcludeOnlyQuery(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)

	// FTS5 cannot subtract from nothing, so this would come back as a
	// confident empty result set. Fail with a reason instead.
	//
	// `--` is required for a query that STARTS with a hyphen, otherwise cobra
	// claims it as a flag before the command runs. A hyphenated term anywhere
	// else in the query ("budget -draft") needs no separator — see
	// TestSearchSendsParsedGrammar.
	_, _, err := runCLI(t, d, "search", "--", "-draft")
	if err == nil {
		t.Fatal("expected an error for an exclude-only query")
	}
	if !strings.Contains(err.Error(), "at least one term") {
		t.Errorf("unhelpful error: %v", err)
	}
	if s.lastQuery != nil {
		t.Error("should not have reached the server")
	}
}

// An all-exclusions query reaching the server would be a silent empty result;
// this is the same guard as above via the --not flag, which needs no `--`.
func TestSearchRejectsNotFlagWithoutTerms(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)

	_, _, err := runCLI(t, d, "search", "--not", "draft", "--", "  ")
	if err == nil {
		t.Fatal("expected an error when every term is an exclusion")
	}
	if !strings.Contains(err.Error(), "at least one term") {
		t.Errorf("unhelpful error: %v", err)
	}
	if s.lastQuery != nil {
		t.Error("should not have reached the server")
	}
}

// --limit bounds the MERGED result set, and a package with more matches than
// were returned must be named: without it a bounded list reads as the complete
// answer, which is the failure --limit makes most likely.
// A binary built without an assembled workspace has no generated slug list.
// It must still search: validating --pkg against an empty list would reject
// every package the server can actually reach, and the server validates scope
// authoritatively anyway.
func TestSearchWithNoGeneratedSlugsStillSearches(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)
	d.slugs = []string{}

	if _, _, err := runCLI(t, d, "search", "budget", "--pkg", "mail"); err != nil {
		t.Fatalf("should not reject --pkg with no slug list: %v", err)
	}
	if got := s.lastQuery["pkg"]; len(got) != 1 || got[0] != "mail" {
		t.Errorf("pkg = %v, want [mail]", got)
	}
	// With no list, `mail:` cannot be recognized as a chip and stays a term.
	if got := s.lastQuery.Get("q"); got != "budget" {
		t.Errorf("q = %q, want %q", got, "budget")
	}
}

func TestSearchLimitIsSentAndTruncationReported(t *testing.T) {
	s := newSearchServer(t)
	s.response = searchResponse{
		Rows: []searchRow{
			{Slug: "mail", ID: "m1", Title: "Q3 budget"},
			{Slug: "mail", ID: "m2", Title: "budget draft"},
		},
		Counts:    map[string]int{"mail": 12},
		Truncated: []string{"mail"},
	}
	d := searchDeps(t, s)

	_, stderr, err := runCLI(t, d, "search", "budget", "--limit", "2")
	if err != nil {
		t.Fatal(err)
	}

	if got := s.lastQuery.Get("limit"); got != "2" {
		t.Errorf("limit = %q, want %q", got, "2")
	}
	// The count is the package's full total, not the number of rows shown.
	if !strings.Contains(stderr, "12 in mail") {
		t.Errorf("full count not reported: %q", stderr)
	}
	if !strings.Contains(stderr, "more matches available in mail") {
		t.Errorf("truncation not reported: %q", stderr)
	}
}

// Omitting --limit must not send limit=0, which would read as an explicit
// "return nothing" rather than "server default".
func TestSearchOmitsLimitWhenUnset(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)

	if _, _, err := runCLI(t, d, "search", "budget"); err != nil {
		t.Fatal(err)
	}
	if s.lastQuery.Has("limit") {
		t.Errorf("limit should be absent, got %q", s.lastQuery.Get("limit"))
	}
}

func TestSearchRejectsUnknownPackage(t *testing.T) {
	s := newSearchServer(t)
	d := searchDeps(t, s)

	// A typo would otherwise return an empty set that looks like a real answer.
	_, _, err := runCLI(t, d, "search", "budget", "--pkg", "maill")
	if err == nil {
		t.Fatal("expected an error for an unknown package")
	}
	if !strings.Contains(err.Error(), "maill") {
		t.Errorf("error should name the bad slug: %v", err)
	}
	if s.lastQuery != nil {
		t.Error("should not have reached the server")
	}
}

// --not is a list of terms to exclude. Anything else it parses into — a `pkg:`
// chip, a nested `-term` — has no meaning there, and was silently dropped:
// `--not "pkg:mail"` looked like it worked and filtered nothing.
func TestSearchNotRejectsNonTerms(t *testing.T) {
	srv := newSearchServer(t)
	d := searchDeps(t, srv)

	for _, tc := range []struct{ name, not, wantMsg string }{
		{"a package chip", "mail:", "--pkg"},
		{"a nested negation", "-spam", "nested negation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runCLI(t, d, "search", "invoice", "--not", tc.not)
			if err == nil {
				t.Fatalf("--not %q must be refused, not silently dropped", tc.not)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error should mention %q, got %q", tc.wantMsg, err)
			}
		})
	}
}
