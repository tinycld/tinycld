package carddav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/davauth"
	"tinycld.org/core/davcond"
	"tinycld.org/core/pkgaccess"
)

// requirePkgWrite refuses the write when the caller's org_pkg_access level
// for this source's package is not full. CardDAV bypasses the REST layer
// (where core's request-hook guard lives), so a readonly user's address-book
// client could otherwise still create, edit, and delete contacts. Reads are
// untouched: readonly means read.
func (b *Backend) requirePkgWrite(user *core.Record, src Source) error {
	if err := pkgaccess.WriteError(b.app, user, src.Slug); err != nil {
		return webdav.NewHTTPError(http.StatusForbidden, err)
	}
	return nil
}

type contextKey string

const httpRequestKey contextKey = "httpRequest"

// Backend is a generic carddav.Backend serving one or more feature collections
// as vCards, scoped per org. Each org the authenticated user belongs to is an
// address book; the objects come from the Source's collection.
//
// For the pilot exactly one Source (contacts) is registered. The Source is
// selected per request; when multiple exist the path's book segment could encode
// the source, but today the single Source is used.
type Backend struct {
	app     core.App
	sources []Source
	scope   OrgScope
}

// source returns the Source backing address books. With one registered Source
// (the pilot), that's it; this is the single seam to extend when more feature
// collections register CardDAV books.
func (b *Backend) source() (Source, bool) {
	if len(b.sources) == 0 {
		return Source{}, false
	}
	return b.sources[0], true
}

func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	if _, err := b.authFromContext(ctx); err != nil {
		return "", err
	}
	return "/carddav/u/", nil
}

func (b *Backend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	if _, err := b.authFromContext(ctx); err != nil {
		return "", err
	}
	return "/carddav/u/ab/", nil
}

func (b *Backend) ListAddressBooks(ctx context.Context) ([]carddav.AddressBook, error) {
	user, err := b.authFromContext(ctx)
	if err != nil {
		return nil, err
	}

	scoped, err := b.scope.Books(b.app, user)
	if err != nil {
		return nil, err
	}
	books := make([]carddav.AddressBook, len(scoped))
	for i, bk := range scoped {
		books[i] = carddav.AddressBook{Path: bk.Path, Name: bk.Name, Description: bk.Description}
	}
	return books, nil
}

func (b *Backend) GetAddressBook(ctx context.Context, path string) (*carddav.AddressBook, error) {
	books, err := b.ListAddressBooks(ctx)
	if err != nil {
		return nil, err
	}
	for _, book := range books {
		if book.Path == path {
			return &book, nil
		}
	}
	return nil, fmt.Errorf("address book not found")
}

func (b *Backend) CreateAddressBook(_ context.Context, _ *carddav.AddressBook) error {
	return fmt.Errorf("creating address books is not supported")
}

func (b *Backend) DeleteAddressBook(_ context.Context, _ string) error {
	return fmt.Errorf("deleting address books is not supported")
}

func (b *Backend) ListAddressObjects(ctx context.Context, path string, req *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	src, ok := b.source()
	if !ok {
		return nil, fmt.Errorf("no carddav source registered")
	}
	user, err := b.authFromContext(ctx)
	if err != nil {
		return nil, err
	}
	ownerID, bookPath, err := b.scope.ResolveBook(b.app, user)
	if err != nil {
		return nil, err
	}

	records, err := b.app.FindRecordsByFilter(src.Collection, src.ListFilter, src.Sort, 0, 0,
		map[string]any{"ownerId": ownerID})
	if err != nil {
		return nil, err
	}

	objects := make([]carddav.AddressObject, 0, len(records))
	for _, record := range records {
		// Backfill uid for records created before the autogen hook existed.
		if record.GetString(src.UIDField) == "" {
			record.Set(src.UIDField, "urn:uuid:"+uuid.NewString())
			_ = b.app.Save(record)
		}
		obj, err := b.recordToAddressObject(src, record, bookPath, req)
		if err != nil {
			continue
		}
		objects = append(objects, *obj)
	}

	return objects, nil
}

