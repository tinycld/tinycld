package webdav

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/net/webdav"
)

type contextKey string

// userKey is the context key under which the request middleware stashes the
// authenticated *core.Record. FileSystem methods read it via userFromContext —
// they never re-authenticate.
//
// Auth runs ONCE per request, in the middleware, rather than per FileSystem
// call. A single PROPFIND drives many calls, and bcrypt-verifying each one
// would be a real regression. (CardDAV re-authenticates per backend call; its
// call volume per request is far lower.)
const userKey contextKey = "tinycld.webdav.user"

// FileSystem implements webdav.FileSystem backed by a PocketBase collection
// shaped as a tree, per its Source config. The paths it serves are rooted at
// Source.Prefix; the root itself is a synthetic directory.
type FileSystem struct {
	app core.App
	src Source
	ts  TSHooks
}

var _ webdav.FileSystem = (*FileSystem)(nil)

// NewFileSystem builds a FileSystem for one Source, validating the config.
//
// Validation is not defensive politeness: Source field and collection names are
// interpolated into the recursive-CTE SQL (dbx cannot bind an identifier), so a
// malformed name would be an injection vector. Rejecting at construction keeps
// the query builder free of per-call checks.
func NewFileSystem(app core.App, src Source) (*FileSystem, error) {
	if err := validateSource(src); err != nil {
		return nil, err
	}
	return &FileSystem{app: app, src: src}, nil
}

func validateSource(src Source) error {
	if src.Prefix == "" || src.Prefix[0] != '/' {
		return fmt.Errorf("webdav: Source.Prefix must be an absolute path, got %q", src.Prefix)
	}
	if src.Prefix[len(src.Prefix)-1] == '/' {
		return fmt.Errorf("webdav: Source.Prefix must not end in a slash, got %q", src.Prefix)
	}

	idents := map[string]string{
		"Collection":      src.Collection,
		"Fields.Name":     src.Fields.Name,
		"Fields.Parent":   src.Fields.Parent,
		"Fields.IsFolder": src.Fields.IsFolder,
		"Fields.Size":     src.Fields.Size,
		"Fields.File":     src.Fields.File,
		"Fields.Owner":    src.Fields.Owner,
	}
	for label, v := range idents {
		if v == "" {
			return fmt.Errorf("webdav: Source.%s is required", label)
		}
		if !safeIdent.MatchString(v) {
			return fmt.Errorf("webdav: Source.%s = %q is not a valid SQL identifier", label, v)
		}
	}

	// Optional identifiers still have to be safe when set.
	for label, v := range map[string]string{
		"Fields.MimeType": src.Fields.MimeType,
		"Fields.Updated":  src.Fields.Updated,
	} {
		if v != "" && !safeIdent.MatchString(v) {
			return fmt.Errorf("webdav: Source.%s = %q is not a valid SQL identifier", label, v)
		}
	}

	return nil
}

// userFromContext returns the user the auth middleware attached to the request.
// Reaching a FileSystem method without one is a programming error — the
// middleware always enforces auth before dispatching to the handler.
func (fs *FileSystem) userFromContext(ctx context.Context) (*core.Record, error) {
	user, ok := ctx.Value(userKey).(*core.Record)
	if !ok || user == nil {
		return nil, errors.New("webdav: missing authenticated user in context")
	}
	return user, nil
}

// resolveContext authenticates and parses the path into segments. Callers
// handle the root case (no segments) themselves.
func (fs *FileSystem) resolveContext(ctx context.Context, name string) (*core.Record, []string, error) {
	user, err := fs.userFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	return user, parsePath(fs.src.Prefix, name), nil
}

// ruleKind selects which of a collection's access rules to evaluate.
type ruleKind int

const (
	ruleList ruleKind = iota
	ruleView
	ruleCreate
	ruleUpdate
	ruleDelete
)

