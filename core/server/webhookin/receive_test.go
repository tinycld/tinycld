package webhookin

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/ratelimit"
)

func TestVerifySignature_AcceptsAMatchingDigest(t *testing.T) {
	secret := "s3cret"
	body := []byte(`{"hello":"world"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifySignature(secret, body, header) {
		t.Error("a correct signature was rejected")
	}
}

func TestVerifySignature_RejectsTampering(t *testing.T) {
	secret := "s3cret"
	body := []byte(`{"hello":"world"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if verifySignature(secret, []byte(`{"hello":"tampered"}`), header) {
		t.Error("a body that did not match its signature was accepted")
	}
	if verifySignature("wrong-secret", body, header) {
		t.Error("a signature under the wrong secret was accepted")
	}
}

func TestVerifySignature_RejectsMalformedHeaders(t *testing.T) {
	body := []byte(`{}`)
	for _, header := range []string{
		"",                  // absent
		"deadbeef",          // no algorithm prefix
		"sha1=deadbeef",     // wrong algorithm
		"sha256=",           // empty digest
		"sha256=not-hex-!!", // undecodable
	} {
		if verifySignature("s3cret", body, header) {
			t.Errorf("malformed header %q was accepted", header)
		}
	}
}

func TestVerifySignature_RejectsWhenNoSecretIsConfigured(t *testing.T) {
	// An unconfigured source must not fall open to "no secret, no check".
	body := []byte(`{}`)
	mac := hmac.New(sha256.New, []byte(""))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if verifySignature("", body, header) {
		t.Error("a blank secret was treated as valid configuration")
	}
}

func TestDeliveryLimiter_MetersPerSource(t *testing.T) {
	limiter := ratelimit.New(2, time.Minute)

	if !limiter.Allow("acme") || !limiter.Allow("acme") {
		t.Fatal("the first two deliveries were rejected")
	}
	if limiter.Allow("acme") {
		t.Error("a third delivery slipped past the ceiling")
	}
	if !limiter.Allow("widgets") {
		t.Error("one source's burst consumed another's budget")
	}
}

// --- Handler-level integration tests -------------------------------------
//
// verifySignature and the rate limiter are unit-tested above, but nothing
// exercised handleDelivery end to end. The ordering it enforces — read body,
// verify signature, rate-limit, claim delivery, dispatch — IS the security
// property this task adds; a silent reordering must fail one of these tests,
// not just look wrong on inspection. Handle-call counts (not merely status
// codes) are the load-bearing assertion in every negative case: a 403 with
// Handle still invoked would be exactly the fail-open bug this guards
// against.

// newDeliveriesTestApp builds an in-memory PocketBase test app with the
// webhook_deliveries collection, mirroring
// pb_migrations/1910000030_create_webhook_deliveries.js by hand: tests.NewTestApp
// only wires PocketBase's own system migrations, not this repo's
// pb_migrations, so claimDelivery's target collection does not exist unless a
// test creates it itself (the same reason offboard_test.go and oauth's
// newSchemaApp hand-build their fixture collections).
func newDeliveriesTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	deliveries := core.NewBaseCollection("webhook_deliveries")
	deliveries.Fields.Add(&core.TextField{Name: "source", Required: true, Max: 50})
	deliveries.Fields.Add(&core.TextField{Name: "delivery_id", Required: true, Max: 200})
	// The unique index is the mechanism under test in the duplicate-delivery
	// case: claimDelivery relies on the second insert violating it.
	deliveries.AddIndex("idx_webhook_deliveries_unique", true, "source, delivery_id", "")
	if err := app.Save(deliveries); err != nil {
		t.Fatalf("save webhook_deliveries: %v", err)
	}

	return app
}

// newDeliveryRequestEvent builds a RequestEvent for POST /api/webhooks/{source}
// as handleDelivery reads it directly, bypassing the router (same convention
// as oauth's newRevokeRequestEvent): no router is running to parse the
// {source} path segment or apply middleware, so the test drives handleDelivery
// itself with a body reader and headers it controls.
func newDeliveryRequestEvent(app core.App, body []byte, headers map[string]string) *core.RequestEvent {
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/acme", bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	return re
}

// countingSource returns a Source alongside a *int that counts Handle
// invocations and a func to retrieve the last Delivery passed to it. Every
// negative-path test below asserts on this counter, not merely on the
// response status — a status-only assertion could not distinguish "refused
// before dispatch" from "dispatched, and the handler itself decided to
// return an error that happens to map to the same status".
func countingSource(secret string, handleErr error) (*Source, *int, func() Delivery) {
	calls := 0
	var last Delivery
	src := &Source{
		Secret:          func(core.App, *http.Request) (string, error) { return secret, nil },
		SignatureHeader: defaultSignatureHeader,
		EventHeader:     "X-Acme-Event",
		DeliveryID:      func(r *http.Request) string { return r.Header.Get("X-Acme-Delivery") },
		Handle: func(_ core.App, d Delivery) error {
			calls++
			last = d
			return handleErr
		},
	}
	return src, &calls, func() Delivery { return last }
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandleDelivery_ValidSignatureDispatches(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	secret := "s3cret"
	src, calls, lastDelivery := countingSource(secret, nil)
	Register("acme", *src)

	body := []byte(`{"hello":"world"}`)
	re := newDeliveryRequestEvent(app, body, map[string]string{
		defaultSignatureHeader: signBody(secret, body),
		"X-Acme-Event":         "push",
		"X-Acme-Delivery":      "dlv-1",
	})

	source, ok := lookup("acme")
	if !ok {
		t.Fatal("acme not registered")
	}
	if err := handleDelivery(re, "acme", source); err != nil {
		t.Fatalf("handleDelivery returned an error for a valid delivery: %v", err)
	}
	if rec := re.Response.(*httptest.ResponseRecorder); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if *calls != 1 {
		t.Fatalf("Handle called %d times, want exactly 1", *calls)
	}
	got := lastDelivery()
	if string(got.Body) != string(body) {
		t.Errorf("Delivery.Body = %q, want the exact raw bytes sent %q", got.Body, body)
	}
	if got.Event != "push" {
		t.Errorf("Delivery.Event = %q, want %q", got.Event, "push")
	}
	if got.DeliveryID != "dlv-1" {
		t.Errorf("Delivery.DeliveryID = %q, want %q", got.DeliveryID, "dlv-1")
	}
}

func TestHandleDelivery_TamperedBodyIsRefused(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	secret := "s3cret"
	src, calls, _ := countingSource(secret, nil)
	Register("acme", *src)

	signedBody := []byte(`{"hello":"world"}`)
	sentBody := []byte(`{"hello":"tampered"}`)
	re := newDeliveryRequestEvent(app, sentBody, map[string]string{
		defaultSignatureHeader: signBody(secret, signedBody),
	})

	source, _ := lookup("acme")
	err := handleDelivery(re, "acme", source)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("status = %d (err=%v), want 403", status, err)
	}
	if *calls != 0 {
		t.Errorf("Handle called %d times, want 0 — an unverified caller must never reach the handler", *calls)
	}
}

func TestHandleDelivery_WrongSecretIsRefused(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	src, calls, _ := countingSource("s3cret", nil)
	Register("acme", *src)

	body := []byte(`{"hello":"world"}`)
	re := newDeliveryRequestEvent(app, body, map[string]string{
		defaultSignatureHeader: signBody("wrong-secret", body),
	})

	source, _ := lookup("acme")
	err := handleDelivery(re, "acme", source)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("status = %d (err=%v), want 403", status, err)
	}
	if *calls != 0 {
		t.Errorf("Handle called %d times, want 0", *calls)
	}
}

