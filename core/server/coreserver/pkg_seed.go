package coreserver

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase/core"
)

type bundledPackage struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Version      string `json:"version"`
	Icon         string `json:"icon"`
	Description  string `json:"description"`
	HasServer    bool   `json:"hasServer"`
	NavOrder     int    `json:"navOrder"`
	ManifestJSON string `json:"manifestJson"`
	// Source is the canonical git spec (e.g. `github:tinycld/mail`) the in-app
	// upgrader fetches newer versions from. Seeded into npm_package so a bundled
	// feature flows through the same version-discovery + version-change pipeline
	// as an installed package. Empty for rows the generator emits without a
	// source spec.
	Source string `json:"source"`
}

func SyncBundledPackages(app core.App) {
	jsonPath := findBundledPackagesJSON()
	if jsonPath == "" {
		log.Println("pkg_seed: bundled-packages.json not found, skipping sync")
		return
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		log.Printf("pkg_seed: failed to read %s: %v", jsonPath, err)
		return
	}

	var packages []bundledPackage
	if err := json.Unmarshal(data, &packages); err != nil {
		log.Printf("pkg_seed: failed to parse bundled-packages.json: %v", err)
		return
	}

	collection, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		log.Printf("pkg_seed: pkg_registry collection not found: %v", err)
		return
	}

	// Build a set of bundled slugs
	bundledSlugs := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		bundledSlugs[pkg.Slug] = true
	}

	// Upsert each bundled package
	for _, pkg := range packages {
		existing, err := app.FindFirstRecordByFilter(
			"pkg_registry",
			"slug = {:slug}",
			map[string]any{"slug": pkg.Slug},
		)

		if err != nil {
			// Create new record
			record := core.NewRecord(collection)
			record.Set("name", pkg.Name)
			record.Set("slug", pkg.Slug)
			record.Set("version", pkg.Version)
			record.Set("icon", pkg.Icon)
			record.Set("description", pkg.Description)
			record.Set("has_server", pkg.HasServer)
			record.Set("nav_order", pkg.NavOrder)
			record.Set("status", "bundled")
			// npm_package is the upgrade source spec. With it set, handleVersions
			// stops short-circuiting on empty spec and discovers git tags, which
			// lights up the version picker + the version-change pipeline for this
			// bundled feature. Left unset only for rows the generator emits with no source spec.
			if pkg.Source != "" {
				record.Set("npm_package", pkg.Source)
			}
			// manifest_json feeds the version-management compatibility solver
			// (peerVersions). Keep it in sync with the installed-package path
			// (upsertPkgRegistry), which stores the full manifest here.
			if pkg.ManifestJSON != "" {
				record.Set("manifest_json", pkg.ManifestJSON)
			}
			if err := app.Save(record); err != nil {
				log.Printf("pkg_seed: failed to create %s: %v", pkg.Slug, err)
			}
			continue
		}

		// Update existing bundled record. version tracks bundled-packages.json,
		// which the generator re-emits from the on-disk member after an in-app
		// upgrade (regenerateWiring) — so the seed file stays authoritative and a
		// redeploy correctly reconciles the version either way.
		existing.Set("name", pkg.Name)
		existing.Set("version", pkg.Version)
		existing.Set("icon", pkg.Icon)
		existing.Set("description", pkg.Description)
		existing.Set("has_server", pkg.HasServer)
		existing.Set("nav_order", pkg.NavOrder)
		// Backfill the upgrade source onto already-seeded rows (which predate the
		// source field and so carry an empty npm_package). Only when empty: once a
		// row has a spec — whether from a prior backfill or an in-app upgrade that
		// pinned `#<tag>` — leave it alone so we don't clobber the resolved source.
		if pkg.Source != "" && existing.GetString("npm_package") == "" {
			existing.Set("npm_package", pkg.Source)
		}
		if pkg.ManifestJSON != "" {
			existing.Set("manifest_json", pkg.ManifestJSON)
		}
		if existing.GetString("status") == "disabled" {
			// Re-enable if it was disabled but is still bundled
			existing.Set("status", "bundled")
		}
		if err := app.Save(existing); err != nil {
			log.Printf("pkg_seed: failed to update %s: %v", pkg.Slug, err)
		}
	}

	// Mark removed bundled packages as disabled
	allRecords, err := app.FindRecordsByFilter(
		"pkg_registry",
		"status = 'bundled'",
		"",
		0,
		0,
	)
	if err != nil {
		return
	}

	for _, record := range allRecords {
		slug := record.GetString("slug")
		if !bundledSlugs[slug] {
			record.Set("status", "disabled")
			if err := app.Save(record); err != nil {
				log.Printf("pkg_seed: failed to disable %s: %v", slug, err)
			}
		}
	}

	log.Printf("pkg_seed: synced %d bundled packages", len(packages))
}

func findBundledPackagesJSON() string {
	// Try relative to working directory (typical for dev: server/)
	candidates := []string{
		"bundled-packages.json",
		"../server/bundled-packages.json",
		filepath.Join(filepath.Dir(os.Args[0]), "bundled-packages.json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
