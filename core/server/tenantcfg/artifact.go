package tenantcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tinycld.org/core/pkgbuild"
)

// The build-artifact provenance ABI (DESIGN-org-package-agency D4/D6). The
// multi-org builder writes these files into every committed artifact
// (builds/<hash>/); an artifact-booted tenant reads them from its own
// binary's directory to learn its built-in package set — the source for the
// pkg_registry reconciliation and the current-set half of a deploy proposal.
// One definition for the router's writer and the tenant's loader, like
// DeployResult.

// ArtifactRecipeFile is the provenance record in an artifact's root. Its
// presence marks a COMMITTED artifact: the builder's rename makes it appear
// atomically, and a directory without one is a crashed or in-flight stage.
const ArtifactRecipeFile = "recipe.json"

// ArtifactManifestsDir is the artifact subdirectory holding each feature
// member's evaluated manifest as manifests/<slug>/manifest.json — the same
// shape the package store materializes. Written by the builder's trusted
// parent from resolve-time evaluation, never by the confined build job. The
// base member ships no manifest.
const ArtifactManifestsDir = "manifests"

// ArtifactRecipe is the parsed shape of ArtifactRecipeFile. Its facts
// (members, integrities, toolchain, hash) are computed by the builder's
// TRUSTED parent during resolve — never copied from anything the confined
// build job reported — so they can be believed by everyone downstream: the
// router's capability wiring, the GC sweep, and the tenant's registry
// reconcile.
type ArtifactRecipe struct {
	RecipeHash     string                    `json:"recipeHash"`
	BuildID        string                    `json:"buildId"`
	BuiltAt        time.Time                 `json:"builtAt"`
	Toolchain      pkgbuild.Toolchain        `json:"toolchain"`
	Members        []pkgbuild.ResolvedMember `json:"members"`
	Overrides      map[string]string         `json:"overrides"`
	ReleaseID      string                    `json:"releaseId"`
	RuntimeVersion string                    `json:"runtimeVersion"`

	// Bundles indexes the native OTA bundles staged under the artifact's
	// pb_public/native/<platform>/. It is what a tenant's /api/app/update
	// serves from — the hosted analog of the single-tenant pkg_build row's
	// `bundles` field, which an org dir has no equivalent of. Empty on an
	// artifact built before this field existed, and on a build whose
	// toolchain skipped native export; both mean "no native bundle" (204),
	// never "serve something unverified".
	Bundles []pkgbuild.BundleMeta `json:"bundles,omitempty"`
}

// ArtifactRecipePath is where an artifact's recipe lives.
func ArtifactRecipePath(artifactDir string) string {
	return filepath.Join(artifactDir, ArtifactRecipeFile)
}

// ArtifactManifestPath is where a feature member's evaluated manifest lives
// inside an artifact.
func ArtifactManifestPath(artifactDir, slug string) string {
	return filepath.Join(artifactDir, ArtifactManifestsDir, slug, "manifest.json")
}

// LoadArtifactRecipe reads an artifact's recipe. ok is false when none exists
// — the discriminator between an artifact-built tenant binary (recipe.json is
// its sibling) and every other install (serve-org's shared binary, a dev
// build), which have no built-in set to reconcile from.
func LoadArtifactRecipe(artifactDir string) (rec ArtifactRecipe, ok bool, err error) {
	raw, err := os.ReadFile(ArtifactRecipePath(artifactDir))
	if err != nil {
		if os.IsNotExist(err) {
			return ArtifactRecipe{}, false, nil
		}
		return ArtifactRecipe{}, false, err
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return ArtifactRecipe{}, false, fmt.Errorf("parse %s: %w", ArtifactRecipePath(artifactDir), err)
	}
	return rec, true, nil
}
