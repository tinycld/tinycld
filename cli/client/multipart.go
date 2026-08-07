package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
)

// FilePart is one file in a multipart request. It is path-backed (not an
// io.Reader) so the body can be rebuilt from scratch when Do retries after a
// mid-flight 401 refresh — GetBody reopens the file.
type FilePart struct {
	Field string // multipart field name
	Name  string // filename presented to the server
	Path  string // local file to read
}

// ProgressFunc receives cumulative file bytes written and the precomputed
// total. It is called from the request-body goroutine.
type ProgressFunc func(written, total int64)

// CreateRecordMultipart creates a record with plain fields plus file uploads,
// streaming the files rather than buffering them. Field order is
// deterministic (sorted) so tests can assert the exact body.
func CreateRecordMultipart[T any](ctx context.Context, c *Client, collection string, fields map[string]string, files []FilePart, progress ProgressFunc) (T, error) {
	var out T
	err := postMultipart(ctx, c, recordsPath(collection), func(mw *multipart.Writer, count *countingSink) error {
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := mw.WriteField(k, fields[k]); err != nil {
				return err
			}
		}
		return writeFileParts(mw, files, count, progress)
	}, &out)
	return out, err
}

// PostMultipart sends one JSON document under jsonField plus file uploads —
// the shape endpoints like /api/mail/send accept.
func PostMultipart[T any](ctx context.Context, c *Client, path, jsonField string, jsonBody any, files []FilePart, progress ProgressFunc) (T, error) {
	var out T
	data, err := json.Marshal(jsonBody)
	if err != nil {
		return out, err
	}
	err = postMultipart(ctx, c, path, func(mw *multipart.Writer, count *countingSink) error {
		if err := mw.WriteField(jsonField, string(data)); err != nil {
			return err
		}
		return writeFileParts(mw, files, count, progress)
	}, &out)
	return out, err
}

func writeFileParts(mw *multipart.Writer, files []FilePart, count *countingSink, progress ProgressFunc) error {
	var total int64
	if progress != nil {
		for _, f := range files {
			info, err := os.Stat(f.Path)
			if err != nil {
				return fmt.Errorf("stat %s: %w", f.Path, err)
			}
			total += info.Size()
		}
		count.total = total
		count.progress = progress
	}
	for _, f := range files {
		part, err := mw.CreateFormFile(f.Field, f.Name)
		if err != nil {
			return err
		}
		src, err := os.Open(f.Path)
		if err != nil {
			return err
		}
		_, err = io.Copy(io.MultiWriter(part, count), src)
		src.Close()
		if err != nil {
			return fmt.Errorf("reading %s: %w", f.Path, err)
		}
	}
	return nil
}

// countingSink tallies file bytes (only — field boilerplate is excluded) and
// reports them to the progress callback.
type countingSink struct {
	written  int64
	total    int64
	progress ProgressFunc
}

func (s *countingSink) Write(p []byte) (int, error) {
	s.written += int64(len(p))
	if s.progress != nil {
		s.progress(s.written, s.total)
	}
	return len(p), nil
}

// postMultipart streams a multipart POST through an io.Pipe. The boundary is
// fixed up front so a GetBody rebuild (the 401-refresh retry) produces a body
// matching the already-sent Content-Type header.
func postMultipart(ctx context.Context, c *Client, path string, build func(*multipart.Writer, *countingSink) error, out any) error {
	if err := c.ensure(); err != nil {
		return err
	}
	var raw [15]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}
	boundary := "tinycld" + hex.EncodeToString(raw[:])

	makeBody := func() io.ReadCloser {
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		if err := mw.SetBoundary(boundary); err != nil {
			pw.CloseWithError(err)
			return pr
		}
		go func() {
			err := build(mw, &countingSink{})
			if err == nil {
				err = mw.Close()
			}
			pw.CloseWithError(err)
		}()
		return pr
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, makeBody())
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.GetBody = func() (io.ReadCloser, error) {
		return makeBody(), nil
	}
	return c.doJSON(req, out)
}
