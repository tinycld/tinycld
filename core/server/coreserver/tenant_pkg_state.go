package coreserver

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/tenantcfg"
)

// Boot-time package-state reconciliation for an ARTIFACT-BOOTED tenant
// (DESIGN-org-package-agency §7 step 4). A hosted org never runs the
// single-tenant rebuild, so the two writes that pipeline makes before its
// exit-75 restart — finalizing the pkg_install_log row and mirroring the new
// member set into pkg_registry — have to happen on the other side of the
// restart discontinuity instead, from the two durable sources the deploy
// protocol leaves behind:
//
//   - .runtime/deploy-result.json (tenantcfg.DeployResult), the router's
//     record of the most recent deploy outcome. Consumed exactly once, after
//     its facts are durably recorded — the hosted analog of the single-tenant
//     .rollback-pending breadcrumb (ReconcileRolledBackInstall).
//   - recipe.json + manifests/<slug>/manifest.json beside the tenant's own
//     binary — the artifact's built-in set, written by the builder's trusted
//     parent. The artifact IS the org's installed set by construction, so the
//     registry is reconciled to it on EVERY boot (an operator-driven deploy
//     has no proposing tenant and leaves no install-log row; the registry
//     must still follow the build).

// reconcileTenantPackageState runs both reconciles. It is bound at OnServe
// (after RunAllMigrations, before serving) by RegisterTenant when the tenant
// is artifact-booted, and re-run lazily by the hosted status endpoints (see
// consumePendingDeployResult) because a COMMITTED deploy's result is written
// only after this very process's respawn settled — the boot reconcile ran
// before the file existed. Serialized: boot and a poll can race.
var tenantPkgStateMu sync.Mutex

func reconcileTenantPackageState(app core.App, orgDir, artifactDir string) {
	tenantPkgStateMu.Lock()
	defer tenantPkgStateMu.Unlock()
	uninstalledSlug := reconcileDeployResult(app, orgDir)
	if err := reconcileRegistryFromArtifact(app, artifactDir, uninstalledSlug); err != nil {
		srvLog.Error("tenant registry reconcile failed", "err", err)
	}
}

// consumePendingDeployResult runs the full reconcile iff a deploy result is
// waiting. The router's commit path writes the result AFTER the respawned
// tenant became ready (readiness IS the commit decision), so the boot-time
// reconcile cannot see a commit's outcome; the Packages UI's job-status poll
// — the thing waiting for that outcome — triggers the consume instead. Cheap
// when idle: one stat-shaped read.
func consumePendingDeployResult(app core.App, orgDir, artifactDir string) {
	if _, ok, err := tenantcfg.LoadDeployResult(orgDir); err != nil || !ok {
		return
	}
	reconcileTenantPackageState(app, orgDir, artifactDir)
}

