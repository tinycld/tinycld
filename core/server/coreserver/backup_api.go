package coreserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/format"
)

// minPassphrase is the floor the API enforces on every archive passphrase. An
// age scrypt recipient is only as strong as the phrase behind it, and an archive
// is the whole organization in one file — so the refusal is here, at the edge,
// rather than left to whatever the caller happens to send.
const minPassphrase = 12

const passphraseTooShort = "The passphrase must be at least 12 characters."

// maintenanceMiddlewareID names the handler that answers 503 while a restore is
// in flight. Named so the binding can be asserted and so a later change cannot
// silently register a second copy.
const maintenanceMiddlewareID = "tinycldBackupMaintenance"

// maintenancePriority runs the maintenance check BEFORE PocketBase's own
// loadAuthToken, and therefore before every record CRUD handler. A write that
// got through while a restore was staging would land in a pb_data the next
// process replaces, so the caller would be told it succeeded and the row would
// be gone after the restart. Lower number = earlier.
//
// The offset is the same one oauth/middleware.go's middlewarePriority uses, for
// the same reason — which makes the two a TIE. That is deliberate: they are
// independent and either order is correct. A 503 before the grant check tells a
// token-authenticated caller the server is restoring rather than whether its
// token is good, and a grant check first refuses an invalid token during a
// restore — both are honest answers, and neither lets a write through.
var maintenancePriority = apis.DefaultLoadAuthTokenMiddlewarePriority - 10

type backupBody struct {
	Target     string `json:"target"`
	Stream     bool   `json:"stream"`
	Passphrase string `json:"passphrase"`
}

// orgBackupsPrefix is the org backup API's route namespace.
//
// NOT "/api/backups": PocketBase already owns that path. apis.NewRouter binds
// its own superuser-only backup API there (a zip of pb_data — a different
// feature with a different audience), including POST /api/backups and
// GET /api/backups/{key}. Registering this API on the same paths makes
// http.ServeMux panic while the router's mux is built, which happens at boot —
// so the collision is not a test artifact, it stops the server starting.
// Unbinding PocketBase's routes was the alternative and is worse: its bundled
// admin UI calls them, so its Backups screen would break.
//
// "org-" says which backup this is: the whole organization, not the pb_data zip.
const orgBackupsPrefix = "/api/org-backups"

// RegisterBackupEndpoints binds the org backup API: the same routes in every
// composition, because every deployment's operator has the same claim on a copy
// of their own data. An embedder that wants to trigger a backup from outside the
// HTTP surface calls the backup package directly.
func RegisterBackupEndpoints(app core.App) {
	// Read at registration rather than per request: an operator who changes it
	// must restart, and a per-request read would let a stray environment write
	// loosen the check mid-flight.
	refreshBackupTargetPolicy()
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group(orgBackupsPrefix)
		g.POST("", func(re *core.RequestEvent) error { return handleBackupCreate(app, re) }).BindFunc(requireAdmin)
		// "/verify" and "/{id}" both match GET /api/org-backups/verify. Go's
		// ServeMux picks the MORE SPECIFIC pattern, not the first registered, so
		// the literal wins wherever it sits in this list.
		g.GET("/verify", func(re *core.RequestEvent) error { return handleBackupVerify(app, re) }).BindFunc(requireAdmin)
		g.POST("/restore", func(re *core.RequestEvent) error { return handleRestore(app, re) }).BindFunc(requireOwner)
		g.PATCH("/restore/{id}", handleRestoreSwap).BindFunc(requireOwner)
		g.GET("/{id}", func(re *core.RequestEvent) error { return handleBackupGet(app, re) }).BindFunc(requireAdmin)
		return e.Next()
	})
}