func (b *Backend) GetAddressObject(ctx context.Context, path string, req *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	src, ok := b.source()
	if !ok {
		return nil, fmt.Errorf("no carddav source registered")
	}
	record, bookPath, err := b.resolveObjectByPath(ctx, src, path)
	if err != nil {
		return nil, err
	}
	return b.recordToAddressObject(src, record, bookPath, req)
}

func (b *Backend) QueryAddressObjects(ctx context.Context, path string, query *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	return b.ListAddressObjects(ctx, path, &query.DataRequest)
}

func (b *Backend) PutAddressObject(ctx context.Context, path string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	src, ok := b.source()
	if !ok {
		return nil, fmt.Errorf("no carddav source registered")
	}
	user, err := b.authFromContext(ctx)
	if err != nil {
		return nil, err
	}

	uid := card.Value(vcard.FieldUID)

	if err := b.requirePkgWrite(user, src); err != nil {
		return nil, err
	}

	existing, bookPath, _ := b.resolveObjectByPath(ctx, src, path)

	if opts != nil {
		// The stored ETag is the record's `updated` stamp (see
		// recordToAddressObject); enforce the client's conditional headers
		// against it or a concurrent edit silently loses to last-writer-wins.
		etag := ""
		if existing != nil {
			etag = existing.GetString("updated")
		}
		if err := davcond.Check(opts.IfNoneMatch, opts.IfMatch, etag, existing != nil); err != nil {
			return nil, err
		}
	}

	if existing != nil {
		ApplyVCardToRecord(card, existing, src.VCard)
		if err := b.saveAuthorized(user, existing, ruleUpdate); err != nil {
			return nil, err
		}
		return b.recordToAddressObject(src, existing, bookPath, nil)
	}

	ownerID, newBookPath, err := b.scope.ResolveBook(b.app, user)
	if err != nil {
		return nil, fmt.Errorf("cannot find org membership: %w", err)
	}

	collection, err := b.app.FindCollectionByNameOrId(src.Collection)
	if err != nil {
		return nil, err
	}

	record := core.NewRecord(collection)
	if uid == "" {
		uid = "urn:uuid:" + uuid.NewString()
	}
	record.Set(src.UIDField, uid)
	record.Set(src.OwnerField, ownerID)
	ApplyVCardToRecord(card, record, src.VCard)

	if err := b.saveAuthorized(user, record, ruleCreate); err != nil {
		return nil, err
	}

	return b.recordToAddressObject(src, record, newBookPath, nil)
}

// ruleKind selects which of a collection's access rules a write is judged by.
type ruleKind int

const (
	ruleCreate ruleKind = iota
	ruleUpdate
)

// errDenied is returned when a collection rule refuses the write. Distinct
// from a validation or storage failure so callers can tell "you may not" from
// "it did not work".
var errDenied = errors.New("carddav: write denied by collection rule")

// saveAuthorized persists a record only if the collection's own rule admits it.
//
// CardDAV does not go through PocketBase's REST layer, so no rule is applied
// for us. Reimplementing the check in Go would mean two definitions of the same
// permission, free to drift — and drift is not hypothetical here: the write
// path previously evaluated nothing at all and was correct only because the
// collection happened to be owner-scoped. Ask the rule engine the same question
// the REST API would.
//
// The save happens inside a transaction and rolls back on refusal, because
// CanAccessRecord evaluates a rule as a query filtered to `id = record.Id`: an
// unsaved record matches nothing, so every create rule would deny. This is what
// PocketBase's own record-create API does, and what core/webdav and core/caldav
// both do.
func (b *Backend) saveAuthorized(user *core.Record, record *core.Record, kind ruleKind) error {
	method := http.MethodPost
	if kind == ruleUpdate {
		method = http.MethodPatch
	}

	err := b.app.RunInTransaction(func(txApp core.App) error {
		if err := txApp.Save(record); err != nil {
			return err
		}

		var rule *string
		if kind == ruleCreate {
			rule = record.Collection().CreateRule
		} else {
			rule = record.Collection().UpdateRule
		}

		ok, err := txApp.CanAccessRecord(record, &core.RequestInfo{
			Auth:    user,
			Method:  method,
			Context: core.RequestInfoContextDefault,
		}, rule)
		if err != nil {
			// An unevaluable rule must not fail open.
			b.app.Logger().Warn("carddav: rule evaluation failed",
				"id", record.Id, "error", err)
			return errDenied
		}
		if !ok {
			return errDenied
		}
		return nil
	})

	if errors.Is(err, errDenied) {
		return errDenied
	}
	return err
}

