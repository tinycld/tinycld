package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"tinycld.org/core/audit"
	"tinycld.org/core/carddav"
	"tinycld.org/core/fts"
)

// Package capability config, materialized from each package's manifest.
//
// A package declares protocol/index behavior as DATA in its manifest (an `fts`
// block, a `carddav` block, …). The generator serializes the full manifest into
// bundled-packages.json (already consumed by SyncBundledPackages). Here we read
// the same file at Register time — BEFORE serve, so capability record hooks bind
// in time — and turn the declared blocks into the capability packages' Config
// types. This is the config-driven seam: core owns the Go capability, the package
// contributes only data, and no feature Go is linked into the host. It works
// identically in single-tenant (this app's bundled-packages.json) and is the
// shape multi-org mirrors per tenant.

// manifestCapabilities is the subset of a package manifest core reads to wire
// host-side capabilities. Unknown manifest fields are ignored.
type manifestCapabilities struct {
	Slug string `json:"slug"`
	FTS  *struct {
		Collection string `json:"collection"`
		Table      string `json:"table"`
		Columns    []struct {
			FTS   string `json:"fts"`
			Field string `json:"field"`
			Strip bool   `json:"strip"`
		} `json:"columns"`
		Owner struct {
			Field     string `json:"field"`
			Via       string `json:"via"`
			UserField string `json:"userField"`
		} `json:"owner"`
		Output []struct {
			Column string `json:"column"`
			Type   string `json:"type"`
		} `json:"output"`
		SoftDeleteField string `json:"softDeleteField"`
	} `json:"fts"`
	CardDAV *struct {
		Collection      string `json:"collection"`
		ListFilter      string `json:"listFilter"`
		Sort            string `json:"sort"`
		OwnerField      string `json:"ownerField"`
		UIDField        string `json:"uidField"`
		SoftDeleteField string `json:"softDeleteField"`
		VCard           struct {
			Version string `json:"version"`
			Name    struct {
				Given  string `json:"given"`
				Family string `json:"family"`
			} `json:"name"`
			Simple   map[string]string `json:"simple"`
			RevField string            `json:"revField"`
		} `json:"vcard"`
	} `json:"carddav"`
	Audit []struct {
		Collection string `json:"collection"`
		ResolveOrg struct {
			Field      string `json:"field"`
			Collection string `json:"collection"`
			OrgField   string `json:"orgField"`
		} `json:"resolveOrg"`
		LabelFields []string `json:"labelFields"`
		LabelJoin   string   `json:"labelJoin"`
	} `json:"audit"`
}

// loadFTSConfigs reads bundled-packages.json and returns one fts.Config per
// package that declares an `fts` block. A malformed block is fatal (log.Fatal),
// mirroring assertSafeImportField's fail-loud stance: a silently-unwired search
// index is a worse outcome than a loud boot failure.
func loadFTSConfigs() []fts.Config {
	caps := loadCapabilityManifests()
	var configs []fts.Config
	for _, c := range caps {
		if c.FTS == nil {
			continue
		}
		if err := validateFTS(c); err != nil {
			log.Fatalf("coreserver: invalid fts config for package %q: %v", c.Slug, err)
		}
		cols := make([]fts.Column, len(c.FTS.Columns))
		for i, col := range c.FTS.Columns {
			cols[i] = fts.Column{FTS: col.FTS, Field: col.Field, Strip: col.Strip}
		}
		output := make([]fts.OutputColumn, len(c.FTS.Output))
		for i, o := range c.FTS.Output {
			output[i] = fts.OutputColumn{Name: o.Column, Type: o.Type}
		}
		configs = append(configs, fts.Config{
			Slug:       c.Slug,
			Collection: c.FTS.Collection,
			Table:      c.FTS.Table,
			Columns:    cols,
			Owner: fts.OwnerScope{
				Field:     c.FTS.Owner.Field,
				Via:       c.FTS.Owner.Via,
				UserField: c.FTS.Owner.UserField,
			},
			Output:          output,
			SoftDeleteField: c.FTS.SoftDeleteField,
		})
	}
	return configs
}

// validateFTS ensures the required fields of an fts block are present so a typo
// fails loud at boot instead of serving an empty or panicking search.
func validateFTS(c manifestCapabilities) error {
	f := c.FTS
	if f.Collection == "" || f.Table == "" {
		return fmt.Errorf("fts requires collection and table")
	}
	if len(f.Columns) == 0 {
		return fmt.Errorf("fts requires at least one column")
	}
	for i, col := range f.Columns {
		if col.FTS == "" || col.Field == "" {
			return fmt.Errorf("fts column %d requires fts and field", i)
		}
	}
	if f.Owner.Field == "" || f.Owner.Via == "" || f.Owner.UserField == "" {
		return fmt.Errorf("fts requires owner.field, owner.via and owner.userField")
	}
	return nil
}

