package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

func newBackupCreateCmd(d *deps) *cobra.Command {
	var out, to, ppFile string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a backup and stream it to a file, or have the server upload it to a URL",
		Long: "Creates one encrypted archive of the whole organization.\n\n" +
			"--out streams the archive through this machine. --to hands the server a\n" +
			"presigned PUT URL and follows the run in the ledger, so the archive never\n" +
			"crosses your connection.\n\n" +
			"The passphrase is read from " + ui.PassphraseEnv + ", from --passphrase-file, or\n" +
			"interactively. There is no flag for it: a flag would be visible in `ps`\n" +
			"and left in shell history. Without the passphrase the archive cannot be\n" +
			"read — nobody can recover it for you.",
		Example: "  tinycld backup create --out ./backup.age\n" +
			"  tinycld backup create --out - | aws s3 cp - s3://bucket/backup.age\n" +
			"  tinycld backup create --to 'https://…presigned PUT…'",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (out == "") == (to == "") {
				return Usage(errors.New("pass exactly one of --out or --to"))
			}
			o := d.out
			// Before the client is even built: a passphrase the server would
			// reject must not travel, and a mistyped one must not start a run.
			pp, err := passphraseSource(d, ppFile).Read(true)
			if err != nil {
				return Usage(err)
			}
			c, _, err := d.apiClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			if to != "" {
				var res struct {
					ID string `json:"id"`
				}
				if err := c.PostJSON(ctx, backupsPath, map[string]any{"target": to, "passphrase": pp}, &res); err != nil {
					return Failed(err)
				}
				o.Info(d.stderr, "backup %s started", res.ID)
				row, err := pollRow(ctx, d, c, res.ID, o, pollOptions{})
				if err != nil {
					return err
				}
				return finishExit(row, o, d.stderr)
			}
			return streamBackup(cmd, d, o, c, out, pp)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write the archive to this file ('-' for stdout)")
	cmd.Flags().StringVar(&to, "to", "", "have the server upload the archive to this presigned PUT URL")
	cmd.Flags().StringVar(&ppFile, "passphrase-file", "", "read the passphrase from this file")
	return cmd
}

// streamBackup pulls the archive through this machine to out. Split out of RunE
// so the destination handling (stdout, an existing file, the early-truncation
// message) reads in one piece.
func streamBackup(cmd *cobra.Command, d *deps, o output.Options, c *client.Client, out, pp string) error {
	// Asked before the request, so a refusal costs nothing; the file itself is
	// only truncated once the server has accepted the run.
	if out != "-" {
		if err := ui.ConfirmOverwrite(o, d.yes, cmd.InOrStdin(), d.stderr, out); err != nil {
			return Usage(err)
		}
	}
	body, _, err := c.PostStream(cmd.Context(), backupsPath, map[string]any{"stream": true, "passphrase": pp})
	if err != nil {
		return Failed(err)
	}
	defer body.Close()

	dest := d.stdout
	var file *os.File
	if out != "-" {
		f, ferr := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if ferr != nil {
			return Failed(ferr)
		}
		file, dest = f, f
	}

	prog := ui.NewProgress(o, d.stderr, "downloading")
	counter := &progressWriter{p: prog}
	n, copyErr := io.Copy(io.MultiWriter(dest, counter), body)
	prog.Done()
	if file != nil {
		if cerr := file.Close(); copyErr == nil {
			copyErr = cerr
		}
	}
	if copyErr != nil {
		// The server sent its headers before the walk began, so a failure
		// mid-archive arrives as a truncated body with no status to read. The
		// ledger row carries the reason.
		return Failed(fmt.Errorf("the stream ended early after %s: %w — the server's backup ledger has the reason",
			output.FormatBytes(n), copyErr))
	}
	if out == "-" {
		o.Info(d.stderr, "wrote %s to stdout", output.FormatBytes(n))
	} else {
		o.Info(d.stderr, "wrote %s (%s)", out, output.FormatBytes(n))
	}
	return nil
}

// progressWriter feeds a running total to the indicator, which renders a
// cumulative count rather than a per-chunk one.
type progressWriter struct {
	p       *ui.Progress
	written int64
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.written += int64(len(b))
	w.p.Update(w.written, 0)
	return len(b), nil
}