// reconcileDeployResult finalizes the pkg_install_log row a deploy proposal
// left at "running" (the proposing process was killed before any outcome
// existed) from the router's durable deploy-result. Returns the registry slug
// a COMMITTED uninstall removed ("" otherwise) so the registry reconcile can
// delete that row rather than merely disabling it — the same distinction
// commitRegistry draws in the single-tenant pipeline.
//
// Idempotent + safe, mirroring ReconcileRolledBackInstall:
//   - no result file → no-op (normal boot);
//   - result names no row (operator deploy, or the row predates the log
//     collection) → consume, no-op;
//   - a transient failure recording the outcome keeps the file so the next
//     boot retries rather than losing the signal.
func reconcileDeployResult(app core.App, orgDir string) (uninstalledSlug string) {
	res, ok, err := tenantcfg.LoadDeployResult(orgDir)
	if err != nil {
		srvLog.Warn("unreadable deploy result, leaving for next boot", "err", err)
		return ""
	}
	if !ok {
		return ""
	}

	if _, cErr := app.FindCollectionByNameOrId("pkg_install_log"); cErr != nil {
		return "" // migrations not applied yet — leave the file for a later boot
	}

	status, errMsg := "success", ""
	switch res.Status {
	case tenantcfg.DeployCommitted:
	case tenantcfg.DeployReverted:
		// The router restored the pre-deploy snapshot and respawned the previous
		// build — the hosted analog of the entrypoint's health-check rollback,
		// so it gets the same terminal state.
		status, errMsg = "rolled_back", res.Error
		if errMsg == "" {
			errMsg = "deploy reverted: the proposed build failed to become ready"
		}
	default:
		srvLog.Warn("deploy result has unknown status, consuming", "status", res.Status)
		consumeDeployResult(orgDir)
		return ""
	}

	var row *core.Record
	if res.JobID != "" {
		row, _ = app.FindFirstRecordByFilter("pkg_install_log", "job_id = {:jobId}",
			map[string]any{"jobId": res.JobID})
	}
	if row == nil {
		// No proposing row to finalize (operator-driven deploy). Nothing to
		// record; the registry reconcile below still follows the artifact.
		consumeDeployResult(orgDir)
		return ""
	}

	if s := row.GetString("status"); s == "running" || s == "pending" {
		row.Set("status", status)
		row.Set("error", errMsg)
		row.Set("completed_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
		if err := app.Save(row); err != nil {
			srvLog.Warn("could not finalize install-log, retrying next boot", "recordID", row.Id, "err", err)
			return ""
		}
		srvLog.Info("finalized install-log", "recordID", row.Id, "pkgSlug", row.GetString("pkg_slug"), "status", status)
	}

	consumeDeployResult(orgDir)
	if res.Status == tenantcfg.DeployCommitted && row.GetString("action") == "uninstall" {
		return row.GetString("pkg_slug")
	}
	return ""
}

func consumeDeployResult(orgDir string) {
	if err := tenantcfg.ConsumeDeployResult(orgDir); err != nil {
		srvLog.Warn("could not consume deploy result", "err", err)
	}
}

// reconcileRegistryFromArtifact mirrors the artifact's built-in member set
// into pkg_registry — the hosted counterpart of commitRegistry, sourced from
// the artifact's trusted provenance instead of a build dir:
//
//   - every feature member gets a full row from its staged evaluated manifest
//     (created or updated), status "installed" — hosted rows are never
//     "bundled", because the org admin may uninstall any of them (goal 2);
//   - the base member maps to the "core" row (version = CORE's version, the
//     value peerVersions constrain), status "bundled" so the UI keeps hiding
//     uninstall controls for the platform itself;
//   - the row a committed uninstall removed is DELETED (it must leave the
//     admin list; its code is gone from the artifact);
//   - any other row absent from the artifact is disabled, not deleted (e.g.
//     an operator deploy that stepped past a package).
func reconcileRegistryFromArtifact(app core.App, artifactDir, uninstalledSlug string) error {
	recipe, ok, err := tenantcfg.LoadArtifactRecipe(artifactDir)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no %s in %s — not an artifact tenant", tenantcfg.ArtifactRecipeFile, artifactDir)
	}
	if _, err := app.FindCollectionByNameOrId("pkg_registry"); err != nil {
		return fmt.Errorf("pkg_registry collection unavailable: %w", err)
	}

	present := map[string]bool{}
	for _, member := range recipe.Members {
		regSlug := pkgbuild.MemberSlugToRegistry(member.Slug)
		present[regSlug] = true
		if err := upsertArtifactMemberRow(app, artifactDir, member); err != nil {
			return fmt.Errorf("reconcile %s: %w", regSlug, err)
		}
	}

	recs, err := app.FindAllRecords("pkg_registry")
	if err != nil {
		return err
	}
	for _, r := range recs {
		slug := r.GetString("slug")
		if present[slug] {
			continue
		}
		if slug == uninstalledSlug && uninstalledSlug != "" {
			if err := app.Delete(r); err != nil {
				return err
			}
			srvLog.Info("registry entry deleted (uninstalled)", "slug", slug)
			continue
		}
		if s := r.GetString("status"); s != "disabled" {
			r.Set("status", "disabled")
			if err := app.Save(r); err != nil {
				return err
			}
			srvLog.Info("registry entry disabled (absent from artifact)", "slug", slug)
		}
	}
	return nil
}

