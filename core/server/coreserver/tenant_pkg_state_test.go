package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/tenantcfg"
)

// newTenantPkgStateApp builds a test app with the pkg_install_log and
// pkg_registry collections the boot reconcile reads/writes.
func newTenantPkgStateApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	logCol := core.NewBaseCollection("pkg_install_log")
	logCol.Fields.Add(&core.SelectField{
		Name: "action", Required: true, MaxSelect: 1,
		Values: []string{"install", "uninstall", "enable", "disable", "revert", "version_change"},
	})
	logCol.Fields.Add(&core.TextField{Name: "pkg_slug", Required: true})
	logCol.Fields.Add(&core.TextField{Name: "npm_package"})
	logCol.Fields.Add(&core.SelectField{
		Name: "status", Required: true, MaxSelect: 1,
		Values: []string{"pending", "running", "success", "failed", "rolled_back"},
	})
	logCol.Fields.Add(&core.TextField{Name: "error", Max: 5000})
	logCol.Fields.Add(&core.TextField{Name: "job_id"})
	logCol.Fields.Add(&core.DateField{Name: "started_at"})
	logCol.Fields.Add(&core.DateField{Name: "completed_at"})
	if err := app.Save(logCol); err != nil {
		t.Fatalf("save pkg_install_log collection: %v", err)
	}

	regCol := core.NewBaseCollection("pkg_registry")
	regCol.Fields.Add(&core.TextField{Name: "slug", Required: true})
	regCol.Fields.Add(&core.TextField{Name: "name"})
	regCol.Fields.Add(&core.TextField{Name: "version"})
	regCol.Fields.Add(&core.TextField{Name: "npm_package"})
	regCol.Fields.Add(&core.TextField{Name: "description"})
	regCol.Fields.Add(&core.TextField{Name: "icon"})
	regCol.Fields.Add(&core.BoolField{Name: "has_server"})
	regCol.Fields.Add(&core.SelectField{
		Name: "status", Required: true, MaxSelect: 1,
		Values: []string{"bundled", "available", "installed", "disabled"},
	})
	regCol.Fields.Add(&core.JSONField{Name: "manifest_json"})
	regCol.Fields.Add(&core.NumberField{Name: "nav_order"})
	if err := app.Save(regCol); err != nil {
		t.Fatalf("save pkg_registry collection: %v", err)
	}
	return app
}

// writeDeployResultFile drops a deploy-result.json into orgDir the way the
// router's Deployer does.
func writeDeployResultFile(t *testing.T, orgDir string, res tenantcfg.DeployResult) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(orgDir, ".runtime"), 0o755); err != nil {
		t.Fatalf("mkdir .runtime: %v", err)
	}
	body, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal deploy result: %v", err)
	}
	if err := os.WriteFile(tenantcfg.DeployResultPath(orgDir), body, 0o644); err != nil {
		t.Fatalf("write deploy result: %v", err)
	}
}

// writeArtifact stages an artifact dir: recipe.json plus a staged evaluated
// manifest per feature member, the way the builder's trusted parent does.
func writeArtifact(t *testing.T, members []pkgbuild.ResolvedMember, manifests map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	recipe := tenantcfg.ArtifactRecipe{RecipeHash: "sha256:abc", Members: members}
	body, err := json.Marshal(recipe)
	if err != nil {
		t.Fatalf("marshal recipe: %v", err)
	}
	if err := os.WriteFile(tenantcfg.ArtifactRecipePath(dir), body, 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	for slug, manifest := range manifests {
		p := tenantcfg.ArtifactManifestPath(dir, slug)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir manifest dir: %v", err)
		}
		if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}
	return dir
}

