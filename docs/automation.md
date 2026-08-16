# Automation: writing triggers and actions for your package

How a package plugs into the workflow-rules system: declare **triggers**
(events users can build rules on) and **actions** (things rules can do) as
pure data, and core does the rest — the engine listens, evaluates each
user's rules, executes actions, and logs every run to `rule_runs`; the
builder UI renders your catalog with zero package-side UI code.

Reference implementation: mail (`mail/tinycld/mail/automation.ts`,
`mail/server/automation.go`). User-facing docs live in the in-app help
(`core:rules`, `mail:rules`); this document is for package authors.

## The one-file declaration

1. Point the manifest at a definitions module:

   ```ts
   automation: { definitions: 'automation' },
   ```

2. Add the matching exports-map entry in `package.json`:

   ```json
   "./automation": "./tinycld/<slug>/automation.ts",
   ```

3. Write `tinycld/<slug>/automation.ts` — a default-exported, **pure-data**
   object, typed against your generated schema:

   ```ts
   import type { AutomationDefinitions } from '@tinycld/core/lib/automation/types'
   // Relative, NOT the ~/ self-alias: this module is config-reachable
   // (imported by the generated tinycld.config.ts under the app shell's
   // tsconfig, where your self-alias doesn't resolve) — same reason
   // collections.ts imports './types'.
   import type { MailSchema } from './types'

   const automation = {
       triggers: [
           {
               id: 'message-received',
               label: 'A message arrives',
               collection: 'mail_messages',
               on: 'create',
               fields: ['subject', { key: 'sender_email', label: 'Sender' }],
           },
       ],
       actions: [ /* … */ ],
   } satisfies AutomationDefinitions<MailSchema>

   export default automation
   ```

The schema generic makes collection and column references compile-checked —
a typo'd field name fails your package's typecheck. The generator
additionally validates every definition structurally at generate time
(kebab-case ids, unique ids, param references, no synthetic triggers outside
core) and **fails the build** on violations, then materializes the merged
definitions to `server/automation_defs.json` for the Go engine. `import
type` only; the module must stay JSON-serializable data.

Everything is referenced by qualified ref: `<slug>:<id>`, e.g.
`mail:message-received`, `core:apply-label`.

## Triggers

A record trigger names a collection and an operation:

| Field | Meaning |
|---|---|
| `id`, `label` | kebab-case id; the human sentence ("A message arrives"). |
| `collection`, `on` | Which collection, and `create` \| `update` \| `delete`. |
| `watch` | Update triggers only: fire only when one of these columns changed. |
| `fields` | Curated allowlist of columns exposed to conditions, templates, and run summaries. Entries are bare keys or `{ key, label }` overrides (labels default to humanized column names). **Omit to expose every column** (plus `created`/`updated`). |
| `ownerField` | Overrides owner auto-detection (see *Personal-rule scoping*). |

Exposure is a **security surface**: hidden and system columns are filtered
out everywhere (templates, summaries, condition evaluation, the client
catalog) even if you name them explicitly — autodate timestamps are the one
sanctioned system exception. Columns with no condition operators (`json`,
`file`) are skipped automatically. Conditions on non-exposed fields fail
closed at evaluation time, so curate `fields` freely.

Synthetic triggers (`core:schedule`, `core:manual`) are core-only; the
generator rejects them in feature packages.

### Trigger filters: when a row is not an event

If your collection carries rows the trigger's *semantics* exclude, register
a filter from your Go `Register(app)` — it gates the event for **all** rule
scopes (org and personal):

```go
automation.RegisterTriggerFilter("mail:message-received", func(app core.App, record *core.Record) bool {
    return messageIsInbound(record) // drafts and outbound sends are not "received"
})
```

Without one, every matching create/update/delete dispatches. Mail's filter
exists because `mail_messages` rows include drafts and outbound sends —
"a message arrives" must not fire on them.

### Personal-rule scoping (owner resolution)

