// Package output renders command results as a table, JSON, or CSV.
// Commands hand over headers+rows for the tabular formats and a typed value
// for JSON, so --json stays stable regardless of table cosmetics.
package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Format string

const (
	Table Format = "table"
	JSON  Format = "json"
	CSV   Format = "csv"
)

func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case Table, JSON, CSV:
		return Format(s), nil
	}
	return "", fmt.Errorf("invalid --output %q (expected table, json, or csv)", s)
}

type Options struct {
	Format  Format
	Quiet   bool
	NoColor bool
	TTY     bool
}

var headerStyle = lipgloss.NewStyle().Bold(true).Faint(true)

// Write renders one result set. raw is the value serialized for JSON and
// must be a stable struct/slice, never a map built ad hoc.
func (o Options) Write(w io.Writer, headers []string, rows [][]string, raw any) error {
	switch o.Format {
	case JSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "    ")
		return enc.Encode(raw)
	case CSV:
		cw := csv.NewWriter(w)
		if err := cw.Write(headers); err != nil {
			return err
		}
		if err := cw.WriteAll(rows); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	default:
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		if len(headers) > 0 {
			line := strings.Join(headers, "\t")
			if o.color() {
				line = headerStyle.Render(line)
			}
			fmt.Fprintln(tw, line)
		}
		for _, r := range rows {
			fmt.Fprintln(tw, strings.Join(r, "\t"))
		}
		return tw.Flush()
	}
}

// Info prints progress/status chatter. Suppressed by --quiet and excluded
// from machine formats' stdout contract (callers send it to stderr for
// json/csv runs).
func (o Options) Info(w io.Writer, format string, a ...any) {
	if o.Quiet {
		return
	}
	fmt.Fprintf(w, format+"\n", a...)
}

func (o Options) color() bool { return !o.NoColor && o.TTY }

// FromCommand rebuilds Options and the --yes answer from the root's
// inherited persistent flags. Member command packages receive only
// (root, *client.Client) at wiring time, so the flag set — not the root's
// private deps struct — is their contract with the shell.
func FromCommand(cmd *cobra.Command) (Options, bool, error) {
	formatFlag, _ := cmd.Flags().GetString("output")
	format, err := ParseFormat(formatFlag)
	if err != nil {
		return Options{}, false, err
	}
	if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
		format = JSON
	}
	quiet, _ := cmd.Flags().GetBool("quiet")
	noColor, _ := cmd.Flags().GetBool("no-color")
	yes, _ := cmd.Flags().GetBool("yes")
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	return Options{
		Format:  format,
		Quiet:   quiet,
		NoColor: noColor || !tty,
		TTY:     tty,
	}, yes, nil
}
