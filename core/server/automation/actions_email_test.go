package automation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
// The ledger counts SENDS, not matched runs: an earlier rule_runs-based count
// let a rule with N send actions emit N× the ceiling.
func TestReserveEmailSend(t *testing.T) {
	tests := []struct {
		name      string
		prior     int
		age       time.Duration
		wantBlock bool
	}{
		{name: "no history sends", prior: 0, age: time.Minute},
		{name: "under the cap sends", prior: maxEmailsPerRulePerHour - 1, age: time.Minute},
		{name: "at the cap is blocked", prior: maxEmailsPerRulePerHour, age: time.Minute, wantBlock: true},
		{
			// A rolling hour, not a lifetime total: yesterday's burst must not
			// stop today's mail.
			name: "old sends fall outside the window", prior: maxEmailsPerRulePerHour + 5,
			age: 2 * time.Hour,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(ResetRunStateForTest)
			now := time.Now()
			emailSendLedger.Lock()
			for i := 0; i < tc.prior; i++ {
				emailSendLedger.sends["rule-a"] = append(emailSendLedger.sends["rule-a"], now.Add(-tc.age))
			}
			emailSendLedger.Unlock()

			err := reserveEmailSend("rule-a", now)
			if tc.wantBlock && err == nil {
				t.Error("expected the send to be blocked, got nil")
			}
			if !tc.wantBlock && err != nil {
				t.Errorf("expected the send to be allowed, got %v", err)
			}
		})
	}
}

// The slot is claimed before the send and stays consumed on failure, so the
// in-flight message is inside its own hour (no off-by-one) and a rule whose
// sends all fail still cannot spin: 20 failed attempts exhaust the budget.
func TestReserveEmailSend_FailedSendsStillConsumeBudget(t *testing.T) {
	app, rule := runsApp(t)
	original := sendEmailNow
	sendEmailNow = func(_ context.Context, _ *mailer.Message) error {
		return fmt.Errorf("provider refused")
	}
	t.Cleanup(func() { sendEmailNow = original })

	req := ActionRequest{
		Rule:   rule,
		Params: map[string]string{"to": "a@example.com", "subject": "s", "body": "b"},
	}
	for i := 0; i < maxEmailsPerRulePerHour; i++ {
		if err := actionSendEmail(app, req); err == nil {
			t.Fatalf("send %d: provider failure must surface", i)
		}
	}

	err := actionSendEmail(app, req)
	if err == nil || !strings.Contains(err.Error(), "hourly email limit") {
		t.Fatalf("send %d must be blocked by the ledger, got %v", maxEmailsPerRulePerHour+1, err)
	}
}

// One ledger, many workers: the reservation must hand out exactly the cap.
func TestReserveEmailSend_ConcurrentReservationsRespectTheCap(t *testing.T) {
	t.Cleanup(ResetRunStateForTest)
	now := time.Now()

	var wg sync.WaitGroup
	var allowed atomic.Int32
	for i := 0; i < 2*maxEmailsPerRulePerHour; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if reserveEmailSend("rule-a", now) == nil {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != maxEmailsPerRulePerHour {
		t.Fatalf("allowed %d concurrent sends, want exactly %d", got, maxEmailsPerRulePerHour)
	}
}

// End to end through the handler: the 21st delivery attempt within the hour
// is refused and never reaches the mailer.
func TestActionSendEmail_MetersPerSend(t *testing.T) {
	app, rule := runsApp(t)
	sent := captureEmails(t)

	req := ActionRequest{
		Rule:   rule,
		Params: map[string]string{"to": "a@example.com", "subject": "s", "body": "b"},
	}
	for i := 0; i < maxEmailsPerRulePerHour; i++ {
		if err := actionSendEmail(app, req); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	err := actionSendEmail(app, req)
	if err == nil || !strings.Contains(err.Error(), "hourly email limit") {
		t.Fatalf("want the ledger to block the send, got %v", err)
	}
	if len(*sent) != maxEmailsPerRulePerHour {
		t.Fatalf("mailer saw %d messages, want %d", len(*sent), maxEmailsPerRulePerHour)
	}
}

// A request with no rule record has nothing to count against, and refusing
// would break "run now"-style invocations.
func TestActionSendEmail_NoRuleIsNotMetered(t *testing.T) {
	app, _ := runsApp(t)
	sent := captureEmails(t)

	if err := actionSendEmail(app, ActionRequest{
		Params: map[string]string{"to": "a@example.com", "subject": "s", "body": "b"},
	}); err != nil {
		t.Errorf("a request with no rule must send: %v", err)
	}
	if len(*sent) != 1 {
		t.Fatalf("mailer saw %d messages, want 1", len(*sent))
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