func TestHandleDelivery_BlankSecretIsRefused(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	// "Unconfigured is not permission": a Source whose Secret resolves to ""
	// must fail closed exactly like a bad signature, never fall open to
	// "no secret, no check".
	calls := 0
	src := Source{
		Secret:          func(core.App, *http.Request) (string, error) { return "", nil },
		SignatureHeader: defaultSignatureHeader,
		Handle: func(core.App, Delivery) error {
			calls++
			return nil
		},
	}
	Register("acme", src)

	body := []byte(`{"hello":"world"}`)
	re := newDeliveryRequestEvent(app, body, map[string]string{
		defaultSignatureHeader: signBody("", body),
	})

	source, _ := lookup("acme")
	err := handleDelivery(re, "acme", source)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("status = %d (err=%v), want 403", status, err)
	}
	if calls != 0 {
		t.Errorf("Handle called %d times, want 0 — an unconfigured secret must fail closed", calls)
	}
}

func TestHandleDelivery_MissingSignatureHeaderIsRefused(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	src, calls, _ := countingSource("s3cret", nil)
	Register("acme", *src)

	body := []byte(`{"hello":"world"}`)
	re := newDeliveryRequestEvent(app, body, nil) // no signature header at all

	source, _ := lookup("acme")
	err := handleDelivery(re, "acme", source)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("status = %d (err=%v), want 403", status, err)
	}
	if *calls != 0 {
		t.Errorf("Handle called %d times, want 0", *calls)
	}
}

