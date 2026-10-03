package apis

// Fork-only: the test lives in package apis (not apis_test) because it
// exercises the unexported serveHTTPRedirect. It builds the app with
// core.NewBaseApp instead of tests.NewTestApp: the tests package imports
// apis, so importing it here would be a cycle.

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestServeHTTPRedirectUsesTheInjectedListener(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	SetRedirectListener(app, l)
	go serveHTTPRedirect(app, "203.0.113.1:80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "redirect")
	}))
	client := &http.Client{Timeout: 5 * time.Second}
	var res *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if res, err = client.Get("http://" + l.Addr().String() + "/"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "redirect" {
		t.Fatalf("body = %q", body)
	}
}
