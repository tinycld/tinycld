package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

// backupsPath is the org backup API's namespace. Not "/api/backups":
// PocketBase owns that path for its own pb_data zip API.
const backupsPath = "/api/org-backups"

func newBackupCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up and restore this organization",
		Long:  "Create an encrypted backup of the whole organization, inspect one, restore one, or list past runs.",
	}
	cmd.AddCommand(newBackupCreateCmd(d), newBackupInspectCmd(d), newBackupRestoreCmd(d), newBackupListCmd(d))
	return cmd
}

// ledgerRow mirrors a backups record as the server exports it. Both read paths
// produce it: GET /api/org-backups/{id} (one row) and the records API (the
// list).
type ledgerRow struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	Started    string          `json:"started"`
	Finished   string          `json:"finished"`
	Bytes      int64           `json:"bytes"`
	Sha256     string          `json:"sha256"`
	TargetHost string          `json:"target_host"`
	Error      string          `json:"error"`
	Manifest   json.RawMessage `json:"manifest"`
	Metadata   map[string]any  `json:"metadata"`
}

func (r ledgerRow) terminal() bool {
	switch r.Status {
	case "succeeded", "failed", "interrupted":
		return true
	}
	return false
}

// passphraseSource wires the deps' stubs into ui.PassphraseSource. The
// passphrase is never a flag: flags land in `ps` output and shell history.
func passphraseSource(d *deps, file string) ui.PassphraseSource {
	return ui.PassphraseSource{
		File:         file,
		Env:          d.getenv,
		Interactive:  d.isInteractive,
		ReadPassword: d.readPassword,
		Stdin:        d.stdinFile,
		Stderr:       d.stderr,
	}
}

const (
	pollInterval = 2 * time.Second
	// reconnectWindow is how long a poll tolerates an unreachable or restoring
	// server. A restore ends by replacing the process, and the ledger row is the
	// only record of the outcome that survives it, so the poll has to outlive the
	// restart rather than report the disconnect as the result.
	reconnectWindow = 5 * time.Minute
)

// pollOptions carries the two things that differ between following a backup
// and following a restore. A restore can stall on an expired source URL and can
// end by replacing the database its own row lives in; a backup does neither, and
// must not be given either behaviour by accident.
type pollOptions struct {
	// onWaiting supplies a fresh source URL for a restore that stalled on an
	// expired presigned link. Nil makes that state an error, because nothing
	// else will move it along.
	onWaiting func(ledgerRow) (string, error)
	// followSwap chases a vanished row into the restored database. Only a
	// restore replaces the database, so only a restore sets it: on a backup a
	// 404 means the row is simply gone, and looking for a follow-up row that
	// cannot exist would report a confusing error for a plain data loss.
	followSwap bool
}

// pollRow follows a ledger row until it reaches a terminal status, is staged
// awaiting a manual restart, or disappears with the database it lived in.
func pollRow(ctx context.Context, d *deps, c *client.Client, id string, o output.Options, opts pollOptions) (ledgerRow, error) {
	var last ledgerRow
	lastBytes := int64(-1)
	var downSince time.Time
	sleep := d.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	now := d.now
	if now == nil {
		now = time.Now
	}
	window := reconnectWindow
	for {
		var row ledgerRow
		err := c.GetJSON(ctx, backupsPath+"/"+id, &row)
		switch {
		case err == nil:
			downSince = time.Time{}
			if row.Bytes != lastBytes && row.Bytes > 0 && !row.terminal() {
				o.Info(d.stderr, "%s transferred", output.FormatBytes(row.Bytes))
				lastBytes = row.Bytes
			}
			if row.Status == "waiting_for_source" && last.Status != "waiting_for_source" {
				if opts.onWaiting == nil {
					return row, Failed(errors.New("the restore is waiting for a fresh source URL"))
				}
				fresh, werr := opts.onWaiting(row)
				if werr != nil {
					return row, werr
				}
				if perr := c.PatchJSON(ctx, backupsPath+"/restore/"+id, map[string]string{"source": fresh}, nil); perr != nil {
					return row, Failed(perr)
				}
			}
			last = row
			if row.terminal() {
				return row, nil
			}
			// Staged but unapplied: the server built the restore, found nothing
			// would end its own process (a dev-mode server's restart request is
			// a no-op) and went back to serving its CURRENT data. The row stays
			// "running" and never becomes terminal, so without this the poll
			// would loop until the context died — a hang on the one outcome
			// that needs the operator to act.
			if awaitingRestart(row) {
				return row, nil
			}
		case errors.Is(err, client.ErrAuthExpired):
			return last, err
		case isRestoringError(err):
			// The server is up but is staging a restore, so every request
			// outside the exempt list meets a 503. That is the restore working,
			// not a failure — and this poll may be following THAT restore. It
			// shares the reconnect window with an unreachable server, because it
			// is the same wait for the same event.
			if downSince.IsZero() {
				downSince = now()
				o.Info(d.stderr, "server is restoring — waiting")
			} else if now().Sub(downSince) > window {
				return last, Failed(fmt.Errorf("the server was still restoring after %s: %w", window, err))
			}
		case isConnectionError(err):
			if downSince.IsZero() {
				downSince = now()
				o.Info(d.stderr, "server unreachable — waiting for it to come back")
			} else if now().Sub(downSince) > window {
				return last, Failed(fmt.Errorf("the server did not come back within %s: %w", window, err))
			}
		default:
			// A 404 after the row has been read at least once means the restore
			// replaced the database the row lived in — the row is gone with it,
			// and the outcome was written into the restored database as a NEW
			// row pointing back at this job. Following that pointer is the only
			// way to answer "did it work". Matched on the status, not on
			// "HTTP 404" in the message: the message is prose the server may
			// reword.
			var api *client.APIError
			if errors.As(err, &api) && api.Status == http.StatusNotFound && last.ID != "" {
				if opts.followSwap {
					return outcomeAfterSwap(ctx, c, id, last)
				}
				return last, Failed(fmt.Errorf(
					"%s %s: its ledger row disappeared mid-run — run `tinycld backup list`", last.Kind, id))
			}
			return last, Failed(err)
		}
		if err := ctx.Err(); err != nil {
			return last, err
		}
		sleep(pollInterval)
	}
}

