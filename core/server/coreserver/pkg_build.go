package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// A "build" is a restorable snapshot of one successful live install: the server
// binary + web bundle that were live after that install, plus the metadata a
// revert needs (notably how many migrations the install applied, so a revert can
// `migrate down N`). Builds are archived under <appDir>/builds/<build_id>/ and
// recorded in the pkg_build collection. See app/docs/live-install.md.

// buildsDirName is the per-build archive root under appDir.
const buildsDirName = "builds"

// buildArchive captures the on-disk paths for one build's archive.
type buildArchive struct {
	root       string // <appDir>/builds/<build_id>
	binary     string // <root>/tinycld (server packages only)
	release    string // <root>/release/
	migrations string // <root>/pb_migrations/ (this build's owned migration files)
}

// buildArchiveFor resolves a build's on-disk archive paths. buildID is always
// server-generated (`build-<UnixMilli>` from the install pipeline or the
// `build-base` constant) and the revert/delete endpoints look builds up by the
// stored build_id — it is never taken from user input, so joining it into a path
// here is not a traversal vector. Keep it that way: do not make build_id
// client-settable.
func buildArchiveFor(appDir, buildID string) buildArchive {
	root := filepath.Join(appDir, buildsDirName, buildID)
	return buildArchive{
		root:       root,
		binary:     filepath.Join(root, binaryName),
		release:    filepath.Join(root, "release"),
		migrations: filepath.Join(root, "pb_migrations"),
	}
}

// appliedMigrationFiles returns the filenames in the _migrations history table,
// most-recently-applied first. The delta between a before- and after-install
// snapshot is exactly the set of migrations that install applied, which drives
// the `migrate down N` count on revert.
func appliedMigrationFiles(app core.App) ([]string, error) {
	var files []string
	err := app.DB().
		Select("file").
		From(core.DefaultMigrationsTable).
		OrderBy("substr(applied||'0000000000000000', 0, 17) DESC").
		Column(&files)
	if err != nil {
		return nil, fmt.Errorf("query _migrations: %w", err)
	}
	return files, nil
}

// newMigrationFiles returns the entries in after that are not in before — the
// migrations applied between the two snapshots. Order follows after (newest
// first).
func newMigrationFiles(before, after []string) []string {
	seen := make(map[string]bool, len(before))
	for _, f := range before {
		seen[f] = true
	}
	out := make([]string, 0)
	for _, f := range after {
		if !seen[f] {
			out = append(out, f)
		}
	}
	return out
}

// archiveBuild copies the now-live binary (server packages) and the staged web
// bundle into <appDir>/builds/<build_id>/ and writes build.json. stageDir is the
// release-staging/<id> directory produced by stageRelease. Returns the archive
// and a cleanup func for the install pipeline's rollback stack.
func archiveBuild(appDir, buildID, stageDir string, hasServer bool, meta map[string]any) (buildArchive, func(), error) {
	arch := buildArchiveFor(appDir, buildID)
	if err := os.MkdirAll(arch.root, 0o755); err != nil {
		return arch, nil, fmt.Errorf("create build dir: %w", err)
	}
	cleanup := func() {
		os.RemoveAll(arch.root)
		log.Printf("pkg_build: rollback — removed archive %s", buildID)
	}

	if hasServer {
		// Copy (not move) the live binary so the running server keeps its file.
		if _, err := runCmd(".", "cp", "-a", filepath.Join(appDir, binaryName), arch.binary); err != nil {
			cleanup()
			return arch, nil, fmt.Errorf("archive binary: %w", err)
		}
	}

	// Copy the staged release bundle (app.html + assets + release-id.txt). The
	// staged dir is still on disk here — the entrypoint only consumes it on the
	// next boot — so a copy is safe and leaves the original for promotion.
	if err := copyDir(stageDir, arch.release); err != nil {
		cleanup()
		return arch, nil, fmt.Errorf("archive release: %w", err)
	}

	// Archive this build's owned migration FILES so a future forward revert (one
	// whose schema is AHEAD of the live state, e.g. reverting back up after a
	// downgrade tore the package's migrations off disk) can restore them and
	// re-apply. The on-disk server/pb_migrations entries are symlinks the generator
	// re-points on every regen — after a downgrade they no longer include this
	// build's newer files — so capturing the resolved file contents here is the
	// only durable record. pkg_migration_files is the package's FULL target set at
	// this build, which is exactly what reproduces this build's schema.
	if err := archiveBuildMigrations(appDir, arch, migrationBasenames(meta)); err != nil {
		cleanup()
		return arch, nil, err
	}

	// build.json is an advisory mirror of the pkg_build record for offline
	// inspection — the DB record is the source of truth — so a marshal error on
	// this flat string/number/[]string map (which can't realistically fail) is
	// ignored; the write error is not.
	metaJSON, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(arch.root, "build.json"), metaJSON, 0o644); err != nil {
		cleanup()
		return arch, nil, fmt.Errorf("write build.json: %w", err)
	}

	return arch, cleanup, nil
}

