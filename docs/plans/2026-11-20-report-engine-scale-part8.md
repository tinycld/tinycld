# Report Engine Scale, Part 8: a Report Can Be Not Live (core server) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a package mark a report source as too large to follow, so that a run over it carries no data topics and is marked manual, and let a package keep one shipped report live in spite of that.

**Architecture:** Two declarations, both made by a package: `reports.Source.NotLive` and `reports.Shipped.Live`. The run endpoint reads the saved report's `shipped_key`, sets `Request.Live` from the registered `Shipped.Live`, and the engine sets `Result.Manual` and empties `Result.Topics` when the run read a collection of a `NotLive` source and the request is not live. The endpoint still adds the saved-report topics, so an edit, a share change and lost access still reach an open report.

**Tech Stack:** Go, package `reports` in `tinycld/core/server/reports` (module `tinycld.org/core`), PocketBase test app, the fictional `zoo` package of the tests.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part7.md` (section "Design proposed", items 1 to 3), the outline `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (task 8a), and the human's decision of 2026-11-20 in `/Users/nas/code/tinycld/docs/plans/auto-helix/p5-report-engine-scale.md`.

Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/`, on the branch `books/p5-report-engine-scale`. This part changes core only. It holds ONE task (8a). Tasks 8b (core client: Refresh button and note, core help), 8c (Books declarations, the Go test that pins the list, Books help) and 8d (Books e2e) are the next plan part.

## Global Constraints

- Core names no package: no Books slug, source key or collection name in `tinycld/core`. The tests use the `zoo` fixtures only.
- The human's decision (2026-11-20, copied): "the not-live list is every built-in report that reads all journal lines: General Ledger, Transaction Detail, Trial Balance, Balance Sheet, the Journal report and Profit and Loss by Month, plus any user-built report over journal lines. Profit and Loss and smaller reports stay live." This task builds the mechanism; Books declares the list in task 8c.
- A manual run's `topics` is the JSON `[]`, never `null`: the client reads `result.topics` as an array.
- The JSON key is `manual`, with the tag `json:"manual,omitzero"`. This tree builds with Go 1.27, where `omitempty` does NOT omit a `false` bool (measured: the body held `"manual":false`). A live run's body must not hold the key.
- No backward compatibility work: the product is not live.
- Run only the named Go tests. The driver runs the full checks.

## Review Focus

- A user who writes `shipped_key` on their own report to make it live: the key is server-owned, the write does not land, the run stays manual (`TestAUserCannotClaimALiveShippedKey`, step 1).
- The owner or an editor of a live shipped record who changes its spec (the update rule allows it, and the server puts the shipped spec back only when the revision rises): the changed report is manual (`TestTheRunEndpointMarksAManualRun`, the `ad` PATCH, step 1).
- A report on another source whose list rule reads a collection of a `NotLive` source: the run read that collection, so it is manual (`TestARuleThatReadsANotLiveSourceMakesTheRunManual`, step 1).
- A report that reaches the `NotLive` source only through a link: manual (`TestARunOverANotLiveSourceIsManual`, the case "a link", step 1).
- A copy of a live shipped report, and the builder's preview (a posted draft spec): both manual, because neither is the shipped record (`TestTheRunEndpointMarksAManualRun`, step 1).
- A manual saved report that someone edits, shares or unshares while it is open: the run still carries the topics of `reports` and `report_shares` (`savedOnlyTopics` in `TestTheRunEndpointMarksAManualRun`, step 1).

Not tested, by design: a later page of a manual run is its own `Run` and is manual in the same way; the export read (`export.go`) leaves `Live` false and never reads `Topics`; the collections a `NotLive` source names in `Watch` are dropped with the other topics (a settings change shows at the next Refresh); a collection that a read source names in `Watch` counts as read, so when it belongs to a `NotLive` source the run is manual.

---

### Task 8a: A run over a source that is not live is manual

**Files:**
- Create: `tinycld/core/server/reports/live_test.go`
- Modify: `tinycld/core/server/reports/registry.go` (the type `Source`, its last field `Watch` at line 199)
- Modify: `tinycld/core/server/reports/shipped.go` (the type `Shipped` at line 23; a new function before `isShipped` at line 78)
- Modify: `tinycld/core/server/reports/run.go` (the types `Request` at line 64 and `Result` at line 87; the type `runner`, field `askedPageCap` at line 295; `Run` at line 381; `rn.run()` at lines 556-570)
- Modify: `tinycld/core/server/reports/endpoint.go` (`handleRun` at line 117, `requestSpec` at line 160, `viewableSpec` at line 180)

**Interfaces:**
- Consumes (test helpers that exist in package `reports`): `newZoo(t) *zoo`, `(z *zoo) run(t, user, raw string, opts Options, params map[string]string) *Result`, `lookupRegistered(key string) *Source`, `feedingsByDay` (a spec on `zoo.feedings`; its run's topics are `collection/zoo_animals`, `collection/zoo_feedings`, `collection/zoo_keepers`), `newSavedZoo(t) *savedZoo` (users `ann`, `ad` (admin); saved report `R` owned by `ann` with `savedSpec`), `(s *savedZoo) expect(t, user, method, url, body string, want int, parts ...string) string`, `(s *savedZoo) call(...) (int, string)`, `savedRun(id string) string`, `runBodyOf(spec string) string`, `runURL`, `reportsURL`, `savedTopics`, `savedSpec`, `feedCosts(revision int, title string) Shipped` (key `zoo.feed_costs`), `shippedRow(t, s, key) *core.Record`. `newZoo` resets the source and shipped registries.
- Produces (tasks 8b, 8c and 8d rely on these exact names):
  - `reports.Source.NotLive bool`
  - `reports.Shipped.Live bool`
  - `reports.Request.Live bool`
  - `reports.Result.Manual bool`, JSON `"manual":true`; the key is absent when false. A manual result's `"topics"` is `[]` from `Run`, and `["collection/report_shares","collection/reports"]` from `POST /api/reports/run` with a `reportId`.
  - `shippedLive(rec *core.Record) bool` (unexported, in `shipped.go`): true only when the record's `shipped_key` names a `Shipped` with `Live` AND the record still holds the shipped spec.
  - `requestRun(re *core.RequestEvent, spec json.RawMessage, reportID string) (raw []byte, live bool, err error)` (unexported, in `endpoint.go`); `viewableSpec` is removed.

- [ ] **Step 1: Write the failing tests**

Create `tinycld/core/server/reports/live_test.go`:

```go
package reports

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// notLive marks the zoo's feedings source as one a run does not follow.
func notLive(t *testing.T) {
	t.Helper()
	src := *lookupRegistered("zoo.feedings")
	src.NotLive = true
	RegisterSource(src)
}

