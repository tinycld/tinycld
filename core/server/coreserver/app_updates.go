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

// buildIDPattern matches the only build_id shapes either composition mints:
// the host installer's `build-<unixMilli>` and seed `build-base`, and the
// multi-org builder's content-addressed `recipe-<hash12>` (buildIDFor takes the
// first 12 hex chars of the recipe hash). serveBuildFile interpolates the build
// id into the archive path, so it MUST be validated against this before
// joining — Go's mux percent-decodes path segments, so an un-validated id like
// `..%2f..%2f..` would otherwise let a public, pre-auth request escape the
// builds dir and read arbitrary files. (See buildArchiveFor's contract note.)
//
// Keep this an exact-match allowlist of known shapes. Loosening it to something
// permissive (e.g. "no slashes") would re-open exactly the traversal this
// closes — the regression cases live in app_updates_http_test.go.
var buildIDPattern = regexp.MustCompile(`^(build-(\d+|base)|recipe-[a-f0-9]{12})$`)

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
//
// `badIDs`/`badHashes` are bundle ids/hashes clients reported as crash-looping
// (pkg_bad_bundle). A matching bundle is treated as if it doesn't exist (skipped),
// so a bundle that bricked a device is never advertised to the rest of the fleet.
func resolveManifest(
	bundles []any,
	platform, runtimeVersion, currentID, currentHash string,
	badIDs, badHashes map[string]bool,
) (clientManifest, manifestStatus) {
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
		// Skip a bundle the fleet reported as bad — don't push a known-bricking
		// bundle to anyone else. (A client already running it recovers via its own
		// local crash-rollback.)
		if badIDs[id] || (hash != "" && badHashes[hash]) {
			continue
		}
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
		srvLog.Error("app-update: failed to decode current build bundles",
			"buildID", rec.GetString("build_id"), "err", err)
		return rec.GetString("build_id"), nil
	}
	return rec.GetString("build_id"), bundles
}

// loadBadBundles returns the set of bundle ids and hashes clients have reported
// as crash-looping (the pkg_bad_bundle collection), for resolveManifest to skip.
// Best-effort: a query error logs and returns empty sets (fail OPEN — we'd rather
// risk re-offering a bad bundle, which each client's local rollback still catches,
// than block all updates on a reporting-table hiccup).
func loadBadBundles(app core.App) (ids map[string]bool, hashes map[string]bool) {
	ids = map[string]bool{}
	hashes = map[string]bool{}
	recs, err := app.FindRecordsByFilter("pkg_bad_bundle", "1=1", "", 0, 0)
	if err != nil {
		srvLog.Error("app-update: failed to load bad bundles", "err", err)
		return ids, hashes
	}
	for _, r := range recs {
		if id := r.GetString("bundle_id"); id != "" {
			ids[id] = true
		}
		if h := r.GetString("bundle_hash"); h != "" {
			hashes[h] = true
		}
	}
	return ids, hashes
}

// reportBadBody is the POST /api/app/update/report-bad payload: the bundle a
// client found to crash-loop, plus an optional error string for triage.
type reportBadBody struct {
	ID       string `json:"id"`
	Hash     string `json:"hash"`
	Platform string `json:"platform"`
	Error    string `json:"error"`
}

