package pkgbuild

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultPnpmStoreDir is the fixed content-addressable store baked into the
// single-tenant runtime image. Reusing it makes a per-build `pnpm install`
// hardlink-fast instead of re-downloading the ~2GB dependency graph. See the
// Dockerfile store comment. Hosts with a different layout (the multi-org
// builder) override it via ScaffoldOptions.
const DefaultPnpmStoreDir = "/workspace/.pnpm-store"

// PackageManagerSpec pins pnpm for every assembled build — the exact spec
// (version + integrity) written into the assembled root's packageManager
// field. Kept in sync with the canonical workspace-root package.json. It is
// also a RecipeHash input: two builds installed by different pnpm versions
// are different artifacts.
const PackageManagerSpec = "pnpm@11.3.0+sha512.2c403d6594527287672b1f7056343a1f7c3634036a67ffabfcc2b3d7595d843768f8787148d1b57cf7956c90606bbd192857c363af19e96d2d0ec9ec5741d215"

const postinstallScript = "tsx scripts/link-members.ts && cd tinycld && pnpm run packages:generate && cd .. && tsx scripts/link-members.ts && cd tinycld && pnpm run assets:copy-pdfjs"

// scaffoldExtras are the workspace-root files (beyond the two generated
// manifests) a build needs but that aren't fetched per-member: the link-members
// script, the package-enumeration helper, the shared test stubs, and .npmrc.
// They are copied verbatim from srcRoot (the active build's root, or the
// builder's scaffold source).
var scaffoldExtras = []string{".npmrc", "tinycld.packages.ts", "scripts", "tests", OverridesFile}

// OverridesFile is the workspace-root data file holding the framework/native/
// styling version pins. It is a DERIVED copy of tinycld/core/package-versions.json
// (the source of truth, committed with core), written on dev machines by
// tinycld/scripts/write-workspace-root.ts and copied verbatim from srcRoot into
// each new build (via scaffoldExtras) so OTA pins stay frozen at the active
// build's versions — see ServerRebuildEnv in pipeline.go.
const OverridesFile = "package-versions.json"

// ScaffoldOptions carries the host-specific knobs of the workspace scaffold.
// The zero value is the single-tenant host's configuration.
type ScaffoldOptions struct {
	// PnpmStoreDir is the pnpm content-addressable store the assembled
	// workspace points at; empty means DefaultPnpmStoreDir.
	PnpmStoreDir string
}

func (o ScaffoldOptions) storeDir() string {
	if o.PnpmStoreDir == "" {
		return DefaultPnpmStoreDir
	}
	return o.PnpmStoreDir
}

// MemberSource materializes workspace members into a build dir. It is the
// host seam the design doc names: the single-tenant host fetches changed
// members and copies unchanged ones from its currently-active build; the
// multi-org builder always fetches (it has no "current build").
//
// Both methods report the member's tarball integrity ("sha256:<hex>" of the
// exact bytes it was materialized from) — the fact RecipeHash keys the build
// cache on. CopyCurrent carries it forward from the active build's
// members.lock.json; "" means unknown (a current build that predates the
// lock), which RecipeHash refuses rather than silently omitting.
type MemberSource interface {
	// Fetch materializes a changed member from its spec (npm pack).
	Fetch(ms MemberSpec, buildDir string) (integrity string, err error)
	// CopyCurrent materializes an unchanged (FromCurrent) member from the
	// host's currently-active build.
	CopyCurrent(ms MemberSpec, buildDir string) (integrity string, err error)
}

// packFn packs spec (npm name / git URL / git+file://) and returns the path
// to the extracted "package" directory plus the tarball's integrity.
// Injectable for tests.
type packFn func(spec, workDir string) (extractedPackageDir, integrity string, err error)

type npmPackSource struct {
	pack packFn
}