// rule returns the collection's rule for the given operation. A nil *string
// means "superusers only" and an empty string means "public" — CanAccessRecord
// gives both their PocketBase meaning, so they are passed through untouched.
func ruleFor(collection *core.Collection, kind ruleKind) *string {
	switch kind {
	case ruleList:
		return collection.ListRule
	case ruleView:
		return collection.ViewRule
	case ruleCreate:
		return collection.CreateRule
	case ruleUpdate:
		return collection.UpdateRule
	case ruleDelete:
		return collection.DeleteRule
	}
	return nil
}

// authorize evaluates the record's own collection rule for this user.
//
// WebDAV does not go through PocketBase's REST layer, so no rule is applied for
// us — but reimplementing the check in Go would mean two definitions of the same
// permission, free to drift. Instead we ask the rule engine the same question
// the REST API would, with the DAV request's authenticated user.
//
// The listing path uses ListRule and single-record resolution uses ViewRule,
// mirroring how PocketBase itself splits them.
func (fs *FileSystem) authorize(user *core.Record, record *core.Record, kind ruleKind) error {
	if record == nil {
		return nil
	}

	info := &core.RequestInfo{
		Auth:    user,
		Method:  "GET",
		Context: core.RequestInfoContextDefault,
	}

	ok, err := fs.app.CanAccessRecord(record, info, ruleFor(record.Collection(), kind))
	if err != nil {
		// An unevaluable rule must not fail open.
		fs.app.Logger().Warn("webdav: access rule evaluation failed",
			"slug", fs.src.Slug, "id", record.Id, "error", err)
		return os.ErrPermission
	}
	if !ok {
		return os.ErrPermission
	}
	return nil
}

// canRead reports whether the user may resolve this record by path.
func (fs *FileSystem) canRead(user *core.Record, record *core.Record) error {
	return fs.authorize(user, record, ruleView)
}

// errConflict is the namespace collision reported when a create or move lands
// on a name that is already taken by a record the caller cannot see.
//
// It is deliberately NOT os.ErrExist. The (parent, name) namespace is globally
// unique, so the write genuinely cannot proceed — but ErrExist is the handler's
// signal for "this path is occupied", which tells the caller that a specific
// name exists in a tree they cannot read. Probing names would then map another
// user's whole directory. A generic conflict refuses the write without
// answering the question.
//
// x/net/webdav maps an unrecognized error to 500. That is a worse status than
// the 405 ErrExist would produce and a fair trade for not leaking, but a caller
// wanting a nicer surface should translate this at the handler.
var errConflict = errors.New("webdav: name is unavailable")

// authorizeOrMask evaluates a rule and converts a refusal into "not found".
//
// Read verbs already did this (see Stat and openForRead) so an entry the caller
// cannot see is invisible rather than forbidden. The write verbs did not: a 403
// on DELETE/MOVE confirms the path exists just as loudly as a 200 would, which
// makes the read-side masking pointless — a client just probes with a verb that
// still answers honestly.
func (fs *FileSystem) authorizeOrMask(user *core.Record, record *core.Record, kind ruleKind) error {
	if err := fs.authorize(user, record, kind); err != nil {
		return os.ErrNotExist
	}
	return nil
}

// nameIsTaken reports whether a path resolves to a record, and whether the
// caller may see it. A create or move onto a taken name must fail either way;
// only the error differs, and only so the refusal reveals nothing.
func (fs *FileSystem) nameIsTaken(user *core.Record, segments []string) error {
	existing, _ := fs.resolveByPath(segments)
	if existing == nil {
		return nil
	}
	if fs.canRead(user, existing) != nil {
		return errConflict
	}
	return os.ErrExist
}