// upsertArtifactMemberRow writes one member's registry row from the artifact.
// The base member has no staged manifest — its row is synthesized (the same
// facts bundled-packages.json seeds in the single-tenant app), keyed to the
// core version recorded in the recipe. Feature members decode their staged
// evaluated manifest and reuse the installed-package upsert so the row shape
// (nav icon/order, has_server, manifest_json for the compat solver) cannot
// drift from the single-tenant path.
func upsertArtifactMemberRow(app core.App, artifactDir string, member pkgbuild.ResolvedMember) error {
	if member.Slug == pkgbuild.BaseMemberSlug {
		return upsertArtifactBaseRow(app, member)
	}

	data, err := os.ReadFile(tenantcfg.ArtifactManifestPath(artifactDir, member.Slug))
	if err != nil {
		return fmt.Errorf("read staged manifest: %w", err)
	}
	manifest, err := pkgbuild.DecodeManifestJSON(data)
	if err != nil {
		return err
	}
	// npm_package is the version-discovery + reinstall source, resolved through
	// the router's control socket in hosted mode (the tenant has no toolchain).
	// It must be the spec the member was FETCHED from, not merely its npm name:
	// an org installed from git upgrades against its remote's tags, and storing
	// the bare name here sent every lookup to the npm registry instead — which
	// 404s for packages that were never published there. SourceSpec falls back
	// to the name for artifacts built before the recipe carried a spec.
	if err := upsertPkgRegistry(app, manifest, member.SourceSpec(), data); err != nil {
		return err
	}
	// upsertPkgRegistry preserves a pre-existing "bundled" status (the
	// single-tenant upgrade semantics). Hosted rows must not be bundled — the
	// org admin may uninstall any feature — so normalize.
	rec, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:slug}",
		map[string]any{"slug": manifest.Slug})
	if err != nil {
		return err
	}
	if rec.GetString("status") != "installed" {
		rec.Set("status", "installed")
		return app.Save(rec)
	}
	return nil
}

// upsertArtifactBaseRow maintains the "core" registry row for the platform
// itself. Version is CORE's (what peerVersions ranges constrain — the same
// choice changedMemberVersion makes in the single-tenant pipeline);
// npm_package is the app shell's npm name, which is what a hosted core
// version change resolves versions against.
func upsertArtifactBaseRow(app core.App, member pkgbuild.ResolvedMember) error {
	manifestJSON, err := json.Marshal(map[string]string{
		"name":        "TinyCld Base",
		"slug":        baseRegistrySlug,
		"version":     member.Version,
		"description": "The TinyCld base — app shell, core library, and server.",
	})
	if err != nil {
		return err
	}

	rec, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:slug}",
		map[string]any{"slug": baseRegistrySlug})
	if err != nil {
		col, cErr := app.FindCollectionByNameOrId("pkg_registry")
		if cErr != nil {
			return cErr
		}
		rec = core.NewRecord(col)
		rec.Set("slug", baseRegistrySlug)
		rec.Set("name", "TinyCld Base")
		rec.Set("description", "The TinyCld base — app shell, core library, and server.")
	}
	rec.Set("version", member.Version)
	rec.Set("has_server", true)
	rec.Set("status", "bundled")
	// The shell's own fetch spec when the artifact records one (a git-installed
	// base upgrades from its remote's tags), else its npm name — the
	// pre-existing behaviour for artifacts built before Spec existed. Note the
	// base's SourceSpec falls back to Name, which is CorePackageKey
	// (@tinycld/core), so pin the historical BaseMemberSlug explicitly: the
	// shell publishes as `tinycld`, and that is what a core version change has
	// always resolved against.
	if member.Spec != "" {
		rec.Set("npm_package", member.Spec)
	} else {
		rec.Set("npm_package", pkgbuild.BaseMemberSlug)
	}
	rec.Set("manifest_json", json.RawMessage(manifestJSON))
	return app.Save(rec)
}
