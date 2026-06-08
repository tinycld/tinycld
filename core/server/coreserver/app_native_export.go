package coreserver

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
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
