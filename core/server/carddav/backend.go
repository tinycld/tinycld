package carddav

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

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
	ownerID, bookPath, err := b.scope.ResolveBook(b.app, user, b.orgSlugFromContext(ctx, path))
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

func (b *Backend) PutAddressObject(ctx context.Context, path string, card vcard.Card, _ *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	src, ok := b.source()
	if !ok {
		return nil, fmt.Errorf("no carddav source registered")
	}
	user, err := b.authFromContext(ctx)
	if err != nil {
		return nil, err
	}

	uid := card.Value(vcard.FieldUID)

	existing, bookPath, _ := b.resolveObjectByPath(ctx, src, path)
	if existing != nil {
		applyVCardToRecord(card, existing, src.VCard)
		if err := b.app.Save(existing); err != nil {
			return nil, err
		}
		return b.recordToAddressObject(src, existing, bookPath, nil)
	}

	ownerID, newBookPath, err := b.scope.ResolveBook(b.app, user, b.orgSlugFromContext(ctx, path))
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
	applyVCardToRecord(card, record, src.VCard)

	if err := b.app.Save(record); err != nil {
		return nil, err
	}

	return b.recordToAddressObject(src, record, newBookPath, nil)
}

func (b *Backend) DeleteAddressObject(ctx context.Context, path string) error {
	src, ok := b.source()
	if !ok {
		return fmt.Errorf("no carddav source registered")
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
		return nil, errUnauthorized
	}
	return authenticateRequest(b.app, r)
}

func (b *Backend) recordToAddressObject(src Source, record *core.Record, bookPath string, _ *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	card := recordToVCard(record, src.VCard)

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

	ownerID, bookPath, err := b.scope.ResolveBook(b.app, user, b.orgSlugFromContext(ctx, path))
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

// orgSlugFromContext prefers the Host-header subdomain, then falls back to path.
// Used only by sharedDBScope; singleOrgScope ignores the returned hint.
func (b *Backend) orgSlugFromContext(ctx context.Context, path string) string {
	if r, ok := ctx.Value(httpRequestKey).(*http.Request); ok {
		if slug := extractOrgSlugFromHost(r.Host); slug != "" {
			return slug
		}
	}
	return extractOrgSlug(path)
}

// extractOrgSlug gets the org slug from /carddav/u/ab/{orgSlug}/...
func extractOrgSlug(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

// extractOrgSlugFromHost parses the subdomain from a Host header.
// "acme.localhost:8100" → "acme", "acme.tinycld.com" → "acme". Returns "" for IP
// addresses, bare hostnames, and two-label domains.
func extractOrgSlugFromHost(host string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	if strings.HasSuffix(host, ".localhost") {
		return strings.TrimSuffix(host, ".localhost")
	}
	if isIPv4(host) {
		return ""
	}
	parts := strings.Split(host, ".")
	if len(parts) >= 3 {
		return parts[0]
	}
	return ""
}

func isIPv4(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// extractVCardUID gets the vCard UID from /carddav/u/ab/{orgSlug}/{uid}.vcf
func extractVCardUID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 5 {
		return strings.TrimSuffix(parts[4], ".vcf")
	}
	return ""
}
