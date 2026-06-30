# tinycld.org/core/mailer

The shared outbound email stack for all TinyCld packages: a provider registry
(Postmark, self-hosted SMTP) selected by configuration, so any package can send
transactional email without depending on the mail feature package.

## Usage

```go
import "tinycld.org/core/mailer"

// Simple transactional email (notifications, invites, etc.)
err := mailer.DefaultSender().Send(ctx, &mailer.Message{
    To:      []mailer.Recipient{{Name: "Holly", Email: "holly@example.com"}},
    Subject: "You've been invited",
    HTML:    "<p>Hello!</p>",
    Text:    "Hello!",
})
```

`DefaultSender()` returns the configured provider, or a log-only sender when
delivery is disabled or nothing is configured. `CanDeliver()` reports whether a
real send would actually go out.

## Configuration

Configuration lives in the **`system_settings`** collection (the `mail.*` keys),
edited from the **/admin → Settings** console — there is no `os.Getenv` in the
runtime read path. The server reads through `mailer.ConfigResolver`, which
`coreserver` points at the in-memory `SystemConfig`. Reads happen per-send, so an
/admin edit applies on the next send without a restart.

| `system_settings` key | Secret | Description |
|---|---|---|
| `mail.provider` | no | `postmark` (default) or `smtp` |
| `mail.postmark_server_token` | yes | Postmark server API token (required to deliver via Postmark) |
| `mail.postmark_account_token` | yes | Postmark account token (domain ops; mail feature) |
| `mail.from_address` | no | Default "From" address. Defaults to `noreply@tinycld.org` |
| `mail.delivery_enabled` | no | `false` logs instead of delivering. Any other value (or unset) delivers in production |
| `mail.smtp_public_hostname` | no | EHLO/MX hostname for the self-hosted SMTP sender |

The mail feature package contributes the **Provider** panel that edits provider
selection, the Postmark/SMTP credentials, and inbound (SMTP/IMAP) config. The
core **Mail — Sending** panel edits the from-address and the delivery switch
(and, in a mail-less assembly, provider + token), so the two never edit the same
key.

## Development

PocketBase processes started with `--dev` (dev/test/seed) **log emails to stdout
instead of delivering**, regardless of `mail.delivery_enabled`, so local and CI
runs never send real mail. Production runs without `--dev` and delivers unless
`mail.delivery_enabled` is `false`.

Logged emails are printed in a formatted box (both `Send` and `SendFull`). When
the `TINYCLD_EMAIL_LOG` env var is set, each logged email is also appended as a
JSON line to that file — the e2e suite reads it to assert on outbound mail.

```
╭──────────────────────────────────────────────────────────╮
│  EMAIL (not delivered — delivery is disabled)       │
├──────────────────────────────────────────────────────────┤
│  To:      Holly Stitt <holly@example.com>
│  Subject: Nathan shared "API Design Proposal" with you
├──────────────────────────────────────────────────────────┤
Hi Holly,

Nathan shared "API Design Proposal" with you.

Open: http://localhost:7100/a/test-org/drive?file=abc123
╰──────────────────────────────────────────────────────────╯
```
