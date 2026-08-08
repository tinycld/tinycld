package yjsdoc

import (
	"sort"
	"strings"

	ycrdt "github.com/skyterra/y-crdt"
)

// FragmentNames returns the document's top-level XmlFragment keys carrying the
// given prefix, sorted.
//
// This is what lets one document hold many editors: the flush path enumerates
// `card:` fragments to discover which cards a board room actually has content
// for, rather than assuming the set it seeded. A card created while the room is
// live has a fragment but no baseline, and it must still be persisted.
//
// Only roots that are genuinely XmlFragment-shaped are returned; a root of some
// other type carrying the prefix is skipped rather than reported as an editor.
func FragmentNames(doc *Doc, prefix string) []string {
	if doc == nil || doc.Share == nil {
		return nil
	}
	names := make([]string, 0, len(doc.Share))
	for key, value := range doc.Share {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if !isXMLFragmentish(value) {
			continue
		}
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

// isXMLFragmentish reports whether a Share entry can be read as an
// XmlFragment. y-crdt stores roots as AbstractType until something forces a
// concrete type, so an entry minted by an inbound update may not yet be a
// *YXmlFragment even though GetXmlFragment will happily view it as one.
func isXMLFragmentish(value any) bool {
	switch value.(type) {
	case *ycrdt.YXmlFragment, *ycrdt.AbstractType:
		return true
	case *ycrdt.YXmlElement:
		return true
	}
	return false
}