// NpmPackSource returns the standard fetch-only MemberSource: `npm pack` +
// sha256 + untar. Its CopyCurrent always fails — only a host with a
// currently-active build can copy members from one, and such a host wraps
// this source with its own CopyCurrent (coreserver's hostMemberSource).
func NpmPackSource() MemberSource { return npmPackSource{pack: realPack} }

func (s npmPackSource) Fetch(ms MemberSpec, buildDir string) (string, error) {
	return fetchMemberWith(ms, buildDir, s.pack)
}

func (s npmPackSource) CopyCurrent(ms MemberSpec, buildDir string) (string, error) {
	return "", fmt.Errorf("npm pack source has no current build to copy member %q from", ms.Slug)
}

// realPack runs `npm pack <spec>` in a fresh temp dir, hashes the resulting
// tarball, untars it, and returns the extracted package/ path plus the
// tarball integrity. The hash is taken between pack and untar — the one
// moment the exact fetched bytes exist as a single artifact — so the recipe
// identity covers precisely what the build consumed.
func realPack(spec, _ string) (string, string, error) {
	tmp, err := os.MkdirTemp("", "tinycld-fetch-*")
	if err != nil {
		return "", "", err
	}
	if _, err := RunCmd(tmp, "npm", "pack", spec); err != nil {
		return "", "", fmt.Errorf("npm pack %s: %w", spec, err)
	}
	entries, _ := os.ReadDir(tmp)
	var tgz string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tgz") {
			tgz = e.Name()
			break
		}
	}
	if tgz == "" {
		return "", "", fmt.Errorf("no .tgz after npm pack %s", spec)
	}
	hash, err := sha256OfFile(filepath.Join(tmp, tgz))
	if err != nil {
		return "", "", fmt.Errorf("hash %s: %w", tgz, err)
	}
	if _, err := RunCmd(tmp, "tar", "xzf", tgz); err != nil {
		return "", "", fmt.Errorf("untar %s: %w", tgz, err)
	}
	// npm pack always extracts into a subdirectory named "package".
	return filepath.Join(tmp, "package"), "sha256:" + hash, nil
}

func fetchMemberWith(ms MemberSpec, buildDir string, pack packFn) (string, error) {
	extracted, integrity, err := pack(ms.Spec, buildDir)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(buildDir, ms.Slug)
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := CopyDir(extracted, dest); err != nil {
		return "", err
	}
	return integrity, nil
}

// AssembleBuild writes the manifest, materializes every member (fetching the
// changed ones, copying the unchanged ones via src.CopyCurrent), and writes
// the workspace scaffold into buildDir. After this the build dir is a complete
// pre-install workspace; the build pipeline turns it into a runnable one.
// srcRoot is where the scaffold extras (incl. the overrides file) are copied
// from — the host's active workspace root, or the builder's scaffold source.
func AssembleBuild(sink ProgressSink, m RebuildManifest, buildDir string, src MemberSource, srcRoot string, opts ScaffoldOptions) error {
	sink = sinkOrNop(sink)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return err
	}
	// Write the manifest FIRST so a crashed build is self-describing.
	mb, err := json.MarshalIndent(m, "", "    ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(buildDir, "manifest.json"), append(mb, '\n'), 0o644); err != nil {
		return err
	}
	members := make([]string, 0, len(m.Members))
	integrities := make(map[string]string, len(m.Members))
	for i, ms := range m.Members {
		// Tick the progress bar across the assemble band [ProgAssembleStart,
		// ProgAssembleEnd) as each member is materialized, BEFORE the work so a
		// slow npm-pack visibly parks the bar on the member it's fetching rather
		// than after it finishes.
		sink.Progress("Assembling build", assembleMemberPct(i, len(m.Members)), memberAssembleMsg(ms))
		memStart := time.Now()
		if ms.FromCurrent {
			integrity, err := src.CopyCurrent(ms, buildDir)
			if err != nil {
				return fmt.Errorf("copy %s from current: %w", ms.Slug, err)
			}
			integrities[ms.Slug] = integrity
			sink.Logf("member %s: copied from current build in %s", ms.Slug, sinceRounded(memStart))
		} else {
			sink.Logf("member %s: fetching %s", ms.Slug, ms.Spec)
			integrity, err := src.Fetch(ms, buildDir)
			if err != nil {
				return fmt.Errorf("fetch %s: %w", ms.Slug, err)
			}
			integrities[ms.Slug] = integrity
			sink.Logf("member %s: fetched in %s", ms.Slug, sinceRounded(memStart))
		}
		members = append(members, ms.Slug)
	}
	sink.Logf("assembled %d members; writing workspace scaffold", len(members))
	if err := WriteWorkspaceScaffold(buildDir, members, srcRoot, opts); err != nil {
		return err
	}
	// Record the RESOLVED member set (names + semvers from the on-disk
	// manifests, tarball integrities from the source) so the build carries its
	// own identity — the RecipeHash input, and the integrity carry-forward
	// source for the NEXT build's FromCurrent members.
	resolvedMembers, err := ResolveMembers(m, buildDir, integrities)
	if err != nil {
		return err
	}
	return WriteMembersLock(buildDir, resolvedMembers)
}

