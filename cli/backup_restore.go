package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
	"tinycld.org/core/backup/format"
)

// restoreResponse is the accepted-restore body. Named rather than inlined so
// the multipart call's type parameter and the JSON call's target are the same
// shape.
type restoreResponse struct {
	JobID string `json:"jobId"`
}

func newBackupRestoreCmd(d *deps) *cobra.Command {
	var from, ppFile string
	var force bool
	cmd := &cobra.Command{
		Use:   "restore --from <file|url>",
		Short: "Replace this organization's data with a backup",
		Long: "Restores a backup into the server you are signed in to. ALL current data\n" +
			"is replaced. The server takes a safety copy first and restarts to apply\n" +
			"the restore, so this command loses its connection and then reads the\n" +
			"outcome back from the backup ledger.\n\n" +
			"A local file is verified here before anything is uploaded. A URL is\n" +
			"fetched by the server, which can be handed a fresh link if the presigned\n" +
			"one expires mid-transfer.",
		Example: "  tinycld backup restore --from ./backup.age\n" +
			"  tinycld backup restore --from 'https://…presigned GET…'",
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if from == "" {
				return Usage(errors.New("--from is required"))
			}
			o := d.out
			pp, err := passphraseSource(d, ppFile).Read(false)
			if err != nil {
				return Usage(err)
			}
			fromURL := isHTTPURL(from)
			if !fromURL {
				// An upload that turns out to be corrupt costs the whole
				// transfer and then replaces live data, so the local check
				// happens before the request rather than on the server.
				if err := verifyLocalArchive(d, o, from, pp); err != nil {
					return err
				}
			}
			// ONE buffered reader for every prompt this command makes.
			// ui.Confirm wraps what it is given in bufio.NewReader, which
			// reuses an existing *bufio.Reader — so sharing one means the
			// confirmation cannot swallow the line the URL prompt is waiting
			// for, which is exactly what a fresh reader per prompt did.
			stdin := bufio.NewReader(cmd.InOrStdin())
			ok, err := ui.Confirm(o, d.yes, stdin, d.stderr,
				"Replace ALL current data on "+serverName(d)+" with this backup?")
			if err != nil {
				return Usage(err)
			}
			if !ok {
				return Usage(errors.New("restore cancelled"))
			}

			c, _, err := d.apiClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			var res restoreResponse
			if fromURL {
				err = c.PostJSON(ctx, backupsPath+"/restore",
					map[string]any{"source": from, "passphrase": pp, "force": force}, &res)
			} else {
				// Ordered, not sorted: the server reads the passphrase before
				// it will accept the archive part, so it can stream the upload
				// straight into the restore instead of buffering it.
				fields := []client.Field{{Name: "passphrase", Value: pp}}
				if force {
					fields = append(fields, client.Field{Name: "force", Value: "true"})
				}
				prog := ui.NewProgress(o, d.stderr, "uploading")
				res, err = client.PostMultipartFields[restoreResponse](ctx, c, backupsPath+"/restore", fields,
					[]client.FilePart{{Field: "archive", Name: filepath.Base(from), Path: from}}, prog.Func())
				prog.Done()
			}
			if err != nil {
				return restoreStartError(err, d.stderr)
			}
			o.Info(d.stderr, "restore %s started", res.JobID)

			row, err := pollRow(ctx, d, c, res.JobID, o, pollOptions{
				onWaiting:  freshSourcePrompt(d, stdin),
				followSwap: true,
			})
			if err != nil {
				return err
			}
			if awaitingRestart(row) && !row.terminal() {
				// The data is staged and armed on disk; only a restart applies
				// it. Exit 0: nothing failed, and the operator has one step left.
				o.Info(d.stderr, "restore staged; restart the server to apply it")
				return nil
			}
			return finishExit(row, o, d.stderr)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "archive file, or a presigned GET URL the server fetches")
	cmd.Flags().StringVar(&ppFile, "passphrase-file", "", "read the passphrase from this file")
	cmd.Flags().BoolVar(&force, "force", false, "restore the data even if the package set differs (single binary only)")
	return cmd
}

