package format

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type putSink struct {
	pw    *io.PipeWriter
	done  chan error
	guard *stallGuard

	// cause is the transport's own reason, recorded before the pipe is closed.
	// http.Client closes the request body itself when it gives up, so a Write
	// racing that close gets "io: read/write on closed pipe" — true, and useless
	// on a ledger row. Write reports cause instead when there is one.
	mu    sync.Mutex
	cause error
}

// NewPutSink streams everything written to it as the body of one HTTP PUT.
// Close waits for the response and reports a non-2xx as an error.
func NewPutSink(ctx context.Context, url string) io.WriteCloser {
	pr, pw := io.Pipe()
	// A target that accepts the connection and then never reads the body is
	// indistinguishable from a slow one at the socket level. The guard cancels
	// the request when no bytes have moved for StallDeadline, which is what
	// releases the transfer goroutine and the installjob interlock behind it.
	guarded, guard := newStallGuard(ctx)
	s := &putSink{pw: pw, done: make(chan error, 1), guard: guard}
	go func() {
		fail := func(err error) {
			err = guard.classify(err)
			s.setCause(err)
			_ = pr.CloseWithError(err)
			s.done <- err
		}
		req, err := http.NewRequestWithContext(guarded, http.MethodPut, url, pr)
		if err != nil {
			// Redacted here, at the failure point: this error carries the whole
			// presigned URL, and CloseWithError hands it to whoever is writing
			// into the sink — the engine, which puts it in the ledger row.
			fail(redactURLError(err))
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		res, err := NoRedirectClient().Do(req)
		if err != nil {
			fail(redactURLError(err))
			return
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			fail(fmt.Errorf("backup: target returned %d", res.StatusCode))
			return
		}
		s.done <- nil
	}()
	return s
}

func (s *putSink) setCause(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cause == nil {
		s.cause = err
	}
}

func (s *putSink) recordedCause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cause
}

func (s *putSink) Write(p []byte) (int, error) {
	n, err := s.pw.Write(p)
	if n > 0 {
		// The pipe is unbuffered, so a returned Write means the transport read
		// those bytes: real progress, not just bytes handed over.
		s.guard.progressed()
	}
	if err != nil {
		if cause := s.recordedCause(); cause != nil {
			return n, cause
		}
		// http.Client closes the pipe reader the moment it gives up, which can
		// beat the transport goroutine's own record of why. The guard knows
		// whether the cancellation was a stall.
		return n, s.guard.classify(err)
	}
	return n, err
}

func (s *putSink) Close() error {
	// The guard is stopped only AFTER the response is in: the PUT is still in
	// flight while the body is being flushed, and a target that goes quiet in
	// that window is exactly the case the deadline exists for.
	defer s.guard.stop()
	if err := s.pw.Close(); err != nil {
		return s.guard.classify(err)
	}
	return s.guard.classify(<-s.done)
}
