package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"tinycld.org/cli/client"
	"tinycld.org/cli/internal/config"
)

type fakeReq struct {
	Method, Path string
	Query        url.Values
	Body         map[string]any
}

type fakeFailure struct {
	status int
	body   string
}

// fakePB is an in-memory stand-in for the PocketBase records API, the
// userinfo endpoint, the hosts route, the retry route and stored files. It
// runs no access rules: the server's own tests prove those.
type fakePB struct {
	t        *testing.T
	srv      *httptest.Server
	rows     map[string][]map[string]any
	files    map[string][]byte
	hosts    string
	fail     map[string]fakeFailure
	requests []fakeReq
	nextID   int
}

func newFakePB(t *testing.T) *fakePB {
	t.Helper()
	f := &fakePB{t: t, rows: map[string][]map[string]any{}, files: map[string][]byte{}, fail: map[string]fakeFailure{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePB) put(collection string, rows ...map[string]any) {
	f.rows[collection] = append(f.rows[collection], rows...)
}

func (f *fakePB) failNext(method, collection string, status int, body string) {
	f.fail[method+" "+collection] = fakeFailure{status, body}
}

// deps wires a CLI to this server with a stored, valid credential.
func (f *fakePB) deps(t *testing.T) *deps {
	t.Helper()
	d, store := testDeps(t)
	d.httpClient = f.srv.Client()
	host := strings.TrimPrefix(f.srv.URL, "http://")
	cfg := &config.Config{Current: host, Contexts: map[string]config.Context{host: {Origin: f.srv.URL}}}
	if err := cfg.Save(d.configDir); err != nil {
		t.Fatal(err)
	}
	tok, _ := json.Marshal(client.TokenSet{AccessToken: "access-1", ExpiresAt: time.Now().Add(time.Hour)})
	if err := store.Set(host, string(tok)); err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *fakePB) last(method, prefix string) fakeReq {
	f.t.Helper()
	for i := len(f.requests) - 1; i >= 0; i-- {
		r := f.requests[i]
		if r.Method == method && strings.HasPrefix(r.Path, prefix) {
			return r
		}
	}
	f.t.Fatalf("no %s %s request; got %+v", method, prefix, f.requests)
	return fakeReq{}
}

func (f *fakePB) serve(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	f.requests = append(f.requests, fakeReq{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: body})
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/oauth/userinfo":
		_, _ = io.WriteString(w, `{"sub":"me123"}`)
		return
	case r.URL.Path == "/api/forms/hosts":
		if f.hosts == "" {
			http.Error(w, `{"status":404,"message":"Not found."}`, 404)
			return
		}
		_, _ = io.WriteString(w, f.hosts)
		return
	case strings.HasPrefix(r.URL.Path, "/api/automation/rules/"):
		_, _ = io.WriteString(w, `{"ok":true}`)
		return
	case strings.HasPrefix(r.URL.Path, "/api/files/"):
		b, ok := f.files[r.URL.Path]
		if !ok {
			http.Error(w, `{"status":404}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(b)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/collections/"), "/")
	if len(parts) < 2 || parts[1] != "records" {
		f.t.Fatalf("fake: unexpected path %s", r.URL.Path)
	}
	col := parts[0]
	if fail, ok := f.fail[r.Method+" "+col]; ok {
		delete(f.fail, r.Method+" "+col)
		w.WriteHeader(fail.status)
		_, _ = io.WriteString(w, fail.body)
		return
	}
	id := ""
	if len(parts) > 2 {
		id = parts[2]
	}
	switch {
	case r.Method == http.MethodGet && id == "":
		f.list(w, col, r.URL.Query())
	case r.Method == http.MethodGet:
		row := f.find(col, id)
		if row == nil {
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"status":404,"message":"The requested resource wasn't found."}`)
			return
		}
		_ = json.NewEncoder(w).Encode(row)
	case r.Method == http.MethodPost:
		f.nextID++
		row := map[string]any{"id": fmt.Sprintf("new%d", f.nextID)}
		for k, v := range body {
			row[k] = v
		}
		if col == "form_publications" {
			row["token"] = "tok" + strconv.Itoa(f.nextID)
		}
		f.rows[col] = append(f.rows[col], row)
		_ = json.NewEncoder(w).Encode(row)
	case r.Method == http.MethodPatch:
		row := f.find(col, id)
		for k, v := range body {
			row[k] = v
		}
		if col == "form_publications" && body["token"] == "" {
			row["token"] = "regenerated"
		}
		_ = json.NewEncoder(w).Encode(row)
	case r.Method == http.MethodDelete:
		kept := f.rows[col][:0]
		for _, row := range f.rows[col] {
			if row["id"] != id {
				kept = append(kept, row)
			}
		}
		f.rows[col] = kept
		w.WriteHeader(204)
	}
}

func (f *fakePB) find(col, id string) map[string]any {
	for _, row := range f.rows[col] {
		if row["id"] == id {
			return row
		}
	}
	return nil
}

func (f *fakePB) list(w http.ResponseWriter, col string, q url.Values) {
	items := []map[string]any{}
	for _, row := range f.rows[col] {
		if f.matches(row, q.Get("filter")) {
			items = append(items, row)
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"page": 1, "perPage": len(items), "totalItems": len(items), "items": items})
}

// matches evaluates the few filter shapes the CLI sends: terms joined by
// " && ", alternatives joined by " || ", each `field = "v"`, `field >= "v"`
// or `field = true`. Any other shape fails the test, so a new filter must
// be added here on purpose.
func (f *fakePB) matches(row map[string]any, filter string) bool {
	if filter == "" {
		return true
	}
	for _, alt := range strings.Split(filter, " || ") {
		ok := true
		for _, term := range strings.Split(strings.Trim(alt, "() "), " && ") {
			term = strings.Trim(term, "() ")
			op := " = "
			if strings.Contains(term, " >= ") {
				op = " >= "
			}
			field, raw, found := strings.Cut(term, op)
			if !found {
				f.t.Fatalf("fake: unsupported filter %q", filter)
			}
			want := raw
			if unq, err := strconv.Unquote(raw); err == nil {
				want = unq
			}
			got := fmt.Sprint(row[field])
			if op == " = " && got != want || op == " >= " && got < want {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
