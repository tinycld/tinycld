package coreserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
)

// nativePlatform is one of the two native targets exported for OTA updates.
type nativePlatform string

const (
	platformIOS     nativePlatform = "ios"
	platformAndroid nativePlatform = "android"
)

// bundleMeta describes one exported platform bundle, persisted in the
// pkg_build `bundles` JSON field and surfaced verbatim by /api/app/update.
type bundleMeta struct {
	Platform       string      `json:"platform"`
	BundleID       string      `json:"bundle_id"`       // build-<ts>-<platform>
	BundleHash     string      `json:"bundle_hash"`     // hex sha256 of the .hbc
	BundleFile     string      `json:"bundle_file"`     // path of the .hbc relative to the release dir
	RuntimeVersion string      `json:"runtime_version"` // appVersion policy → app version
	Assets         []assetMeta `json:"assets"`
	distDir        string      `json:"-"` // export output dir, used to stage; not persisted
}

type assetMeta struct {
	Key         string `json:"key"`
	Hash        string `json:"hash"`
	ContentType string `json:"content_type"`
	File        string `json:"file"` // path relative to the release dir
}

// sha256OfFile returns the lowercase hex SHA-256 of the file at p.
func sha256OfFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// exportMetadata mirrors the subset of `expo export`'s metadata.json we read.
type exportMetadata struct {
	FileMetadata map[string]struct {
		Bundle string `json:"bundle"`
		Assets []struct {
			Path string `json:"path"`
			Ext  string `json:"ext"`
		} `json:"assets"`
	} `json:"fileMetadata"`
}

// parseExportMetadata reads <distDir>/metadata.json and builds the bundleMeta
// for one platform: hashing the .hbc and every asset, and resolving each
// asset's content type from its declared extension. File paths in the result
// are RELATIVE to distDir so they survive the dist→release-staging move.
func parseExportMetadata(distDir string, platform nativePlatform, buildID, runtimeVersion string) (bundleMeta, error) {
	raw, err := os.ReadFile(filepath.Join(distDir, "metadata.json"))
	if err != nil {
		return bundleMeta{}, fmt.Errorf("read metadata.json: %w", err)
	}
	var md exportMetadata
	if err := json.Unmarshal(raw, &md); err != nil {
		return bundleMeta{}, fmt.Errorf("parse metadata.json: %w", err)
	}
	pm, ok := md.FileMetadata[string(platform)]
	if !ok {
		return bundleMeta{}, fmt.Errorf("metadata.json has no %s platform", platform)
	}

	bundleHash, err := sha256OfFile(filepath.Join(distDir, pm.Bundle))
	if err != nil {
		return bundleMeta{}, fmt.Errorf("hash bundle: %w", err)
	}

	assets := make([]assetMeta, 0, len(pm.Assets))
	for _, a := range pm.Assets {
		hash, hErr := sha256OfFile(filepath.Join(distDir, a.Path))
		if hErr != nil {
			return bundleMeta{}, fmt.Errorf("hash asset %s: %w", a.Path, hErr)
		}
		ct := mime.TypeByExtension("." + a.Ext)
		if ct == "" {
			ct = "application/octet-stream"
		}
		assets = append(assets, assetMeta{
			Key:         a.Path,
			Hash:        hash,
			ContentType: ct,
			File:        a.Path,
		})
	}

	return bundleMeta{
		Platform:       string(platform),
		BundleID:       fmt.Sprintf("%s-%s", buildID, platform),
		BundleHash:     bundleHash,
		BundleFile:     pm.Bundle,
		RuntimeVersion: runtimeVersion,
		Assets:         assets,
	}, nil
}

