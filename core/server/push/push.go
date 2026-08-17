package push

import (
	"encoding/json"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("push")

// systemSetting reads a value from the system_settings collection — the
// system-wide config store (the same one coreserver.SystemConfig loads). push
// reads it directly from the app (not via coreserver) to avoid an import cycle;
// VAPID keys are needed per-send and rarely, so a direct lookup is fine.
func systemSetting(app core.App, key string) string {
	rec, err := app.FindFirstRecordByFilter("system_settings", "key = {:key}", map[string]any{"key": key})
	if err != nil {
		return ""
	}
	return rec.GetString("value")
}

// GenerateVAPIDKeys mints a fresh VAPID keypair (ECDSA P-256, base64url). The
// public key derives from the private, so they must be generated together — this
// wraps webpush so coreserver's admin endpoint can offer a "generate" action
// without importing the webpush library directly.
func GenerateVAPIDKeys() (privateKey, publicKey string, err error) {
	return webpush.GenerateVAPIDKeys()
}

// Payload is the JSON structure sent to the browser push service.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Tag   string `json:"tag,omitempty"`
	URL   string `json:"url,omitempty"`
}

// SendToUser sends a push notification to all subscriptions for the given user.
// Stale subscriptions (410/404) are automatically deleted.
func SendToUser(app core.App, userID string, payload Payload) {
	records, err := app.FindRecordsByFilter(
		"push_subscriptions",
		"user = {:userId}",
		"",
		0,
		0,
		map[string]any{"userId": userID},
	)
	if err != nil {
		log.Warn("failed to query subscriptions", "userID", userID, "err", err)
		return
	}

	if len(records) == 0 {
		return
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Warn("failed to marshal payload", "err", err)
		return
	}

	vapidPublicKey := systemSetting(app, "vapid.public_key")
	vapidPrivateKey := systemSetting(app, "vapid.private_key")
	vapidSubject := systemSetting(app, "vapid.subject")
	if vapidSubject == "" {
		vapidSubject = "mailto:admin@tinycld.com"
	}

	for _, record := range records {
		endpoint := record.GetString("endpoint")
		keysRaw := record.Get("keys")

		var keys struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		}

		switch v := keysRaw.(type) {
		case string:
			if err := json.Unmarshal([]byte(v), &keys); err != nil {
				log.Warn("invalid keys JSON for subscription", "subscriptionID", record.Id, "err", err)
				continue
			}
		case map[string]any:
			if p, ok := v["p256dh"].(string); ok {
				keys.P256dh = p
			}
			if a, ok := v["auth"].(string); ok {
				keys.Auth = a
			}
		default:
			log.Warn("unexpected keys type for subscription", "subscriptionID", record.Id)
			continue
		}

		sub := &webpush.Subscription{
			Endpoint: endpoint,
			Keys: webpush.Keys{
				P256dh: keys.P256dh,
				Auth:   keys.Auth,
			},
		}

		resp, err := webpush.SendNotification(payloadBytes, sub, &webpush.Options{
			VAPIDPublicKey:  vapidPublicKey,
			VAPIDPrivateKey: vapidPrivateKey,
			Subscriber:      vapidSubject,
			TTL:             86400,
		})
		if err != nil {
			log.Info("send failed for subscription", "subscriptionID", record.Id, "err", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			log.Info("removing stale subscription", "subscriptionID", record.Id, "status", resp.StatusCode)
			if err := app.Delete(record); err != nil {
				log.Info("failed to delete stale subscription", "subscriptionID", record.Id, "err", err)
			}
		}
	}
}
