package yjsdoc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	ycrdt "github.com/skyterra/y-crdt"

	"tinycld.org/core/markdown"
)

// yTiptapHashSuffixRe matches the `--<8 base64 chars>` suffix that
// y-tiptap (the JS-side Yjs <-> ProseMirror binding) appends to a
// mark name when the mark type is "overlapping" — i.e. the mark
// type's excludes set permits a same-type mark on the same range
// (suggestedInsert / suggestedDelete use excludes:” so two authors
// can layer independent proposals over the same run).
//
// The suffix is 6 bytes from a SHA-256 digest re-encoded as base64
// (see y-tiptap's `hashOfJSON` + `_convolute`) — exactly 8 base64
// characters from the alphabet [A-Za-z0-9+/=]. The same regex
// y-tiptap uses on decode (`yattr2markname`) is mirrored here so
// the Go bridge surfaces the canonical mark type name to downstream
// consumers (docx emitter, comment extractor, etc.) instead of the
// opaque hashed name. Without this strip, a `suggestedDelete--AbCd1234`
// key would surface as a PM mark whose type is `suggestedDelete--AbCd1234`
// — a name no emitter recognizes, so the mark is SILENTLY dropped
// from the docx output and the strikethrough vanishes on reload.
var yTiptapHashSuffixRe = regexp.MustCompile(`^(.*)--[A-Za-z0-9+/=]{8}$`)

// stripYTiptapHashSuffix returns the canonical mark name from a
// y-tiptap-encoded attribute key. Pass-through for keys that don't
// match the hashed-suffix shape, so non-overlapping marks (which
// y-tiptap writes under their bare name) round-trip unchanged.
func stripYTiptapHashSuffix(attrKey string) string {
	if m := yTiptapHashSuffixRe.FindStringSubmatch(attrKey); m != nil {
		return m[1]
	}
	return attrKey
}

// IMPLEMENTATION NOTE on the y-crdt API surface used here.
//
// Chosen path: Path A (Y.XmlFragment + Y.XmlElement + Y.XmlText). The
// y-crdt Go library at github.com/skyterra/y-crdt v0.0.0-20260224023949
// exposes the XML primitives that y-prosemirror uses on the JS side,
// so we can produce the canonical wire format the Tiptap Collaboration
// extension expects:
//
//   * doc.GetXmlFragment(name) returns IAbstractType which we
//     type-assert to *YXmlFragment (initialized properly by the
//     library — Map / EH / DEH are non-nil).
//   * NewYXmlElement(nodeName) and NewYXmlText() build prelim children.
//     Iteration on read uses (*YXmlFragment).ToArray() / GetAttributes()
//     and YXmlText.ToDelta(...) to recover marks as Quill-style ops.
//
// Why this matters past M3.6: the M5 Tiptap Collaboration extension on
// the JS side will mount y-prosemirror against the same XmlFragment
// (named "prosemirror" by convention here), so this bridge produces a
// wire format the client can consume directly without a custom
// serialization shim.
//
// Two y-crdt library quirks the bridge has to work around — both
// motivate the helpers and the integrate-then-populate ordering below.
// Both should be reported upstream if we ever do that.
//
//   1. NewYXmlElement / NewYXmlText do NOT initialize the embedded
//      AbstractType.Map / EH / DEH fields the way NewYXmlFragment does.
//      Setting any attribute on a freshly-constructed element panics
//      inside Item.Integrate (writes through GetMap()[...] = item with
//      a nil map). Observers fired on insert also panic because EH is
//      nil. The newXmlElement / newXmlText helpers fix this by
//      patching the fields after construction.
//   2. YXmlElement.Integrate shadows YXmlFragment.Integrate and only
//      flushes PrelimAttrs — it never inserts the element's
//      PrelimContent. Pushing a pre-built tree therefore drops every
//      child below the top-level fragment. The bridge sidesteps this
//      by building the tree top-down: push an empty element into its
//      already-integrated parent, then push children into it after it
//      has a Doc reference.
//
// The encode / decode switches are schema-agnostic: a node's type name becomes
// the XmlElement name and its attrs become element attributes, so any node the
// editor schema defines round-trips without a change here.

