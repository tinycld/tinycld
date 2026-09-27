package format

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
)

// signedURL points at a closed port, so every transport fails at dial — the
// path that produces a *url.Error carrying the whole URL.
const signedURL = "https://127.0.0.1:1/secret?X-Amz-Signature=DEADBEEF"

// assertRedacted is the whole point of redactURLError: the host survives so an
// operator can tell where the transfer went, and nothing else does.
func assertRedacted(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "127.0.0.1") {
		t.Errorf("error should name the host, got %q", msg)
	}
	for _, leak := range []string{"secret", "X-Amz", "DEADBEEF", "?"} {
		if strings.Contains(msg, leak) {
			t.Errorf("error leaks %q: %q", leak, msg)
		}
	}
}

func TestRangeSourceDoesNotLeakTheSignedURL(t *testing.T) {
	src := NewRangeSource(context.Background(), signedURL)
	_, err := io.ReadAll(src)
	assertRedacted(t, err)
}

func TestPutSinkDoesNotLeakTheSignedURL(t *testing.T) {
	sink := NewPutSink(context.Background(), signedURL)
	// The first Write may succeed (the pipe is buffered by the reader) or fail
	// once the request goroutine has closed the pipe; either way Close reports.
	_, _ = sink.Write([]byte("hello"))
	assertRedacted(t, sink.Close())
}

// A redacted error still has to be matchable, or a caller's errors.Is on the
// transport's own error silently stops working.
func TestRedactURLErrorKeepsTheInnerError(t *testing.T) {
	inner := errors.New("connection refused")
	got := redactURLError(&url.Error{Op: "Put", URL: signedURL, Err: inner})
	if !errors.Is(got, inner) {
		t.Fatalf("inner error not reachable: %v", got)
	}
	assertRedacted(t, got)
}

func TestRedactURLErrorPassesOtherErrorsThrough(t *testing.T) {
	inner := errors.New("plain")
	if got := redactURLError(inner); got != inner {
		t.Fatalf("want the same error back, got %v", got)
	}
}

// A URL too malformed to parse must not be echoed: it could still carry a
// signature.
func TestRedactURLErrorHidesAnUnparseableURL(t *testing.T) {
	got := redactURLError(&url.Error{Op: "Get", URL: "ht tp://x?X-Amz-Signature=DEADBEEF", Err: errors.New("bad")})
	if strings.Contains(got.Error(), "DEADBEEF") {
		t.Fatalf("leaked the signature: %v", got)
	}
}
