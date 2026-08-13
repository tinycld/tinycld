package automation

import (
	"os"
	"path/filepath"
	"testing"
)

const fixtureJSON = `{
    "packages": [
        {
            "slug": "core",
            "triggers": [
                { "id": "schedule", "label": "On a schedule", "synthetic": "schedule" },
                { "id": "manual", "label": "Run manually", "synthetic": "manual" }
            ],
            "actions": [
                {
                    "id": "apply-label", "label": "Apply label", "kind": "record-op",
                    "collection": "label_assignments",
                    "op": { "type": "create", "set": {
                        "label": { "param": "label" },
                        "record_id": { "context": "record-id" },
                        "collection": { "context": "collection" },
                        "user": { "context": "owner" }
                    } },
                    "params": [{ "key": "label", "field": "label" }]
                },
                { "id": "notify", "label": "Send me a notification", "kind": "native",
                  "params": [{ "key": "title", "type": "text" }, { "key": "body", "type": "text" }] }
            ]
        },
        {
            "slug": "mail",
            "triggers": [
                { "id": "message-received", "label": "A message arrives",
                  "collection": "mail_messages", "on": "create",
                  "fields": ["subject", { "key": "sender_email", "label": "Sender" }] }
            ],
            "actions": []
        }
    ]
}`

func writeFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "automation_defs.json")
	if err := os.WriteFile(p, []byte(fixtureJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDefsMissingFileIsInert(t *testing.T) {
	defs, err := LoadDefs(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file must be inert, got %v", err)
	}
	if len(defs.Packages) != 0 {
		t.Fatalf("expected empty defs, got %d packages", len(defs.Packages))
	}
}

func TestLookupByQualifiedRef(t *testing.T) {
	defs, err := LoadDefs(writeFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	trig, pkg, ok := defs.Trigger("mail:message-received")
	if !ok || pkg != "mail" || trig.Collection != "mail_messages" || trig.On != "create" {
		t.Fatalf("trigger lookup failed: %+v %q %v", trig, pkg, ok)
	}
	if trig.Fields[1].Key != "sender_email" || trig.Fields[1].Label != "Sender" {
		t.Fatalf("mixed-form fields not decoded: %+v", trig.Fields)
	}
	act, pkg, ok := defs.Action("core:apply-label")
	if !ok || pkg != "core" || act.Kind != "record-op" || act.Op.Type != "create" {
		t.Fatalf("action lookup failed: %+v", act)
	}
	sv := act.Op.Set["record_id"]
	if sv.Context != "record-id" {
		t.Fatalf("context SetValue not decoded: %+v", sv)
	}
	if _, _, ok := defs.Trigger("mail:nope"); ok {
		t.Fatal("unknown ref must miss")
	}
}

func TestTriggersForCollectionOp(t *testing.T) {
	defs, _ := LoadDefs(writeFixture(t))
	hits := defs.TriggersFor("mail_messages", "create")
	if len(hits) != 1 || hits[0].Ref != "mail:message-received" {
		t.Fatalf("TriggersFor: %+v", hits)
	}
	if len(defs.TriggersFor("mail_messages", "delete")) != 0 {
		t.Fatal("op filter failed")
	}
}
