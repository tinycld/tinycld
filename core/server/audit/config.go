package audit

import (
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Descriptor is the declarative form of a collection's audit config, materialized
// from a package's manifest `audit` block. It replaces the Go-closure
// CollectionConfig for feature packages so audit registration is data, not a
// feature-Go call — required for the multi-org host, which links no feature Go.
type Descriptor struct {
	// Collection is the collection to audit.
	Collection string

	// ResolveOrg describes how to find a record's org. When zero, the default
	// resolver is used (checks "org", then common relation patterns).
	ResolveOrg OrgVia

	// LabelFields are the record fields joined (with LabelJoin) into the audit
	// label. Empty falls back to the default extractor.
	LabelFields []string

	// LabelJoin separates LabelFields (default " "). Trimmed of surrounding
	// space after joining.
	LabelJoin string
}

// OrgVia declares a resolve-via-relation chain: read Field off the record (a
// relation id), load it from Collection, and return its OrgField. When Field is
// empty the default org resolver is used instead.
type OrgVia struct {
	// Field is the record field holding the relation id (e.g. "owner").
	Field string
	// Collection is what Field points at (e.g. "user_org").
	Collection string
	// OrgField is the field on Collection holding the org id (e.g. "org").
	OrgField string
}

// RegisterFromDescriptors wires audit hooks for each descriptor, translating the
// declarative form into the closure-based CollectionConfig.
func RegisterFromDescriptors(app *pocketbase.PocketBase, descriptors []Descriptor) {
	for _, d := range descriptors {
		RegisterCollection(app, d.Collection, descriptorConfig(d))
	}
}

func descriptorConfig(d Descriptor) *CollectionConfig {
	cfg := &CollectionConfig{}

	if d.ResolveOrg.Field != "" {
		via := d.ResolveOrg
		cfg.ResolveOrg = func(a core.App, record *core.Record) string {
			id := record.GetString(via.Field)
			if id == "" {
				return ""
			}
			return ResolveViaRelation(a, via.Collection, id, via.OrgField)
		}
	}

	if len(d.LabelFields) > 0 {
		fields := d.LabelFields
		join := d.LabelJoin
		if join == "" {
			join = " "
		}
		cfg.ExtractLabel = func(record *core.Record) string {
			parts := make([]string, 0, len(fields))
			for _, f := range fields {
				parts = append(parts, record.GetString(f))
			}
			return strings.TrimSpace(strings.Join(parts, join))
		}
	}

	return cfg
}