const animalNames = `{"version":4,"source":"zoo.animals","columns":[{"id":"n","path":[{"field":"name"}]}]}`

// A report that reaches the feedings only through a link reads them too.
const feedGramsByLink = `{"version":4,"source":"zoo.animals","columns":[{"id":"a","path":[{"field":"name"}]},{"id":"g","path":[{"link":"feedings","mode":"aggregate"},{"field":"grams"}],"aggregate":"sum"}]}`

func TestARunOverANotLiveSourceIsManual(t *testing.T) {
	z := newZoo(t)
	live := z.run(t, "ann", feedingsByDay, Options{}, nil)
	if live.Manual || !slices.Contains(live.Topics, "collection/zoo_feedings") {
		t.Fatalf("before NotLive: manual %v, topics %v", live.Manual, live.Topics)
	}
	notLive(t)
	for name, spec := range map[string]string{"its base": feedingsByDay, "a link": feedGramsByLink} {
		res := z.run(t, "ann", spec, Options{}, nil)
		if !res.Manual {
			t.Fatalf("%s: the run is not manual; topics %v", name, res.Topics)
		}
		// Empty, never nil: the client reads "topics":[].
		if res.Topics == nil || len(res.Topics) != 0 {
			t.Fatalf("%s: a manual run has topics %#v", name, res.Topics)
		}
	}
	// A report that reads no such source stays live.
	other := z.run(t, "ann", animalNames, Options{}, nil)
	if other.Manual || !slices.Contains(other.Topics, "collection/zoo_animals") {
		t.Fatalf("animals: manual %v, topics %v", other.Manual, other.Topics)
	}
}

