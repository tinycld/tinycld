package reports

import (
	"fmt"
	"slices"
)

const (
	maxDisplayTracks = 12
	maxDisplayLines  = 8
)

var displayWidths = []string{"mobile", "tablet", "desktop"}

func (b *binder) display(p *plan) {
	d := p.spec.Display
	if d == nil {
		return
	}
	b.columnFits(p)
	if len(d.Layouts) > len(displayWidths) {
		b.bad("display.layouts", fmt.Sprintf("a report has at most %d display layouts, one per width", len(displayWidths)))
		return
	}
	seen := map[string]bool{}
	for i, l := range d.Layouts {
		at := fmt.Sprintf("display.layouts.%d", i)
		switch {
		case !slices.Contains(displayWidths, l.Width):
			b.bad(at+".width", "width is mobile, tablet or desktop")
		case seen[l.Width]:
			b.bad(at+".width", "one layout per width")
		}
		seen[l.Width] = true
		if l.MinWidth != "" {
			if msg := fitLengthProblem(l.MinWidth, pxRule); msg != "" {
				b.bad(at+".minWidth", msg)
			}
		}
		if l.Tracks < 1 || l.Tracks > maxDisplayTracks {
			b.bad(at+".tracks", fmt.Sprintf("tracks is 1 to %d", maxDisplayTracks))
			continue
		}
		// A layout with no cells would show nothing at its width, and a stored
		// one would read back as "cells": null.
		if len(l.Cells) == 0 {
			b.bad(at+".cells", "a layout places at least one column")
			continue
		}
		if len(l.Cells) > maxColumns {
			b.bad(at+".cells", fmt.Sprintf("a layout places at most %d columns", maxColumns))
			continue
		}
		cols := map[string]bool{}
		for j, cell := range l.Cells {
			cat := fmt.Sprintf("%s.cells.%d", at, j)
			if b.column(p, cell.Col, cat+".col") != nil && cols[cell.Col] {
				b.bad(cat+".col", "a column is placed once per layout")
			}
			cols[cell.Col] = true
			if cell.Justify != "" && cell.Justify != "start" && cell.Justify != "center" && cell.Justify != "end" {
				b.bad(cat+".justify", "justify is start, center or end")
			}
			if msg := cellRangeProblem(l.Tracks, cell); msg != "" {
				b.bad(cat, msg)
			}
		}
	}
}

// cellRangeProblem is why a cell's line, lines, span or start is out of
// range, or "". Where the cells land, and whether they overlap or cut down by
// straight lines, is the client's (core/reports/lib/table-lines.ts
// placeCells); the engine checks the definition only.
func cellRangeProblem(tracks int, c DisplayCell) string {
	span, lines := max(c.Span, 1), max(c.Lines, 1)
	switch {
	case c.Line < 1 || c.Line > maxDisplayLines:
		return fmt.Sprintf("line is 1 to %d", maxDisplayLines)
	case c.Lines < 0 || lines > maxDisplayLines:
		return fmt.Sprintf("lines is 1 to %d", maxDisplayLines)
	case c.Line+lines-1 > maxDisplayLines:
		return fmt.Sprintf("a cell ends by line %d", maxDisplayLines)
	case c.Span < 0 || span > tracks:
		return fmt.Sprintf("span is 1 to %d", tracks)
	case c.Start < 0 || c.Start > tracks || (c.Start > 0 && c.Start+span-1 > tracks):
		return "the cell does not fit its line"
	}
	return ""
}
