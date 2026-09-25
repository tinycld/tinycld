package format

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type putSink struct {
	pw   *io.PipeWriter
	done chan error

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
	s := &putSink{pw: pw, done: make(chan error, 1)}
	go func() {
		fail := func(err error) {
			s.setCause(err)
			_ = pr.CloseWithError(err)
			s.done <- err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, pr)
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
	if err != nil {
		if cause := s.recordedCause(); cause != nil {
			return n, cause
		}
	}
	return n, err
}

func (s *putSink) Close() error {
	if err := s.pw.Close(); err != nil {
		return err
	}
	return <-s.done
}
