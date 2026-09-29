package apis

// Fork-only: the collection's viewRule gates EVERY file download, not only a
// `protected` one. See third_party/pocketbase/FORK.md.

import (
	"errors"

	"github.com/pocketbase/pocketbase/core"
)

// checkUnprotectedFileAccess evaluates the collection's viewRule for a file
// field that is NOT `protected`. Upstream's download handler checks the rule
// only for protected fields, and that check is left untouched.
//
// Upstream serves every other file publicly, and says so in field_file.go: "by
// default all files are publicly accessible … all file names have a random
// part appended which needs to be known by the user before accessing the
// file". That is security by obscurity, and it is the wrong default for this
// app: every file field we ship holds an org's data (mail attachments, drive
// items, cards attachments, text snapshots), a filename leaks through any
// share of the URL, and none of our collections had opted in — so a record id
// plus a filename downloaded anyone's file, unauthenticated.
//
// So `Protected` now means only "evaluate the rule as the ?token= file token
// ALONE" (upstream's semantics, kept exactly). Deliberately NOT renamed to an
// inverted `Public` flag: the field is persisted collection schema
// (`json:"protected"`, read by migrations/1717233556_v0.23_migrate.go), so
// flipping the stored key's meaning would make every existing collection
// deserialize as the opposite of what it is.
//
// A caller that fails the viewRule gets 404 instead of bytes. That is the
// point, but any client fetching a file it cannot read the record for breaks —
// which is correct and worth knowing when a thumbnail goes blank.
//
// A nil viewRule is skipped for upstream compatibility, not as a hole we lean
// on: nil means "superusers only" to CanAccessRecord, yet upstream has always
// served such a collection's files publicly and its own suite downloads from
// one unauthenticated. Every file-bearing collection we ship declares a rule,
// so the carve-out never applies to us.
func checkUnprotectedFileAccess(e *core.RequestEvent, record *core.Record, fileField *core.FileField) error {
	if fileField.Protected || record.Collection().ViewRule == nil {
		return nil
	}

	originalRequestInfo, err := e.RequestInfo()
	if err != nil {
		return e.InternalServerError("Failed to load request info", err)
	}

	// The rule is evaluated as the caller. A ?token= is still honoured when
	// one is supplied, because that is how this app's own clients fetch files
	// (core/file-viewer/use-authed-file-url.ts appends one to every URL so
	// native HTTP fetches, which carry no cookies or headers, can be
	// authorized at all).
	var authRecord *core.Record
	if token := e.Request.URL.Query().Get("token"); token != "" {
		authRecord, _ = e.App.FindAuthRecordByToken(token, core.TokenTypeFile)
	}
	if authRecord == nil {
		authRecord = originalRequestInfo.Auth
	}

	// create a shallow copy of the cached request data and adjust it to the current auth record (if any)
	requestInfo := *originalRequestInfo
	requestInfo.Context = core.RequestInfoContextProtectedFile
	requestInfo.Auth = authRecord

	if ok, _ := e.App.CanAccessRecord(record, &requestInfo, record.Collection().ViewRule); !ok {
		return e.NotFoundError("", errors.New("insufficient permissions to access the file resource"))
	}

	return nil
}