// recordBadBundle upserts a pkg_bad_bundle row for the reported bundle,
// incrementing its report count. Keyed by bundle_id (unique index). Returns the
// new report count.
func recordBadBundle(app core.App, body reportBadBody) (int, error) {
	var count int
	err := app.RunInTransaction(func(txApp core.App) error {
		existing, _ := txApp.FindFirstRecordByFilter(
			"pkg_bad_bundle", "bundle_id = {:id}", map[string]any{"id": body.ID},
		)
		if existing != nil {
			count = existing.GetInt("reports") + 1
			existing.Set("reports", count)
			if body.Error != "" {
				existing.Set("last_error", truncate(body.Error, 2000))
			}
			return txApp.Save(existing)
		}
		coll, err := txApp.FindCollectionByNameOrId("pkg_bad_bundle")
		if err != nil {
			return err
		}
		count = 1
		rec := core.NewRecord(coll)
		rec.Set("bundle_id", body.ID)
		rec.Set("bundle_hash", body.Hash)
		rec.Set("platform", body.Platform)
		rec.Set("reports", count)
		rec.Set("last_error", truncate(body.Error, 2000))
		return txApp.Save(rec)
	})
	return count, err
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// appUpdateSources supplies the two composition-specific pieces of the OTA
// endpoints; everything else (the manifest decision, the bad-bundle skip, the
// traversal hardening) is shared, so host and tenant can never drift apart on
// the parts that matter for correctness or safety.
type appUpdateSources struct {
	// bundles returns the build id and bundle metadata to serve from. The host
	// reads the pkg_build "current" row; a tenant reads its own build artifact's
	// recipe.json (an org dir has no build archive). ("", nil) means "nothing to
	// advertise" → 204.
	bundles func(core.App) (string, []any)

	// nativeRoot returns the directory holding <platform>/<file...> for a given
	// build id — the host's build archive, or the tenant's pb_public/native.
	nativeRoot func(buildID string) string
}

// RegisterAppUpdateEndpoints wires the public OTA update endpoints for the HOST
// composition: a JSON manifest check and static serving of bundle + asset files
// from the build archive. Public (no superuser guard) — the app calls these
// pre/post-auth. The tenant composition registers the same endpoints against
// its artifact via RegisterTenantAppUpdateEndpoints.
func RegisterAppUpdateEndpoints(app *pocketbase.PocketBase) {
	registerAppUpdateEndpoints(app, appUpdateSources{
		bundles: currentBuildBundles,
		nativeRoot: func(buildID string) string {
			return buildArchiveFor(resolveServerDir(), buildID).release
		},
	})
}

// registerAppUpdateEndpoints binds the shared handlers. It takes core.App
// rather than *pocketbase.PocketBase because that is all the handlers need —
// which also lets the HTTP tests drive the real registration against a
// tests.TestApp instead of re-implementing it.
func registerAppUpdateEndpoints(app core.App, src appUpdateSources) {
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
			buildID, bundles := src.bundles(app)
			srvLog.InfoContext(re.Request.Context(), "app-update: request",
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
				srvLog.InfoContext(re.Request.Context(), "app-update: response 400 (missing platform/runtimeVersion)",
					"q.platform", platform, "q.runtimeVersion", runtime)
				return re.BadRequestError("platform and runtimeVersion are required", nil)
			}
			if buildID == "" {
				srvLog.InfoContext(re.Request.Context(), "app-update: response 204 (no current build / no bundles)")
				return re.NoContent(204)
			}
			badIDs, badHashes := loadBadBundles(app)
			m, status := resolveManifest(bundles, platform, runtime, currentID, currentHash, badIDs, badHashes)
			if status != manifestNew {
				srvLog.InfoContext(re.Request.Context(), "app-update: response 204 (no new bundle)",
					"status", manifestStatusName(status),
					"q.platform", platform, "q.runtimeVersion", runtime,
					"q.currentId", currentID, "q.currentHash", currentHash)
				return re.NoContent(204)
			}
			fillManifestURLs(&m, buildID, platform)
			srvLog.InfoContext(re.Request.Context(), "app-update: response 200 (update available)",
				"manifest.id", m.ID,
				"manifest.runtimeVersion", m.RuntimeVersion,
				"manifest.bundleHash", m.BundleHash,
				"manifest.bundleUrl", m.BundleURL,
				"manifest.assetCount", len(m.Assets),
			)
			return re.JSON(http.StatusOK, m)
		})

		// A client whose freshly-applied bundle crash-looped reports it here so the
		// server stops advertising it to every other device. Public, like the rest
		// of /api/app — the app may report pre-auth. Idempotent (upsert by id).
		g.POST("/update/report-bad", func(re *core.RequestEvent) error {
			var body reportBadBody
			if err := re.BindBody(&body); err != nil {
				return re.BadRequestError("invalid body", err)
			}
			if body.ID == "" || (body.Platform != string(platformIOS) && body.Platform != string(platformAndroid)) {
				return re.BadRequestError("id and a valid platform are required", nil)
			}
			count, err := recordBadBundle(app, body)
			if err != nil {
				srvLog.ErrorContext(re.Request.Context(), "app-update: failed to record bad bundle",
					"bundleID", body.ID, "platform", body.Platform, "err", err)
				return re.InternalServerError("failed to record report", err)
			}
			srvLog.WarnContext(re.Request.Context(), "app-update: bundle reported bad",
				"bundleID", body.ID, "platform", body.Platform,
				"reports", count, "err", body.Error)
			return re.JSON(http.StatusOK, map[string]any{"ok": true, "reports": count})
		})

		// A freshly-booted bundle's JS posts here once the real provider tree has
		// mounted (BundleSentinel) — the proof the new bundle EXECUTED and rendered, not
		// just that the native loader promoted it. Public, like the rest of /api/app
		// (the app may post pre-auth). Logged at Info so the OTA e2e can read the beacon
		// from _logs; console.log can't be observed in a Release build, which is why this
		// server beacon exists.
		g.POST("/boot", func(re *core.RequestEvent) error {
			var body struct {
				ID       string `json:"id"`
				Platform string `json:"platform"`
				Hash     string `json:"hash"`
			}
			if err := re.BindBody(&body); err != nil {
				return re.BadRequestError("invalid body", err)
			}
			if body.ID == "" {
				return re.BadRequestError("id is required", nil)
			}
			// Deliberately NOT migrated to srvLog. The OTA e2e harness
			// (scripts/ota-e2e/boot-beacon-poller.ts) polls _logs with an exact
			// filter message='app-boot: rendered' and reads data.q.bundleId, so
			// this record's shape is release-verification infrastructure. The
			// fan-out would very likely preserve it, but "very likely" is not a
			// basis for risking a silent break in release verification.
			app.Logger().Info("app-boot: rendered",
				"q.bundleId", body.ID,
				"q.platform", body.Platform,
				"q.hash", body.Hash,
				"remoteAddr", re.Request.RemoteAddr,
			)
			return re.JSON(http.StatusOK, map[string]any{"ok": true})
		})

		g.GET("/bundle/{buildId}/{platform}/{path...}", func(re *core.RequestEvent) error {
			return serveBuildFile(re, src.nativeRoot)
		})
		g.GET("/asset/{buildId}/{platform}/{path...}", func(re *core.RequestEvent) error {
			return serveBuildFile(re, src.nativeRoot)
		})

		return e.Next()
	})
}

// serveBuildFile serves a file from <nativeRoot(buildID)>/native/<platform>/<path>.
// os.DirFS roots the FS at that dir, so fs.Open confines reads to it — no manual
// traversal check needed (same approach as PoolAssets).
func serveBuildFile(re *core.RequestEvent, nativeRoot func(string) string) error {
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

	base := filepath.Join(nativeRoot(buildID), "native", platform)
	fs := os.DirFS(base)
	f, err := fs.Open(rest)
	if err != nil {
		return re.NotFoundError("", nil)
	}
	f.Close()
	return re.FileFS(fs, rest)
}
