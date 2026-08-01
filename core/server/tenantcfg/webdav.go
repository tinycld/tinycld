package tenantcfg

import (
	"tinycld.org/core/quota"
	"tinycld.org/core/webdav"
)

// The WebDAV wire format, mirroring tinycld.org/core/webdav's Source/FieldMap
// for the same reason the CardDAV mirror exists: core's types carry no JSON
// tags, so marshalling them directly would make the wire format depend on Go
// field names staying stable across a repo boundary.
//
// Note what is NOT carried: webdav.Source.Hooks. Those are Go callbacks and
// cannot cross a process boundary. Under the single-Register contract this no
// longer matters: feature Go self-mounts its sources in tenants too, closures
// riding along directly — these materialized shapes are consumed only by
// OLDER artifact binaries, whose cores still mount from config.

type WebDAVSource struct {
	Slug       string         `json:"slug"`
	Prefix     string         `json:"prefix"`
	Collection string         `json:"collection"`
	Fields     WebDAVFieldMap `json:"fields"`
	Trash      *WebDAVTrash   `json:"trash,omitempty"`
}

type WebDAVFieldMap struct {
	Name     string `json:"name"`
	Parent   string `json:"parent"`
	IsFolder string `json:"isFolder"`
	Size     string `json:"size"`
	MimeType string `json:"mimeType"`
	File     string `json:"file"`
	Owner    string `json:"owner"`
	Updated  string `json:"updated"`
}

// WebDAVTrash mirrors webdav.TrashConfig: the per-user soft-delete state a
// DAV DELETE stamps instead of destroying the record.
type WebDAVTrash struct {
	Collection     string `json:"collection"`
	ItemField      string `json:"itemField"`
	UserField      string `json:"userField"`
	TrashedAtField string `json:"trashedAtField"`
}

// EncodeWebDAV converts core's Sources into the wire form.
func EncodeWebDAV(sources []webdav.Source) []WebDAVSource {
	out := make([]WebDAVSource, 0, len(sources))
	for _, s := range sources {
		ws := WebDAVSource{
			Slug:       s.Slug,
			Prefix:     s.Prefix,
			Collection: s.Collection,
			Fields: WebDAVFieldMap{
				Name:     s.Fields.Name,
				Parent:   s.Fields.Parent,
				IsFolder: s.Fields.IsFolder,
				Size:     s.Fields.Size,
				MimeType: s.Fields.MimeType,
				File:     s.Fields.File,
				Owner:    s.Fields.Owner,
				Updated:  s.Fields.Updated,
			},
		}
		if s.Trash != nil {
			ws.Trash = &WebDAVTrash{
				Collection:     s.Trash.Collection,
				ItemField:      s.Trash.ItemField,
				UserField:      s.Trash.UserField,
				TrashedAtField: s.Trash.TrashedAtField,
			}
		}
		out = append(out, ws)
	}
	return out
}

// DecodeWebDAV converts the wire form back into core's Sources.
func DecodeWebDAV(sources []WebDAVSource) []webdav.Source {
	out := make([]webdav.Source, 0, len(sources))
	for _, s := range sources {
		src := webdav.Source{
			Slug:       s.Slug,
			Prefix:     s.Prefix,
			Collection: s.Collection,
			Fields: webdav.FieldMap{
				Name:     s.Fields.Name,
				Parent:   s.Fields.Parent,
				IsFolder: s.Fields.IsFolder,
				Size:     s.Fields.Size,
				MimeType: s.Fields.MimeType,
				File:     s.Fields.File,
				Owner:    s.Fields.Owner,
				Updated:  s.Fields.Updated,
			},
		}
		if s.Trash != nil {
			src.Trash = &webdav.TrashConfig{
				Collection:     s.Trash.Collection,
				ItemField:      s.Trash.ItemField,
				UserField:      s.Trash.UserField,
				TrashedAtField: s.Trash.TrashedAtField,
			}
		}
		out = append(out, src)
	}
	return out
}

// The quota wire format. Mirrored for the same reason as the DAV ones:
// tinycld.org/core/quota's Source carries no JSON tags, so marshalling it
// directly would make this file's shape depend on Go field names staying stable
// across a repo boundary.
type QuotaSource struct {
	Slug       string `json:"slug"`
	Collection string `json:"collection"`
	SizeField  string `json:"sizeField"`
	OwnerField string `json:"ownerField"`
}

// EncodeQuota converts core's Sources into the wire form.
func EncodeQuota(sources []quota.Source) []QuotaSource {
	out := make([]QuotaSource, 0, len(sources))
	for _, s := range sources {
		out = append(out, QuotaSource{
			Slug:       s.Slug,
			Collection: s.Collection,
			SizeField:  s.SizeField,
			OwnerField: s.OwnerField,
		})
	}
	return out
}

// DecodeQuota converts the wire form back into core's Sources.
func DecodeQuota(sources []QuotaSource) []quota.Source {
	out := make([]quota.Source, 0, len(sources))
	for _, s := range sources {
		out = append(out, quota.Source{
			Slug:       s.Slug,
			Collection: s.Collection,
			SizeField:  s.SizeField,
			OwnerField: s.OwnerField,
		})
	}
	return out
}