func TestHandleDelivery_DuplicateDeliveryDoesNotRedispatch(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	secret := "s3cret"
	src, calls, _ := countingSource(secret, nil)
	Register("acme", *src)
	source, _ := lookup("acme")

	body := []byte(`{"hello":"world"}`)
	headers := map[string]string{
		defaultSignatureHeader: signBody(secret, body),
		"X-Acme-Delivery":      "dlv-replay",
	}

	re1 := newDeliveryRequestEvent(app, body, headers)
	if err := handleDelivery(re1, "acme", source); err != nil {
		t.Fatalf("first delivery: unexpected error %v", err)
	}
	if rec := re1.Response.(*httptest.ResponseRecorder); rec.Code != http.StatusOK {
		t.Fatalf("first delivery status = %d, want 200", rec.Code)
	}

	re2 := newDeliveryRequestEvent(app, body, headers)
	if err := handleDelivery(re2, "acme", source); err != nil {
		t.Fatalf("replayed delivery: unexpected error %v", err)
	}
	if rec := re2.Response.(*httptest.ResponseRecorder); rec.Code != http.StatusOK {
		t.Fatalf("replayed delivery status = %d, want 200 (a retry of completed work is a success)", rec.Code)
	}

	if *calls != 1 {
		t.Fatalf("Handle called %d times across original + replay, want exactly 1", *calls)
	}
}

func TestHandleDelivery_HandlerErrorReturns500(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	secret := "s3cret"
	src, calls, _ := countingSource(secret, errSentinel)
	Register("acme", *src)

	body := []byte(`{"hello":"world"}`)
	re := newDeliveryRequestEvent(app, body, map[string]string{
		defaultSignatureHeader: signBody(secret, body),
		"X-Acme-Delivery":      "dlv-err",
	})

	source, _ := lookup("acme")
	err := handleDelivery(re, "acme", source)
	if status := apiStatus(err); status != http.StatusInternalServerError {
		t.Fatalf("status = %d (err=%v), want 500 so a well-behaved provider retries", status, err)
	}
	if *calls != 1 {
		t.Fatalf("Handle called %d times, want exactly 1 (it did run — it just failed)", *calls)
	}
}

func TestHandleDelivery_OversizedBodyIsRejected(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	secret := "s3cret"
	src, calls, _ := countingSource(secret, nil)
	Register("acme", *src)

	// One byte over the cap. The signature is deliberately NOT computed over
	// this body: an oversized payload must be rejected by the size check
	// before signature verification even runs.
	body := make([]byte, maxBodyBytes+1)
	re := newDeliveryRequestEvent(app, body, map[string]string{
		defaultSignatureHeader: signBody(secret, body),
	})

	source, _ := lookup("acme")
	err := handleDelivery(re, "acme", source)
	status := apiStatus(err)
	if status < 400 || status >= 500 {
		t.Fatalf("status = %d (err=%v), want a 4xx", status, err)
	}
	if *calls != 0 {
		t.Errorf("Handle called %d times, want 0", *calls)
	}
}

// TestMountRoutes_UnknownSourceIs404 goes through the real router — unlike
// every other case in this file, which calls handleDelivery directly — so it
// is the one test that actually exercises MountRoutes' own lookup-then-404
// branch and its {source} path-value wiring, rather than assuming the router
// will parse the wildcard the way handleDelivery's caller expects.
func TestMountRoutes_UnknownSourceIs404(t *testing.T) {
	resetRegistry(t)
	app := newDeliveriesTestApp(t)

	pbRouter := router.NewRouter(func(w http.ResponseWriter, r *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		event := new(core.RequestEvent)
		event.Response = w
		event.Request = r
		event.App = app
		return event, nil
	})
	MountRoutes(&core.ServeEvent{App: app, Router: pbRouter})

	mux, err := pbRouter.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/nope", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an unregistered source", rec.Code)
	}
}

// apiStatus extracts the HTTP status handleDelivery refused with, or 0 if err
// is not the *router.ApiError it is documented to return.
func apiStatus(err error) int {
	apiErr, ok := err.(*router.ApiError)
	if !ok {
		return 0
	}
	return apiErr.Status
}
