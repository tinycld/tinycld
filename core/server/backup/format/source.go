package format

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var (
	ErrSourceExpired = errors.New("backup: source URL expired")
	ErrNoResume      = errors.New("backup: source does not support resume")
)

// NoRedirectClient never follows a redirect: a presigned URL that redirects
// would leak its signature to the next host.
func NoRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// RangeSource reads a URL as one continuous stream and transparently resumes
// with Range requests after a dropped connection. When the URL has expired
// (401/403 on a retry) Read blocks until SwapURL provides a fresh one or the
// expiry wait elapses.
type RangeSource struct {
	ctx        context.Context
	client     *http.Client
	mu         sync.Mutex
	url        string
	swapped    chan struct{}
	body       io.ReadCloser
	offset     int64
	etag       string
	retries    int
	expiryWait time.Duration
	blocked    bool
}

const maxRetries = 8

func NewRangeSource(ctx context.Context, url string) *RangeSource {
	return &RangeSource{ctx: ctx, client: NoRedirectClient(), url: url, swapped: make(chan struct{}, 1), expiryWait: 15 * time.Minute}
}

func (s *RangeSource) SetExpiryWait(d time.Duration) { s.expiryWait = d }

func (s *RangeSource) Offset() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offset
}

// Blocked reports whether Read is waiting for SwapURL after an expired URL.
func (s *RangeSource) Blocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocked
}

func (s *RangeSource) SwapURL(url string) {
	s.mu.Lock()
	s.url = url
	s.mu.Unlock()
	select {
	case s.swapped <- struct{}{}:
	default:
	}
}

func (s *RangeSource) Read(p []byte) (int, error) {
	for {
		if s.body == nil {
			if err := s.open(); err != nil {
				return 0, err
			}
		}
		n, err := s.body.Read(p)
		s.mu.Lock()
		s.offset += int64(n)
		s.mu.Unlock()
		if err == nil || err == io.EOF {
			return n, err
		}
		// Connection dropped mid-body: close and let the next loop reopen.
		_ = s.body.Close()
		s.body = nil
		s.retries++
		if s.retries > maxRetries {
			return n, fmt.Errorf("backup: source gave up after %d retries: %w", maxRetries, err)
		}
		if n > 0 {
			return n, nil
		}
		time.Sleep(backoff(s.retries))
	}
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt-1)) * 250 * time.Millisecond
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	return d
}

func (s *RangeSource) open() error {
	for {
		s.mu.Lock()
		url, offset, etag := s.url, s.offset, s.etag
		s.mu.Unlock()
		req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		if offset > 0 {
			req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
			if etag != "" {
				req.Header.Set("If-Range", etag)
			}
		}
		res, err := s.client.Do(req)
		if err != nil {
			return err
		}
		switch {
		case offset == 0 && res.StatusCode == http.StatusOK:
			s.mu.Lock()
			s.etag = res.Header.Get("ETag")
			s.mu.Unlock()
			s.body = res.Body
			return nil
		case offset > 0 && res.StatusCode == http.StatusPartialContent:
			s.body = res.Body
			return nil
		case offset > 0 && res.StatusCode == http.StatusOK:
			_ = res.Body.Close()
			return ErrNoResume
		case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
			_ = res.Body.Close()
			if err := s.waitForSwap(); err != nil {
				return err
			}
			continue
		case res.StatusCode >= 300 && res.StatusCode < 400:
			_ = res.Body.Close()
			return fmt.Errorf("backup: source redirected (%d); redirects are not followed", res.StatusCode)
		default:
			_ = res.Body.Close()
			return fmt.Errorf("backup: source returned %d", res.StatusCode)
		}
	}
}

func (s *RangeSource) waitForSwap() error {
	s.mu.Lock()
	s.blocked = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.blocked = false
		s.mu.Unlock()
	}()
	select {
	case <-s.swapped:
		return nil
	case <-time.After(s.expiryWait):
		return ErrSourceExpired
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *RangeSource) Close() error {
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}
