# First-run setup wizard — Design Spec

**Date:** 2026-09-25
**Status:** Approved design. Next step: implementation plan.
**Repos:** `tinycld` (core, app shell, generator).

## 1. Problem

A new standalone server prints a URL with a 64-character token. The person must copy that URL from the log. `/a/setup` then shows one form (app name, email, password, URL), and after it the person lands in the package manager, not in the app. Nothing helps them set up branding, email sending, or their team.

Without a shared wizard, a package that needs onboarding steps must build a wizard of its own. This spec gives core one wizard that every deployment uses. Packages add steps through a manifest registry, so core never names a package.

## 2. Decisions

- One wizard shell and one step registry in core. Packages contribute steps with a manifest field.
- A short setup code replaces the long URL token. The printed link still fills in the code.
- Standalone steps: code, owner account (both before sign-in), then Workspace, Apps, Email sending, Invite your team, Done.
- The Apps step shows only bundled packages. Clearing one hides it for everyone; it never uninstalls or rebuilds.
- Layout "workspace assembles as you go": form on the left, a live miniature of the workspace on the right (a strip at the top on phones).
- Step order uses a fractional-indexing rank scheme (the `fractional-indexing` library's keys).
- Wizard state is one `system_settings` row. "Done" is derived from real data wherever possible.

## 3. Prerequisite fix: package hide control

The Settings → Packages checkbox writes `pkg_registry.status = 'disabled'`. Two bugs make that unusable for the wizard:

1. `core/server/coreserver/pkg_seed.go:117-120` — boot sync sets a `disabled` bundled row back to `bundled`. A hidden app comes back after every restart.
2. `core/components/setup/PackageManager.tsx:507-508` — `isBundled` is read when the status is already `disabled`, so re-enabling always writes `installed`, which also shows the uninstall button on a bundled app.

Root cause: `disabled` means both "the owner hid it" and "it left the build" (`pkg_seed.go:139`, `rebuild.go:520`). Boot sync cannot tell them apart.

Fix, with no new field:

- Boot sync no longer changes a `disabled` row. It still sets `disabled` on a `bundled` row that left the build.
- The server chooses the status on re-enable. A PocketBase update hook on `pkg_registry`: when `status` changes from `disabled` to anything other than `disabled`, set `bundled` if the slug is in `bundled-packages.json`, else `installed`. The client sends only enable/disable.
- Accepted side effect: a package that leaves the build and later returns stays hidden until the owner turns it on again.

## 4. Server: setup code

In `core/server/coreserver/setup_bootstrap.go`:

- **Code.** 8 characters from an alphabet with no ambiguous characters (no `0 O 1 I L`), shown as `K7QM-3XPD` (about 40 bits). The dash is display only; input ignores dashes, spaces and case. It stays in memory only. A new code prints at every boot until an owner exists.
- **Printed box.** Shows the link with `?code=` and the code on its own line.
- **`POST /api/setup/verify {code}`.** Constant-time compare. Returns `200` or a specific error. Lets the UI check the code before the account step.
- **Lockout.** 5 failed tries from one IP within 10 minutes lock that IP for 10 minutes. 20 failed tries in total make a new code, which is printed again. Both `verify` and `init` count.
- **`POST /api/setup/init`.** As today, but takes `code` (not `token`) and the owner's `name`. The check and the clear of the code happen inside one locked section, so two concurrent calls cannot both succeed (this closes the current TOCTOU gap). It also writes the wizard state row (§7).
- **`GET /api/setup/check`.** Unchanged.
- **`create-owner`** (`owner_command.go`) also writes the wizard state row, on every run when no row exists (even if the owner already exists). A service provider that creates owners with it gets the wizard with no special code in core.

## 5. Step registry

### 5.1 Manifest field

```ts
setupSteps?: { id: string; label: string; module: string; order?: string }[]
```

The generator emits each package's steps into its `tinycld.config.ts` entry as `load` thunks (not `React.lazy`, because the module exports hooks too), next to `settings`. The registry id is `<slug>:<id>`. The generator rejects an `order` for which `generateKeyBetween(order, null)` throws.

### 5.2 Step module contract

- `default` — the step component, props `{ next }`. It renders its own form and main button and calls `next()` after its mutation succeeds.
- `useIsStepDone?(): boolean | undefined` — derived from real data; `undefined` while loading. When absent, the step is done once it is in `acknowledged` (§7).
- `useIsStepVisible?(): boolean` — defaults to visible.

### 5.3 Order

`order` is a fractional-indexing key. Steps sort by plain string comparison (not `localeCompare`), ties broken by registry id. A step with no `order` sorts after all keyed steps.

### 5.4 Core steps

Core has no manifest, so its steps live in a static list in `core/lib/setup/core-steps.ts`, merged with the generated list.

| order | id | Step | Done when |
|---|---|---|---|
| `a0` | `core:workspace` | Workspace — name and logo, fields from `OrgBrandingSection` | acknowledged (a new server already has PocketBase's default name, so the name cannot tell) |
| `a1` | `core:apps` | Apps — bundled packages, enable/disable (owner only: `pkg_registry` writes are owner-only) | acknowledged |
| `a2` | `core:email` | Email sending — fields from `MailSendingPanel` (owner only) | acknowledged |
| `a3` | `core:team` | Invite your team — `/api/invite-member` | more than one user, or a pending invite |

`core:email` is visible only when `!useIsSettingManaged('mail')`, so a deployment where mail settings are managed never shows it.

Packages place steps between these keys, e.g. a package step at `a0V` or `a0k`. A key before `a0` (e.g. `Zz`, which is `generateKeyBetween(null, 'a0')`) puts a step first after sign-in.

### 5.5 Core slot in the invite step

A package can show context inside the invite step (for example a seat meter) without core knowing what it is. `sidebarContributions` today targets only package slots. The generator also accepts `target: 'core'` for slot names that core declares in a static list, `core/lib/setup/core-slots.ts`. Core declares one: `setup-team`, rendered above the invite form. A contribution to an unknown core slot is a build error, as it is for package slots.

## 6. UI

### 6.1 Layout

`core/components/setup/wizard/`:

- `SetupWizardShell` — segmented progress bar (done, current, skipped), "Finish later", Skip, form column, preview pane.
- `WorkspacePreview` — a miniature of the workspace: dark rail with the org logo and one icon per enabled app, sidebar with the workspace name, member avatars. It reads only core data (branding, enabled `pkg_registry` rows, users and invites, SMTP state) and updates live as the owner types or toggles. Package steps do not add to it.
- `WorkspacePreviewStrip` — below the `md` breakpoint the preview becomes a dark strip above the form: logo, name, app icons, avatars.
- All colors come from semantic tokens (`primary`, `rail-background`, `muted`, …); light and dark modes both work.

### 6.2 Screens

1. **Claim this server** (before sign-in). Code input: one `TextInput` drawn as eight cells so paste and autofill work. The preview shows the server log box with the code highlighted, and the note "Look for this box in the server log." A `?code=` in the URL is verified and skips this screen. Help line: "Code not in the log? Restart the server. A new code prints each time it starts until the server is claimed."
2. **Create your owner account** (before sign-in). Name, email, password (min 10), confirm; "Advanced" disclosure holds the web address, filled from the browser origin (native: the resolved server address, as today). The preview shows a ghost workspace with the owner's avatar. "Create account" calls `init`, stores the auth token, and opens the first visible step.
3. **Workspace.** "People see this name and logo when they sign in and in invite emails." The name is saved through a new `POST /api/org-info/name` (owner/admin), which writes `Meta.AppName`. The account step no longer asks for an app name.
4. **Choose your apps.** Bundled apps as cards, all selected at first. Copy: "These apps come with your server. Clear an app to hide it from everyone. You can show it again, or add more apps, at any time in Settings → Packages." Button: "Continue". Toggling updates the preview rail at once; nothing rebuilds.
5. **Email sending.** SMTP fields and a test send.
6. **Invite your team.** One invite form: username, optional email, and role (member or admin). "Send invite" creates one invite at a time and shows its link; the people already in the workspace are listed below the form. The `setup-team` slot renders above the form. When the server refuses an invite (for example a seat limit), the refusal shows on the form.
7. **Done.** The preview grows to fill the screen. "<Name> is ready" with a one-line summary ("3 apps on · 2 people") and "Open <Name>".

Mockups: `.superpowers/brainstorm/28983-1790348859/content/steps-v2.html` (not committed).

### 6.3 Routes

- `/a/setup` — before sign-in: code and account screens. The recovery console (`SuperuserLoginForm` → `SetupDashboard`) moves to `/a/setup/recovery` unchanged.
- `/a/setup/[step]` — after sign-in. Outside `(app)` on purpose: the workspace chrome (and its own redirect into the wizard) must not wrap it, so it asks for a session itself, with its own `AuthGate` and admin guard.
- When `needsSetup` is true, an unauthenticated visit to the app root goes to `/a/setup`.

## 7. State, entry, resume

One `system_settings` row, key `setup.wizard`, owner/admin only (the collection already enforces this):

```ts
{ startedAt: string; acknowledged: string[]; skipped: string[]; dismissedAt?: string; completedAt?: string; orgNameSeeded?: boolean }
```

- `init` and `create-owner` write `startedAt` when no row exists. Every `create-owner` run does this, even when the owner already exists, so re-running `create-owner` is how an existing installation opts into the wizard. An existing row is never reset. Installations that never run `create-owner` again have no row and never see the wizard.
- `create-owner --org-name <name>` sets `Meta.AppName` and `orgNameSeeded: true`. The wizard shows the saved name (field, preview, Done) only when `acknowledged` has `core:workspace` or `orgNameSeeded` is true; otherwise the field starts empty and Done says "Your workspace is ready".
- **Continue** (`next()`) always moves on. If the step's `useIsStepDone` is false (or still loading), Continue adds the step to `skipped`, the same as Skip. Otherwise (done, or no `useIsStepDone`) Continue adds the step to `acknowledged` and removes it from `skipped`. Continue never removes a skip from a step that is still not done.
- **Skip** adds the step to `skipped`. A skipped step can be reopened. A skipped step that later becomes done shows as done.
- **Entry.** On sign-in or app load, an owner or admin goes to the wizard when the row exists with no `dismissedAt` and no `completedAt`. Members and guests never see it; the route guard sends them to the app.
- **Resume.** Open the first visible step that is not done and not skipped; if none, open Done.
- **Finish later** sets `dismissedAt` and stops the redirect. A "Finish setup: 2 of 5 done · Continue" card shows at the top of Settings (above Account) until `completedAt` is set.
- **Done** sets `completedAt`.

## 8. Errors and logging

- Code errors are specific and do not apologize: "That code does not match. Check the server log for the latest code." / "Too many tries. Wait 10 minutes, or restart the server for a new code." If `needsSetup` is false: "This server is already set up" with a sign-in link.
- Step mutations use `useMutation` with `handleMutationErrorsWithForm`; errors show on fields and the wizard does not advance.
- A failed preview read never blocks the form; the preview shows its ghost state.
- Server: `logging.ForPackage("coreserver")`. Client: `log.error('core.setup', …)`.

## 9. Platforms

The same screens run on web and native. On native, the connect-to-server flow calls `/api/setup/check` and opens the code screen when setup is needed.

## 10. Help

- New `core/help/first-run-setup.md`: the setup code, each step, Finish later, the Settings card.
- `core/help/installing-packages.md`: one paragraph on hiding bundled apps.

## 11. Testing

**Go**
- Code length and alphabet; input normalization.
- Constant-time verify.
- Lockout per IP and in total; a new code after the total limit.
- `init` race: two concurrent calls, exactly one success.
- `init` and `create-owner` write the state row.
- Boot sync keeps a `disabled` bundled row.
- Re-enable writes `bundled` or `installed` correctly.

**Vitest**
- Registry merge and rank sort, ties, missing key.
- Generator accepts a `core` slot contribution and rejects an unknown core slot.
- Generator rejects an invalid `order`.
- Resume picks the right step.
- Entry rule per role and state.
- `useIsStepVisible` hides steps.

**Playwright** (`tinycld/tests/standalone`, which boots the shipped binary in an empty directory; `tests/e2e` cannot be used because its server starts with a superuser, and the wizard state is deployment-wide, so writing it there would redirect every parallel spec)
- A new server: code → account → every step → Done → app.
- Finish later shows the Settings card; Continue resumes at the next step.
- Hide an app in the Apps step, restart the server, the app is still hidden.
- A wrong code shows the mismatch message.

## 12. Out of scope

- A package banner slot in the app shell.
- Package steps adding to the preview.
- Installing non-bundled packages from the wizard.
