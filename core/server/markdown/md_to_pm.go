package markdown

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// parser is shared: goldmark.Markdown is documented as safe for concurrent use
// and building one per call would re-register every extension on a hot path
// (seeding runs once per card at room open).
var parser = goldmark.New(goldmark.WithExtensions(extension.GFM))

// ToPM parses Markdown into a ProseMirror document.
//
// Constructs the editor schema cannot represent (footnotes, definition lists,
// raw HTML) degrade to their text content rather than erroring: the input is
// user prose that may predate any given schema, and dropping a paragraph is
// worse than losing its formatting.
func ToPM(src string) *PMNode {
	source := []byte(src)
	root := parser.Parser().Parse(text.NewReader(source))

	doc := &PMNode{Type: NodeDoc}
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		doc.Content = append(doc.Content, blockToPM(child, source)...)
	}
	return doc
}

// blockToPM converts one block node. It returns a slice because a few
// constructs (a list mixing task and plain items) expand to more than one node.
func blockToPM(n ast.Node, src []byte) []PMNode {
	switch node := n.(type) {
	case *ast.Paragraph:
		return []PMNode{{Type: NodeParagraph, Content: inlineToPM(node, src)}}

	case *ast.TextBlock:
		// A tight list item's content arrives as a TextBlock rather than a
		// Paragraph; the editor schema has no TextBlock, so normalize it.
		return []PMNode{{Type: NodeParagraph, Content: inlineToPM(node, src)}}

	case *ast.Heading:
		return []PMNode{{
			Type:    NodeHeading,
			Attrs:   map[string]any{"level": node.Level},
			Content: inlineToPM(node, src),
		}}

	case *ast.Blockquote:
		return []PMNode{{Type: NodeBlockquote, Content: childBlocks(node, src)}}

	case *ast.FencedCodeBlock:
		attrs := map[string]any{"language": nil}
		if lang := string(node.Language(src)); lang != "" {
			attrs["language"] = lang
		}
		return []PMNode{{Type: NodeCodeBlock, Attrs: attrs, Content: codeText(node, src)}}

	case *ast.CodeBlock:
		return []PMNode{{
			Type:    NodeCodeBlock,
			Attrs:   map[string]any{"language": nil},
			Content: codeText(node, src),
		}}

	case *ast.ThematicBreak:
		return []PMNode{{Type: NodeHorizontalRule}}

	case *ast.List:
		return []PMNode{listToPM(node, src)}

	case *extast.Table:
		return []PMNode{tableToPM(node, src)}

	case *ast.HTMLBlock:
		// Raw HTML has no schema representation. Emit its source as a
		// paragraph so the words survive even though the markup does not.
		if txt := rawLines(node.Lines(), src); txt != "" {
			return []PMNode{{Type: NodeParagraph, Content: []PMNode{{Type: NodeText, Text: txt}}}}
		}
		return nil

	default:
		// Unknown container: keep its blocks, drop the wrapper.
		if n.Type() == ast.TypeBlock && n.HasChildren() {
			return childBlocks(n, src)
		}
		return nil
	}
}

func childBlocks(n ast.Node, src []byte) []PMNode {
	var out []PMNode
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		out = append(out, blockToPM(child, src)...)
	}
	return out
}

// listToPM maps a goldmark list. GFM represents a task item as a normal list
// item whose first inline child is a TaskCheckBox, so the item kind is decided
// per list: if any item is a checkbox item, the whole list becomes a taskList
// (the editor schema has no mixed list, and splitting one authored list into
// two would reorder the document).
func listToPM(node *ast.List, src []byte) PMNode {
	isTask := false
	for item := node.FirstChild(); item != nil; item = item.NextSibling() {
		if _, ok := taskCheckbox(item); ok {
			isTask = true
			break
		}
	}

	listType := NodeBulletList
	itemType := NodeListItem
	var attrs map[string]any
	switch {
	case isTask:
		listType, itemType = NodeTaskList, NodeTaskItem
	case node.IsOrdered():
		listType = NodeOrderedList
		if node.Start != 1 {
			attrs = map[string]any{"start": node.Start}
		}
	}

	out := PMNode{Type: listType, Attrs: attrs}
	for item := node.FirstChild(); item != nil; item = item.NextSibling() {
		pmItem := PMNode{Type: itemType}
		if isTask {
			checked := false
			if box, ok := taskCheckbox(item); ok {
				checked = box.IsChecked
			}
			pmItem.Attrs = map[string]any{"checked": checked}
		}
		pmItem.Content = childBlocks(item, src)
		if len(pmItem.Content) == 0 {
			// The schema requires at least one block inside an item.
			pmItem.Content = []PMNode{{Type: NodeParagraph}}
		}
		out.Content = append(out.Content, pmItem)
	}
	return out
}

