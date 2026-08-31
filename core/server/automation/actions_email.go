package automation

import (
	"context"
	"fmt"
	netmail "net/mail"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/mailer"
)

// maxEmailsPerRulePerHour caps how much mail one rule may emit.
//
// The engine's depth cap stops a rule re-triggering itself inside one
// dispatch, but it cannot see across dispatches — an auto-reply answering
// another system's auto-responder returns as genuinely new inbound mail at
// depth 0, so two systems each doing one hop can loop forever. This ceiling
// is the only control bounding that exchange.
//
// Mail's send actions cap per MAILBOX because several rules share one outbox;
// core has no mailbox to key on, so the rule is the unit. Per-recipient was
// considered and rejected: a rule replying to N distinct senders would evade
// it, while an address-pair loop always recurs through the same rule.
const maxEmailsPerRulePerHour = 20

// sendEmailNow is the seam tests replace. Production sends through the same
// core mailer every transactional email uses, so a rule inherits the
// deployment's configured provider rather than opening a second outbound path.
var sendEmailNow = func(ctx context.Context, msg *mailer.Message) error {
	return mailer.DefaultSender().Send(ctx, msg)
}

// registerCoreEmailAction installs core:send-email.
//
// Native, but native IN CORE, which is the point: it ships in every build
// regardless of which feature packages an org installed, so an org without
// mail still gets the universal "email me when X".
func registerCoreEmailAction() {
	RegisterAction("core:send-email", actionSendEmail)
}

// actionSendEmail sends one email on behalf of a rule.
//
// Native handlers are not pkgaccess-gated — the engine hands them a superuser
// app — so this validates its own inputs. The `to` param arrives after
// template substitution, meaning its value is only as trustworthy as the
// record that triggered the rule: {{sender_email}} is a documented recipe, so
// whoever can create that record chooses this string.
func actionSendEmail(app core.App, req ActionRequest) error {
	recipient, err := emailRecipient(req.Params["to"])
	if err != nil {
		return fmt.Errorf("core:send-email: %w", err)
	}

	// A request with no rule (a dry-run style invocation) has nothing to
	// count against; every engine path supplies the rule.
	if req.Rule != nil {
		if err := reserveEmailSend(req.Rule.Id, time.Now()); err != nil {
			return fmt.Errorf("core:send-email: %w", err)
		}
	}

	subject := strings.TrimSpace(req.Params["subject"])
	if subject == "" {
		subject = "(no subject)"
	}

	msg := &mailer.Message{
		To:      []mailer.Recipient{{Email: recipient}},
		Subject: subject,
		Text:    req.Params["body"],
	}

	if err := sendEmailNow(context.Background(), msg); err != nil {
		return fmt.Errorf("core:send-email: %w", err)
	}
	return nil
}

// emailRecipient validates the rule-supplied address.
//
// ParseAddress rejects the header-injection shapes a raw string could carry
// (embedded newlines, a bare comma-separated list) and normalizes a
// "Name <addr>" form down to the address. Exactly one recipient: a list here
// would let one templated value fan out to arbitrarily many.
func emailRecipient(to string) (string, error) {
	raw := strings.TrimSpace(to)
	if raw == "" {
		return "", fmt.Errorf("no recipient")
	}
	parsed, err := netmail.ParseAddress(raw)
	if err != nil {
		return "", fmt.Errorf("invalid recipient %q: %w", raw, err)
	}
	return parsed.Address, nil
}

// emailSendLedger tracks each rule's sends inside the rolling hour, in memory.
//
// In-memory is deliberate (the failureStreaks precedent): there is no query
// that can fail, so the cap cannot fail open — this ceiling is the only
// control bounding a cross-dispatch mail loop, and an unavailable count must
// never be read as permission to send. It also cannot be pruned out from
// under itself the way a rule_runs-based count was (pruneRuns deletes the
// rows such a count relies on).
//
// A new process starts with an empty ledger. That is acceptable because the
// processes involved do not recycle mid-loop: a single-tenant server restarts
// when an operator says so, and a hosting tenant is evicted only after 30
// idle minutes with zero connections — a state an active mail loop never
// reaches.
var emailSendLedger = struct {
	sync.Mutex
	sends map[string][]time.Time
}{sends: map[string][]time.Time{}}

// reserveEmailSend claims one send slot for the rule, or reports the rule is
// over its hourly ceiling.
//
// The slot is claimed BEFORE the send so the in-flight message is part of its
// own hour (a rule with several send actions is metered per message, and a
// concurrent dispatch cannot slip past the ceiling). A claimed slot stays
// consumed even if the send then fails or is abandoned by the action timeout
// — over-counting is the safe direction for a loop guard.
func reserveEmailSend(ruleID string, now time.Time) error {
	emailSendLedger.Lock()
	defer emailSendLedger.Unlock()

	cutoff := now.Add(-time.Hour)
	kept := emailSendLedger.sends[ruleID][:0]
	for _, t := range emailSendLedger.sends[ruleID] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= maxEmailsPerRulePerHour {
		emailSendLedger.sends[ruleID] = kept
		return fmt.Errorf(
			"rule %s reached its hourly email limit (%d) — skipping to break a possible loop",
			ruleID, maxEmailsPerRulePerHour,
		)
	}
	emailSendLedger.sends[ruleID] = append(kept, now)
	return nil
}
