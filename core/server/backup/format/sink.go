package format

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type putSink struct {
	pw   *io.PipeWriter
	done chan error
}

// NewPutSink streams everything written to it as the body of one HTTP PUT.
// Close waits for the response and reports a non-2xx as an error.
func NewPutSink(ctx context.Context, url string) io.WriteCloser {
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, pr)
		if err != nil {
			_ = pr.CloseWithError(err)
			done <- err
			return
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		res, err := NoRedirectClient().Do(req)
		if err != nil {
			_ = pr.CloseWithError(err)
			done <- err
			return
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			err = fmt.Errorf("backup: target returned %d", res.StatusCode)
			_ = pr.CloseWithError(err)
			done <- err
			return
		}
		done <- nil
	}()
	return &putSink{pw: pw, done: done}
}

func (s *putSink) Write(p []byte) (int, error) { return s.pw.Write(p) }

func (s *putSink) Close() error {
	if err := s.pw.Close(); err != nil {
		return err
	}
	return <-s.done
}
