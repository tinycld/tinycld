package reports

import (
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/core"
)

// CatalogSource is a report source as the builder offers it: its fields and
// links by key and label. It never names the source's collection or columns.
type CatalogSource struct {
	Key    string         `json:"key"`
	Label  string         `json:"label"`
	Fields []CatalogField `json:"fields"`
	Links  []CatalogLink  `json:"links"`
}

// CatalogField is a field the builder can add as a column, filter or group on.
type CatalogField struct {
	Key        string      `json:"key"`
	Label      string      `json:"label"`
	Type       FieldType   `json:"type"`
	Aggregates []Aggregate `json:"aggregates"`
	Options    []Option    `json:"options,omitempty"`
	// Currency is the key of the field that holds each row's currency, for a
	// money field that has one.
	Currency string `json:"currency,omitempty"`
}

// CatalogLink is a link the builder can follow.
type CatalogLink struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	Target      string      `json:"target"`
	Cardinality Cardinality `json:"cardinality"`
}

// Catalog lists the sources a report may use, sorted by key, as copies. A
// field registered with no aggregates offers CountOnly, as the engine reads it.
func Catalog() []CatalogSource {
	all := Sources()
	out := make([]CatalogSource, 0, len(all))
	for _, src := range all {
		cs := CatalogSource{
			Key:    src.Key,
			Label:  src.Label,
			Fields: make([]CatalogField, 0, len(src.Fields)),
			Links:  make([]CatalogLink, 0, len(src.Links)),
		}
		for _, f := range src.Fields {
			aggregates := f.Aggregates
			if aggregates == nil {
				aggregates = CountOnly
			}
			cs.Fields = append(cs.Fields, CatalogField{
				Key: f.Key, Label: f.Label, Type: f.Type,
				Aggregates: slices.Clone(aggregates), Options: slices.Clone(f.Options), Currency: f.Currency,
			})
		}
		for _, l := range src.Links {
			cs.Links = append(cs.Links, CatalogLink{Key: l.Key, Label: l.Label, Target: l.Target, Cardinality: l.Cardinality})
		}
		out = append(out, cs)
	}
	return out
}

// handleSources answers GET /api/reports/sources with the catalog. It reads
// the registry in memory and no collection, so it shows no record data.
func handleSources(re *core.RequestEvent) error {
	return re.JSON(http.StatusOK, map[string]any{"sources": Catalog()})
}
