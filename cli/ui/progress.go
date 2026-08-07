package ui

import (
	"fmt"
	"io"

	"tinycld.org/cli/output"
)

// Progress is a single-line byte-transfer indicator rewritten in place with
// \r. It renders only on a TTY without --quiet; otherwise every method is a
// no-op, so machine-readable output never sees control characters.
type Progress struct {
	w       io.Writer
	label   string
	active  bool
	started bool
}

// NewProgress builds an indicator for label writing to w (conventionally
// stderr, keeping stdout clean for the command's result).
func NewProgress(o output.Options, w io.Writer, label string) *Progress {
	return &Progress{w: w, label: label, active: o.TTY && !o.Quiet}
}

// Update rewrites the line. total <= 0 (unknown Content-Length) renders the
// running byte count alone.
func (p *Progress) Update(written, total int64) {
	if !p.active {
		return
	}
	p.started = true
	if total > 0 {
		pct := written * 100 / total
		fmt.Fprintf(p.w, "\r%s %s / %s (%d%%)", p.label,
			output.FormatBytes(written), output.FormatBytes(total), pct)
		return
	}
	fmt.Fprintf(p.w, "\r%s %s", p.label, output.FormatBytes(written))
}

// Done terminates the rewritten line so subsequent output starts clean.
func (p *Progress) Done() {
	if p.active && p.started {
		fmt.Fprintln(p.w)
	}
}

// Func adapts the indicator to the client.ProgressFunc signature.
func (p *Progress) Func() func(written, total int64) {
	if !p.active {
		return nil
	}
	return p.Update
}
