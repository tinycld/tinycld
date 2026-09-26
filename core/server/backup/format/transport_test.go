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

// repeatedDropServer serves payload with Range support and drops the
// connection after chunk bytes on every request until the remaining body
// fits in one chunk, forcing more than maxRetries reconnects over the whole
// transfer. The retry budget must therefore reset on progress, not accumulate
// across the whole transfer.
func repeatedDropServer(t *testing.T, chunk int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		start := 0
		if rh := r.Header.Get("Range"); rh != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rh, "bytes="), "-"))
			w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(len(payload)-1)+"/"+strconv.Itoa(len(payload)))
			w.WriteHeader(http.StatusPartialContent)
		}
		body := payload[start:]
		if chunk < len(body) {
			_, _ = w.Write(body[:chunk])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, _ := hj.Hijack()
				_ = conn.Close()
			}
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRangeSourceResetsRetryBudgetOnProgress(t *testing.T) {
	// 64 KiB payload dropped every 6 KiB needs > 8 reconnects (maxRetries),
	// so this only succeeds if the retry counter resets on progress instead
	// of being a lifetime budget for the whole transfer.
	srv := repeatedDropServer(t, 6*1024)
	src := NewRangeSource(context.Background(), srv.URL)
	got, err := io.ReadAll(src)
	if err != nil {
		t.Fatalf("expected transfer to succeed despite >maxRetries drops, got: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: %d vs %d bytes", len(got), len(payload))
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

// shortStallDeadline shortens the idle-progress deadline for one test.
func shortStallDeadline(t *testing.T, d time.Duration) {
	t.Helper()
	prev := StallDeadline
	StallDeadline = d
	t.Cleanup(func() { StallDeadline = prev })
}

// A target that accepts the connection and then never reads the body is the
// worst case: at the socket level it is indistinguishable from a slow one, so
// without a progress deadline the transfer goroutine parks forever and takes the
// installjob interlock with it.
func TestPutSinkGivesUpOnAStalledTarget(t *testing.T) {
	shortStallDeadline(t, 150*time.Millisecond)
	accepted := make(chan struct{})
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(accepted)
		<-block // never read the body, never answer
	}))
	// LIFO: the handler is released BEFORE the server is closed, or Close waits
	// on a handler that is waiting on the channel.
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	sink := NewPutSink(context.Background(), srv.URL+"/x")
	var err error
	for i := 0; i < 64 && err == nil; i++ {
		_, err = sink.Write(payload)
	}
	if err == nil {
		err = sink.Close()
	} else {
		_ = sink.Close()
	}
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("want ErrStalled, got %v", err)
	}
	<-accepted
}

// A source that sends its headers and then stops sending bytes parks the reader
// inside the transport, where no retry budget can reach it.
func TestRangeSourceGivesUpOnAStalledSource(t *testing.T) {
	shortStallDeadline(t, 150*time.Millisecond)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload[:16])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-block
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	src := NewRangeSource(context.Background(), srv.URL+"/x")
	t.Cleanup(func() { _ = src.Close() })
	_, err := io.ReadAll(src)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("want ErrStalled, got %v", err)
	}
}

// A graceful stop must reach a transfer that is already in flight.
func TestCancelAllEndsATransferInFlight(t *testing.T) {
	t.Cleanup(ResetShutdownForTesting)
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	SetShutdown(context.Background())
	sink := NewPutSink(context.Background(), srv.URL+"/x")
	done := make(chan error, 1)
	go func() {
		var err error
		for i := 0; i < 64 && err == nil; i++ {
			_, err = sink.Write(payload)
		}
		if err == nil {
			err = sink.Close()
		} else {
			_ = sink.Close()
		}
		done <- err
	}()
	CancelAll()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled transfer must report an error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("CancelAll did not release the transfer")
	}
}

// The transport's budgets are what stop a peer that never answers from holding a
// connection open indefinitely.
func TestNoRedirectClientHasPhaseTimeouts(t *testing.T) {
	tr, ok := NoRedirectClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("the client must carry its own transport")
	}
	if tr.ResponseHeaderTimeout == 0 || tr.TLSHandshakeTimeout == 0 || tr.ExpectContinueTimeout == 0 {
		t.Fatalf("a phase timeout is unset: %+v", tr)
	}
	if tr.DialContext == nil {
		t.Fatal("the dialer must have its own timeout")
	}
	// No overall Timeout: a backup of a large organization legitimately runs for
	// hours and must not be cut off for taking them.
	if NoRedirectClient().Timeout != 0 {
		t.Fatal("an overall client timeout would cut a long transfer off")
	}
}

// fired() is the reliable signal the loop uses, and it is worth pinning directly:
// the errors.Is spelling it replaced was silently dead.
func TestStallGuardReportsThatItFired(t *testing.T) {
	shortStallDeadline(t, 20*time.Millisecond)
	_, g := newStallGuard(context.Background())
	t.Cleanup(g.stop)
	if g.fired() {
		t.Fatal("a fresh guard has not fired")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !g.fired() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !g.fired() {
		t.Fatal("the guard never reported firing")
	}
}