// saveAuthorizedCreate persists a new record only if the collection's
// CreateRule admits it.
//
// CanAccessRecord evaluates a rule as a query filtered to `id = record.Id`, so
// an unsaved record matches nothing and every create rule would deny — and a
// create rule that traverses a relation could not be evaluated any other way.
// So save inside a transaction, ask the rule, and roll back if it says no.
// This is what PocketBase's own record-create API does, and what CalDAV's
// PutCalendarObject already did.
//
// Without this, PUT-of-new-file and MKCOL evaluate no rule at all — and
// CreateRule is the only place a package can put a clause that applies to
// creates alone, which is where drive's guest exclusion and the disabled-user
// clause both live.
// persist does the actual save on the transaction's handle; it is a parameter
// because a file create must attach its blob in the same transaction that
// authorizes it, while a folder create is a bare save.
func (fs *FileSystem) saveAuthorizedCreate(user *core.Record, record *core.Record, persist func(core.App) error) error {
	denied := errors.New("webdav: create denied by rule")

	err := fs.app.RunInTransaction(func(txApp core.App) error {
		if err := persist(txApp); err != nil {
			return err
		}

		info := &core.RequestInfo{
			Auth:    user,
			Method:  "POST",
			Context: core.RequestInfoContextDefault,
		}
		ok, err := txApp.CanAccessRecord(record, info, ruleFor(record.Collection(), ruleCreate))
		if err != nil {
			// An unevaluable rule must not fail open.
			fs.app.Logger().Warn("webdav: create rule evaluation failed",
				"slug", fs.src.Slug, "id", record.Id, "error", err)
			return denied
		}
		if !ok {
			return denied
		}
		return nil
	})

	if errors.Is(err, denied) {
		return os.ErrPermission
	}
	return err
}

// Stat resolves the path to either the synthetic root or a backing record.
func (fs *FileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	user, segments, err := fs.resolveContext(ctx, name)
	if err != nil {
		return nil, err
	}

	if len(segments) == 0 {
		return &fileInfo{name: fs.rootName(), isDir: true}, nil
	}

	record, err := fs.resolveByPath(segments)
	if err != nil {
		return nil, err
	}

	// An entry the viewer may not read must be invisible, not forbidden:
	// answering not-found avoids confirming that the path exists.
	if err := fs.canRead(user, record); err != nil {
		return nil, os.ErrNotExist
	}
	if err := fs.maskTrashed(user, record); err != nil {
		return nil, err
	}

	return fs.recordToFileInfo(record), nil
}

// OpenFile dispatches to the read or write path based on flag bits.
func (fs *FileSystem) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		return fs.openForWrite(ctx, name, flag)
	}
	return fs.openForRead(ctx, name)
}

// openForRead handles GET, HEAD and the read side of COPY. Opening a file
// returns a davFile with rdSeeker primed; opening a directory (root or folder)
// returns one with children pre-populated for Readdir.
func (fs *FileSystem) openForRead(ctx context.Context, name string) (webdav.File, error) {
	user, segments, err := fs.resolveContext(ctx, name)
	if err != nil {
		return nil, err
	}

	if len(segments) == 0 {
		children, err := fs.listChildren(ctx, "", user)
		if err != nil {
			return nil, err
		}
		return &davFile{
			info:     &fileInfo{name: fs.rootName(), isDir: true},
			children: children,
		}, nil
	}

	record, err := fs.resolveByPath(segments)
	if err != nil {
		return nil, err
	}

	// Same not-found masking as Stat.
	if err := fs.canRead(user, record); err != nil {
		return nil, os.ErrNotExist
	}
	if err := fs.maskTrashed(user, record); err != nil {
		return nil, err
	}

	if record.GetBool(fs.src.Fields.IsFolder) {
		children, err := fs.listChildren(ctx, record.Id, user)
		if err != nil {
			return nil, err
		}
		return &davFile{
			info:     fs.recordToFileInfo(record),
			children: children,
		}, nil
	}

	rdr, tmpPath, err := fs.openSeekableBlob(record)
	if err != nil {
		return nil, err
	}
	return &davFile{
		info:     fs.recordToFileInfo(record),
		rdSeeker: rdr,
		tmpPath:  tmpPath,
	}, nil
}