func (b *Backend) DeleteAddressObject(ctx context.Context, path string) error {
	src, ok := b.source()
	if !ok {
		return fmt.Errorf("no carddav source registered")
	}
	user, err := b.authFromContext(ctx)
	if err != nil {
		return err
	}
	if err := b.requirePkgWrite(user, src); err != nil {
		return err
	}
	record, _, err := b.resolveObjectByPath(ctx, src, path)
	if err != nil {
		return err
	}
	// Soft-delete (when configured) keeps a CardDAV DELETE consistent with the
	// web UI so the record stays restorable instead of being lost.
	if src.SoftDeleteField != "" {
		record.Set(src.SoftDeleteField, types.NowDateTime())
		return b.app.Save(record)
	}
	return b.app.Delete(record)
}

func (b *Backend) authFromContext(ctx context.Context) (*core.Record, error) {
	r, ok := ctx.Value(httpRequestKey).(*http.Request)
	if !ok {
		return nil, davauth.ErrUnauthorized
	}
	// The route wrapper already authenticated and settled the per-request
	// cache (and recorded the failure/success for rate limiting), so this
	// resolves from the cache without another bcrypt. It stays fail-closed as
	// a backstop for any caller that reaches the backend without the wrapper.
	return davauth.Authenticate(b.app, r)
}

func (b *Backend) recordToAddressObject(src Source, record *core.Record, bookPath string, _ *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	card := RecordToVCard(record, src.VCard)

	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return nil, err
	}

	modTime := time.Time{}
	if updated := record.GetString("updated"); updated != "" {
		if t, err := time.Parse(pbTimeLayout, updated); err == nil {
			modTime = t
		}
	}

	uid := record.GetString(src.UIDField)
	return &carddav.AddressObject{
		Path:          bookPath + uid + ".vcf",
		ModTime:       modTime,
		ContentLength: int64(buf.Len()),
		ETag:          fmt.Sprintf(`"%s"`, record.GetString("updated")),
		Card:          card,
	}, nil
}

func (b *Backend) resolveObjectByPath(ctx context.Context, src Source, path string) (*core.Record, string, error) {
	user, err := b.authFromContext(ctx)
	if err != nil {
		return nil, "", err
	}

	uid := extractVCardUID(path)
	if uid == "" {
		return nil, "", fmt.Errorf("invalid object path")
	}

	ownerID, bookPath, err := b.scope.ResolveBook(b.app, user)
	if err != nil {
		return nil, "", err
	}

	records, err := b.app.FindRecordsByFilter(src.Collection,
		src.UIDField+" = {:uid} && "+src.OwnerField+" = {:ownerId}", "", 1, 0,
		map[string]any{"uid": uid, "ownerId": ownerID})
	if err != nil || len(records) == 0 {
		return nil, "", fmt.Errorf("object not found")
	}

	return records[0], bookPath, nil
}

// extractVCardUID gets the vCard UID from /carddav/u/ab/{book}/{uid}.vcf
func extractVCardUID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 5 {
		return strings.TrimSuffix(parts[4], ".vcf")
	}
	return ""
}
