package apis_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
	"github.com/pocketbase/pocketbase/tools/types"
)

const resumeIP = "10.0.0.1"

type sseEvent struct {
	id    string
	event string
	data  string
}

// sseStream is one realtime connection, read the way an EventSource reads
// it.
type sseStream struct {
	events chan sseEvent
	cancel context.CancelFunc
	ended  chan struct{}
}

type resumeEnv struct {
	t     *testing.T
	app   *tests.TestApp
	srv   *httptest.Server
	token string
}

type connectData struct {
	ClientId string `json:"clientId"`
	Resumed  *bool  `json:"resumed"`
}

type resumeRecordEvent struct {
	Seq    uint64         `json:"seq"`
	Action string         `json:"action"`
	Record map[string]any `json:"record"`
}

func newResumeEnv(t *testing.T) *resumeEnv {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	// lets each request choose its client IP
	app.Settings().TrustedProxy.Headers = []string{"X-Forwarded-For"}

	collection := core.NewBaseCollection("test_resume")
	collection.ListRule = types.Pointer(`@request.auth.id != ""`)
	collection.ViewRule = types.Pointer(`@request.auth.id != ""`)
	collection.Fields.Add(&core.TextField{Name: "title"})
	if err := app.Save(collection); err != nil {
		t.Fatal(err)
	}

	mux, err := apis.BuildServeMux(app, apis.ServeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &resumeEnv{t: t, app: app, srv: srv, token: userToken(t, app, "test@example.com")}
}

func userToken(t *testing.T, app core.App, email string) string {
	t.Helper()

	user, err := app.FindAuthRecordByEmail("users", email)
	if err != nil {
		t.Fatal(err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	return token
}

func (env *resumeEnv) open(query string, token string, ip string) *sseStream {
	env.t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.srv.URL+"/api/realtime"+query, nil)
	if err != nil {
		env.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	req.Header.Set("X-Forwarded-For", ip)

	res, err := env.srv.Client().Do(req)
	if err != nil {
		cancel()
		env.t.Fatal(err)
	}

	stream := &sseStream{events: make(chan sseEvent, 100), cancel: cancel, ended: make(chan struct{})}
	env.t.Cleanup(cancel)

	go func() {
		defer close(stream.ended)
		defer res.Body.Close()

		scanner := bufio.NewScanner(res.Body)
		var current sseEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				stream.events <- current
				current = sseEvent{}
			case strings.HasPrefix(line, "id:"):
				current.id = strings.TrimPrefix(line, "id:")
			case strings.HasPrefix(line, "event:"):
				current.event = strings.TrimPrefix(line, "event:")
			case strings.HasPrefix(line, "data:"):
				current.data = strings.TrimPrefix(line, "data:")
			}
		}
	}()

	return stream
}

// connect opens a connection and returns its PB_CONNECT payload.
func (env *resumeEnv) connect(query string, token string, ip string) (*sseStream, connectData) {
	env.t.Helper()

	stream := env.open(query, token, ip)
	event := stream.next(env.t)
	if event.event != "PB_CONNECT" {
		env.t.Fatalf("expected PB_CONNECT first, got %q", event.event)
	}

	var data connectData
	if err := json.Unmarshal([]byte(event.data), &data); err != nil {
		env.t.Fatal(err)
	}
	if event.id != data.ClientId {
		env.t.Fatalf("expected the SSE id to be the client id %q, got %q", data.ClientId, event.id)
	}

	return stream, data
}

func (env *resumeEnv) subscribe(clientId string, token string, topics ...string) {
	env.t.Helper()

	body, _ := json.Marshal(map[string]any{"clientId": clientId, "subscriptions": topics})
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/api/realtime", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", resumeIP)
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	res, err := env.srv.Client().Do(req)
	if err != nil {
		env.t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		env.t.Fatalf("subscriptions POST returned %d", res.StatusCode)
	}
}

func (env *resumeEnv) create(title string) *core.Record {
	env.t.Helper()

	collection, err := env.app.FindCollectionByNameOrId("test_resume")
	if err != nil {
		env.t.Fatal(err)
	}
	record := core.NewRecord(collection)
	record.Set("title", title)
	if err := env.app.Save(record); err != nil {
		env.t.Fatal(err)
	}

	return record
}

func (env *resumeEnv) resumable(clientId string) *subscriptions.ResumableClient {
	env.t.Helper()

	client, err := env.app.SubscriptionsBroker().ClientById(clientId)
	if err != nil {
		return nil
	}
	resumable, _ := client.(*subscriptions.ResumableClient)

	return resumable
}

