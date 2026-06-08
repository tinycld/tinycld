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