// nativeToolchainPresent reports whether the RN toolchain needed to run `expo
// export` for native platforms is available to appDir. A web-only deploy image
// omits it; in that case we skip native export and the update endpoint returns
// 204 for mobile.
//
// Resolution must match how `npx expo export` actually finds expo: under pnpm's
// hoisted workspace layout the package is installed once at the WORKSPACE ROOT
// (<wsRoot>/node_modules/expo), not in the app member's own node_modules — the
// app's web export works precisely because Node walks up to the root. So a check
// that only stats <appDir>/node_modules/expo wrongly reports "absent" on a
// perfectly capable image and silently skips native export (mobile then never
// gets OTA updates). Check appDir first, then walk up to the workspace root.
func nativeToolchainPresent(appDir string) bool {
	candidates := []string{
		filepath.Join(appDir, "node_modules", "expo"),
		filepath.Join(filepath.Dir(appDir), "node_modules", "expo"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// copyFile copies the file at src to dst, creating dst if it does not exist.
// Both the file contents and the parent directory entry are fsync'd so a crash
// during install can't leave a build archive whose bundles metadata references a
// file that isn't durably on disk (which /api/app/update would then advertise but
// /api/app/bundle would 404 — a spurious client rollback).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dst))
}

// syncDir fsyncs a directory so a newly-created entry within it survives a crash.
// Opening a dir read-only and calling Sync is the portable way to flush the
// directory entry on the platforms we deploy to (Linux). Best-effort on systems
// that reject the open — the file content sync above is the load-bearing part.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// stageNativeBundlesIntoRelease copies each platform's bundle and assets out of
// its export dir into <releaseDir>/native/<platform>/<relative-path>, preserving
// the relative layout the bundleMeta paths reference. This makes the build
// archive self-contained: /api/app/bundle and /api/app/asset serve from here,
// and a revert restoring the archive restores the native bundles too.
func stageNativeBundlesIntoRelease(releaseDir string, bundles []bundleMeta) error {
	for _, bm := range bundles {
		dest := filepath.Join(releaseDir, "native", bm.Platform)
		files := []string{bm.BundleFile}
		for _, a := range bm.Assets {
			files = append(files, a.File)
		}
		for _, rel := range files {
			from := filepath.Join(bm.distDir, rel)
			to := filepath.Join(dest, rel)
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return err
			}
			if err := copyFile(from, to); err != nil {
				return fmt.Errorf("stage %s/%s: %w", bm.Platform, rel, err)
			}
			// Confirm the file the bundles metadata will reference is actually on
			// disk before the install proceeds to write the pkg_build row. The row
			// is the single source of truth /api/app/update serves from, so a
			// half-staged archive must abort the install rather than advertise a
			// bundle URL that 404s.
			if _, err := os.Stat(to); err != nil {
				return fmt.Errorf("staged %s/%s missing after copy: %w", bm.Platform, rel, err)
			}
		}
	}
	return nil
}

// exportNativeBundles runs `expo export` for ios and android (sequentially,
// after the caller's web export), parses each platform's metadata.json, and
// returns the per-platform bundleMeta. Returns (nil, nil) when the toolchain is
// absent. buildID is the server-generated build id (build-<UnixMilli>);
// runtimeVersion is the app version (appVersion policy).
func exportNativeBundles(job *installJob, appDir, buildID, runtimeVersion string) ([]bundleMeta, error) {
	if !nativeToolchainPresent(appDir) {
		emitProgress(job, "Native export skipped", progNativeStart, "RN toolchain absent — mobile served embedded bundle")
		return nil, nil
	}

	// A bundle whose runtime_version is empty can never match a real client (every
	// device reports a concrete app version), so it would be undeliverable dead
	// weight. The RN toolchain is present here, so we're committed to native
	// export — fail loudly rather than spend minutes building bundles no device
	// can receive. An empty version means app.json couldn't be read or has no
	// expo.version; see appVersionFromManifest.
	if runtimeVersion == "" {
		return nil, fmt.Errorf("empty runtimeVersion: app.json has no expo.version (or wasn't found near %s) — bundles would be undeliverable", appDir)
	}

	var out []bundleMeta
	platforms := []nativePlatform{platformIOS, platformAndroid}
	for i, p := range platforms {
		outDir := filepath.Join(appDir, fmt.Sprintf("dist-%s", p))
		os.RemoveAll(outDir) // clean any prior export
		// Carve the native band [progNativeStart, progNativeEnd) into one
		// sub-band per platform so each export climbs its own slice from Metro's
		// bundling progress, without ever exceeding the pipeline's native ceiling.
		span := progNativeEnd - progNativeStart
		lo := progNativeStart + (span*i)/len(platforms)
		hi := progNativeStart + (span*(i+1))/len(platforms)
		step := "Building " + string(p) + " bundle"
		emitProgress(job, step, lo, "Running expo export --platform "+string(p))
		// --source-maps external emits a <bundle>.hbc.map next to each .hbc so we can
		// upload it to Sentry below (mirrors the web export's --source-maps external
		// in the Dockerfile). Without it an OTA-bundle crash arrives as unsymbolicated
		// minified frames — the gap that made this incident's crash unreadable.
		// runExportWithProgress streams Metro's per-module progress onto [lo, hi).
		if cmdOut, err := runExportWithProgress(job, lo, hi, step, appDir,
			"--platform", string(p), "--source-maps", "external", "--output-dir", outDir); err != nil {
			return nil, fmt.Errorf("expo export %s: %v: %s", p, err, cmdOut)
		}
		bm, err := parseExportMetadata(outDir, p, buildID, runtimeVersion)
		if err != nil {
			return nil, fmt.Errorf("parse %s export: %w", p, err)
		}
		bm.distDir = outDir
		// Upload this bundle's sourcemaps to Sentry keyed by dist = bundle_id, so a
		// crash in this OTA bundle symbolicates. The client tags the same release +
		// dist (see sentry.ts). Best-effort: a failed/skipped upload must NOT fail
		// the install — the bundle is still deliverable, just harder to debug.
		uploadBundleSourcemaps(job, outDir, runtimeVersion, bm.BundleID)
		out = append(out, bm)
	}
	return out, nil
}

// orDefault returns v when non-empty, else def.
func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// sentryReleaseFor returns the Sentry `release` for an OTA bundle. It must match
// what the running app reports in sentry.ts EXACTLY or symbolication silently
// fails — both sides derive it the SAME way: "tinycld@<app-version>". We
// deliberately do NOT include the native build number (the server doesn't know
// it, and EAS auto-increments it); per-bundle disambiguation is carried by `dist`
// (the bundle id), not the release.
func sentryReleaseFor(runtimeVersion string) string {
	return "tinycld@" + runtimeVersion
}

// uploadBundleSourcemaps uploads the .hbc + .map under outDir to Sentry via
// sentry-cli, keyed by (release, dist). No-op (logs) when the Sentry auth token
// is unset — self-hosters without Sentry build normally. Never returns an error:
// the install succeeds whether or not the upload does. All Sentry config comes
// from system settings (the system-wide source of truth), not env vars.
func uploadBundleSourcemaps(job *installJob, outDir, runtimeVersion, bundleID string) {
	authToken := systemConfig.Get("sentry.auth_token")
	if authToken == "" {
		jobLogf(job, "sourcemap upload skipped for %s (sentry.auth_token unset)", bundleID)
		return
	}
	release := sentryReleaseFor(runtimeVersion)
	// org/project: ios/sentry.properties is prebuild output and is NOT in the
	// server's build tree, so sentry-cli can't discover them — pass explicitly.
	// Configurable via system settings but defaulted to the known argosity/tinycld
	// project so a standard deploy needs only the auth token.
	org := orDefault(systemConfig.Get("sentry.org"), "argosity")
	project := orDefault(systemConfig.Get("sentry.project"), "tinycld")
	// sentry-cli reads the auth token from SENTRY_AUTH_TOKEN in its env. Pass it
	// via runCmdEnv (NOT a CLI flag) so the secret stays out of the logged args.
	jsDir := filepath.Join(outDir, "_expo", "static", "js")
	out, err := runCmdEnv(outDir, []string{"SENTRY_AUTH_TOKEN=" + authToken},
		"pnpm", "exec", "sentry-cli", "sourcemaps", "upload",
		"--org", org, "--project", project,
		"--release", release, "--dist", bundleID, jsDir)
	if err != nil {
		// Surface to the job log AND Sentry-less stderr, but swallow: a debugging
		// aid failing must never brick a working install.
		jobLogf(job, "sourcemap upload FAILED for %s (release=%s): %v: %s", bundleID, release, err, out)
		return
	}
	jobLogf(job, "uploaded sourcemaps for %s (release=%s)", bundleID, release)
}

// cleanupNativeExportDirs removes the per-platform `dist-<platform>` export dirs
// after their contents have been staged into the build archive. Best-effort —
// a failure here is non-fatal (the next export's os.RemoveAll clears it anyway),
// so errors are ignored rather than aborting a successful install.
func cleanupNativeExportDirs(bundles []bundleMeta) {
	for _, b := range bundles {
		if b.distDir != "" {
			os.RemoveAll(b.distDir)
		}
	}
}

// appVersionFromManifest resolves the Expo app version, which is the
// runtimeVersion under the appVersion policy. The app's single source of truth
// is package.json → version: app.config.ts injects it over a static app.json
// that intentionally carries no expo.version (see app.config.ts). We mirror that
// here rather than evaluate the TS config — read expo.version from app.json
// first (in case a build pins it statically), then fall back to package.json's
// version. Each file may sit at appDir in the runtime image or one level up in
// dev, so try appDir then its parent for both.
func appVersionFromManifest(appDir string) string {
	bases := []string{appDir, filepath.Dir(appDir)}

	for _, base := range bases {
		raw, err := os.ReadFile(filepath.Join(base, "app.json"))
		if err != nil {
			continue
		}
		var cfg struct {
			Expo struct {
				Version string `json:"version"`
			} `json:"expo"`
		}
		if json.Unmarshal(raw, &cfg) == nil && cfg.Expo.Version != "" {
			return cfg.Expo.Version
		}
	}

	// app.json had no expo.version — the normal case here, since the version
	// lives in package.json and is merged in by app.config.ts at config time.
	for _, base := range bases {
		raw, err := os.ReadFile(filepath.Join(base, "package.json"))
		if err != nil {
			continue
		}
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(raw, &pkg) == nil && pkg.Version != "" {
			return pkg.Version
		}
	}

	return ""
}

// serializeBundles converts the typed metadata into the []any shape PocketBase
// stores in the `bundles` JSON field. Returns an empty (non-nil) slice so the
// stored value is always a JSON array.
func serializeBundles(bundles []bundleMeta) []any {
	out := make([]any, 0, len(bundles))
	for _, b := range bundles {
		assets := make([]any, 0, len(b.Assets))
		for _, a := range b.Assets {
			assets = append(assets, map[string]any{
				"key": a.Key, "hash": a.Hash, "content_type": a.ContentType, "file": a.File,
			})
		}
		out = append(out, map[string]any{
			"platform": b.Platform, "bundle_id": b.BundleID, "bundle_hash": b.BundleHash,
			"bundle_file": b.BundleFile, "runtime_version": b.RuntimeVersion, "assets": assets,
		})
	}
	return out
}