// waitDetached waits until the server has noticed that the connection
// ended. A write made before that would go to the dead connection, which
// is the half-open case and not what these tests are about.
func (env *resumeEnv) waitDetached(clientId string) {
	env.t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client := env.resumable(clientId)
		if client == nil {
			env.t.Fatalf("client %q is not registered", clientId)
		}
		if !client.Attached() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	env.t.Fatalf("client %q is still attached", clientId)
}

// waitUnregistered waits until the server has removed the client.
func (env *resumeEnv) waitUnregistered(clientId string) {
	env.t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := env.app.SubscriptionsBroker().ClientById(clientId); err != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	env.t.Fatalf("client %q is still registered", clientId)
}

// detachedClient connects, subscribes, receives one event (seq 1) and
// drops the connection. It returns the client id.
func (env *resumeEnv) detachedClient(topics ...string) string {
	env.t.Helper()

	if len(topics) == 0 {
		topics = []string{"test_resume"}
	}

	stream, data := env.connect("", env.token, resumeIP)
	env.subscribe(data.ClientId, env.token, topics...)

	env.create("first")
	if seq := stream.nextRecord(env.t).Seq; seq != 1 {
		env.t.Fatalf("expected seq 1, got %d", seq)
	}

	stream.cancel()
	env.waitDetached(data.ClientId)

	return data.ClientId
}

func (s *sseStream) next(t *testing.T) sseEvent {
	t.Helper()

	select {
	case event := <-s.events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
	}

	return sseEvent{}
}

func (s *sseStream) nextRecord(t *testing.T) resumeRecordEvent {
	t.Helper()

	event := s.next(t)

	var data resumeRecordEvent
	if err := json.Unmarshal([]byte(event.data), &data); err != nil {
		t.Fatalf("event %q: %v", event.data, err)
	}

	return data
}

func (s *sseStream) expectNothing(t *testing.T) {
	t.Helper()

	select {
	case event := <-s.events:
		t.Fatalf("expected no event, got %s %s", event.event, event.data)
	case <-time.After(100 * time.Millisecond):
	}
}

func resumeQuery(clientId string, after string) string {
	query := url.Values{"resume": {clientId}}
	if after != "" {
		query.Set("after", after)
	}

	return "?" + query.Encode()
}

func expectResumed(t *testing.T, data connectData, clientId string) {
	t.Helper()

	if data.ClientId != clientId || data.Resumed == nil || !*data.Resumed {
		t.Fatalf("expected a resume of %q, got %+v", clientId, data)
	}
}

func expectNotResumed(t *testing.T, data connectData, clientId string) {
	t.Helper()

	if data.ClientId == clientId || data.Resumed != nil {
		t.Fatalf("expected a new client without resumed, got %+v", data)
	}
}

func TestRealtimeResumeReplaysGap(t *testing.T) {
	env := newResumeEnv(t)
	clientId := env.detachedClient()

	second := env.create("second")
	third := env.create("third")

	stream, data := env.connect(resumeQuery(clientId, "1"), env.token, resumeIP)
	expectResumed(t, data, clientId)

	for _, want := range []struct {
		seq uint64
		id  string
	}{{2, second.Id}, {3, third.Id}} {
		event := stream.nextRecord(t)
		if event.Seq != want.seq || event.Action != "create" || event.Record["id"] != want.id {
			t.Fatalf("expected create of %s with seq %d, got %+v", want.id, want.seq, event)
		}
	}

	// the stream goes on with the same numbering and the same topics
	env.create("fourth")
	if seq := stream.nextRecord(t).Seq; seq != 4 {
		t.Fatalf("expected seq 4, got %d", seq)
	}
}

func TestRealtimeResumeReplaysLeaveAndDelete(t *testing.T) {
	env := newResumeEnv(t)
	clientId := env.detachedClient(leaveTopic("test_resume", `title = "first"`))

	record, err := env.app.FindFirstRecordByData("test_resume", "title", "first")
	if err != nil {
		t.Fatal(err)
	}
	other := env.create("first")

	// leaves the filtered subscription
	record.Set("title", "renamed")
	if err := env.app.Save(record); err != nil {
		t.Fatal(err)
	}
	// deleted while it matches
	if err := env.app.Delete(other); err != nil {
		t.Fatal(err)
	}

	stream, data := env.connect(resumeQuery(clientId, "1"), env.token, resumeIP)
	expectResumed(t, data, clientId)

	expected := []struct {
		action string
		id     string
	}{{"create", other.Id}, {"delete", record.Id}, {"delete", other.Id}}
	for i, want := range expected {
		event := stream.nextRecord(t)
		if event.Seq != uint64(i+2) || event.Action != want.action || event.Record["id"] != want.id {
			t.Fatalf("expected %s of %s with seq %d, got %+v", want.action, want.id, i+2, event)
		}
	}
	stream.expectNothing(t)
}

