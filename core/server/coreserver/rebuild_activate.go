package coreserver

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func currentLinkPath() string { return filepath.Join(resolveStateDir(), "current") }

// previousBuildPath records the build id that `current` pointed at before the
// most recent activation, so the entrypoint can roll back the symlink if the
// new build fails its post-restart health probe.
func previousBuildPath() string { return filepath.Join(resolveStateDir(), ".previous-build") }

// activateBuild atomically points <state>/current at <state>/builds/<id>/tinycld.
// Before flipping, it records the outgoing build id to <state>/.previous-build
// so the entrypoint can roll the symlink back on a failed health probe.
func activateBuild(buildID string) error {
	target := filepath.Join(stateBuildsDir(), buildID, "tinycld")
	if _, err := os.Stat(target); err != nil {
		return err
	}
	prev := currentBuildID()
	if prev != "" && prev != buildID {
		_ = os.WriteFile(previousBuildPath(), []byte(prev), 0o644)
	}
	tmp := currentLinkPath() + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, currentLinkPath()); err != nil {
		return err
	}
	// Durable (docker logs) record of the atomic swap + the rollback target the
	// entrypoint would flip back to on a failed post-restart health probe.
	log.Printf("[pkg_install] activate: current %s -> %s (rollback target: %s)",
		orNone(prev), buildID, orNone(prev))
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// currentBuildID returns the build id `current` points at, or "" if unset.
func currentBuildID() string {
	dest, err := os.Readlink(currentLinkPath())
	if err != nil {
		return ""
	}
	// dest = <builds>/<id>/tinycld → id is the parent dir name.
	return filepath.Base(filepath.Dir(dest))
}

// pruneBuilds keeps the `keep` newest build dirs plus the current one,
// removing the rest. Build ids sort lexicographically by their build-<millis>
// suffix, so newest = lexicographically-last.
func pruneBuilds(keep int) error {
	cur := currentBuildID()
	entries, err := os.ReadDir(stateBuildsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids) // oldest-first
	// Survivors = the newest `keep` ids, unioned with the current one (which
	// must never be pruned even if it's older than the newest `keep`).
	survive := map[string]bool{}
	if cur != "" {
		survive[cur] = true
	}
	for i := len(ids) - 1; i >= 0 && len(ids)-1-i < keep; i-- {
		survive[ids[i]] = true
	}
	var pruned []string
	for _, id := range ids {
		if !survive[id] {
			if err := os.RemoveAll(filepath.Join(stateBuildsDir(), id)); err != nil {
				return err
			}
			pruned = append(pruned, id)
		}
	}
	if len(pruned) > 0 {
		log.Printf("[pkg_install] prune: removed %d old build(s): %s (kept current=%s + newest %d)",
			len(pruned), strings.Join(pruned, ", "), orNone(cur), keep)
	}
	return nil
}
