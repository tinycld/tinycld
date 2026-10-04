package supervise

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	releaseID, src, found, err := stagedRelease(stagingDir)
	if err != nil || !found {
		return err
	}
	return s.promote(releaseID, src)
}

// PromoteReleaseIfNewer promotes the newest release the build at build
// staged, but only when it was staged after the release releases/current
// serves, or when nothing is served yet. A rollback calls it for the build
// it went back to: that build may have been replaced before its own ready
// promoted its bundle, while a build whose bundle is older than the one
// served must not take it back.
//
// Release ids do not sort by age (an image build's "<date>-<sha>" against
// an in-app build's "install-<ms>"), so age is release-id.txt's mtime: every
// copy of it (the build's copyTree, cp -a, copyFile) keeps the mtime it got
// when the release was staged.
func (s State) PromoteReleaseIfNewer(build string) error {
	releaseID, src, found, err := stagedRelease(filepath.Join(build, "release-staging"))
	if err != nil || !found {
		return err
	}
	newer, err := s.stagedAfterCurrent(releaseID, src)
	if err != nil {
		return err
	}
	if !newer {
		return nil
	}
	return s.promote(releaseID, src)
}

// stagedAfterCurrent reports whether the staged release at src is not the
// one releases/current serves and was staged after it.
func (s State) stagedAfterCurrent(releaseID, src string) (bool, error) {
	servedPath := filepath.Join(s.currentReleaseLinkPath(), "release-id.txt")
	served, err := os.Stat(servedPath)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("promote release: stat %s: %w", servedPath, err)
	}
	servedID, err := os.ReadFile(servedPath)
	if err != nil {
		return false, fmt.Errorf("promote release: read %s: %w", servedPath, err)
	}
	if strings.TrimSpace(string(servedID)) == strings.TrimSpace(releaseID) {
		return false, nil
	}
	staged, err := os.Stat(filepath.Join(src, "release-id.txt"))
	if err != nil {
		return false, fmt.Errorf("promote release: stat %s: %w", filepath.Join(src, "release-id.txt"), err)
	}
	return staged.ModTime().After(served.ModTime()), nil
}

// stagedRelease finds the newest staged release under stagingDir. A missing
// staging dir is not an error (the shell logs a WARN and returns 0): found
// is false and there is nothing to promote.
func stagedRelease(stagingDir string) (releaseID, src string, found bool, err error) {
	entries, err := os.ReadDir(stagingDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("release staging dir missing; skipping release promotion (SPA fallback will 404)", "dir", stagingDir)
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("promote release: read staging dir %s: %w", stagingDir, err)
	}
	releaseID, src, err = newestStagedRelease(stagingDir, entries)
	if err != nil {
		return "", "", false, err
	}
	return releaseID, src, true, nil
}

// promote merges the staged release at src into the pool, copies it to
// releases/<releaseID> and points releases/current at it.
func (s State) promote(releaseID, src string) error {
	pool := s.releaseStaticPoolDir()
	for _, sub := range []string{"_expo/static", "assets"} {
		if err := mkdirAllOwned(filepath.Join(pool, sub)); err != nil {
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
	if err := mkdirAllOwned(tmp); err != nil {
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
	if err := mkdirAllOwned(dst); err != nil {
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

// mkdirAllOwned is os.MkdirAll where each directory it creates takes the
// owner of its parent. The supervisor runs as root while its children do
// not, and a root-owned directory under releases/ is one a child cannot add
// to.
func mkdirAllOwned(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("mkdir %s: a file is in the way", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return err
	}
	if err := mkdirAllOwned(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		// Made by someone else since the stat: its owner is theirs to set.
		if info, statErr := os.Stat(path); errors.Is(err, os.ErrExist) && statErr == nil && info.IsDir() {
			return nil
		}
		return err
	}
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return err
	}
	return matchOwner(path, parentInfo)
}

// copyFile puts src's bytes at dst the way a reader of dst must see them.
// Children serve the pool while the supervisor promotes, so dst is never
// written in place: a reader would see a truncated file. The bytes go to a
// temp file in dst's directory, which then renames over dst in one step. A
// dst that already holds the same bytes is left alone, so its mtime (what
// no-cache revalidation compares) does not move. The source mtime and mode
// carry over, as `cp -a` did. The owner is the replaced file's, or the
// source's for a new file: the supervisor runs as root, and a root-owned
// file in the pool is one a child's later rebuild cannot replace.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	same, err := sameBytes(src, dst, info.Size())
	if err != nil {
		return err
	}
	if same {
		return nil
	}
	owner := info
	if dinfo, err := os.Stat(dst); err == nil {
		owner = dinfo
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			os.Remove(tmp.Name())
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := matchOwner(tmp.Name(), owner); err != nil {
		return err
	}
	if err := os.Chtimes(tmp.Name(), info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return err
	}
	done = true
	return nil
}

// sameBytes reports whether dst exists and holds exactly src's bytes.
func sameBytes(src, dst string, size int64) (bool, error) {
	dinfo, err := os.Stat(dst)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !dinfo.Mode().IsRegular() || dinfo.Size() != size {
		return false, nil
	}
	a, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer a.Close()
	b, err := os.Open(dst)
	if err != nil {
		return false, err
	}
	defer b.Close()
	bufA, bufB := make([]byte, 32*1024), make([]byte, 32*1024)
	for {
		na, errA := io.ReadFull(a, bufA)
		nb, errB := io.ReadFull(b, bufB)
		if na != nb || !bytes.Equal(bufA[:na], bufB[:nb]) {
			return false, nil
		}
		endA := errors.Is(errA, io.EOF) || errors.Is(errA, io.ErrUnexpectedEOF)
		endB := errors.Is(errB, io.EOF) || errors.Is(errB, io.ErrUnexpectedEOF)
		if endA || endB {
			return endA && endB, nil
		}
		if errA != nil {
			return false, errA
		}
		if errB != nil {
			return false, errB
		}
	}
}
