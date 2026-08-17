package coreserver

import (
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/tenantcfg"
)

// RegisterTenantAppUpdateEndpoints wires the public OTA endpoints for an
// artifact-booted tenant (DESIGN-org-package-agency §6 "Native OTA per org").
//
// It serves the SAME routes with the SAME semantics as the host — only the two
// composition-specific seams differ:
//
//   - Bundle metadata comes from the artifact's recipe.json, not a pkg_build
//     row. An org dir has no build archive and no install pipeline, so nothing
//     would ever write that row; the builder records the bundles it staged
//     (derived parent-side by hashing the committed bytes) into the recipe.
//   - Files are served from the org's materialized pb_public/native/<platform>/,
//     which is where the build pipeline stages them and stageArtifact copies
//     them, rather than from builds/<id>/release/native/.
//
// Per-org is the correct granularity: each org runs its own build artifact with
// its own package set, so its bundle is genuinely different from another org's.
// Because the builder's build id is content-addressed (recipe-<hash12>), two
// orgs that resolve to the same package set advertise the SAME bundle id — so a
// device moving between them is correctly told "up to date" rather than being
// made to re-download identical bytes.
//
// The pkg_bad_bundle collection ships in every artifact's migrations, so the
// report-bad and boot-beacon endpoints work per-org with no schema change. Note
// the fleet signal is scoped to the org: a bundle one org's devices reported as
// crash-looping is not suppressed for a different org, even at the same recipe
// hash. That is the safer direction — a tenant cannot silence another tenant's
// updates — and each device still has its own local crash-rollback.
func RegisterTenantAppUpdateEndpoints(app core.App, orgDir, artifactDir string) {
	loader := &artifactBundles{artifactDir: artifactDir}
	registerAppUpdateEndpoints(app, appUpdateSources{
		bundles: loader.source,
		// The org's pb_public is materialized as a symlink into the artifact, so
		// serve through orgDir: it is the path the tenant is confined to, and it
		// follows a deploy's atomic repoint without this closure going stale.
		nativeRoot: func(string) string { return filepath.Join(orgDir, "pb_public") },
	})
}

// artifactBundles reads the tenant's own build recipe once and caches the
// result. An artifact is IMMUTABLE (content-addressed, committed by rename), and
// a deploy replaces the whole process rather than mutating the tree in place, so
// the recipe cannot change under a running tenant — re-reading and re-parsing it
// on every public, unauthenticated update check would be pure waste.
type artifactBundles struct {
	artifactDir string
	once        sync.Once
	buildID     string
	bundles     []any
}

// source adapts the recipe to the shape resolveManifest consumes ([]any of
// map[string]any, matching the pkg_build JSON field), by round-tripping the
// typed BundleMeta list through JSON. Reusing the untyped shape keeps the
// manifest decision byte-identical across both compositions rather than forking
// it for a second representation.
func (a *artifactBundles) source(app core.App) (string, []any) {
	a.once.Do(func() {
		recipe, ok, err := tenantcfg.LoadArtifactRecipe(a.artifactDir)
		if err != nil {
			// Not fatal: mobile stays on its embedded bundle. Log it, because
			// the alternative presentation is "updates mysteriously never
			// arrive" with nothing in the logs to explain why.
			srvLog.Error("app-update: failed to load artifact recipe",
				"artifactDir", a.artifactDir, "err", err)
			return
		}
		if !ok || len(recipe.Bundles) == 0 {
			// No recipe (not an artifact tenant) or no native bundles (web-only
			// toolchain, or an artifact built before bundles were recorded).
			// Both mean 204 — never serve something we cannot vouch for.
			return
		}
		raw, err := json.Marshal(recipe.Bundles)
		if err != nil {
			srvLog.Error("app-update: failed to encode artifact bundles",
				"artifactDir", a.artifactDir, "err", err)
			return
		}
		var decoded []any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			srvLog.Error("app-update: failed to decode artifact bundles",
				"artifactDir", a.artifactDir, "err", err)
			return
		}
		a.buildID, a.bundles = recipe.BuildID, decoded
	})
	return a.buildID, a.bundles
}