// taskCheckbox finds the leading TaskCheckBox of a list item, if present.
func taskCheckbox(item ast.Node) (*extast.TaskCheckBox, bool) {
	first := item.FirstChild()
	if first == nil {
		return nil, false
	}
	box, ok := first.FirstChild().(*extast.TaskCheckBox)
	return box, ok
}

func tableToPM(node *extast.Table, src []byte) PMNode {
	out := PMNode{Type: NodeTable}
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		_, isHeader := row.(*extast.TableHeader)
		pmRow := PMNode{Type: NodeTableRow}
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cellType := NodeTableCell
			if isHeader {
				cellType = NodeTableHeader
			}
			inline := inlineToPM(cell, src)
			// A cell's content is block-level in the tiptap schema even
			// though markdown only allows inline there.
			pmRow.Content = append(pmRow.Content, PMNode{
				Type:    cellType,
				Content: []PMNode{{Type: NodeParagraph, Content: inline}},
			})
		}
		out.Content = append(out.Content, pmRow)
	}
	return out
}

func codeText(n ast.Node, src []byte) []PMNode {
	txt := rawLines(n.Lines(), src)
	// A code block's trailing newline belongs to the fence, not the content.
	if len(txt) > 0 && txt[len(txt)-1] == '\n' {
		txt = txt[:len(txt)-1]
	}
	if txt == "" {
		return nil
	}
	return []PMNode{{Type: NodeText, Text: txt}}
}

func rawLines(lines *text.Segments, src []byte) string {
	var b []byte
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b = append(b, seg.Value(src)...)
	}
	return string(b)
}

// inlineToPM walks inline children, carrying the active mark set down. Marks
// accumulate rather than nest as separate nodes, matching ProseMirror's flat
// text-run model.
func inlineToPM(n ast.Node, src []byte) []PMNode {
	var out []PMNode
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		out = append(out, inlineNodeToPM(child, src, nil)...)
	}
	return mergeAdjacentText(out)
}

