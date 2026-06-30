# Unified core mailer — consolidate email sending into core

**Status:** Plan. Supersedes and absorbs `docs/plans/mailer-config-from-system-settings.md`
(that doc's narrower "core mailer reads env instead of SystemConfig" bug is a subset of this).
Discovered while building password-reset (`feat/password-reset`); the reset-hook fix rides along here.

## Goal & motivation

Today there are **two parallel mailer stacks**, and more consumers are coming
(calendar invites, a future SES provider):

| Stack | Used by | Providers | Config source | Live reconfig? |
|---|---|---|---|---|
| Core `mailer` (`tinycld.org/core/mailer`) | invites, password-reset, drive shares | Postmark only | env (`sync.Once` + `init()`) | no |
| Mail `Provider` (`mail/server/`) | user mailboxes | Postmark + self-hosted SMTP | `system_settings` (`mail.*`, per-use) | yes (record hooks) |

This is divergent and duplicative. **The fix is one outbound abstraction in core**, sourced
from `system_settings`, that every transactional consumer and the mail product share — so
calendar gets a sender for free and SES plugs in once.

### Confirmed decisions

1. **Move SMTP-outbound into core.** Core becomes a real 2-provider registry (postmark +
   smtp). The self-hosted direct-MX sender + RFC-5322 builder move out of the mail package
   into core, routed through the deliver gate. Mail keeps its **inbound** SMTP/IMAP server,
   bounce/domain/DKIM, and IMAP fetcher.
2. **Shared `mail.*` namespace.** Core reads the existing `mail.provider` /
   `mail.postmark_server_token` keys, plus new `mail.from_address` and `mail.delivery_enabled`.
   **No data migration** — reuse the 13 keys the mail product already defines.
3. **Admin panel only, no env seed.** Configure from `/admin` going forward; no env carry-over
   migration. Existing env-configured deployments re-enter the token once.
4. **Scope = config refactor + reset-hook fix + a shared template layer.** Calendar email and
   SES are future work *enabled by* this seam, not wired here.

## The seam (what moves, what stays)

The mail package already drew the line: `Send` + `Configured` are the **outbound/shared**
half; `ParseInbound` / `ParseBounce` / `AddDomain` / `CheckDomainVerification` /
`CheckInboundDomain` / `VerifyWebhookSignature` are **mailbox-specific**.

```
MOVES INTO core/server/mailer/:
  - Provider abstraction: a SendProvider interface (Send + Configured) + a registry keyed by
    `mail.provider` ("postmark" | "smtp"), read per-send from system_settings.
  - PostmarkProvider's SENDING (already mostly here via PostmarkSender).
  - SMTP-outbound: smtp_provider.go (Send + MX/dial), smtp_outbound_build.go (RFC-5322),
    SMTPConfig's OUTBOUND fields (PublicHostname, OutboundTimeout, DKIMSelector for signing later),
    generateMessageID, envelopeAddress, groupRecipientsByDomain, resolveMXHosts,
    runSMTPConversation, isPermanentSMTPError. Keep the swappable smtpDial/smtpMXLookup
    test hooks (move the seam, keep it injectable).
  - Config resolver: read mail.provider / mail.postmark_server_token / mail.postmark_account_token /
    mail.from_address / mail.delivery_enabled / mail.smtp_public_hostname from system_settings.

STAYS in mail/server/:
  - Inbound SMTP MX listener (smtp_inbound_server.go), submission server + sessions
    (smtp_server.go, smtp_session.go, auth.go), the whole IMAP server + fetcher,
    domain/DKIM verification, bounce/inbound parsing, webhook routing (resolveWebhookProvider),
    settings-cache invalidation + IMAP reconcile hooks.
  - Mail's Provider interface stays (it still needs the mailbox methods); its Send/Configured
    now delegate to the core SendProvider rather than re-implementing. The SMTPConfig inbound
    fields (IMAP*, InboundMode) stay in mail.
```

Mail's `Provider` continues to exist for the mailbox product; its `Send` becomes a thin
delegate to the core provider built from the same `mail.*` keys (exactly as `PostmarkProvider.Send`
already delegates to `mailer.PostmarkSender.SendFull` today). The inbound/SMTP-listener code that
shares `parseRFC5322` with outbound build keeps a copy of (or imports) the shared parser — confirm
the inbound parser dependency when splitting `smtp_provider.go` (it reuses `parseRFC5322`).

## Config keys (all `mail.*`, in `system_settings`)

Outbound (read by core):

| Key | Secret | Meaning | Default |
|---|---|---|---|
| `mail.provider` | no | `postmark` \| `smtp` | `postmark` |
| `mail.postmark_server_token` | yes | Postmark server token | — |
| `mail.postmark_account_token` | yes | Postmark account token (domain ops) | — |
| `mail.from_address` | no | default From | `noreply@tinycld.org` |
| `mail.delivery_enabled` | no | master deliver switch | ON (prod) |
| `mail.smtp_public_hostname` | no | SMTP EHLO/MX host | `localhost` |

Inbound / mailbox-only (stay read by mail): `mail.smtp_inbound_mode`, `mail.smtp_imap_*`,
`mail.smtp_dkim_selector`. No new keys for these.

## Delivery gating (replacing `sync.Once` + `init()` `deliver` bool)

Resolve per-send (matches how mail builds its provider per-call, enables live `/admin` reconfig):

```
deliver = devAutoLog ? false : delivery_enabled_from_system_settings(default true)
```

- Keep the **`--dev` auto-log** so dev/test/seed/e2e log by default with zero config (today's
  behavior). `--dev` detection stays via `os.Args` at startup (it's a process property, fine to
  read once).
- `mail.delivery_enabled` is the admin-editable master switch (default ON; an operator can pause
  delivery from `/admin` without unsetting the token).
- **Drop `sync.Once`**: build the sender per-send (or per a cheap cache invalidated by a
  `system_settings` `mail.*` record hook). Per-send construction is the simplest and matches the
  mail product — no caching needed.
- **`TINYCLD_EMAIL_LOG` JSONL path is unchanged** — the e2e helpers
  (`tests/e2e/email-log-helpers.ts`) read it and the reset spec asserts on it. `LogSender` stays
  exactly as-is.
- Retire `SKIP_SENDING_MAIL` / `POSTMARK_SERVER_TOKEN` / `MAIL_FROM_ADDRESS` env reads from the
  runtime path. (No env seed — decision #3. Document the change in the README; existing
  deployments reconfigure via `/admin`.)

## Admin UI

- **Core contributes its own outbound "Mail" panel** to `/admin` Settings, always present
  (transactional mail is core, must be configurable even in a mail-less assembly). Mirror
  `SentrySettings`/`VapidSettings` in `core/components/setup/SettingsTab.tsx`: `useSystemSettings()`,
  `SecretField` (write-only) for the token, plain fields for provider / from-address /
  delivery-enabled / smtp public hostname. Add schemas to `system-settings-logic.ts` with unit
  tests in `__tests__/`.
- **The mail product's existing "Provider" panel** (`mail/.../system-settings/provider.tsx`) keeps
  owning the **mailbox-only** keys (account token, SMTP/IMAP inbound, DKIM). Both share the `mail.*`
  namespace but manage **disjoint** keys.
- **Avoid operator confusion:** core panel label = "Mail — Sending (transactional)"; mail panel
  stays "Mail — Provider" (mailbox/inbound). Today both can write `mail.provider` /
  `mail.postmark_server_token`; make the core panel own those outbound keys and have the mail panel
  defer to / cross-link them (or render them read-only with a "configured in core Mail settings"
  note) so there's a single source of truth for the shared keys. Resolve the exact ownership split
  during implementation; the rule is **each shared key is editable in exactly one panel.**

## Reset-hook fix (folds in from the password-reset branch)

- **Drop the `mailer.CanDeliver()` gate** in `password_reset_mailer.go`. Always override + send
  via the core sender exactly like invites (`invite_lifecycle.go` `send()` /
  `invite_link.go:sendInviteEmailTo`). PB native SMTP isn't configured in this app; the invite
  pattern is proven; `DefaultSender()` already degrades to `LogSender` (logs in dev/e2e, writes
  `TINYCLD_EMAIL_LOG`) — which is what `tests/e2e/password-reset.spec.ts` asserts on. This fixes the
  e2e timeout the reviewer flagged (gate false in test env → deferred to PB → nothing logged).
- Remove `mailer.CanDeliver()` if nothing else uses it after the change (grep: only the reset hook
  references it).
- **Fix the `e.App` vs captured `app` wart:** `password_reset_mailer.go:47` reads `e.App.Settings().Meta`
  while lines 41/50 use the captured `app`. Switch line 47 to `app.Settings().Meta` for consistency
  with every sibling registrar.

## Shared template layer

Four sites hand-roll near-identical HTML+text (invite-link, invite-lifecycle, password-reset,
drive share/share-otp). Extract a small shared template helper in core (the invite/reset emails
already share `buildInviteEmailHTML`/`buildPasswordResetEmailHTML`, `greeting`, `htmlEscape`,
`brandColor` — generalize into one `RenderTransactionalEmail({appName, greeting, body, ctaLabel,
ctaLink})` returning `(html, text)`). Migrate the 4 core sites; offer drive the same helper (its 3
sites duplicate the markup too). Keep the diff focused: helper + call-site swaps, no behavior change
to the rendered output beyond consolidation.

## Work items

**Core mailer package (`core/server/mailer/`)**
- [ ] Introduce a `SendProvider` interface (`Send`/`SendFull` + `Configured`) and a registry
      selected by `mail.provider`. Keep `Sender`/`FullSender`/`Message`/`SendRequest`/`LogSender`/
      `NoopSender` as-is (they're the I/O contract).
- [ ] Move SMTP-outbound from `mail/server/` (`smtp_provider.go` send path,
      `smtp_outbound_build.go`, outbound `SMTPConfig` fields, MX/dial/conversation helpers, test
      hooks). Route it through the `deliver` gate (it currently ignores it — real fix).
- [ ] Replace `sync.Once`/`init()` env reads with a per-send resolver over `system_settings`
      (provider, token, account token, from, delivery_enabled, smtp public hostname). Drop
      `Default()`/`CanDeliver()` env semantics; keep `DefaultSender()` returning a `Sender` that
      logs when delivery is off / unconfigured.
- [ ] Inject the resolver from `coreserver`: set it inside `RegisterSystemConfig` (it owns the
      config lifecycle + `OnServe` load). The resolver closes over `systemConfig.Get` and reads
      lazily, so registration order is safe (config is empty pre-`OnServe`, populated after).
- [ ] Fix `defaultFrom` asymmetry (`Send` applies it, `SendFull` doesn't) while here.
- [ ] Clean doc drift: dead `DELIVER_MAIL` comments, stale `tinycld/mailer` import in README,
      document `TINYCLD_EMAIL_LOG`.

**Mail package (`mail/server/`)**
- [ ] Repoint mail's `Provider.Send`/`Configured` at the core `SendProvider` (delegate, don't
      re-implement). Keep the mailbox-specific interface methods + inbound/IMAP/domain code.
- [ ] Remove the now-moved outbound SMTP files; keep inbound SMTP/IMAP and the shared
      `parseRFC5322` (decide import-from-core vs keep-local for the parser).
- [ ] Resolve panel ownership of the shared `mail.*` outbound keys (see Admin UI).

**Core transactional consumers**
- [ ] Drop the reset-hook `CanDeliver()` gate; fix `e.App`/`app`; remove `CanDeliver()` if unused.
- [ ] Extract `RenderTransactionalEmail`; migrate invite-link, invite-lifecycle, password-reset
      (and drive's 3 sites) onto it.

**Admin / config**
- [ ] Add the core "Mail — Sending" panel + `mail.from_address` / `mail.delivery_enabled` /
      provider / token fields; schemas + unit tests in `setup/__tests__/`.
- [ ] Update `core/server/mailer/README.md`: env-var table → `system_settings` + `/admin`.

**Tests**
- [ ] Go: a mailer test that sending reads provider/token/from/delivery through the resolver (not
      env); move/keep the SMTP provider tests (`smtp_provider_test.go`) alongside the moved code.
- [ ] e2e: confirm invite + password-reset specs pass with the gate dropped (the reset spec relies
      on the override always firing → `LogSender` → `TINYCLD_EMAIL_LOG`).

## Risks & notes

- **SMTP-outbound move is the largest, riskiest piece** — it pulls real network code (MX lookup,
  dial, STARTTLS) across a package boundary and couples to the inbound parser. Land it behind the
  test hooks; keep `smtp_provider_test.go` green through the move. Consider doing the move as the
  first, isolated commit (pure relocation, no behavior change) before wiring config.
- **Two reconcile mechanisms** (core `SystemConfig.OnChange`; mail's PB record hooks on
  `system_settings`). The core mailer reading per-send needs neither; if any caching is added, use a
  `mail.*`-prefixed record hook (mirror mail's `reconcileOnSystemMail`) rather than `OnChange`.
- **Shared-key ownership** between the two panels is the subtle UX trap (decision #2 chose to share
  `mail.*`). Enforce single-editor-per-key.
- **Calendar** is the first future consumer of the new seam (invites/reminders today only emit
  in-app `notify` records). Out of scope here, but the abstraction should make adding it trivial.
- **SES** plugs into the registry as a third provider keyed by `mail.provider` = `ses` — no further
  structural change once the registry exists.
```
