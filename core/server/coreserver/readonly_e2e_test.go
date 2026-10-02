package coreserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"tinycld.org/core/readonly"
)

// sseEvent is one message read off a realtime stream.
type sseEvent struct {
	name string
	data string
}

// The whole server, not only the middleware: during a pause a record create
// is refused, a list read is served, and an SSE stream opened before the pause
// stays open and carries the first event written after it.
//
// A real listener is needed because the stream must stay open across several
// other requests; tests.ApiScenario serves one request into a recorder.
func TestReadOnlyE2E(t *testing.T) {
	t.Cleanup(readonly.Leave)

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	registerSharedMiddleware(app)

	notes := core.NewBaseCollection("notes")
	notes.Fields.Add(&core.TextField{Name: "body"})
	notes.CreateRule = types.Pointer("")
	notes.ListRule = types.Pointer("")
	if err := app.Save(notes); err != nil {
		t.Fatal(err)
	}

	srv := serveOnListener(t, app)
	defer srv.Close()

	// Cancelled before srv.Close runs (defers are LIFO): Close waits for active
	// requests, and the SSE handler returns only when its request ends.
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	events := openRealtime(streamCtx, t, srv.URL)

	connect := nextSSEEvent(t, events)
	if connect.name != "PB_CONNECT" {
		t.Fatalf("first event = %q, want PB_CONNECT", connect.name)
	}
	var connectData struct {
		ClientID string `json:"clientId"`
	}
	if err := json.Unmarshal([]byte(connect.data), &connectData); err != nil || connectData.ClientID == "" {
		t.Fatalf("PB_CONNECT data %q: %v", connect.data, err)
	}

	readonly.Enter()

	sub, _ := json.Marshal(map[string]any{"clientId": connectData.ClientID, "subscriptions": []string{"notes"}})
	res, body := doRequest(t, srv.URL, http.MethodPost, "/api/realtime", string(sub))
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("subscribe during pause: status %d, body %s", res.StatusCode, body)
	}

	res, body = doRequest(t, srv.URL, http.MethodPost, "/api/collections/notes/records", `{"body":"x"}`)
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("create during pause: status %d, want 503, body %s", res.StatusCode, body)
	}
	if got := res.Header.Get("Retry-After"); got != "2" {
		t.Fatalf("create during pause: Retry-After %q, want 2", got)
	}
	if !strings.Contains(body, `"code":"read_only"`) {
		t.Fatalf("create during pause: body %s, want code read_only", body)
	}

	res, body = doRequest(t, srv.URL, http.MethodGet, "/api/collections/notes/records", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list during pause: status %d, body %s", res.StatusCode, body)
	}

	readonly.Leave()

	res, body = doRequest(t, srv.URL, http.MethodPost, "/api/collections/notes/records", `{"body":"x"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create after pause: status %d, body %s", res.StatusCode, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.ID == "" {
		t.Fatalf("create after pause: body %s: %v", body, err)
	}

	// The first event on the stream must be this record: the refused create
	// wrote nothing, so it must not have produced an event either.
	ev := nextSSEEvent(t, events)
	if ev.name != "notes" {
		t.Fatalf("event after pause = %q, want notes", ev.name)
	}
	var msg struct {
		Action string `json:"action"`
		Record struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"record"`
	}
	if err := json.Unmarshal([]byte(ev.data), &msg); err != nil {
		t.Fatalf("notes event data %q: %v", ev.data, err)
	}
	if msg.Action != "create" || msg.Record.ID != created.ID || msg.Record.Body != "x" {
		t.Fatalf("notes event = %+v, want create of %s with body x", msg, created.ID)
	}
}

// serveOnListener serves app's full router, with every OnServe hook applied, on a
// real local listener.
func serveOnListener(t *testing.T, app *tests.TestApp) *httptest.Server {
	t.Helper()
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	se := &core.ServeEvent{App: app, Router: router}
	var mux http.Handler
	err = app.OnServe().Trigger(se, func(e *core.ServeEvent) error {
		var buildErr error
		mux, buildErr = e.Router.BuildMux()
		return buildErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(mux)
}

// openRealtime opens GET /api/realtime and parses the stream on its own
// goroutine. The channel closes when the stream ends, so a reader never
// blocks on a dead stream.
func openRealtime(ctx context.Context, t *testing.T, baseURL string) <-chan sseEvent {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/realtime", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		t.Fatalf("open realtime: status %d", res.StatusCode)
	}

	events := make(chan sseEvent)
	go func() {
		defer close(events)
		defer res.Body.Close()
		scanner := bufio.NewScanner(res.Body)
		var ev sseEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				select {
				case events <- ev:
				case <-ctx.Done():
					return
				}
				ev = sseEvent{}
			case strings.HasPrefix(line, "event:"):
				ev.name = strings.TrimPrefix(line, "event:")
			case strings.HasPrefix(line, "data:"):
				ev.data = strings.TrimPrefix(line, "data:")
			}
		}
	}()
	return events
}

func nextSSEEvent(t *testing.T, events <-chan sseEvent) sseEvent {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("realtime stream closed")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no realtime event within 5s")
	}
	return sseEvent{}
}

func doRequest(t *testing.T, baseURL, method, path, body string) (*http.Response, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res, string(raw)
}
