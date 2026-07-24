package audit

import (
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Descriptor is the declarative form of a collection's audit config, materialized
// from a package's manifest `audit` block. It replaces the Go-closure
// CollectionConfig for feature packages so audit registration is data, not a
// feature-Go call — required for the host, which links no feature Go.
type Descriptor struct {
	// Collection is the collection to audit.
	Collection string

	// LabelFields are the record fields joined (with LabelJoin) into the audit
	// label. Empty falls back to the default extractor.
	LabelFields []string

	// LabelJoin separates LabelFields (default " "). Trimmed of surrounding
	// space after joining.
	LabelJoin string
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
