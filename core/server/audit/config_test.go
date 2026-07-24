package audit

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func newRec(t *testing.T, fields map[string]string) *core.Record {
	t.Helper()
	col := core.NewBaseCollection("contacts")
	for name := range fields {
		col.Fields.Add(&core.TextField{Name: name})
	}
	rec := core.NewRecord(col)
	for k, v := range fields {
		rec.Set(k, v)
	}
	return rec
}

func TestDescriptorConfig_LabelJoinDefaultsToSpace(t *testing.T) {
	cfg := descriptorConfig(Descriptor{
		Collection:  "contacts",
		LabelFields: []string{"first_name", "last_name"},
	})
	rec := newRec(t, map[string]string{"first_name": "Alice", "last_name": "Smith"})
	if got := cfg.ExtractLabel(rec); got != "Alice Smith" {
		t.Errorf("label = %q, want %q", got, "Alice Smith")
	}
}

func TestDescriptorConfig_LabelTrimsWhenAFieldEmpty(t *testing.T) {
	cfg := descriptorConfig(Descriptor{
		Collection:  "contacts",
		LabelFields: []string{"first_name", "last_name"},
	})
	rec := newRec(t, map[string]string{"first_name": "Alice", "last_name": ""})
	if got := cfg.ExtractLabel(rec); got != "Alice" {
		t.Errorf("label = %q, want %q (trimmed)", got, "Alice")
	}
}

func TestDescriptorConfig_CustomJoin(t *testing.T) {
	cfg := descriptorConfig(Descriptor{
		Collection:  "x",
		LabelFields: []string{"a", "b"},
		LabelJoin:   ":",
	})
	rec := newRec(t, map[string]string{"a": "1", "b": "2"})
	if got := cfg.ExtractLabel(rec); got != "1:2" {
		t.Errorf("label = %q, want %q", got, "1:2")
	}
}

func TestDescriptorConfig_NoLabelFieldsLeavesDefault(t *testing.T) {
	cfg := descriptorConfig(Descriptor{Collection: "x"})
	if cfg.ExtractLabel != nil {
		t.Error("ExtractLabel should be nil when no LabelFields (falls back to default)")
	}
}

func TestDescriptorConfig_NoResolveOrgLeavesDefault(t *testing.T) {
	cfg := descriptorConfig(Descriptor{Collection: "x", LabelFields: []string{"a"}})
	if cfg.ResolveOrg != nil {
		t.Error("ResolveOrg should be nil when ResolveOrg.Field empty (falls back to default)")
	}
}

func TestDescriptorConfig_ResolveOrgEmptyRelationReturnsEmpty(t *testing.T) {
	cfg := descriptorConfig(Descriptor{
		Collection: "contacts",
		ResolveOrg: OrgVia{Field: "owner", Collection: "user_org", OrgField: "org"},
	})
	// Record with no owner set → resolver returns "" without touching the DB.
	rec := newRec(t, map[string]string{"owner": ""})
	if got := cfg.ResolveOrg(nil, rec); got != "" {
		t.Errorf("resolveOrg with empty owner = %q, want empty", got)
	}
}