// migrationBasenames extracts the build's package migration file set from the
// build meta. finalizeVersionChange / the install pipeline build meta in-memory,
// so "pkg_migration_files" is a []string here; tolerate []any too (e.g. a JSON
// round-trip) so a future caller can't silently lose the set. A missing or
// wrong-typed value yields an empty set — archiveBuildMigrations then copies
// nothing, which only weakens a forward revert TO this build, never the current op.
func migrationBasenames(meta map[string]any) []string {
	raw, ok := meta["pkg_migration_files"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// archiveBuildMigrations copies each named migration file from the live
// server/pb_migrations dir (resolving the generator's symlinks to the real file)
// into the build archive's pb_migrations/ subdir. It is defensive: a basename
// missing from server/pb_migrations is logged and skipped rather than failing the
// archive (an archive missing one migration only breaks a forward revert to it,
// not the in-progress install/version-change). With no basenames it does nothing.
func archiveBuildMigrations(appDir string, arch buildArchive, basenames []string) error {
	if len(basenames) == 0 {
		return nil
	}
	if err := os.MkdirAll(arch.migrations, 0o755); err != nil {
		return fmt.Errorf("archive migrations: mkdir: %w", err)
	}
	srcDir := filepath.Join(appDir, "server", "pb_migrations")
	for _, name := range basenames {
		src := filepath.Join(srcDir, name)
		if _, err := os.Stat(src); err != nil {
			// os.Stat follows symlinks; a missing target (broken/absent link) lands
			// here. Don't fail — just record that this build's archive is incomplete.
			log.Printf("pkg_build: archive migrations — %s absent in server/pb_migrations, skipping", name)
			continue
		}
		// cp -aL dereferences the symlink and copies the real file contents, so the
		// archive holds a standalone copy that survives later generator regens.
		if _, err := runCmd(".", "cp", "-aL", src, filepath.Join(arch.migrations, name)); err != nil {
			return fmt.Errorf("archive migration %s: %w", name, err)
		}
	}
	return nil
}

// ---------- pkg_build record helpers ----------

// recordBuild upserts a pkg_build record as the new current build and demotes the
// prior current build to available. The demote + insert run in a single
// transaction so an interrupted run can never leave zero or multiple `current`
// builds. Returns the created record.
func recordBuild(app core.App, fields map[string]any) (*core.Record, error) {
	collection, err := app.FindCollectionByNameOrId("pkg_build")
	if err != nil {
		return nil, err
	}

	record := core.NewRecord(collection)
	for k, v := range fields {
		record.Set(k, v)
	}
	record.Set("status", "current")

	err = app.RunInTransaction(func(txApp core.App) error {
		// Demote every existing current build (normally at most one).
		current, findErr := txApp.FindRecordsByFilter("pkg_build", "status = 'current'", "", 0, 0)
		if findErr != nil {
			return findErr
		}
		for _, prev := range current {
			prev.Set("status", "available")
			if saveErr := txApp.Save(prev); saveErr != nil {
				return saveErr
			}
		}
		return txApp.Save(record)
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

// buildsNewerThan returns the pkg_build records created after the given build,
// newest first. These are the builds a revert to `target` will supersede, and
// whose migrations must be stepped down.
//
// Comparison is `created >= target.created` with the target itself excluded by
// id, rather than a bare `created > target.created`: PocketBase autodate is
// millisecond-precision, so two builds created in the same millisecond would tie
// and a strict `>` could silently drop a genuine newer sibling (under-counting
// the down-migrations). The `>=` + id-exclusion captures same-millisecond
// siblings; the only residual ambiguity is a same-millisecond build that is
// actually *older*, which can't happen here (installs are an operator action
// seconds-to-minutes apart, and build_id embeds a monotonic UnixMilli).
func buildsNewerThan(app core.App, target *core.Record) ([]*core.Record, error) {
	return app.FindRecordsByFilter(
		"pkg_build",
		"created >= {:ts} && id != {:id}",
		"-created",
		0,
		0,
		dbx.Params{"ts": target.GetString("created"), "id": target.Id},
	)
}

// ---------- base build (initial deploy) ----------

const baseBuildID = "build-base"

// baseImageSlug is the synthetic pkg_slug for the base build — it represents the
// whole bundled image, not a single installed package, so registry reconciliation
// skips it.
const baseImageSlug = "(base image)"

// SeedBaseBuild records the deployed image's baseline as a revertible build the
// first time the server boots, so an operator can always return to "fresh image,
// before any live install". It is idempotent: it does nothing once any pkg_build
// record exists (the base build itself, or any install). It only runs in the
// deployed-image layout — it needs the live binary on disk and a promoted web
// release to archive — and silently no-ops in dev.
//
// The base build's migrations_applied is 0 with an empty migration_files: the
// bundled migrations already applied at first boot are the schema floor and must
// never be stepped down. Reverting TO the base build therefore reverses exactly
// the migrations of every live-installed build that came after it, and stops.
func SeedBaseBuild(app core.App) {
	if _, err := app.FindCollectionByNameOrId("pkg_build"); err != nil {
		return // migration not applied yet
	}
	// Any existing build means we've already seeded (or the operator has
	// installed something) — nothing to do.
	if count, err := app.CountRecords("pkg_build"); err != nil || count > 0 {
		return
	}

	appDir := resolveServerDir()
	binPath := filepath.Join(appDir, binaryName)
	if _, err := os.Stat(binPath); err != nil {
		// No binary on disk to archive (dev / `go run`): skip.
		return
	}
	currentRelease := filepath.Join(appDir, "releases", "current")
	if _, err := os.Stat(filepath.Join(currentRelease, "app.html")); err != nil {
		// No promoted web bundle to archive: skip.
		return
	}

	releaseID := readReleaseID(currentRelease)
	fields := map[string]any{
		"build_id":           baseBuildID,
		"pkg_slug":           baseImageSlug,
		"version":            releaseID,
		"action":             "install",
		"binary_archived":    true,
		"release_id":         releaseID,
		"migrations_applied": 0,
		"migration_files":    []string{},
		"notes":              "Initial deploy — baseline state before any live install",
	}

	// Archive the live binary + the promoted release (app.html + release-id.txt;
	// its hashed assets already live in the cross-release _static/ pool, which is
	// append-only, so a base revert promotes correctly without re-archiving them).
	arch := buildArchiveFor(appDir, baseBuildID)
	if err := os.MkdirAll(arch.root, 0o755); err != nil {
		log.Printf("pkg_build: base seed — mkdir failed: %v", err)
		return
	}
	if _, err := runCmd(".", "cp", "-a", binPath, arch.binary); err != nil {
		log.Printf("pkg_build: base seed — archive binary failed: %v", err)
		os.RemoveAll(arch.root)
		return
	}
	if err := copyDir(currentRelease, arch.release); err != nil {
		log.Printf("pkg_build: base seed — archive release failed: %v", err)
		os.RemoveAll(arch.root)
		return
	}
	metaJSON, _ := json.MarshalIndent(fields, "", "  ")
	os.WriteFile(filepath.Join(arch.root, "build.json"), metaJSON, 0o644)

	if _, err := recordBuild(app, fields); err != nil {
		log.Printf("pkg_build: base seed — record failed: %v", err)
		os.RemoveAll(arch.root)
		return
	}
	log.Printf("pkg_build: seeded base build %s (release %s)", baseBuildID, releaseID)
}

// readReleaseID reads a release dir's release-id.txt, returning "" if absent.
func readReleaseID(releaseDir string) string {
	data, err := os.ReadFile(filepath.Join(releaseDir, "release-id.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