// loadCardDAVConfigs reads bundled-packages.json and returns one carddav.Source
// per package that declares a `carddav` block. A malformed block is fatal
// (fail-loud), matching loadFTSConfigs.
func loadCardDAVConfigs() []carddav.Source {
	caps := loadCapabilityManifests()
	var sources []carddav.Source
	for _, c := range caps {
		if c.CardDAV == nil {
			continue
		}
		if err := validateCardDAV(c); err != nil {
			log.Fatalf("coreserver: invalid carddav config for package %q: %v", c.Slug, err)
		}
		cd := c.CardDAV
		sources = append(sources, carddav.Source{
			Slug:            c.Slug,
			Collection:      cd.Collection,
			ListFilter:      cd.ListFilter,
			Sort:            cd.Sort,
			OwnerField:      cd.OwnerField,
			UIDField:        cd.UIDField,
			SoftDeleteField: cd.SoftDeleteField,
			VCard: carddav.VCardMap{
				Version: cd.VCard.Version,
				Name: carddav.NameMap{
					Given:  cd.VCard.Name.Given,
					Family: cd.VCard.Name.Family,
				},
				Simple:   cd.VCard.Simple,
				RevField: cd.VCard.RevField,
			},
		})
	}
	return sources
}

// validateCardDAV ensures the required fields of a carddav block are present.
func validateCardDAV(c manifestCapabilities) error {
	cd := c.CardDAV
	if cd.Collection == "" || cd.ListFilter == "" {
		return fmt.Errorf("carddav requires collection and listFilter")
	}
	if cd.OwnerField == "" || cd.UIDField == "" {
		return fmt.Errorf("carddav requires ownerField and uidField")
	}
	if cd.VCard.Name.Given == "" || cd.VCard.Name.Family == "" {
		return fmt.Errorf("carddav requires vcard.name.given and vcard.name.family")
	}
	return nil
}

// loadAuditDescriptors reads bundled-packages.json and returns one
// audit.Descriptor per `audit` entry declared by any package. A package may audit
// several collections, so `audit` is a list.
func loadAuditDescriptors() []audit.Descriptor {
	caps := loadCapabilityManifests()
	var descriptors []audit.Descriptor
	for _, c := range caps {
		for _, a := range c.Audit {
			if a.Collection == "" {
				log.Fatalf("coreserver: invalid audit config for package %q: collection required", c.Slug)
			}
			descriptors = append(descriptors, audit.Descriptor{
				Collection: a.Collection,
				ResolveOrg: audit.OrgVia{
					Field:      a.ResolveOrg.Field,
					Collection: a.ResolveOrg.Collection,
					OrgField:   a.ResolveOrg.OrgField,
				},
				LabelFields: a.LabelFields,
				LabelJoin:   a.LabelJoin,
			})
		}
	}
	return descriptors
}

// loadCapabilityManifests reads bundled-packages.json and parses the capability
// subset out of each package's manifestJson. Returns nil (and logs) when the file
// is absent — dev/test contexts without a generated bundle still boot.
func loadCapabilityManifests() []manifestCapabilities {
	jsonPath := findBundledPackagesJSON()
	if jsonPath == "" {
		log.Println("coreserver: bundled-packages.json not found, no package capabilities wired")
		return nil
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		log.Printf("coreserver: failed to read %s: %v", jsonPath, err)
		return nil
	}
	var packages []struct {
		Slug         string `json:"slug"`
		ManifestJSON string `json:"manifestJson"`
	}
	if err := json.Unmarshal(data, &packages); err != nil {
		log.Printf("coreserver: failed to parse bundled-packages.json: %v", err)
		return nil
	}

	var out []manifestCapabilities
	for _, pkg := range packages {
		if pkg.ManifestJSON == "" {
			continue
		}
		var mc manifestCapabilities
		if err := json.Unmarshal([]byte(pkg.ManifestJSON), &mc); err != nil {
			log.Printf("coreserver: failed to parse manifest for %q: %v", pkg.Slug, err)
			continue
		}
		// Prefer the row slug; manifest slug should match but the row is canonical.
		if mc.Slug == "" {
			mc.Slug = pkg.Slug
		}
		out = append(out, mc)
	}
	return out
}
