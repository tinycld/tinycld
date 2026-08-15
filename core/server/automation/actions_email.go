package automation

import (
	"context"
	"fmt"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/mailer"
)

// maxEmailsPerRulePerHour caps how much mail one rule may emit.
//
// The engine's depth cap stops a rule re-triggering itself inside one
// dispatch, but it cannot see across dispatches — an auto-reply answering
// another system's auto-responder is two systems each doing one hop, forever.
// A per-rule hourly ceiling bounds that without stopping legitimate bursts.
//
// Mail's send actions cap per MAILBOX because several rules share one outbox;
// core has no mailbox to key on, so the rule is the unit.
const maxEmailsPerRulePerHour = 20

// sendEmailNow is the seam tests replace. Production sends through the same
// core mailer every transactional email uses, so a rule inherits the
// deployment's configured provider rather than opening a second outbound path.
var sendEmailNow = func(ctx context.Context, msg *mailer.Message) error {
	return mailer.DefaultSender().Send(ctx, msg)
}

// registerCoreEmailAction installs core:send-email.
//
// Native, but native IN CORE, which is the whole point: a multi-org tenant
// links no feature package's Go, so mail:send-message is unavailable there
// while this is not. It is the universal "email me when X".
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

	if err := checkEmailRateLimit(app, req); err != nil {
		return fmt.Errorf("core:send-email: %w", err)
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

// checkEmailRateLimit reports whether the rule has room to send.
//
// Counts this rule's matched runs in the last hour from rule_runs — the same
// durable log the run-history UI reads, so a restart doesn't reset the ceiling
// the way an in-memory counter would.
//
// Fails OPEN on a query error: a broken count should not silently stop a
// user's mail. That is the opposite of mail's per-mailbox limiter, which fails
// closed — there, an unresolvable mailbox means an unverifiable self-send,
// which is the loop being guarded. Here there is no self-send to verify, so
// the risk of blocking legitimate mail outweighs one uncounted send.
func checkEmailRateLimit(app core.App, req ActionRequest) error {
	if req.Rule == nil {
		return nil
	}

	// types.DateTime, not an RFC3339 string: PocketBase persists dates as
	// "2006-01-02 15:04:05.000Z" and compares them as text, so an RFC3339
	// literal sorts above every stored value and the filter would silently
	// match nothing — the cap would never engage.
	since := types.NowDateTime().Add(-time.Hour)

	runs, err := app.FindRecordsByFilter(
		"rule_runs",
		"rule = {:rule} && matched = true && fired_at >= {:since}",
		"",
		maxEmailsPerRulePerHour+1,
		0,
		map[string]any{"rule": req.Rule.Id, "since": since},
	)
	if err != nil {
		app.Logger().Warn("automation: email rate-limit check failed, allowing send",
			"rule", req.Rule.Id, "error", err)
		return nil
	}

	if len(runs) >= maxEmailsPerRulePerHour {
		return fmt.Errorf(
			"rule %s reached its hourly email limit (%d) — skipping to break a possible loop",
			req.Rule.Id, maxEmailsPerRulePerHour,
		)
	}
	return nil
}
