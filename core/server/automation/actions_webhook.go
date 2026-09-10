package automation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/safehttp"
)

// maxWebhookPostsPerRulePerHour caps how many outbound posts one rule may
// emit. The reasoning is actions_email.go's, unchanged: the engine's depth cap
// stops a rule re-triggering itself inside one dispatch but cannot see across
// dispatches, and a webhook whose receiver writes back into this deployment
// closes exactly that loop. The rule is the unit because core has no other
// key to meter on.
const maxWebhookPostsPerRulePerHour = 60

// webhookPostTimeout bounds one delivery. Receivers like Zapier acknowledge
// quickly; a slow one must not hold an action slot open.
const webhookPostTimeout = 10 * time.Second

// postWebhookNow is the seam tests replace. Production goes through safehttp,
// which validates the target and refuses redirects.
var postWebhookNow = func(
	ctx context.Context,
	url string,
	body []byte,
	headers map[string]string,
) (int, error) {
	return safehttp.PostJSON(ctx, url, body, headers, webhookPostTimeout)
}

// registerCoreWebhookAction installs core:post-webhook.
//
// Native, but native IN CORE, the actions_email.go argument: it ships in every
// build regardless of which feature packages an org installed, so "POST to my
// automation tool when X" exists on every deployment. It is also the outbound
// half of the integration story — an external service consuming these posts
// needs no package-specific code at all.
func registerCoreWebhookAction() {
	RegisterAction("core:post-webhook", actionPostWebhook)
}

// actionPostWebhook delivers one JSON POST on behalf of a rule.
//
// Native handlers are not pkgaccess-gated — the engine hands them a superuser
// app — so this validates its own inputs. `url` arrives after template
// substitution, so its value is only as trustworthy as the record that
// triggered the rule; safehttp.ValidateURL is what stands between a templated
// value and an internal address.
func actionPostWebhook(_ core.App, req ActionRequest) error {
	target := strings.TrimSpace(req.Params["url"])
	if target == "" {
		return fmt.Errorf("core:post-webhook: no url")
	}

	// A request with no rule (a dry-run style invocation) has nothing to
	// count against; every engine path supplies the rule.
	if req.Rule != nil {
		if err := reserveWebhookPost(req.Rule.Id, time.Now()); err != nil {
			return fmt.Errorf("core:post-webhook: %w", err)
		}
	}

	body, err := webhookBody(req)
	if err != nil {
		return fmt.Errorf("core:post-webhook: %w", err)
	}

	headers := map[string]string{}
	if secret := req.Params["secret"]; secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		headers["X-TinyCld-Signature-256"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}

	ctx, cancel := context.WithTimeout(context.Background(), webhookPostTimeout)
	defer cancel()

	status, err := postWebhookNow(ctx, target, body, headers)
	if err != nil {
		return fmt.Errorf("core:post-webhook: %w", err)
	}
	if status < 200 || status > 299 {
		return fmt.Errorf("core:post-webhook: receiver returned %d", status)
	}
	return nil
}

// webhookBody renders the triggering record as the POST payload.
//
// PublicExport is what the engine already uses to expose a record to
// templates, so a webhook cannot see fields a rule's own {{placeholders}}
// could not — notably users.password and users.tokenKey, which the engine's
// exposure rules filter out everywhere.
func webhookBody(req ActionRequest) ([]byte, error) {
	payload := map[string]any{}
	if req.Record != nil {
		payload["collection"] = req.Record.Collection().Name
		payload["record"] = req.Record.PublicExport()
	}
	if req.Rule != nil {
		payload["rule"] = map[string]any{
			"id":   req.Rule.Id,
			"name": req.Rule.GetString("name"),
		}
	}
	return json.Marshal(payload)
}

// webhookPostLedger tracks each rule's posts inside the rolling hour, in
// memory. In-memory is deliberate, actions_email.go's reasoning: there is no
// query that can fail, so the cap cannot fail open — and an unavailable count
// must never be read as permission to send.
var webhookPostLedger = struct {
	sync.Mutex
	posts map[string][]time.Time
}{posts: map[string][]time.Time{}}

// reserveWebhookPost claims one slot for the rule, or reports it is over the
// hourly ceiling. Claimed BEFORE the post, so a concurrent dispatch cannot
// slip past the ceiling and a failed delivery still consumes its slot —
// over-counting is the safe direction for a loop guard.
func reserveWebhookPost(ruleID string, now time.Time) error {
	webhookPostLedger.Lock()
	defer webhookPostLedger.Unlock()

	cutoff := now.Add(-time.Hour)
	kept := make([]time.Time, 0, len(webhookPostLedger.posts[ruleID]))
	for _, at := range webhookPostLedger.posts[ruleID] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) >= maxWebhookPostsPerRulePerHour {
		webhookPostLedger.posts[ruleID] = kept
		return fmt.Errorf(
			"rule is over its ceiling of %d webhook posts per hour",
			maxWebhookPostsPerRulePerHour,
		)
	}
	webhookPostLedger.posts[ruleID] = append(kept, now)
	return nil
}
