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
// hosting router can serve the same discovery over the per-org control
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

// 5 minutes: discovery shells out to `git ls-remote`/`npm view` once per spec,
// and the Packages screen asks for every registry row at once. A newly pushed
// tag showing up to 5 minutes late is a non-event for an operator browsing
// versions, whereas a short TTL means a screen revisit (or a dev-server reload)
// re-shells every remote. Kept in sync with ctlVersionsTTL
// (coreserver/pkg_hosted_channel.go), the hosted path's mirror of this cache.
const versionCacheTTL = 5 * time.Minute

// versionCacheMax caps how many spec entries the cache holds. The router serves
// /v1/versions for tenant-supplied specs; without a bound a tenant sending an
// unbounded stream of distinct (but individually valid) specs grows the router
// heap forever. 1024 comfortably covers every package an org's operators browse
// while keeping the map's worst-case footprint trivially small. On overflow the
// oldest-inserted entry is dropped (insertion order tracked in versionCacheKeys)
// — cache eviction, not correctness: a dropped spec simply re-fetches.
const versionCacheMax = 1024

var (
	versionCacheMu   sync.Mutex
	versionCache     = map[string]versionCacheEntry{}
	versionCacheKeys []string // insertion order, for drop-oldest eviction
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
	// Opportunistic sweep: drop entries whose TTL has expired. This bounds the
	// map by time under a churny-but-slow spec stream (the common case), before
	// the hard size cap has to fire.
	sweepExpiredLocked()
	if _, exists := versionCache[spec]; !exists {
		// Enforce the hard cap by dropping the oldest-inserted entry until there
		// is room for the newcomer. A loop, not a single drop: the sweep above
		// may have removed nothing (all entries fresh) yet still be at the cap.
		for len(versionCache) >= versionCacheMax && len(versionCacheKeys) > 0 {
			oldest := versionCacheKeys[0]
			versionCacheKeys = versionCacheKeys[1:]
			delete(versionCache, oldest)
		}
		versionCacheKeys = append(versionCacheKeys, spec)
	}
	versionCache[spec] = e
}

// sweepExpiredLocked removes every entry past its TTL. Caller holds
// versionCacheMu. Rebuilds versionCacheKeys in place, preserving insertion order
// of the survivors.
func sweepExpiredLocked() {
	now := versionNow()
	kept := versionCacheKeys[:0]
	for _, k := range versionCacheKeys {
		e, ok := versionCache[k]
		if !ok {
			continue
		}
		if now.Sub(e.fetched) > versionCacheTTL {
			delete(versionCache, k)
			continue
		}
		kept = append(kept, k)
	}
	versionCacheKeys = kept
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
	return listNpmVersions(name, "")
}

// listNpmVersions is ListNpmVersions with an optional registry override
// (empty = npm's default resolution) — the same seam npmPackWith gives member
// fetches, so a router fronting a local registry discovers versions from it
// too.
func listNpmVersions(name, registry string) ([]string, error) {
	args := []string{"view", name, "versions", "--json"}
	if registry != "" {
		args = append(args, "--registry", registry)
	}
	// Stdout-only: npm's warnings go to stderr, and RunCmd's combined capture
	// would put them in front of the JSON — every listing then failed to
	// parse whenever npm warned about anything (e.g. a workspace .npmrc near
	// the server's cwd).
	out, err := RunCmdStdout(".", "npm", args...)
	if err != nil {
		return nil, err
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
	return VersionsForSpecVia(spec, "")
}

// VersionsForSpecVia is VersionsForSpec against an overridden npm registry
// (empty = default). The override joins the cache key so a default-registry
// entry can never satisfy an overridden lookup or vice versa; git specs
// ignore it.
func VersionsForSpecVia(spec, registry string) (source PkgSource, versions []string, fetchErr string) {
	cacheKey := spec
	if registry != "" {
		cacheKey = registry + "\x00" + spec
	}
	if e, ok := cachedVersions(cacheKey); ok {
		return e.source, e.versions, e.err
	}
	src, key := ClassifySpec(spec)
	entry := versionCacheEntry{source: src}
	switch src {
	case SourceNpm:
		v, err := listNpmVersions(key, registry)
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
	storeVersions(cacheKey, entry)
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