// openForWrite handles PUT and the write side of COPY. The temp file is created
// eagerly so the handler's io.Copy works as soon as this returns; persistence
// happens on Close, which is when the final size is known — hence quota and
// write-permission checks are deferred to there too.
func (fs *FileSystem) openForWrite(ctx context.Context, name string, flag int) (webdav.File, error) {
	user, segments, err := fs.resolveContext(ctx, name)
	if err != nil {
		return nil, err
	}

	if err := fs.requirePkgWrite(user); err != nil {
		return nil, err
	}

	if len(segments) == 0 {
		return nil, os.ErrPermission
	}

	parentID, itemName, err := fs.resolveParentByPath(user, segments)
	if err != nil {
		return nil, err
	}

	existing, _ := fs.resolveByPath(segments)
	// An entry the caller cannot see must not be treated as an overwrite
	// target: the write would be refused later by the update rule, and the
	// difference between that refusal and a clean create is enough to prove
	// the name is taken in a tree they cannot read.
	if existing != nil && fs.canRead(user, existing) != nil {
		return nil, errConflict
	}
	// Nor may an entry in the caller's trash: writing into it would silently
	// resurrect (and overwrite) trashed content. The record still owns the
	// (parent, name) slot, so the honest answer is the same conflict the web
	// UI reports for a taken name.
	if existing != nil && fs.isTrashedFor(user, existing) {
		return nil, errConflict
	}
	if existing != nil && existing.GetBool(fs.src.Fields.IsFolder) {
		return nil, os.ErrPermission
	}

	if existing == nil && flag&os.O_CREATE == 0 {
		return nil, os.ErrNotExist
	}

	if err := fs.hookBeforeWrite(ctx, user, itemName, name, existing == nil); err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp("", "tinycld-dav-*")
	if err != nil {
		return nil, err
	}

	return &davFile{
		fs:       fs,
		info:     &fileInfo{name: itemName},
		wrBuf:    tmp,
		wrName:   itemName,
		parentID: parentID,
		user:     user,
		existing: existing,
	}, nil
}

// Mkdir creates a folder record at the given path. The parent must exist. The
// handler treats os.ErrExist as 405.
func (fs *FileSystem) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	user, segments, err := fs.resolveContext(ctx, name)
	if err != nil {
		return err
	}

	if err := fs.requirePkgWrite(user); err != nil {
		return err
	}

	if len(segments) == 0 {
		return os.ErrPermission
	}

	parentID, folderName, err := fs.resolveParentByPath(user, segments)
	if err != nil {
		return err
	}

	if err := fs.nameIsTaken(user, segments); err != nil {
		return err
	}

	if err := fs.hookBeforeWrite(ctx, user, folderName, name, true); err != nil {
		return err
	}

	collection, err := fs.app.FindCollectionByNameOrId(fs.src.Collection)
	if err != nil {
		return err
	}

	f := fs.src.Fields
	record := core.NewRecord(collection)
	record.Set(f.Name, folderName)
	record.Set(f.IsFolder, true)
	record.Set(f.Parent, parentID)
	record.Set(f.Owner, user.Id)

	return fs.saveAuthorizedCreate(user, record, func(txApp core.App) error {
		return txApp.Save(record)
	})
}

// RemoveAll handles DELETE. With a Trash binding the record is stamped into
// the user's trash — the same write the feature's own trash action performs,
// restorable from its trash screen — and only an unbound source destroys the
// record (PocketBase cascade rules then handle a folder's children).
func (fs *FileSystem) RemoveAll(ctx context.Context, name string) error {
	user, segments, err := fs.resolveContext(ctx, name)
	if err != nil {
		return err
	}

	if err := fs.requirePkgWrite(user); err != nil {
		return err
	}

	if len(segments) == 0 {
		return os.ErrPermission
	}

	record, err := fs.resolveByPath(segments)
	if err != nil {
		return err
	}

	// Already in this user's trash ⇒ gone from their DAV view.
	if err := fs.maskTrashed(user, record); err != nil {
		return err
	}

	// Masked, not forbidden: a 403 here would confirm the path exists to a
	// caller who cannot read it.
	if err := fs.authorizeOrMask(user, record, ruleDelete); err != nil {
		return err
	}

	if err := fs.hookBeforeDelete(ctx, user, record, name); err != nil {
		return err
	}

	if fs.src.Trash != nil {
		return fs.moveToTrash(user, record)
	}
	return fs.app.Delete(record)
}

