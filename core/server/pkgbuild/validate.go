package pkgbuild

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ParsedManifest represents the validated fields from a package manifest.ts
type ParsedManifest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Routes      *ManifestRoutes `json:"routes"`
	Nav         *ManifestNav    `json:"nav"`
	Server      *ManifestServer `json:"server,omitempty"`
	// PeerVersions is the package's enforced semver requirements on other
	// packages (keyed by slug) / @tinycld/core. The install pipeline gates on
	// these before any migration runs (checkInstallCompat); the version-change
	// solver checks them too (peerVersionsFromManifest).
	PeerVersions map[string]string `json:"peerVersions,omitempty"`
	HasServer    bool              `json:"-"`
	RawJSON      map[string]any    `json:"-"`
}

type ManifestRoutes struct {
	Directory string `json:"directory"`
}

type ManifestNav struct {
	Label string `json:"label"`
	Icon  string `json:"icon"`
	Order int    `json:"order,omitempty"`
}

type ManifestServer struct {
	Package string `json:"package"`
	Module  string `json:"module"`
}

var SlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var NpmPackagePattern = regexp.MustCompile(`^(@[a-z0-9-~][a-z0-9-._~]*/)?[a-z0-9-~][a-z0-9-._~]*$`)
var goModulePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)+(/[a-z0-9][a-z0-9_-]*)+$`)

// GitSpecPattern matches the git/URL forms `npm pack` understands natively:
// host shorthand (github:owner/repo, gitlab:…, bitbucket:…), bare
// owner/repo shorthand, and git+https / git+ssh / git+file / https URLs
// (optionally .git-suffixed). Anchored, so the whole string must match — no
// trailing junk that could smuggle a second argument.
var GitSpecPattern = regexp.MustCompile(
	`^(` +
		`(github|gitlab|bitbucket):[\w.-]+/[\w.-]+` + // host:owner/repo
		`|[a-zA-Z0-9][\w.-]*/[a-zA-Z0-9][\w.-]*` + // owner/repo shorthand (segments start alphanumeric, so no ../)
		`|git\+https://[\w./@:-]+` + // git+https URL
		`|git\+ssh://[\w./@:-]+` + // git+ssh URL
		`|git\+file:///[\w./@:-]+` + // git+file URL (local bare remote — air-gapped/self-hosted base, and the integration test's provisioned remote)
		`|https://[\w./@:-]+` + // https URL (incl. .git)
		`)$`,
)

// NpmVersionedPattern matches a bare npm name (optionally @scoped) with an
// optional trailing @<version> — e.g. `mail`, `mail@1.2.3`, `mail@latest`,
// `@tinycld/mail@1.2.3`. The version segment is a tight charset (no slashes,
// no metachars) so it can't smuggle a second npm-pack argument. The bare
// NpmPackagePattern (no version) still covers the un-suffixed case; this is
// additive.
var NpmVersionedPattern = regexp.MustCompile(
	`^(@[a-z0-9-~][a-z0-9-._~]*/)?[a-z0-9-~][a-z0-9-._~]*(@[a-zA-Z0-9][a-zA-Z0-9.+-]*)?$`,
)

// VersionTokenPattern constrains a target version / git tag to a safe charset
// before it is concatenated into an `npm pack name@<v>` or git `remote#<v>` spec.
// It must start alphanumeric (so a leading '-' can't be read as a flag) and
// allows the semver/tag charset (digits, letters, dot, plus, hyphen) — covering
// `1.2.3`, `1.2.3-beta.1`, `v1.0.0`, `latest`. Anchored, no whitespace/slashes.
var VersionTokenPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.+-]*$`)

// shellUnsafePattern flags any character that has no business in a package
// spec. exec.Command uses no shell so these can't *execute*, but rejecting
// them keeps the surface tight and stops a leading dash from being parsed
// as an npm flag.
var shellUnsafePattern = regexp.MustCompile(`[\s;&|$<>` + "`" + `(){}\[\]'"\\]`)

// ParseManifestViaNode parses a manifest.ts by shelling out to Node, extracting
// the object literal with a targeted regex and evaluating it via vm.runInNewContext.
//
// NOTE: the vm context here is NOT a security sandbox. Node's `vm` is escapable
// and we make no attempt to airtight it — it's used only to evaluate a JS object
// literal in a clean global scope (scoping/robustness, not isolation). It does not
// need to be a security boundary: the same package's server/ Go is compiled into
// and runs AS the server (see runBuildPipelineWith), so member code already runs
// with full server privileges by design. Installs are gated to super-admins; the
// trust boundary is install authorization + source selection, not this parse.
func ParseManifestViaNode(packageDir string) (*ParsedManifest, error) {
	script := `