A personal rule fires only on events that belong to its owner. The engine
resolves owners by, in order: a registered resolver, the trigger's declared
`ownerField`, then auto-detection (the first of `user`/`owner`/`author`
that is a relation to `users`). If none resolves, **personal rules never
fire for that trigger** (org rules are unaffected).

Collections without a direct user FK (e.g. mail's shared mailboxes) register
a resolver from `Register(app)`:

```go
automation.RegisterOwnerResolver("mail:message-received", func(app core.App, record *core.Record) []string {
    return mailboxMemberIDs(app, record) // thread → mailbox → members; nil on malformed data, never an error
})
```

Return every user id the event belongs to; the engine set-matches rule
owners against it. Returning the wrong owner fires other users' personal
rules on this data — treat resolvers as security code.

## Actions

Two kinds. **Record-ops** are declarative database writes the core engine
executes itself — no handler to register, though a relation param still
needs its authorizer (see **Record-level authorization** below):

```ts
{
    id: 'move-to-folder',
    label: 'Move to folder',
    kind: 'record-op',
    collection: 'mail_messages',
    op: { type: 'update', target: 'trigger-record', set: { folder: { param: 'folder' } } },
    params: [{ key: 'folder', field: 'folder' }],
}
```

- `op.type`: `create` (a new record in `collection`), or `update`/`delete`
  with `target: 'trigger-record'`. Trigger-record ops are only offered for
  triggers on the same collection — cross-package compatibility is
  structural, never enumerated.
- `op.set` values: `{ param: '<key>' }` (rule author supplies it),
  `{ context: 'record-id' | 'collection' | 'owner' }` (engine-provided), or
  a literal.
- Params with `field: '<column>'` inherit the column's type, relation
  target, and select options; novel params declare `type` (`text`,
  `number`, `boolean`, `date`, `select`, `relation`). A novel `relation`
  param must also declare `relationTarget: '<collection>'` — the generator
  rejects one without it (there is no column to inherit a target from, and
  a targetless relation param renders a picker over nothing). Every text
  param accepts `{{field}}` placeholders filled from the trigger's exposed
  fields — no opt-in flag; relation params are never template-substituted
  (ids are picker-chosen, and letting trigger content pick the id would be
  an injection channel).

**Native actions** dispatch to a Go handler you register in `Register(app)`
(mirrors `$`-binding registration — must run before hooks load):

```ts
{ id: 'send-message', label: 'Send a message', kind: 'native',
  params: [{ key: 'to', type: 'text' }, { key: 'subject', type: 'text' }] }
```

```go
automation.RegisterAction("mail:send-message", func(app core.App, req automation.ActionRequest) error {
    // req.Params are template-substituted strings; req.Record is the trigger
    // record (nil for scheduled/manual rules); req.OwnerID is the rule's owner.
    return sendFrom(app, req.OwnerID, req.Params)
})
```

Native-action caveats:

- **Available wherever your package's Go is linked.** Every deployment
  shape links feature Go — a multi-org router builds a per-org artifact
  from exactly that org's package set (`multi-org/README.md`), so your
  handler exists wherever your package is installed. The catalog marks an
  action whose handler is absent (package removed / not installed)
  unavailable and the UI greys it out ("needs <pkg>") —
  declared-but-unregistered is a supported state, not an error.
- **Handlers self-enforce access** — see **Record-level authorization**
  below for what the engine gates for you (relation params) and what stays
  your job (everything else).
- Returning an error records the action as failed in `rule_runs` and
  continues to the rule's later actions (mail-filter semantics).

## Record-level authorization

The engine runs actions with system authority: record-ops write with a
superuser `Save`, and native handlers receive a superuser-powered `app`.
PocketBase collection rules therefore do NOT protect anything on this path —
whatever check a write needs has to happen in engine or package Go. The
division of labor:

**The engine gates relation params — and only relation params.** A relation
param's value is a caller-supplied record id (the rule JSON is
client-authored; the picker is a convenience, not a boundary). Before an
action of either kind runs, every non-empty relation param passes two
fail-closed layers:

