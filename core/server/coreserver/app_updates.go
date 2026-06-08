package coreserver

type manifestStatus int

const (
	manifestNoMatch  manifestStatus = iota // no bundle for this platform+runtime → 204
	manifestUpToDate                       // current bundle id matches → 204
	manifestNew                            // a newer bundle is available → 200
)

// clientManifest is the JSON body /api/app/update returns when an update is
// available. Asset/bundle URLs are filled in by the HTTP handler (Task 8); the
// internal BundleFile/File fields carry the relative paths used to build them.
type clientManifest struct {
	ID             string          `json:"id"`
	RuntimeVersion string          `json:"runtimeVersion"`
	BundleFile     string          `json:"-"`
	BundleHash     string          `json:"bundleHash"`
	BundleURL      string          `json:"bundleUrl"`
	Assets         []manifestAsset `json:"assets"`
}

type manifestAsset struct {
	Key         string `json:"key"`
	Hash        string `json:"hash"`
	ContentType string `json:"contentType"`
	URL         string `json:"url"`
	File        string `json:"-"`
}

// resolveManifest finds the bundle for platform whose runtime_version matches
// runtimeVersion. Returns manifestNoMatch when none matches platform+runtime,
// manifestUpToDate when its bundle_id equals currentID, else manifestNew with
// the populated (URL-less) manifest. `bundles` is the pkg_build record's bundles
// field decoded as []any.
func resolveManifest(bundles []any, platform, runtimeVersion, currentID string) (clientManifest, manifestStatus) {
	for _, raw := range bundles {
		b, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if str(b["platform"]) != platform || str(b["runtime_version"]) != runtimeVersion {
			continue
		}
		id := str(b["bundle_id"])
		if id == currentID {
			return clientManifest{}, manifestUpToDate
		}
		assets := make([]manifestAsset, 0)
		if rawAssets, ok := b["assets"].([]any); ok {
			for _, ra := range rawAssets {
				a, ok := ra.(map[string]any)
				if !ok {
					continue
				}
				assets = append(assets, manifestAsset{
					Key:         str(a["key"]),
					Hash:        str(a["hash"]),
					ContentType: str(a["content_type"]),
					File:        str(a["file"]),
				})
			}
		}
		return clientManifest{
			ID:             id,
			RuntimeVersion: runtimeVersion,
			BundleFile:     str(b["bundle_file"]),
			BundleHash:     str(b["bundle_hash"]),
			Assets:         assets,
		}, manifestNew
	}
	return clientManifest{}, manifestNoMatch
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
