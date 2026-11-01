package reports

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/rlstest"
)

// Sources are registered once for the process, but each app checks them
// against its own database. A source whose collection exists in one app and
// not in another is refused only in the other: checking a second app must not
// take the source away from the first.
func TestARefusalBelongsToTheAppThatWasChecked(t *testing.T) {
	withZoo := newZoo(t)
	bare := &zoo{app: rlstest.NewAssembledApp(t), recs: map[string]*core.Record{}}
	bare.user(t, "ann")

	if err := CheckSources(withZoo.app); err != nil {
		t.Fatalf("CheckSources on the app with the zoo: %v", err)
	}
	if err := CheckSources(bare.app); err == nil {
		t.Fatal("CheckSources on the app without the zoo refused nothing")
	}

	const sites = `{"version":1,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]}]}`
	if _, err := withZoo.try(t, "ann", sites, Options{}, nil); err != nil {
		t.Fatalf("the app with the zoo lost zoo.sites after another app was checked: %v", err)
	}
	_, err := bare.try(t, "ann", sites, Options{}, nil)
	requireSpecError(t, err, 422, "source", "does not exist")

	// A saved report is marked broken by the refusals of its own app.
	if b := checkSpec(withZoo.app, []byte(sites)); b != nil {
		t.Errorf("a report on zoo.sites is broken on the app with the zoo: %+v", b)
	}
	if b := checkSpec(bare.app, []byte(sites)); b == nil || b.Status != 422 {
		t.Errorf("a report on zoo.sites on the app without the zoo: %+v, want a 422 mark", b)
	}

	if _, bad := Refused(withZoo.app)["zoo.sites"]; bad {
		t.Error("Refused(app with the zoo) names zoo.sites")
	}
	if _, bad := Refused(bare.app)["zoo.sites"]; !bad {
		t.Error("Refused(app without the zoo) does not name zoo.sites")
	}
	if got := len(Sources(withZoo.app)); got != 6 {
		t.Errorf("Sources(app with the zoo) has %d sources, want 6", got)
	}
	if got := Sources(bare.app); len(got) != 1 || got[0].Key != "zoo.people" {
		t.Errorf("Sources(app without the zoo) = %v, want only zoo.people", got)
	}
}