// SeedFragmentFromPMJSON populates one named XmlFragment with the content
// described by a ProseMirror JSON tree. The caller owns the Doc; on success the
// fragment holds a faithful representation of pmJSON.
//
// The fragment name is a parameter rather than text/'s fixed "prosemirror"
// because one document can carry many independent editors: cards keeps a
// fragment per card (`card:<id>`), so a whole board shares one document and
// therefore one websocket. It must match the `field` option passed to tiptap's
// Collaboration extension on the client.
func SeedFragmentFromPMJSON(doc *Doc, fragment string, pmJSON []byte) error {
	var root markdown.PMNode
	if err := json.Unmarshal(pmJSON, &root); err != nil {
		return fmt.Errorf("yjsdoc: unmarshal pmJSON: %w", err)
	}
	if root.Type != markdown.NodeDoc {
		return fmt.Errorf("yjsdoc: pmJSON root must be type=doc, got %q", root.Type)
	}

	frag, ok := doc.GetXmlFragment(fragment).(*ycrdt.YXmlFragment)
	if !ok {
		return fmt.Errorf("yjsdoc: GetXmlFragment did not return *YXmlFragment")
	}

	for _, child := range root.Content {
		if err := seedChildIntoFragment(frag, child); err != nil {
			return err
		}
	}
	return nil
}

// PMJSONFromFragment reads the Y.Doc's XmlFragment and reconstructs the
// markdown.PMNode tree as JSON. The output is normalized: nil maps and empty
// slices are omitted via markdown.PMNode's `,omitempty` tags.
func PMJSONFromFragment(doc *Doc, fragment string) ([]byte, error) {
	root := markdown.PMNode{Type: markdown.NodeDoc}

	frag, ok := doc.GetXmlFragment(fragment).(*ycrdt.YXmlFragment)
	if !ok {
		return nil, fmt.Errorf("yjsdoc: GetXmlFragment did not return *YXmlFragment")
	}

	for _, item := range frag.ToArray() {
		children, err := decodeXMLChild(item)
		if err != nil {
			return nil, err
		}
		root.Content = append(root.Content, children...)
	}
	return json.Marshal(root)
}

// seedChildIntoFragment inserts one markdown.PMNode (block- or inline-level)
// into a YXmlFragment that's already integrated into a Doc. Children
// are pushed onto the now-integrated parent recursively.
func seedChildIntoFragment(parent *ycrdt.YXmlFragment, node markdown.PMNode) error {
	switch node.Type {
	case markdown.NodeText:
		text, err := buildXMLText(node)
		if err != nil {
			return err
		}
		parent.Push([]any{text})
		return nil
	default:
		el := newXmlElement(node.Type)
		for k, v := range node.Attrs {
			el.SetAttribute(k, normalizeAttrValue(v))
		}
		parent.Push([]any{el})
		// el now has a Doc; recursively push children into it. We
		// use the embedded YXmlFragment because YXmlElement inherits
		// its insert/push behavior from there.
		for _, child := range node.Content {
			if err := seedChildIntoFragment(&el.YXmlFragment, child); err != nil {
				return err
			}
		}
		return nil
	}
}

// normalizeAttrValue coerces JSON-decoded numbers into the concrete
// types ycrdt.TypeMapSet's type switch accepts. y-crdt's TypeMapSet
// only encodes Number (= int), Object, bool, ArrayAny, and string —
// any other type falls through to a silent error from a deferred
// goroutine path, which means attributes set with a float64 value
// (the default JSON-number type) vanish without surfacing an error.
// JSON numbers that are integral integers (e.g. heading level, list
// start) become ints; non-integral floats stay float64 with a
// best-effort cast (they'll still fail upstream, but that's a callsite
// programming error we'd want to see, not a number we should round).
func normalizeAttrValue(v any) any {
	switch n := v.(type) {
	case float64:
		if n == float64(int(n)) {
			return int(n)
		}
	case float32:
		if n == float32(int(n)) {
			return int(n)
		}
	}
	return v
}

