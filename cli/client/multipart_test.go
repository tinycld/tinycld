package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// parsedUpload is what the test server saw after parsing the multipart body.
type parsedUpload struct {
	fields map[string]string
	files  map[string]struct{ name, content string }
}

func multipartServer(t *testing.T, acceptToken string, got *[]parsedUpload) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+acceptToken {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"message": "expired"})
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		p := parsedUpload{
			fields: map[string]string{},
			files:  map[string]struct{ name, content string }{},
		}
		for k, v := range r.MultipartForm.Value {
			p.fields[k] = v[0]
		}
		for k, headers := range r.MultipartForm.File {
			f, err := headers[0].Open()
			if err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, headers[0].Size)
			f.Read(buf)
			f.Close()
			p.files[k] = struct{ name, content string }{headers[0].Filename, string(buf)}
		}
		*got = append(*got, p)
		json.NewEncoder(w).Encode(testRow{ID: "created", Name: p.fields["name"]})
	})
	// The refresh endpoint must exist for the retry test.
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-2", "token_type": "Bearer",
			"expires_in": 3600, "refresh_token": "refresh-2", "scope": "profile",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateRecordMultipart(t *testing.T) {
	var got []parsedUpload
	srv := multipartServer(t, "access-1", &got)
	c := New(srv.URL, validStore("access-1"), srv.Client())

	path := writeTempFile(t, "report.pdf", "pdf-bytes")
	var progressCalls int
	var lastWritten, lastTotal int64
	out, err := CreateRecordMultipart[testRow](context.Background(), c, "drive_items",
		map[string]string{"name": "report.pdf", "parent": "", "is_folder": "false"},
		[]FilePart{{Field: "file", Name: "report.pdf", Path: path}},
		func(written, total int64) {
			progressCalls++
			lastWritten, lastTotal = written, total
		})
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != "created" {
		t.Fatalf("out = %+v", out)
	}
	if len(got) != 1 {
		t.Fatalf("uploads = %d", len(got))
	}
	up := got[0]
	if up.fields["name"] != "report.pdf" || up.fields["is_folder"] != "false" {
		t.Fatalf("fields = %v", up.fields)
	}
	if f := up.files["file"]; f.name != "report.pdf" || f.content != "pdf-bytes" {
		t.Fatalf("file part = %+v", f)
	}
	if progressCalls == 0 || lastWritten != int64(len("pdf-bytes")) || lastTotal != int64(len("pdf-bytes")) {
		t.Fatalf("progress: calls=%d written=%d total=%d", progressCalls, lastWritten, lastTotal)
	}
}

func TestMultipartRetriesAfter401(t *testing.T) {
	// The stored token is stale; the first attempt 401s, the client refreshes
	// and must send a COMPLETE body again — GetBody reopens the source file.
	var got []parsedUpload
	srv := multipartServer(t, "access-2", &got)
	c := New(srv.URL, validStore("access-1"), srv.Client())

	path := writeTempFile(t, "data.txt", "same-bytes-twice")
	out, err := CreateRecordMultipart[testRow](context.Background(), c, "drive_items",
		map[string]string{"name": "data.txt"},
		[]FilePart{{Field: "file", Name: "data.txt", Path: path}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != "created" {
		t.Fatalf("out = %+v", out)
	}
	if len(got) != 1 {
		t.Fatalf("successful uploads = %d, want exactly 1 (the retry)", len(got))
	}
	if f := got[0].files["file"]; f.content != "same-bytes-twice" {
		t.Fatalf("retried body incomplete: %+v", f)
	}
}

func TestPostMultipartJSONField(t *testing.T) {
	var got []parsedUpload
	srv := multipartServer(t, "access-1", &got)
	c := New(srv.URL, validStore("access-1"), srv.Client())

	attach := writeTempFile(t, "a.txt", "attachment-bytes")
	type sendReq struct {
		Subject string `json:"subject"`
	}
	_, err := PostMultipart[testRow](context.Background(), c, "/api/mail/send",
		"json", sendReq{Subject: "hello"},
		[]FilePart{{Field: "attachments", Name: "a.txt", Path: attach}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("uploads = %d", len(got))
	}
	var decoded sendReq
	if err := json.Unmarshal([]byte(got[0].fields["json"]), &decoded); err != nil || decoded.Subject != "hello" {
		t.Fatalf("json field = %q (%v)", got[0].fields["json"], err)
	}
	if f := got[0].files["attachments"]; f.name != "a.txt" || f.content != "attachment-bytes" {
		t.Fatalf("attachment part = %+v", f)
	}
}
