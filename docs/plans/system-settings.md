# Plan: System Settings (move "use-your-own-service" env vars into /admin)

## Goal

Let an operator configure **system-wide** service credentials/config from the
`/admin` console instead of environment variables — so a third-party host doesn't
ship our Sentry project, our web-push identity, or our mail provider.

"System settings" = settings that apply to the **entire system**, not a single org
(contrast the existing **org settings** under `/a/[orgSlug]/settings`).

Motivating case: **Sentry DSN**. The same mechanism covers **VAPID** (web-push
keypair, core) and **mail** credentials (Postmark/SMTP/IMAP, the `mail` package).

**Env var support is REMOVED entirely** — `system_settings` is the sole source of
truth (no `os.Getenv` fallback). A live in-memory `SystemConfig` struct holds the
current values and re-initializes stateful consumers (Sentry today) when a value
changes, so edits take effect without a restart.

The /admin Settings section is **package-extensible** — built with the exact same
manifest + lazy-component pattern that org `settings` use, so `mail` contributes its
own system-settings panel instead of core hard-coding it.

## Scope of vars (triaged from a full env sweep)

**In scope — system-specific service config a host must own:**
- Sentry: `SENTRY_DSN` (client+server), `SENTRY_ORG`, `SENTRY_PROJECT`, `SENTRY_AUTH_TOKEN` *(secret)* — core
- VAPID web-push: `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` *(secret)*, `VAPID_SUBJECT` — core
- Mail: `MAIL_PROVIDER`, `POSTMARK_SERVER_TOKEN` *(secret)*, `POSTMARK_ACCOUNT_TOKEN` *(secret)*,
  `MAIL_FROM_ADDRESS`, `SMTP_IMAP_USERNAME`, `SMTP_IMAP_PASSWORD` *(secret)*, `SMTP_DKIM_SELECTOR`, `SMTP_DOMAIN` — **mail package**

**Out of scope (stay env) — infra/topology/build/test:** listen addresses + ports +
enable flags (`SMTP_ADDR`, `IMAP_ADDR`, `SMTP_ENABLED`, `SMTP_INBOUND_MODE`, ...),
`TINYCLD_PUBLIC_URL`, `TINYCLD_STATE_DIR`, TLS/domain vars, `ENVIRONMENT`,
`EXPO_PUBLIC_ENV`, all `PW_*`/`TEST_*`/`SEED_*`/`RUN_*`, demo/dev toggles. These are
set by the host's process manager and often differ per replica; they're not credentials.

**Migration of existing deployments:** since env support is removed, the
`system_settings` migration performs a **one-time seed**: for each in-scope key, if
the row is absent, read the legacy env var and write it in. After that the env var is
never read again. This carries running deployments across without manual re-entry.
(A deployment that relied purely on env keeps working; the values just move into the DB.)

## Key facts the design rests on (verified)

- Client Sentry DSN is currently **hardcoded** in `lib/app-config.ts`, read
  synchronously at module-init (`configure-core` → `initSentry`), NOT at runtime.
- Web `app.html` is **read from disk and written per-request** by the Go server
  (`static.go:211-218`) → we can inject a `<script>` into `<head>` at serve time;
  a `window.__` global set there is available before the deferred bundle JS runs
  (precedent: `window.location.origin`, `globalThis.__TINYCLD_*`).
- Native has no `window`/HTML and inits Sentry before connecting to a server →
  native uses the **build-time** path (`EXPO_PUBLIC_SENTRY_DSN` injected by the
  rebuild) with the current hardcoded value as the floor.
- `app.Settings().Meta` is a fixed PB struct → use a **dedicated collection**.
- Rebuild injects per-platform env at `runExportWithProgress` (`rebuild_pipeline.go:268`).
- Server Sentry inits once at startup (`sentry.go`); a DSN change needs a restart to
  affect the SERVER's own capture (the web client picks it up on next load).

