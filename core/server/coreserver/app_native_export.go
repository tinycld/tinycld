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

// nativeToolchainPresent reports whether the app dir carries enough of the RN
// toolchain to run `expo export` for native platforms. A web-only deploy image
// omits these; in that case we skip native export and the update endpoint just
// returns 204 for mobile.
func nativeToolchainPresent(appDir string) bool {
	info, err := os.Stat(filepath.Join(appDir, "node_modules", "expo"))
	return err == nil && info.IsDir()
}

// copyFile copies the file at src to dst, creating dst if it does not exist.
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
	return out.Sync()
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
		emitProgress(job, "Native export skipped", 89, "RN toolchain absent — mobile served embedded bundle")
		return nil, nil
	}

	var out []bundleMeta
	platforms := []nativePlatform{platformIOS, platformAndroid}
	for i, p := range platforms {
		outDir := filepath.Join(appDir, fmt.Sprintf("dist-%s", p))
		os.RemoveAll(outDir) // clean any prior export
		emitProgress(job, "Building "+string(p)+" bundle", 86+i, "Running expo export --platform "+string(p))
		if cmdOut, err := runCmd(appDir, "npx", "expo", "export", "--platform", string(p), "--output-dir", outDir); err != nil {
			return nil, fmt.Errorf("expo export %s: %v: %s", p, err, cmdOut)
		}
		bm, err := parseExportMetadata(outDir, p, buildID, runtimeVersion)
		if err != nil {
			return nil, fmt.Errorf("parse %s export: %w", p, err)
		}
		bm.distDir = outDir
		out = append(out, bm)
	}
	return out, nil
}

// appVersionFromManifest reads the Expo app version (app.json → expo.version),
// which is the runtimeVersion under the appVersion policy. app.json may sit at
// appDir in the runtime image or one level up in dev — try appDir then parent.
func appVersionFromManifest(appDir string) string {
	for _, base := range []string{appDir, filepath.Dir(appDir)} {
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
