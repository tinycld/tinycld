package reports

import (
	"errors"
	"fmt"
	"slices"

	validation "github.com/pocketbase/ozzo-validation/v4"
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
		}
		if _, err := placeCells(l.Tracks, l.Cells); err != nil {
			b.merge(prefixProblem(at, err))
		}
	}
}

// placed is where a display cell sits: its first track, tracks, line and
// lines, all from 1.
type placed struct{ start, span, line, lines int }

// placeCells lays cells out in order: a cell with a start sits there; one
// without takes the first tracks of its line that are free on every line it
// spans; then the cells must cut down by straight lines (no pinwheel, R28).
// lib/table-lines.ts places and cuts cells by the same rules.
func placeCells(tracks int, cells []DisplayCell) ([]placed, error) {
	taken := map[[2]int]bool{}
	out := make([]placed, 0, len(cells))
	for j, c := range cells {
		at := fmt.Sprintf("cells.%d", j)
		span, lines := max(c.Span, 1), max(c.Lines, 1)
		switch {
		case c.Line < 1 || c.Line > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("line is 1 to %d", maxDisplayLines))
		case c.Lines < 0 || lines > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("lines is 1 to %d", maxDisplayLines))
		case c.Line+lines-1 > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("a cell ends by line %d", maxDisplayLines))
		case c.Span < 0 || span > tracks:
			return nil, invalid(at, fmt.Sprintf("span is 1 to %d", tracks))
		case c.Start < 0 || c.Start > tracks || (c.Start > 0 && c.Start+span-1 > tracks):
			return nil, invalid(at, "the cell does not fit its line")
		}
		free := func(start int) bool {
			for line := c.Line; line < c.Line+lines; line++ {
				for t := start; t < start+span; t++ {
					if taken[[2]int{line, t}] {
						return false
					}
				}
			}
			return true
		}
		start := c.Start
		if start == 0 {
			for s := 1; s+span-1 <= tracks && start == 0; s++ {
				if free(s) {
					start = s
				}
			}
			if start == 0 {
				return nil, invalid(at, fmt.Sprintf("no room on line %d", c.Line))
			}
		} else if !free(start) {
			return nil, invalid(at, "the cell overlaps another")
		}
		for line := c.Line; line < c.Line+lines; line++ {
			for t := start; t < start+span; t++ {
				taken[[2]int{line, t}] = true
			}
		}
		out = append(out, placed{start, span, c.Line, lines})
	}
	// The client draws an arrangement as blocks cut by straight lines (R28): a
	// pinwheel no straight cut can split cannot be drawn.
	if j := uncuttable(out, tracks); j >= 0 {
		return nil, invalid(fmt.Sprintf("cells.%d", j), pinwheelMessage)
	}
	return out, nil
}

const pinwheelMessage = "These cells overlap in a way that can't be laid out; move one cell."

// piece is a placed cell as a box: tracks [x0, x1) and lines [y0, y1), from 0.
type piece struct{ index, x0, x1, y0, y1 int }

// uncuttable is the index of the first cell of a part no straight cut between
// lines or tracks can split into two non-empty sides, or -1 when the cells cut
// down to one each (guillotine cuts; lib/table-lines.ts cuts the same way).
// Any straight cut that splits a part is as good as another: a part that cuts
// down keeps cutting down after any one cut, so the first cut found is taken.
func uncuttable(ps []placed, tracks int) int {
	pieces := make([]piece, len(ps))
	lines := 0
	for i, p := range ps {
		pieces[i] = piece{i, p.start - 1, p.start - 1 + p.span, p.line - 1, p.line - 1 + p.lines}
		lines = max(lines, p.line-1+p.lines)
	}
	return cutDown(pieces, 0, tracks, 0, lines)
}

func cutDown(pieces []piece, x0, x1, y0, y1 int) int {
	if len(pieces) <= 1 {
		return -1
	}
	split := func(at int, across bool) ([]piece, []piece, bool) {
		var lo, hi []piece
		for _, p := range pieces {
			a0, a1 := p.x0, p.x1
			if across {
				a0, a1 = p.y0, p.y1
			}
			switch {
			case a1 <= at:
				lo = append(lo, p)
			case a0 >= at:
				hi = append(hi, p)
			default:
				return nil, nil, false
			}
		}
		return lo, hi, len(lo) > 0 && len(hi) > 0
	}
	for y := y0 + 1; y < y1; y++ {
		if lo, hi, ok := split(y, true); ok {
			if j := cutDown(lo, x0, x1, y0, y); j >= 0 {
				return j
			}
			return cutDown(hi, x0, x1, y, y1)
		}
	}
	for x := x0 + 1; x < x1; x++ {
		if lo, hi, ok := split(x, false); ok {
			if j := cutDown(lo, x0, x, y0, y1); j >= 0 {
				return j
			}
			return cutDown(hi, x, x1, y0, y1)
		}
	}
	first := pieces[0].index
	for _, p := range pieces {
		first = min(first, p.index)
	}
	return first
}

// prefixProblem moves an error's problem keys under at.
func prefixProblem(at string, err error) *SpecError {
	var se *SpecError
	if !errors.As(err, &se) {
		return invalid(at, err.Error())
	}
	out := &SpecError{Status: se.Status, Problems: map[string]validation.Error{}}
	for k, v := range se.Problems {
		out.Problems[at+"."+k] = v
	}
	return out
}
