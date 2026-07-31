package coreserver

// The native OTA export itself lives in pkgbuild/nativeexport.go. What stays
// here is the persistence-side helper: converting the pipeline's typed bundle
// metadata into the shape the pkg_build JSON field stores.

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