const fs = require('fs');
const vm = require('vm');
const dir = process.argv[1];

let content = '';
for (const ext of ['ts', 'js']) {
    const p = dir + '/manifest.' + ext;
    try { content = fs.readFileSync(p, 'utf-8'); break; } catch {}
}
if (!content) { console.error('No manifest found'); process.exit(1); }

// Extract the object literal from export default or const assignment
const m = content.match(/(?:export\s+default|module\.exports\s*=)\s*(\{[\s\S]*\})/) ||
          content.match(/(?:const|let|var)\s+\w+\s*=\s*(\{[\s\S]*\})\s*(?:;?\s*$)/m);
if (!m) { console.error('Could not parse manifest object'); process.exit(1); }

// Evaluate the object literal in a clean global scope (NOT a security sandbox —
// Node vm is escapable; this only avoids leaking Node globals into the literal).
try {
    const sandbox = Object.create(null);
    const obj = vm.runInNewContext('(' + m[1] + ')', sandbox, { timeout: 5000 });
    console.log(JSON.stringify(obj));
} catch (e) {
    console.error('Failed to evaluate manifest: ' + e.message);
    process.exit(1);
}
`
	cmd := exec.Command("node", "-e", script, packageDir)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("manifest parse failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to run node: %w", err)
	}

	return DecodeManifestJSON(out)
}

// DecodeManifestJSON decodes an ALREADY-EVALUATED manifest (the JSON a
// manifest evaluator emitted — this file's node script, the multi-org
// manifesteval subprocess, or an artifact's staged manifests/<slug>/
// manifest.json) into a ParsedManifest, populating the derived fields
// (RawJSON, HasServer) exactly like ParseManifestViaNode. It runs no code:
// use it wherever the evaluation already happened somewhere trusted.
func DecodeManifestJSON(data []byte) (*ParsedManifest, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid manifest JSON: %w", err)
	}

	var manifest ParsedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("manifest structure invalid: %w", err)
	}
	manifest.RawJSON = raw
	manifest.HasServer = manifest.Server != nil

	return &manifest, nil
}

// ValidateManifest performs the minimal pre-flight checks the install pipeline
// needs *before* it commits to a build: it confirms the manifest carries the
// identity fields the pipeline reads up front (name/slug/version), and enforces
// the env-specific gates the generator cannot make (slug shape + path-traversal
// safety since the slug feeds path construction, the no-server-toolchain gate,
// and slug collision with a bundled package).
//
// It deliberately does NOT re-validate manifest *shape* (nav, routes, settings,
// slots, …). Those fields are all optional and their well-formedness is the
// authority of the assemble step (`pnpm run packages:generate` → loadManifest →
// manifestToConfigPkg), which runs before the DB backup — so a malformed
// manifest fails assembly with live state untouched. Duplicating those rules
// here only let this copy drift out of sync (it once wrongly required `nav`,
// rejecting valid slot-only contributor packages).
//
// If allowServer is false (Phase 2), packages with server fields are rejected.
func ValidateManifest(m *ParsedManifest, allowServer bool, bundledSlugs map[string]bool) error {
	if m.Name == "" {
		return fmt.Errorf("manifest missing required field: name")
	}
	if m.Slug == "" {
		return fmt.Errorf("manifest missing required field: slug")
	}
	if !SlugPattern.MatchString(m.Slug) {
		return fmt.Errorf("invalid slug %q: must match ^[a-z0-9][a-z0-9-]*$", m.Slug)
	}
	if m.Version == "" {
		return fmt.Errorf("manifest missing required field: version")
	}

	// routes is optional (a slot-only / settings-only / publicRoutes-only
	// package declares none), but if present its directory feeds path
	// construction, so guard against traversal.
	if m.Routes != nil && strings.Contains(m.Routes.Directory, "..") {
		return fmt.Errorf("path traversal detected in directory: %s", m.Routes.Directory)
	}

	// Phase 2: reject packages with server components
	if !allowServer && m.HasServer {
		return fmt.Errorf("package %q has server components which require Phase 3 support", m.Slug)
	}

	// Validate server field if present
	if m.Server != nil {
		if m.Server.Module == "" {
			return fmt.Errorf("server.module is required when server field is present")
		}
		if m.Server.Package == "" {
			return fmt.Errorf("server.package is required when server field is present")
		}
		if strings.Contains(m.Server.Module, "..") {
			return fmt.Errorf("path traversal detected in server.module")
		}
		if strings.Contains(m.Server.Package, "..") {
			return fmt.Errorf("path traversal detected in server.package")
		}
		// Go module naming: must look like a Go import path
		if !goModulePattern.MatchString(m.Server.Module) {
			return fmt.Errorf("server.module %q doesn't look like a valid Go import path", m.Server.Module)
		}
	}

	// Check collision with bundled package slugs
	if bundledSlugs[m.Slug] {
		return fmt.Errorf("slug %q conflicts with a bundled package", m.Slug)
	}

	return nil
}

// ValidatePackageSpec checks that a string is a safe spec to hand to
// `npm pack`: either a bare npm package name or one of the git/URL forms
// npm pack understands natively. Rejects empty input, leading dashes
// (flag injection), and any shell-metacharacter / whitespace.
//
// A git spec may carry a trailing `#<ref>` to pin a tag/branch/commit (e.g.
// `github:tinycld/todo#v1.0.0`), which `npm pack` resolves by cloning at that
// ref. `#` isn't part of GitSpecPattern, so the bare spec and the ref are
// validated separately here: the bare part against GitSpecPattern, the ref
// against VersionTokenPattern (so it can't smuggle a flag or second arg).
// npm specs never contain `#`, so this only affects git forms — matching how
// classifySpec / specForVersion strip the ref downstream.
func ValidatePackageSpec(spec string) error {
	if spec == "" {
		return fmt.Errorf("package spec is required")
	}
	if strings.HasPrefix(spec, "-") {
		return fmt.Errorf("invalid package spec (leading dash): %s", spec)
	}
	if shellUnsafePattern.MatchString(spec) {
		return fmt.Errorf("invalid package spec (unsafe characters): %s", spec)
	}

	// Split off a trailing git ref (`#<ref>`) before pattern-matching. Only a
	// git-form bare spec may carry one; an npm name with a `#` falls through to
	// the final error.
	bare := spec
	if hash := strings.Index(spec, "#"); hash >= 0 {
		ref := spec[hash+1:]
		bare = spec[:hash]
		if !GitSpecPattern.MatchString(bare) {
			return fmt.Errorf("invalid package spec (only git specs may pin a #ref): %s", spec)
		}
		if !VersionTokenPattern.MatchString(ref) {
			return fmt.Errorf("invalid git ref %q in package spec: %s", ref, spec)
		}
		return nil
	}

	if NpmPackagePattern.MatchString(bare) || NpmVersionedPattern.MatchString(bare) || GitSpecPattern.MatchString(bare) {
		return nil
	}
	return fmt.Errorf("invalid package spec: %s", spec)
}

// IsTrustedScope returns true only for bare @tinycld/* npm names. Any git
// spec is third-party by definition and therefore untrusted, which makes
// the install pipeline emit its "proceed with caution" warning.
func IsTrustedScope(spec string) bool {
	return NpmPackagePattern.MatchString(spec) && strings.HasPrefix(spec, "@tinycld/")
}
