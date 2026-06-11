package coreserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Package version discovery.
//
// For each installed package the operator can update or downgrade to any version
// the source publishes: npm registry versions for npm-installed packages, git
// tags for git-installed ones. The source is inferred from the stored
// pkg_registry.npm_package spec (npm name / name@ver vs github:o/r / git URL),
// reusing the same classification as install validation.

// pkgSource classifies where a package's versions come from.
type pkgSource string

const (
	sourceNpm     pkgSource = "npm"
	sourceGit     pkgSource = "git"
	sourceUnknown pkgSource = "unknown"
)

// versionInfo is the per-package discovery result returned to the UI.
type versionInfo struct {
	Slug      string    `json:"slug"`
	Source    pkgSource `json:"source"`
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`
	Available []string  `json:"available"` // descending (newest first), semver-sorted
	HasUpdate bool      `json:"hasUpdate"`
	Error     string    `json:"error,omitempty"` // per-package fetch failure; others still returned
}

// ---------- discovery cache ----------

// Discovery shells out to npm/git, which is slow; cache results briefly so the
// list screen doesn't refetch on every poll. Keyed by the spec string.
type versionCacheEntry struct {
	versions []string
	source   pkgSource
	fetched  time.Time
	err      string
}

const versionCacheTTL = 60 * time.Second

var (
	versionCacheMu sync.Mutex
	versionCache   = map[string]versionCacheEntry{}
)

func cachedVersions(spec string) (versionCacheEntry, bool) {
	versionCacheMu.Lock()
	defer versionCacheMu.Unlock()
	e, ok := versionCache[spec]
	if !ok || nowFunc().Sub(e.fetched) > versionCacheTTL {
		return versionCacheEntry{}, false
	}
	return e, true
}

func storeVersions(spec string, e versionCacheEntry) {
	versionCacheMu.Lock()
	defer versionCacheMu.Unlock()
	e.fetched = nowFunc()
	versionCache[spec] = e
}

// nowFunc is overridable in tests; production uses time.Now.
var nowFunc = time.Now

// ---------- spec classification ----------

// classifySpec determines a spec's source and its lookup key: for npm the bare
// package name (scope included, version stripped); for git the remote URL/spec
// npm/git understands. Returns sourceUnknown if neither pattern matches.
func classifySpec(spec string) (pkgSource, string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return sourceUnknown, ""
	}
	// A git source may carry a pinned #ref (github:o/r#v1, https://…#tag). The
	// ref isn't part of gitSpecPattern, so strip it before matching; the returned
	// key is the bare remote (callers re-pin the ref as needed). npm specs never
	// contain '#', so this only affects git.
	bare := spec
	if hash := strings.Index(bare, "#"); hash >= 0 {
		bare = bare[:hash]
	}
	// Git specs take precedence: a `github:owner/repo` also superficially
	// resembles nothing npm, and `owner/repo` shorthand is git-only.
	if gitSpecPattern.MatchString(bare) && !npmVersionedPattern.MatchString(bare) {
		return sourceGit, bare
	}
	if npmVersionedPattern.MatchString(spec) || npmPackagePattern.MatchString(spec) {
		return sourceNpm, stripNpmVersion(spec)
	}
	if gitSpecPattern.MatchString(bare) {
		return sourceGit, bare
	}
	return sourceUnknown, ""
}

// stripNpmVersion removes a trailing @version from an npm spec, preserving a
// leading scope's @. e.g. "@tinycld/mail@1.2.3" → "@tinycld/mail", "mail@1" → "mail".
func stripNpmVersion(spec string) string {
	at := strings.LastIndex(spec, "@")
	if at <= 0 { // no version, or the scope's leading @ at index 0
		return spec
	}
	return spec[:at]
}

// ---------- version listing ----------

// listNpmVersions returns all published versions of an npm package, newest
// first. Shells out to `npm view <name> versions --json`.
func listNpmVersions(name string) ([]string, error) {
	out, err := runCmd(".", "npm", "view", name, "versions", "--json")
	if err != nil {
		return nil, errFromCmd("npm view", out, err)
	}
	// `npm view ... versions --json` prints a JSON array, or a bare JSON string
	// when only one version exists.
	trimmed := strings.TrimSpace(out)
	var versions []string
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &versions); err != nil {
			return nil, err
		}
	} else {
		var single string
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, err
		}
		versions = []string{single}
	}
	return sortVersionsDesc(versions), nil
}

// listGitTagVersions returns the semver tags of a git remote, newest first.
// Shells out to `git ls-remote --tags <remote>` and keeps tags that parse as
// semver (with or without a leading v), normalized to the bare version.
func listGitTagVersions(remote string) ([]string, error) {
	// A local file:// remote (self-hosted/air-gapped base, or the integration
	// test's provisioned bare repo) may be owned by a different user than the
	// runtime user, so git would refuse with "detected dubious ownership". The
	// entrypoint writes `safe.directory=*` to the runtime user's GLOBAL git
	// config to allow it — git honors the wildcard ONLY from a config file, not
	// from `-c`/GIT_CONFIG_* env, so it can't be set here on the command line.
	out, err := runCmd(".", "git", "ls-remote", "--tags", "--refs", gitRemoteURL(remote))
	if err != nil {
		return nil, errFromCmd("git ls-remote", out, err)
	}
	versions := []string{}
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, "refs/tags/")
		if idx < 0 {
			continue
		}
		tag := strings.TrimSpace(line[idx+len("refs/tags/"):])
		if tag == "" {
			continue
		}
		// Keep the RAW tag (e.g. `v1.0.0`, NOT a normalized `1.0.0`): it becomes
		// the `#<ref>` in specForVersion, and `npm pack github:o/r#1.0.0` does NOT
		// resolve a `v1.0.0` tag (git checkout of `1.0.0` fails — the tag is named
		// `v1.0.0`). Comparisons against the registry's bare `current` are made
		// semver-aware at the call sites (see sameVersion / isNewer / the UI's
		// compareVersions) so a `v`-prefixed tag still matches a bare current.
		if _, err := semver.NewVersion(tag); err == nil {
			versions = append(versions, tag)
		}
	}
	return sortVersionsDesc(versions), nil
}

