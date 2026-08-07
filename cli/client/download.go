package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// DownloadToFile streams an authenticated GET of path (origin-relative) into
// dest, writing through a temp file in dest's directory and renaming into
// place so a failed transfer never leaves a truncated dest.
func DownloadToFile(ctx context.Context, c *Client, path, dest string, progress ProgressFunc) error {
	body, resp, err := c.Get(ctx, path)
	if err != nil {
		return err
	}
	defer body.Close()
	return writeStream(body, resp.ContentLength, dest, progress)
}

// DownloadPublic fetches an absolute URL WITHOUT credentials — for
// single-use-token URLs (folder zip, export) that are credential-less by
// design and must not have a bearer attached.
func DownloadPublic(ctx context.Context, hc *http.Client, url, dest string, progress ProgressFunc) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return apiError(resp.StatusCode, data)
	}
	return writeStream(resp.Body, resp.ContentLength, dest, progress)
}

func writeStream(src io.Reader, total int64, dest string, progress ProgressFunc) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".tinycld-download-*")
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	var out io.Writer = tmp
	if progress != nil {
		out = io.MultiWriter(tmp, &countingSink{total: total, progress: progress})
	}
	if _, err := io.Copy(out, src); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}