func sinceRounded(t time.Time) string {
	return time.Since(t).Round(time.Millisecond).String()
}

// assembleMemberPct maps member index i of n onto the assemble progress band
// [ProgAssembleStart, ProgAssembleEnd), so the bar climbs evenly as members are
// materialized. n is always >= 1 (every manifest carries tinycld).
func assembleMemberPct(i, n int) int {
	if n <= 0 {
		return ProgAssembleStart
	}
	span := ProgAssembleEnd - ProgAssembleStart
	return ProgAssembleStart + (span*i)/n
}

// memberAssembleMsg is the progress message for materializing one member —
// "Fetching <spec>" for a changed member, "Copying <slug>" for an unchanged one.
func memberAssembleMsg(ms MemberSpec) string {
	if ms.FromCurrent {
		return "Copying " + ms.Slug
	}
	return "Fetching " + ms.Spec
}

// WriteWorkspaceScaffold writes the workspace-root manifests into buildDir and
// copies the static scaffold extras from srcRoot (the active build's root).
// members is the ordered slug list of present members (must include "tinycld").
// The contents mirror the canonical assembled-root files; only the pnpm
// `packages:` list varies per build, plus the injected storeDir.
func WriteWorkspaceScaffold(buildDir string, members []string, srcRoot string, opts ScaffoldOptions) error {
	if err := writeRootPackageJSON(buildDir, members); err != nil {
		return err
	}
	// Copy the scaffold extras FIRST: writePnpmWorkspaceYAML reads
	// package-versions.json (a scaffold extra) to emit the `overrides:` block, so
	// the file must already be in buildDir when the YAML is generated.
	if err := copyScaffoldExtras(srcRoot, buildDir); err != nil {
		return err
	}
	return writePnpmWorkspaceYAML(buildDir, members, opts.storeDir())
}

