package automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestActionPostWebhook_PostsTriggerFieldsAsJSON(t *testing.T) {
	var gotURL string
	var gotBody []byte
	var gotHeaders map[string]string
	restore := stubPostWebhook(func(url string, body []byte, headers map[string]string) (int, error) {
		gotURL, gotBody, gotHeaders = url, body, headers
		return 200, nil
	})
	defer restore()

	err := actionPostWebhook(nil, ActionRequest{
		Params: map[string]string{"url": "https://hooks.example.com/abc"},
	})
	if err != nil {
		t.Fatalf("actionPostWebhook: %v", err)
	}
	if gotURL != "https://hooks.example.com/abc" {
		t.Errorf("url = %q", gotURL)
	}
	var decoded map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if _, signed := gotHeaders["X-TinyCld-Signature-256"]; signed {
		t.Error("unsigned request carries a signature header")
	}
}

func TestActionPostWebhook_SignsWhenSecretIsSet(t *testing.T) {
	var gotHeaders map[string]string
	restore := stubPostWebhook(func(_ string, _ []byte, headers map[string]string) (int, error) {
		gotHeaders = headers
		return 200, nil
	})
	defer restore()

	err := actionPostWebhook(nil, ActionRequest{
		Params: map[string]string{
			"url":    "https://hooks.example.com/abc",
			"secret": "shhh",
		},
	})
	if err != nil {
		t.Fatalf("actionPostWebhook: %v", err)
	}
	sig := gotHeaders["X-TinyCld-Signature-256"]
	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("signature = %q, want a sha256= prefix", sig)
	}
}

func TestActionPostWebhook_RequiresAURL(t *testing.T) {
	restore := stubPostWebhook(func(string, []byte, map[string]string) (int, error) {
		t.Fatal("must not post without a URL")
		return 0, nil
	})
	defer restore()

	if err := actionPostWebhook(nil, ActionRequest{Params: map[string]string{}}); err == nil {
		t.Error("expected an error for a missing URL")
	}
}

func TestActionPostWebhook_RejectsNon2xx(t *testing.T) {
	restore := stubPostWebhook(func(string, []byte, map[string]string) (int, error) {
		return 500, nil
	})
	defer restore()

	err := actionPostWebhook(nil, ActionRequest{
		Params: map[string]string{"url": "https://hooks.example.com/abc"},
	})
	if err == nil {
		t.Error("expected an error for a 500 response")
	}
}

func TestReserveWebhookPost_EnforcesHourlyCeiling(t *testing.T) {
	webhookPostLedger.Lock()
	webhookPostLedger.posts = map[string][]time.Time{}
	webhookPostLedger.Unlock()

	now := time.Now()
	for i := 0; i < maxWebhookPostsPerRulePerHour; i++ {
		if err := reserveWebhookPost("rule1", now); err != nil {
			t.Fatalf("post %d rejected early: %v", i+1, err)
		}
	}
	if err := reserveWebhookPost("rule1", now); err == nil {
		t.Error("expected the ceiling to reject the next post")
	}
	// A different rule has its own budget.
	if err := reserveWebhookPost("rule2", now); err != nil {
		t.Errorf("a second rule was blocked by the first's budget: %v", err)
	}
	// An hour later the window has rolled.
	if err := reserveWebhookPost("rule1", now.Add(61*time.Minute)); err != nil {
		t.Errorf("the window did not roll: %v", err)
	}
}

// stubPostWebhook replaces the network seam and returns a restore func.
func stubPostWebhook(fn func(url string, body []byte, headers map[string]string) (int, error)) func() {
	original := postWebhookNow
	postWebhookNow = func(_ context.Context, url string, body []byte, headers map[string]string) (int, error) {
		return fn(url, body, headers)
	}
	return func() { postWebhookNow = original }
}