// Rename implements MOVE. The handler pre-removes an existing destination only
// when Overwrite: T is set; otherwise os.ErrExist must be reported if the
// destination is still present.
func (fs *FileSystem) Rename(ctx context.Context, oldName, newName string) error {
	user, srcSegments, err := fs.resolveContext(ctx, oldName)
	if err != nil {
		return err
	}

	if err := fs.requirePkgWrite(user); err != nil {
		return err
	}

	destSegments := parsePath(fs.src.Prefix, newName)

	if len(srcSegments) == 0 || len(destSegments) == 0 {
		return os.ErrPermission
	}

	srcRecord, err := fs.resolveByPath(srcSegments)
	if err != nil {
		return err
	}

	// An entry in the caller's trash cannot be MOVEd — it is not in their view.
	if err := fs.maskTrashed(user, srcRecord); err != nil {
		return err
	}

	if err := fs.authorizeOrMask(user, srcRecord, ruleUpdate); err != nil {
		return err
	}

	if err := fs.nameIsTaken(user, destSegments); err != nil {
		return err
	}

	destParentID, destName, err := fs.resolveParentByPath(user, destSegments)
	if err != nil {
		return err
	}

	// A MOVE that reparents a folder under its own descendant would create a
	// cycle, which then makes any recursive walk of the tree loop.
	if srcRecord.GetBool(fs.src.Fields.IsFolder) {
		cyclic, err := fs.wouldCreateCycle(srcRecord.Id, destParentID)
		if err != nil {
			return err
		}
		if cyclic {
			return os.ErrInvalid
		}
	}

	if err := fs.hookBeforeMove(ctx, user, srcRecord, oldName, newName); err != nil {
		return err
	}

	f := fs.src.Fields
	srcRecord.Set(f.Name, destName)
	srcRecord.Set(f.Parent, destParentID)

	return fs.app.Save(srcRecord)
}

// listChildren returns the immediate children of a parent (or of the root for
// an empty parentID), limited to entries the viewer may read.
//
// FindRecordsByFilter does not apply the collection's rules, so each candidate
// is put through ListRule — being authenticated must not expose every user's
// files. An entry that fails is silently omitted, which is what a listing
// should do.
func (fs *FileSystem) listChildren(ctx context.Context, parentID string, user *core.Record) ([]os.FileInfo, error) {
	f := fs.src.Fields

	var filter string
	params := map[string]any{}
	if parentID == "" {
		filter = f.Parent + " = ''"
	} else {
		filter = f.Parent + " = {:parent}"
		params["parent"] = parentID
	}

	records, err := fs.app.FindRecordsByFilter(fs.src.Collection, filter, f.Name, 0, 0, params)
	if err != nil {
		return nil, err
	}

	// One query for the caller's whole trash, not one per row.
	trashed, err := fs.trashedIDsFor(user)
	if err != nil {
		return nil, err
	}

	infos := make([]os.FileInfo, 0, len(records))
	for _, r := range records {
		if trashed[r.Id] {
			continue
		}
		if fs.authorize(user, r, ruleList) != nil {
			continue
		}
		infos = append(infos, fs.recordToFileInfo(r))
	}

	return fs.hookFilterList(ctx, user.Id, infos)
}

// rootName is the display name of the synthetic root directory: the mount
// prefix without its leading slash.
func (fs *FileSystem) rootName() string {
	return fs.src.Prefix[1:]
}