// buildXMLText builds a YXmlText preloaded with the node's text and
// any inline marks. The text gets a usable EH/DEH via newXmlText —
// y-crdt's NewYXmlText does set EH/DEH on the embedded YText, but
// it leaves Map nil; we patch it for symmetry with newXmlElement.
// Insert calls on a freshly-constructed YXmlText buffer into Pending
// and flush when the parent integrates it.
func buildXMLText(node markdown.PMNode) (*ycrdt.YXmlText, error) {
	text := newXmlText()
	if node.Text == "" {
		return text, nil
	}
	attrs := marksToAttributes(node.Marks)
	text.Insert(0, node.Text, attrs)
	return text, nil
}

// newXmlElement returns a YXmlElement with EH/DEH/Map pre-initialized.
// Without these, SetAttribute and observer dispatch panic on integration
// — see the file-level "library quirks" note above.
func newXmlElement(nodeName string) *ycrdt.YXmlElement {
	el := ycrdt.NewYXmlElement(nodeName)
	if el.EH == nil {
		el.EH = ycrdt.NewEventHandler()
	}
	if el.DEH == nil {
		el.DEH = ycrdt.NewEventHandler()
	}
	if el.Map == nil {
		el.Map = make(map[string]*ycrdt.Item)
	}
	return el
}

// newXmlText returns a YXmlText with Map initialized — EH/DEH are
// already set by NewYXmlText, but Map is left nil and would panic
// in the (currently unused) attribute-set path. Patch for safety.
func newXmlText() *ycrdt.YXmlText {
	t := ycrdt.NewYXmlText()
	if t.Map == nil {
		t.Map = make(map[string]*ycrdt.Item)
	}
	return t
}

// marksToAttributes turns a markdown.PMMark slice into the Object that YText
// expects for Insert / Format. Returns nil (not an empty Object) when
// there are no marks, matching y-crdt's convention for unmarked text.
func marksToAttributes(marks []markdown.PMMark) ycrdt.Object {
	if len(marks) == 0 {
		return nil
	}
	attrs := ycrdt.NewObject()
	for _, mark := range marks {
		if len(mark.Attrs) == 0 {
			attrs[mark.Type] = true
			continue
		}
		// Mark with attrs (e.g. link href) — store the attrs map
		// as the value so decode can reverse the same shape. Number
		// values get the same normalization treatment as element
		// attrs (see normalizeAttrValue) so any future numeric mark
		// attribute survives the Y.Doc encoder's strict type switch.
		normalized := make(map[string]any, len(mark.Attrs))
		for k, v := range mark.Attrs {
			normalized[k] = normalizeAttrValue(v)
		}
		attrs[mark.Type] = normalized
	}
	return attrs
}

// decodeXMLChild converts one item out of a YXmlFragment / YXmlElement
// child list back into zero-or-more PMNodes. Items are either
// *YXmlElement (one block node), *YXmlText (one or more text runs —
// y-tiptap may pack multiple differently-formatted runs into a single
// YText, e.g. a paragraph "prefix [marked CUTME] suffix" lives as one
// YText with three delta ops), or — defensively — anything else, which
// we drop.
//
// We return a slice (rather than a single *markdown.PMNode) so that the YText
// case can fan out into N PMNodes — one per delta op — without the
// callers losing every run past the first. Returning a single node was
// the source of a silent-data-loss bug: anything after the first
// formatted segment of a multi-run YText was discarded, so a paragraph
// like "prefix CUTME suffix" with a suggestedDelete mark on CUTME
// flushed back to docx as just "prefix " — both CUTME and suffix were
// gone.
func decodeXMLChild(item any) ([]markdown.PMNode, error) {
	switch v := item.(type) {
	case *ycrdt.YXmlElement:
		node, err := decodeXMLElement(v)
		if err != nil {
			return nil, err
		}
		if node == nil {
			return nil, nil
		}
		return []markdown.PMNode{*node}, nil
	case *ycrdt.YXmlText:
		return decodeXMLText(v), nil
	case ycrdt.IXmlType:
		// Some XML-shaped types we don't yet support (e.g. YXmlHook).
		// Skip rather than fail so a bad attribute can't poison the
		// whole document; future tasks tighten this.
		return nil, nil
	default:
		return nil, nil
	}
}