// RegisterBackupBoot wires the two things a boot has to do about backups.
//
// Before the database is opened: swap in data a restore staged before the last
// restart. After bootstrap, in ONE hook: finalize a swapped-in restore, then
// close rows a dead process left running.
//
// A process started ONLY as a boot probe — a full server a supervisor launches
// to ask "does this build boot?" and then kills — does none of it. Such a probe
// runs on the real data directory, so without the guard it performs the swap and
// the finalize that belong to the real boot, and a kill landing between them
// makes the real boot roll the restore back. A supervisor that boots the binary
// as a probe MUST set TINYCLD_BOOT_PROBE=1 for that process.
//
// FinalizeRestore goes FIRST, defensively. The two cannot collide as they stand
// — the finalize inserts its row already "succeeded" with started = now, and
// MarkInterrupted only rewrites "running" rows started before bootedAt — so the
// order is not load-bearing today and no test can pin it. It is still the right
// order, because either half could change: a finalize that inserted "running"
// first, or a MarkInterrupted that stopped filtering on bootedAt, would close the
// row the finalize had just written and report a restore that worked as
// interrupted. Keep them adjacent and in this order so that change stays safe.
func RegisterBackupBoot(app core.App) {
	bootedAt := time.Now()
	probe := backup.IsBootProbe()
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if probe {
			srvLog.Info("boot probe: restore state left untouched for the real boot")
			return e.Next()
		}
		// Before e.Next(): PocketBase opens the database inside it, and the swap
		// renames pb_data as a whole.
		if err := backup.ApplyPendingRestore(app.DataDir()); err != nil {
			return fmt.Errorf("apply pending restore: %w", err)
		}
		if err := e.Next(); err != nil {
			return err
		}
		// A snapshot from a run the previous process never finished is dead
		// weight: the archive it fed is gone with the process.
		if err := os.RemoveAll(filepath.Join(backup.LedgerPath(app), "backup-tmp")); err != nil {
			srvLog.Warn("could not clear the backup scratch directory", "err", err)
		}
		if err := backup.FinalizeRestore(app); err != nil {
			srvLog.Error("could not finalize a restore", "err", err)
		}
		if err := backup.MarkInterrupted(app, bootedAt); err != nil {
			srvLog.Warn("could not close interrupted backup rows", "err", err)
		}
		return nil
	})
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id:       maintenanceMiddlewareID,
			Priority: maintenancePriority,
			Func:     backup.MaintenanceMiddleware(),
		})
		// A transfer is started from an HTTP handler but outlives it, so its
		// only other bound lifetime is the process. Without this a target that
		// accepts and never reads holds the transfer goroutine — and the
		// installjob interlock behind it — until the process is killed, so no
		// backup, restore or package install can run again.
		backup.SetShutdown(context.Background())
		return e.Next()
	})
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		backup.CancelAll()
		return e.Next()
	})
}

// passphraseRecipient refuses a short phrase before anything is attempted.
func passphraseRecipient(p string) (age.Recipient, error) {
	if len(p) < minPassphrase {
		return nil, errors.New(passphraseTooShort)
	}
	return age.NewScryptRecipient(p)
}

// initiatorOf is the users id to record against a run, or "" for a superuser —
// initiated_by is a relation into users, so a PB superuser has no id to file.
func initiatorOf(re *core.RequestEvent) string {
	if re.Auth != nil && re.Auth.Collection().Name == "users" {
		return re.Auth.Id
	}
	return ""
}

func handleBackupCreate(app core.App, re *core.RequestEvent) error {
	var body backupBody
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("invalid JSON body", err)
	}
	rcpt, err := passphraseRecipient(body.Passphrase)
	if err != nil {
		return re.BadRequestError(err.Error(), nil)
	}
	initiator := initiatorOf(re)
	if body.Stream {
		re.Response.Header().Set("Content-Type", "application/octet-stream")
		re.Response.Header().Set("Content-Disposition", `attachment; filename="tinycld-backup.age"`)
		re.Response.Header().Set("Cache-Control", "no-store")
		re.Response.WriteHeader(http.StatusOK)
		_, err := backup.Run(app, backup.Request{
			Kind: backup.KindManual, Recipient: rcpt,
			Sink: flushingSink{w: re.Response}, Initiator: initiator, Request: re,
		})
		if err != nil {
			// The status line is already sent, so there is no error to return
			// that the client would see as one. It sees a truncated body, and
			// the ledger row says why — which is the same evidence an operator
			// reads for a target-URL run.
			srvLog.Error("a streamed backup failed after its headers were sent", "err", err)
		}
		return nil
	}
	// Anything but http(s) is refused rather than handed to the sink: the sink
	// would try it, fail, and record a run that never had a chance.
	if !httpScheme(body.Target) {
		return re.BadRequestError("The target must be an http(s) URL.", nil)
	}
	if err := checkBackupTarget(body.Target); err != nil {
		return re.BadRequestError(err.Error(), nil)
	}
	id, err := backup.Start(app, backup.Request{
		Kind:      backup.KindManual,
		Recipient: rcpt,
		// NOT re.Request.Context(): Start runs the backup on a goroutine that
		// outlives this handler, and net/http cancels the request context the
		// moment the handler returns — which killed the PUT after its first
		// bytes ("io: read/write on closed pipe"). The streamed branch above is
		// synchronous, so it is the only one that may use the request context.
		Sink:      format.NewPutSink(context.WithoutCancel(re.Request.Context()), body.Target),
		Initiator: initiator,
		// Only the hostname reaches the ledger: a presigned target URL carries
		// its own credentials in the query string.
		TargetHost: backup.HostOnly(body.Target),
		Request:    re,
	})
	return backupStartError(re, id, err)
}