func TestALiveRequestKeepsItsTopics(t *testing.T) {
	z := newZoo(t)
	want := z.run(t, "ann", feedingsByDay, Options{}, nil).Topics
	notLive(t)
	spec, info, err := Load([]byte(feedingsByDay))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(t.Context(), z.app, Request{Spec: spec, Loaded: info, Auth: z.recs["ann"], Live: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Manual || !slices.Equal(res.Topics, want) {
		t.Fatalf("manual %v, topics %v, want %v", res.Manual, res.Topics, want)
	}
}

const feedingNotes = `{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}]}`

const savedOnlyTopics = `"topics":["collection/report_shares","collection/reports"]`

func saysManual(body string) bool { return strings.Contains(body, `"manual"`) }

// The run endpoint: only a shipped report registered Live stays live. A
// saved report, a copy and a draft over the same source are manual, and a
// saved one keeps the topics that tell of an edit, a share or lost access.
func TestTheRunEndpointMarksAManualRun(t *testing.T) {
	s := newSavedZoo(t)
	notLive(t)
	ship := func(live bool) string {
		t.Helper()
		sh := feedCosts(1, "Feed costs")
		sh.Live = live
		RegisterShipped(sh)
		if err := ReconcileShipped(s.app); err != nil {
			t.Fatal(err)
		}
		return shippedRow(t, s, "zoo.feed_costs").Id
	}
	id := ship(true)
	body := s.expect(t, "ann", http.MethodPost, runURL, savedRun(id), http.StatusOK, savedTopics)
	if saysManual(body) {
		t.Fatalf("a live shipped report ran manual\n%s", body)
	}
	// The same report, shipped without Live.
	ship(false)
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(id), http.StatusOK, `"manual":true`, savedOnlyTopics)
	ship(true)
	// Its owner changes the spec of the live shipped record: it is no longer
	// the report the package shipped.
	s.expect(t, "ad", http.MethodPost, runURL, savedRun(id), http.StatusOK, savedTopics)
	s.expect(t, "ad", http.MethodPatch, reportsURL+"/"+id, `{"spec":`+feedingNotes+`}`, http.StatusOK)
	s.expect(t, "ad", http.MethodPost, runURL, savedRun(id), http.StatusOK, `"manual":true`, savedOnlyTopics)
	// A user's saved report with the live report's spec is not shipped.
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK, `"manual":true`, savedOnlyTopics)
	// A draft: the builder's preview.
	s.expect(t, "ann", http.MethodPost, runURL, runBodyOf(savedSpec), http.StatusOK, `"manual":true`, `"topics":[]`)
	// A report that reads no such source says nothing of manual.
	body = s.expect(t, "ann", http.MethodPost, runURL, runBodyOf(animalNames), http.StatusOK, `"collection/zoo_animals"`)
	if saysManual(body) {
		t.Fatalf("a live draft ran manual\n%s", body)
	}
}

// A user cannot make a report live: shipped_key is the server's.
func TestAUserCannotClaimALiveShippedKey(t *testing.T) {
	s := newSavedZoo(t)
	notLive(t)
	sh := feedCosts(1, "Feed costs")
	sh.Live = true
	RegisterShipped(sh)
	s.call(t, "ann", http.MethodPatch, reportsURL+"/"+s.id("R"), `{"shipped_key":"zoo.feed_costs"}`)
	rec, err := s.app.FindRecordById(reportsCollection, s.id("R"))
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.GetString("shipped_key"); got != "" {
		t.Fatalf("a user set shipped_key to %q", got)
	}
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK, `"manual":true`)
}

