package reports

import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func withHeading(t *testing.T, heading string, watch ...string) {
	t.Helper()
	src := *lookupRegistered("zoo.feedings")
	src.Heading = func(core.App) string { return heading }
	src.Watch = watch
	RegisterSource(src)
}

func TestARunPrintsItsSourcesHeadingAndWatchesWhatItReads(t *testing.T) {
	z := newZoo(t)
	withHeading(t, "  Zoo Ltd  ", "zoo_sites")
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	if res.Heading != "Zoo Ltd" {
		t.Fatalf("heading = %q", res.Heading)
	}
	if !slices.Contains(res.Topics, "collection/zoo_sites") {
		t.Fatalf("topics = %v", res.Topics)
	}
	other := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[{"id":"n","path":[{"field":"name"}]}]}`, Options{}, nil)
	if other.Heading != "" || slices.Contains(other.Topics, "collection/zoo_sites") {
		t.Fatalf("animals: heading %q, topics %v", other.Heading, other.Topics)
	}
}

func TestCheckSourcesRefusesAWatchedCollectionItCannotSignal(t *testing.T) {
	z := newZoo(t)
	withHeading(t, "", "zoo_nothing")
	if err := CheckSources(z.app); err == nil || !strings.Contains(err.Error(), `watched collection "zoo_nothing" does not exist`) {
		t.Fatalf("err = %v", err)
	}
	withHeading(t, "", "_superusers")
	if err := CheckSources(z.app); err == nil || !strings.Contains(err.Error(), "is a system collection") {
		t.Fatalf("err = %v", err)
	}
}
