# Core transactional mailer reads env vars instead of SystemConfig — config refactor

**Status:** Discovered while building the password-reset feature (branch `feat/password-reset`).
Deferred to its own session because it's a cross-cutting config refactor, not part of the
reset feature itself.

## The bug

`core/server/mailer/mailer.go` — the shared transactional mailer used by **invites** and
(now) **password reset** — reads its configuration directly from environment variables,
cached once via `sync.Once`:

- `mailer.go:127` → `os.Getenv("POSTMARK_SERVER_TOKEN")`
- `mailer.go:128` → `os.Getenv("MAIL_FROM_ADDRESS")` (defaults to `noreply@tinycld.org`)
- `mailer.go:116` → `os.Getenv("SKIP_SENDING_MAIL")` (delivery on/off, read in `init()`)
- `mailer.go:189` → `os.Getenv("TINYCLD_EMAIL_LOG")` (test-only log path — fine to leave as env)

This contradicts the app's established configuration model. Per
`core/server/coreserver/system_config.go` and the seed migration
`core/server/pb_migrations/1910000010_create_system_settings.js`:

> "This is the SOLE source of truth at runtime: the server reads values through the
> in-memory SystemConfig … there is no os.Getenv fallback in the read path. Env vars are
> consulted ONCE, by the seed below, to carry an existing env-configured deployment across
> to the DB on the boot that runs this migration."

So Sentry, web-push/VAPID, and the `mail` **feature package** all read their creds from the
`system_settings` collection (via `coreserver.SystemSettings().Get(key)` in Go, or
`systemSetting(app, key)` in the mail package), configurable from the `/admin` Settings
console. The **core transactional mailer is the one subsystem still on env vars** — it can't
be reconfigured from `/admin`, and its config diverges from how the rest of the system works.

### Why it surfaced

