package coreserver

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// handleGenerateVapid mints a keypair, persists BOTH keys to system_settings, and
// returns ONLY the public key — the private key must never leave the server.
func TestHandleGenerateVapid(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	createSystemSettingsCollection(t, app)

	req := httptest.NewRequest("POST", "/api/admin/vapid/generate", strings.NewReader("{}"))
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()

	if err := handleGenerateVapid(app, re); err != nil {
		t.Fatalf("handleGenerateVapid returned error: %v", err)
	}

	// Response carries the public key and NOT the private key.
	body := re.Response.(*httptest.ResponseRecorder).Body.String()
	var resp map[string]string
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("response not JSON: %v (%s)", err, body)
	}
	if resp["publicKey"] == "" {
		t.Error("response missing publicKey")
	}
	if _, leaked := resp["privateKey"]; leaked {
		t.Error("response must NOT include privateKey")
	}
	if strings.Contains(body, "privateKey") {
		t.Errorf("private key leaked into response body: %s", body)
	}

	// Both keys are persisted to system_settings; the private one is marked secret.
	pub, err := app.FindFirstRecordByFilter("system_settings", "key = 'vapid.public_key'")
	if err != nil {
		t.Fatalf("vapid.public_key not persisted: %v", err)
	}
	if pub.GetString("value") != resp["publicKey"] {
		t.Error("persisted public key does not match the returned one")
	}
	if pub.GetBool("is_secret") {
		t.Error("public key should not be marked secret")
	}

	priv, err := app.FindFirstRecordByFilter("system_settings", "key = 'vapid.private_key'")
	if err != nil {
		t.Fatalf("vapid.private_key not persisted: %v", err)
	}
	if priv.GetString("value") == "" {
		t.Error("private key value should be stored server-side")
	}
	if !priv.GetBool("is_secret") {
		t.Error("private key must be marked is_secret")
	}
}

// upsertSystemSetting updates an existing row or inserts a new one.
func TestUpsertSystemSetting(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	createSystemSettingsCollection(t, app)

	if err := upsertSystemSetting(app, "sentry.dsn", "https://a/1", false); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := upsertSystemSetting(app, "sentry.dsn", "https://b/2", false); err != nil {
		t.Fatalf("update: %v", err)
	}

	recs, err := app.FindRecordsByFilter("system_settings", "key = 'sentry.dsn'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", len(recs))
	}
	if recs[0].GetString("value") != "https://b/2" {
		t.Errorf("upsert did not update value, got %q", recs[0].GetString("value"))
	}
}
