package format

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var payload = bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KiB

// rangeServer serves payload with Range support and drops the connection
// after dropAfter bytes on the first request only.
func rangeServer(t *testing.T, dropAfter int, supportRange bool) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("ETag", `"v1"`)
		start := 0
		if rh := r.Header.Get("Range"); rh != "" && supportRange {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rh, "bytes="), "-"))
			w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(len(payload)-1)+"/"+strconv.Itoa(len(payload)))
			w.WriteHeader(http.StatusPartialContent)
		}
		body := payload[start:]
		if n == 1 && dropAfter > 0 && dropAfter < len(body) {
			_, _ = w.Write(body[:dropAfter])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				_ = conn.Close()
			}
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestRangeSourceResumesAfterDrop(t *testing.T) {
	srv, calls := rangeServer(t, 10_000, true)
	src := NewRangeSource(context.Background(), srv.URL)
	got, err := io.ReadAll(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: %d vs %d bytes", len(got), len(payload))
	}
	if atomic.LoadInt32(calls) < 2 {
		t.Fatal("expected a resume request")
	}
}

func TestRangeSourceRejectsNoRange(t *testing.T) {
	srv, _ := rangeServer(t, 10_000, false)
	src := NewRangeSource(context.Background(), srv.URL)
	_, err := io.ReadAll(src)
	if !errors.Is(err, ErrNoResume) {
		t.Fatalf("want ErrNoResume, got %v", err)
	}
}

func TestRangeSourceSwapURLOnExpiry(t *testing.T) {
	var calls int32
	good, _ := rangeServer(t, 0, true)
	expired := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write(payload[:5000])
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				_ = c.Close()
			}
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(expired.Close)

	src := NewRangeSource(context.Background(), expired.URL)
	src.SetExpiryWait(2 * time.Second)
	go func() {
		time.Sleep(300 * time.Millisecond)
		src.SwapURL(good.URL)
	}()
	got, err := io.ReadAll(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("payload mismatch after swap")
	}
}

func TestRangeSourceExpiryTimeout(t *testing.T) {
	var calls int32
	expired := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write(payload[:5000])
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				_ = c.Close()
			}
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(expired.Close)
	src := NewRangeSource(context.Background(), expired.URL)
	src.SetExpiryWait(200 * time.Millisecond)
	_, err := io.ReadAll(src)
	if !errors.Is(err, ErrSourceExpired) {
		t.Fatalf("want ErrSourceExpired, got %v", err)
	}
}

func TestNoRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	t.Cleanup(target.Close)
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(redir.Close)
	src := NewRangeSource(context.Background(), redir.URL)
	if _, err := io.ReadAll(src); err == nil {
		t.Fatal("redirect was followed")
	}
}

func TestPutSinkStreams(t *testing.T) {
	var received bytes.Buffer
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		_, _ = io.Copy(&received, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	sink := NewPutSink(context.Background(), srv.URL)
	if _, err := sink.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || !bytes.Equal(received.Bytes(), payload) {
		t.Fatalf("method %s, %d bytes", method, received.Len())
	}
}

func TestPutSinkReportsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	sink := NewPutSink(context.Background(), srv.URL)
	_, _ = sink.Write(payload)
	if err := sink.Close(); err == nil {
		t.Fatal("403 not reported")
	}
}

func TestRangeSourceBlockedDuringExpiryWait(t *testing.T) {
	var calls int32
	good, _ := rangeServer(t, 0, true)
	expired := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write(payload[:5000])
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				_ = c.Close()
			}
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(expired.Close)

	src := NewRangeSource(context.Background(), expired.URL)
	src.SetExpiryWait(2 * time.Second)

	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, len(payload))
		_, _ = io.ReadFull(src, buf)
		close(readDone)
	}()

	// Wait until the source has hit the 403 and entered waitForSwap.
	deadline := time.Now().Add(2 * time.Second)
	for !src.Blocked() {
		if time.Now().After(deadline) {
			t.Fatal("source never became blocked")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !src.Blocked() {
		t.Fatal("expected Blocked() to be true while waiting for swap")
	}

	src.SwapURL(good.URL)

	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
		t.Fatal("read did not complete after swap")
	}
	if src.Blocked() {
		t.Fatal("expected Blocked() to be false after swap")
	}
}