The password-reset hook (`core/server/coreserver/password_reset_mailer.go`) added a
`mailer.CanDeliver()` gate (`Default() != nil`, i.e. "is `POSTMARK_SERVER_TOKEN` set in
env?") to decide whether to override PB's reset email or defer to PB's native SMTP. Because
that signal is env-based, it doesn't reflect the real (system_settings-based) config state,
and it routed the e2e/test environment down the wrong branch. The correct fix is not to
patch the gate — it's to make the core mailer read from SystemConfig like everything else.

## Two parallel mail systems (context)

There are two distinct mailers, and this refactor only concerns the first:

| System | Used by | Token source today | "Configured?" signal |
|---|---|---|---|
| **Core `mailer`** (`tinycld.org/core/mailer`) | invites, password reset | env `POSTMARK_SERVER_TOKEN` | `mailer.Default() != nil` |
| **`mail` package `Provider`** (`mail/server/`) | user mailboxes (Gmail-style client) | `system_settings` key `mail.postmark_server_token` (via `systemSetting()`) | `provider.Configured()` |

The `mail` package already does it right (`mail/server/register.go:428` `systemSetting()`,
`:446` `newProviderFromSystem`). The core mailer should follow the same model. Note `mail`
*depends on* `tinycld.org/core/mailer` (builds `PostmarkProvider` on top of
`mailer.PostmarkSender`) — the dependency is one-way; core never imports `mail`.

## Constraint: no import cycle

`mailer` is a standalone Go package that `coreserver` imports. `coreserver` owns
`SystemConfig`. So `mailer` **cannot** import `coreserver` (cycle:
`coreserver → mailer → coreserver`).

**Resolution — invert the dependency.** Have `mailer` expose a config-provider seam (a
package-level function variable, or a small interface) that `coreserver` populates at startup
to read from `SystemConfig`. Sketch:

```go
// in mailer: a settable resolver, default returns zero/empty so the package
// still builds and works standalone (tests, the bootstrap binary).
var ConfigResolver func(key string) string = func(string) string { return "" }

// Default()/DefaultSender()/CanDeliver()/deliver read via ConfigResolver(...)
// instead of os.Getenv(...). Drop the sync.Once caching (or make it re-read on
// SystemConfig OnChange) so an /admin edit takes effect without a restart —
// mirror how the mail package builds its provider per-call.

// in coreserver Register(), after RegisterSystemConfig(app):
mailer.ConfigResolver = func(key string) string { return SystemSettings().Get(key) }
```

`SystemConfig.OnChange` (`system_config.go:83`) already exists for stateful consumers to
re-init on change — use it if any caching remains.

## Open design decisions (resolve in the new session)

1. **Token key sharing.** Should the core mailer read the **same** `mail.postmark_server_token`
   the mail package uses, or a separate `core.*` key? (Leaning: share `mail.postmark_server_token`
   — one token to configure; core reads it via SystemConfig whether or not the mail package is
   installed, since the key lives in the shared `system_settings` collection. But confirm the
   mail package's seed of that key happens even in a mail-less assembly — it may not, in which
   case core needs to own the seed/field.)
2. **Delivery toggle.** Model today's `SKIP_SENDING_MAIL` as an admin-editable
   `mail.delivery_enabled` bool in system_settings (default ON in prod; keep the `--dev`
   auto-skip in code so dev/test still log by default), or derive "deliver" purely from token
   presence (no explicit field).
3. **From address.** Add `mail.from_address` (public) as an `/admin` field, defaulting to
   `noreply@tinycld.org` when unset.
4. **Caching / live reconfig.** Decide whether an `/admin` change should take effect without a
   restart (preferred, matches Sentry/VAPID/mail) — if so, drop `sync.Once` and resolve
   per-send (or wire `OnChange`).

## Work items

- [ ] Add a config-provider seam to `core/server/mailer/mailer.go`; replace the `os.Getenv`
      reads for `POSTMARK_SERVER_TOKEN` / `MAIL_FROM_ADDRESS` / `SKIP_SENDING_MAIL` with it.
      Keep `TINYCLD_EMAIL_LOG` as-is (test infra).
- [ ] Populate the seam from `SystemConfig` in `coreserver.Register()` (after
      `RegisterSystemConfig`). Use `OnChange` if any state is cached.
- [ ] Decide token-key sharing vs. a dedicated core key (decision #1). If core owns keys, add
      them to the seed list in `1910000010_create_system_settings.js` (key → legacy env:
      `POSTMARK_SERVER_TOKEN`, `MAIL_FROM_ADDRESS`) so existing env deployments carry over once.
- [ ] Add a delivery-enabled field + a from-address field (decisions #2/#3) and seed them.
- [ ] Add a **Mail** panel to the `/admin` Settings console
      (`core/components/setup/SettingsTab.tsx`) — mirror `SentrySettings`/`VapidSettings`,
      using `useSystemSettings()` and the write-only `SecretField` for the token. (Note: the
      mail *feature package* already contributes its own provider panel via manifest
      `systemSettings`; this new core panel is for the transactional/core mailer creds — make
      sure the two don't confuse operators. Consider whether they should be unified.)
- [ ] Update `core/server/mailer/README.md` (the env-var table) to point at system_settings.
- [ ] Tests: a Go test that `mailer` reads through the resolver (not env); confirm invite +
      reset e2e still pass once the override path is unconditional.

## Interaction with the password-reset feature (branch `feat/password-reset`)

The reset hook currently gates on `mailer.CanDeliver()` and defers to PB native SMTP when
false. Once this refactor lands, revisit the reset hook:

- The cleanest end state is to **drop the gate** and always override + send via
  `mailer.DefaultSender()` exactly like invites (`invite_lifecycle.go:162`,
  `invite_link.go:235`), since PB's native SMTP isn't configured in this app anyway and the
  invite pattern is the proven one. `DefaultSender()` already falls back to `LogSender` (logs
  in dev/e2e, writes `TINYCLD_EMAIL_LOG`), which is what the reset e2e asserts on.
- The reviewer flagged that the current `CanDeliver()` gate breaks the reset e2e
  (`tests/e2e/password-reset.spec.ts`): in the test env the gate is false → defers to PB
  native SMTP → nothing written to the email log → `waitForEmailTo` times out. Dropping the
  gate fixes this. Remove `mailer.CanDeliver()` (added for this gate) if nothing else uses it.

A smaller secondary review note to fold in: the reset hook mixes `e.App` (settings) and the
captured `app` (logger/send) — pick one consistently.
```