// verifyLocalArchive reads the archive and prints what it holds, refusing the
// restore when a member does not match its recorded checksum.
func verifyLocalArchive(d *deps, o output.Options, path, pp string) error {
	identity, err := age.NewScryptIdentity(pp)
	if err != nil {
		return Usage(err)
	}
	f, err := os.Open(path)
	if err != nil {
		return Usage(err)
	}
	manifest, report, verr := format.Inspect(f, identity)
	f.Close()
	if verr != nil {
		return Failed(fmt.Errorf("the archive does not verify; refusing to restore it: %w", verr))
	}
	// To stderr: the command's own result goes to stdout, and a scripted
	// caller reading that must not have the manifest mixed into it.
	return writeInspect(o, d.stderr, manifest, report)
}

// freshSourcePrompt supplies a replacement URL for a restore whose presigned
// source expired mid-transfer. Under --yes, or with no terminal, there is
// nobody to ask, so the restore ends rather than waiting for a URL that will
// never arrive.
func freshSourcePrompt(d *deps, stdin *bufio.Reader) func(ledgerRow) (string, error) {
	return func(ledgerRow) (string, error) {
		if d.yes || !d.isInteractive {
			return "", Failed(errors.New(
				"the restore is waiting for a fresh source URL; rerun without --yes, on a terminal, to supply one"))
		}
		fmt.Fprint(d.stderr, "The source URL expired. Fresh URL: ")
		raw, err := stdin.ReadString('\n')
		if err != nil && raw == "" {
			return "", Failed(fmt.Errorf("reading the fresh URL: %w", err))
		}
		line := strings.TrimSpace(raw)
		if line == "" {
			return "", Failed(errors.New("no URL given; the restore cannot continue"))
		}
		return line, nil
	}
}

const (
	// mismatchMessage is the substring core's *MismatchError carries. Matched
	// only on the fallback path, where there is no structured diff to read.
	mismatchMessage = "does not match this binary's package set"
	forceHint       = "Rerun with --force to restore the data anyway (single binary only)."
)

// restoreStartError renders the server's package-set refusal as the diff an
// operator can act on. The 409 body carries the diff as structure; the message
// text is the fallback for a server that predates it.
func restoreStartError(err error, stderr io.Writer) error {
	var api *client.APIError
	if !errors.As(err, &api) {
		return Failed(err)
	}
	var body struct {
		Message string `json:"message"`
		Diff    *struct {
			Missing      []string             `json:"missing"`
			Extra        []string             `json:"extra"`
			VersionDelta map[string][2]string `json:"versionDelta"`
		} `json:"diff"`
	}
	if jerr := json.Unmarshal(api.Body, &body); jerr != nil || body.Diff == nil {
		// No structured diff: either an older server, or a refusal of another
		// kind. The message still says which it was, and a mismatch still has
		// the same way out, so the hint is worth printing without the breakdown.
		if strings.Contains(api.Message, mismatchMessage) {
			fmt.Fprintln(stderr, "The archive's package set does not match the server:")
			fmt.Fprintln(stderr, "  "+api.Message)
			fmt.Fprintln(stderr, forceHint)
			return Failed(errors.New("package set mismatch"))
		}
		return Failed(err)
	}
	fmt.Fprintln(stderr, "The archive's package set does not match the server:")
	if len(body.Diff.Missing) > 0 {
		fmt.Fprintln(stderr, "  missing: "+strings.Join(body.Diff.Missing, ", ")+
			" (in the archive, not in this binary)")
	}
	if len(body.Diff.Extra) > 0 {
		fmt.Fprintln(stderr, "  extra: "+strings.Join(body.Diff.Extra, ", ")+
			" (in this binary, not in the archive)")
	}
	slugs := make([]string, 0, len(body.Diff.VersionDelta))
	for slug := range body.Diff.VersionDelta {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		v := body.Diff.VersionDelta[slug]
		fmt.Fprintf(stderr, "  %s: archive %s, binary %s\n", slug, v[0], v[1])
	}
	fmt.Fprintln(stderr, forceHint)
	return Failed(errors.New("package set mismatch"))
}

// serverName is the context name the restore is about to overwrite, so the
// confirmation names the server rather than asking about "this server".
func serverName(d *deps) string {
	cfg, err := d.loadConfig()
	if err != nil {
		return "the server"
	}
	name, _, err := cfg.Resolve(d.ctxFlag)
	if err != nil {
		return "the server"
	}
	return name
}
