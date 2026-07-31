package coreserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// DB-backed compatibility gates. The solver itself (and the authoritative
// post-assemble verify) moved to pkgbuild/compat.go; what stays here are the
// pre-flight paths that read pkg_registry: the advisory check endpoint, the
// version-change apply gate, and the fresh-install gate.

// handleVersionsCheck validates a proposed set of version changes. Request body:
//
//	{ "changes": { "<slug>": "<targetVersion>", ... } }
//
// The resolved version map is every installed package's current version with the
// proposed changes overlaid, plus @tinycld/core. peerVersions for each package
// come from its currently-installed manifest_json. Responds:
//
//	{ "ok": bool, "violations": [ { package, requires, range, found }, ... ] }
func handleVersionsCheck(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		Changes map[string]string `json:"changes"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	violations, err := solveRegistryCompat(app, body.Changes)
	if err != nil {
		return re.InternalServerError("Failed to load package registry", err)
	}
	return re.JSON(http.StatusOK, map[string]any{
		"ok":         len(violations) == 0,
		"violations": violations,
	})
}

// solveRegistryCompat resolves the registry's current versions with the
// proposed changes overlaid (keyed by registry slug) and runs the solver
// against every installed package's declared peerVersions. Shared by the
// advisory check endpoint and the apply pre-flight gate so the two cannot
// drift.
func solveRegistryCompat(app core.App, changes map[string]string) ([]compatViolation, error) {
	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return nil, fmt.Errorf("load package registry for compat check: %w", err)
	}

	resolved := map[string]string{}
	peerVersionsBySlug := map[string]map[string]string{}

	for _, rec := range records {
		slug := rec.GetString("slug")
		if target, changing := changes[slug]; changing {
			resolved[slug] = target
		} else {
			resolved[slug] = rec.GetString("version")
		}
		if peers := peerVersionsFromManifest(rec.GetString("manifest_json")); len(peers) > 0 {
			peerVersionsBySlug[slug] = peers
		}
	}

	// Resolve @tinycld/core's version so peerVersions can constrain it. Prefer an
	// explicit 'core' registry row; otherwise leave it absent (a core constraint
	// then surfaces as a violation, which is the safe default).
	if coreVer, ok := resolved["core"]; ok {
		resolved[corePackageKey] = coreVer
	}

	return solveCompat(resolved, peerVersionsBySlug), nil
}

// checkVersionChangeCompat is the apply pipeline's pre-flight gate: the same
// solve the Versions UI runs, enforced server-side so posting straight to
// /versions/apply cannot bypass the UI's advisory check (docs/packages.md
// promises the server is authoritative — this is that check).
func checkVersionChangeCompat(app core.App, changes []versionChange) error {
	proposed := make(map[string]string, len(changes))
	for _, c := range changes {
		proposed[c.Slug] = c.TargetVersion
	}
	violations, err := solveRegistryCompat(app, proposed)
	if err != nil {
		return err
	}
	return compatError(violations)
}

// checkInstallCompat gates a fresh install on the incoming package's declared
// peerVersions BEFORE any workspace mutation or migration. It resolves the
// currently-installed version set from pkg_registry, overlays the incoming
// package at its own version, and runs the same solver the Versions UI uses —
// but here it also evaluates the NOT-yet-installed package's own requirements,
// which the install path never checked before (so a package with a hard relation
// into an absent dependency could install and silently roll its migration back,
// leaving no tables). A requirement on an absent package resolves to Found="" and
// is a violation. Returns nil when compatible.
func checkInstallCompat(app core.App, m *parsedManifest) error {
	if len(m.PeerVersions) == 0 {
		return nil
	}

	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return fmt.Errorf("load package registry for compat check: %w", err)
	}

	resolved := map[string]string{}
	for _, rec := range records {
		resolved[rec.GetString("slug")] = rec.GetString("version")
	}
	// Overlay the incoming package at its own version so a self-referential or
	// reflexive constraint resolves rather than reporting itself absent.
	resolved[m.Slug] = m.Version
	if coreVer, ok := resolved["core"]; ok {
		resolved[corePackageKey] = coreVer
	}

	violations := solveCompat(resolved, map[string]map[string]string{m.Slug: m.PeerVersions})
	return compatError(violations)
}
