package markdown

import (
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Underline (`++text++`) is tiptap's markdown spelling for the underline mark
// (@tiptap/markdown, matching markdown-it's ins plugin). It is not GFM, so
// goldmark needs this inline extension — without it a flushed `++text++`
// re-seeds as literal plus signs, and before the serializer learned to EMIT
// it (see pm_to_md.go) the mark was silently dropped on every flush: an
// underline survived only until the first board close.
//
// Modeled on goldmark's own strikethrough extension, minus the HTML renderer —
// nothing in this package renders HTML; md_to_pm walks the AST itself.

// underlineNode is the AST node for a ++…++ run.
type underlineNode struct {
	gast.BaseInline
}

var kindUnderline = gast.NewNodeKind("TinycldUnderline")

func (n *underlineNode) Kind() gast.NodeKind { return kindUnderline }

func (n *underlineNode) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

type underlineDelimiterProcessor struct{}

func (p *underlineDelimiterProcessor) IsDelimiter(b byte) bool { return b == '+' }

func (p *underlineDelimiterProcessor) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char
}

func (p *underlineDelimiterProcessor) OnMatch(_ int) gast.Node { return &underlineNode{} }

var defaultUnderlineDelimiterProcessor = &underlineDelimiterProcessor{}

type underlineParser struct{}

func (s *underlineParser) Trigger() []byte { return []byte{'+'} }

func (s *underlineParser) Parse(_ gast.Node, block text.Reader, pc parser.Context) gast.Node {
	before := block.PrecendingCharacter()
	line, segment := block.PeekLine()
	// Exactly two plus signs, unlike strikethrough's one-or-two tildes: a
	// single + is ordinary prose ("1 + 2") and must stay literal.
	node := parser.ScanDelimiter(line, before, 2, defaultUnderlineDelimiterProcessor)
	if node == nil || node.OriginalLength != 2 || before == '+' {
		return nil
	}
	node.Segment = segment.WithStop(segment.Start + node.OriginalLength)
	block.Advance(node.OriginalLength)
	pc.PushDelimiter(node)
	return node
}

func (s *underlineParser) CloseBlock(_ gast.Node, _ parser.Context) {}