// gitRemoteURL turns a host-shorthand spec into a fetchable URL for ls-remote.
// `git+`-prefixed and bare https/ssh URLs are used as-is (minus the git+);
// github:/gitlab:/bitbucket: and bare owner/repo expand to https github by
// default (matching npm pack's shorthand expansion for the common case).
func gitRemoteURL(spec string) string {
	switch {
	case strings.HasPrefix(spec, "git+"):
		return strings.TrimPrefix(spec, "git+")
	case strings.HasPrefix(spec, "https://"), strings.HasPrefix(spec, "git://"):
		return spec
	case strings.HasPrefix(spec, "github:"):
		return "https://github.com/" + strings.TrimPrefix(spec, "github:") + ".git"
	case strings.HasPrefix(spec, "gitlab:"):
		return "https://gitlab.com/" + strings.TrimPrefix(spec, "gitlab:") + ".git"
	case strings.HasPrefix(spec, "bitbucket:"):
		return "https://bitbucket.org/" + strings.TrimPrefix(spec, "bitbucket:") + ".git"
	default:
		// bare owner/repo shorthand → github
		return "https://github.com/" + spec + ".git"
	}
}

// versionsForSpec resolves a spec's available versions through the cache.
func versionsForSpec(spec string) (source pkgSource, versions []string, fetchErr string) {
	if e, ok := cachedVersions(spec); ok {
		return e.source, e.versions, e.err
	}
	src, key := classifySpec(spec)
	entry := versionCacheEntry{source: src}
	switch src {
	case sourceNpm:
		v, err := listNpmVersions(key)
		if err != nil {
			entry.err = err.Error()
		} else {
			entry.versions = v
		}
	case sourceGit:
		v, err := listGitTagVersions(key)
		if err != nil {
			entry.err = err.Error()
		} else {
			entry.versions = v
		}
	default:
		entry.err = "unrecognized package spec; cannot determine version source"
	}
	storeVersions(spec, entry)
	return entry.source, entry.versions, entry.err
}

