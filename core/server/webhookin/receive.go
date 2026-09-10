package webhookin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
	"tinycld.org/core/ratelimit"
)

// maxBodyBytes bounds one delivery. Providers cap their own payloads well
// below this; the limit exists so an unauthenticated route cannot be used to
// buffer arbitrary memory. Read BEFORE verification, because the signature is
// computed over the body we must therefore already hold.
const maxBodyBytes = 1 << 20 // 1 MiB

const defaultSignatureHeader = "X-TinyCld-Signature-256"

var log = logging.ForPackage("core")

// deliveryLimiter meters inbound deliveries per source.
//
// Generous, because a provider legitimately bursts — a repo with many open
// PRs can deliver dozens of events in a few seconds — and the signature
// check already rejects a forged caller before any database write. This is a
// ceiling against a verified-but-runaway sender, not an authentication
// control.
var deliveryLimiter = ratelimit.New(300, time.Minute)

// RegisterRoutes binds MountRoutes into the app's OnServe hook. Named
// RegisterRoutes rather than Register because this package already exports
// Register(name, Source) for a package registering its own webhook source —
// two different registrations, so they cannot share a name.
func RegisterRoutes(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		MountRoutes(se)
		return se.Next()
	})
}

// MountRoutes installs POST /api/webhooks/{source}.
//
// UNAUTHENTICATED by necessity — a provider holds no session and no OAuth
// token — so the HMAC signature IS the authentication, and every path below
// fails closed. The route must be excluded from OAuth scope classification
// rather than assigned a scope: it is not a caller acting for a user.
func MountRoutes(se *core.ServeEvent) {
	se.Router.POST("/api/webhooks/{source}", func(re *core.RequestEvent) error {
		name := re.Request.PathValue("source")
		source, ok := lookup(name)
		if !ok {
			// 404 rather than a hint that some other name would work.
			return re.NotFoundError("unknown webhook source", nil)
		}
		return handleDelivery(re, name, source)
	})
}

func handleDelivery(re *core.RequestEvent, name string, source Source) error {
	body, err := io.ReadAll(io.LimitReader(re.Request.Body, maxBodyBytes+1))
	if err != nil {
		return re.BadRequestError("could not read the request body", nil)
	}
	if len(body) > maxBodyBytes {
		return re.BadRequestError("payload too large", nil)
	}

	if source.Secret == nil {
		log.Error("webhook source has no secret resolver", "source", name)
		return re.InternalServerError("source is not configured", nil)
	}
	secret, err := source.Secret(re.App, re.Request)
	if err != nil || secret == "" {
		// Unconfigured is not permission. Fail closed.
		log.WarnContext(re.Request.Context(),
			"rejecting a webhook for a source with no usable secret",
			"source", name)
		return re.ForbiddenError("source is not configured", nil)
	}

	headerName := source.SignatureHeader
	if headerName == "" {
		headerName = defaultSignatureHeader
	}
	if !verifySignature(secret, body, re.Request.Header.Get(headerName)) {
		log.WarnContext(re.Request.Context(),
			"rejecting a webhook whose signature did not verify",
			"source", name)
		return re.ForbiddenError("signature verification failed", nil)
	}

	// Metered after verification so an unverified caller cannot consume
	// another source's budget.
	if !deliveryLimiter.Allow(name) {
		log.WarnContext(re.Request.Context(),
			"rejecting a webhook over its rate ceiling", "source", name)
		return re.TooManyRequestsError("rate limit exceeded", nil)
	}

	delivery := Delivery{
		Source: name,
		Body:   body,
		Header: re.Request.Header,
	}
	if source.EventHeader != "" {
		delivery.Event = re.Request.Header.Get(source.EventHeader)
	}
	if source.DeliveryID != nil {
		delivery.DeliveryID = source.DeliveryID(re.Request)
	}

	// Replay check AFTER verification: an unverified caller must not be able
	// to probe or populate the ledger.
	if delivery.DeliveryID != "" {
		fresh, err := claimDelivery(re.App, name, delivery.DeliveryID)
		if err != nil {
			return re.InternalServerError("could not record the delivery", nil)
		}
		if !fresh {
			// 200: a retry of work already done is a success, and any other
			// status invites the provider to keep retrying.
			return re.JSON(http.StatusOK, map[string]any{"duplicate": true})
		}
	}

	if source.Handle == nil {
		return re.InternalServerError("source has no handler", nil)
	}
	if err := source.Handle(re.App, delivery); err != nil {
		// 500 so a well-behaved provider retries. The delivery is already
		// claimed, so the retry will read as a duplicate — deliberately: a
		// handler that failed halfway must reconcile on its own terms rather
		// than have the same payload replayed into it.
		log.ErrorContext(re.Request.Context(), "webhook handler failed",
			"source", name, "event", delivery.Event, "error", err)
		return re.InternalServerError("handler failed", nil)
	}
	return re.JSON(http.StatusOK, map[string]any{"ok": true})
}

// verifySignature reports whether `header` is a valid sha256 HMAC of body
// under secret. Constant-time compare, and closed against every malformed
// shape: a blank secret, a missing or wrongly-prefixed header, and an
// undecodable digest all return false rather than skipping the check.
func verifySignature(secret string, body []byte, header string) bool {
	if secret == "" || header == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil || len(want) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

// claimDelivery records this delivery id, reporting false when the provider
// has sent it before. The unique index on (source, delivery_id) is what makes
// the claim atomic — two concurrent retries race to insert and exactly one
// wins, so this cannot be defeated by timing.
func claimDelivery(app core.App, source, deliveryID string) (bool, error) {
	collection, err := app.FindCollectionByNameOrId("webhook_deliveries")
	if err != nil {
		return false, fmt.Errorf("webhook_deliveries collection: %w", err)
	}
	record := core.NewRecord(collection)
	record.Set("source", source)
	record.Set("delivery_id", deliveryID)
	if err := app.Save(record); err != nil {
		// A unique-constraint violation is the duplicate case, not a fault.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
