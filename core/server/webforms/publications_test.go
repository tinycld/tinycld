package webforms

import (
	"strings"
	"testing"
)

func TestPublication_ServerOwnsTokenAndDefaults(t *testing.T) {
	f := newFixture(t, contactDef)
	if len(f.token) != 32 {
		t.Fatalf("want a 32-char token, got %q", f.token)
	}
	if f.pub.GetInt("retention_days") != 90 || f.pub.GetInt("max_upload_mb") != 25 {
		t.Fatalf("defaults not applied: %v", f.pub.FieldsData())
	}
	if !strings.Contains(f.pub.GetString("confirmation"), "Thank you") {
		t.Fatal("default confirmation not applied")
	}

	f.pub.Set("token", "chosen-by-client")
	if err := f.app.Save(f.pub); err == nil {
		t.Fatal("a client-chosen token must be refused")
	}

	f.pub.Set("token", "")
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	if got := f.pub.GetString("token"); got == f.token || len(got) != 32 {
		t.Fatalf("an empty token must regenerate, got %q", got)
	}
}

func TestPublication_RejectsBadOriginsAndOtherHosts(t *testing.T) {
	f := newFixture(t, contactDef)
	f.pub.Set("allowed_domains", "example.com")
	if err := f.app.Save(f.pub); err == nil || !strings.Contains(err.Error(), "allowed_domains") {
		t.Fatalf("want an allowed_domains error, got %v", err)
	}
	f.pub.Set("allowed_domains", "https://a.example  https://a.example")
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	if f.pub.GetString("allowed_domains") != "https://a.example" {
		t.Fatalf("origins not normalized: %q", f.pub.GetString("allowed_domains"))
	}
}