// ---------- semver helpers (shared with the compatibility solver) ----------

// sortVersionsDesc sorts version strings newest-first by semver, dropping any
// that don't parse. Stable for equal versions.
func sortVersionsDesc(versions []string) []string {
	parsed := make([]*semver.Version, 0, len(versions))
	for _, v := range versions {
		if sv, err := semver.NewVersion(v); err == nil {
			parsed = append(parsed, sv)
		}
	}
	sort.SliceStable(parsed, func(i, j int) bool {
		return parsed[i].GreaterThan(parsed[j])
	})
	out := make([]string, len(parsed))
	for i, sv := range parsed {
		out[i] = sv.Original()
	}
	return out
}

// ---------- discovery endpoint ----------

// handleVersions returns version info for every package in pkg_registry that has
// a resolvable source spec. A per-package fetch failure is reported in that
// row's Error field rather than failing the whole response.
func handleVersions(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return re.InternalServerError("Failed to load package registry", err)
	}

	infos := make([]versionInfo, 0, len(records))
	for _, rec := range records {
		spec := rec.GetString("npm_package")
		current := rec.GetString("version")
		info := versionInfo{
			Slug:    rec.GetString("slug"),
			Current: current,
			// Always a non-nil slice so it marshals as `[]`, never `null`: the
			// client types `available` as string[] and calls `.length`/`.indexOf`
			// on it (the Packages version controls, detectDowngrade), which throw
			// on null. A nil Go slice JSON-encodes to `null`, so initialize it
			// here — the unknown-source continue path below relies on this default.
			Available: []string{},
		}
		if spec == "" {
			// Bundled packages with no install spec have no external source.
			info.Source = sourceUnknown
			infos = append(infos, info)
			continue
		}
		src, versions, fetchErr := versionsForSpec(spec)
		info.Source = src
		if versions != nil {
			info.Available = versions
		}
		info.Error = fetchErr
		if len(versions) > 0 {
			info.Latest = versions[0]
			info.HasUpdate = isNewer(versions[0], current)
		}
		infos = append(infos, info)
	}

	return re.JSON(http.StatusOK, map[string]any{"packages": infos})
}

// specForVersion builds an install spec pinned to targetVersion from a package's
// stored source spec. For npm it appends @<version> to the bare name; for git it
// pins the ref via #<version> (the tag). Returns an error for unrecognized specs.
func specForVersion(sourceSpec, targetVersion string) (string, error) {
	// A git source may already carry a pinned #ref (e.g. github:o/r#v0.1.0).
	// Strip it before classifying — the ref isn't part of the gitSpecPattern and
	// we're replacing it anyway.
	base := sourceSpec
	if hash := strings.Index(base, "#"); hash >= 0 {
		base = base[:hash]
	}
	src, key := classifySpec(base)
	switch src {
	case sourceNpm:
		return key + "@" + targetVersion, nil
	case sourceGit:
		return key + "#" + targetVersion, nil
	default:
		return "", fmt.Errorf("cannot build install spec for unrecognized source %q", sourceSpec)
	}
}

// errFromCmd wraps a failed command's combined output into the error, matching
// the install pipeline's `%v: %s` convention so failures carry the tool output.
func errFromCmd(label, out string, err error) error {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return fmt.Errorf("%s: %w", label, err)
	}
	return fmt.Errorf("%s: %w: %s", label, err, trimmed)
}

// isNewer reports whether candidate is a strictly greater semver than current.
// Unparsable inputs yield false (treat as "no update" rather than guessing).
func isNewer(candidate, current string) bool {
	c, err1 := semver.NewVersion(candidate)
	cur, err2 := semver.NewVersion(current)
	if err1 != nil || err2 != nil {
		return false
	}
	return c.GreaterThan(cur)
}