// copyScaffoldExtras copies each scaffoldExtras entry from srcRoot to buildDir.
// A missing source entry is skipped (unit tests assemble without a full root);
// a real build's srcRoot always carries them.
func copyScaffoldExtras(srcRoot, buildDir string) error {
	for _, name := range scaffoldExtras {
		src := filepath.Join(srcRoot, name)
		info, err := os.Stat(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := copyPath(src, filepath.Join(buildDir, name), info.IsDir()); err != nil {
			return fmt.Errorf("copy scaffold %s: %w", name, err)
		}
	}
	return nil
}

// copyPath copies src→dst, dispatching to CopyDir for directories and a plain
// `cp -a` for single files (CopyDir's `src/.` form is directory-only).
func copyPath(src, dst string, isDir bool) error {
	if isDir {
		return CopyDir(src, dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_, err := RunCmd(".", "cp", "-a", src, dst)
	return err
}

// workspacePackages expands the member slug list into the pnpm `packages:`
// entries: tinycld carries its two nested members (core, package-scripts).
func workspacePackages(members []string) []string {
	var out []string
	for _, m := range members {
		out = append(out, m)
		if m == "tinycld" {
			out = append(out, "tinycld/core", "tinycld/package-scripts")
		}
	}
	return out
}

func writeRootPackageJSON(buildDir string, members []string) error {
	pkg := map[string]any{
		"name":            "@tinycld/workspace",
		"version":         "0.0.1",
		"private":         true,
		"type":            "module",
		"workspaces":      workspacePackages(members),
		"scripts":         map[string]any{"postinstall": postinstallScript},
		"devDependencies": map[string]any{"tsx": "^4.21.0"},
		"packageManager":  PackageManagerSpec,
	}
	b, err := json.MarshalIndent(pkg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(buildDir, "package.json"), append(b, '\n'), 0o644)
}

func writePnpmWorkspaceYAML(buildDir string, members []string, storeDir string) error {
	overrides, err := readOverrides(buildDir)
	if err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("nodeLinker: hoisted\n")
	sb.WriteString("linkWorkspacePackages: true\n")
	sb.WriteString("strictPeerDependencies: false\n")
	sb.WriteString("enablePrePostScripts: true\n")
	// Reuse the host's baked store so per-build installs are hardlink-fast.
	sb.WriteString(fmt.Sprintf("storeDir: %s\n", storeDir))
	sb.WriteString("\npackages:\n")
	for _, p := range workspacePackages(members) {
		sb.WriteString(fmt.Sprintf("  - %s\n", p))
	}
	sb.WriteString("\nallowBuilds:\n")
	sb.WriteString("  esbuild: true\n")
	sb.WriteString("  '@sentry/cli': true\n")
	sb.WriteString(renderOverridesBlock(overrides))
	return os.WriteFile(filepath.Join(buildDir, "pnpm-workspace.yaml"), []byte(sb.String()), 0o644)
}

// ReadOverrides loads the version pins from package-versions.json under root —
// the overrides input of RecipeHash. Exported for hosts that must compute the
// recipe hash from the scaffold source BEFORE a build dir exists (the multi-org
// builder's cache-hit check); RecipeHashForBuild covers the post-assemble case.
func ReadOverrides(root string) (map[string]string, error) {
	return readOverrides(root)
}

// readOverrides loads the version pins from package-versions.json in buildDir
// (copied there as a scaffold extra). The pins force the framework/native/
// styling stack to the embedded-binary versions so the OTA rebuild's
// `pnpm install --no-frozen-lockfile` can't drift them. The file is required in
// a real build (it's baked into the image and copied from the active build); a
// missing or malformed file is a hard error rather than a silently-unpinned
// install that recompiles classNames wrong on every device.
func readOverrides(buildDir string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(buildDir, OverridesFile))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", OverridesFile, err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", OverridesFile, err)
	}
	delete(m, "//") // documentation key, not a package
	if len(m) == 0 {
		return nil, fmt.Errorf("%s has no version pins", OverridesFile)
	}
	return m, nil
}

// renderOverridesBlock formats the pins as a pnpm `overrides:` YAML block,
// sorted for a stable, diff-friendly output. A package name containing a
// character YAML would otherwise interpret (the leading @ of a scope) is single-
// quoted; plain names are emitted bare, matching the hand-written committed root.
func renderOverridesBlock(overrides map[string]string) string {
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	var sb strings.Builder
	sb.WriteString("\noverrides:\n")
	for _, name := range names {
		key := name
		if strings.HasPrefix(name, "@") {
			key = "'" + name + "'"
		}
		sb.WriteString(fmt.Sprintf("  %s: %s\n", key, overrides[name]))
	}
	return sb.String()
}