func inlineNodeToPM(n ast.Node, src []byte, marks []PMMark) []PMNode {
	switch node := n.(type) {
	case *ast.Text:
		// goldmark resolves backslash escapes in its renderers, not in the AST:
		// the segment for `\~` still contains the backslash. Left as-is, the
		// serializer would escape that backslash too and the text would grow a
		// slash on every save.
		txt := unescapeBackslashes(string(node.Segment.Value(src)))
		var out []PMNode
		if txt != "" {
			out = append(out, PMNode{Type: NodeText, Text: txt, Marks: cloneMarks(marks)})
		}
		// A soft line break is a space in the rendered output; a hard break
		// is an explicit node.
		if node.HardLineBreak() {
			out = append(out, PMNode{Type: NodeHardBreak})
		} else if node.SoftLineBreak() {
			out = append(out, PMNode{Type: NodeText, Text: "\n", Marks: cloneMarks(marks)})
		}
		return out

	case *ast.String:
		return []PMNode{{Type: NodeText, Text: string(node.Value), Marks: cloneMarks(marks)}}

	case *ast.CodeSpan:
		return []PMNode{{
			Type:  NodeText,
			Text:  inlineRawText(node, src),
			Marks: appendMark(marks, PMMark{Type: MarkCode}),
		}}

	case *ast.Emphasis:
		mark := MarkItalic
		if node.Level >= 2 {
			mark = MarkBold
		}
		return childInlines(node, src, appendMark(marks, PMMark{Type: mark}))

	case *extast.Strikethrough:
		return childInlines(node, src, appendMark(marks, PMMark{Type: MarkStrike}))

	case *ast.Link:
		attrs := map[string]any{"href": string(node.Destination)}
		if len(node.Title) > 0 {
			attrs["title"] = string(node.Title)
		}
		return childInlines(node, src, appendMark(marks, PMMark{Type: MarkLink, Attrs: attrs}))

	case *ast.AutoLink:
		url := string(node.URL(src))
		return []PMNode{{
			Type:  NodeText,
			Text:  url,
			Marks: appendMark(marks, PMMark{Type: MarkLink, Attrs: map[string]any{"href": url}}),
		}}

	case *ast.Image:
		attrs := map[string]any{"src": string(node.Destination)}
		if alt := inlineRawText(node, src); alt != "" {
			attrs["alt"] = alt
		}
		return []PMNode{{Type: NodeImage, Attrs: attrs}}

	case *extast.TaskCheckBox:
		// Consumed by listToPM as the item's checked attribute; emitting it
		// here would leak a literal "[x]" into the item text.
		return nil

	case *ast.RawHTML:
		// Inline markup with no schema equivalent: drop the tags, keep
		// whatever text the surrounding nodes carry.
		return nil

	default:
		if n.HasChildren() {
			return childInlines(n, src, marks)
		}
		return nil
	}
}

func childInlines(n ast.Node, src []byte, marks []PMMark) []PMNode {
	var out []PMNode
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		out = append(out, inlineNodeToPM(child, src, marks)...)
	}
	return out
}

// inlineRawText collects the plain text under a node, used where markdown
// allows no formatting (code spans, image alt text).
func inlineRawText(n ast.Node, src []byte) string {
	var b []byte
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Text:
			b = append(b, c.Segment.Value(src)...)
		case *ast.String:
			b = append(b, c.Value...)
		default:
			b = append(b, inlineRawText(child, src)...)
		}
	}
	return string(b)
}

// unescapeBackslashes resolves CommonMark backslash escapes. Only ASCII
// punctuation is escapable — a backslash before anything else (including one at
// end of input) is a literal backslash and must survive.
func unescapeBackslashes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			i++
			b.WriteByte(s[i])
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isASCIIPunct(c byte) bool {
	switch {
	case c >= '!' && c <= '/':
		return true
	case c >= ':' && c <= '@':
		return true
	case c >= '[' && c <= '`':
		return true
	case c >= '{' && c <= '~':
		return true
	}
	return false
}

func cloneMarks(marks []PMMark) []PMMark {
	if len(marks) == 0 {
		return nil
	}
	out := make([]PMMark, len(marks))
	copy(out, marks)
	return out
}

func appendMark(marks []PMMark, m PMMark) []PMMark {
	out := make([]PMMark, 0, len(marks)+1)
	out = append(out, marks...)
	return append(out, m)
}

// mergeAdjacentText joins neighbouring text runs that carry identical marks.
// goldmark splits on soft breaks and entity boundaries, and leaving the pieces
// separate would make the PM JSON differ from what the editor produces for the
// same content.
func mergeAdjacentText(nodes []PMNode) []PMNode {
	if len(nodes) < 2 {
		return nodes
	}
	out := make([]PMNode, 0, len(nodes))
	for _, node := range nodes {
		if len(out) > 0 {
			prev := &out[len(out)-1]
			if prev.Type == NodeText && node.Type == NodeText && sameMarks(prev.Marks, node.Marks) {
				prev.Text += node.Text
				continue
			}
		}
		out = append(out, node)
	}
	return out
}

func sameMarks(a, b []PMMark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type {
			return false
		}
		if attrString(a[i].Attrs, "href") != attrString(b[i].Attrs, "href") {
			return false
		}
	}
	return true
}
