package carddav

import (
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/pocketbase/pocketbase/core"
)

// pbTimeLayout is PocketBase's stored datetime format (used for updated/REV).
const pbTimeLayout = "2006-01-02 15:04:05.000Z"

// RecordToVCard builds a vCard from a record using the Source's field map. FN is
// composed from the name parts; REV is formatted from the configured timestamp
// field; UID is emitted when VCardMap.UIDField is mapped. Simple properties are
// emitted only when the source field is non-empty.
//
// Exported so a package can serve a vCard outside the CardDAV protocol — the
// contacts export endpoint reuses it, which is what keeps one field mapping
// (the package's own carddav.Source) behind both CardDAV and file export.
func RecordToVCard(record *core.Record, m VCardMap) vcard.Card {
	card := make(vcard.Card)
	version := m.Version
	if version == "" {
		version = "4.0"
	}
	card.SetValue(vcard.FieldVersion, version)

	given := record.GetString(m.Name.Given)
	family := record.GetString(m.Name.Family)
	card.SetValue(vcard.FieldFormattedName, strings.TrimSpace(given+" "+family))
	card.Set(vcard.FieldName, &vcard.Field{Value: family + ";" + given + ";;;"})

	// UID identifies the card across systems. CardDAV does not rely on this —
	// it addresses objects by URL path — but a vCard file has no path, so file
	// export/import matches on this property alone. Emitted only when mapped
	// and non-empty, so a Source predating VCardMap.UIDField is unaffected.
	if m.UIDField != "" {
		if uid := record.GetString(m.UIDField); uid != "" {
			card.SetValue(vcard.FieldUID, uid)
		}
	}

	for prop, field := range m.Simple {
		if v := record.GetString(field); v != "" {
			card.Set(prop, &vcard.Field{Value: v})
		}
	}

	if m.RevField != "" {
		// GetDateTime handles both a stored datetime string (the DB read path)
		// and a types.DateTime value; a plain GetString returns "" for the
		// latter, so prefer GetDateTime here.
		if rev := record.GetDateTime(m.RevField); !rev.IsZero() {
			card.SetValue(vcard.FieldRevision, rev.Time().UTC().Format("20060102T150405Z"))
		}
	}

	return card
}

// ApplyVCardToRecord writes a vCard's values onto a record using the field map.
// N is preferred for the name; FN is split as a fallback. Simple properties set
// their mapped record field when present.
//
// Exported alongside RecordToVCard so file import shares the CardDAV parse
// path. Note it only SETS mapped fields — it never clears one the card omits,
// so applying a partial card to an existing record leaves the rest intact.
func ApplyVCardToRecord(card vcard.Card, record *core.Record, m VCardMap) {
	if n := card.Name(); n != nil {
		record.Set(m.Name.Given, n.GivenName)
		record.Set(m.Name.Family, n.FamilyName)
	} else if fn := card.FormattedNames(); len(fn) > 0 {
		parts := strings.SplitN(fn[0].Value, " ", 2)
		record.Set(m.Name.Given, parts[0])
		if len(parts) > 1 {
			record.Set(m.Name.Family, parts[1])
		}
	}

	for prop, field := range m.Simple {
		if v := card.Value(prop); v != "" {
			record.Set(field, v)
		}
	}
}
