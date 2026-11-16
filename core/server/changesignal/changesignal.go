package changesignal

import (
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/routine"
	"github.com/pocketbase/pocketbase/tools/subscriptions"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("changesignal")

// Window is how long a topic collects writes before its one message goes
// out. The window starts at the first write, so a steady stream of writes
// still sends one message per window.
const Window = 200 * time.Millisecond

const storeKey = "tinycld.changesignal"

// An empty JSON object, not empty bytes: pbtsdb drops an event whose data is
// not JSON, and a resumable realtime client adds its "seq" only to a JSON
// object.
const payload = "{}"

type signaler struct {
	app    core.App
	window time.Duration
	// afterFunc runs f once d has passed. Tests replace it to fire windows
	// on demand instead of on the wall clock.
	afterFunc func(d time.Duration, f func())
	mu        sync.Mutex
	pending   map[string]bool
}

// Register binds the change signal to app's record writes. PocketBase runs
// the AfterSuccess model hooks after the transaction commits, so a
// rolled-back write sends nothing.
func Register(app core.App) {
	register(app, Window)
}

func register(app core.App, window time.Duration) *signaler {
	s := &signaler{
		app:       app,
		window:    window,
		afterFunc: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		pending:   map[string]bool{},
	}
	// The store is shared with every transaction app cloned from app, which
	// is how Touch finds the signaler from inside a transaction.
	app.Store().Set(storeKey, s)
	onWrite := func(e *core.ModelEvent) error {
		if name := collectionOf(e.App, e.Model); name != "" {
			s.changed(name)
		}
		return e.Next()
	}
	app.OnModelAfterCreateSuccess().BindFunc(onWrite)
	app.OnModelAfterUpdateSuccess().BindFunc(onWrite)
	app.OnModelAfterDeleteSuccess().BindFunc(onWrite)
	return s
}

// collectionOf names the collection a written model belongs to, or "" for a
// model that is not a record (a collection schema, a log row) or belongs to a
// system collection. Any signed-in client may subscribe to any topic, so a
// system collection's topic would tell it when others sign in or use a
// one-time password (_otps, _mfas, _authOrigins, _externalAuths). No report
// or view reads a system collection, so nothing is lost.
func collectionOf(app core.App, model core.Model) string {
	var col *core.Collection
	switch m := model.(type) {
	case *core.Record:
		col = m.Collection()
	case core.RecordProxy:
		col = m.ProxyRecord().Collection()
	default:
		if model.TableName() == core.LogsTableName {
			return ""
		}
		found, err := app.FindCachedCollectionByNameOrId(model.TableName())
		if err != nil {
			return ""
		}
		col = found
	}
	if col.System {
		return ""
	}
	return col.Name
}

func (s *signaler) changed(collection string) {
	for _, topic := range topicsFor(collection) {
		s.schedule(topic)
	}
}

func (s *signaler) schedule(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[topic] {
		return
	}
	s.pending[topic] = true
	s.afterFunc(s.window, func() {
		s.mu.Lock()
		delete(s.pending, topic)
		s.mu.Unlock()
		s.send(topic)
	})
}

// send delivers the message to every signed-in client subscribed to topic.
// A default client's Send blocks until its connection reads the message, so
// each send runs on its own goroutine.
func (s *signaler) send(topic string) {
	for _, client := range s.app.SubscriptionsBroker().Clients() {
		if !client.HasSubscription(topic) || !isSignedIn(client) {
			continue
		}
		routine.FireAndForget(func() {
			client.Send(subscriptions.Message{Name: topic, Data: []byte(payload)})
		})
	}
}

// isSignedIn reports whether the client's subscribe request carried an
// auth record. The payload is empty, but the timing of writes is still
// information, and no signed-out screen reads a live report.
func isSignedIn(client subscriptions.Client) bool {
	auth, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
	return auth != nil
}

// Touch signals collection after a write that model hooks did not see: a
// raw app.DB() statement, a bulk INSERT … SELECT, a migration. Pass the
// app that ran the write; inside a transaction the signal waits for the
// commit and is dropped on a rollback. An unknown or system collection
// logs a warning and sends nothing, as a model write to a system
// collection sends nothing.
func Touch(app core.App, collection string) {
	s, _ := app.Store().Get(storeKey).(*signaler)
	if s == nil {
		return
	}
	col, err := app.FindCachedCollectionByNameOrId(collection)
	if err != nil || col.System {
		log.Warn("touch names no signalable collection", "collection", collection)
		return
	}
	name := col.Name
	if tx := app.TxInfo(); tx != nil {
		tx.OnComplete(func(txErr error) error {
			if txErr == nil {
				s.changed(name)
			}
			return nil
		})
		return
	}
	s.changed(name)
}
