// Package audit provides audit logging for PocketBase record lifecycle events.
// It writes entries to the `audit_logs` collection with field-level diffs,
// delete snapshots, and sensitive-field redaction.
//
// Single-org: the process IS one org, so audit rows carry no org field and no
// org resolution runs. Core registers its own collections here; a feature
// package registers its collections by calling RegisterCollection from its own
// server module's Register(app) (see the contacts package for the reference).
package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
	"tinycld.org/core/readonly"
)

var log = logging.ForPackage("audit")

const auditDroppedMsg = "audit row dropped: the server stayed read-only past the wait"

// LabelExtractor returns a human-readable label for a record.
type LabelExtractor func(record *core.Record) string

// CollectionConfig describes how to audit a single collection.
type CollectionConfig struct {
	// ExtractLabel returns a display label for the record (e.g. contact name,
	// file name). If nil, the default extractor tries common fields (name,
	// title, label, address).
	ExtractLabel LabelExtractor
}

// Fields that should never appear in diffs.
var redactedFields = map[string]bool{
	"password":        true,
	"passwordConfirm": true,
	"tokenKey":        true,
	"keys":            true,
}

// System fields to skip in diffs.
var systemFields = map[string]bool{
	"id":      true,
	"created": true,
	"updated": true,
}

// RegisterCollection registers audit hooks for a single collection. Call this
// from a package's Register() function to add that package's collections to the
// audit log. Pass nil for config to use default org resolution and label extraction.
func RegisterCollection(app *pocketbase.PocketBase, collectionName string, config *CollectionConfig) {
	if config == nil {
		config = &CollectionConfig{}
	}

	extractLabel := config.ExtractLabel
	if extractLabel == nil {
		extractLabel = DefaultLabelExtractor
	}

	app.OnRecordCreateRequest(collectionName).BindFunc(func(e *core.RecordRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		record, info := e.Record, requestInfoOf(e.RequestEvent)
		logWhenWritable(app, collectionName, record.Id, func() {
			logCreate(app, record, info, collectionName, extractLabel)
		})
		return nil
	})

	app.OnRecordUpdateRequest(collectionName).BindFunc(func(e *core.RecordRequestEvent) error {
		original := e.Record.Original()
		if err := e.Next(); err != nil {
			return err
		}
		record, info := e.Record, requestInfoOf(e.RequestEvent)
		logWhenWritable(app, collectionName, record.Id, func() {
			logUpdate(app, record, original, info, collectionName, extractLabel)
		})
		return nil
	})

	app.OnRecordDeleteRequest(collectionName).BindFunc(func(e *core.RecordRequestEvent) error {
		snapshot := BuildSnapshot(e.Record)
		recordID := e.Record.Id
		label := extractLabel(e.Record)
		if err := e.Next(); err != nil {
			return err
		}
		info := requestInfoOf(e.RequestEvent)
		logWhenWritable(app, collectionName, recordID, func() {
			logDelete(app, recordID, label, snapshot, info, collectionName)
		})
		return nil
	})
}

// logWhenWritable writes the audit row of a request already accepted, off
// the request goroutine. The request may have been accepted just before the
// server went read-only; the row then waits for the mode to end, bounded by
// readonly.TailContext (TailWait, or the app's terminate).
func logWhenWritable(app core.App, collectionName, recordID string, write func()) {
	ctx, release := readonly.TailContext(app)
	go func() {
		defer release()
		writeWhenWritable(ctx, collectionName, recordID, write)
	}()
}

// writeWhenWritable logs a dropped row at Error, not Warn: a missing audit
// row is a compliance gap, not a lost courtesy.
func writeWhenWritable(ctx context.Context, collectionName, recordID string, write func()) {
	err := readonly.WhenWritable(ctx, func() error {
		write()
		return nil
	})
	if err != nil {
		log.Error(auditDroppedMsg, "collection", collectionName, "recordID", recordID, "err", err)
	}
}

// RegisterCollections is a convenience for registering multiple collections
// that all share the same config.
func RegisterCollections(app *pocketbase.PocketBase, names []string, config *CollectionConfig) {
	for _, name := range names {
		RegisterCollection(app, name, config)
	}
}

func logCreate(app core.App, record *core.Record, info requestInfo, collectionName string, extractLabel LabelExtractor) {
	auditRecord := newAuditRecord(app, "created", collectionName, record.Id, extractLabel(record))
	if auditRecord == nil {
		return
	}
	info.apply(auditRecord)

	if err := app.Save(auditRecord); err != nil {
		log.Error("failed to save audit log", "collection", collectionName, "recordID", record.Id, "err", err)
	}
}

