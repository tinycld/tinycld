package changesignal

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const testWindow = 50 * time.Millisecond

func TestCreateUpdateDeleteEachSignal(t *testing.T) {
	app := newZooApp(t, testWindow)
	l := listen(t, app, "collection/zoo_animals", true)

	rec := saveAnimal(t, app, "Leo")
	if got := l.collect(4 * testWindow); len(got) != 1 {
		t.Fatalf("create: %d messages, want 1", len(got))
	}
	rec.Set("name", "Leona")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	if got := l.collect(4 * testWindow); len(got) != 1 {
		t.Fatalf("update: %d messages, want 1", len(got))
	}
	if err := app.Delete(rec); err != nil {
		t.Fatal(err)
	}
	if got := l.collect(4 * testWindow); len(got) != 1 {
		t.Fatalf("delete: %d messages, want 1", len(got))
	}
}

func TestPayloadCarriesNoData(t *testing.T) {
	app := newZooApp(t, testWindow)
	l := listen(t, app, "collection/zoo_animals", true)

	rec := saveAnimal(t, app, "Leo")
	got := l.collect(4 * testWindow)
	if len(got) != 1 {
		t.Fatalf("%d messages, want 1", len(got))
	}
	if got[0].Name != "collection/zoo_animals" {
		t.Fatalf("name = %q", got[0].Name)
	}
	// The resumable client adds its "seq" to a JSON object payload; nothing
	// else may be there.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(got[0].Data, &fields); err != nil {
		t.Fatalf("payload %q is not a JSON object: %v", got[0].Data, err)
	}
	delete(fields, "seq")
	if len(fields) != 0 {
		t.Fatalf("payload carries more than seq: %q", got[0].Data)
	}
	if strings.Contains(string(got[0].Data), rec.Id) || strings.Contains(string(got[0].Data), "Leo") {
		t.Fatalf("payload leaks record data: %q", got[0].Data)
	}
}

func TestRolledBackTransactionSendsNothing(t *testing.T) {
	app := newZooApp(t, testWindow)
	l := listen(t, app, "collection/zoo_animals", true)

	err := app.RunInTransaction(func(tx core.App) error {
		saveAnimal(t, tx, "Ghost")
		return errors.New("roll back")
	})
	if err == nil {
		t.Fatal("transaction did not fail")
	}
	if got := l.collect(4 * testWindow); len(got) != 0 {
		t.Fatalf("rolled-back write sent %d messages", len(got))
	}
}

func TestSignalWaitsForTheCommit(t *testing.T) {
	app := newZooApp(t, testWindow)
	l := listen(t, app, "collection/zoo_animals", true)

	var duringTx int
	err := app.RunInTransaction(func(tx core.App) error {
		saveAnimal(t, tx, "Slow")
		duringTx = len(l.collect(3 * testWindow))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if duringTx != 0 {
		t.Fatalf("%d messages before the commit", duringTx)
	}
	if got := l.collect(4 * testWindow); len(got) != 1 {
		t.Fatalf("after commit: %d messages, want 1", len(got))
	}
}

func TestBatchInOneTransactionSendsOneMessage(t *testing.T) {
	app := newZooApp(t, testWindow)
	l := listen(t, app, "collection/zoo_animals", true)

	err := app.RunInTransaction(func(tx core.App) error {
		for range 20 {
			saveAnimal(t, tx, "Herd")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := l.collect(4 * testWindow); len(got) != 1 {
		t.Fatalf("%d messages, want 1", len(got))
	}
}

func TestSeparateWritesInOneWindowSendOneMessage(t *testing.T) {
	// A long window makes "inside one window" certain on a slow machine.
	app := newZooApp(t, time.Second)
	l := listen(t, app, "collection/zoo_animals", true)

	for range 5 {
		saveAnimal(t, app, "Burst")
	}
	if got := l.collect(1500 * time.Millisecond); len(got) != 1 {
		t.Fatalf("burst: %d messages, want 1", len(got))
	}
	saveAnimal(t, app, "Later")
	if got := l.collect(1500 * time.Millisecond); len(got) != 1 {
		t.Fatalf("next window: %d messages, want 1", len(got))
	}
}

// The windows fire on demand, not on the wall clock: with real timers a slow
// runner stretched the writes past a fixed collect period and the test saw
// one message where the signaler had sent one per window.
func TestSteadyStreamIsNotStarved(t *testing.T) {
	app := newZooApp(t, testWindow)
	timers := manualTimers(t, app)
	l := listen(t, app, "collection/zoo_animals", true)

	for window := range 3 {
		// Writes that keep coming inside a window must not push its end out:
		// the first one arms the window and the rest join it.
		for range 4 {
			saveAnimal(t, app, "Stream")
		}
		fire := timers.armed(t, 1)
		fire()
		if got := l.next(t); got.Name != "collection/zoo_animals" {
			t.Fatalf("window %d: message for %q", window, got.Name)
		}
	}
	if got := l.collect(testWindow); len(got) != 0 {
		t.Fatalf("%d extra messages after three windows", len(got))
	}
}

func TestSkipsUnauthenticatedAndUnsubscribedClients(t *testing.T) {
	app := newZooApp(t, testWindow)
	signedOut := listen(t, app, "collection/zoo_animals", false)
	signedIn := listen(t, app, "collection/zoo_animals", true)
	elsewhere := listen(t, app, "collection/zoo_keepers", true)

	saveAnimal(t, app, "Leo")
	if got := signedOut.collect(4 * testWindow); len(got) != 0 {
		t.Fatalf("signed-out client got %d messages", len(got))
	}
	if got := elsewhere.collect(testWindow); len(got) != 0 {
		t.Fatalf("client of another topic got %d messages", len(got))
	}
	// The control proves the write did signal, so the silences above mean
	// something.
	if got := signedIn.collect(testWindow); len(got) != 1 {
		t.Fatalf("signed-in client got %d messages, want 1", len(got))
	}
}

func TestViewTopicGetsItsSourcesWrites(t *testing.T) {
	app := newZooApp(t, testWindow)
	RegisterView("zoo_fed", "zoo_animals", "zoo_feedings")
	view := listen(t, app, "collection/zoo_fed", true)

	saveAnimal(t, app, "Leo")
	if got := view.collect(4 * testWindow); len(got) != 1 {
		t.Fatalf("view topic: %d messages, want 1", len(got))
	}
}

func TestCollectionSchemaSaveSendsNothing(t *testing.T) {
	app := newZooApp(t, testWindow)
	l := listen(t, app, "collection/zoo_animals", true)

	col, err := app.FindCollectionByNameOrId("zoo_animals")
	if err != nil {
		t.Fatal(err)
	}
	col.Fields.Add(&core.TextField{Name: "note"})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	if got := l.collect(4 * testWindow); len(got) != 0 {
		t.Fatalf("schema change sent %d messages", len(got))
	}
}

func TestSystemCollectionWriteSendsNothing(t *testing.T) {
	app := newZooApp(t, testWindow)
	l := listen(t, app, "collection/_otps", true)

	col, err := app.FindCachedCollectionByNameOrId(core.CollectionNameOTPs)
	if err != nil {
		t.Fatal(err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("zoo@example.com")
	user.SetPassword("zoo-password-1")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	otp := core.NewRecord(col)
	otp.Set("collectionRef", users.Id)
	otp.Set("recordRef", user.Id)
	otp.Set("password", "hash")
	if err := app.Save(otp); err != nil {
		t.Fatalf("save otp: %v", err)
	}
	if got := l.collect(4 * testWindow); len(got) != 0 {
		t.Fatalf("system collection write sent %d messages", len(got))
	}
}