## Package extensibility — mirror org `settings` exactly

Org settings flow: manifest `settings: [{slug,component,label}]` → `describe-packages.ts`
maps it → `gen-config.ts` emits `Component: lazy(() => import('<pkg>/<component>'))`
→ `derive-components.ts` `deriveSettings()` groups by package → the settings hub
maps `packageSettings` and a `[...section]` route renders the lazy component.
Component strings resolve via each package's `exports` map (`"./settings/*"`).

We replicate this for system settings with a new manifest field:

```ts
systemSettings?: { slug: string; component: string; label: string }[]
```

- Core itself contributes the **Sentry** (and later VAPID) panel via the synthetic
  `admin`/core entry (or a small core-owned registration mirroring `builtin-admin.ts`).
- `mail` contributes its mail-provider panel: `systemSettings: [{ slug:'provider',
  label:'Mail Provider', component:'system-settings/provider' }]` + an exports entry
  `"./system-settings/*": "./tinycld/mail/system-settings/*.tsx"`.
- New runtime export `packageSystemSettings = deriveSystemSettings(tinycldConfig)`.
- No slot/target validation needed (flat list, like org settings — not the
  `slots`/`sidebarContributions` cross-package mechanism).

## Data model

Core collection `system_settings` (core migration):
- `key` (text, unique) — `sentry.dsn`, `vapid.private_key`, `mail.postmark_server_token`, ...
- `value` (text), `is_secret` (bool), `updated_by` (rel users, optional), autodate created/updated
- Rules: super-admin OR superuser for all ops (reuse `@collection.super_admins...`
  clause from `1910000007_*super_admin_rules`). Secret rows never surface to clients
  (see API). Registered as a pbtsdb store in `core/lib/pocketbase.ts`.
- **Migration seeds from legacy env once** (see "Migration of existing deployments"):
  for each known key with no row, write `os.Getenv(<legacy var>)` if non-empty. This is
  the ONLY place env vars are ever read; the runtime path never reads env.

## Server: SystemConfig struct (single source of truth + re-init on change)

`core/server/coreserver/system_config.go`:

```go
type SystemConfig struct {
    mu     sync.RWMutex
    app    core.App
    values map[string]string // key → value, loaded from system_settings
}

func (c *SystemConfig) Get(key string) string          // RLock; pure read of current value
func (c *SystemConfig) load()                           // (re)read all rows from the DB into values
func (c *SystemConfig) onChanged(key, value string)     // update map + fire reinit hooks
```

- One instance constructed in `Register()` (server.go), held on the app (or a package
  var like other coreserver singletons). `load()` runs once at boot, AFTER migrations.
- **No `os.Getenv`** anywhere in the read path — `Get` returns only the stored value.
- A PB hook re-syncs on edit: `OnRecordAfterCreateSuccess("system_settings")` +
  `OnRecordAfterUpdateSuccess("system_settings")` (precedent: `invite_lifecycle.go:32`)
  → `onChanged(key, value)`.

**Re-init policy by consumer (driven by how each currently reads its config):**
- **Sentry** — holds a stateful global client `sentry.Init`'d once at boot. `onChanged`
  for any `sentry.*` key calls a new `reinitSentry(cfg)` that re-runs `sentry.Init`
  with the new DSN/environment. (This is the only true re-init.)
- **Push** — `push.SendToUser` reads VAPID **per send** (push.go:47-49), so it just
  reads `cfg.Get("vapid.*")` at send time and picks up changes with no re-init.
- **Mail** — the provider is built **per call** (`providerForOrg` / `newProviderFromEnv`,
  register.go:127), so it reads `cfg.Get("mail.*")` when constructing and needs no re-init.

So `SystemConfig` notifies exactly one stateful consumer today (Sentry); push/mail are
naturally live because they read per-use. The struct still centralizes all reads.

## Read-site changes (remove env)

