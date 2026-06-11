package coreserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pnpmStoreDir is the fixed content-addressable store baked into the runtime
// image. Reusing it makes a per-build `pnpm install` hardlink-fast instead of
// re-downloading the ~2GB dependency graph. See the Dockerfile store comment.
const pnpmStoreDir = "/workspace/.pnpm-store"

// packageManagerSpec pins pnpm for every assembled build. Kept in sync with the
// canonical workspace-root package.json.
const packageManagerSpec = "pnpm@11.3.0+sha512.2c403d6594527287672b1f7056343a1f7c3634036a67ffabfcc2b3d7595d843768f8787148d1b57cf7956c90606bbd192857c363af19e96d2d0ec9ec5741d215"

const postinstallScript = "tsx scripts/link-members.ts && cd tinycld && pnpm run packages:generate && cd .. && tsx scripts/link-members.ts && cd tinycld && pnpm run assets:copy-pdfjs"

// scaffoldExtras are the workspace-root files (beyond the two generated
// manifests) a build needs but that aren't fetched per-member: the link-members
// script, the package-enumeration helper, the shared test stubs, and .npmrc.
// They are copied verbatim from the active build's root.
var scaffoldExtras = []string{".npmrc", "tinycld.packages.ts", "scripts", "tests"}

// writeWorkspaceScaffold writes the workspace-root manifests into buildDir and
// copies the static scaffold extras from srcRoot (the active build's root).
// members is the ordered slug list of present members (must include "tinycld").
// The contents mirror the canonical assembled-root files; only the pnpm
// `packages:` list varies per build, plus the injected storeDir.
func writeWorkspaceScaffold(buildDir string, members []string) error {
	return writeWorkspaceScaffoldFrom(buildDir, members, currentWorkspaceRoot())
}

// currentWorkspaceRoot returns the workspace root of the running build: the
// binary lives at <root>/tinycld/tinycld, so resolveServerDir() == <root>/tinycld
// and its parent is the root.
func currentWorkspaceRoot() string {
	return filepath.Dir(resolveServerDir())
}

func writeWorkspaceScaffoldFrom(buildDir string, members []string, srcRoot string) error {
	if err := writeRootPackageJSON(buildDir, members); err != nil {
		return err
	}
	if err := writePnpmWorkspaceYAML(buildDir, members); err != nil {
		return err
	}
	return copyScaffoldExtras(srcRoot, buildDir)
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

// copyPath copies src→dst, dispatching to copyDir for directories and a plain
// `cp -a` for single files (copyDir's `src/.` form is directory-only).
func copyPath(src, dst string, isDir bool) error {
	if isDir {
		return copyDir(src, dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_, err := runCmd(".", "cp", "-a", src, dst)
	return err
}

// packFn packs spec (npm name / git URL / git+file://) and returns the path
// to the extracted "package" directory. Injectable for tests.
type packFn func(spec, workDir string) (extractedPackageDir string, err error)

// realPack runs `npm pack <spec>` in a fresh temp dir, untars the resulting
// tarball, and returns the extracted package/ path. Mirrors pkg_install.go's
// fetch step but for an arbitrary member spec.
func realPack(spec, _ string) (string, error) {
	tmp, err := os.MkdirTemp("", "tinycld-fetch-*")
	if err != nil {
		return "", err
	}
	if _, err := runCmd(tmp, "npm", "pack", spec); err != nil {
		return "", fmt.Errorf("npm pack %s: %w", spec, err)
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
		return "", fmt.Errorf("no .tgz after npm pack %s", spec)
	}
	if _, err := runCmd(tmp, "tar", "xzf", tgz); err != nil {
		return "", fmt.Errorf("untar %s: %w", tgz, err)
	}
	// npm pack always extracts into a subdirectory named "package".
	return filepath.Join(tmp, "package"), nil
}

func fetchMember(ms MemberSpec, buildDir string) error {
	return fetchMemberWith(ms, buildDir, realPack)
}

func fetchMemberWith(ms MemberSpec, buildDir string, pack packFn) error {
	extracted, err := pack(ms.Spec, buildDir)
	if err != nil {
		return err
	}
	dest := filepath.Join(buildDir, ms.Slug)
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return copyDir(extracted, dest)
}

// fetchFn fetches one member into buildDir. Injectable for tests.
type fetchFn func(ms MemberSpec, buildDir string) error

// assembleBuild writes the manifest, fetches every member by spec, and writes
// the workspace scaffold into buildDir. After this the build dir is a complete
// pre-install workspace; runBuildPipeline turns it into a runnable one.
func assembleBuild(m RebuildManifest, buildDir string) error {
	return assembleBuildWith(m, buildDir, fetchMember)
}

func assembleBuildWith(m RebuildManifest, buildDir string, fetch fetchFn) error {
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
	for _, ms := range m.Members {
		if err := fetch(ms, buildDir); err != nil {
			return fmt.Errorf("fetch %s: %w", ms.Slug, err)
		}
		members = append(members, ms.Slug)
	}
	return writeWorkspaceScaffold(buildDir, members)
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
		"packageManager":  packageManagerSpec,
	}
	b, err := json.MarshalIndent(pkg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(buildDir, "package.json"), append(b, '\n'), 0o644)
}

func writePnpmWorkspaceYAML(buildDir string, members []string) error {
	var sb strings.Builder
	sb.WriteString("nodeLinker: hoisted\n")
	sb.WriteString("linkWorkspacePackages: true\n")
	sb.WriteString("strictPeerDependencies: false\n")
	sb.WriteString("enablePrePostScripts: true\n")
	// Reuse the image's baked store so per-build installs are hardlink-fast.
	sb.WriteString(fmt.Sprintf("storeDir: %s\n", pnpmStoreDir))
	sb.WriteString("\npackages:\n")
	for _, p := range workspacePackages(members) {
		sb.WriteString(fmt.Sprintf("  - %s\n", p))
	}
	sb.WriteString("\nallowBuilds:\n")
	sb.WriteString("  esbuild: true\n")
	sb.WriteString("  '@sentry/cli': true\n")
	return os.WriteFile(filepath.Join(buildDir, "pnpm-workspace.yaml"), []byte(sb.String()), 0o644)
}
