package automation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/mailer"
)

// actions_email_test.go covers core:send-email. The send itself goes through
// the sendEmailNow seam so these never touch a real provider.

// captureEmails swaps the send seam for the duration of a test and returns the
// slice the handler writes into.
func captureEmails(t *testing.T) *[]*mailer.Message {
	t.Helper()
	sent := &[]*mailer.Message{}
	original := sendEmailNow
	sendEmailNow = func(_ context.Context, msg *mailer.Message) error {
		*sent = append(*sent, msg)
		return nil
	}
	t.Cleanup(func() { sendEmailNow = original })
	return sent
}

func TestActionSendEmail_SendsThroughTheCoreMailer(t *testing.T) {
	app, rule := runsApp(t)
	sent := captureEmails(t)

	err := actionSendEmail(app, ActionRequest{
		Rule: rule,
		Params: map[string]string{
			"to":      "someone@example.com",
			"subject": "Invoice received",
			"body":    "An invoice arrived.",
		},
	})
	if err != nil {
		t.Fatalf("actionSendEmail: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(*sent))
	}
	msg := (*sent)[0]
	if len(msg.To) != 1 || msg.To[0].Email != "someone@example.com" {
		t.Errorf("To = %v, want someone@example.com", msg.To)
	}
	if msg.Subject != "Invoice received" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if msg.Text != "An invoice arrived." {
		t.Errorf("Text = %q", msg.Text)
	}
}

// The `to` param arrives after template substitution, so its value is only as
// trustworthy as the record that triggered the rule — {{sender_email}} is a
// documented recipe, which means whoever can create that record chooses this
// string.
func TestActionSendEmail_RejectsUnusableRecipients(t *testing.T) {
	app, rule := runsApp(t)
	sent := captureEmails(t)

	bad := []struct {
		name string
		to   string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"not an address", "not-an-address"},
		// Header injection: a newline in a recipient would split the header.
		{"embedded newline", "a@example.com\nBcc: victim@example.com"},
		// One templated value must not fan out to many recipients.
		{"address list", "a@example.com, b@example.com"},
	}

	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			err := actionSendEmail(app, ActionRequest{
				Rule:   rule,
				Params: map[string]string{"to": tc.to, "subject": "s", "body": "b"},
			})
			if err == nil {
				t.Errorf("recipient %q was accepted, want rejection", tc.to)
			}
		})
	}

	if len(*sent) != 0 {
		t.Fatalf("sent %d emails despite every recipient being invalid", len(*sent))
	}
}

// "Name <addr>" is a legitimate form a template could produce; the address is
// what reaches the mailer.
func TestActionSendEmail_NormalizesADisplayNameAddress(t *testing.T) {
	app, rule := runsApp(t)
	sent := captureEmails(t)

	err := actionSendEmail(app, ActionRequest{
		Rule:   rule,
		Params: map[string]string{"to": "Ada Lovelace <ada@example.com>", "subject": "s", "body": "b"},
	})
	if err != nil {
		t.Fatalf("actionSendEmail: %v", err)
	}
	if len(*sent) != 1 || (*sent)[0].To[0].Email != "ada@example.com" {
		t.Fatalf("To = %v, want the bare address", (*sent)[0].To)
	}
}

func TestActionSendEmail_EmptySubjectGetsAPlaceholder(t *testing.T) {
	app, rule := runsApp(t)
	sent := captureEmails(t)

	if err := actionSendEmail(app, ActionRequest{
		Rule:   rule,
		Params: map[string]string{"to": "a@example.com", "body": "b"},
	}); err != nil {
		t.Fatalf("actionSendEmail: %v", err)
	}
	if (*sent)[0].Subject != "(no subject)" {
		t.Errorf("Subject = %q, want the placeholder", (*sent)[0].Subject)
	}
}

// The engine's depth cap stops a rule re-triggering itself within one
// dispatch; it cannot see an exchange with another system's auto-responder.
func TestCheckEmailRateLimit(t *testing.T) {
	tests := []struct {
		name       string
		recentRuns int
		age        time.Duration
		wantErr    bool
	}{
		{name: "no history sends", recentRuns: 0, age: time.Minute},
		{name: "under the cap sends", recentRuns: maxEmailsPerRulePerHour - 1, age: time.Minute},
		{name: "at the cap is blocked", recentRuns: maxEmailsPerRulePerHour, age: time.Minute, wantErr: true},
		{
			// A rolling hour, not a lifetime total: yesterday's burst must not
			// stop today's mail.
			name: "old runs fall outside the window", recentRuns: maxEmailsPerRulePerHour + 5,
			age: 2 * time.Hour,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, rule := runsApp(t)
			col, err := app.FindCollectionByNameOrId("rule_runs")
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < tc.recentRuns; i++ {
				r := core.NewRecord(col)
				r.Set("rule", rule.Id)
				r.Set("matched", true)
				r.Set("fired_at", types.NowDateTime().Add(-tc.age))
				if err := app.Save(r); err != nil {
					t.Fatal(err)
				}
			}

			err = checkEmailRateLimit(app, ActionRequest{Rule: rule})
			if tc.wantErr && err == nil {
				t.Error("expected the send to be blocked, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected the send to be allowed, got %v", err)
			}
		})
	}
}

// A manual or scheduled rule with no rule record still sends: there is
// nothing to count against, and refusing would break "run now".
func TestCheckEmailRateLimit_NoRuleIsAllowed(t *testing.T) {
	app, _ := runsApp(t)
	if err := checkEmailRateLimit(app, ActionRequest{}); err != nil {
		t.Errorf("a request with no rule must be allowed: %v", err)
	}
}

// A failing send must surface as an action error so the run is recorded as
// failed and auto-disable can see a rule that never delivers.
func TestActionSendEmail_SurfacesSendFailures(t *testing.T) {
	app, rule := runsApp(t)
	original := sendEmailNow
	sendEmailNow = func(_ context.Context, _ *mailer.Message) error {
		return fmt.Errorf("provider refused")
	}
	t.Cleanup(func() { sendEmailNow = original })

	err := actionSendEmail(app, ActionRequest{
		Rule:   rule,
		Params: map[string]string{"to": "a@example.com", "subject": "s", "body": "b"},
	})
	if err == nil {
		t.Fatal("a provider failure must surface as an action error")
	}
}
