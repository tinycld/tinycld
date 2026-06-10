package coreserver

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// buildIDPattern matches the only build_id shapes the install pipeline mints:
// `build-<unixMilli>` and the seed `build-base`. serveBuildFile interpolates the
// build id into the archive path, so it MUST be validated against this before
// joining — Go's mux percent-decodes path segments, so an un-validated id like
// `..%2f..%2f..` would otherwise let a public, pre-auth request escape the
// builds dir and read arbitrary files. (See buildArchiveFor's contract note.)
var buildIDPattern = regexp.MustCompile(`^build-(\d+|base)$`)

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
// manifestUpToDate when the client already runs this bundle, else manifestNew
// with the populated (URL-less) manifest. `bundles` is the pkg_build record's
// bundles field decoded as []any.
//
// "Already runs this bundle" is true when EITHER the bundle_id equals currentID
// OR the bundle_hash equals currentHash. The hash check is what spares a fresh
// App Store install (whose currentID is the embedded `embedded-<version>`, never
// equal to a server `build-<ts>-<platform>` id) from a guaranteed download +
// reload on first foreground: when the embedded bytecode is identical to the
// server's current bundle, the hashes match and we report up-to-date. currentHash
// may be empty (older clients / hash unavailable) — then only the id check applies.
func resolveManifest(bundles []any, platform, runtimeVersion, currentID, currentHash string) (clientManifest, manifestStatus) {
	for _, raw := range bundles {
		b, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if str(b["platform"]) != platform || str(b["runtime_version"]) != runtimeVersion {
			continue
		}
		id := str(b["bundle_id"])
		hash := str(b["bundle_hash"])
		if id == currentID || (currentHash != "" && hash == currentHash) {
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
			BundleHash:     hash,
			Assets:         assets,
		}, manifestNew
	}
	return clientManifest{}, manifestNoMatch
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// manifestStatusName renders a manifestStatus for the debug log.
func manifestStatusName(s manifestStatus) string {
	switch s {
	case manifestNoMatch:
		return "no-match"
	case manifestUpToDate:
		return "up-to-date"
	case manifestNew:
		return "new"
	default:
		return "unknown"
	}
}

// summarizeBundles renders the per-platform bundle metadata (platform, id, hash,
// runtime) for the /api/app/update debug log without dumping the full asset
// lists. Each entry is "platform=…,id=…,hash=…,runtime=…".
func summarizeBundles(bundles []any) []string {
	out := make([]string, 0, len(bundles))
	for _, raw := range bundles {
		b, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, fmt.Sprintf("platform=%s,id=%s,hash=%s,runtime=%s",
			str(b["platform"]), str(b["bundle_id"]), str(b["bundle_hash"]), str(b["runtime_version"])))
	}
	return out
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
	if err := rec.UnmarshalJSONField("bundles", &bundles); err != nil {
		// A malformed bundles field shouldn't happen (serializeBundles always
		// writes a JSON array), but if it does we'd otherwise silently serve 204
		// to every mobile client forever. Log it so the cause is visible rather
		// than presenting as "updates mysteriously never arrive".
		app.Logger().Error("app-update: failed to decode current build bundles",
			"build_id", rec.GetString("build_id"), "err", err)
		return rec.GetString("build_id"), nil
	}
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
			currentHash := re.Request.URL.Query().Get("currentHash")

			// Verbose per-request debug log: who asked, with what client state, and
			// what the server knows. Lets the OTA flow be traced end-to-end from
			// `docker logs` (and lets the install e2e assert on the decision). Kept at
			// Info so it's visible without enabling debug-level logging.
			buildID, bundles := currentBuildBundles(app)
			app.Logger().Info("app-update: request",
				"method", re.Request.Method,
				"path", re.Request.URL.RequestURI(),
				"remoteAddr", re.Request.RemoteAddr,
				"realIP", re.RealIP(),
				"userAgent", re.Request.UserAgent(),
				"q.platform", platform,
				"q.runtimeVersion", runtime,
				"q.currentId", currentID,
				"q.currentHash", currentHash,
				"server.currentBuildId", buildID,
				"server.bundleCount", len(bundles),
				"server.bundles", summarizeBundles(bundles),
			)

			if platform == "" || runtime == "" {
				app.Logger().Info("app-update: response 400 (missing platform/runtimeVersion)",
					"q.platform", platform, "q.runtimeVersion", runtime)
				return re.BadRequestError("platform and runtimeVersion are required", nil)
			}
			if buildID == "" {
				app.Logger().Info("app-update: response 204 (no current build / no bundles)")
				return re.NoContent(204)
			}
			m, status := resolveManifest(bundles, platform, runtime, currentID, currentHash)
			if status != manifestNew {
				app.Logger().Info("app-update: response 204 (no new bundle)",
					"status", manifestStatusName(status),
					"q.platform", platform, "q.runtimeVersion", runtime,
					"q.currentId", currentID, "q.currentHash", currentHash)
				return re.NoContent(204)
			}
			fillManifestURLs(&m, buildID, platform)
			app.Logger().Info("app-update: response 200 (update available)",
				"manifest.id", m.ID,
				"manifest.runtimeVersion", m.RuntimeVersion,
				"manifest.bundleHash", m.BundleHash,
				"manifest.bundleUrl", m.BundleURL,
				"manifest.assetCount", len(m.Assets),
			)
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

	// Both segments are interpolated into the FS root below, so validate them
	// against fixed shapes BEFORE joining. os.DirFS + fs.ValidPath only confine
	// the `rest` wildcard, not the root itself — a percent-decoded `..` in
	// buildID/platform would otherwise escape the builds dir on this public path.
	if platform != string(platformIOS) && platform != string(platformAndroid) {
		return re.NotFoundError("", nil)
	}
	if !buildIDPattern.MatchString(buildID) {
		return re.NotFoundError("", nil)
	}

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
