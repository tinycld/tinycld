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
	body := s.expect(t, "ann", http.MethodPost, runURL, savedRun(id), http.StatusOK, savedTopics(id, s.id("ann")))
	if saysManual(body) {
		t.Fatalf("a live shipped report ran manual\n%s", body)
	}
	// The same report, shipped without Live.
	ship(false)
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(id), http.StatusOK, `"manual":true`, savedOnlyTopics(id, s.id("ann")))
	ship(true)
	// Its owner changes the spec of the live shipped record: it is no longer
	// the report the package shipped.
	s.expect(t, "ad", http.MethodPost, runURL, savedRun(id), http.StatusOK, savedTopics(id, s.id("ad")))
	s.expect(t, "ad", http.MethodPatch, reportsURL+"/"+id, `{"spec":`+feedingNotes+`}`, http.StatusOK)
	s.expect(t, "ad", http.MethodPost, runURL, savedRun(id), http.StatusOK, `"manual":true`, savedOnlyTopics(id, s.id("ad")))
	// A user's saved report with the live report's spec is not shipped.
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK, `"manual":true`, savedOnlyTopics(s.id("R"), s.id("ann")))
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
	// The serverOwned guard (saved.go) drops the field and the update answers 200.
	s.expect(t, "ann", http.MethodPatch, reportsURL+"/"+s.id("R"), `{"shipped_key":"zoo.feed_costs"}`, http.StatusOK)
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
