package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pocketbase/pocketbase"
)

// Base (core) file swap. The TinyCld base is the WHOLE tinycld app-shell repo
// (app shell + nested core/ + Go server), not a swappable sibling member, so a
// version change to `core` fetches the whole repo by git clone (vs a feature's
// `npm pack` of one dir) and swaps SOURCE files only — the live runtime state
// dirs (pb_data, builds, releases, node_modules, the running binary) sit inside
// the same appDir and MUST survive the swap untouched. Everything downstream of
// the swap (regen, migrate, rebuild, stage, archive, restart, rollback) is the
// shared per-package pipeline; only this fetch/swap front-door is base-specific.

// basePreserveNames are the runtime-state entries under appDir that a base swap
// must keep in place — never overwrite them from the cloned source tree.
var basePreserveNames = map[string]bool{
	"pb_data":      true, // the live, bind-mounted SQLite DB the restart depends on
	"builds":       true, // archived build snapshots
	"releases":     true, // staged web bundles served by the running server
	"node_modules": true, // hoisted workspace links resolved at install time
}

// basePreserve reports whether an appDir entry is runtime state that must be
// preserved across a base swap. The running binary and its swap siblings
// (tinycld, tinycld.new, tinycld.prev, tinycld.failed) are always preserved —
// the binary is replaced atomically later by swapBinary, never by the source copy.
func basePreserve(name string) bool {
	if basePreserveNames[name] {
		return true
	}
	if name == binaryName || strings.HasPrefix(name, binaryName+".") {
		return true
	}
	return false
}

// baseSourceEntries filters a cloned repo's top-level entries down to the source
// set a base swap copies over appDir, dropping any runtime-state names that must
// be preserved (a clone never legitimately carries pb_data/builds/etc., but we
// filter defensively so a stray dir in the clone can't clobber live state).
func baseSourceEntries(cloneEntries []string) []string {
	out := make([]string, 0, len(cloneEntries))
	for _, e := range cloneEntries {
		if basePreserve(e) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// swapBaseFiles git-clones the tinycld base repo at targetRef and swaps its
// source files over appDir, preserving runtime state. It returns a synthesized
// parsedManifest (slug "core", HasServer true — core ships core/server) and
// records a rollback that restores the prior source tree.
//
// Signature mirrors swapPackageFiles so applyOneVersionChange can branch on slug.
func swapBaseFiles(
	app *pocketbase.PocketBase,
	job *installJob,
	remoteSpec, targetRef, wsRoot, appDir string,
	rollbackStack *[]func(),
) (*parsedManifest, error) {
	remoteURL := gitRemoteURL(remoteSpec)
	tmpDir, err := os.MkdirTemp("", "tinycld-base-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	cloneDir := filepath.Join(tmpDir, "clone")
	if out, err := runCmd(wsRoot, "git", "clone", "--depth", "1", "--branch", targetRef, remoteURL, cloneDir); err != nil {
		return nil, errFromCmd("git clone "+remoteURL+"#"+targetRef, out, err)
	}

	// Synthesize the manifest from the clone's core/package.json.
	manifest, err := synthesizeBaseManifest(cloneDir)
	if err != nil {
		return nil, err
	}
	if manifest.Version != "" && job != nil && len(job.Changes) > 0 {
		want := job.Changes[0].TargetVersion
		if manifest.Version != want {
			return nil, fmt.Errorf("base tag %q ships core version %q, not %q (tag must match core/package.json)", targetRef, manifest.Version, want)
		}
	}

	entries, err := os.ReadDir(cloneDir)
	if err != nil {
		return nil, fmt.Errorf("read clone dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		names = append(names, e.Name())
	}
	source := baseSourceEntries(names)

	// Back up exactly the source entries we are about to overwrite, under
	// wsRoot/backups/base/ (two levels deep, no package.json -> invisible to member
	// enumeration; same filesystem -> fast rename rollback).
	backupRoot := filepath.Join(wsRoot, backupsDirName, "base")
	os.RemoveAll(backupRoot)
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create base backup dir: %w", err)
	}
	for _, name := range source {
		src := filepath.Join(appDir, name)
		if _, statErr := os.Stat(src); statErr != nil {
			continue // appDir doesn't currently have this entry — nothing to back up
		}
		if _, err := runCmd(".", "cp", "-a", src, filepath.Join(backupRoot, name)); err != nil {
			return nil, fmt.Errorf("backup base entry %s: %w", name, err)
		}
	}
	*rollbackStack = append(*rollbackStack, func() {
		for _, name := range source {
			dest := filepath.Join(appDir, name)
			os.RemoveAll(dest)
			bak := filepath.Join(backupRoot, name)
			if _, statErr := os.Stat(bak); statErr != nil {
				continue
			}
			if _, err := runCmd(".", "mv", bak, dest); err != nil {
				log.Printf("base_swap: rollback — restore %s failed: %v", name, err)
			}
		}
	})

	// Copy clone source over appDir, entry by entry, never touching preserved
	// state. copyDir is dir-only (`cp -a src/. dst/`), so branch on file-vs-dir:
	// top-level files (app.json, package.json, *.config.*) are copied with a
	// plain `cp -a`, directories (app/, core/, server/, scripts/) via copyDir.
	for _, name := range source {
		srcPath := filepath.Join(cloneDir, name)
		dest := filepath.Join(appDir, name)
		os.RemoveAll(dest)
		fi, statErr := os.Stat(srcPath)
		if statErr != nil {
			return nil, fmt.Errorf("stat clone entry %s: %w", name, statErr)
		}
		if fi.IsDir() {
			if err := copyDir(srcPath, dest); err != nil {
				return nil, fmt.Errorf("copy base dir %s: %w", name, err)
			}
		} else {
			if _, err := runCmd(".", "cp", "-a", srcPath, dest); err != nil {
				return nil, fmt.Errorf("copy base file %s: %w", name, err)
			}
		}
	}

	return manifest, nil
}

// synthesizeBaseManifest builds the parsedManifest for a base swap from the
// clone's core/package.json. core has no manifest.ts; this mirrors the
// generator's buildCoreManifest shape. HasServer is true — core ships a Go
// server (core/server), so the version-change pipeline's HasServer gate rebuilds
// the binary without any slug-based special-casing.
func synthesizeBaseManifest(cloneDir string) (*parsedManifest, error) {
	raw, err := os.ReadFile(filepath.Join(cloneDir, "core", "package.json"))
	if err != nil {
		return nil, fmt.Errorf("read clone core/package.json: %w", err)
	}
	var pkg struct {
		Version string `json:"version"`
		Tinycld struct {
			PeerVersions map[string]string `json:"peerVersions"`
		} `json:"tinycld"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, fmt.Errorf("parse clone core/package.json: %w", err)
	}
	rawJSON := map[string]any{
		"name":        "TinyCld Base",
		"slug":        "core",
		"version":     pkg.Version,
		"description": "The TinyCld base — app shell, core library, and server.",
	}
	if len(pkg.Tinycld.PeerVersions) > 0 {
		rawJSON["peerVersions"] = pkg.Tinycld.PeerVersions
	}
	return &parsedManifest{
		Name:        "TinyCld Base",
		Slug:        "core",
		Version:     pkg.Version,
		Description: "The TinyCld base — app shell, core library, and server.",
		HasServer:   true,
		// Core has no nav rail entry (the generator's buildCoreManifest omits nav),
		// but upsertPkgRegistry dereferences m.Nav.Icon/.Order — provide a zero-value
		// Nav rather than nil so the registry upsert gets safe zero values, not a panic.
		Nav:     &manifestNav{},
		RawJSON: rawJSON,
	}, nil
}