func logUpdate(app core.App, record *core.Record, original *core.Record, info requestInfo, collectionName string, extractLabel LabelExtractor) {
	auditRecord := newAuditRecord(app, "updated", collectionName, record.Id, extractLabel(record))
	if auditRecord == nil {
		return
	}
	info.apply(auditRecord)

	if original != nil {
		diff := ComputeDiff(original, record)
		if len(diff) > 0 {
			auditRecord.Set("changes", diff)
		}
	}

	if err := app.Save(auditRecord); err != nil {
		log.Error("failed to save audit log", "collection", collectionName, "recordID", record.Id, "err", err)
	}
}

func logDelete(app core.App, recordID string, label string, snapshot map[string]any, info requestInfo, collectionName string) {
	auditRecord := newAuditRecord(app, "deleted", collectionName, recordID, label)
	if auditRecord == nil {
		return
	}
	auditRecord.Set("snapshot", snapshot)
	info.apply(auditRecord)

	if err := app.Save(auditRecord); err != nil {
		log.Error("failed to save audit log", "collection", collectionName, "recordID", recordID, "err", err)
	}
}

func newAuditRecord(app core.App, action string, resourceType string, resourceID string, label string) *core.Record {
	auditCollection, err := app.FindCollectionByNameOrId("audit_logs")
	if err != nil {
		log.Error("could not find audit_logs collection", "err", err)
		return nil
	}

	r := core.NewRecord(auditCollection)
	r.Set("action", action)
	r.Set("resource_type", resourceType)
	r.Set("resource_id", resourceID)
	r.Set("resource_label", label)
	return r
}

func setRequestInfo(auditRecord *core.Record, re *core.RequestEvent) {
	requestInfoOf(re).apply(auditRecord)
}

// requestInfo is what an audit row records of the request behind it. It is
// read in the request hook, so a tail that waits out read-only mode does not
// hold on to the request.
type requestInfo struct {
	system    bool
	actor     string
	ip        string
	userAgent string
}

func requestInfoOf(re *core.RequestEvent) requestInfo {
	if re == nil {
		return requestInfo{system: true}
	}
	info := requestInfo{ip: re.RealIP(), userAgent: re.Request.UserAgent()}
	if re.Auth != nil && re.Auth.Collection().Name == "users" {
		info.actor = re.Auth.Id
	}
	return info
}

func (info requestInfo) apply(auditRecord *core.Record) {
	if info.system {
		auditRecord.Set("metadata", map[string]any{"source": "system"})
		return
	}
	if info.actor != "" {
		auditRecord.Set("actor", info.actor)
	}
	auditRecord.Set("ip_address", info.ip)
	auditRecord.Set("user_agent", info.userAgent)
}

// --- Label extractors ---

// DefaultLabelExtractor tries common display fields: name, title, label, address.
func DefaultLabelExtractor(record *core.Record) string {
	for _, field := range []string{"name", "title", "label", "address"} {
		if v := record.GetString(field); v != "" {
			return v
		}
	}
	return ""
}

// LabelFromField returns a LabelExtractor that reads a single field.
func LabelFromField(fieldName string) LabelExtractor {
	return func(record *core.Record) string {
		return record.GetString(fieldName)
	}
}

// LabelFromFields returns a LabelExtractor that joins multiple fields with ":".
func LabelFromFields(fieldNames ...string) LabelExtractor {
	return func(record *core.Record) string {
		parts := make([]string, 0, len(fieldNames))
		for _, f := range fieldNames {
			if v := record.GetString(f); v != "" {
				parts = append(parts, v)
			}
		}
		return strings.Join(parts, ":")
	}
}

// --- Diff / Snapshot utilities ---

// ComputeDiff compares original vs current record fields and returns changed fields.
func ComputeDiff(original *core.Record, current *core.Record) map[string]any {
	if original == nil {
		return nil
	}

	diff := map[string]any{}

	for _, field := range current.Collection().Fields {
		fieldName := field.GetName()
		if systemFields[fieldName] {
			continue
		}
		if redactedFields[fieldName] {
			oldVal := original.Get(fieldName)
			newVal := current.Get(fieldName)
			if fieldToString(oldVal) != fieldToString(newVal) {
				diff[fieldName] = map[string]any{"redacted": true}
			}
			continue
		}

		oldVal := original.Get(fieldName)
		newVal := current.Get(fieldName)

		if fieldToString(oldVal) != fieldToString(newVal) {
			diff[fieldName] = map[string]any{
				"before": oldVal,
				"after":  newVal,
			}
		}
	}

	return diff
}

// BuildSnapshot captures all non-system, non-redacted fields for delete events.
func BuildSnapshot(record *core.Record) map[string]any {
	snapshot := map[string]any{}

	for _, field := range record.Collection().Fields {
		fieldName := field.GetName()
		if systemFields[fieldName] {
			continue
		}
		if redactedFields[fieldName] {
			snapshot[fieldName] = "[redacted]"
			continue
		}
		snapshot[fieldName] = record.Get(fieldName)
	}

	return snapshot
}

func fieldToString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}
