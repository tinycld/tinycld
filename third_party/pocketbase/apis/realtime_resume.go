package apis

import (
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// Realtime resume.
//
// Stock PocketBase drops a realtime client when its SSE request ends, so a
// reconnect gets a new client id and every event sent during the gap is
// lost. It also ends every connection on a schedule (IdleTimeout and
// MaxTimeout), so gaps are routine, not rare.
//
// Here a connection that ends leaves its client detached in the broker for
// realtimeResumeGrace. Broadcasts keep reaching it, rule-checked at event
// time as for a live connection, and queue up (see
// subscriptions.ResumableClient). Each event payload carries a per-client
// "seq". A reconnect with `?resume=<clientId>&after=<seq>` gets the same
// client back, `"resumed":true` in PB_CONNECT and the queued events, with no
// subscriptions POST.
//
// The SSE request carries the subscriber's Authorization header, so a resume
// is bound to the auth record stored by the client's subscriptions POST (and
// to its IP, as that POST is). A guest client never resumes: it has no auth
// record to bind to. Every refused resume falls back to a new client, which
// the subscriber treats as "reload everything".

// vars, not consts, so that tests can shorten them
var (
	// realtimeResumeGrace is how long a detached client is kept.
	realtimeResumeGrace = time.Hour

	// realtimeResumeMaxMessages and realtimeResumeMaxBytes bound one
	// client's queue; past either the client cannot replay everything.
	realtimeResumeMaxMessages = 1000
	realtimeResumeMaxBytes    = 1 << 20
)

const (
	// realtimeResumeBudgetBytes bounds the queues of all detached clients
	// of one app.
	realtimeResumeBudgetBytes = 64 << 20

	// realtimeSubscribedKey marks a client whose subscriptions POST
	// succeeded. A client without one never resumes: the subscriber did not
	// get its topics confirmed, so it must reload after it reconnects.
	realtimeSubscribedKey = "tinycld.realtimeSubscribed"

	realtimeResumeBudgetStoreKey = "tinycld.realtimeResumeBudget"
	realtimeResumeRequestKey     = "tinycld.realtimeResume"
)

// realtimeResumeState is what a connect request knows about its resume.
type realtimeResumeState struct {
	candidate *subscriptions.ResumableClient
	after     uint64
	resumed   bool
}

// realtimeNewClient creates the client of a new realtime connection.
func realtimeNewClient(e *core.RequestEvent) *subscriptions.ResumableClient {
	app := e.App
	budget := app.Store().GetOrSet(realtimeResumeBudgetStoreKey, func() any {
		return subscriptions.NewQueueBudget(realtimeResumeBudgetBytes)
	}).(*subscriptions.QueueBudget)

	return subscriptions.NewResumableClient(subscriptions.ResumableOptions{
		MaxMessages: realtimeResumeMaxMessages,
		MaxBytes:    realtimeResumeMaxBytes,
		Budget:      budget,
		OnOverflow: func(client *subscriptions.ResumableClient) {
			app.SubscriptionsBroker().Unregister(client.Id())
		},
	})
}

// realtimeConnectClient returns the client of a connect request: the client
// it may resume, or a new one. It does not attach yet, because the connect
// hooks may still reject the request.
func realtimeConnectClient(e *core.RequestEvent) subscriptions.Client {
	query := e.Request.URL.Query()

	clientId := query.Get("resume")
	if clientId == "" {
		return realtimeNewClient(e)
	}

	var after uint64
	if raw := query.Get("after"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return realtimeNewClient(e)
		}
		after = parsed
	}

	existing, err := e.App.SubscriptionsBroker().ClientById(clientId)
	if err != nil {
		return realtimeNewClient(e)
	}

	client, ok := existing.(*subscriptions.ResumableClient)
	if !ok || !realtimeCanResume(e, client) {
		return realtimeNewClient(e)
	}

	e.Set(realtimeResumeRequestKey, &realtimeResumeState{candidate: client, after: after})

	return client
}

// realtimeCanResume reports whether the request may take over the client.
func realtimeCanResume(e *core.RequestEvent, client *subscriptions.ResumableClient) bool {
	if subscribed, _ := client.Get(realtimeSubscribedKey).(bool); !subscribed {
		return false
	}

	clientAuth, _ := client.Get(RealtimeClientAuthKey).(*core.Record)
	if clientAuth == nil || !isSameAuth(clientAuth, e.Auth) {
		return false
	}

	clientIP, _ := client.Get(RealtimeClientIPKey).(string)

	return clientIP == e.RealIP()
}

// realtimeRegisterClient registers the connection's client and attaches the
// connection to it, resuming the candidate when it still can be.
func realtimeRegisterClient(ce *core.RealtimeConnectRequestEvent) {
	broker := ce.App.SubscriptionsBroker()

	state, _ := ce.Get(realtimeResumeRequestKey).(*realtimeResumeState)
	if state != nil && state.candidate == ce.Client {
		conn, ok := state.candidate.Attach(state.after)
		if ok {
			state.resumed = true
			ce.Client = conn
			return
		}

		// the subscriber missed messages that are no longer queued, so the
		// client can never resume again
		broker.Unregister(state.candidate.Id())

		client := realtimeNewClient(ce.RequestEvent)
		client.Set(RealtimeClientIPKey, ce.RealIP())
		ce.Client = client
	}

	client, ok := ce.Client.(*subscriptions.ResumableClient)
	if !ok {
		// a connect hook supplied its own client
		broker.Register(ce.Client)
		return
	}

	broker.Register(client)

	// a new client has sent nothing, so Attach(0) cannot be refused
	conn, _ := client.Attach(0)
	ce.Client = conn
}

// realtimeReleaseClient runs when the connection ends. It keeps a client
// that can resume detached, and unregisters any other.
func realtimeReleaseClient(app core.App, client subscriptions.Client) {
	conn, ok := client.(*subscriptions.ResumableConn)
	if !ok {
		app.SubscriptionsBroker().Unregister(client.Id())
		return
	}

	if !realtimeKeepDetached(conn) {
		app.SubscriptionsBroker().Unregister(conn.Id())
		return
	}

	if conn.Detach() {
		gen := conn.Gen()
		time.AfterFunc(realtimeResumeGrace, func() {
			if conn.Detached(gen) {
				app.SubscriptionsBroker().Unregister(conn.Id())
			}
		})
		return
	}

	// another connection attached since: it owns the client now
	if conn.IsDiscarded() {
		app.SubscriptionsBroker().Unregister(conn.Id())
	}
}

// realtimeKeepDetached reports whether a client whose connection ended
// could be resumed later.
func realtimeKeepDetached(conn *subscriptions.ResumableConn) bool {
	if conn.IsDiscarded() {
		return false
	}

	subscribed, _ := conn.Get(realtimeSubscribedKey).(bool)
	clientAuth, _ := conn.Get(RealtimeClientAuthKey).(*core.Record)

	return subscribed && clientAuth != nil
}

// realtimeConnectData is the PB_CONNECT payload.
func realtimeConnectData(ce *core.RealtimeConnectRequestEvent) []byte {
	data := `{"clientId":"` + ce.Client.Id() + `"`

	if state, _ := ce.Get(realtimeResumeRequestKey).(*realtimeResumeState); state != nil && state.resumed {
		data += `,"resumed":true`
	}

	return []byte(data + `}`)
}

func bindRealtimeResumeEvents(app core.App) {
	app.OnRealtimeSubscribeRequest().BindFunc(func(e *core.RealtimeSubscribeRequestEvent) error {
		err := e.Next()
		if err == nil {
			e.Client.Set(realtimeSubscribedKey, true)
		}

		return err
	})
}
