package carddav

import (
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/pocketbase/pocketbase/core"
)

// contactsVCardMap mirrors the field map contacts declares in its manifest — the
// reference shape for the generic codec.
var contactsVCardMap = VCardMap{
	Version: "4.0",
	Name:    NameMap{Given: "first_name", Family: "last_name"},
	Simple: map[string]string{
		vcard.FieldEmail:        "email",
		vcard.FieldTelephone:    "phone",
		vcard.FieldOrganization: "company",
		vcard.FieldTitle:        "job_title",
		vcard.FieldNote:         "notes",
	},
	RevField: "updated",
}

// newContactRecord builds a bare `contacts`-shaped record for codec tests.
func newContactRecord(t *testing.T, fields map[string]any) *core.Record {
	t.Helper()
	col := core.NewBaseCollection("contacts")
	for name := range map[string]bool{
		"first_name": true, "last_name": true, "email": true, "phone": true,
		"company": true, "job_title": true, "notes": true, "vcard_uid": true,
	} {
		col.Fields.Add(&core.TextField{Name: name})
	}
	// `updated` is an autodate column in the live schema, but reads back from
	// SQLite as a string — model it as a TextField here so the test can populate
	// it (an in-memory AutodateField ignores Set, unlike a real DB read).
	col.Fields.Add(&core.TextField{Name: "updated"})
	rec := core.NewRecord(col)
	for k, v := range fields {
		rec.Set(k, v)
	}
	return rec
}

func TestRecordToVCard_MapsAllFields(t *testing.T) {
	rec := newContactRecord(t, map[string]any{
		"first_name": "Alice",
		"last_name":  "Smith",
		"email":      "alice@example.com",
		"phone":      "555-1212",
		"company":    "Acme",
		"job_title":  "CTO",
		"notes":      "VIP",
		"vcard_uid":  "urn:uuid:abc",
	})

	card := RecordToVCard(rec, contactsVCardMap)

	if got := card.Value(vcard.FieldFormattedName); got != "Alice Smith" {
		t.Errorf("FN = %q, want %q", got, "Alice Smith")
	}
	if n := card.Name(); n == nil || n.GivenName != "Alice" || n.FamilyName != "Smith" {
		t.Errorf("N = %+v, want given=Alice family=Smith", n)
	}
	if got := card.Value(vcard.FieldEmail); got != "alice@example.com" {
		t.Errorf("EMAIL = %q", got)
	}
	if got := card.Value(vcard.FieldTelephone); got != "555-1212" {
		t.Errorf("TEL = %q", got)
	}
	if got := card.Value(vcard.FieldOrganization); got != "Acme" {
		t.Errorf("ORG = %q", got)
	}
	if got := card.Value(vcard.FieldTitle); got != "CTO" {
		t.Errorf("TITLE = %q", got)
	}
	if got := card.Value(vcard.FieldNote); got != "VIP" {
		t.Errorf("NOTE = %q", got)
	}
	if got := card.Value(vcard.FieldVersion); got != "4.0" {
		t.Errorf("VERSION = %q", got)
	}
}

func TestRecordToVCard_OmitsEmptySimpleFields(t *testing.T) {
	rec := newContactRecord(t, map[string]any{
		"first_name": "Bob",
		"last_name":  "Jones",
	})
	card := RecordToVCard(rec, contactsVCardMap)

	if got := card.Value(vcard.FieldEmail); got != "" {
		t.Errorf("empty email should be omitted, got %q", got)
	}
	if got := card.Value(vcard.FieldFormattedName); got != "Bob Jones" {
		t.Errorf("FN = %q", got)
	}
}

func TestApplyVCardToRecord_PrefersN(t *testing.T) {
	card := make(vcard.Card)
	card.Set(vcard.FieldName, &vcard.Field{Value: "Smith;Alice;;;"})
	card.Set(vcard.FieldEmail, &vcard.Field{Value: "a@example.com"})
	card.Set(vcard.FieldOrganization, &vcard.Field{Value: "Acme"})

	rec := newContactRecord(t, nil)
	ApplyVCardToRecord(card, rec, contactsVCardMap)

	if got := rec.GetString("first_name"); got != "Alice" {
		t.Errorf("first_name = %q, want Alice", got)
	}
	if got := rec.GetString("last_name"); got != "Smith" {
		t.Errorf("last_name = %q, want Smith", got)
	}
	if got := rec.GetString("email"); got != "a@example.com" {
		t.Errorf("email = %q", got)
	}
	if got := rec.GetString("company"); got != "Acme" {
		t.Errorf("company = %q", got)
	}
}

func TestApplyVCardToRecord_FallsBackToFN(t *testing.T) {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldFormattedName, "Carol Nguyen")

	rec := newContactRecord(t, nil)
	ApplyVCardToRecord(card, rec, contactsVCardMap)

	if got := rec.GetString("first_name"); got != "Carol" {
		t.Errorf("first_name = %q, want Carol", got)
	}
	if got := rec.GetString("last_name"); got != "Nguyen" {
		t.Errorf("last_name = %q, want Nguyen", got)
	}
}

// Round-trip: encoding a record then decoding it back preserves the mapped fields.
func TestVCardRoundTrip(t *testing.T) {
	orig := newContactRecord(t, map[string]any{
		"first_name": "Dana",
		"last_name":  "Lee",
		"email":      "dana@example.com",
		"company":    "Globex",
	})
	card := RecordToVCard(orig, contactsVCardMap)

	dst := newContactRecord(t, nil)
	ApplyVCardToRecord(card, dst, contactsVCardMap)

	for _, f := range []string{"first_name", "last_name", "email", "company"} {
		if orig.GetString(f) != dst.GetString(f) {
			t.Errorf("round-trip %s: %q != %q", f, orig.GetString(f), dst.GetString(f))
		}
	}
}

// A stored REV timestamp is emitted in vCard's basic-format UTC.
func TestRecordToVCard_RevFormatting(t *testing.T) {
	rec := newContactRecord(t, map[string]any{
		"first_name": "E", "last_name": "F",
		"updated": "2026-03-15 09:30:00.000Z", // as PB stores/reads it
	})

	card := RecordToVCard(rec, contactsVCardMap)
	if got := card.Value(vcard.FieldRevision); got != "20260315T093000Z" {
		t.Errorf("REV = %q, want 20260315T093000Z", got)
	}
}
