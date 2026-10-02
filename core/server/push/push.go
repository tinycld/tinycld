package push

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
	"tinycld.org/core/syscfg"
)

var log = logging.ForPackage("push")

// GenerateVAPIDKeys mints a fresh VAPID keypair (ECDSA P-256, base64url). The
// public key derives from the private, so they must be generated together — this
// wraps webpush so coreserver's admin endpoint can offer a "generate" action
// without importing the webpush library directly.
func GenerateVAPIDKeys() (privateKey, publicKey string, err error) {
	return webpush.GenerateVAPIDKeys()
}

// SendTimeout bounds one request to a push service. The client has no timeout
// of its own, and a caller often cannot afford to wait on a peer it does not
// control: a backup announces itself before it releases the job interlock, so
// one hung push service would otherwise block every backup, restore and package
// install. A healthy push service answers in well under a second.
const SendTimeout = 15 * time.Second

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
	SendToUserContext(context.Background(), app, userID, payload)
}

// SendToUserContext is SendToUser bound to ctx. Each send is a request to a
// push service this process does not control, so a caller that must finish
// before a deadline — a run the server waits for on its way down — passes the
// context that ends it.
func SendToUserContext(ctx context.Context, app core.App, userID string, payload Payload) {
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

	// Read through syscfg, never the system_settings collection directly: where
	// a supervising composition owns web push, the keys live in that supervisor's
	// memory and were never written to this deployment's database. A direct
	// lookup would find nothing, and every send would sign with an empty key —
	// silently, since a push failure is only logged.
	vapidPublicKey := syscfg.Get("vapid.public_key")
	vapidPrivateKey := syscfg.Get("vapid.private_key")
	vapidSubject := syscfg.Get("vapid.subject")
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

		sendCtx, cancel := context.WithTimeout(ctx, SendTimeout)
		resp, err := webpush.SendNotificationWithContext(sendCtx, payloadBytes, sub, &webpush.Options{
			VAPIDPublicKey:  vapidPublicKey,
			VAPIDPrivateKey: vapidPrivateKey,
			Subscriber:      vapidSubject,
			TTL:             86400,
		})
		if err != nil {
			cancel()
			log.Info("send failed for subscription", "subscriptionID", record.Id, "err", err)
			continue
		}
		resp.Body.Close()
		cancel()

		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			log.Info("removing stale subscription", "subscriptionID", record.Id, "status", resp.StatusCode)
			if err := app.Delete(record); err != nil {
				log.Info("failed to delete stale subscription", "subscriptionID", record.Id, "err", err)
			}
		}
	}
}
