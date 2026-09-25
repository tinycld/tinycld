package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/spf13/cobra"

	"tinycld.org/cli/output"
	"tinycld.org/core/backup/format"
)

func newBackupInspectCmd(d *deps) *cobra.Command {
	var ppFile string
	cmd := &cobra.Command{
		Use:   "inspect <file|url>",
		Short: "Read an archive's manifest and verify every member, without restoring",
		Long: "Reads the archive end to end and checks every member against the\n" +
			"checksums recorded inside it. Nothing is written and no server is\n" +
			"contacted: the archive is decrypted on this machine.\n\n" +
			"A URL is read with range requests, so inspecting a remote archive still\n" +
			"downloads all of it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := d.out
			pp, err := passphraseSource(d, ppFile).Read(false)
			if err != nil {
				return Usage(err)
			}
			identity, err := age.NewScryptIdentity(pp)
			if err != nil {
				return Usage(err)
			}
			src, err := openArchive(cmd, args[0])
			if err != nil {
				return Usage(err)
			}
			defer src.Close()

			manifest, report, verr := format.Inspect(src, identity)
			if verr != nil && manifest.Format == "" {
				return Failed(fmt.Errorf("not a readable backup (wrong passphrase, or not an archive): %w",
					format.RedactURLError(verr)))
			}
			// Printed even on a failure: which member failed, and what the
			// archive claims to hold, is the whole point of the command.
			if err := writeInspect(o, d.stdout, manifest, report); err != nil {
				return err
			}
			if verr != nil {
				return Failed(fmt.Errorf("verification failed: %w", format.RedactURLError(verr)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ppFile, "passphrase-file", "", "read the passphrase from this file")
	return cmd
}

// openArchive resolves an argument that is either a local path or an http(s)
// URL into a reader.
func openArchive(cmd *cobra.Command, ref string) (io.ReadCloser, error) {
	if isHTTPURL(ref) {
		return format.NewRangeSource(cmd.Context(), ref), nil
	}
	return os.Open(ref)
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// writeInspect renders the manifest and the per-member verification as one
// field/value table, or as the raw manifest plus report under --json.
func writeInspect(o output.Options, w io.Writer, m format.Manifest, rep format.Report) error {
	rows := [][]string{
		{"format", m.Format},
		{"created", m.Created.UTC().Format(time.RFC3339)},
		{"instance", m.Instance},
		{"source", m.Source},
		{"kind", m.Kind},
		{"core", m.Core},
	}
	slugs := make([]string, 0, len(m.Packages))
	for slug := range m.Packages {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		value := m.Packages[slug]
		if lock := m.Lockfile[slug]; lock != "" {
			value += "  (" + lock + ")"
		}
		rows = append(rows, []string{"package " + slug, value})
	}
	rows = append(rows, []string{"files", fmt.Sprintf("%d (%s)", m.Counts.Files, output.FormatBytes(m.Counts.Bytes))})

	var total int64
	for _, member := range rep.Members {
		total += member.Size
		state := "FAIL"
		if member.OK {
			state = "OK"
		}
		rows = append(rows, []string{"member " + member.Name,
			fmt.Sprintf("%s  %s", output.FormatBytes(member.Size), state)})
	}
	rows = append(rows,
		[]string{"archive total", output.FormatBytes(total)},
		[]string{"verified", fmt.Sprint(rep.OK)},
	)
	raw := map[string]any{"manifest": m, "report": rep}
	return o.Write(w, []string{"FIELD", "VALUE"}, rows, raw)
}
