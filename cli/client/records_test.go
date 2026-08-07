package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

type testRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestListRecordsBuildsQuery(t *testing.T) {
	var gotQuery url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/collections/drive_items/records", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]any{
			"page": 2, "perPage": 50, "totalItems": 120, "totalPages": 3,
			"items": []testRow{{ID: "a", Name: "x"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("t"), srv.Client())

	res, err := ListRecords[testRow](context.Background(), c, "drive_items", ListOptions{
		Filter: `parent = "abc"`, Sort: "-is_folder,name", Fields: "id,name",
		Page: 2, PerPage: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{
		"filter": {`parent = "abc"`}, "sort": {"-is_folder,name"},
		"fields": {"id,name"}, "page": {"2"}, "perPage": {"50"},
	}
	for k, v := range want {
		if gotQuery.Get(k) != v[0] {
			t.Errorf("query %s = %q, want %q", k, gotQuery.Get(k), v[0])
		}
	}
	if gotQuery.Has("skipTotal") {
		t.Error("skipTotal must be absent unless requested")
	}
	if res.TotalItems != 120 || len(res.Items) != 1 || res.Items[0].Name != "x" {
		t.Fatalf("res = %+v", res)
	}
}

func TestListAllPages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/collections/things/records", func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if r.URL.Query().Get("skipTotal") != "1" {
			t.Error("ListAll must skip the COUNT")
		}
		count := 200
		if page == 2 {
			count = 3
		}
		items := make([]testRow, count)
		for i := range items {
			items[i] = testRow{ID: strconv.Itoa((page-1)*200 + i)}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"page": page, "perPage": 200, "totalItems": -1, "totalPages": -1,
			"items": items,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("t"), srv.Client())

	all, err := ListAll[testRow](context.Background(), c, "things", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 203 {
		t.Fatalf("len = %d, want 203", len(all))
	}
}

func TestRecordCRUDPaths(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody = nil
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&gotBody)
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		json.NewEncoder(w).Encode(testRow{ID: "r1", Name: "n"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("t"), srv.Client())
	ctx := context.Background()

	if _, err := GetRecord[testRow](ctx, c, "things", "r1"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "GET" || gotPath != "/api/collections/things/records/r1" {
		t.Fatalf("get: %s %s", gotMethod, gotPath)
	}

	if _, err := CreateRecord[testRow](ctx, c, "things", map[string]any{"name": "n"}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/collections/things/records" || gotBody["name"] != "n" {
		t.Fatalf("create: %s %s %v", gotMethod, gotPath, gotBody)
	}

	if _, err := UpdateRecord[testRow](ctx, c, "things", "r1", map[string]any{"name": "m"}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/collections/things/records/r1" || gotBody["name"] != "m" {
		t.Fatalf("update: %s %s %v", gotMethod, gotPath, gotBody)
	}

	if err := DeleteRecord(ctx, c, "things", "r1"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/collections/things/records/r1" {
		t.Fatalf("delete: %s %s", gotMethod, gotPath)
	}
}

func TestFilterQuoting(t *testing.T) {
	cases := []struct {
		expr   string
		params map[string]any
		want   string
	}{
		{
			`parent = {:p} && name = {:n}`,
			map[string]any{"p": "", "n": `Quo"te\slash`},
			`parent = "" && name = "Quo\"te\\slash"`,
		},
		{
			`trashed = {:t} && size > {:s}`,
			map[string]any{"t": true, "s": int64(1024)},
			`trashed = true && size > 1024`,
		},
		{
			`user = {:u}`,
			map[string]any{"u": "rec123"},
			`user = "rec123"`,
		},
	}
	for _, c := range cases {
		if got := Filter(c.expr, c.params); got != c.want {
			t.Errorf("Filter(%q) = %q, want %q", c.expr, got, c.want)
		}
	}
}

func TestFileURLEscapes(t *testing.T) {
	got := FileURL("mail_messages", "rec 1", "body_ab12cd34ef.html")
	want := "/api/files/mail_messages/rec%201/body_ab12cd34ef.html"
	if got != want {
		t.Fatalf("FileURL = %q, want %q", got, want)
	}
}

func TestUserIDMemoizesUserinfo(t *testing.T) {
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]string{"sub": "user123"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("t"), srv.Client())

	for range 3 {
		id, err := c.UserID(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if id != "user123" {
			t.Fatalf("id = %q", id)
		}
	}
	if calls != 1 {
		t.Fatalf("userinfo calls = %d, want 1 (memoized)", calls)
	}
}