// The feedings list rule reads the keepers, so a feedings run reads a
// keepers collection that is not live.
func TestARuleThatReadsANotLiveSourceMakesTheRunManual(t *testing.T) {
	z := newZoo(t)
	src := *lookupRegistered("zoo.keepers")
	src.NotLive = true
	RegisterSource(src)
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	if !res.Manual || len(res.Topics) != 0 {
		t.Fatalf("manual %v, topics %v", res.Manual, res.Topics)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run, from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:

```bash
go vet ./reports/
```

Expected: FAIL, the first line names `live_test.go:14:6: src.NotLive undefined (type Source has no field or method NotLive)`. The package does not build, so no test runs yet.

- [ ] **Step 3: Add the two declarations**

In `registry.go`, in the type `Source`, after the field `Watch []string`:

```go
	Watch []string
	// NotLive marks a source too large to read again on every change. A run
	// that reads one of this source's collections is Manual: it carries no
	// data topics, so it runs again only when the reader asks. A shipped
	// report with Live set is the exception.
	NotLive bool
}
```

In `shipped.go`, in the type `Shipped`, after the field `Unpublished bool`:

```go
	Unpublished bool
	// Live keeps this report live though it reads a source that is NotLive:
	// its runs carry their data topics. Set it only on a report whose run
	// stays fast on the largest data set. A copy of the report, and the
	// shipped record after someone changed its spec, are not live.
	Live bool
}
```

In `shipped.go`, directly before `func isShipped(key string) bool {`:

```go
// shippedLive reports whether the saved report rec is a shipped report that
// is registered Live and still holds the shipped spec. Its owner or an
// editor can change the spec of a shipped record; the changed report is not
// the one the package measured, so it is not live.
func shippedLive(rec *core.Record) bool {
	shippedMu.Lock()
	s, ok := shipped[rec.GetString("shipped_key")]
	shippedMu.Unlock()
	if !ok || !s.Live {
		return false
	}
	_, stored, err := loadStored(s.Spec)
	return err == nil && sameJSON(specBytes(rec), stored)
}
```

`loadStored`, `sameJSON` and `specBytes` exist in `saved.go` (lines 121, 142, 134). `loadStored` gives the bytes `ReconcileShipped` stores; `sameJSON` compares values, because PocketBase encodes a posted spec again. Do not hold `shippedMu` across `loadStored`.

- [ ] **Step 4: Make the engine mark a manual run**

In `run.go`, in the type `Request`, after the field `PageCap int`:

```go
	PageCap int
	// Live keeps the run's data topics though it reads a source that is
	// NotLive. Only the run endpoint sets it, for a shipped report that is
	// registered Live.
	Live bool
}
```

In `run.go`, in the type `Result`, after the field `Topics`:

```go
	Topics  []string       `json:"topics"`
	// Manual is set when the run reads a source that is NotLive and the
	// request is not Live: Topics is then empty, and the reader runs the
	// report again by hand.
	Manual bool `json:"manual,omitzero"`
```

Use `omitzero`, not `omitempty` (see Global Constraints).

In `run.go`, in the type `runner`, after the field `askedPageCap int`:

```go
	askedPageCap int
	// live is the request's Live.
	live bool
```

In `run.go`, in `Run`, where the runner is built, add `live: req.Live` to the first line of the literal:

```go
	rn := &runner{
		ctx: ctx, app: app, c: c, opts: req.Options, expand: expand, askedRowCap: req.RowCap, askedPageCap: req.PageCap, live: req.Live,
```

In `run.go`, at the end of `rn.run()`, the loop over `Sources(rn.app)` (lines 556-565) already finds each source whose collections the run read. Replace that loop and the lines after it, down to the function's last `return nil` and closing brace (lines 556-572), with the code below. The two comment lines above the loop ("A source's Go hooks read collections no statement reads, so their topics are added by name.") stay.

```go
	all := Sources(rn.app)
	reads := func(src Source) bool {
		return slices.ContainsFunc(src.collections(), func(c string) bool {
			return rn.topics[changesignal.Topic(c)]
		})
	}
	for _, src := range all {
		if reads(src) {
			for _, w := range src.Watch {
				rn.topics[changesignal.Topic(w)] = true
			}
		}
	}
	rn.res.Topics = make([]string, 0, len(rn.topics))
	// A manual run follows no data: its topics stay empty. This is decided
	// after every Watch is added, so a watched collection counts as read
	// whatever the order of the sources.
	if !rn.live && slices.ContainsFunc(all, func(src Source) bool { return src.NotLive && reads(src) }) {
		rn.res.Manual = true
		return nil
	}
	for t := range rn.topics {
		rn.res.Topics = append(rn.res.Topics, t)
	}
	slices.Sort(rn.res.Topics)
	return nil
}
```

`Sources` returns `[]Source` sorted by key. The manual decision is its own pass after the Watch pass, so it does not depend on that order.

- [ ] **Step 5: Make the run endpoint ask for a live run**

In `endpoint.go`, in `handleRun`, replace the call of `requestSpec` and pass `Live` to `Run`:

```go
	raw, live, err := requestRun(re, body.Spec, body.ReportID)
	if err != nil {
		return err
	}
	saved := body.ReportID != ""
	spec, loaded, err := Load(raw)
	if err != nil {
		return runError(re, err)
	}
	res, err := Run(re.Request.Context(), re.App, Request{
		Spec: spec, Loaded: loaded, Params: body.Params, Options: body.Options,
		Auth: re.Auth, Headers: ruleHeaders(re.Request.Header), Live: live,
	})
```

The rest of `handleRun` (the `runError`, `withSavedTopics`, `re.JSON`) does not change.

In `endpoint.go`, replace the whole function `requestSpec` (its comment stays) with these two functions. `options.go:641` still calls `requestSpec`; do not change that call.

```go
func requestSpec(re *core.RequestEvent, spec json.RawMessage, reportID string) ([]byte, error) {
	raw, _, err := requestRun(re, spec, reportID)
	return raw, err
}

// requestRun is requestSpec, with whether the run stays live over a source
// that is NotLive: only a saved report that a package ships as Live does. A
// posted draft never does, so the builder's preview of such a report is
// manual too.
func requestRun(re *core.RequestEvent, spec json.RawMessage, reportID string) (raw []byte, live bool, err error) {
	switch {
	case reportID != "" && spec != nil:
		return nil, false, re.BadRequestError("The request has a spec or a reportId, not both.", nil)
	case reportID != "":
		rec, err := viewableReport(re, reportID)
		if err != nil {
			return nil, false, err
		}
		return specBytes(rec), shippedLive(rec), nil
	case spec == nil:
		return nil, false, re.BadRequestError("The request has no spec and no reportId.", nil)
	}
	return []byte(spec), false, nil
}
```

In `endpoint.go`, delete the function `viewableSpec` with its comment (nothing calls it now), and give its comment's content to `viewableReport`. The comment of `viewableReport` becomes:

```go
// viewableReport reads a saved report record for the caller, through the
// reports view rule as PocketBase's view API applies it. A report that does
// not exist answers as one the caller cannot view, so an id tells nobody
// whether a report exists. Any other failure, such as a database error or a
// rule that cannot be built, is a server fault: it is logged and answered
// 500, never as a refusal.
func viewableReport(re *core.RequestEvent, id string) (*core.Record, error) {
```

- [ ] **Step 6: Run the tests to verify they pass**

Run, from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:

```bash
gofmt -l reports/ && go vet ./reports/ && go test ./reports/ -run 'NotLive|TestALiveRequestKeepsItsTopics|TestTheRunEndpointMarksAManualRun|TestAUserCannotClaimALiveShippedKey|TestASaved|TestARunPrintsItsSourcesHeading' -v
```

Expected: `gofmt -l` prints nothing; vet prints nothing; PASS for `TestARunOverANotLiveSourceIsManual`, `TestALiveRequestKeepsItsTopics`, `TestTheRunEndpointMarksAManualRun`, `TestAUserCannotClaimALiveShippedKey`, `TestARuleThatReadsANotLiveSourceMakesTheRunManual`, and for the saved-run and heading tests that existed (`TestASavedReportRunsAsItsReader` proves a run with no `NotLive` source keeps `savedTopics`).

If a body holds `"manual":false`, the tag is `omitempty`: change it to `omitzero`.

- [ ] **Step 7: Commit**

From `/Users/nas/tmp/tinycld/books-purchases/tinycld`:

```bash
git add core/server/reports/live_test.go core/server/reports/registry.go core/server/reports/shipped.go core/server/reports/run.go core/server/reports/endpoint.go
git commit -m "feat(reports): a run over a source that is not live is manual"
```