func addLogRow(t *testing.T, app *tests.TestApp, action, slug, jobID, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_install_log")
	if err != nil {
		t.Fatalf("find pkg_install_log: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("action", action)
	rec.Set("pkg_slug", slug)
	rec.Set("job_id", jobID)
	rec.Set("status", status)
	rec.Set("started_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := app.Save(rec); err != nil {
		t.Fatalf("save install-log row: %v", err)
	}
	return rec
}

func addRegistryRow(t *testing.T, app *tests.TestApp, slug, version, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatalf("find pkg_registry: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("slug", slug)
	rec.Set("version", version)
	rec.Set("status", status)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save registry row: %v", err)
	}
	return rec
}

const todoManifest = `{"name":"Todo","slug":"todo","version":"1.0.0","description":"Todos",` +
	`"nav":{"label":"Todo","icon":"check","order":5},"server":{"package":"server","module":"tinycld.org/packages/todo"},` +
	`"peerVersions":{"@tinycld/core":">=0.0.4 <0.1.0"}}`

func baseAndTodoMembers() []pkgbuild.ResolvedMember {
	return []pkgbuild.ResolvedMember{
		{Slug: pkgbuild.BaseMemberSlug, Name: "@tinycld/core", Version: "0.0.4", Integrity: "sha256:aa"},
		{Slug: "todo", Name: "@tinycld/todo", Version: "1.0.0", Integrity: "sha256:bb"},
	}
}

func TestReconcileDeployResult_CommittedFinalizesRow(t *testing.T) {
	app := newTenantPkgStateApp(t)
	orgDir := t.TempDir()
	row := addLogRow(t, app, "install", "todo", "job_1", "running")
	writeDeployResultFile(t, orgDir, tenantcfg.DeployResult{
		JobID: "job_1", Status: tenantcfg.DeployCommitted, RecipeHash: "sha256:abc",
		CompletedAt: time.Now().UTC(),
	})

	if slug := reconcileDeployResult(app, orgDir); slug != "" {
		t.Fatalf("install commit should return no uninstalled slug, got %q", slug)
	}

	got, err := app.FindRecordById("pkg_install_log", row.Id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got.GetString("status") != "success" {
		t.Fatalf("status = %q, want success", got.GetString("status"))
	}
	if got.GetString("completed_at") == "" {
		t.Fatal("completed_at not set")
	}
	if _, err := os.Stat(tenantcfg.DeployResultPath(orgDir)); !os.IsNotExist(err) {
		t.Fatal("deploy result should be consumed after finalize")
	}
}

func TestReconcileDeployResult_RevertedMarksRolledBack(t *testing.T) {
	app := newTenantPkgStateApp(t)
	orgDir := t.TempDir()
	row := addLogRow(t, app, "install", "todo", "job_2", "running")
	writeDeployResultFile(t, orgDir, tenantcfg.DeployResult{
		JobID: "job_2", Status: tenantcfg.DeployReverted, Error: "boot failed: $os is not defined",
		RecipeHash: "sha256:prev", CompletedAt: time.Now().UTC(),
	})

	reconcileDeployResult(app, orgDir)

	got, err := app.FindRecordById("pkg_install_log", row.Id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got.GetString("status") != "rolled_back" {
		t.Fatalf("status = %q, want rolled_back", got.GetString("status"))
	}
	if got.GetString("error") != "boot failed: $os is not defined" {
		t.Fatalf("error = %q, want the router's reason", got.GetString("error"))
	}
	if _, err := os.Stat(tenantcfg.DeployResultPath(orgDir)); !os.IsNotExist(err) {
		t.Fatal("deploy result should be consumed")
	}
}

func TestReconcileDeployResult_NoFileIsNoop(t *testing.T) {
	app := newTenantPkgStateApp(t)
	orgDir := t.TempDir()
	row := addLogRow(t, app, "install", "todo", "job_3", "running")

	reconcileDeployResult(app, orgDir)

	got, _ := app.FindRecordById("pkg_install_log", row.Id)
	if got.GetString("status") != "running" {
		t.Fatalf("no result file must leave the row alone, got %q", got.GetString("status"))
	}
}

func TestReconcileDeployResult_NoMatchingRowConsumes(t *testing.T) {
	app := newTenantPkgStateApp(t)
	orgDir := t.TempDir()
	// Operator-driven deploy: result carries no job the log knows about.
	writeDeployResultFile(t, orgDir, tenantcfg.DeployResult{
		Status: tenantcfg.DeployCommitted, RecipeHash: "sha256:abc", CompletedAt: time.Now().UTC(),
	})

	reconcileDeployResult(app, orgDir)

	if _, err := os.Stat(tenantcfg.DeployResultPath(orgDir)); !os.IsNotExist(err) {
		t.Fatal("row-less deploy result should still be consumed")
	}
}

func TestReconcileDeployResult_CommittedUninstallReturnsSlug(t *testing.T) {
	app := newTenantPkgStateApp(t)
	orgDir := t.TempDir()
	addLogRow(t, app, "uninstall", "todo", "job_4", "running")
	writeDeployResultFile(t, orgDir, tenantcfg.DeployResult{
		JobID: "job_4", Status: tenantcfg.DeployCommitted, CompletedAt: time.Now().UTC(),
	})

	if slug := reconcileDeployResult(app, orgDir); slug != "todo" {
		t.Fatalf("committed uninstall should return its slug, got %q", slug)
	}
}

func TestReconcileDeployResult_RevertedUninstallReturnsNoSlug(t *testing.T) {
	app := newTenantPkgStateApp(t)
	orgDir := t.TempDir()
	addLogRow(t, app, "uninstall", "todo", "job_5", "running")
	writeDeployResultFile(t, orgDir, tenantcfg.DeployResult{
		JobID: "job_5", Status: tenantcfg.DeployReverted, Error: "build failed", CompletedAt: time.Now().UTC(),
	})

	// The uninstall never landed — the registry row must survive.
	if slug := reconcileDeployResult(app, orgDir); slug != "" {
		t.Fatalf("reverted uninstall must not report an uninstalled slug, got %q", slug)
	}
}

func TestReconcileRegistryFromArtifact_CreatesRows(t *testing.T) {
	app := newTenantPkgStateApp(t)
	dir := writeArtifact(t, baseAndTodoMembers(), map[string]string{"todo": todoManifest})

	if err := reconcileRegistryFromArtifact(app, dir, ""); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	coreRow, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'core'", nil)
	if err != nil {
		t.Fatalf("core row not created: %v", err)
	}
	if v := coreRow.GetString("version"); v != "0.0.4" {
		t.Fatalf("core version = %q, want the CORE version from the recipe", v)
	}
	if s := coreRow.GetString("status"); s != "bundled" {
		t.Fatalf("core status = %q, want bundled (uninstall stays hidden)", s)
	}
	if got := coreRow.GetString("npm_package"); got != "tinycld" {
		t.Fatalf("core npm_package = %q, want the app shell's npm name", got)
	}

	todoRow, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'todo'", nil)
	if err != nil {
		t.Fatalf("todo row not created: %v", err)
	}
	if v := todoRow.GetString("version"); v != "1.0.0" {
		t.Fatalf("todo version = %q", v)
	}
	if s := todoRow.GetString("status"); s != "installed" {
		t.Fatalf("todo status = %q, want installed (org admin may uninstall)", s)
	}
	if got := todoRow.GetString("npm_package"); got != "@tinycld/todo" {
		t.Fatalf("todo npm_package = %q, want the member's npm name", got)
	}
	if !todoRow.GetBool("has_server") {
		t.Fatal("todo has_server should come from the staged manifest's server block")
	}
	if icon := todoRow.GetString("icon"); icon != "check" {
		t.Fatalf("todo icon = %q, want nav icon from the manifest", icon)
	}
	// manifest_json must round-trip: the compat solver reads peerVersions here.
	var stored map[string]any
	if err := json.Unmarshal([]byte(todoRow.GetString("manifest_json")), &stored); err != nil {
		t.Fatalf("stored manifest_json unparsable: %v", err)
	}
	if _, ok := stored["peerVersions"]; !ok {
		t.Fatal("stored manifest_json lost peerVersions")
	}
}

func TestReconcileRegistryFromArtifact_UpdatesAndNormalizesStatus(t *testing.T) {
	app := newTenantPkgStateApp(t)
	// A pre-existing bundled row (e.g. an org whose DB predates hosted
	// reconcile) must normalize to installed so uninstall stays available.
	addRegistryRow(t, app, "todo", "0.9.0", "bundled")
	dir := writeArtifact(t, baseAndTodoMembers(), map[string]string{"todo": todoManifest})

	if err := reconcileRegistryFromArtifact(app, dir, ""); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	row, _ := app.FindFirstRecordByFilter("pkg_registry", "slug = 'todo'", nil)
	if v := row.GetString("version"); v != "1.0.0" {
		t.Fatalf("version not updated: %q", v)
	}
	if s := row.GetString("status"); s != "installed" {
		t.Fatalf("status = %q, want installed", s)
	}
}

func TestReconcileRegistryFromArtifact_DeletesUninstalledDisablesOthers(t *testing.T) {
	app := newTenantPkgStateApp(t)
	addRegistryRow(t, app, "todo", "1.0.0", "installed") // uninstalled by this deploy
	addRegistryRow(t, app, "mail", "0.5.0", "installed") // absent for another reason
	members := []pkgbuild.ResolvedMember{
		{Slug: pkgbuild.BaseMemberSlug, Name: "@tinycld/core", Version: "0.0.4"},
	}
	dir := writeArtifact(t, members, nil)

	if err := reconcileRegistryFromArtifact(app, dir, "todo"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if _, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'todo'", nil); err == nil {
		t.Fatal("uninstalled row should be deleted")
	}
	mail, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if err != nil {
		t.Fatalf("mail row should survive: %v", err)
	}
	if s := mail.GetString("status"); s != "disabled" {
		t.Fatalf("mail status = %q, want disabled (absent but not uninstalled)", s)
	}
}

func TestReconcileTenantPackageState_UninstallEndToEnd(t *testing.T) {
	app := newTenantPkgStateApp(t)
	orgDir := t.TempDir()
	addLogRow(t, app, "uninstall", "todo", "job_9", "running")
	addRegistryRow(t, app, "todo", "1.0.0", "installed")
	writeDeployResultFile(t, orgDir, tenantcfg.DeployResult{
		JobID: "job_9", Status: tenantcfg.DeployCommitted, CompletedAt: time.Now().UTC(),
	})
	dir := writeArtifact(t, []pkgbuild.ResolvedMember{
		{Slug: pkgbuild.BaseMemberSlug, Name: "@tinycld/core", Version: "0.0.4"},
	}, nil)

	reconcileTenantPackageState(app, orgDir, dir)

	row, err := app.FindFirstRecordByFilter("pkg_install_log", "job_id = 'job_9'", nil)
	if err != nil {
		t.Fatalf("log row: %v", err)
	}
	if s := row.GetString("status"); s != "success" {
		t.Fatalf("log status = %q, want success", s)
	}
	if _, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'todo'", nil); err == nil {
		t.Fatal("uninstalled registry row should be deleted by the end-to-end reconcile")
	}
}
