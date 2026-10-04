package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
	"tinycld.org/core/push"
)

var log = logging.ForPackage("notify")

// NotifyParams describes a notification to send to a user.
type NotifyParams struct {
	UserID  string         `json:"userId"`
	Type    string         `json:"type"`
	Package string         `json:"package"`
	Title   string         `json:"title"`
	Body    string         `json:"body"`
	URL     string         `json:"url"`
	Meta    map[string]any `json:"metadata"`
}

// NotifyUser persists a notification record and dispatches push notifications
// to all registered devices for the user.
// NotifyUser delivers a notification, logging and swallowing any failure.
// Callers that need to REPORT the outcome (an automation action recording a
// run result, say) should use DeliverToUser instead.
func NotifyUser(app core.App, params NotifyParams) {
	_ = DeliverToUser(app, params)
}

// DeliverToUser is NotifyUser with the outcome returned rather than swallowed.
//
// "Delivered" means the notifications row was written — that is the durable
// part a user can still see later. The push dispatches below are genuinely
// best-effort (an unreachable device is not a rule failure), so they stay
// non-fatal. A muted type is a success: the user asked not to be told.
func DeliverToUser(app core.App, params NotifyParams) error {
	return DeliverToUserContext(context.Background(), app, params)
}

// DeliverToUserContext is DeliverToUser with its push dispatches bound to ctx.
// The row is written regardless; ctx ends only the requests to push services,
// which a caller on a deadline cannot afford to wait on.
func DeliverToUserContext(ctx context.Context, app core.App, params NotifyParams) error {
	// Check user preferences — skip if this notification type is muted
	if isNotificationMuted(app, params.UserID, params.Type) {
		return nil
	}

	// Insert into notifications collection
	collection, err := app.FindCollectionByNameOrId("notifications")
	if err != nil {
		log.Error("failed to find notifications collection", "err", err)
		return fmt.Errorf("notifications collection unavailable: %w", err)
	}

	record := core.NewRecord(collection)
	record.Set("user", params.UserID)
	record.Set("type", params.Type)
	record.Set("package", params.Package)
	record.Set("title", params.Title)
	record.Set("body", params.Body)
	record.Set("url", params.URL)
	record.Set("metadata", params.Meta)
	record.Set("read", false)
	record.Set("dismissed", false)

	if err := app.Save(record); err != nil {
		log.Error("failed to save notification", "userID", params.UserID, "err", err)
		return fmt.Errorf("saving notification: %w", err)
	}

	// Dispatch web push
	push.SendToUserContext(ctx, app, params.UserID, push.Payload{
		Title: params.Title,
		Body:  params.Body,
		Tag:   fmt.Sprintf("%s-%s", params.Type, record.Id),
		URL:   params.URL,
	})

	// Dispatch Expo push
	sendExpoPush(ctx, app, params.UserID, params)
	return nil
}

// isNotificationMuted checks user_preferences for a muted notification type.
func isNotificationMuted(app core.App, userID, notifType string) bool {
	records, err := app.FindRecordsByFilter(
		"user_preferences",
		"user = {:userId} && app = 'notifications' && key = 'preferences'",
		"",
		1,
		0,
		map[string]any{"userId": userID},
	)
	if err != nil || len(records) == 0 {
		return false
	}

	prefsRaw := records[0].Get("value")
	prefsJSON, err := json.Marshal(prefsRaw)
	if err != nil {
		return false
	}

	var prefs map[string]any
	if err := json.Unmarshal(prefsJSON, &prefs); err != nil {
		return false
	}

	if muted, ok := prefs[notifType]; ok {
		if enabled, ok := muted.(bool); ok {
			return !enabled
		}
	}
	return false
}

// isDemoUser reports whether the given user has the is_demo flag set.
// Defined locally rather than calling coreserver.IsDemoUser to avoid an
// import cycle (coreserver imports notify). Returns false on lookup failure
// so non-demo behavior is the safe default.
func isDemoUser(app core.App, userID string) bool {
	if userID == "" {
		return false
	}
	rec, err := app.FindRecordById("users", userID)
	if err != nil {
		return false
	}
	return rec.GetBool("is_demo")
}

// expoPushURL is the Expo Push API endpoint. A package var only so a test can
// point it at a local server; nothing else writes it.
var expoPushURL = "https://exp.host/--/api/v2/push/send"

// sendExpoPush sends push notifications to all Expo push subscriptions for the user.
func sendExpoPush(ctx context.Context, app core.App, userID string, params NotifyParams) {
	// Demo users: skip the external Expo Push API hop. The notification
	// record is already saved and the in-app web push has fired, so the user
	// still sees the notification in the app — we just don't wake an actual
	// device.
	if isDemoUser(app, userID) {
		return
	}

	records, err := app.FindRecordsByFilter(
		"push_subscriptions",
		"user = {:userId} && platform = 'expo'",
		"",
		0,
		0,
		map[string]any{"userId": userID},
	)
	if err != nil || len(records) == 0 {
		return
	}

	for _, record := range records {
		token := record.GetString("expo_token")
		if token == "" {
			continue
		}

		payload := map[string]any{
			"to":    token,
			"title": params.Title,
			"body":  params.Body,
			"data": map[string]string{
				"url": params.URL,
			},
			"sound": "default",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			log.Warn("failed to marshal expo payload", "token", token, "err", err)
			continue
		}

		stale, err := postExpo(ctx, body)
		if err != nil {
			log.Info("expo send failed for token", "token", token, "err", err)
			continue
		}
		if stale {
			log.Info("removing stale expo token", "token", token)
			if err := app.Delete(record); err != nil {
				log.Info("failed to delete stale expo token", "tokenID", record.Id, "err", err)
			}
		}
	}
}

// expoSendTimeout is push.SendTimeout, held in a var only so a test can shorten
// it; nothing else writes it.
var expoSendTimeout = push.SendTimeout

// postExpo sends one message to the Expo Push API and reports whether Expo says
// the device is gone. The timeout covers the body read too, so it is cancelled
// only once the response is consumed.
func postExpo(ctx context.Context, body []byte) (stale bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, expoSendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Status  string `json:"status"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, nil
	}
	return result.Data.Details.Error == "DeviceNotRegistered", nil
}
