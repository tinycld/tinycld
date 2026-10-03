package supervise

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// releasesDir / releaseStaticPoolDir / currentReleaseLinkPath mirror
// entrypoint.sh's promote_release layout: releases_dir and pool_dir.
func (s State) releasesDir() string          { return filepath.Join(s.Root, "releases") }
func (s State) releaseStaticPoolDir() string { return filepath.Join(s.releasesDir(), "_static") }
func (s State) currentReleaseLinkPath() string {
	return filepath.Join(s.releasesDir(), "current")
}

// releaseStagingDir is where the active build stages a freshly-built web
// bundle before it is promoted — <current>/release-staging in
// entrypoint.sh's promote_release.
func (s State) releaseStagingDir() (string, error) {
	current, err := s.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(current, "release-staging"), nil
}

// PromoteRelease ports entrypoint.sh's promote_release: pick the
// most-recently-modified staging dir under <current>/release-staging that
// carries a release-id.txt, merge its asset trees into the cross-release
// pool at releases/_static, copy its app.html + release-id.txt (+
// manifest.json if present) into releases/<id>, then atomically flip
// releases/current -> <id>.
//
// Matches the shell's observable outcomes:
//   - no staging dir at all: not an error (the shell logs a WARN and
//     returns 0 — nothing to promote yet, e.g. a build with no web bundle).
//   - a staging dir exists but none carries release-id.txt, or the promoted
//     release ends up without app.html: an error (the shell exits 1).
func (s State) PromoteRelease() error {
	stagingDir, err := s.releaseStagingDir()
	if err != nil {
		return fmt.Errorf("promote release: resolve current build: %w", err)
	}

	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("release staging dir missing; skipping release promotion (SPA fallback will 404)", "dir", stagingDir)
			return nil
		}
		return fmt.Errorf("promote release: read staging dir %s: %w", stagingDir, err)
	}

	releaseID, src, err := newestStagedRelease(stagingDir, entries)
	if err != nil {
		return err
	}

	pool := s.releaseStaticPoolDir()
	for _, sub := range []string{"_expo/static", "assets"} {
		if err := os.MkdirAll(filepath.Join(pool, sub), 0o755); err != nil {
			return fmt.Errorf("promote release: create pool dir %s: %w", sub, err)
		}
	}
	// Merge (not replace): the current release's bytes must win a name
	// collision without erasing files only an earlier release's tree holds
	// (see entrypoint.sh's promote_release pool comment — Expo chunk names
	// aren't content hashes, and a few asset names are stable across
	// releases).
	if info, statErr := os.Stat(filepath.Join(src, "_expo", "static")); statErr == nil && info.IsDir() {
		if err := copyMerge(filepath.Join(src, "_expo", "static"), filepath.Join(pool, "_expo", "static")); err != nil {
			return fmt.Errorf("promote release: merge _expo/static into pool: %w", err)
		}
	}
	if info, statErr := os.Stat(filepath.Join(src, "assets")); statErr == nil && info.IsDir() {
		if err := copyMerge(filepath.Join(src, "assets"), filepath.Join(pool, "assets")); err != nil {
			return fmt.Errorf("promote release: merge assets into pool: %w", err)
		}
	}

	if err := s.promoteReleaseDir(releaseID, src); err != nil {
		return err
	}

	if err := s.flipCurrentRelease(releaseID); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(s.currentReleaseLinkPath(), "app.html")); err != nil {
		return fmt.Errorf("promote release: %s missing after promotion: %w", filepath.Join(s.currentReleaseLinkPath(), "app.html"), err)
	}
	log.Info("promoted release", "releaseID", releaseID)
	return nil
}

// newestStagedRelease picks the most-recently-modified staging dir that
// carries release-id.txt, mirroring the shell's `ls -1dt` (newest-mtime-
// first) loop. An in-app package install leaves both the base image's
// release and the install's freshly-built bundle in staging; only the
// newest carries the just-installed package's routes.
func newestStagedRelease(stagingDir string, entries []os.DirEntry) (releaseID, src string, err error) {
	type candidate struct {
		path    string
		modTime int64
	}
	var dirs []candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, candidate{path: filepath.Join(stagingDir, e.Name()), modTime: info.ModTime().UnixNano()})
	}
	// Newest-first, stable so equal mtimes keep directory-read order.
	for i := 1; i < len(dirs); i++ {
		for j := i; j > 0 && dirs[j-1].modTime < dirs[j].modTime; j-- {
			dirs[j-1], dirs[j] = dirs[j], dirs[j-1]
		}
	}
	for _, d := range dirs {
		idPath := filepath.Join(d.path, "release-id.txt")
		data, err := os.ReadFile(idPath)
		if err != nil {
			continue
		}
		return string(data), d.path, nil
	}
	return "", "", fmt.Errorf("promote release: no release-id.txt found under %s", stagingDir)
}

// promoteReleaseDir copies src's app.html + release-id.txt (+ manifest.json
// if present) into releases/<releaseID>, via a .tmp dir + rename so a reader
// never sees a half-written release dir. A prior half-promoted dir (missing
// app.html, e.g. from an interrupted copy) is wiped and re-promoted; an
// already-complete one is left alone — entrypoint.sh's same two checks.
func (s State) promoteReleaseDir(releaseID, src string) error {
	dst := filepath.Join(s.releasesDir(), releaseID)

	if _, err := os.Stat(filepath.Join(dst, "app.html")); err == nil {
		return nil // already promoted
	}
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("promote release: clear incomplete release dir %s: %w", dst, err)
	}

	tmp := dst + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("promote release: clear stale %s: %w", tmp, err)
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return fmt.Errorf("promote release: create %s: %w", tmp, err)
	}
	if err := copyFile(filepath.Join(src, "app.html"), filepath.Join(tmp, "app.html")); err != nil {
		return fmt.Errorf("promote release: copy app.html: %w", err)
	}
	if err := copyFile(filepath.Join(src, "release-id.txt"), filepath.Join(tmp, "release-id.txt")); err != nil {
		return fmt.Errorf("promote release: copy release-id.txt: %w", err)
	}
	// manifest.json is present only on release builds; absent is fine.
	if _, err := os.Stat(filepath.Join(src, "manifest.json")); err == nil {
		if err := copyFile(filepath.Join(src, "manifest.json"), filepath.Join(tmp, "manifest.json")); err != nil {
			return fmt.Errorf("promote release: copy manifest.json: %w", err)
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("promote release: rename %s to %s: %w", tmp, dst, err)
	}
	return nil
}

// flipCurrentRelease atomically points releases/current -> releaseID, the
// same current.tmp-then-rename swap entrypoint.sh's promote_release uses.
func (s State) flipCurrentRelease(releaseID string) error {
	tmp := s.currentReleaseLinkPath() + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(releaseID, tmp); err != nil {
		return fmt.Errorf("promote release: symlink %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.currentReleaseLinkPath()); err != nil {
		return fmt.Errorf("promote release: flip current release: %w", err)
	}
	return nil
}

// copyMerge recursively copies src's contents into dst, overwriting any
// same-named file already there — the Go equivalent of `cp -a src/. dst/`.
// Used only for the asset pool merge, where the newest release's bytes must
// win a name collision (see PromoteRelease's doc comment).
func copyMerge(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyMerge(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies one regular file, truncating/creating dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