- `sentry.go`: `Dsn: cfg.Get("sentry.dsn")` (was `os.Getenv("SENTRY_DSN")`); done in
  phase 3 via `initSentryFromConfig`, re-run on any `sentry.*` change. **Done.**
- Sourcemap upload (`app_native_export.go`: `SENTRY_AUTH_TOKEN`/`SENTRY_ORG`/
  `SENTRY_PROJECT`) — these feed the `sentry-cli` SUBPROCESS env, not just a Go read,
  so they move in **phase 5** (build-env injection) alongside the native build vars,
  not phase 3.
- `push/push.go`: VAPID trio via `cfg.Get` (drop the three `os.Getenv`).
- `mail` package `register.go`: replace the env fallbacks in `providerForOrg` /
  `newProviderFromEnv` / `smtpConfigFromEnv` with `cfg.Get("mail.*")`. **Layering note:**
  mail already reads **org** settings first, then env. New order: org settings →
  **system settings** (was env). The mail package reads the shared `system_settings`
  collection (it's a sibling repo; it reads via PB/core, not a core Go import).

## Secrets handling

Per-key `is_secret`:
- **Non-secret** (Sentry DSN — already public/in-bundle; `VAPID_PUBLIC_KEY`;
  `MAIL_FROM_ADDRESS`; `SENTRY_ORG`/`PROJECT`): stored plain, may be injected into web HTML / build env.
- **Secret** (`SENTRY_AUTH_TOKEN`, `VAPID_PRIVATE_KEY`, `POSTMARK_*`,
  `SMTP_IMAP_PASSWORD`): **write-only** in the UI (shows set/not-set + replace field),
  never returned to non-admins, never injected client-side; server reads from DB.
- Encryption-at-rest deferred (PB DB is the existing trust boundary); noted as future hardening.

## Web client: runtime DSN via window global

1. Server injects into `app.html` `<head>` a `<script>` setting
   `window.__TINYCLD_PUBLIC_CONFIG__ = { sentryDsn: <non-secret value> }`, built from
   **non-secret** system settings only.
2. `lib/app-config.ts` `sentryDsn` =
   `globalThis.__TINYCLD_PUBLIC_CONFIG__?.sentryDsn` (web, runtime-injected)
   ?? `process.env.EXPO_PUBLIC_SENTRY_DSN` (native; inlined at build from the stored
   value — see "Native client"). No hardcoded DSN remains in the source.
   Declared in a `declare global` block.
3. Changing the DSN in /admin takes effect on the next web page load — no rebuild.

Note: `EXPO_PUBLIC_SENTRY_DSN` here is a *build input* (read at `expo export`, inlined),
not a runtime env read — consistent with "no runtime env." Native reflects a change
only after the next rebuild/OTA.

**Done (phase 4):** server injection + `injectPublicConfig` (non-secret only, before
`</head>`, `</script>`-escaped); `app-config.ts` resolves `window → EXPO_PUBLIC_SENTRY_DSN
→ ''` with **no hardcoded DSN**. Because the official EAS native build previously
depended on that hardcode, `eas.json`'s production profile now sets
`EXPO_PUBLIC_SENTRY_DSN` so the official app keeps reporting; a third-party native
build is silent until they set their own. **Release-note this** (behavior change for
self-hosters relying on the env var / on our DSN).

## Native client: build-time inject

`rebuild_pipeline.go::runExportWithProgress`: for native platforms add
`EXPO_PUBLIC_SENTRY_DSN=<SettingValue(app,'sentry.dsn')>` to the scoped export env.
Picked up on the next package rebuild / OTA. Web export doesn't need it.

**Done (phase 5):** native export injects `EXPO_PUBLIC_SENTRY_DSN` from
`systemConfig.Get("sentry.dsn")` (web omitted — runtime-injected). The sourcemap
upload (`uploadBundleSourcemaps`) now reads `sentry.auth_token`/`org`/`project`
from `systemConfig` and passes the token to the `sentry-cli` subprocess via a new
`runCmdEnv` (env, not a logged arg — keeps the secret out of build logs). The old
`envOr` env helper is gone (replaced by the pure `orDefault`). No `os.Getenv("SENTRY_*")`
remains anywhere in coreserver.

## /admin Settings section

- `SetupDashboard.tsx`: add `'settings'` to the `SetupTab` union, a NAV entry
  `{ tab:'settings', label:'Settings', crumb:'settings', Icon: Settings }`, and render
  `<SettingsTab isVisible={activeTab==='settings'} pb={pb} />`.
- `core/components/setup/SettingsTab.tsx`: lists `packageSystemSettings` groups (core's
  Sentry/VAPID + mail's panel) and renders the selected lazy panel — same discovery
  pattern as the org settings hub, but system-scoped (no org filter). Panels read/write
  the `system_settings` store via `useStore` + `useMutation` (console runs as a
  super-admin app user, so writes are authorized).

**Done (phase 6):** `SetupDashboard` has a Settings nav entry + `SettingsTab`.
`SettingsTab` renders a core-owned **Sentry** panel (DSN field; upserts `sentry.dsn`
into the `system_settings` store via `useStore`+`useMutation`, keyed by row, using
RHF `values:` for reactive seeding — no `useEffect`) plus any package-contributed
`packageSystemSettings` panels (lazy, in Suspense). The panel is non-secret; secret
fields get the write-only treatment when VAPID/mail land in phase 7. End-to-end
visual verification (UI → store → server load → web injection) still wants a fresh
image build — deferred.

## Phases

1. **Storage + SystemConfig** — `system_settings` collection + migration (incl. one-time
   seed-from-env) + super-admin rules; register pbtsdb store; `SystemConfig` struct with
   `load()` at boot + the `OnRecordAfter{Create,Update}Success("system_settings")` →
   `onChanged` hook. Go unit tests (Get reads current value; onChanged updates it).
2. **Package mechanism** — manifest `systemSettings` field; `describe-packages.ts` +
   `gen-config.ts` emission; `deriveSystemSettings` + `packageSystemSettings` export;
   generator/derive unit tests.
3. **Server read site (Sentry) + re-init** — `sentry.go` reads `cfg.Get("sentry.dsn")`;
   add `reinitSentry`; wire it into `onChanged` for `sentry.*` keys. Remove `os.Getenv`.
4. **Web window injection** — inject non-secret config into `app.html`; `app-config.ts` reads it.
5. **Native build inject** — feed `cfg.Get("sentry.dsn")` as `EXPO_PUBLIC_SENTRY_DSN` to the native export.
6. **/admin Settings UI** — NAV + `SettingsTab`; core contributes the Sentry panel.
7. **Follow-ups** — VAPID panel + read site (core; per-send, no re-init) and mail panel +
   read sites (mail package; per-call, no re-init), reusing 1–6. Remove their `os.Getenv`.

**Done (phase 7):**
- **VAPID (core):** `push/push.go` reads `vapid.*` from `system_settings` directly off
  `app` (no coreserver import → no cycle), per-send. `SettingsTab` gained a VAPID panel
  with a reusable write-only `SecretField` (private key never seeded back into the form;
  blank submit = unchanged). Shared `useSystemSettings` hook (`system-settings-store.ts`)
  backs both core panels.
- **Mail (sibling repo):** `register.go` provider/SMTP/IMAP-credential reads now layer
  org `settings` → `system_settings` (`mail.*`) → default; the env fallback is gone
  (`smtpConfigFromEnv`→`smtpConfigFromSystem(app)`, `newProviderFromEnv`→
  `newProviderFromSystem(app)`, threaded `app` through `smtpConfigFromSettings`/imap
  fetcher). Mail contributes a `system-settings/provider` panel (provider + write-only
  Postmark tokens) via its manifest + a `./system-settings/*` exports entry.
- **Out of scope (stay env, confirmed):** mail's listen-address/port/enable-flag vars
  (`SMTP_ADDR`, `IMAP_ENABLED`, `SMTP_DOMAIN`, `SMTP_INBOUND_ADDR`, …) — host topology.
- **Two repos:** core (`tinycld`) and `mail` are separate git repos → two commits,
  coordinated release. No migrated `os.Getenv("MAIL_*"/"POSTMARK_*"/"SMTP_IMAP_*")`,
  `VAPID_*`, or `SENTRY_*` reads remain.
- **Secrets:** front-end obfuscation only (write-only fields); the value still reaches
  the super-admin client over the wire (existing trust model). NOT injected into public
  web HTML. (Server-side redaction was considered and declined.)

## Tests

- Go: `SystemConfig.Get` returns the stored value; `onChanged` updates it live;
  `reinitSentry` runs on a `sentry.dsn` change; migration seeds from env once; collection
  rules (super-admin write, anon denied).
- Generator/derive: a manifest with `systemSettings` produces the grouped registry.
- E2E (extend first-boot install spec): set a Sentry DSN in /admin Settings → reload web →
  assert injected `window.__TINYCLD_PUBLIC_CONFIG__.sentryDsn` matches AND secret keys absent from HTML.
- Unit: `app-config.ts` DSN resolution order (window > native-build env input).

## Resolved decisions

- **No env fallback at runtime** — `system_settings` is the sole source; env is read
  only by the one-time migration seed.
- **Live re-init on change** via `SystemConfig.onChanged` — Sentry re-inits; push/mail
  read per-use so they're already live. No restart required.

## End-to-end verification (done)

`tests/install/run-first-boot-admin.sh` (new smoketest runner — boots a fresh-DB
image, scrapes the setup token, runs `setup-and-packages.spec.ts`) passes all 5
specs on an image built from this branch **after rebasing onto main (post-#82)**:
bootstrap → lists bundled packages → create org → grant super-admin → **configure
system settings** (save Sentry DSN → reload → `window.__TINYCLD_PUBLIC_CONFIG__.
sentryDsn` injection confirmed → Generate VAPID → "Configured ✓").

Dependency note: this branch required PR #82's `appPb`-auth fix (the console now
runs authenticated) — before the rebase, the package list and the Settings-panel
store reads came back empty because `appPb` was anonymous. #82 merged; rebased in.

## Upgrade notes — BREAKING for self-hosters

Service config is no longer read from environment variables at runtime; it lives in
**/admin → Settings** (the `system_settings` collection). The collection migration
performs a **one-time seed from the legacy env vars** on the first boot that runs
it, so an existing env-configured deployment carries over automatically. After
that, env changes have no effect — edit the values in /admin.

Things that change on upgrade:
- **Sentry** (`SENTRY_DSN` etc.): server + web read it from settings; the hardcoded
  client DSN is gone. The official EAS build supplies it via `eas.json`. A
  **third-party native rebuild reports nowhere until they set their own DSN** in
  /admin (web picks up a change on next load; native on next rebuild/OTA).
- **VAPID** (`VAPID_*`): web-push keys come from settings — use the "Generate
  keypair" button or paste an existing pair. An unconfigured deployment sends no push.
- **Mail** (`MAIL_PROVIDER`/`POSTMARK_*`/`SMTP_IMAP_*`): provider + credentials are
  now **system-wide** (Settings → Mail), not per-org. Org mail settings keep only
  domain management. The seed migrates env values; otherwise configure in /admin.
- **NOT changed** (still env): listen addresses/ports/enable-flags (`SMTP_ADDR`,
  `IMAP_ENABLED`, `SMTP_DOMAIN`, …), `TINYCLD_PUBLIC_URL`, TLS/domain vars — host
  topology, set by the process manager.

For the release: tag the system-settings commits as a breaking change in the notes.