// backupStartError maps the engine's two refusals onto the statuses a client can
// act on, and everything else onto a 500.
func backupStartError(re *core.RequestEvent, id string, err error) error {
	switch {
	case err == nil:
		return re.JSON(http.StatusAccepted, map[string]string{"id": id})
	case errors.Is(err, backup.ErrBusy):
		return re.JSON(http.StatusConflict, map[string]string{
			"message": "Another backup, restore or package job is running.",
		})
	case errors.Is(err, backup.ErrRateLimit):
		return re.JSON(http.StatusTooManyRequests, map[string]string{
			"message": "The daily limit for manual backups has been reached.",
		})
	default:
		return re.InternalServerError("could not start the backup", err)
	}
}

// flushingSink pushes each chunk to the client rather than waiting for the
// transport's buffer to fill, so a slow walk over a large storage tree still
// streams instead of looking hung.
type flushingSink struct{ w http.ResponseWriter }

func (f flushingSink) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}

func (f flushingSink) Close() error { return nil }

func handleBackupGet(app core.App, re *core.RequestEvent) error {
	row, err := app.FindRecordById("backups", re.Request.PathValue("id"))
	if err != nil {
		return re.NotFoundError("backup not found", nil)
	}
	return re.JSON(http.StatusOK, row.PublicExport())
}

func handleBackupVerify(app core.App, re *core.RequestEvent) error {
	rep, err := backup.Verify(app)
	if err != nil {
		return re.InternalServerError("could not verify this deployment's data", err)
	}
	return re.JSON(http.StatusOK, rep)
}

type restoreBody struct {
	Source     string `json:"source"`
	Passphrase string `json:"passphrase"`
	Force      bool   `json:"force"`
}

// handleRestore takes an archive two ways, and they differ in more than shape.
//
// An upload IS the request body, so it cannot outlive the request: the restore
// runs synchronously and a package-set mismatch surfaces as the 409 below. The
// connection then drops when the rebuilder ends the process, which the CLI
// expects — after reconnecting it reads the outcome from GET /api/org-backups/{id},
// because the row is the only thing that survives the restart.
//
// A URL can be re-read, so that branch starts the restore on a goroutine and
// answers with the job id. A mismatch there lands on the row instead.
func handleRestore(app core.App, re *core.RequestEvent) error {
	req := backup.RestoreRequest{Initiator: initiatorOf(re), Request: re}
	upload := strings.HasPrefix(re.Request.Header.Get("Content-Type"), "multipart/")
	if upload {
		if err := readUploadedArchive(re, &req); err != nil {
			return err
		}
	} else if err := readRemoteArchive(re, &req); err != nil {
		return err
	}
	if req.Source == nil {
		return re.BadRequestError("No archive was supplied.", nil)
	}

	var jobID string
	var err error
	if upload {
		jobID, err = backup.Restore(app, req)
	} else {
		jobID, err = backup.StartRestore(app, req)
	}
	var mismatch *backup.MismatchError
	switch {
	case err == nil:
		return re.JSON(http.StatusAccepted, map[string]string{"jobId": jobID})
	case errors.As(err, &mismatch):
		// The diff is the actionable part: it names what this binary would have
		// to gain or drop to run the archive's package set.
		return re.JSON(http.StatusConflict, map[string]any{
			"message": err.Error(), "diff": mismatch.Diff, "jobId": jobID,
		})
	case errors.Is(err, backup.ErrBusy):
		return re.JSON(http.StatusConflict, map[string]string{
			"message": "Another backup, restore or package job is running.",
		})
	default:
		return re.JSON(http.StatusUnprocessableEntity, map[string]string{
			"message": err.Error(), "jobId": jobID,
		})
	}
}

// maxPassphraseField bounds what is read from the passphrase form field. A
// passphrase is a phrase; anything longer is a client sending the archive under
// the wrong field name, and reading it all would buffer the whole upload.
const maxPassphraseField = 1024