// awaitingRestart reports whether the server staged this restore and then found
// nothing would restart it, so it is armed but unapplied.
func awaitingRestart(row ledgerRow) bool {
	staged, _ := row.Metadata["awaiting_restart"].(bool)
	return staged
}

// outcomeAfterSwap finds the row the restored database recorded for jobID. The
// restore replaced the database the original row lived in, so the boot that
// came up on the restored data wrote its own row — succeeded or failed —
// carrying metadata.restored_from_job = jobID.
//
// last is what the poll saw before the swap, returned when no such row exists
// yet: the outcome is genuinely not knowable at that moment, and inventing
// "succeeded" from a 404 would report a rolled-back restore as a good one.
func outcomeAfterSwap(ctx context.Context, c *client.Client, jobID string, last ledgerRow) (ledgerRow, error) {
	rows, err := client.ListAll[ledgerRow](ctx, c, "backups",
		client.Filter("kind = 'restore' && metadata.restored_from_job = {:id}", map[string]any{"id": jobID}),
		"-started")
	if err != nil {
		return last, Failed(fmt.Errorf("the restore's row is gone and its outcome could not be read: %w", err))
	}
	if len(rows) == 0 {
		return last, Failed(fmt.Errorf(
			"restore %s: the server replaced its database and recorded no outcome — run `tinycld backup list`", jobID))
	}
	return rows[0], nil
}

// isRestoringError reports whether err is the maintenance 503 a server answers
// while it stages a restore.
//
// Both halves have to match. The status alone is any temporary refusal, and a
// poll that waited out every 503 would sit through a genuine overload for the
// whole window; the body's {"status":"restoring"} is what says the server is
// deliberately not serving, which is the state the poll is waiting for.
func isRestoringError(err error) bool {
	var api *client.APIError
	if !errors.As(err, &api) || api.Status != http.StatusServiceUnavailable {
		return false
	}
	var body struct {
		Status string `json:"status"`
	}
	if jerr := json.Unmarshal(api.Body, &body); jerr != nil {
		return false
	}
	return body.Status == "restoring"
}

// isConnectionError reports whether err means the server could not be reached
// at all, as opposed to answering with something.
//
// Typed, not a substring scan on the message: a poll that mistakes a decode
// failure ("unexpected EOF" in a truncated body) for a disconnect retries it
// for the whole reconnect window instead of reporting it, so a real bug reads
// as a slow restart. Everything the transport reports for an unreachable host
// is a *url.Error or a net.Error; a body that arrives and does not parse is
// neither.
func isConnectionError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	// A keep-alive connection the server closed mid-exchange surfaces as bare
	// io.EOF (or io.ErrUnexpectedEOF) from the transport, with no wrapper.
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED)
}

// finishExit turns a terminal row into the command's exit: a succeeded run
// reports its size, anything else carries the row's reason.
func finishExit(row ledgerRow, o output.Options, stderr io.Writer) error {
	if row.Status == "succeeded" {
		o.Info(stderr, "%s: succeeded (%s)", row.Kind, output.FormatBytes(row.Bytes))
		return nil
	}
	reason := row.Error
	if reason == "" {
		reason = row.Status
	}
	return Failed(fmt.Errorf("%s %s: %s", row.Kind, row.Status, reason))
}
