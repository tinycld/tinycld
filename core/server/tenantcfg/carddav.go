// Package tenantcfg is the router↔tenant runtime-config ABI: the wire shapes
// of the .runtime/*.json files a hosting router materializes into an org's
// directory and a tenant process reads at boot (DAV source lists, the quota
// ceiling, resolved package slugs, app URL / proxy trust). It lives in core —
// not in the router — because the two sides of the contract are built from
// DIFFERENT checkouts: a tenant binary is built from its org's own package
// set, pinned to whatever core version that set resolves, while the router
// evolves independently. One definition, imported by both, is what keeps the
// contract from forking (DESIGN-org-package-agency.md §6 names this ABI as a
// versioned interface; changes here must stay backward-compatible).
//
// The DAV types mirror tinycld.org/core/{carddav,webdav,caldav}'s Sources
// rather than marshalling those types directly: they carry no JSON tags, so
// encoding them would silently depend on Go field names staying stable.
package tenantcfg

import "tinycld.org/core/carddav"

type Source struct {
	Slug            string   `json:"slug"`
	Collection      string   `json:"collection"`
	ListFilter      string   `json:"listFilter"`
	Sort            string   `json:"sort"`
	OwnerField      string   `json:"ownerField"`
	UIDField        string   `json:"uidField"`
	SoftDeleteField string   `json:"softDeleteField"`
	VCard           VCardMap `json:"vcard"`
}

type VCardMap struct {
	Version  string            `json:"version"`
	Name     NameMap           `json:"name"`
	Simple   map[string]string `json:"simple"`
	RevField string            `json:"revField"`
}

type NameMap struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

// Encode converts core's Sources into the wire form.
func Encode(sources []carddav.Source) []Source {
	out := make([]Source, 0, len(sources))
	for _, s := range sources {
		out = append(out, Source{
			Slug:            s.Slug,
			Collection:      s.Collection,
			ListFilter:      s.ListFilter,
			Sort:            s.Sort,
			OwnerField:      s.OwnerField,
			UIDField:        s.UIDField,
			SoftDeleteField: s.SoftDeleteField,
			VCard: VCardMap{
				Version:  s.VCard.Version,
				Name:     NameMap{Given: s.VCard.Name.Given, Family: s.VCard.Name.Family},
				Simple:   s.VCard.Simple,
				RevField: s.VCard.RevField,
			},
		})
	}
	return out
}

// Decode converts the wire form back into core's Sources.
func Decode(sources []Source) []carddav.Source {
	out := make([]carddav.Source, 0, len(sources))
	for _, s := range sources {
		out = append(out, carddav.Source{
			Slug:            s.Slug,
			Collection:      s.Collection,
			ListFilter:      s.ListFilter,
			Sort:            s.Sort,
			OwnerField:      s.OwnerField,
			UIDField:        s.UIDField,
			SoftDeleteField: s.SoftDeleteField,
			VCard: carddav.VCardMap{
				Version:  s.VCard.Version,
				Name:     carddav.NameMap{Given: s.VCard.Name.Given, Family: s.VCard.Name.Family},
				Simple:   s.VCard.Simple,
				RevField: s.VCard.RevField,
				// Derived from the Source's own UIDField rather than carried as
				// a second wire field: both always name the same column, and a
				// separate JSON key would only create a way for them to
				// disagree. Keeping it out of the wire shape also means older
				// routers' .runtime JSON still decodes to a UID-emitting map.
				UIDField: s.UIDField,
			},
		})
	}
	return out
}
