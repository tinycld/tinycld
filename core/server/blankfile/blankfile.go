// Package blankfile lets a document package (text, calc, …) auto-attach a
// minimal valid backing file to a freshly-created drive_items row that was
// created with no file.
//
// Why this exists: creating a blank document used to require the *client* to
// materialize a valid empty office file (a Blob or a temp file) and upload it as
// multipart form data. That is brittle — React Native's Android FormData
// polyfill can't upload an in-memory Blob at all (the request commits server-side
// but the response read fails with "Network request failed"), so every client
// platform had to re-implement a temp-file workaround per format. Moving the
// blank-file synthesis to the server removes that whole class of client bugs: the
// client just inserts a drive_items row with no file, and the owning package's
// registration here attaches the skeleton.
//
// The skeleton is a static, embedded artifact supplied by the caller (via
// go:embed) — identical bytes to what the client used to upload. No runtime
// document generation is involved.
//
// Blank files are quota-exempt: this hook sets `size` for the Drive UI but does
// not run the storage-quota check (drive's own OnRecordCreate ran that against
// the incoming size, which is 0 for a no-file create).
package blankfile

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// driveItemsCollection is the collection blank documents live in. Matches the
// const each package server already uses (calc/server/persist.go etc.).
const driveItemsCollection = "drive_items"

// Register binds an OnRecordCreate("drive_items") hook that attaches `skeleton`
// (a valid empty file of the given mimeType) to any create whose mime_type
// matches and that carries no file yet.
//
//   - mimeType:  the drive_items.mime_type this package owns (e.g. the docx or
//     xlsx MIME). The hook is a no-op for any other value.
//   - filename:  the on-disk name given to the attached file. PocketBase renames
//     the blob to a content hash on save; this only seeds mime/extension.
//   - skeleton:  the embedded blank-file bytes (via go:embed). Must be non-empty.
//
// Idempotent and upload-safe: a create that already has a file set (a real
// upload) is left untouched, so this never overwrites user content. Multiple
// packages may each Register for their own mimeType against the same collection;
// each hook only fires for its match.
//
// The file is set on e.Record BEFORE e.Next(), so it persists in the same INSERT
// — no second save, and it composes with drive's own OnRecordCreate hook
// (quota/dedup/owner-share), which only touches `name`/`size`.
func Register(app core.App, mimeType, filename string, skeleton []byte) {
	if len(skeleton) == 0 {
		// A caller wiring up an empty skeleton is a build-time mistake (bad
		// embed path); fail loud at startup rather than silently attaching
		// zero bytes — a zero-byte file is exactly the corrupt-download bug
		// this package exists to prevent.
		panic(fmt.Sprintf("blankfile.Register(%q): skeleton is empty", mimeType))
	}

	app.OnRecordCreate(driveItemsCollection).BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("mime_type") != mimeType {
			return e.Next()
		}
		// Leave any create that already carries a file. Two forms to check:
		//   - GetString("file") — a filename already persisted (e.g. an update,
		//     or a record hydrated from an existing file).
		//   - GetUnsavedFiles("file") — a pending upload on THIS create. On a
		//     create hook the uploaded blob isn't a filename yet, so GetString
		//     returns "" while the file is very much present; without this check
		//     the hook would clobber a real upload's bytes with the skeleton.
		if e.Record.GetString("file") != "" || len(e.Record.GetUnsavedFiles("file")) > 0 {
			return e.Next()
		}

		f, err := filesystem.NewFileFromBytes(skeleton, filename)
		if err != nil {
			return fmt.Errorf("blankfile: build skeleton file (%s): %w", mimeType, err)
		}
		e.Record.Set("file", f)
		// Set size for the Drive UI column. Quota-exempt by design: drive's
		// OnRecordCreate already checked quota against the incoming size (0),
		// and we deliberately don't re-check for the fixed skeleton cost.
		e.Record.Set("size", len(skeleton))

		return e.Next()
	})
}
