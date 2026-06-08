package coreserver

import (
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

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

// fillManifestURLs sets server-root-relative URLs for the bundle and each asset,
// keyed by the build id (the archive dir) and platform. buildID here is the
// pkg_build build_id (e.g. build-200), NOT the per-platform bundle_id.
func fillManifestURLs(m *clientManifest, buildID, platform string) {
	m.BundleURL = path.Join("/api/app/bundle", buildID, platform, m.BundleFile)
	for i := range m.Assets {
		m.Assets[i].URL = path.Join("/api/app/asset", buildID, platform, m.Assets[i].File)
	}
}

// currentBuildBundles loads the pkg_build "current" record and returns its
// build_id and decoded bundles field. Returns ("", nil) when there is no current
// build (fresh server, or web-only).
func currentBuildBundles(app core.App) (string, []any) {
	recs, err := app.FindRecordsByFilter("pkg_build", "status = 'current'", "-created", 1, 0)
	if err != nil || len(recs) == 0 {
		return "", nil
	}
	rec := recs[0]
	var bundles []any
	_ = rec.UnmarshalJSONField("bundles", &bundles)
	return rec.GetString("build_id"), bundles
}

// RegisterAppUpdateEndpoints wires the public OTA update endpoints: a JSON
// manifest check and static serving of bundle + asset files from the build
// archive. Public (no superuser guard) — the app calls these pre/post-auth.
func RegisterAppUpdateEndpoints(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/app")

		g.GET("/update", func(re *core.RequestEvent) error {
			platform := re.Request.URL.Query().Get("platform")
			runtime := re.Request.URL.Query().Get("runtimeVersion")
			currentID := re.Request.URL.Query().Get("currentId")
			if platform == "" || runtime == "" {
				return re.BadRequestError("platform and runtimeVersion are required", nil)
			}
			buildID, bundles := currentBuildBundles(app)
			if buildID == "" {
				return re.NoContent(204)
			}
			m, status := resolveManifest(bundles, platform, runtime, currentID)
			if status != manifestNew {
				return re.NoContent(204)
			}
			fillManifestURLs(&m, buildID, platform)
			return re.JSON(http.StatusOK, m)
		})

		g.GET("/bundle/{buildId}/{platform}/{path...}", func(re *core.RequestEvent) error {
			return serveBuildFile(re)
		})
		g.GET("/asset/{buildId}/{platform}/{path...}", func(re *core.RequestEvent) error {
			return serveBuildFile(re)
		})

		return e.Next()
	})
}

// serveBuildFile serves a file from <archive>/release/native/<platform>/<path>.
// os.DirFS roots the FS at that dir, so fs.Open confines reads to it — no manual
// traversal check needed (same approach as PoolAssets).
func serveBuildFile(re *core.RequestEvent) error {
	buildID := re.Request.PathValue("buildId")
	platform := re.Request.PathValue("platform")
	rest := re.Request.PathValue("path")

	appDir := resolveServerDir()
	base := filepath.Join(buildArchiveFor(appDir, buildID).release, "native", platform)
	fs := os.DirFS(base)
	f, err := fs.Open(rest)
	if err != nil {
		return re.NotFoundError("", nil)
	}
	f.Close()
	return re.FileFS(fs, rest)
}