func TestRealtimeResumeRefused(t *testing.T) {
	scenarios := []struct {
		name  string
		query func(clientId string) string
		token func(env *resumeEnv) string
		ip    string
	}{
		{
			name:  "another user",
			query: func(id string) string { return resumeQuery(id, "1") },
			token: func(env *resumeEnv) string { return userToken(t, env.app, "test2@example.com") },
		},
		{
			name:  "no auth",
			query: func(id string) string { return resumeQuery(id, "1") },
			token: func(env *resumeEnv) string { return "" },
		},
		{
			name:  "another IP",
			query: func(id string) string { return resumeQuery(id, "1") },
			ip:    "10.0.0.2",
		},
		{
			name:  "a missed event that is no longer queued",
			query: func(id string) string { return resumeQuery(id, "0") },
		},
		{
			name:  "an event that was never sent",
			query: func(id string) string { return resumeQuery(id, "2") },
		},
		{
			name:  "an invalid after",
			query: func(id string) string { return resumeQuery(id, "x") },
		},
		{
			name:  "an unknown client",
			query: func(id string) string { return resumeQuery("unknown", "1") },
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			env := newResumeEnv(t)
			clientId := env.detachedClient()

			token := env.token
			if s.token != nil {
				token = s.token(env)
			}
			ip := resumeIP
			if s.ip != "" {
				ip = s.ip
			}

			_, data := env.connect(s.query(clientId), token, ip)
			expectNotResumed(t, data, clientId)
		})
	}
}

func TestRealtimeResumeRefusedClientCannotResumeLater(t *testing.T) {
	env := newResumeEnv(t)
	clientId := env.detachedClient()

	_, data := env.connect(resumeQuery(clientId, "0"), env.token, resumeIP)
	expectNotResumed(t, data, clientId)

	if env.resumable(clientId) != nil {
		t.Fatal("expected the refused client to be unregistered")
	}
}

func TestRealtimeResumeNeedsSubscriptionsPost(t *testing.T) {
	env := newResumeEnv(t)

	stream, data := env.connect("", env.token, resumeIP)
	stream.cancel()
	env.waitUnregistered(data.ClientId)

	_, resumed := env.connect(resumeQuery(data.ClientId, ""), env.token, resumeIP)
	expectNotResumed(t, resumed, data.ClientId)
}

func TestRealtimeResumeNotForGuests(t *testing.T) {
	env := newResumeEnv(t)

	stream, data := env.connect("", "", resumeIP)
	env.subscribe(data.ClientId, "", "test_resume")
	stream.cancel()
	env.waitUnregistered(data.ClientId)

	_, resumed := env.connect(resumeQuery(data.ClientId, ""), "", resumeIP)
	expectNotResumed(t, resumed, data.ClientId)
}

func TestRealtimeResumeGraceExpiry(t *testing.T) {
	restore := apis.SetRealtimeResumeLimits(50*time.Millisecond, 1000)
	defer restore()

	env := newResumeEnv(t)
	clientId := env.detachedClient()
	env.waitUnregistered(clientId)

	_, data := env.connect(resumeQuery(clientId, "1"), env.token, resumeIP)
	expectNotResumed(t, data, clientId)
}

func TestRealtimeResumeOverflow(t *testing.T) {
	restore := apis.SetRealtimeResumeLimits(time.Hour, 2)
	defer restore()

	env := newResumeEnv(t)
	clientId := env.detachedClient()

	env.create("second")
	env.create("third")
	env.create("fourth")
	env.waitUnregistered(clientId)

	_, data := env.connect(resumeQuery(clientId, "1"), env.token, resumeIP)
	expectNotResumed(t, data, clientId)
}

func TestRealtimeResumeAfterUnregister(t *testing.T) {
	env := newResumeEnv(t)
	clientId := env.detachedClient()

	// what core does to every client when the process drains
	env.app.SubscriptionsBroker().Unregister(clientId)

	_, data := env.connect(resumeQuery(clientId, "1"), env.token, resumeIP)
	expectNotResumed(t, data, clientId)
}

func TestRealtimeResumeTakesOverOpenStream(t *testing.T) {
	env := newResumeEnv(t)

	old, data := env.connect("", env.token, resumeIP)
	env.subscribe(data.ClientId, env.token, "test_resume")
	env.create("first")
	if seq := old.nextRecord(t).Seq; seq != 1 {
		t.Fatalf("expected seq 1, got %d", seq)
	}

	// the old stream is half-open: the server still thinks it is attached
	stream, resumed := env.connect(resumeQuery(data.ClientId, "1"), env.token, resumeIP)
	expectResumed(t, resumed, data.ClientId)

	select {
	case <-old.ended:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the old stream to end")
	}

	env.create("second")
	if seq := stream.nextRecord(t).Seq; seq != 2 {
		t.Fatalf("expected seq 2, got %d", seq)
	}
}
