package markdown

import (
	"strings"
)

// markOrder fixes the nesting order of marks from outside in, so a run
// carrying {bold, italic, link} always serializes as `[***…***](href)` and
// never as one of its permutations. Ported from the TypeScript emitter in
// text/tinycld/text/lib/markdown/pm-to-md.ts, extended with strike.
//
// code is listed but handled separately: a code span renders verbatim, so it
// wraps the raw text before any other mark and suppresses escaping.
var markOrder = []string{MarkLink, MarkBold, MarkItalic, MarkStrike, MarkCode}

// escapeText escapes only what would actually change the parse at this
// position. Escaping every candidate character unconditionally is tempting but
// wrong: the parser strips the backslash, so the next serialization escapes the
// backslash too and the text grows a slash per save (`\~2s` → `\\\~2s` → …).
// A document that is merely re-saved must come back byte-identical.
//
// The rules mirror what a CommonMark writer needs:
//   - `\` always, since it is the escape character itself.
//   - A run-forming character (`*`, `_`, `~`) only when it could open or close
//     a span — i.e. when it is doubled, or adjacent to a non-space on the side
//     that would make it a delimiter. `_` additionally only counts at a word
//     boundary, because intra-word underscores are literal in CommonMark.
//   - A backtick always, since one opens a code span anywhere.
//   - `[` and `]` always, since they form links and are cheap to escape.
func escapeText(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i, r := range runes {
		switch r {
		case '\\', '`', '[', ']':
			b.WriteRune('\\')
		case '*', '~':
			if isDelimiterCandidate(runes, i) {
				b.WriteRune('\\')
			}
		case '_':
			// Intra-word underscores (snake_case) are literal in CommonMark,
			// so only escape at a word boundary.
			if isDelimiterCandidate(runes, i) && !withinWord(runes, i) {
				b.WriteRune('\\')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isDelimiterCandidate reports whether the rune at i could act as an emphasis
// delimiter: either it is part of a doubled run, or it sits against non-space
// text on one side.
func isDelimiterCandidate(runes []rune, i int) bool {
	r := runes[i]
	if (i > 0 && runes[i-1] == r) || (i+1 < len(runes) && runes[i+1] == r) {
		return true
	}
	leftOpen := i == 0 || isSpace(runes[i-1])
	rightOpen := i+1 == len(runes) || isSpace(runes[i+1])
	// A delimiter needs a non-space neighbour to bind to.
	return !leftOpen || !rightOpen
}

func withinWord(runes []rune, i int) bool {
	return i > 0 && i+1 < len(runes) && isWordRune(runes[i-1]) && isWordRune(runes[i+1])
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n'
}

func isWordRune(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127
}

// wrapCode fences an inline code span. CommonMark requires the fence to be
// longer than the longest backtick run inside it; the double-backtick form with
// padding spaces is the standard idiom for text that contains a backtick.
func wrapCode(s string) string {
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

// applyMarks wraps a text run in its marks, innermost first.
func applyMarks(text string, marks []PMMark) string {
	present := make(map[string]PMMark, len(marks))
	for _, m := range marks {
		if _, seen := present[m.Type]; !seen {
			present[m.Type] = m
		}
	}

	_, hasCode := present[MarkCode]
	body := text
	if hasCode {
		// Code spans are verbatim — escaping would emit literal backslashes.
		body = wrapCode(body)
	} else {
		body = escapeText(body)
	}

	for i := len(markOrder) - 1; i >= 0; i-- {
		typ := markOrder[i]
		mark, ok := present[typ]
		if !ok || typ == MarkCode {
			continue
		}
		switch typ {
		case MarkBold:
			body = "**" + body + "**"
		case MarkItalic:
			body = "*" + body + "*"
		case MarkStrike:
			body = "~~" + body + "~~"
		case MarkLink:
			body = "[" + body + "](" + escapeLinkTarget(attrString(mark.Attrs, "href")) + ")"
		}
	}
	return body
}

// escapeLinkTarget percent-encodes the one character that would terminate an
// inline link target early. Spaces are left alone: they are rare in real hrefs
// and encoding them would rewrite URLs users pasted.
func escapeLinkTarget(href string) string {
	return strings.ReplaceAll(href, ")", "%29")
}

func renderInline(nodes []PMNode) string {
	var b strings.Builder
	for i := range nodes {
		node := &nodes[i]
		switch node.Type {
		case NodeImage:
			alt := attrString(node.Attrs, "alt")
			src := escapeLinkTarget(attrString(node.Attrs, "src"))
			b.WriteString("![" + alt + "](" + src + ")")
		case NodeHardBreak:
			// Two trailing spaces is the portable hard break; a backslash
			// break is GFM-only and reads as a stray character elsewhere.
			b.WriteString("  \n")
		case NodeText:
			if node.Text == "\n" {
				b.WriteString("  \n")
				continue
			}
			b.WriteString(applyMarks(node.Text, node.Marks))
		default:
			// An unknown inline node still has readable descendants.
			b.WriteString(renderInline(node.Content))
		}
	}
	return b.String()
}

func indentLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// renderListItem lays the first block inline after the marker and indents any
// following blocks to the marker's content column.
func renderListItem(item *PMNode, marker, indent string) string {
	blocks := item.Content
	if len(blocks) == 0 {
		return strings.TrimRight(marker, " ")
	}
	parts := make([]string, 0, len(blocks))
	for i := range blocks {
		parts = append(parts, renderBlock(&blocks[i]))
	}
	out := marker + parts[0]
	if len(parts) > 1 {
		tail := make([]string, 0, len(parts)-1)
		for _, p := range parts[1:] {
			tail = append(tail, indentLines(p, indent))
		}
		out += "\n" + strings.Join(tail, "\n")
	}
	return out
}

func renderBulletList(node *PMNode) string {
	var items []string
	for i := range node.Content {
		child := &node.Content[i]
		if child.Type != NodeListItem {
			continue
		}
		items = append(items, renderListItem(child, "- ", "  "))
	}
	return strings.Join(items, "\n")
}

func renderOrderedList(node *PMNode) string {
	start := 1
	if v, ok := attrInt(node.Attrs, "start"); ok && v > 0 {
		start = v
	}
	var items []string
	n := start
	for i := range node.Content {
		child := &node.Content[i]
		if child.Type != NodeListItem {
			continue
		}
		marker := itoa(n) + ". "
		// Indent continuation lines to the width of the marker so nested
		// blocks stay inside the item.
		items = append(items, renderListItem(child, marker, strings.Repeat(" ", len(marker))))
		n++
	}
	return strings.Join(items, "\n")
}

func renderTaskList(node *PMNode) string {
	var items []string
	for i := range node.Content {
		child := &node.Content[i]
		if child.Type != NodeTaskItem {
			continue
		}
		marker := "- [ ] "
		if attrBool(child.Attrs, "checked") {
			marker = "- [x] "
		}
		items = append(items, renderListItem(child, marker, "  "))
	}
	return strings.Join(items, "\n")
}

func renderBlockquote(node *PMNode) string {
	inner := make([]string, 0, len(node.Content))
	for i := range node.Content {
		inner = append(inner, renderBlock(&node.Content[i]))
	}
	joined := strings.Join(inner, "\n\n")
	lines := strings.Split(joined, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + line
		}
	}
	return strings.Join(lines, "\n")
}

// renderCodeBlock emits the fence with its language. The TypeScript emitter
// always wrote a bare ``` and dropped the language; that loses syntax
// highlighting on every round trip, so the attribute is honored here.
func renderCodeBlock(node *PMNode) string {
	var text strings.Builder
	for i := range node.Content {
		child := &node.Content[i]
		if child.Type == NodeText {
			text.WriteString(child.Text)
		}
	}
	lang := attrString(node.Attrs, "language")
	body := text.String()
	// A fence must be longer than any backtick run in the body, or the block
	// terminates early.
	fence := "```"
	for strings.Contains(body, fence) {
		fence += "`"
	}
	return fence + lang + "\n" + body + "\n" + fence
}

// flattenInlineForCell collapses a cell to a single line: pipe tables allow
// only inline content, and a literal pipe would end the cell.
func flattenInlineForCell(nodes []PMNode) string {
	s := renderInline(nodes)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.TrimSpace(s)
}

// cellTexts flattens one table row. A cell's inline content usually sits inside
// a paragraph, so descend through block children rather than reading the cell's
// content as inline directly.
func cellTexts(row *PMNode) []string {
	out := make([]string, 0, len(row.Content))
	for i := range row.Content {
		cell := &row.Content[i]
		var parts []string
		for j := range cell.Content {
			block := &cell.Content[j]
			if block.Type == NodeText || block.Type == NodeImage {
				parts = append(parts, flattenInlineForCell(cell.Content))
				break
			}
			parts = append(parts, flattenInlineForCell(block.Content))
		}
		out = append(out, strings.TrimSpace(strings.Join(parts, " ")))
	}
	return out
}

// renderTable emits a GFM pipe table with padded columns. Padding is what makes
// the output stable and readable; it also matches what the tiptap serializer
// produces, so a doc edited on either side round-trips byte-identically.
func renderTable(node *PMNode) string {
	var rows []*PMNode
	for i := range node.Content {
		if node.Content[i].Type == NodeTableRow {
			rows = append(rows, &node.Content[i])
		}
	}
	if len(rows) == 0 {
		return ""
	}

	grid := make([][]string, 0, len(rows))
	colCount := 0
	for _, row := range rows {
		cells := cellTexts(row)
		if len(cells) > colCount {
			colCount = len(cells)
		}
		grid = append(grid, cells)
	}
	if colCount == 0 {
		return ""
	}
	// Pad ragged rows so the pipe table stays syntactically valid.
	for i := range grid {
		for len(grid[i]) < colCount {
			grid[i] = append(grid[i], "")
		}
	}

	widths := make([]int, colCount)
	for c := range widths {
		widths[c] = 3 // the delimiter row needs at least "---"
	}
	for _, row := range grid {
		for c, cell := range row {
			if n := len([]rune(cell)); n > widths[c] {
				widths[c] = n
			}
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		b.WriteString("|")
		for c, cell := range cells {
			b.WriteString(" ")
			b.WriteString(cell)
			b.WriteString(strings.Repeat(" ", widths[c]-len([]rune(cell))))
			b.WriteString(" |")
		}
		b.WriteString("\n")
	}
	writeRow(grid[0])
	b.WriteString("|")
	for c := range widths {
		b.WriteString(" ")
		b.WriteString(strings.Repeat("-", widths[c]))
		b.WriteString(" |")
	}
	b.WriteString("\n")
	for _, row := range grid[1:] {
		writeRow(row)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderHeading(node *PMNode) string {
	level := 1
	if v, ok := attrInt(node.Attrs, "level"); ok {
		level = v
	}
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	return strings.Repeat("#", level) + " " + renderInline(node.Content)
}

func renderBlock(node *PMNode) string {
	switch node.Type {
	case NodeParagraph:
		return renderInline(node.Content)
	case NodeHeading:
		return renderHeading(node)
	case NodeBulletList:
		return renderBulletList(node)
	case NodeOrderedList:
		return renderOrderedList(node)
	case NodeTaskList:
		return renderTaskList(node)
	case NodeListItem:
		// Only meaningful inside a list; degrade to a single bullet rather
		// than dropping the content.
		return renderListItem(node, "- ", "  ")
	case NodeBlockquote:
		return renderBlockquote(node)
	case NodeCodeBlock:
		return renderCodeBlock(node)
	case NodeTable:
		return renderTable(node)
	case NodeHorizontalRule:
		return "---"
	default:
		return renderInline(node.Content)
	}
}

// FromPM serializes a ProseMirror document to Markdown. Output always ends in a
// single newline; an empty document produces exactly "\n".
func FromPM(doc *PMNode) string {
	if doc == nil || len(doc.Content) == 0 {
		return "\n"
	}
	blocks := make([]string, 0, len(doc.Content))
	for i := range doc.Content {
		if s := renderBlock(&doc.Content[i]); s != "" {
			blocks = append(blocks, s)
		}
	}
	if len(blocks) == 0 {
		return "\n"
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

// itoa avoids pulling strconv in for the one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
