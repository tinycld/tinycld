package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LocalFileName reduces a server-supplied name to a single path segment safe to
// join onto a local directory. Remote names are data, never paths: an item or
// attachment named "../../.ssh/authorized_keys" must land in the directory the
// user chose, not wherever its name points.
//
// filepath.Base collapses any traversal and strips separators; the fallbacks
// cover names that are entirely separators or dots ("..", "/", ""), where Base
// still yields something unusable as a filename.
func LocalFileName(name string) string {
	// Treat both separators as such regardless of host OS: a name authored on
	// one platform must not become a path component on the other.
	name = strings.ReplaceAll(name, "\\", "/")
	base := filepath.Base(filepath.FromSlash(name))
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return "download"
	}
	return base
}

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
// design and must not have a bearer attached. A nil hc gets a fresh client
// with no whole-exchange timeout (the body may be arbitrarily large); pass
// one explicitly to control transport or deadline.
func DownloadPublic(ctx context.Context, hc *http.Client, url, dest string, progress ProgressFunc) error {
	if hc == nil {
		hc = &http.Client{}
	}
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
	// os.CreateTemp always makes 0600 files. That is right for a credential but
	// wrong for a download: this is ordinary user data (a spreadsheet, an
	// attachment), and landing it 0600 regardless of umask means a file the
	// user cannot share with their own group — unlike curl, scp, or a browser.
	// 0666 &^ umask is the conventional "new data file" mode.
	if err := tmp.Chmod(dataFileMode()); err != nil {
		tmp.Close()
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
