package exitcode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tinycld.org/cli/client"
)

func TestOf(t *testing.T) {
	inner := errors.New("boom")
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, OK},
		{"plain", inner, Failed},
		{"coded", New(Usage, inner), Usage},
		{"coded and wrapped", fmt.Errorf("acct1: %w", New(NotFound, inner)), NotFound},
	} {
		if got := Of(tc.err); got != tc.want {
			t.Errorf("%s: Of = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestNewKeepsNilAndUnwraps(t *testing.T) {
	if New(Usage, nil) != nil {
		t.Fatal("New(code, nil) must stay nil: a command returns it as its result")
	}
	inner := errors.New("boom")
	err := New(Refused, inner)
	if !errors.Is(err, inner) {
		t.Fatal("Unwrap did not reach the inner error")
	}
	if err.Error() != "boom" {
		t.Fatalf("Error() = %q, want the inner text unchanged", err.Error())
	}
}

func TestErrorWithNoInnerErrorDoesNotPanic(t *testing.T) {
	err := &Error{Code: Usage}
	if got := err.Error(); got != "usage" {
		t.Fatalf("Error() = %q, want the code name", got)
	}
	if err.Unwrap() != nil {
		t.Fatal("Unwrap of an Error with no inner error must be nil")
	}
}

// A cancelled call is the caller's own doing (Ctrl-C), not a server that is
// down: retrying it is wrong, so it keeps Failed.
func TestClassifyCancelledIsFailed(t *testing.T) {
	for _, err := range []error{context.Canceled, fmt.Errorf("list: %w", context.Canceled)} {
		if got := Of(Classify(err)); got != Failed {
			t.Errorf("%v: code = %d, want Failed", err, got)
		}
	}
}

func TestClassifyByHTTPStatus(t *testing.T) {
	for status, want := range map[int]int{
		400: Refused, 401: Auth, 403: Forbidden, 404: NotFound, 408: Unavailable,
		409: Refused, 413: Refused, 422: Refused, 429: Unavailable,
		500: Unavailable, 502: Unavailable, 503: Unavailable,
	} {
		err := Classify(fmt.Errorf("acct1: %w", &client.APIError{Status: status}))
		if got := Of(err); got != want {
			t.Errorf("HTTP %d: code = %d (%s), want %d (%s)", status, got, Name(got), want, Name(want))
		}
	}
}

func TestClassifyKeepsACodeAlreadySet(t *testing.T) {
	err := Classify(New(Usage, &client.APIError{Status: 404}))
	if got := Of(err); got != Usage {
		t.Fatalf("code = %d, want Usage: the command's own code wins", got)
	}
	if Classify(nil) != nil {
		t.Fatal("Classify(nil) must stay nil")
	}
}

func TestClassifyAuthExpiredAndPlain(t *testing.T) {
	if got := Of(Classify(fmt.Errorf("list: %w", client.ErrAuthExpired))); got != Auth {
		t.Errorf("ErrAuthExpired: code = %d, want Auth", got)
	}
	if got := Of(Classify(errors.New("unexpected end of JSON input"))); got != Failed {
		t.Errorf("plain error: code = %d, want Failed", got)
	}
	if got := Of(Classify(context.DeadlineExceeded)); got != Unavailable {
		t.Errorf("deadline: code = %d, want Unavailable", got)
	}
}

// A server that is not there is the case an agent retries: the real client
// error of a refused connection must classify as Unavailable.
func TestClassifyAServerThatDoesNotAnswer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	origin := srv.URL
	srv.Close()
	store := &memStore{tok: client.TokenSet{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}}
	c := client.New(origin, store, &http.Client{Timeout: 2 * time.Second})
	err := c.GetJSON(context.Background(), "/api/health", nil)
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if got := Of(Classify(err)); got != Unavailable {
		t.Fatalf("code = %d (%s), want Unavailable; err: %v", got, Name(got), err)
	}
}

func TestNames(t *testing.T) {
	want := map[int]string{
		OK: "ok", Failed: "failed", Usage: "usage", Auth: "auth", Forbidden: "forbidden",
		NotFound: "not_found", Refused: "refused", Unavailable: "unavailable", 99: "failed",
	}
	for code, name := range want {
		if got := Name(code); got != name {
			t.Errorf("Name(%d) = %q, want %q", code, got, name)
		}
	}
}

// The numbers are public: a script tests `$? -eq 6`.
func TestCodesAreStable(t *testing.T) {
	got := []int{OK, Failed, Usage, Auth, Forbidden, NotFound, Refused, Unavailable}
	for want, code := range got {
		if code != want {
			t.Errorf("code at position %d = %d: the numbers must never change", want, code)
		}
	}
}

type memStore struct{ tok client.TokenSet }

func (s *memStore) Load() (client.TokenSet, error) { return s.tok, nil }
func (s *memStore) Save(t client.TokenSet) error   { s.tok = t; return nil }
