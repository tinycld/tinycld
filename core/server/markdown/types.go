// Package markdown converts between Markdown source and ProseMirror JSON.
//
// It exists so the server can own a document whose storage format is Markdown
// while the live collaborative form is a Yjs doc holding ProseMirror content:
// ToPM seeds a fragment from the stored text, FromPM serializes it back on
// flush.
//
// The node and mark vocabulary deliberately matches the shared editor's schema
// (StarterKit + task lists + tables + images + strike), NOT the wider docx-
// shaped set in text/server/translate. A construct the editor cannot represent
// has no business round-tripping through here — it would be silently dropped on
// the next save.
//
// FromPM's output is byte-stable: marks nest in a fixed order, list indentation
// is fixed, and table columns are padded deterministically. That matters because
// the flush path diffs serialized output against a baseline to decide whether a
// row changed; unstable output would rewrite every card on every flush.
package markdown

// Node types. These are the ProseMirror node names emitted by the tiptap
// schema in tinycld/core/lib/editor/rich/extensions.ts — keep the two in
// lock-step, or content silently degrades on one side of the wire.
const (
	NodeDoc         = "doc"
	NodeParagraph   = "paragraph"
	NodeHeading     = "heading"
	NodeBulletList  = "bulletList"
	NodeOrderedList = "orderedList"
	NodeListItem    = "listItem"
	NodeTaskList    = "taskList"
	NodeTaskItem    = "taskItem"
	NodeBlockquote  = "blockquote"
	NodeCodeBlock   = "codeBlock"
	NodeTable       = "table"
	NodeTableRow    = "tableRow"
	NodeTableCell   = "tableCell"
	// NodeTableHeader is a distinct node in the tiptap Table extension. The
	// TypeScript md-to-pm in text/ flattens header cells to tableCell; that
	// flattening is NOT copied here, because a doc seeded with plain cells
	// loses its header row the first time it round-trips.
	NodeTableHeader = "tableHeader"
	NodeImage       = "image"
	NodeText        = "text"
	// NodeMention is an @mention. It is an ATOM carrying only a user id — the
	// name a reader sees is resolved at render time from the board roster, so
	// the node has no children and no text of its own. That is why it needs an
	// explicit case in the serializer: the default branch renders an unknown
	// inline node's descendants, and an atom has none, so a mention would
	// serialize to nothing and be wiped from the description on the next flush.
	NodeMention        = "tinycldMention"
	NodeHardBreak      = "hardBreak"
	NodeHorizontalRule = "horizontalRule"
)

// Mark types.
const (
	MarkBold      = "bold"
	MarkItalic    = "italic"
	MarkStrike    = "strike"
	MarkUnderline = "underline"
	MarkCode      = "code"
	MarkLink      = "link"
)

// PMNode is a ProseMirror node in its JSON form. The field set and omitempty
// tags match what tiptap's getJSON() produces, so a marshalled PMNode can be
// handed to the editor verbatim.
type PMNode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []PMNode       `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []PMMark       `json:"marks,omitempty"`
}

// PMMark is an inline mark applied to a text node.
type PMMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// attrString reads a string attribute, tolerating a missing map or a non-string
// value — attrs arrive from JSON and from the editor, neither of which this
// package controls.
func attrString(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	v, ok := attrs[key].(string)
	if !ok {
		return ""
	}
	return v
}

// attrBool reads a bool attribute with the same tolerance as attrString.
func attrBool(attrs map[string]any, key string) bool {
	if attrs == nil {
		return false
	}
	v, _ := attrs[key].(bool)
	return v
}

// attrInt reads a numeric attribute. JSON numbers decode to float64, but a
// value constructed in Go may already be an int, so both are accepted.
func attrInt(attrs map[string]any, key string) (int, bool) {
	if attrs == nil {
		return 0, false
	}
	switch v := attrs[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	}
	return 0, false
}