1. **The floor (engine-owned, generic):** the rule owner must pass the
   target collection's own `viewRule` for the referenced record, evaluated
   as the owner (`CanAccessRecord`). Missing record, failed rule, or a
   locked (nil) rule refuses the action. This proves the owner may *see*
   the record — nothing more. Rule branches needing request context beyond
   auth (e.g. a share-link token header) correctly evaluate false:
   automation acts as the rule owner, never as an anonymous link holder.
2. **Your registered `RelationAuthorizer` (required):** the write-level
   question — may this rule file into that folder, assign that user, move
   to that list — is package semantics the engine cannot know. Declaring a
   relation param without registering its authorizer is refused at
   execution and greys the action out in the catalog, so the question gets
   answered in code, not assumed:

   ```go
   automation.RegisterRelationAuthorizer("drive:move-to-folder", "parent",
       func(app core.App, req automation.ActionRequest, id string) error {
           return destinationWritableBy(app, req.OwnerID, id)
       })
   ```

   A deliberate pass is fine when the collection's own model is org-wide —
   core's `apply-label` authorizer returns nil with a comment saying why
   (labels are org-wide by design). The point is that the decision is
   written down where review can see it.

**Everything else stays the handler's job.** The floor+authorizer pair
covers ids the *engine* hands you; records your handler looks up itself,
text params that name things (recipients, addresses), and who an action
acts *as* are still yours to check — `checkPersonalAccess` applies
`pkgaccess` to personal-rule record-ops only, deliberately: it is a
package-level floor that answers "may this user write to this package at
all", never *which record*, and every real bug this system has produced
was a which-record bug. Reference implementations worth copying:

- `mail/server/automation_actions.go` — `actionAudience`/`applyToAudience`
  (org rules act on all mailbox members, personal rules only as a
  still-current member) and `ruleRecipient` (a rule may not mail the
  mailbox's own addresses — the self-feeding-loop check).
- `calendar/server/automation.go` — `ownedCalendarFor` +
  `writableCalendarRoles` (rule-created events land only on a calendar the
  owner can write, mirroring the collection's create rule the superuser
  path bypasses).
- `text/server/automation.go` / `calc/server/automation.go` — delegate to
  core's `driveshare.ParticipantIDs` rather than re-deriving sharing rules
  (a second copy would drift, and an over-reporting resolver fires other
  users' rules on documents they cannot see).
- `cards/server/automation.go` — `cardOwnerResolver` (project membership
  scopes which personal rules fire at all).

## Execution semantics you inherit

Package authors don't implement any of this, but should know it:

- Org rules run before personal rules, each ordered by the user's `order`;
  `stop_processing` halts downstream rules. Org rules execute with system
  authority (admin-authored); personal record-ops are pkgaccess-checked as
  the owner.
- Engine writes carry provenance: a rule never re-fires on its own write,
  and action→trigger cascades cap at depth 3 (logged as
  `chain-depth-exceeded`).
- Actions time out at 30s (abandoned, not killed — don't write handlers
  that hold locks past that).
- Every dispatch writes a `rule_runs` row (matched or not); runs are pruned
  to 200 per rule; ~20 consecutive fully-failed runs auto-disable the rule
  and notify its owner.
- Condition operators are typed (text ops case-insensitive; `is`/`is_not`
  match any element of multi-value fields; dates accept PB datetime, bare
  date, or RFC3339).

## Your ingress must finish before it fires a trigger

**If your package writes records around the one a trigger watches, wrap that
write path in `app.RunInTransaction`.** This is the one execution detail that
*is* your responsibility, and getting it wrong produces a rule that reports
success and does nothing visible.

Dispatch is bound to `OnRecordAfterCreateSuccess` (and the update/delete
equivalents). Outside a transaction that hook fires *the instant the record is
saved* — while the rest of your ingress function is still running. Rule actions
then execute on a worker goroutine, concurrently with your remaining writes.

Mail hit this. Its inbound path stored the message (firing the trigger), then
wrote each recipient's `mail_thread_state` with `folder: "inbox"`. A
`mail:move-to-folder` action archived the thread, and delivery's write landed
afterwards and put it back:

```go
storeMessage(app, thread.Id, stored)   // fires the trigger; actions start
...
ensureThreadState(app, thread.Id, userID, "inbox", false)  // clobbers the action
```

The run logged `matched: true` with `status: "ok"`. Nothing was red — no error,
no failed test — and the message simply stayed in the inbox.

The fix is structural, because `OnModelAfterCreateSuccess` is *"delayed and
executed only AFTER the transaction has been committed"* and is **not**
triggered on rollback:

```go
return app.RunInTransaction(func(txApp core.App) error {
    // every write for this delivery, using txApp
    return nil
})  // hook fires here, once, over settled state
```

Two properties you get for free: rules fire exactly once with all related rows
in place, and a failure partway through means rules never fire at all rather
than firing against a half-written record.

Reordering so the triggering save comes last also works, but leaves the
ordering load-bearing — the next person to add a write to that function
reintroduces the bug. Prefer the transaction.

Worth checking in your package: bulk importers that save in a loop (each
iteration is a dispatch), and any path that writes a parent/summary record
after the child rows.

## The catalog (how the UI learns about you)

At boot the engine resolves every declared trigger/action against live
collection metadata — field types, relation targets with display-field
heuristics, select options, native availability — and materializes rows
into the read-only `automation_catalog` collection. The builder UI
live-queries it; you never write catalog code. Consequences worth knowing:
relation params get a record picker automatically (display field chosen
from `name`/`title`/`label`/`subject`/`display_name`/`email`/`username`);
an action on a collection that doesn't exist in the deployment shows as
unavailable rather than vanishing.

## Verifying your declarations

- `pnpm exec tinycld-pkg typecheck` in your member catches bad
  collection/field refs; `pnpm run packages:generate` from `tinycld/` runs
  the structural validation and materializes the defs (inspect
  `server/automation_defs.json`).
- Go-side registrations (`RegisterAction`, `RegisterOwnerResolver`,
  `RegisterTriggerFilter`) are unit-testable against real migrations — see
  `mail/server/automation_test.go` and core's
  `core/server/automation/*_test.go` for the `rlstest` idiom.
- E2E: drive the builder UI and use your package's real ingress (mail uses
  its inbound webhook helper) — never raw PB writes. `mail/tests/rules.spec.ts`
  is the reference.
- **Assert the visible effect, not the run row.** `rule_runs` saying
  `matched: true` / `status: "ok"` only proves the action was called — it
  survives the ingress race above, and it survives an action that writes
  somewhere no view reads. Assert what the user would see: the row appears in
  the destination, and is gone from where it was.

## Gotchas

- `automation.ts` imports its own types **relatively** (`./types`), never
  via the `~/` self-alias — the module is loaded under the app shell's
  tsconfig.
- Never import another sibling from `automation.ts` (or anywhere) — the
  lean-shell rule applies; cross-package reach happens through record-ops
  on your own declared collections and core's built-ins.
- An action that writes rows some UI never reads is worse than no action:
  the rule "succeeds" and the user sees nothing. Verify the full loop
  (trigger → action → visible effect) before declaring — the known
  `core:apply-label`-on-`mail_messages` gap (message-scoped assignments vs
  mail's thread-scoped label views) is the cautionary tale.
- Definitions are data, not migrations — you may evolve them freely within
  a version, but renaming an `id` orphans existing rules (they surface as
  "needs <pkg>" / unknown-ref runs). Treat published trigger/action ids as
  API.
