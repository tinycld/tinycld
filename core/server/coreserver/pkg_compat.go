package coreserver

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Compatibility solver.
//
// A version change is applied as a SET: the operator selects several packages
// and target versions and applies them together. Each package version declares
// peerVersions — semver ranges it requires of other packages and @tinycld/core.
// The solver resolves the proposed set against the versions that will be live
// after the change (proposed targets override, untouched packages keep their
// current version) and reports every declared range the resolved set violates.
//
// peerVersions can differ between versions of the same package. The check
// endpoint and the apply pipeline's pre-flight gate both resolve peerVersions
// from the CURRENTLY installed manifests (pkg_registry.manifest_json) — a
// best-effort early guard. In addition, the apply pipeline re-checks each
// changing package's OWN peerVersions against the resolved set using the
// freshly-fetched target manifest (verifyTargetPeerVersions), right after the
// target files are swapped in — so a target version that tightens its own
// requirements is still caught before its migrations run.

const corePackageKey = "@tinycld/core"

// compatViolation is one unsatisfied peer requirement.
type compatViolation struct {
	Package  string `json:"package"`  // the package whose peerVersions declared the requirement
	Requires string `json:"requires"` // the peer slug/@tinycld/core that must match
	Range    string `json:"range"`    // the declared semver range
	Found    string `json:"found"`    // the version the resolved set provides (or "" if absent)
}

// solveCompat checks every entry of every package's peerVersions against the
// resolved version map. resolved maps package slug (and corePackageKey) to the
// version that will be live. peerVersionsBySlug maps a package slug to its
// declared peerVersions. A requirement on a package absent from resolved is a
// violation with Found "". Returns violations sorted for stable output.
func solveCompat(
	resolved map[string]string,
	peerVersionsBySlug map[string]map[string]string,
) []compatViolation {
	violations := []compatViolation{}

	slugs := make([]string, 0, len(peerVersionsBySlug))
	for slug := range peerVersionsBySlug {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		peers := peerVersionsBySlug[slug]
		peerKeys := make([]string, 0, len(peers))
		for k := range peers {
			peerKeys = append(peerKeys, k)
		}
		sort.Strings(peerKeys)

		for _, peerKey := range peerKeys {
			rangeStr := peers[peerKey]
			constraint, err := semver.NewConstraint(rangeStr)
			if err != nil {
				// An unparsable declared range is itself a violation — surface it
				// rather than silently passing.
				violations = append(violations, compatViolation{
					Package:  slug,
					Requires: peerKey,
					Range:    rangeStr,
					Found:    "",
				})
				continue
			}
			found, present := resolved[peerKey]
			if !present {
				violations = append(violations, compatViolation{
					Package: slug, Requires: peerKey, Range: rangeStr, Found: "",
				})
				continue
			}
			foundVer, verErr := semver.NewVersion(found)
			if verErr != nil || !constraint.Check(foundVer) {
				violations = append(violations, compatViolation{
					Package: slug, Requires: peerKey, Range: rangeStr, Found: found,
				})
			}
		}
	}
	return violations
}

// ---------- check endpoint ----------

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

	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return re.InternalServerError("Failed to load package registry", err)
	}

	resolved := map[string]string{}
	peerVersionsBySlug := map[string]map[string]string{}

	for _, rec := range records {
		slug := rec.GetString("slug")
		current := rec.GetString("version")
		if target, changing := body.Changes[slug]; changing {
			resolved[slug] = target
		} else {
			resolved[slug] = current
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

	violations := solveCompat(resolved, peerVersionsBySlug)
	return re.JSON(http.StatusOK, map[string]any{
		"ok":         len(violations) == 0,
		"violations": violations,
	})
}

// peerVersionsFromManifest extracts the peerVersions map from a stored
// manifest_json blob. Returns nil if absent or malformed.
func peerVersionsFromManifest(manifestJSON string) map[string]string {
	if manifestJSON == "" {
		return nil
	}
	var parsed struct {
		PeerVersions map[string]string `json:"peerVersions"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &parsed); err != nil {
		return nil
	}
	return parsed.PeerVersions
}

// verifyTargetPeerVersions re-checks ONE changing package's freshly-fetched
// target peerVersions against the resolved post-change version set. This catches
// a target version that tightens its own requirements beyond what its currently
// installed manifest declared (the pre-flight gate only sees installed
// manifests). resolved must already reflect every proposed change. Returns the
// violations contributed by this package's target manifest (empty if compatible).

// compatError renders violations into a human-readable multi-line message for
// pipeline failures (the UI uses the structured list; the pipeline logs prose).
