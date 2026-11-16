package changesignal

import (
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// newZooApp is a test app with three fictional zoo collections and the
// change signal bound with the given window. zoo_feedings has a covering
// index on (animal, grams), so a query can read it through the index alone.
func newZooApp(t *testing.T, window time.Duration) *tests.TestApp {
	t.Helper()
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)
	for _, name := range []string{"zoo_animals", "zoo_keepers", "zoo_feedings"} {
		col := core.NewBaseCollection(name)
		col.Fields.Add(
			&core.TextField{Name: "name"},
			&core.TextField{Name: "animal"},
			&core.NumberField{Name: "grams"},
		)
		if name == "zoo_feedings" {
			col.AddIndex("idx_zoo_feedings_animal", false, "animal, grams", "")
		}
		if err := app.Save(col); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}
	register(app, window)
	return app
}

type listener struct {
	conn *subscriptions.ResumableConn
}

// listen registers a realtime client subscribed to topic. A signed-in
// client carries an auth record, as the realtime subscribe request sets it.
func listen(t *testing.T, app core.App, topic string, signedIn bool) *listener {
	t.Helper()
	// Production realtime clients are resumable (apis.realtimeNewClient), and
	// their messages reach the subscriber through an attached connection.
	client := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	if signedIn {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			t.Fatal(err)
		}
		client.Set(apis.RealtimeClientAuthKey, core.NewRecord(users))
	}
	client.Subscribe(topic)
	app.SubscriptionsBroker().Register(client)
	conn, ok := client.Attach(0)
	if !ok {
		t.Fatal("a new resumable client refused Attach(0)")
	}
	t.Cleanup(func() { app.SubscriptionsBroker().Unregister(client.Id()) })
	return &listener{conn: conn}
}

// collect returns every message that arrives within d.
func (l *listener) collect(d time.Duration) []subscriptions.Message {
	var got []subscriptions.Message
	deadline := time.After(d)
	for {
		select {
		case msg := <-l.conn.Channel():
			got = append(got, msg)
		case <-deadline:
			return got
		}
	}
}

// next returns the next message. The bound only stops a test whose message
// never comes; a passing run does not wait for it.
func (l *listener) next(t *testing.T) subscriptions.Message {
	t.Helper()
	select {
	case msg := <-l.conn.Channel():
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("no message")
		return subscriptions.Message{}
	}
}

// timerQueue holds the windows the signaler armed and has not yet fired.
type timerQueue struct {
	window time.Duration
	mu     sync.Mutex
	queued []func()
}

// manualTimers makes the app's signaler arm its windows in a queue that the
// test fires, so no assertion depends on how fast the machine runs.
func manualTimers(t *testing.T, app core.App) *timerQueue {
	t.Helper()
	s, _ := app.Store().Get(storeKey).(*signaler)
	if s == nil {
		t.Fatal("app has no signaler")
	}
	q := &timerQueue{window: s.window}
	s.afterFunc = func(d time.Duration, f func()) {
		if d != q.window {
			t.Errorf("armed for %v, want the window %v", d, q.window)
		}
		q.mu.Lock()
		q.queued = append(q.queued, f)
		q.mu.Unlock()
	}
	return q
}

// armed checks that exactly want windows are queued, and returns a func that
// fires them all.
func (q *timerQueue) armed(t *testing.T, want int) func() {
	t.Helper()
	q.mu.Lock()
	queued := q.queued
	q.queued = nil
	q.mu.Unlock()
	if len(queued) != want {
		t.Fatalf("%d windows armed, want %d", len(queued), want)
	}
	return func() {
		for _, f := range queued {
			f()
		}
	}
}

func saveAnimal(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("zoo_animals")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", name)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save animal: %v", err)
	}
	return rec
}
