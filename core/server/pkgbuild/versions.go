package pkgbuild

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
)

// Package version discovery (moved from coreserver/pkg_versions.go so the
// multi-org router can serve the same discovery over the per-org control
// socket — D5's one-shared-implementation rule; the confined tenant has no
// npm/git of its own).
//
// For each installed package the operator can update or downgrade to any
// version the source publishes: npm registry versions for npm-installed
// packages, git tags for git-installed ones. The source is inferred from the
// stored spec, reusing the same classification as install validation.

// PkgSource classifies where a package's versions come from.
type PkgSource string

const (
	SourceNpm     PkgSource = "npm"
	SourceGit     PkgSource = "git"
	SourceUnknown PkgSource = "unknown"
)

// ---------- discovery cache ----------

// Discovery shells out to npm/git, which is slow; cache results briefly so
// list screens don't refetch on every poll. Keyed by the spec string.
type versionCacheEntry struct {
	versions []string
	source   PkgSource
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
	if !ok || versionNow().Sub(e.fetched) > versionCacheTTL {
		return versionCacheEntry{}, false
	}
	return e, true
}

func storeVersions(spec string, e versionCacheEntry) {
	versionCacheMu.Lock()
	defer versionCacheMu.Unlock()
	e.fetched = versionNow()
	versionCache[spec] = e
}

// versionNow is overridable in tests; production uses time.Now.
var versionNow = time.Now

// ---------- spec classification ----------

// ClassifySpec determines a spec's source and its lookup key: for npm the bare
// package name (scope included, version stripped); for git the remote URL/spec
// npm/git understands. Returns SourceUnknown if neither pattern matches.
func ClassifySpec(spec string) (PkgSource, string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return SourceUnknown, ""
	}
	// A git source may carry a pinned #ref (github:o/r#v1, https://…#tag). The
	// ref isn't part of GitSpecPattern, so strip it before matching; the returned
	// key is the bare remote (callers re-pin the ref as needed). npm specs never
	// contain '#', so this only affects git.
	bare := spec
	if hash := strings.Index(bare, "#"); hash >= 0 {
		bare = bare[:hash]
	}
	// Git specs take precedence: a `github:owner/repo` also superficially
	// resembles nothing npm, and `owner/repo` shorthand is git-only.
	if GitSpecPattern.MatchString(bare) && !NpmVersionedPattern.MatchString(bare) {
		return SourceGit, bare
	}
	if NpmVersionedPattern.MatchString(spec) || NpmPackagePattern.MatchString(spec) {
		return SourceNpm, StripNpmVersion(spec)
	}
	if GitSpecPattern.MatchString(bare) {
		return SourceGit, bare
	}
	return SourceUnknown, ""
}

// StripNpmVersion removes a trailing @version from an npm spec, preserving a
// leading scope's @. e.g. "@tinycld/mail@1.2.3" → "@tinycld/mail", "mail@1" → "mail".
func StripNpmVersion(spec string) string {
	at := strings.LastIndex(spec, "@")
	if at <= 0 { // no version, or the scope's leading @ at index 0
		return spec
	}
	return spec[:at]
}

// ---------- version listing ----------