// readUploadedArchive walks the multipart stream to the file part. The fields
// must arrive BEFORE it — the CLI writes them in that order — so the archive is
// streamed into the restore rather than buffered in memory.
func readUploadedArchive(re *core.RequestEvent, req *backup.RestoreRequest) error {
	mr, err := re.Request.MultipartReader()
	if err != nil {
		return re.BadRequestError("invalid multipart body", err)
	}
	var passphrase string
	seenPassphrase := false
	for {
		part, err := mr.NextPart()
		if err != nil {
			return re.BadRequestError("the archive part is missing", err)
		}
		switch part.FormName() {
		case "passphrase":
			// One byte over the cap is read so a too-long value is REFUSED rather
			// than silently truncated to a different passphrase, which would fail
			// to decrypt with a message about the archive instead of the field.
			raw, err := io.ReadAll(io.LimitReader(part, maxPassphraseField+1))
			if err != nil {
				return re.BadRequestError("could not read the passphrase", err)
			}
			if len(raw) > maxPassphraseField {
				return re.BadRequestError("The passphrase is too long.", nil)
			}
			passphrase, seenPassphrase = string(raw), true
			continue
		case "force":
			// The VALUE decides, not the field's presence. Force skips the
			// package-set check and the rebuild — "this binary, that data" — so
			// a client sending force=false must not get it.
			raw, err := io.ReadAll(io.LimitReader(part, maxPassphraseField))
			if err != nil {
				return re.BadRequestError("could not read the force flag", err)
			}
			req.Force = isTruthyFormValue(string(raw))
			continue
		case "archive":
			// "No passphrase yet" and "too short" are different mistakes: the
			// first means the parts arrived in the wrong order (the archive must
			// come last so it can be streamed), the second is a bad value. One
			// message for both sends a client hunting the wrong problem.
			if !seenPassphrase {
				return re.BadRequestError(
					"The passphrase field must come before the archive part.", nil)
			}
			if len(passphrase) < minPassphrase {
				return re.BadRequestError(passphraseTooShort, nil)
			}
			identity, err := age.NewScryptIdentity(passphrase)
			if err != nil {
				return re.BadRequestError("invalid passphrase", err)
			}
			req.Identity = identity
			req.Source = part
			// Not a hostname: an upload has no origin this deployment can name.
			req.SourceHost = "upload"
			return nil
		default:
			continue
		}
	}
}

// isTruthyFormValue reads a multipart flag the way an HTML form and a CLI both
// spell "yes". Anything else — including "0" and "false" — is no.
func isTruthyFormValue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func readRemoteArchive(re *core.RequestEvent, req *backup.RestoreRequest) error {
	var body restoreBody
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("invalid JSON body", err)
	}
	if len(body.Passphrase) < minPassphrase {
		return re.BadRequestError(passphraseTooShort, nil)
	}
	identity, err := age.NewScryptIdentity(body.Passphrase)
	if err != nil {
		return re.BadRequestError("invalid passphrase", err)
	}
	if !httpScheme(body.Source) {
		return re.BadRequestError("The source must be an http(s) URL.", nil)
	}
	if err := checkBackupTarget(body.Source); err != nil {
		return re.BadRequestError(err.Error(), nil)
	}
	// The source is registered for a URL swap, so an expiring presigned link can
	// be replaced mid-transfer instead of restarting from zero.
	//
	// WithoutCancel for the same reason the backup sink uses it: the URL branch
	// of handleRestore calls StartRestore, which reads the archive on a
	// goroutine after this handler has returned and its context is cancelled.
	src := format.NewRangeSource(context.WithoutCancel(re.Request.Context()), body.Source)
	req.Identity, req.Source, req.Ranged = identity, src, src
	req.Force, req.SourceHost = body.Force, backup.HostOnly(body.Source)
	return nil
}

// handleRestoreSwap hands a waiting restore a fresh URL for the same object. A
// presigned URL can expire mid-transfer on a large archive, and re-downloading
// from zero can cost more than the whole restore.
func handleRestoreSwap(re *core.RequestEvent) error {
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil || body.Source == "" {
		return re.BadRequestError("A source URL is required.", err)
	}
	if !httpScheme(body.Source) {
		return re.BadRequestError("The source must be an http(s) URL.", nil)
	}
	if err := checkBackupTarget(body.Source); err != nil {
		return re.BadRequestError(err.Error(), nil)
	}
	err := backup.SwapSource(re.Request.PathValue("id"), body.Source)
	switch {
	case err == nil:
		return re.NoContent(http.StatusNoContent)
	case errors.Is(err, backup.ErrNotWaiting):
		return re.NotFoundError("No restore is waiting for a source.", nil)
	default:
		// Anything else is this deployment's fault, not the caller's; a 404 here
		// would send an operator looking for a job that does exist.
		return re.InternalServerError("could not hand the restore a new source", err)
	}
}