func decodeXMLElement(el *ycrdt.YXmlElement) (*markdown.PMNode, error) {
	node := markdown.PMNode{Type: el.NodeName}

	if attrs := el.GetAttributes(); len(attrs) > 0 {
		node.Attrs = make(map[string]any, len(attrs))
		for k, v := range attrs {
			node.Attrs[k] = v
		}
	}

	for _, item := range el.ToArray() {
		children, err := decodeXMLChild(item)
		if err != nil {
			return nil, err
		}
		node.Content = append(node.Content, children...)
	}

	return &node, nil
}

// decodeXMLText splits a YXmlText's delta into one markdown.PMNode-text per run
// of identically-marked characters. A YXmlText with no formatting
// produces one markdown.PMNode with empty Marks; a YXmlText carrying multiple
// format runs (the common case when y-tiptap stores a whole paragraph's
// inline content in a single YText) produces one markdown.PMNode per delta op.
//
// Returning a SLICE rather than a single markdown.PMNode is load-bearing: a
// paragraph whose middle word carries a suggestedDelete mark lives in
// Y.Doc as a single YText with three delta ops (prefix / CUTME with
// mark / suffix). Returning only the first op silently dropped CUTME
// and the suffix on docx flush, so after a reload the trailing text was
// missing — the symptom the user reported as "everything after the
// deleted word is gone."
func decodeXMLText(text *ycrdt.YXmlText) []markdown.PMNode {
	delta := text.ToDelta(nil, nil, nil)
	if len(delta) == 0 {
		return nil
	}
	out := make([]markdown.PMNode, 0, len(delta))
	for _, op := range delta {
		if !op.IsInsertDefined {
			continue
		}
		node := deltaOpToTextNode(op)
		if node == nil {
			continue
		}
		out = append(out, *node)
	}
	return out
}

func deltaOpToTextNode(op ycrdt.EventOperator) *markdown.PMNode {
	if !op.IsInsertDefined {
		return nil
	}
	str, ok := op.Insert.(string)
	if !ok {
		return nil
	}
	node := &markdown.PMNode{Type: markdown.NodeText, Text: str}
	if marks := attributesToMarks(op.Attributes); len(marks) > 0 {
		node.Marks = marks
	}
	return node
}

// attributesToMarks reverses marksToAttributes: an Object key whose
// value is `true` becomes a mark with no attrs; an Object key whose
// value is a map becomes a mark whose attrs are that map. Marks are
// emitted in deterministic order so the round-trip test is stable.
func attributesToMarks(attrs ycrdt.Object) []markdown.PMMark {
	if len(attrs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	marks := make([]markdown.PMMark, 0, len(keys))
	for _, k := range keys {
		mark := markdown.PMMark{Type: stripYTiptapHashSuffix(k)}
		switch v := attrs[k].(type) {
		case bool:
			if !v {
				continue
			}
		case map[string]any:
			// ycrdt.Object is `type Object = map[string]any`, an
			// alias, so this branch covers both shapes.
			mark.Attrs = v
		default:
			// Unknown mark value shape — skip rather than panic.
			continue
		}
		marks = append(marks, mark)
	}
	return marks
}