// ListNpmVersions returns all published versions of an npm package, newest
// first. Shells out to `npm view <name> versions --json`.
func ListNpmVersions(name string) ([]string, error) {
	out, err := RunCmd(".", "npm", "view", name, "versions", "--json")
	if err != nil {
		return nil, ErrFromCmd("npm view", out, err)
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
	return SortVersionsDesc(versions), nil
}

// ListGitTagVersions returns the semver tags of a git remote, newest first.
// Shells out to `git ls-remote --tags <remote>` and keeps tags that parse as
// semver (with or without a leading v), normalized to the bare version.
func ListGitTagVersions(remote string) ([]string, error) {
	// A local file:// remote (self-hosted/air-gapped base, or the integration
	// test's provisioned bare repo) may be owned by a different user than the
	// runtime user, so git would refuse with "detected dubious ownership". The
	// entrypoint writes `safe.directory=*` to the runtime user's GLOBAL git
	// config to allow it — git honors the wildcard ONLY from a config file, not
	// from `-c`/GIT_CONFIG_* env, so it can't be set here on the command line.
	out, err := RunCmd(".", "git", "ls-remote", "--tags", "--refs", GitRemoteURL(remote))
	if err != nil {
		return nil, ErrFromCmd("git ls-remote", out, err)
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
		// the `#<ref>` in SpecForVersion, and `npm pack github:o/r#1.0.0` does NOT
		// resolve a `v1.0.0` tag (git checkout of `1.0.0` fails — the tag is named
		// `v1.0.0`). Comparisons against the registry's bare `current` are made
		// semver-aware at the call sites (see IsNewerVersion / the UI's
		// compareVersions) so a `v`-prefixed tag still matches a bare current.
		if _, err := semver.NewVersion(tag); err == nil {
			versions = append(versions, tag)
		}
	}
	return SortVersionsDesc(versions), nil
}

// GitRemoteURL turns a host-shorthand spec into a fetchable URL for ls-remote.
// `git+`-prefixed and bare https/ssh URLs are used as-is (minus the git+);
// github:/gitlab:/bitbucket: and bare owner/repo expand to https github by
// default (matching npm pack's shorthand expansion for the common case).
func GitRemoteURL(spec string) string {
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

// VersionsForSpec resolves a spec's available versions through the cache.
func VersionsForSpec(spec string) (source PkgSource, versions []string, fetchErr string) {
	if e, ok := cachedVersions(spec); ok {
		return e.source, e.versions, e.err
	}
	src, key := ClassifySpec(spec)
	entry := versionCacheEntry{source: src}
	switch src {
	case SourceNpm:
		v, err := ListNpmVersions(key)
		if err != nil {
			entry.err = err.Error()
		} else {
			entry.versions = v
		}
	case SourceGit:
		v, err := ListGitTagVersions(key)
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

// SortVersionsDesc sorts version strings newest-first by semver, dropping any
// that don't parse. Stable for equal versions.
func SortVersionsDesc(versions []string) []string {
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

// SpecForVersion builds an install spec pinned to targetVersion from a package's
// stored source spec. For npm it appends @<version> to the bare name; for git it
// pins the ref via #<version> (the tag). Returns an error for unrecognized specs.
func SpecForVersion(sourceSpec, targetVersion string) (string, error) {
	// A git source may already carry a pinned #ref (e.g. github:o/r#v0.1.0).
	// Strip it before classifying — the ref isn't part of the GitSpecPattern and
	// we're replacing it anyway.
	base := sourceSpec
	if hash := strings.Index(base, "#"); hash >= 0 {
		base = base[:hash]
	}
	src, key := ClassifySpec(base)
	switch src {
	case SourceNpm:
		return key + "@" + targetVersion, nil
	case SourceGit:
		return key + "#" + targetVersion, nil
	default:
		return "", fmt.Errorf("cannot build install spec for unrecognized source %q", sourceSpec)
	}
}

// ErrFromCmd wraps a failed command's combined output into the error, matching
// the install pipeline's `%v: %s` convention so failures carry the tool output.
func ErrFromCmd(label, out string, err error) error {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return fmt.Errorf("%s: %w", label, err)
	}
	return fmt.Errorf("%s: %w: %s", label, err, trimmed)
}

// IsNewerVersion reports whether candidate is a strictly greater semver than
// current. Unparsable inputs yield false (treat as "no update" rather than
// guessing).
func IsNewerVersion(candidate, current string) bool {
	c, err1 := semver.NewVersion(candidate)
	cur, err2 := semver.NewVersion(current)
	if err1 != nil || err2 != nil {
		return false
	}
	return c.GreaterThan(cur)
}
