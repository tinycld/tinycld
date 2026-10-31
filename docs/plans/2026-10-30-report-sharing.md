# Saved Reports and Sharing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Saved reports in core: two collections (`reports`, `report_shares`) with rules for owners, editors, viewers, group members and admins; server hooks that check a spec on save and keep `published` for admins; saved-report runs through `POST /api/reports/run` with `{ reportId }`; the broken-report check (`tinycld reports validate`, and at every server start); reports that packages ship; and the offboard handler.

**Architecture:** One core migration makes `reports` and `report_shares`. The rules decide who may read and write a row. `report_shares` is a group grant table (`core/server/groups`): a grant row (group set, user empty) expands into one derived row per member, so every rule tests only `user`. In `tinycld.org/core/reports`, `RegisterSharing` binds request hooks that guard what a permitted write may change (the spec is loaded and validated as a run would, `published` is admin-only, server-owned fields are replaced), the share guards, the shipped-report registry (`RegisterShipped`, made real when the first admin exists) and the offboard handler. `Register` (the engine's existing `OnServe`) also reconciles shipped reports and marks every saved report that cannot run (`CheckSaved`). The run endpoint reads a saved report through the `reports` view rule with PocketBase's own `CanAccessRecord`.

**Tech Stack:** Go (PocketBase fork v0.40.4 at `tinycld/third_party/pocketbase`, `dbx`, cobra), PocketBase JS migrations, Biome (no TS change in this unit).

**Spec:** `/Users/nas/code/tinycld/docs/superpowers/specs/2026-10-30-report-engine-design.md`: "## 3. Saved reports and sharing" in full; from "## 2. Report engine" the parts it left for saved reports ("### Spec versions": the validator, "saved upgraded on its next save"; "### Run endpoint": the `{ reportId, params, options }` body and its 403); the lines of "## Errors", "## Security" and "## Testing" that apply to saved reports. Read-only: never edit anything under `/Users/nas/code/tinycld`. The previous unit's plan, `docs/plans/2026-10-30-report-engine.md` in the core repo, built the engine this plan uses (`Load`, `Validate`, `Run`, `SpecError`, the zoo test fixture).

**Repos and paths:**
- Workspace root: `/Users/nas/tmp/tinycld/books-purchases` (shortened below as `$WS`).
- Core repo: `$WS/tinycld`, branch `books/p5-report-sharing` (core main `b4b061ea`). Core Go module `tinycld.org/core` at `$WS/tinycld/core/server`; the package is `$WS/tinycld/core/server/reports`. The app server module is `$WS/tinycld/server` (`server/main.go` is tracked; `server/go.work`, `server/package_extensions.go` and the `server/pb_migrations/*` symlinks are generated).
- Binaries: call them from `$WS/node_modules/.bin` (`PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH`). `pnpm exec` starts a workspace install; do not use it. Do not run `pnpm install` or `packages:generate`; do not start servers; no e2e in this unit.
- Go tests always build with `-tags no_ui` (`docs/packages.md`, "Test build tag").
- Do not touch the `books` member.

## Global Constraints

- Core names no package. Tests use the fictional zoo fixture (`zoo_*` collections, `zoo.*` source keys, `zoo.feed_costs` shipped key). `PATH=$WS/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts` stays at 0 references.
- `reports`: `owner` (users), `title`, `description`, `spec` (json), `source` (text, the base source key, for filtering), `published` (bool), `published_by`, `created`, `updated`. `report_shares`: `report`, `user`, `group`, `role` (`viewer` | `editor`); the three row kinds of a group grant table (direct, group grant, derived), expanded by `core/server/groups`, with the login requirement on every rule that reads `report_shares.user`.
- Rules: view/list = owner, or a `report_shares` row for the reader (direct or via group), or `published = true`. Update = owner or editor; only an admin can change `published` / `published_by` (a server hook). Delete = owner, or an admin for a published report. Create = a signed-in user with `owner` = the caller. Shares are added and removed by the owner and editors only.
- Offboarding: reassigned to the chosen user, or deleted with their shares. A published report is always reassigned, never deleted.
- A spec is upgraded on read and saved upgraded on its next save; field keys resolve through each source's aliases on save and on run.
- `POST /api/reports/run` body: `{ reportId, params, options }` or `{ spec, params, options }`, never SQL. 403 when the caller cannot view the saved report; 400 field-level errors for an invalid spec; 422 naming an unknown reference. The endpoint is read-only.
- A share gives the design only: each reader runs the report with their own rights.
- Admin = `users.role` is `owner` or `admin` (and not disabled), as in every core rule.
- Go logs through `logging.ForPackage("reports")` (the package's `log`).
- Every input that can grow is capped, and a value past a cap answers 400, never 500 (see "Caps" below).
- A performance property is pinned by the shape of the work (reads, statements), never by wall-clock time.
- Quality gate for each task and at the end: `go test -tags no_ui ./...` and `go vet -tags no_ui ./...` in `$WS/tinycld/core/server`; `PATH=$WS/node_modules/.bin:$PATH tinycld-pkg check` in `$WS/tinycld`; `PATH=$WS/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts` in `$WS/tinycld`. Each task's steps run the targeted tests; the full gate runs in each task's last step.

## Rulings

Each is a decision the spec leaves open, with the reason.

- **P1. Migration number `2090000008_create_reports.js`.** The newest core migration is `2090000007_autoupgrade_state_failed.js`; package bands start at `2100000000` (Books), so core's next file stays in its own band and sorts before every package migration.
- **P2. Who counts as an admin.** `role` is `owner` or `admin` and the account is not disabled, the same test as every core rule (`2040000000_create_groups.js`, `2090000000_create_forms.js`). The Go helper is `isActiveAdmin`.
- **P3. Guests.** A guest may create, own and be shared reports (create is "any signed-in user"), but never sees a published report: published reports are org-wide content, and core hides org-wide content from guests (`rules` with `scope = "org"`, `groups`). A guest is never an admin, so never publishes.
- **P4. An admin is not an editor.** The update rule is owner or editor, as the spec says. An admin who neither owns nor edits a report cannot edit it or change its `published` flag; an admin can delete a published report. The publish switch therefore needs both: update access and the admin role.
- **P5. Statuses follow PocketBase.** A refused list returns fewer items; a refused view, update or delete is 404; a refused create is 400 `Failed to create record.` (probed). Hook refusals: publish by a non-admin 403 (`published.admin_only`); every other hook refusal 400 with a field code.
- **P6. Server-owned fields.** `source` (the spec's base source), `published_by`, `broken`, `shipped_key` and `shipped_revision` are written only by the server: a client value is replaced, on create and on update. Changing `owner` through the API is refused (400 `owner.read_only`); only the offboard flow moves a report. A superuser request passes every hook unchanged (a superuser can change any row through PocketBase anyway).
- **P7. The spec on save.** Every client save runs `Load` (upgrade, aliases) and stores the spec as `Load` returns it, so a saved spec is "saved upgraded on its next save". A create, or an update that changes the spec, must pass `Validate`, and then clears `broken`. An update that leaves the spec as it was keeps it, valid or not, so a broken report can still be renamed. Refusals have the run endpoint's shape: 400 `The report is not valid.` or 422 `The report names something that does not exist.`, `data` keyed by spec path (`columns.0.id`, `source`).
- **P8. Shares.** A share names one user or one group (400 `user.one_target`), never the owner (400 `user.owner`). An update changes only `role` (400 `<field>.read_only`). A derived row follows its grant and cannot be edited or removed alone (400 `group.follows_grant`). A manager is the owner or an editor, an editor through a group included. A share row is visible to the report's managers and to the user it names.
- **P9. A saved run the caller cannot view is 403, and so is an id that does not exist** (`reportId.no_access`, "You cannot view this report."): an id tells nobody whether a report exists. The check is the `reports` view rule applied by PocketBase's `CanAccessRecord`, so it can never drift from the API. A saved run's `topics` add `collection/reports` and `collection/report_shares`, so an open report runs again when it is edited or its shares change, and learns when the reader lost access.
- **P10. The broken mark is stored.** `reports.broken` (json, server-owned) holds `{status, problems, more}` for a report that cannot run, else null. `CheckSaved` refreshes it at every server start (all modes), `tinycld reports validate` refreshes it and lists the broken reports with their owners, and every validated client save clears it. The spec runs the validator at startup "in dev"; this plan also runs it at every start because the hub must show broken reports ("The admin report list shows them as broken") and a source disappears only when a package install or upgrade restarts the server. Dev logs each broken report at Warn; production logs one Info line with the count. The column is written with SQL (no `updated` change, no audit row) and then `changesignal.Touch(app, "reports")`.
- **P11. `reports validate` is a subcommand of the app binary,** wired in `server/main.go` beside `create-owner`. It bootstraps (PocketBase does this before any subcommand), applies pending migrations as `create-owner` does, checks the sources, prints each refused source, runs `CheckSaved`, and exits non-zero when a report is broken.
- **P12. Shipped reports are a Go registry, not a JS migration.** On a fresh install the migrations run before the setup wizard creates the first owner, so a migration has no admin to own a report. A package calls `reports.RegisterShipped(reports.Shipped{Key, Title, Description, Spec, Revision})` from its `Register(app)`. `ReconcileShipped` runs at every start and whenever an active admin's users record is saved: a missing report is created published, owned and published by the first admin; a stored one with a lower `shipped_revision` gets the new spec, title and description (the spec's "upgraded by the package's migrations when their spec changes" becomes "the package raises `Revision`"). The owner and the published flag stay as people left them. A shipped report cannot be deleted while its package ships it (400 `shipped_key.shipped`: unpublish it instead); once the package is gone it can.
- **P13. The first admin** is the oldest active owner, else the oldest active admin.
- **P14. Offboarding is a handler, not a reassignable ref** (a flat rewrite cannot keep published reports while deleting the rest). Reassign: every report goes to the successor. Delete my data: an unpublished report is deleted with every share row; a published one goes to the heir. Keep: an unpublished report stays with the anonymized account, as other kept content does; a published one goes to the heir. The heir is the successor, else the acting admin (when not the leaver), else the first admin; with no heir and a published report the offboard is refused (`offboard.ErrInvalidPlan`) and rolled back. A report that changes owner drops the new owner's direct share on it. In every mode the leaver's direct shares on other reports are deleted (their derived rows are already gone: `OffboardUser` removes group memberships first).
- **P15. `reports.owner` is required with no cascade.** A hard delete of a user who owns reports is refused (probed: "the record cannot be deleted because it is part of a required reference"), so offboarding is the only way their reports change hands. A cascade would delete published reports.
- **P16. `groups.registerCore` is exported as `groups.BindHooks`.** A grant table outside `groups` needs real expansion in its tests, and `groups.Register` binds the expansion only for tables registered before it runs. `RegisterSharing` is called in `RegisterSharedCore` right before `groups.Register`.
- **P17. Audit.** `reports` (label `title`, `spec` and `broken` redacted) and `report_shares` join the audit log, as `groups` and `group_members` do: a share or a publish changes who sees what.
- **P18. OAuth.** Neither collection is registered with `oauth`, so an API token reaches neither (default-deny), as the run route. A test pins it.
- **P19. No in-app help topic in this unit:** it adds no screen. `p5-report-builder` writes `tinycld/core/help/reports.md` (building, sharing and publishing reports; "Each person sees only the data they can access").

## Deferrals

None. Everything §3 asks for, and every part §2 left for saved reports, is built here. The hub, report view, builder and share dialog are the planned unit `p5-report-builder` (spec "## 4. Reports UI"), not a deferral: this plan gives it every server contract below.

## What was probed

Every probe ran against the code at `b4b061ea`; the probe files were deleted before the plan was committed.

- **The CLI wiring.** A scratch binary (`pocketbase.New()`, a `reports` command with a `validate` subcommand on `app.RootCmd`): PocketBase bootstraps before the subcommand runs (`app.IsBootstrapped()` true in `RunE`), `OnServe` does not run, `reports validate extra` fails with `unknown command "extra" for "... reports validate"`, and a `RunE` error exits 1 through `log.Fatal`. `--dev` makes PocketBase print every SQL statement to stdout. Then the real app server, built from `server/main.go` with this plan's wiring: `tinycld reports validate --dir <tmp>/pb_data` with `<tmp>/pb_migrations` holding core's migrations printed `checked 0 saved reports, 0 broken` and exited 0. jsvm reads migrations from `<dataDir>/../pb_migrations`; the dev server's `tinycld/server/pb_migrations` holds one symlink per migration, written by the generator, so the new migration reaches the dev server and e2e only after the next `pnpm install` at the workspace root.
- **The rules, through the API** (`rlstest.NewAssembledApp` with the migration of Task 1): every cell of the matrix in Task 1 answered as written there. The two `?=` conditions on `report_shares_via_report` (user and role) match the same joined row: a viewer of R who is an editor of another report gets 404 updating R. `RequireAuthGuardOnGrantRules(app, "report_shares")` and `RequireAuthGuardOnAccessRules` pass. The partial index `UNIQUE (shipped_key) WHERE shipped_key != ''` refuses a second `zoo.k` and accepts many empty keys. A required json field refuses `{}` as blank.
- **Hook order.** PocketBase checks the create rule before `OnRecordCreateRequest` (`third_party/pocketbase/apis/record_crud.go`: the rule at line 282, the hook at 358), and finds the record through the update or delete rule before those hooks. So a request hook sees only writes the rules allow.
- **Caps.** A 1 MiB + 1 spec, a 201-character title and a 2001-character description each answer 400.
- **Composition.** With Task 9, `TestRegisterBindsTheRecordedHandlerCounts` moves exactly these golden counts: `OnRecordAfterCreateSuccess` 5 → 6, `OnRecordAfterUpdateSuccess` 3 → 4, `OnRecordCreate` 3 → 4, `OnRecordCreateRequest` 12 → 16, `OnRecordDelete` 4 → 5, `OnRecordDeleteRequest` 14 → 18, `OnRecordUpdate` 7 → 8, `OnRecordUpdateRequest` 15 → 19 (`RegisterSharing`: 2 + 1 per request hook kind, 1 per users hook; `groups.Register` binds 3 more for the new grant table; audit adds 2 per request hook kind). `OnServe` stays 37.
- **The whole plan was built in order** in a scratch worktree of `b4b061ea`: each task's tests fail on the previous task's code with the messages quoted in its Step 2, and pass on its own code. On the final code: `go vet -tags no_ui ./...` and `go test -tags no_ui -count=1 ./...` in `core/server` exit 0 (78 packages ok); `tinycld-pkg check` passes (Biome 1413 files; vitest 3455 tests); `check-core-isolation` reports 0 references; `go vet .` in `server` passes; `go test -race` of `reports` passes.

## Caps

Every input that can grow compile time, SQL size, bound values or row counts, with its cap and the test that the cap answers 400 (not 500).

| Input | Cap | Where | Test |
|---|---|---|---|
| A saved spec | 1 MiB (`maxSpecBytes` = `maxRunBody`; json `maxSize` 1048576) | `prepareSpec`, migration | `TestSavedReportCaps` (400 `spec.too_large`) |
| Title | 1 to 200 characters | migration | `TestSavedReportCaps` (400 `title.validation_max_text_constraint`, `validation_required`) |
| Description | 2,000 characters | migration | `TestSavedReportCaps` (400) |
| Shares of one report (direct + grants; derived rows are bounded by the group) | `shareCap` = 200 | `onShareCreate` | `TestTheShareCapAnswers400` (400 `report.too_many_shares`) |
| A broken mark | 50 problems (`maxBrokenProblems`, `more` counts the rest); json `maxSize` 64 KB | `checkSpec`, migration | `TestABrokenMarkIsBounded` |
| Reports checked at start and by `validate` | read 50 at a time (`checkBatch`), keyset on `id` | `CheckSaved` | `TestCheckSavedReadsInBatches` (pins the number of reads) |
| A shipped report | the same title, description and spec caps | `validateShipped` (panics at boot: a wiring mistake) | `TestRegisterShippedRefusesAMalformedReport` |
| A run with `reportId` | the run body limit (1 MiB, 413) and the engine's caps | existing | existing endpoint tests |

No statement in this unit binds a list of values: each rule is a join PocketBase builds, each hook query binds one or two values, and `CheckSaved` binds one cursor. So SQLite's 32,766 bound variables and its expression depth of 1,000 are never near; nothing here grows a statement with its input.

## Review Focus

1. A viewer of a report who is an editor of another report, or whose report has other editors → cannot edit it or manage its shares (Task 1 `TestReportRulesUpdate` and `TestShareRulesCreateAndDelete`, reader `vi`).
2. A signed-out request, which PocketBase resolves to a NULL id that matches a grant row's empty `user` → sees and changes nothing (the logged-out cell of every Task 1 test; `TestReportRulesGuardGrantRows`).
3. A package is removed, so a saved report names a source that is gone → the run answers 422 naming the source, the start marks it broken, and `reports validate` lists it with its owner (Task 4 `TestASavedReportNamesWhatIsMissing`, Task 5 `TestCheckSavedMarksBrokenReports`, Task 6 `TestReportsValidateListsBrokenReportsWithTheirOwner`).
4. The only admin offboards themself while owning a published report → the offboard is refused and nothing changes (Task 8 `TestOffboardWithNoHeirIsRefused`).
5. A fresh install whose first owner signs up after the server started → the shipped reports appear without a restart (Task 7 `TestAShippedReportWaitsForTheFirstAdmin`).

## Contracts for p5-report-builder

The UI unit (hub, report view, builder, share dialog) builds on these server contracts, all made and tested here.

**Collections** (core migration `2090000008_create_reports.js`; TS types appear in `core/types/pbSchema.ts` at the next workspace-root `pnpm install`, which also links the migration into the dev and e2e servers):

- `reports` (`pbc_reports_01`): `owner` (users, required), `title` (1 to 200), `description` (≤ 2000), `spec` (json, ≤ 1 MiB), `source` (text, the spec's base source key), `published` (bool), `published_by` (users or ""), `broken` (json or null: `{status: 400|422|500, problems: {<spec path>: {code, message}}, more?}`), `shipped_key` ("" or `<package>.<name>`), `shipped_revision` (number), `created`, `updated`.
- `report_shares` (`pbc_report_shares_01`): `report`, `user` (or ""), `group` (or ""), `role` (`viewer` | `editor`). A direct share has `user`; a group grant has `group`; a derived row (both set) is the server's copy of a grant for one member. The share dialog lists rows with `user = "" || group = ""` and shows the group for a grant.

**Who sees and does what** (PocketBase API; statuses as in Ruling P5):

| | owner | editor (direct or via group) | viewer | other member | admin (no share) | guest (no share) | signed out |
|---|---|---|---|---|---|---|---|
| list / view an unpublished report | yes | yes | yes | no | no | no | no |
| list / view a published report | yes | yes | yes | yes | yes | no | no |
| create (owner = me) | yes | yes | yes | yes | yes | yes | no |
| update | yes | yes | no | no | no | no | no |
| delete unpublished / published | yes / yes | no / no | no / no | no / no | no / yes | no / no | no / no |
| add / remove a share | yes | yes | no | no | no | no | no |
| see share rows | all of the report's | all of the report's | own | own | own | own | none |

**Writes and their refusals:**

- Create: `POST /api/collections/reports/records` with `{owner: <my id>, title, description?, spec, published?}`. The server sets `source`, `published_by`, `broken`, `shipped_key`, `shipped_revision`; whatever the client sends for them is replaced.
- `published` changes only for an admin who may update the report: else 403 `data.published.code = "admin_only"`. Publishing sets `published_by` to the admin; unpublishing clears it. The hub shows Publish for `useCurrentRole()` owner or admin.
- A spec that does not validate: 400 `The report is not valid.` or 422 `The report names something that does not exist.`, `data` keyed by spec path exactly as the run endpoint (`columns.0.id`, `sort.0.col`, `source`), so the builder maps both the same way. A spec over 1 MiB: 400 `spec.too_large`. An update that leaves the spec unchanged is not validated (rename a broken report).
- Changing `owner`: 400 `owner.read_only`. Deleting a shipped report while its package ships it: 400 `shipped_key.shipped` (show Unpublish, not Delete, when `shipped_key != ""`).
- Shares: `POST /api/collections/report_shares/records` `{report, user | group, role}`. Refusals: 400 `user.one_target`, 400 `user.owner`, 400 `report.too_many_shares` (200 shares), 400 PocketBase `Value must be unique.` for a second share to the same user or group, 400 `role.validation_invalid_value`. `PATCH` changes only `role` (400 `report|user|group.read_only`). A derived row cannot be changed or removed (400 `group.follows_grant`). "Save as" and "Duplicate" are a plain create with a copied spec.

**Hub queries** (pbtsdb, every collection `on-demand`; register `reports` and `report_shares` in core's collections):

- Mine: `reports` where `owner = me` (`useMyLiveQuery`).
- Shared with me: `report_shares` where `user = me` (`useMyLiveQuery`), `materialize()` the report; a report can have a direct and a derived row for one reader, so de-duplicate by report in JS after the query.
- Published: `reports` where `published = true`.
- Broken: `broken != null`; the badge reads `broken.problems`.

**Runs:** `POST /api/reports/run` with `{reportId, params?, options?}`. 403 `data.reportId.code = "no_access"` means the reader cannot view it (or it does not exist): show "You no longer have access to this report". 422 opens the error state naming the missing part. `topics` include `collection/reports` and `collection/report_shares`: subscribe to every topic the result lists. `{spec, reportId}` together is 400.

**Not in this unit, owned by p5-report-builder:** the source catalog the builder's column tree reads (the previous unit's plan: "p5-report-builder (reads `reports.Sources()`)"), the hub, the views, the share dialog, and the help topic `tinycld/core/help/reports.md`.

**For p5-trial-balance and later package units:** a package ships a published report with `reports.RegisterShipped` from its Go `Register(app)` (Task 7, documented in `docs/packages.md` in Task 10), and raises `Revision` when the report changes.

---

### Task 1: The `reports` and `report_shares` collections and their rules

**Files:**
- Create: `$WS/tinycld/core/server/pb_migrations/2090000008_create_reports.js`
- Modify: `$WS/tinycld/core/server/groups/hooks.go` (export `registerCore` as `BindHooks`)
- Modify: `$WS/tinycld/core/server/groups/expand_test.go:64` (`registerCore(a)` → `BindHooks(a)`)
- Create: `$WS/tinycld/core/server/reports/sharing.go`
- Test: `$WS/tinycld/core/server/reports/saved_fixture_test.go`, `$WS/tinycld/core/server/reports/saved_rules_test.go`

The rules live in the migration. The tests drive every rule cell through PocketBase's API as each kind of reader, with real group expansion. They send `source` in a create body because the hooks that derive it come in Task 2.

**Interfaces:**
- Consumes: the zoo fixture (`newZoo`, `(*zoo).user`, `rec`, `id`, `token`, `mux`, `recs`), `feedingsByDay`, `Register(app)`, `Load`; `groups.RegisterGrantTable`, `groups.ResetForTesting`, `offboard.ResetHandlersForTesting`, `rlstest.RequireAuthGuardOnGrantRules`, `oauth.ScopeForRoute`.
- Produces: `groups.BindHooks(app core.App)`; in `reports`: `const reportsCollection = "reports"`, `const sharesCollection = "report_shares"`, `func RegisterSharing(app core.App)`; test helpers `newSavedZoo(t) *savedZoo` (`savedZoo{*zoo; mux http.Handler}`), `(*zoo).person(t, name, role)`, `(*zoo).group(t, name, members...)`, `(*zoo).saved(t, key, owner, title, spec string, published bool)`, `(*zoo).share(t, key, report, user, group, role string)`, `(*savedZoo).call`, `(*savedZoo).expect(t, user, method, url, body string, want int, parts ...string) string`, `(*savedZoo).listTitles(t, user, url) []string`, `reportURL(id)`, `shareURL(id)`, `reportsURL`, `sharesURL`, `savedSpec`, `readers`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/saved_fixture_test.go`:

```go
package reports

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
	"tinycld.org/core/offboard"
)

// The saved-report zoo: newZoo plus sharing, people in every role, and two
// reports of ann's.
//
//	ann  member, owns R (unpublished) and P (published by ad)
//	ed   member, editor of R (a direct share)
//	vi   member, viewer of R (a direct share) and editor of cy's R2, so an
//	     editor elsewhere and only a viewer on R
//	gm   member of the group keepers; keepers is an editor of R (a grant)
//	ot   member, no share
//	ad   admin, no share
//	gu   guest, no share
//	cy   member, owns R2
type savedZoo struct {
	*zoo
	mux http.Handler
}

// spec of a saved report on the zoo: the cost of every feeding by day.
const savedSpec = feedingsByDay

func newSavedZoo(t *testing.T) *savedZoo {
	t.Helper()
	z := newZoo(t)
	groups.ResetForTesting()
	offboard.ResetHandlersForTesting()
	t.Cleanup(groups.ResetForTesting)
	t.Cleanup(offboard.ResetHandlersForTesting)
	RegisterSharing(z.app)
	groups.BindHooks(z.app)
	Register(z.app)

	for name, role := range map[string]string{"ed": "member", "vi": "member", "gm": "member", "ot": "member", "ad": "admin", "gu": "guest", "cy": "member"} {
		z.person(t, name, role)
	}
	keepers := z.group(t, "keepers", "gm")
	z.saved(t, "R", "ann", "R", savedSpec, false)
	z.saved(t, "P", "ann", "P", savedSpec, true)
	z.saved(t, "R2", "cy", "R2", savedSpec, false)
	z.share(t, "R.ed", "R", "ed", "", "editor")
	z.share(t, "R.vi", "R", "vi", "", "viewer")
	z.share(t, "R.keepers", "R", "", keepers.Id, "editor")
	z.share(t, "R2.vi", "R2", "vi", "", "editor")
	return &savedZoo{zoo: z, mux: z.mux(t)}
}

// person adds a user with a role; z.user adds members.
func (z *zoo) person(t testing.TB, name, role string) *core.Record {
	t.Helper()
	u := z.user(t, name)
	u.Set("role", role)
	if err := z.app.Save(u); err != nil {
		t.Fatalf("set role of %s: %v", name, err)
	}
	return u
}

// group adds a group with members (fixture keys of users).
func (z *zoo) group(t testing.TB, name string, members ...string) *core.Record {
	t.Helper()
	g := z.rec(t, name, "groups", map[string]any{"name": name})
	for _, m := range members {
		z.rec(t, name+"."+m, "group_members", map[string]any{"group": g.Id, "user": z.id(m)})
	}
	return g
}

// saved stores a report directly (no request hooks), as a migration or a
// previous version of the server would have.
func (z *zoo) saved(t testing.TB, key, owner, title, spec string, published bool) *core.Record {
	t.Helper()
	source := "zoo.unreadable" // a spec Load refuses still has a stored source
	if s, _, err := Load([]byte(spec)); err == nil {
		source = s.Source
	}
	data := map[string]any{"owner": z.id(owner), "title": title, "spec": spec, "source": source, "published": published}
	if published {
		data["published_by"] = z.id("ad")
	}
	return z.rec(t, key, reportsCollection, data)
}

// share stores a direct share (user) or a group grant (group).
func (z *zoo) share(t testing.TB, key, report, user, group, role string) *core.Record {
	t.Helper()
	data := map[string]any{"report": z.id(report), "group": group, "role": role}
	if user != "" {
		data["user"] = z.id(user)
	}
	return z.rec(t, key, sharesCollection, data)
}

// call sends one request through the real router as user ("" = signed out)
// and returns the status and body.
func (s *savedZoo) call(t *testing.T, user, method, url, body string) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("Authorization", s.token(t, user))
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// expect is call that fails the test unless the status is want and the body
// holds every part.
func (s *savedZoo) expect(t *testing.T, user, method, url, body string, want int, parts ...string) string {
	t.Helper()
	got, text := s.call(t, user, method, url, body)
	if got != want {
		t.Fatalf("%s %s as %q: status %d, want %d\n%s", method, url, user, got, want, text)
	}
	for _, p := range parts {
		if !strings.Contains(text, p) {
			t.Fatalf("%s %s as %q: body lacks %s\n%s", method, url, user, p, text)
		}
	}
	return text
}

func reportURL(id string) string { return "/api/collections/reports/records/" + id }
func shareURL(id string) string  { return "/api/collections/report_shares/records/" + id }

const (
	reportsURL = "/api/collections/reports/records"
	sharesURL  = "/api/collections/report_shares/records"
)
```

`$WS/tinycld/core/server/reports/saved_rules_test.go`:

```go
package reports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"tinycld.org/core/oauth"
	"tinycld.org/core/rlstest"
)

// Every rule cell of reports and report_shares, through PocketBase's API as
// each kind of reader. PocketBase answers a refused list with fewer items, a
// refused view, update or delete with 404, and a refused create with 400.
var readers = []string{"ann", "ed", "vi", "gm", "ot", "ad", "gu", ""}

func (s *savedZoo) listTitles(t *testing.T, user, url string) []string {
	t.Helper()
	body := s.expect(t, user, http.MethodGet, url+"?perPage=200&sort=title&fields=title", "", http.StatusOK)
	var page struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, it := range page.Items {
		out = append(out, it.Title)
	}
	return out
}

func TestReportRulesListAndView(t *testing.T) {
	s := newSavedZoo(t)
	list := map[string][]string{
		"ann": {"P", "R"}, "ed": {"P", "R"}, "vi": {"P", "R", "R2"}, "gm": {"P", "R"},
		"ot": {"P"}, "ad": {"P"}, "gu": {}, "": {},
	}
	viewR := map[string]int{"ann": 200, "ed": 200, "vi": 200, "gm": 200, "ot": 404, "ad": 404, "gu": 404, "": 404}
	viewP := map[string]int{"ann": 200, "ed": 200, "vi": 200, "gm": 200, "ot": 200, "ad": 200, "gu": 404, "": 404}
	for _, who := range readers {
		if got := s.listTitles(t, who, reportsURL); !slices.Equal(got, list[who]) {
			t.Errorf("list as %q = %v, want %v", who, got, list[who])
		}
		s.expect(t, who, http.MethodGet, reportURL(s.id("R")), "", viewR[who])
		s.expect(t, who, http.MethodGet, reportURL(s.id("P")), "", viewP[who])
	}
}

// A guest sees a report shared with them, and never a published one; a
// disabled owner sees nothing.
func TestReportRulesGuestShareAndDisabledOwner(t *testing.T) {
	s := newSavedZoo(t)
	s.share(t, "R.gu", "R", "gu", "", "viewer")
	if got := s.listTitles(t, "gu", reportsURL); !slices.Equal(got, []string{"R"}) {
		t.Fatalf("guest with a share lists %v, want [R]", got)
	}
	ann := s.recs["ann"]
	ann.Set("disabled", true)
	if err := s.app.Save(ann); err != nil {
		t.Fatal(err)
	}
	if got := s.listTitles(t, "ann", reportsURL); len(got) != 0 {
		t.Fatalf("a disabled owner lists %v", got)
	}
}

func TestReportRulesCreate(t *testing.T) {
	s := newSavedZoo(t)
	want := map[string]int{"ann": 200, "ed": 200, "vi": 200, "gm": 200, "ot": 200, "ad": 200, "gu": 200, "": 400}
	for _, who := range readers {
		owner := ""
		if who != "" {
			owner = s.id(who)
		}
		body := fmt.Sprintf(`{"owner":%q,"title":"mine","source":"zoo.feedings","spec":%s}`, owner, savedSpec)
		s.expect(t, who, http.MethodPost, reportsURL, body, want[who])
	}
	// Nobody creates a report for somebody else.
	s.expect(t, "ad", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"x","source":"zoo.feedings","spec":%s}`, s.id("ann"), savedSpec), 400, `Failed to create record.`)
}

func TestReportRulesUpdate(t *testing.T) {
	s := newSavedZoo(t)
	want := map[string]int{"ann": 200, "ed": 200, "vi": 404, "gm": 200, "ot": 404, "ad": 404, "gu": 404, "": 404}
	for _, who := range readers {
		s.expect(t, who, http.MethodPatch, reportURL(s.id("R")), `{"title":"R"}`, want[who])
	}
	// An admin who neither owns nor edits a published report cannot edit it.
	s.expect(t, "ad", http.MethodPatch, reportURL(s.id("P")), `{"title":"P"}`, 404)
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("P")), `{"title":"P"}`, 200)
}

func TestReportRulesDelete(t *testing.T) {
	s := newSavedZoo(t)
	unpublished := map[string]int{"ann": 204, "ed": 404, "vi": 404, "gm": 404, "ot": 404, "ad": 404, "gu": 404, "": 404}
	published := map[string]int{"ann": 204, "ed": 404, "vi": 404, "gm": 404, "ot": 404, "ad": 204, "gu": 404, "": 404}
	for i, who := range readers {
		for kind, want := range map[string]map[string]int{"unpublished": unpublished, "published": published} {
			key := fmt.Sprintf("del%d%s", i, kind)
			s.saved(t, key, "ann", key, savedSpec, kind == "published")
			s.share(t, key+".ed", key, "ed", "", "editor")
			s.share(t, key+".vi", key, "vi", "", "viewer")
			s.share(t, key+".keepers", key, "", s.id("keepers"), "editor")
			s.expect(t, who, http.MethodDelete, reportURL(s.id(key)), "", want[who])
			_, err := s.app.FindRecordById(reportsCollection, s.id(key))
			if gone := err != nil; gone != (want[who] == 204) {
				t.Fatalf("delete %s as %q: gone = %v", kind, who, gone)
			}
		}
	}
}

func TestShareRulesCreateAndDelete(t *testing.T) {
	s := newSavedZoo(t)
	add := map[string]int{"ann": 200, "ed": 200, "vi": 400, "gm": 200, "ot": 400, "ad": 400, "gu": 400, "": 400}
	remove := map[string]int{"ann": 204, "ed": 204, "vi": 404, "gm": 204, "ot": 404, "ad": 404, "gu": 404, "": 404}
	for i, who := range readers {
		added := s.person(t, fmt.Sprintf("a%d", i), "member")
		s.expect(t, who, http.MethodPost, sharesURL, fmt.Sprintf(`{"report":%q,"user":%q,"role":"viewer"}`, s.id("R"), added.Id), add[who])
		removed := fmt.Sprintf("r%d", i)
		s.person(t, removed, "member")
		row := s.share(t, "R."+removed, "R", removed, "", "viewer")
		s.expect(t, who, http.MethodDelete, shareURL(row.Id), "", remove[who])
		_, err := s.app.FindRecordById(sharesCollection, row.Id)
		if gone := err != nil; gone != (remove[who] == 204) {
			t.Fatalf("remove a share as %q: gone = %v", who, gone)
		}
	}
}

// Managers see every share of the report; anyone else sees only their own
// rows (a group member's derived row included).
func TestShareRulesList(t *testing.T) {
	s := newSavedZoo(t)
	count := func(who string) int {
		body := s.expect(t, who, http.MethodGet, sharesURL+"?perPage=200", "", http.StatusOK)
		var page struct {
			TotalItems int `json:"totalItems"`
		}
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatal(err)
		}
		return page.TotalItems
	}
	// R: ed, vi, the keepers grant and gm's derived row; R2: vi.
	want := map[string]int{"ann": 4, "ed": 4, "vi": 2, "gm": 4, "ot": 0, "ad": 0, "gu": 0, "": 0}
	for _, who := range readers {
		if got := count(who); got != want[who] {
			t.Errorf("shares listed as %q = %d, want %d", who, got, want[who])
		}
	}
}

// No OAuth scope covers saved reports: a report reads across every
// package, so an API token reaches neither collection (default-deny).
func TestSavedReportsAreNotOpenToOAuthTokens(t *testing.T) {
	for _, c := range []string{reportsCollection, sharesCollection} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
			if got := oauth.ScopeForRoute(method, "/api/collections/"+c+"/records"); len(got) != 0 {
				t.Errorf("%s %s resolved to scopes %v, want default-deny", method, c, got)
			}
		}
	}
}

// Every rule that reads report_shares.user also requires a login, so an
// anonymous request's NULL id never matches a grant row's empty user.
// (rlstest's TestCoreMigrations_EveryAccessPathRequiresLogin scans every
// access path of these core rules too.)
func TestReportRulesGuardGrantRows(t *testing.T) {
	s := newSavedZoo(t)
	rlstest.RequireAuthGuardOnGrantRules(t, s.app, sharesCollection)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'TestReportRules|TestShareRules|TestSavedReportsAreNotOpen' ./reports/`
Expected: FAIL to build: `undefined: RegisterSharing`, `undefined: groups.BindHooks`, `undefined: reportsCollection`, `undefined: sharesCollection`.

- [ ] **Step 3: Write the migration**

`$WS/tinycld/core/server/pb_migrations/2090000008_create_reports.js`:

```js
/// <reference path="../pb_data/types.d.ts" />
// Saved reports and their shares (docs/superpowers/specs/2026-10-30-report-engine-design.md,
// "3. Saved reports and sharing").
//
// A report belongs to its owner. A share gives one user, or every member of a
// group, the report's design as viewer or editor; each reader still runs the
// report with their own rights. report_shares is a group grant table
// (core/server/groups): a grant row has `group` set and `user` empty, and the
// expander writes one derived row (both set) per member. So every rule that
// reads report_shares.user starts with the login guard: an anonymous
// request's NULL @request.auth.id matches a grant row's empty user.
//
// The server owns source, published_by, broken, shipped_key and
// shipped_revision, and only an admin may change published: hooks in
// core/server/reports/saved.go enforce that, not these rules.
const SIGNED_IN = '@request.auth.id != "" && @request.auth.disabled != true'
const ADMIN = '(@request.auth.role = "owner" || @request.auth.role = "admin")'
const NOT_GUEST = '@request.auth.role != "guest"'
const EDITOR = 'report_shares_via_report.user ?= @request.auth.id && report_shares_via_report.role ?= "editor"'
const READ = `${SIGNED_IN} && (owner = @request.auth.id || report_shares_via_report.user ?= @request.auth.id || (published = true && ${NOT_GUEST}))`
const SHARE_MANAGER =
    '(report.owner = @request.auth.id || ' +
    '(report.report_shares_via_report.user ?= @request.auth.id && report.report_shares_via_report.role ?= "editor"))'

migrate(
    app => {
        const reports = new Collection({
            id: 'pbc_reports_01',
            name: 'reports',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'reports_owner',
                    name: 'owner',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'reports_title', name: 'title', type: 'text', required: true, min: 1, max: 200 },
                { id: 'reports_description', name: 'description', type: 'text', required: false, max: 2000 },
                { id: 'reports_spec', name: 'spec', type: 'json', required: true, maxSize: 1048576 },
                { id: 'reports_source', name: 'source', type: 'text', required: true, max: 120 },
                { id: 'reports_published', name: 'published', type: 'bool' },
                {
                    id: 'reports_published_by',
                    name: 'published_by',
                    type: 'relation',
                    required: false,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'reports_broken', name: 'broken', type: 'json', required: false, maxSize: 65536 },
                { id: 'reports_shipped_key', name: 'shipped_key', type: 'text', required: false, max: 120 },
                { id: 'reports_shipped_revision', name: 'shipped_revision', type: 'number', required: false, onlyInt: true, min: 0 },
                { id: 'reports_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'reports_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: [
                'CREATE INDEX `idx_reports_owner` ON `reports` (`owner`)',
                'CREATE INDEX `idx_reports_source` ON `reports` (`source`)',
                'CREATE INDEX `idx_reports_published` ON `reports` (`published`)',
                "CREATE UNIQUE INDEX `idx_reports_shipped_key` ON `reports` (`shipped_key`) WHERE `shipped_key` != ''",
            ],
        })
        app.save(reports)

        const shares = new Collection({
            id: 'pbc_report_shares_01',
            name: 'report_shares',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'report_shares_report',
                    name: 'report',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_reports_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'report_shares_user',
                    name: 'user',
                    type: 'relation',
                    required: false,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'report_shares_group',
                    name: 'group',
                    type: 'relation',
                    required: false,
                    collectionId: 'pbc_groups_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                { id: 'report_shares_role', name: 'role', type: 'select', required: true, maxSelect: 1, values: ['viewer', 'editor'] },
                { id: 'report_shares_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'report_shares_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: [
                // groups.GrantTable requires (resource, user, group) unique.
                'CREATE UNIQUE INDEX `idx_report_shares_unique` ON `report_shares` (`report`, `user`, `group`)',
                'CREATE INDEX `idx_report_shares_user` ON `report_shares` (`user`)',
                'CREATE INDEX `idx_report_shares_group` ON `report_shares` (`group`)',
            ],
        })
        app.save(shares)

        // The rules read the back-relation report_shares_via_report, so they
        // are set once both collections exist.
        const shared = app.findCollectionByNameOrId('report_shares')
        shared.listRule = `${SIGNED_IN} && (user = @request.auth.id || ${SHARE_MANAGER})`
        shared.viewRule = shared.listRule
        shared.createRule = `${SIGNED_IN} && ${SHARE_MANAGER}`
        shared.updateRule = shared.createRule
        shared.deleteRule = shared.createRule
        app.save(shared)

        const saved = app.findCollectionByNameOrId('reports')
        saved.listRule = READ
        saved.viewRule = READ
        saved.createRule = `${SIGNED_IN} && owner = @request.auth.id`
        saved.updateRule = `${SIGNED_IN} && (owner = @request.auth.id || (${EDITOR}))`
        saved.deleteRule = `${SIGNED_IN} && (owner = @request.auth.id || (published = true && ${ADMIN}))`
        app.save(saved)
    },
    app => {
        for (const name of ['report_shares', 'reports']) {
            try {
                app.delete(app.findCollectionByNameOrId(name))
            } catch (e) {
                // may not exist
            }
        }
    }
)
```

- [ ] **Step 4: Export the group expansion hooks**

In `$WS/tinycld/core/server/groups/hooks.go`, in `Register`, replace `registerCore(app)` with `BindHooks(app)`. Replace the comment and signature of `registerCore`:

```go
// registerCore is the core.App-typed body so tests can bind it on a
// *tests.TestApp. The REST save path (apis/record_crud.go,
```

with:

```go
// BindHooks binds the expansion hooks for group_members, users and every
// grant table registered so far, without the boot reconcile. It is the
// core.App-typed body of Register, so a test of a grant table outside this
// package binds it on a *tests.TestApp, after RegisterGrantTable, to drive
// real expansion.
//
// The REST save path (apis/record_crud.go,
```

and `func registerCore(app core.App) {` with `func BindHooks(app core.App) {`. In `$WS/tinycld/core/server/groups/expand_test.go` line 64, replace `registerCore(a)` with `BindHooks(a)`.

- [ ] **Step 5: Write `sharing.go`**

`$WS/tinycld/core/server/reports/sharing.go` (later tasks add the hooks this comment names):

```go
package reports

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
)

// The saved-report collections (core migration 2090000008_create_reports.js).
const (
	reportsCollection = "reports"
	sharesCollection  = "report_shares"
)

// RegisterSharing binds saved reports and their sharing: report_shares as a
// group grant table, the save and publish guards on reports, the share
// guards on report_shares, shipped reports for the first admin who appears,
// and the offboard handler. Call it before groups.Register, which binds the
// expansion of every grant table registered by then.
//
// The collections' own rules (the migration) decide who may read and write
// a row; these hooks guard what a permitted write may change. A superuser
// request passes them unchanged: a superuser can change any row anyway.
func RegisterSharing(app core.App) {
	groups.RegisterGrantTable(groups.GrantTable{Collection: sharesCollection, ResourceField: "report"})
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports groups && go vet -tags no_ui ./reports/ ./groups/ && go test -tags no_ui -count=1 -run 'TestReportRules|TestShareRules|TestSavedReportsAreNotOpen' ./reports/ && go test -tags no_ui -count=1 ./groups/ ./rlstest/`
Expected: no gofmt or vet output; `ok` for `reports`, `groups` and `rlstest` (`TestCoreMigrations_EveryAccessPathRequiresLogin` now scans the new rules).

- [ ] **Step 7: Run the quality gate**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet -tags no_ui ./... && go test -tags no_ui -count=1 ./...`
Expected: exit 0.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts`
Expected: Biome, tsc and vitest pass; `0 reference(s)`.

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/pb_migrations/2090000008_create_reports.js core/server/groups/hooks.go \
  core/server/groups/expand_test.go core/server/reports/sharing.go \
  core/server/reports/saved_fixture_test.go core/server/reports/saved_rules_test.go
git commit -m "feat(core): saved report collections, their rules and group shares"
```

---

### Task 2: Save guards on `reports`: the spec, `published`, server-owned fields

**Files:**
- Create: `$WS/tinycld/core/server/reports/saved.go`
- Modify: `$WS/tinycld/core/server/reports/sharing.go` (bind two hooks)
- Test: `$WS/tinycld/core/server/reports/saved_test.go`

**Interfaces:**
- Consumes: `Load`, `Validate`, `*SpecError`, `maxRunBody`, `log`; the Task 1 fixture.
- Produces: `const maxSpecBytes = maxRunBody`; `var serverOwned []string`; `func isActiveAdmin(user *core.Record) bool`; `func fieldError(at, code, message string) map[string]validation.Error`; `const adminOnly`; `onReportCreate`, `onReportUpdate(e *core.RecordRequestEvent) error`; `prepareSpec(e *core.RecordRequestEvent, original *core.Record) error`; `func specBytes(r *core.Record) []byte`; `sameSpec(raw, stored []byte) bool`; `specRefusal(e *core.RecordRequestEvent, err error) error`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/saved_test.go`:

```go
package reports

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func createBody(owner, extra string) string {
	return fmt.Sprintf(`{"owner":%q,"title":"New","spec":%s%s}`, owner, savedSpec, extra)
}

func TestOnlyAnAdminPublishes(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodPost, reportsURL, createBody(s.id("ann"), `,"published":true`),
		http.StatusForbidden, `"published":{"code":"admin_only"`)
	s.expect(t, "ad", http.MethodPost, reportsURL, createBody(s.id("ad"), `,"published":true`),
		http.StatusOK, `"published":true`, fmt.Sprintf(`"published_by":%q`, s.id("ad")))

	// ed edits R but is no admin; ad is an admin and owns its own report.
	s.expect(t, "ed", http.MethodPatch, reportURL(s.id("R")), `{"published":true}`,
		http.StatusForbidden, `"published":{"code":"admin_only"`)
	mine := s.saved(t, "adR", "ad", "adR", savedSpec, false)
	s.expect(t, "ad", http.MethodPatch, reportURL(mine.Id), `{"published":true}`,
		http.StatusOK, `"published":true`, fmt.Sprintf(`"published_by":%q`, s.id("ad")))
	s.expect(t, "ad", http.MethodPatch, reportURL(mine.Id), `{"published":false}`,
		http.StatusOK, `"published":false`, `"published_by":""`)
	// The owner, no admin, cannot unpublish P either.
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("P")), `{"published":false}`,
		http.StatusForbidden, `"published":{"code":"admin_only"`)
	// A save that leaves published as it was is no publish.
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("P")), `{"published":true,"title":"P2"}`, http.StatusOK, `"title":"P2"`)
}

// The server owns source, published_by, broken and the shipped fields: a
// client value is replaced, on create and on update.
func TestTheServerOwnsItsFields(t *testing.T) {
	s := newSavedZoo(t)
	lies := fmt.Sprintf(`,"source":"zoo.lies","published_by":%q,"broken":{"status":1},"shipped_key":"zoo.fake","shipped_revision":9`, s.id("ad"))
	s.expect(t, "ann", http.MethodPost, reportsURL, createBody(s.id("ann"), lies), http.StatusOK,
		`"source":"zoo.feedings"`, `"published_by":""`, `"broken":null`, `"shipped_key":""`, `"shipped_revision":0`)
	s.expect(t, "ed", http.MethodPatch, reportURL(s.id("R")), `{"source":"zoo.lies","shipped_key":"zoo.fake","shipped_revision":3,"broken":{"status":1}}`,
		http.StatusOK, `"source":"zoo.feedings"`, `"shipped_key":""`, `"shipped_revision":0`, `"broken":null`)
	s.expect(t, "ed", http.MethodPatch, reportURL(s.id("R")), fmt.Sprintf(`{"owner":%q}`, s.id("ed")),
		http.StatusBadRequest, `"owner":{"code":"read_only"`)
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("R")), fmt.Sprintf(`{"owner":%q}`, s.id("ann")), http.StatusOK)
}

// A spec is checked on save as on run, and saved as Load returns it: an
// aliased field is stored under its current key.
func TestASpecIsCheckedAndSavedLoaded(t *testing.T) {
	s := newSavedZoo(t)
	ann := s.id("ann")
	s.expect(t, "ann", http.MethodPost, reportsURL,
		fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"source":"zoo.feedings","columns":[{"id":"1x","path":[{"field":"cost"}]}]}}`, ann),
		http.StatusBadRequest, `"message":"The report is not valid."`, `"columns.0.id":{"code":"invalid"`)
	s.expect(t, "ann", http.MethodPost, reportsURL,
		fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}}`, ann),
		http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":2}}`, ann),
		http.StatusBadRequest, `"version":{"code":"invalid"`)
	body := s.expect(t, "ann", http.MethodPost, reportsURL,
		fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"source":"zoo.animals","columns":[{"id":"m","path":[{"field":"mass"}],"aggregate":"sum"}]}}`, ann),
		http.StatusOK, `"source":"zoo.animals"`)
	if !strings.Contains(body, `"field":"weight"`) || strings.Contains(body, `"mass"`) {
		t.Fatalf("the alias was not saved resolved: %s", body)
	}
}

// An update that keeps a broken spec keeps it, so a broken report can be
// renamed; a changed spec must be valid, and clears the mark.
func TestABrokenReportCanBeRenamedNotSavedBroken(t *testing.T) {
	s := newSavedZoo(t)
	gone := `{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}`
	b := s.saved(t, "B", "ann", "B", gone, false)
	b.Set("broken", `{"status":422}`)
	if err := s.app.Save(b); err != nil {
		t.Fatal(err)
	}
	s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"title":"B renamed"}`, http.StatusOK, `"title":"B renamed"`, `"broken":{"status":422}`)
	s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"spec":{"version":1,"source":"zoo.gone","columns":[{"id":"b","path":[{"field":"cost"}]}]}}`,
		http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"spec":`+savedSpec+`}`, http.StatusOK, `"broken":null`, `"source":"zoo.feedings"`)
}

// Every input that grows a saved report is capped, and a value past a cap
// answers 400, not 500.
func TestSavedReportCaps(t *testing.T) {
	s := newSavedZoo(t)
	ann := s.id("ann")
	big := fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"x":"%s"}}`, ann, strings.Repeat("x", maxSpecBytes))
	s.expect(t, "ann", http.MethodPost, reportsURL, big, http.StatusBadRequest, `"spec":{"code":"too_large"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":%q,"spec":%s}`, ann, strings.Repeat("t", 201), savedSpec),
		http.StatusBadRequest, `"title":{"code":"validation_max_text_constraint"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"x","description":%q,"spec":%s}`, ann, strings.Repeat("d", 2001), savedSpec),
		http.StatusBadRequest, `"description":{"code":"validation_max_text_constraint"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"","spec":%s}`, ann, savedSpec),
		http.StatusBadRequest, `"title":{"code":"validation_required"`)
}

// A superuser request is not guarded: it may set any field.
func TestASuperuserIsNotGuarded(t *testing.T) {
	s := newSavedZoo(t)
	superusers, err := s.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(superusers)
	su.SetEmail("root@zoo.test")
	su.SetPassword("password123")
	if err := s.app.Save(su); err != nil {
		t.Fatal(err)
	}
	s.recs["root"] = su
	s.expect(t, "root", http.MethodPatch, reportURL(s.id("R")), `{"published":true,"shipped_key":"zoo.fixed"}`,
		http.StatusOK, `"published":true`, `"shipped_key":"zoo.fixed"`)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'TestOnlyAnAdmin|TestTheServerOwns|TestASpecIs|TestABroken|TestSavedReportCaps|TestASuperuser' ./reports/`
Expected: FAIL to build: `undefined: maxSpecBytes`.

- [ ] **Step 3: Write `saved.go`**

`$WS/tinycld/core/server/reports/saved.go`:

```go
package reports

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// maxSpecBytes bounds a saved spec. It is the size of a draft run's whole
// body, so every spec the builder can run, it can save. The migration's json
// maxSize is the same number.
const maxSpecBytes = maxRunBody

// serverOwned are the reports fields only the server writes. A client value
// for any of them is replaced before the save.
var serverOwned = []string{"source", "published_by", "broken", "shipped_key", "shipped_revision"}

// isActiveAdmin reports a users record whose role is owner or admin and that
// is not disabled: the reader the rules call an admin.
func isActiveAdmin(user *core.Record) bool {
	if user == nil || user.GetBool("disabled") {
		return false
	}
	role := user.GetString("role")
	return role == "owner" || role == "admin"
}

func fieldError(at, code, message string) map[string]validation.Error {
	return map[string]validation.Error{at: validation.NewError(code, message)}
}

const adminOnly = "Only an admin can publish or unpublish a report."

func onReportCreate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	for _, f := range serverOwned {
		e.Record.Set(f, nil)
	}
	if err := prepareSpec(e, nil); err != nil {
		return err
	}
	if e.Record.GetBool("published") {
		if !isActiveAdmin(e.Auth) {
			return e.ForbiddenError(adminOnly, fieldError("published", "admin_only", adminOnly))
		}
		e.Record.Set("published_by", e.Auth.Id)
	}
	return e.Next()
}

func onReportUpdate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	original := e.Record.Original()
	if e.Record.GetString("owner") != original.GetString("owner") {
		return e.BadRequestError("A report's owner does not change.",
			fieldError("owner", "read_only", "A report's owner does not change."))
	}
	for _, f := range serverOwned {
		e.Record.Set(f, original.Get(f))
	}
	if err := prepareSpec(e, original); err != nil {
		return err
	}
	published := e.Record.GetBool("published")
	if published != original.GetBool("published") {
		if !isActiveAdmin(e.Auth) {
			return e.ForbiddenError(adminOnly, fieldError("published", "admin_only", adminOnly))
		}
		by := ""
		if published {
			by = e.Auth.Id
		}
		e.Record.Set("published_by", by)
	}
	return e.Next()
}

// prepareSpec reads the posted spec as a run reads it and stores it as Load
// returns it: upgraded to the current version, aliases resolved. That is how
// a spec is "saved upgraded on its next save". source becomes the spec's
// base source. A new spec, or a changed one, must pass Validate, and then
// the report is no longer broken. An update that leaves the spec as it was
// keeps it, valid or not, so a broken report can still be renamed.
func prepareSpec(e *core.RecordRequestEvent, original *core.Record) error {
	raw := specBytes(e.Record)
	if len(raw) > maxSpecBytes {
		msg := fmt.Sprintf("A report spec has at most %d bytes.", maxSpecBytes)
		return e.BadRequestError(msg, fieldError("spec", "too_large", msg))
	}
	spec, _, err := Load(raw)
	if err != nil {
		return specRefusal(e, err)
	}
	stored, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	if original == nil || !sameSpec(specBytes(original), stored) {
		if err := Validate(spec); err != nil {
			return specRefusal(e, err)
		}
		e.Record.Set("broken", nil)
	}
	e.Record.Set("spec", types.JSONRaw(stored))
	e.Record.Set("source", spec.Source)
	return nil
}

// specBytes is a report's stored or posted spec.
func specBytes(r *core.Record) []byte {
	raw, _ := r.Get("spec").(types.JSONRaw)
	return raw
}

// sameSpec reports whether the stored spec, loaded, is the loaded spec
// about to be stored.
func sameSpec(raw, stored []byte) bool {
	spec, _, err := Load(raw)
	if err != nil {
		return false
	}
	before, err := json.Marshal(spec)
	return err == nil && bytes.Equal(before, stored)
}

// specRefusal answers a spec that does not load or validate exactly as the
// run endpoint answers it: 400 with each invalid part, 422 naming each
// unknown source, link, field or column, keyed by its spec path.
func specRefusal(e *core.RecordRequestEvent, err error) error {
	var specErr *SpecError
	if errors.As(err, &specErr) {
		if specErr.Status == http.StatusUnprocessableEntity {
			return e.Error(http.StatusUnprocessableEntity, "The report names something that does not exist.", specErr.Problems)
		}
		return e.BadRequestError("The report is not valid.", specErr.Problems)
	}
	log.ErrorContext(e.Request.Context(), "report spec check failed", "err", err)
	return e.InternalServerError("The report could not be saved.", nil)
}
```

- [ ] **Step 4: Bind the hooks**

Replace `$WS/tinycld/core/server/reports/sharing.go` with:

```go
package reports

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
)

// The saved-report collections (core migration 2090000008_create_reports.js).
const (
	reportsCollection = "reports"
	sharesCollection  = "report_shares"
)

// RegisterSharing binds saved reports and their sharing: report_shares as a
// group grant table, the save and publish guards on reports, the share
// guards on report_shares, shipped reports for the first admin who appears,
// and the offboard handler. Call it before groups.Register, which binds the
// expansion of every grant table registered by then.
//
// The collections' own rules (the migration) decide who may read and write
// a row; these hooks guard what a permitted write may change. A superuser
// request passes them unchanged: a superuser can change any row anyway.
func RegisterSharing(app core.App) {
	groups.RegisterGrantTable(groups.GrantTable{Collection: sharesCollection, ResourceField: "report"})
	app.OnRecordCreateRequest(reportsCollection).BindFunc(onReportCreate)
	app.OnRecordUpdateRequest(reportsCollection).BindFunc(onReportUpdate)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt or vet output; `ok  tinycld.org/core/reports` (the Task 1 rule tests still pass with the hooks bound).

- [ ] **Step 6: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/saved.go core/server/reports/sharing.go core/server/reports/saved_test.go
git commit -m "feat(core): check a saved report's spec on save and keep publishing for admins"
```

---

### Task 3: Share guards on `report_shares`

**Files:**
- Create: `$WS/tinycld/core/server/reports/shares.go`
- Modify: `$WS/tinycld/core/server/reports/sharing.go` (bind three hooks)
- Test: `$WS/tinycld/core/server/reports/shares_test.go`

**Interfaces:**
- Consumes: `fieldError`, `reportsCollection`, `sharesCollection`; the Task 1 fixture (`listTitles`).
- Produces: `var shareCap = 200`; `func isDerivedShare(r *core.Record) bool`; `onShareCreate`, `onShareUpdate`, `onShareDelete(e *core.RecordRequestEvent) error`; `refuseDerived`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/shares_test.go`:

```go
package reports

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

func shareBody(report, user, group, role string) string {
	return fmt.Sprintf(`{"report":%q,"user":%q,"group":%q,"role":%q}`, report, user, group, role)
}

func TestAShareNamesOneUserOrOneGroupNotTheOwner(t *testing.T) {
	s := newSavedZoo(t)
	r := s.id("R")
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, "", "", "viewer"), http.StatusBadRequest, `"user":{"code":"one_target"`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ot"), s.id("keepers"), "viewer"), http.StatusBadRequest, `"user":{"code":"one_target"`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ann"), "", "viewer"), http.StatusBadRequest, `"user":{"code":"owner"`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ed"), "", "viewer"), http.StatusBadRequest, `Value must be unique`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ot"), "", "owner"), http.StatusBadRequest, `"role":{"code":"validation_invalid_value"`)
}

// A share changes only its role; a member's derived row follows its grant
// and is neither edited nor removed alone.
func TestAShareChangesOnlyItsRole(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.vi")), `{"role":"editor"}`, http.StatusOK, `"role":"editor"`)
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.vi")), fmt.Sprintf(`{"user":%q}`, s.id("ot")), http.StatusBadRequest, `"user":{"code":"read_only"`)
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.vi")), fmt.Sprintf(`{"report":%q}`, s.id("P")), http.StatusBadRequest, `"report":{"code":"read_only"`)
	derived, err := s.app.FindFirstRecordByFilter(sharesCollection, "user = {:u} && group = {:g}", map[string]any{"u": s.id("gm"), "g": s.id("keepers")})
	if err != nil {
		t.Fatal(err)
	}
	s.expect(t, "ann", http.MethodPatch, shareURL(derived.Id), `{"role":"viewer"}`, http.StatusBadRequest, `"group":{"code":"follows_grant"`)
	s.expect(t, "ann", http.MethodDelete, shareURL(derived.Id), "", http.StatusBadRequest, `"group":{"code":"follows_grant"`)
	// The grant's role reaches the member's row.
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.keepers")), `{"role":"viewer"}`, http.StatusOK)
	if got, _ := s.app.FindRecordById(sharesCollection, derived.Id); got.GetString("role") != "viewer" {
		t.Fatalf("the derived row kept role %q", got.GetString("role"))
	}
	s.expect(t, "gm", http.MethodPatch, reportURL(s.id("R")), `{"title":"R"}`, http.StatusNotFound)
}

// A group grant reaches each member through the group expander: a member
// who joins gets the report, one who leaves loses it, and removing the
// grant removes every member's row.
func TestAGroupShareFollowsTheGroup(t *testing.T) {
	s := newSavedZoo(t)
	s.person(t, "nm", "member")
	if got := s.listTitles(t, "nm", reportsURL); !slices.Equal(got, []string{"P"}) {
		t.Fatalf("before joining: %v", got)
	}
	s.rec(t, "keepers.nm", "group_members", map[string]any{"group": s.id("keepers"), "user": s.id("nm")})
	if got := s.listTitles(t, "nm", reportsURL); !slices.Equal(got, []string{"P", "R"}) {
		t.Fatalf("after joining: %v", got)
	}
	if err := s.app.Delete(s.recs["keepers.gm"]); err != nil {
		t.Fatal(err)
	}
	if got := s.listTitles(t, "gm", reportsURL); !slices.Equal(got, []string{"P"}) {
		t.Fatalf("after leaving: %v", got)
	}
	s.expect(t, "ann", http.MethodDelete, shareURL(s.id("R.keepers")), "", http.StatusNoContent)
	if n, _ := s.app.CountRecords(sharesCollection); n != 3 {
		t.Fatalf("after the grant went: %d share rows, want 3 (R.ed, R.vi, R2.vi)", n)
	}
}

// Deleting a report deletes every share row it had: direct, grant and
// derived.
func TestDeletingAReportDeletesItsShares(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodDelete, reportURL(s.id("R")), "", http.StatusNoContent)
	rows, err := s.app.FindRecordsByFilter(sharesCollection, "report = {:r}", "", 0, 0, map[string]any{"r": s.id("R")})
	if err != nil || len(rows) != 0 {
		t.Fatalf("shares left: %d (%v)", len(rows), err)
	}
}

// A report holds at most shareCap shares (direct and grants). The rows a
// grant derives do not count, and the next share past the cap is a 400.
func TestTheShareCapAnswers400(t *testing.T) {
	s := newSavedZoo(t)
	saved := shareCap
	shareCap = 4
	t.Cleanup(func() { shareCap = saved })
	// R has 3: R.ed, R.vi and the keepers grant (gm's derived row is not one).
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(s.id("R"), s.id("ot"), "", "viewer"), http.StatusOK)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(s.id("R"), s.id("cy"), "", "viewer"),
		http.StatusBadRequest, `"report":{"code":"too_many_shares"`)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'TestAShare|TestAGroupShare|TestDeletingAReport|TestTheShareCap' ./reports/`
Expected: FAIL to build: `undefined: shareCap`.

- [ ] **Step 3: Write `shares.go`**

`$WS/tinycld/core/server/reports/shares.go`:

```go
package reports

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// shareCap bounds the shares one report holds: direct shares and group
// grants. The rows a grant derives, one per member, are bounded by the
// group, which an admin manages. Two shares added at the same moment can
// pass the count together, so the cap can be passed by the number of
// concurrent requests, never without bound.
var shareCap = 200

// A report_shares row is one of three kinds, as groups.GrantTable defines
// them: direct (user set), a group grant (group set), or derived (both set,
// written by the group expander for each member of a granted group).
func isDerivedShare(r *core.Record) bool {
	return r.GetString("user") != "" && r.GetString("group") != ""
}

func onShareCreate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	user, group := e.Record.GetString("user"), e.Record.GetString("group")
	if (user == "") == (group == "") {
		const msg = "A share names one user or one group."
		return e.BadRequestError(msg, fieldError("user", "one_target", msg))
	}
	// The create rule passed, so the report exists and the caller manages it.
	report, err := e.App.FindRecordById(reportsCollection, e.Record.GetString("report"))
	if err != nil {
		return err
	}
	if user == report.GetString("owner") {
		const msg = "The owner already has this report."
		return e.BadRequestError(msg, fieldError("user", "owner", msg))
	}
	n, err := e.App.CountRecords(sharesCollection, dbx.NewExp(
		"[[report]] = {:report} AND ([[user]] = '' OR [[group]] = '')", dbx.Params{"report": report.Id}))
	if err != nil {
		return err
	}
	if n >= int64(shareCap) {
		msg := fmt.Sprintf("A report can be shared at most %d times.", shareCap)
		return e.BadRequestError(msg, fieldError("report", "too_many_shares", msg))
	}
	return e.Next()
}

func onShareUpdate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	original := e.Record.Original()
	if isDerivedShare(original) {
		return refuseDerived(e)
	}
	for _, f := range []string{"report", "user", "group"} {
		if e.Record.GetString(f) != original.GetString(f) {
			const msg = "A share changes only its role. Remove it and add another."
			return e.BadRequestError(msg, fieldError(f, "read_only", msg))
		}
	}
	return e.Next()
}

func onShareDelete(e *core.RecordRequestEvent) error {
	if !e.HasSuperuserAuth() && isDerivedShare(e.Record) {
		return refuseDerived(e)
	}
	return e.Next()
}

// refuseDerived: a member's row follows its group's grant. Editing or
// removing it alone would be undone at the next boot reconcile.
func refuseDerived(e *core.RecordRequestEvent) error {
	const msg = "This access comes from a group. Change the group's share instead."
	return e.BadRequestError(msg, fieldError("group", "follows_grant", msg))
}
```

- [ ] **Step 4: Bind the hooks**

Replace `$WS/tinycld/core/server/reports/sharing.go` with:

```go
package reports

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
)

// The saved-report collections (core migration 2090000008_create_reports.js).
const (
	reportsCollection = "reports"
	sharesCollection  = "report_shares"
)

// RegisterSharing binds saved reports and their sharing: report_shares as a
// group grant table, the save and publish guards on reports, the share
// guards on report_shares, shipped reports for the first admin who appears,
// and the offboard handler. Call it before groups.Register, which binds the
// expansion of every grant table registered by then.
//
// The collections' own rules (the migration) decide who may read and write
// a row; these hooks guard what a permitted write may change. A superuser
// request passes them unchanged: a superuser can change any row anyway.
func RegisterSharing(app core.App) {
	groups.RegisterGrantTable(groups.GrantTable{Collection: sharesCollection, ResourceField: "report"})
	app.OnRecordCreateRequest(reportsCollection).BindFunc(onReportCreate)
	app.OnRecordUpdateRequest(reportsCollection).BindFunc(onReportUpdate)
	app.OnRecordCreateRequest(sharesCollection).BindFunc(onShareCreate)
	app.OnRecordUpdateRequest(sharesCollection).BindFunc(onShareUpdate)
	app.OnRecordDeleteRequest(sharesCollection).BindFunc(onShareDelete)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt or vet output; `ok  tinycld.org/core/reports`.

- [ ] **Step 6: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/shares.go core/server/reports/sharing.go core/server/reports/shares_test.go
git commit -m "feat(core): guard report shares: one target, role-only edits, derived rows, a cap"
```

---

### Task 4: Run a saved report: `{ reportId }` and its 403

**Files:**
- Modify: `$WS/tinycld/core/server/reports/endpoint.go` (import, `Register` comment, `runBody`, `handleRun`; new `viewableSpec`, `withSavedTopics`)
- Test: `$WS/tinycld/core/server/reports/endpoint_saved_test.go`

**Interfaces:**
- Consumes: `Load`, `Run`, `runError`, `ruleHeaders`, `listRequestInfo(auth, headers)` (rules.go), `fieldError`, `specBytes`, `changesignal.Topic`, `app.CanAccessRecord(record, requestInfo, rule)`.
- Produces: `runBody{Spec, ReportID, Params, Options}` (JSON `reportId`); `const noAccess`; `viewableSpec(re *core.RequestEvent, id string) ([]byte, error)`; `withSavedTopics(topics []string) []string`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/endpoint_saved_test.go`:

```go
package reports

import (
	"fmt"
	"net/http"
	"testing"
)

const runURL = "/api/reports/run"

func savedRun(id string) string { return fmt.Sprintf(`{"reportId":%q}`, id) }

const savedTopics = `"topics":["collection/report_shares","collection/reports","collection/zoo_animals","collection/zoo_feedings","collection/zoo_keepers"]`

// A saved report runs for anyone who can view it, each with their own
// rights: sharing gives the design, never the data.
func TestASavedReportRunsAsItsReader(t *testing.T) {
	s := newSavedZoo(t)
	s.share(t, "R.bob", "R", "bob", "", "viewer")
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK, `"kind":"total"`, `"cost":4500`, savedTopics)
	s.expect(t, "bob", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK, `"cost":1150`)
	// bob's own draft run of the same spec gives the same figure.
	s.expect(t, "bob", http.MethodPost, runURL, runBodyOf(savedSpec), http.StatusOK, `"cost":1150`)
	// A published report runs for every member, and params and options pass:
	// bob keeps three feedings and asks for a page of one.
	s.expect(t, "bob", http.MethodPost, runURL, fmt.Sprintf(`{"reportId":%q,"params":{},"options":{"limit":1}}`, s.id("P")), http.StatusOK, `"size":1`)
	s.expect(t, "ot", http.MethodPost, runURL, savedRun(s.id("P")), http.StatusOK, `"kind":"total"`)
}

func TestASavedRunTheCallerCannotViewIs403(t *testing.T) {
	s := newSavedZoo(t)
	noAccess := `"reportId":{"code":"no_access"`
	s.expect(t, "ot", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusForbidden, noAccess)
	s.expect(t, "gu", http.MethodPost, runURL, savedRun(s.id("P")), http.StatusForbidden, noAccess)
	// An id that does not exist answers the same: an id tells nobody whether a report exists.
	s.expect(t, "ann", http.MethodPost, runURL, savedRun("nosuchreport00"), http.StatusForbidden, noAccess)
	s.expect(t, "", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusUnauthorized)
	// A share removed while the report is open: the next run is refused.
	s.expect(t, "vi", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK)
	if err := s.app.Delete(s.recs["R.vi"]); err != nil {
		t.Fatal(err)
	}
	s.expect(t, "vi", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusForbidden, noAccess)
}

func TestASavedRunBody(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodPost, runURL, fmt.Sprintf(`{"reportId":%q,"spec":%s}`, s.id("R"), savedSpec),
		http.StatusBadRequest, `not both`)
	s.expect(t, "ann", http.MethodPost, runURL, `{"reportId":7}`, http.StatusBadRequest, `not a report run`)
	s.expect(t, "ann", http.MethodPost, runURL, `{"params":{}}`, http.StatusBadRequest, `no spec`)
}

// A saved report that names something gone opens in the error state: 422
// naming the missing part. One stored with an old field key runs, and says so.
func TestASavedReportNamesWhatIsMissing(t *testing.T) {
	s := newSavedZoo(t)
	gone := s.saved(t, "gone", "ann", "gone", `{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}`, false)
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(gone.Id), http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	old := s.saved(t, "old", "ann", "old", `{"version":1,"source":"zoo.animals","columns":[{"id":"m","path":[{"field":"mass"}],"aggregate":"sum"}]}`, false)
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(old.Id), http.StatusOK, `"code":"aliased"`, `"from":"mass"`)
}

// A saved run reads and writes nothing it should not: no report, no share
// and no feeding changes.
func TestASavedRunWritesNothing(t *testing.T) {
	s := newSavedZoo(t)
	before, _ := s.app.FindRecordById(reportsCollection, s.id("R"))
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK)
	after, _ := s.app.FindRecordById(reportsCollection, s.id("R"))
	if before.GetString("updated") != after.GetString("updated") {
		t.Fatal("a run changed the report")
	}
	if n, _ := s.app.CountRecords("zoo_feedings"); n != 6 {
		t.Fatalf("the run wrote: %d feedings", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'TestASaved|TestRunEndpoint' ./reports/`
Expected: FAIL: `TestASavedReportRunsAsItsReader`: `status 400, want 200 … json: unknown field "reportId"`; `TestASavedRunTheCallerCannotViewIs403`: `status 400, want 403`; `TestASavedRunBody`: `body lacks not both`.

- [ ] **Step 3: Change `endpoint.go`**

In `$WS/tinycld/core/server/reports/endpoint.go`:

1. Add `"tinycld.org/core/changesignal"` to the imports, before `"tinycld.org/core/logging"`.
2. In the comment on `Register`, replace:

```go
// refuses, so a broken source never reaches a user. It serves
// POST /api/reports/run to signed-in users.
```

with:

```go
// refuses, so a broken source never reaches a user. It serves
// POST /api/reports/run to signed-in users: a draft spec, or a saved report
// the caller can view. Saved reports and their shares are bound by
// RegisterSharing.
```

3. Replace the `runBody` type and the whole `handleRun` function (from `// runBody is a draft report run: a spec, never SQL.` to the end of `handleRun`) with:

```go
// runBody is one report run: a draft spec, or the id of a saved report the
// caller can view. Never SQL.
type runBody struct {
	Spec     json.RawMessage            `json:"spec"`
	ReportID string                     `json:"reportId"`
	Params   map[string]json.RawMessage `json:"params"`
	Options  Options                    `json:"options"`
}

func handleRun(re *core.RequestEvent) error {
	var body runBody
	dec := json.NewDecoder(re.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return re.BadRequestError("The request is not a report run: "+err.Error(), nil)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return re.BadRequestError("The request is not a report run: it has data after the first object.", nil)
	}
	raw := []byte(body.Spec)
	saved := body.ReportID != ""
	switch {
	case saved && body.Spec != nil:
		return re.BadRequestError("The request has a spec or a reportId, not both.", nil)
	case saved:
		var err error
		if raw, err = viewableSpec(re, body.ReportID); err != nil {
			return err
		}
	case body.Spec == nil:
		return re.BadRequestError("The request has no spec and no reportId.", nil)
	}
	spec, loaded, err := Load(raw)
	if err != nil {
		return runError(re, err)
	}
	res, err := Run(re.Request.Context(), re.App, Request{
		Spec: spec, Loaded: loaded, Params: body.Params, Options: body.Options,
		Auth: re.Auth, Headers: ruleHeaders(re.Request.Header),
	})
	if err != nil {
		return runError(re, err)
	}
	if saved {
		res.Topics = withSavedTopics(res.Topics)
	}
	return re.JSON(http.StatusOK, res)
}

const noAccess = "You cannot view this report."

// viewableSpec reads a saved report's spec for the caller, through the
// reports view rule as PocketBase's view API applies it. A report that does
// not exist answers as one the caller cannot view, so an id tells nobody
// whether a report exists.
func viewableSpec(re *core.RequestEvent, id string) ([]byte, error) {
	refuse := re.ForbiddenError(noAccess, fieldError("reportId", "no_access", noAccess))
	col, err := re.App.FindCachedCollectionByNameOrId(reportsCollection)
	if err != nil {
		return nil, err
	}
	rec, err := re.App.FindRecordById(col, id)
	if err != nil {
		return nil, refuse
	}
	ok, err := re.App.CanAccessRecord(rec, listRequestInfo(re.Auth, ruleHeaders(re.Request.Header)), col.ViewRule)
	if err != nil || !ok {
		return nil, refuse
	}
	return specBytes(rec), nil
}

// withSavedTopics adds the saved-report collections to a run's topics, so
// the open report runs again when someone edits it or changes its shares,
// and learns when the reader loses access.
func withSavedTopics(topics []string) []string {
	for _, c := range []string{reportsCollection, sharesCollection} {
		if t := changesignal.Topic(c); !slices.Contains(topics, t) {
			topics = append(topics, t)
		}
	}
	slices.Sort(topics)
	return topics
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt or vet output; `ok  tinycld.org/core/reports` (`TestRunEndpoint`'s `no spec` case still matches the new message `The request has no spec and no reportId.`).

- [ ] **Step 5: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/endpoint.go core/server/reports/endpoint_saved_test.go
git commit -m "feat(core): run a saved report by id for any reader who can view it"
```

---

### Task 5: Mark saved reports that cannot run (`CheckSaved`), at every start

**Files:**
- Create: `$WS/tinycld/core/server/reports/broken.go`
- Modify: `$WS/tinycld/core/server/reports/endpoint.go` (`Register` runs `checkSavedAtStart`; new `checkSavedAtStart`, `logBrokenAtStart`)
- Test: `$WS/tinycld/core/server/reports/broken_test.go`

**Interfaces:**
- Consumes: `Load`, `Validate`, `*SpecError`, `specBytes`, `changesignal.Touch`.
- Produces: `type Broken struct{Status int; Problems map[string]Problem; More int}`; `type Problem struct{Code, Message string}`; `const maxBrokenProblems = 50`; `type BrokenReport struct{ID, Title, OwnerID string; Owner *core.Record; Broken Broken}`; `type CheckResult struct{Checked, Batches, Written int; Broken []BrokenReport}`; `var checkBatch = 50`; `func CheckSaved(app core.App) (*CheckResult, error)`; `checkSpec(raw []byte) *Broken`; `writeBroken`; `writeBrokenReports(out io.Writer, res *CheckResult)`; `ownerLabel(owner *core.Record) string`; `checkSavedAtStart(app core.App)`; `logBrokenAtStart(res *CheckResult, dev bool)`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/broken_test.go`:

```go
package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

const goneSpec = `{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}`

func brokenOf(t *testing.T, s *savedZoo, key string) *Broken {
	t.Helper()
	rec, err := s.app.FindRecordById(reportsCollection, s.id(key))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := rec.Get("broken").(types.JSONRaw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var b Broken
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	return &b
}

// CheckSaved marks each report that cannot run with why, names its owner,
// clears the mark when the report is fixed, and writes only what changed.
// It never moves `updated`.
func TestCheckSavedMarksBrokenReports(t *testing.T) {
	s := newSavedZoo(t)
	s.saved(t, "gone", "bob", "Gone", goneSpec, false)
	s.saved(t, "newer", "ann", "Newer", `{"version":99}`, false)
	before, _ := s.app.FindRecordById(reportsCollection, s.id("gone"))

	res, err := CheckSaved(s.app)
	if err != nil {
		t.Fatal(err)
	}
	if res.Checked != 5 || len(res.Broken) != 2 || res.Written != 2 {
		t.Fatalf("checked %d, broken %d, written %d; want 5, 2, 2", res.Checked, len(res.Broken), res.Written)
	}
	gone := brokenOf(t, s, "gone")
	if gone == nil || gone.Status != 422 || gone.Problems["source"].Code != "unknown_source" {
		t.Fatalf("gone = %+v", gone)
	}
	if b := brokenOf(t, s, "newer"); b == nil || b.Status != 400 || b.Problems["version"].Code != "invalid" {
		t.Fatalf("newer = %+v", b)
	}
	for _, b := range res.Broken {
		if b.ID == s.id("gone") && (b.Owner == nil || b.Owner.Id != s.id("bob") || b.Title != "Gone") {
			t.Fatalf("gone's owner = %v", b.Owner)
		}
	}
	after, _ := s.app.FindRecordById(reportsCollection, s.id("gone"))
	if before.GetString("updated") != after.GetString("updated") {
		t.Fatal("marking a report moved updated")
	}

	again, err := CheckSaved(s.app)
	if err != nil || again.Written != 0 || len(again.Broken) != 2 {
		t.Fatalf("second check: %+v %v", again, err)
	}

	// A source that comes back clears the mark.
	RegisterSource(Source{Key: "zoo.gone", Collection: "zoo_feedings", Label: "Back",
		Fields: []Field{{Key: "cost", Label: "Cost", Type: Number, Aggregates: SumAll}}})
	fixed, err := CheckSaved(s.app)
	if err != nil || fixed.Written != 1 || brokenOf(t, s, "gone") != nil {
		t.Fatalf("after the source came back: %+v %v, gone = %+v", fixed, err, brokenOf(t, s, "gone"))
	}
}

// An app without core's report migration has no saved reports to check.
func TestCheckSavedWithoutTheCollection(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if res, err := CheckSaved(app); err != nil || res.Checked != 0 {
		t.Fatalf("CheckSaved = %+v, %v", res, err)
	}
}

// CheckSaved reads the reports checkBatch at a time: the number of reads
// follows the number of reports, never the size of one statement.
func TestCheckSavedReadsInBatches(t *testing.T) {
	s := newSavedZoo(t)
	saved := checkBatch
	checkBatch = 2
	t.Cleanup(func() { checkBatch = saved })
	s.saved(t, "x4", "ann", "x4", savedSpec, false)
	s.saved(t, "x5", "ann", "x5", savedSpec, false)
	res, err := CheckSaved(s.app)
	if err != nil {
		t.Fatal(err)
	}
	// 5 reports in reads of 2: 2, 2, 1.
	if res.Checked != 5 || res.Batches != 3 {
		t.Fatalf("checked %d in %d reads, want 5 in 3", res.Checked, res.Batches)
	}
	s.saved(t, "x6", "ann", "x6", savedSpec, false)
	// 6 in reads of 2: 2, 2, 2, then an empty read ends it.
	if res, _ := CheckSaved(s.app); res.Checked != 6 || res.Batches != 4 {
		t.Fatalf("checked %d in %d reads, want 6 in 4", res.Checked, res.Batches)
	}
}

// The broken column holds at most maxBrokenProblems problems; More counts
// the rest, so the 64 KB column always fits.
func TestABrokenMarkIsBounded(t *testing.T) {
	cols := make([]string, 0, 60)
	for i := range 60 {
		cols = append(cols, fmt.Sprintf(`{"id":"c%d","path":[{"field":"nope%d"}]}`, i, i))
	}
	newZoo(t) // registers the zoo sources
	b := checkSpec([]byte(`{"version":1,"source":"zoo.feedings","columns":[` + strings.Join(cols, ",") + `]}`))
	if b == nil || len(b.Problems) != maxBrokenProblems || b.More != 10 {
		t.Fatalf("problems %d, more %d", len(b.Problems), b.More)
	}
	raw, _ := json.Marshal(b)
	if len(raw) > 65536 {
		t.Fatalf("the mark is %d bytes", len(raw))
	}
}

type messagesHandler struct{ records *[]slog.Record }

func (h messagesHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h messagesHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r)
	return nil
}
func (h messagesHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h messagesHandler) WithGroup(string) slog.Handler      { return h }

// At start, dev logs each broken report with its owner; production logs one
// line with the count and the command that lists them.
func TestBrokenReportsAreLoggedAtStart(t *testing.T) {
	var records []slog.Record
	saved := log
	log = slog.New(messagesHandler{&records})
	t.Cleanup(func() { log = saved })
	res := &CheckResult{Checked: 3, Broken: []BrokenReport{
		{ID: "r1", Title: "One", Broken: Broken{Status: 422}},
		{ID: "r2", Title: "Two", Broken: Broken{Status: 400}},
	}}
	logBrokenAtStart(res, true)
	if len(records) != 2 || records[0].Level != slog.LevelWarn || records[0].Message != "saved report is broken" {
		t.Fatalf("dev logged %v", records)
	}
	records = nil
	logBrokenAtStart(res, false)
	if len(records) != 1 || records[0].Level != slog.LevelInfo || !strings.Contains(records[0].Message, "tinycld reports validate") {
		t.Fatalf("production logged %v", records)
	}
	records = nil
	logBrokenAtStart(&CheckResult{Checked: 3}, true)
	if len(records) != 0 {
		t.Fatalf("nothing broken logged %v", records)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'TestCheckSaved|TestABrokenMark|TestBrokenReports' ./reports/`
Expected: FAIL to build: `undefined: Broken`, `undefined: CheckSaved`, `undefined: checkBatch`, … (`too many errors`).

- [ ] **Step 3: Write `broken.go`**

`$WS/tinycld/core/server/reports/broken.go`:

```go
package reports

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/changesignal"
)

// Broken is why a saved report cannot run: its spec does not load or does
// not validate. Status is 400 (invalid) or 422 (it names a source, link,
// field or column that does not exist), as a run would answer. Problems are
// keyed by spec path, as the run endpoint keys them. At most
// maxBrokenProblems are kept; More counts the rest.
type Broken struct {
	Status   int                `json:"status"`
	Problems map[string]Problem `json:"problems"`
	More     int                `json:"more,omitempty"`
}

// Problem is one part of a spec that is wrong.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// maxBrokenProblems bounds the broken column (64 KB in the migration).
const maxBrokenProblems = 50

// BrokenReport is one saved report that cannot run, with its owner (nil
// when the owner row cannot be read).
type BrokenReport struct {
	ID      string
	Title   string
	OwnerID string
	Owner   *core.Record
	Broken  Broken
}

// CheckResult says what CheckSaved did. Batches counts the reads of saved
// reports; Written counts the reports whose broken column changed.
type CheckResult struct {
	Checked int
	Batches int
	Written int
	Broken  []BrokenReport
}

// checkBatch is how many saved reports one read loads. A spec has at most
// maxSpecBytes, so one read holds at most checkBatch of them in memory.
var checkBatch = 50

// CheckSaved loads and validates every saved report as a run would, against
// the sources registered and checked now, and stores the outcome in each
// report's broken column when it changed. It reads checkBatch reports at a
// time in id order, so memory and statement size stay the same however many
// reports exist. The column is written with SQL, not a record save: it is
// the server's note, not an edit, so `updated` and the audit log stay as
// the people who edit the report left them. Broken comes back in id order.
func CheckSaved(app core.App) (*CheckResult, error) {
	res := &CheckResult{}
	if _, err := app.FindCachedCollectionByNameOrId(reportsCollection); err != nil {
		return res, nil // an app without core's report migration saves no reports
	}
	after := ""
	for {
		batch, err := app.FindRecordsByFilter(reportsCollection, "id > {:after}", "id", checkBatch, 0, dbx.Params{"after": after})
		res.Batches++
		if err != nil {
			return res, fmt.Errorf("reports: read saved reports: %w", err)
		}
		for _, rec := range batch {
			res.Checked++
			broken := checkSpec(specBytes(rec))
			if broken != nil {
				res.Broken = append(res.Broken, BrokenReport{ID: rec.Id, Title: rec.GetString("title"), OwnerID: rec.GetString("owner"), Broken: *broken})
			}
			written, err := writeBroken(app, rec, broken)
			if err != nil {
				return res, err
			}
			if written {
				res.Written++
			}
		}
		if len(batch) < checkBatch {
			break
		}
		after = batch[len(batch)-1].Id
	}
	if res.Written > 0 {
		changesignal.Touch(app, reportsCollection)
	}
	for i := range res.Broken {
		if owner, err := app.FindRecordById("users", res.Broken[i].OwnerID); err == nil {
			res.Broken[i].Owner = owner
		}
	}
	return res, nil
}

// checkSpec is nil for a spec that loads and validates, else why it fails.
func checkSpec(raw []byte) *Broken {
	spec, _, err := Load(raw)
	if err == nil {
		err = Validate(spec)
	}
	if err == nil {
		return nil
	}
	var specErr *SpecError
	if !errors.As(err, &specErr) {
		// An engine fault (a missing upgrade step), not the spec's: the run
		// would answer 500 as well.
		return &Broken{Status: 500, Problems: map[string]Problem{"version": {Code: "no_upgrade", Message: err.Error()}}}
	}
	keys := make([]string, 0, len(specErr.Problems))
	for k := range specErr.Problems {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b := &Broken{Status: specErr.Status, Problems: map[string]Problem{}}
	for i, k := range keys {
		if i == maxBrokenProblems {
			b.More = len(keys) - i
			break
		}
		p := specErr.Problems[k]
		b.Problems[k] = Problem{Code: p.Code(), Message: p.Message()}
	}
	return b
}

// writeBroken stores broken (nil clears it) unless the column holds it
// already, and reports whether it wrote.
func writeBroken(app core.App, rec *core.Record, broken *Broken) (bool, error) {
	var want []byte
	if broken != nil {
		var err error
		if want, err = json.Marshal(broken); err != nil {
			return false, err
		}
	}
	have, _ := rec.Get("broken").(types.JSONRaw)
	if bytes.Equal(have, want) || (want == nil && (len(have) == 0 || string(have) == "null")) {
		return false, nil
	}
	var value any
	if want != nil {
		value = string(want)
	}
	if _, err := app.DB().NewQuery("UPDATE reports SET broken = {:broken} WHERE id = {:id}").
		Bind(dbx.Params{"broken": value, "id": rec.Id}).Execute(); err != nil {
		return false, fmt.Errorf("reports: mark report %s: %w", rec.Id, err)
	}
	return true, nil
}

// writeBrokenReports lists a check's result for a person: a summary line,
// then each broken report with its owner and problems.
func writeBrokenReports(out io.Writer, res *CheckResult) {
	fmt.Fprintf(out, "checked %d saved reports, %d broken\n", res.Checked, len(res.Broken))
	for _, b := range res.Broken {
		fmt.Fprintf(out, "%s  %q  owner %s\n", b.ID, b.Title, ownerLabel(b.Owner))
		keys := make([]string, 0, len(b.Broken.Problems))
		for k := range b.Broken.Problems {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := b.Broken.Problems[k]
			fmt.Fprintf(out, "  %s: %s: %s\n", k, p.Code, p.Message)
		}
		if b.Broken.More > 0 {
			fmt.Fprintf(out, "  and %d more\n", b.Broken.More)
		}
	}
}

func ownerLabel(owner *core.Record) string {
	if owner == nil {
		return "(unknown)"
	}
	return fmt.Sprintf("%s <%s>", owner.GetString("name"), owner.Email())
}
```

- [ ] **Step 4: Check at every start**

In `$WS/tinycld/core/server/reports/endpoint.go`:

1. In the comment on `Register`, replace the four lines Task 4 wrote:

```go
// refuses, so a broken source never reaches a user. It serves
// POST /api/reports/run to signed-in users: a draft spec, or a saved report
// the caller can view. Saved reports and their shares are bound by
// RegisterSharing.
```

with:

```go
// refuses, so a broken source never reaches a user; then it marks every
// saved report that cannot run (checkSavedAtStart). It serves POST
// /api/reports/run to signed-in users: a draft spec, or a saved report the
// caller can view. Saved reports and their shares are bound by
// RegisterSharing.
```

2. In `Register`, directly before `e.Router.POST("/api/reports/run", handleRun).`, add:

```go
		checkSavedAtStart(e.App)
```

3. Directly after `Register`, add:

```go
// checkSavedAtStart refreshes every saved report's broken mark when the
// server starts, after the sources are checked: a package install or
// upgrade restarts the server, and that is when a source, link or field can
// disappear. In dev it also logs each broken report with its owner; in
// production one line says how many and names the command that lists them.
func checkSavedAtStart(app core.App) {
	res, err := CheckSaved(app)
	if err != nil {
		log.Error("saved reports not checked", "err", err)
		return
	}
	logBrokenAtStart(res, app.IsDev())
}

func logBrokenAtStart(res *CheckResult, dev bool) {
	if len(res.Broken) == 0 {
		return
	}
	if !dev {
		log.Info("saved reports are broken; run `tinycld reports validate` to list them", "broken", len(res.Broken), "checked", res.Checked)
		return
	}
	for _, b := range res.Broken {
		log.Warn("saved report is broken", "report", b.ID, "title", b.Title, "owner", ownerLabel(b.Owner), "problems", b.Broken.Problems)
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt or vet output; `ok  tinycld.org/core/reports`.

- [ ] **Step 6: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/broken.go core/server/reports/endpoint.go core/server/reports/broken_test.go
git commit -m "feat(core): mark saved reports that cannot run, at every server start"
```

---

### Task 6: `tinycld reports validate`

**Files:**
- Create: `$WS/tinycld/core/server/reports/command.go`
- Modify: `$WS/tinycld/server/main.go` (add the command after `create-owner`)
- Test: `$WS/tinycld/core/server/reports/command_test.go`

**Interfaces:**
- Consumes: `CheckSources`, `Refused`, `CheckSaved`, `writeBrokenReports`, `ResetForTesting`, `rlstest.AssembledDataDir`, `goneSpec` (Task 5 test).
- Produces: `func NewCommand(app *pocketbase.PocketBase) *cobra.Command` (`reports`, subcommand `validate`); `validateSaved(app core.App, out io.Writer) error`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/command_test.go`:

```go
package reports

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/rlstest"
)

// commandApp is a real PocketBase app on a data dir with core's migrations
// applied, as `tinycld reports validate` finds a deployment's pb_data.
func commandApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: rlstest.AssembledDataDir(t), HideStartBanner: true})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	return app
}

func runValidate(t *testing.T, app *pocketbase.PocketBase, args ...string) (string, error) {
	t.Helper()
	cmd := NewCommand(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"validate"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestReportsValidateListsBrokenReportsWithTheirOwner(t *testing.T) {
	app := commandApp(t)
	out, err := runValidate(t, app)
	if err != nil || !strings.Contains(out, "checked 0 saved reports, 0 broken") {
		t.Fatalf("an empty deployment: %v\n%s", err, out)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	ann := core.NewRecord(users)
	ann.SetEmail("ann@zoo.test")
	ann.SetPassword("password123")
	ann.Set("name", "Ann")
	ann.Set("username", "ann")
	ann.Set("role", "member")
	if err := app.Save(ann); err != nil {
		t.Fatal(err)
	}
	reports, err := app.FindCollectionByNameOrId(reportsCollection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(reports)
	r.Set("owner", ann.Id)
	r.Set("title", "Feed costs")
	r.Set("spec", goneSpec)
	r.Set("source", "zoo.gone")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}

	out, err = runValidate(t, app)
	if err == nil || err.Error() != "1 of 1 saved reports are broken" {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{
		"checked 1 saved reports, 1 broken",
		r.Id + `  "Feed costs"  owner Ann <ann@zoo.test>`,
		`  source: unknown_source: the source "zoo.gone" does not exist`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	stored, _ := app.FindRecordById(reportsCollection, r.Id)
	if !strings.Contains(stored.GetString("broken"), `"unknown_source"`) {
		t.Fatalf("the command did not store the mark: %s", stored.GetString("broken"))
	}
}

func TestReportsValidateTakesNoArguments(t *testing.T) {
	app := commandApp(t)
	if _, err := runValidate(t, app, "extra"); err == nil {
		t.Fatal("validate accepted an argument")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run TestReportsValidate ./reports/`
Expected: FAIL to build: `undefined: NewCommand`.

- [ ] **Step 3: Write `command.go`**

`$WS/tinycld/core/server/reports/command.go`:

```go
package reports

import (
	"fmt"
	"io"
	"slices"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// NewCommand is `reports` with one subcommand, `reports validate`. It
// checks every saved report against the sources the installed packages
// register, stores the outcome (the hub marks a broken report), and lists
// each broken report with its owner. It exits with an error when a report
// is broken, so a deploy script can stop on it.
//
// PocketBase bootstraps the app before it runs a subcommand. The command
// then applies pending migrations, as `create-owner` does: a source whose
// collection a pending migration adds would otherwise be refused, and every
// report on it listed as broken.
func NewCommand(app *pocketbase.PocketBase) *cobra.Command {
	cmd := &cobra.Command{Use: "reports", Short: "Saved reports"}
	cmd.AddCommand(&cobra.Command{
		Use:          "validate",
		Short:        "List saved reports that name a missing source, link or field, with their owners",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := app.Bootstrap(); err != nil {
				return fmt.Errorf("reports validate: bootstrap: %w", err)
			}
			if err := app.RunAppMigrations(); err != nil {
				return fmt.Errorf("reports validate: run migrations: %w", err)
			}
			return validateSaved(app, cmd.OutOrStdout())
		},
	})
	return cmd
}

// validateSaved checks the sources, then every saved report, and writes
// what it found to out.
func validateSaved(app core.App, out io.Writer) error {
	if err := CheckSources(app); err != nil {
		refused := Refused()
		keys := make([]string, 0, len(refused))
		for k := range refused {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintf(out, "source %s is refused: %s\n", k, refused[k])
		}
	}
	res, err := CheckSaved(app)
	if err != nil {
		return err
	}
	writeBrokenReports(out, res)
	if len(res.Broken) > 0 {
		return fmt.Errorf("%d of %d saved reports are broken", len(res.Broken), res.Checked)
	}
	return nil
}
```

- [ ] **Step 4: Wire it into the app binary**

In `$WS/tinycld/server/main.go`, add `"tinycld.org/core/reports"` to the imports between `"tinycld.org/core/listeners"` and `"tinycld.org/core/supervise"`, and directly after `app.RootCmd.AddCommand(coreserver.NewCreateOwnerCommand(app))` add:

```go

	// `reports validate` lists the saved reports that name a source, link or
	// field the installed packages no longer register, with their owners, and
	// exits non-zero when there is one.
	app.RootCmd.AddCommand(reports.NewCommand(app))
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 -run TestReportsValidate ./reports/`
Expected: no gofmt or vet output; `ok  tinycld.org/core/reports`.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/server && gofmt -l main.go && go vet . && go build -o /dev/null .`
Expected: no output (the module builds through the generated `server/go.work`).

- [ ] **Step 6: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/command.go core/server/reports/command_test.go server/main.go
git commit -m "feat(core): tinycld reports validate lists broken saved reports with their owners"
```

---

### Task 7: Reports a package ships

**Files:**
- Create: `$WS/tinycld/core/server/reports/shipped.go`
- Modify: `$WS/tinycld/core/server/reports/registry.go` (`ResetForTesting` also clears shipped reports)
- Modify: `$WS/tinycld/core/server/reports/sharing.go` (bind the delete guard and the users hooks)
- Modify: `$WS/tinycld/core/server/reports/endpoint.go` (`Register` runs `ReconcileShipped`)
- Test: `$WS/tinycld/core/server/reports/shipped_test.go`

**Interfaces:**
- Consumes: `sourceKeyRe` (registry.go), `maxSpecBytes`, `Load`, `Validate`, `isActiveAdmin`, `fieldError`, `log`.
- Produces: `type Shipped struct{Key, Title, Description string; Spec json.RawMessage; Revision int}`; `func RegisterShipped(s Shipped)`; `func ReconcileShipped(app core.App) error`; `isShipped(key string) bool`; `hasShipped() bool`; `shipForAdmin(e *core.RecordEvent) error`; `onReportDelete(e *core.RecordRequestEvent) error`; `func firstAdmin(app core.App, except string) (*core.Record, error)`; package vars `shippedMu`, `shipped`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/shipped_test.go`:

```go
package reports

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func feedCosts(revision int, title string) Shipped {
	return Shipped{Key: "zoo.feed_costs", Title: title, Description: "Every feeding's cost", Spec: json.RawMessage(savedSpec), Revision: revision}
}

func shippedRow(t *testing.T, s *savedZoo, key string) *core.Record {
	t.Helper()
	rec, err := s.app.FindFirstRecordByFilter(reportsCollection, "shipped_key = {:k}", map[string]any{"k": key})
	if err != nil {
		return nil
	}
	return rec
}

func TestRegisterShippedRefusesAMalformedReport(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	for name, s := range map[string]Shipped{
		"a key without a package": {Key: "costs", Title: "x", Spec: json.RawMessage(`{}`), Revision: 1},
		"no title":                {Key: "zoo.costs", Title: " ", Spec: json.RawMessage(`{}`), Revision: 1},
		"revision 0":              {Key: "zoo.costs", Title: "x", Spec: json.RawMessage(`{}`), Revision: 0},
		"a spec that is not JSON": {Key: "zoo.costs", Title: "x", Spec: json.RawMessage(`{`), Revision: 1},
		"a description too long":  {Key: "zoo.costs", Title: "x", Description: strings.Repeat("d", 2001), Spec: json.RawMessage(`{}`), Revision: 1},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("RegisterShipped did not panic")
				}
			}()
			RegisterShipped(s)
		})
	}
}

// A shipped report waits for an admin, then belongs to the first one,
// published; every member sees it.
func TestAShippedReportWaitsForTheFirstAdmin(t *testing.T) {
	s := newSavedZoo(t)
	// The fixture's admin ad would own it: start from a deployment with no admin.
	s.recs["ad"].Set("role", "member")
	if err := s.app.Save(s.recs["ad"]); err != nil {
		t.Fatal(err)
	}
	RegisterShipped(feedCosts(1, "Feed costs"))
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	if shippedRow(t, s, "zoo.feed_costs") != nil {
		t.Fatal("a shipped report was made with no admin to own it")
	}
	// The setup wizard's first owner: its save runs shipForAdmin.
	owner := s.person(t, "own", "owner")
	row := shippedRow(t, s, "zoo.feed_costs")
	if row == nil {
		t.Fatal("the first admin's save did not ship the report")
	}
	if row.GetString("owner") != owner.Id || !row.GetBool("published") || row.GetString("published_by") != owner.Id ||
		row.GetString("source") != "zoo.feedings" || row.GetInt("shipped_revision") != 1 || row.GetString("title") != "Feed costs" {
		t.Fatalf("shipped row = %v", row.PublicExport())
	}
	if got := s.listTitles(t, "ot", reportsURL); !strings.Contains(strings.Join(got, ","), "Feed costs") {
		t.Fatalf("a member lists %v", got)
	}
}

// The oldest active owner owns shipped reports; an admin only when there
// is no owner.
func TestTheFirstAdminIsTheOldestOwner(t *testing.T) {
	s := newSavedZoo(t) // ad, an admin, is older than the owner below
	owner := s.person(t, "own", "owner")
	got, err := firstAdmin(s.app, "")
	if err != nil || got == nil || got.Id != owner.Id {
		t.Fatalf("firstAdmin = %v, %v; want the owner", got, err)
	}
	if got, _ := firstAdmin(s.app, owner.Id); got == nil || got.Id != s.id("ad") {
		t.Fatalf("firstAdmin without the owner = %v; want ad", got)
	}
}

// A newer revision replaces the spec, title and description and keeps the
// owner and what an admin did with publishing; the same revision changes
// nothing.
func TestANewerRevisionReplacesTheShippedSpec(t *testing.T) {
	s := newSavedZoo(t)
	RegisterShipped(feedCosts(1, "Feed costs"))
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row := shippedRow(t, s, "zoo.feed_costs")
	if row == nil || row.GetString("owner") != s.id("ad") {
		t.Fatalf("not shipped to ad: %v", row)
	}
	s.expect(t, "ad", http.MethodPatch, reportURL(row.Id), `{"published":false}`, http.StatusOK)
	updated := shippedRow(t, s, "zoo.feed_costs").GetString("updated")
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	if shippedRow(t, s, "zoo.feed_costs").GetString("updated") != updated {
		t.Fatal("the same revision rewrote the report")
	}
	RegisterShipped(Shipped{Key: "zoo.feed_costs", Title: "Feeding costs", Spec: json.RawMessage(
		`{"version":1,"source":"zoo.feedings","columns":[{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}]}`), Revision: 2})
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row = shippedRow(t, s, "zoo.feed_costs")
	if row.GetString("title") != "Feeding costs" || row.GetInt("shipped_revision") != 2 || !strings.Contains(row.GetString("spec"), `"grams"`) ||
		row.GetBool("published") || row.GetString("owner") != s.id("ad") {
		t.Fatalf("after revision 2: %v", row.PublicExport())
	}
}

// A shipped spec that does not validate is skipped with its key named, and
// the others still ship.
func TestABadShippedSpecDoesNotStopTheOthers(t *testing.T) {
	s := newSavedZoo(t)
	RegisterShipped(Shipped{Key: "zoo.broken", Title: "Broken", Spec: json.RawMessage(goneSpec), Revision: 1})
	RegisterShipped(feedCosts(1, "Feed costs"))
	err := ReconcileShipped(s.app)
	if err == nil || !strings.Contains(err.Error(), `"zoo.broken"`) {
		t.Fatalf("err = %v", err)
	}
	if shippedRow(t, s, "zoo.broken") != nil || shippedRow(t, s, "zoo.feed_costs") == nil {
		t.Fatal("the bad spec stopped the good one, or shipped itself")
	}
}

// A shipped report cannot be deleted while its package ships it (the server
// would make it again); once the package is gone it can.
func TestAShippedReportIsUnpublishedNotDeleted(t *testing.T) {
	s := newSavedZoo(t)
	RegisterShipped(feedCosts(1, "Feed costs"))
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row := shippedRow(t, s, "zoo.feed_costs")
	s.expect(t, "ad", http.MethodDelete, reportURL(row.Id), "", http.StatusBadRequest, `"shipped_key":{"code":"shipped"`)
	shippedMu.Lock()
	delete(shipped, "zoo.feed_costs")
	shippedMu.Unlock()
	s.expect(t, "ad", http.MethodDelete, reportURL(row.Id), "", http.StatusNoContent)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Shipped|TheFirstAdmin|ANewerRevision|ABadShipped' ./reports/`
Expected: FAIL to build: `undefined: Shipped`, `undefined: RegisterShipped`, `undefined: ReconcileShipped`, `undefined: firstAdmin`.

- [ ] **Step 3: Write `shipped.go`**

`$WS/tinycld/core/server/reports/shipped.go`:

```go
package reports

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Shipped is a report a package ships: a published report every user sees,
// owned by the first admin. Register it from the package's Register(app).
type Shipped struct {
	// Key names the report for good: "<package>.<name>".
	Key         string
	Title       string
	Description string
	Spec        json.RawMessage
	// Revision is 1 or more. Raise it whenever Spec, Title or Description
	// changes: the server then replaces the stored report's spec, title and
	// description at its next start.
	Revision int
}

var (
	shippedMu sync.Mutex
	shipped   = map[string]Shipped{}
)

// RegisterShipped adds a package's shipped report. Registering a key again
// replaces it. A malformed report panics at boot: it is a wiring mistake in
// the package's code. Its spec is checked against the sources when the
// server starts (ReconcileShipped).
func RegisterShipped(s Shipped) {
	if err := validateShipped(s); err != nil {
		panic(fmt.Sprintf("reports.RegisterShipped(%q): %v", s.Key, err))
	}
	s.Spec = slices.Clone(s.Spec)
	shippedMu.Lock()
	defer shippedMu.Unlock()
	shipped[s.Key] = s
}

func validateShipped(s Shipped) error {
	switch {
	case !sourceKeyRe.MatchString(s.Key):
		return fmt.Errorf("key must look like \"<package>.<name>\"")
	case strings.TrimSpace(s.Title) == "" || len(s.Title) > 200:
		return fmt.Errorf("a title has 1 to 200 characters")
	case len(s.Description) > 2000:
		return fmt.Errorf("a description has at most 2000 characters")
	case s.Revision < 1:
		return fmt.Errorf("revision starts at 1")
	case len(s.Spec) > maxSpecBytes || !json.Valid(s.Spec):
		return fmt.Errorf("the spec is not JSON of at most %d bytes", maxSpecBytes)
	}
	return nil
}

func isShipped(key string) bool {
	shippedMu.Lock()
	defer shippedMu.Unlock()
	_, ok := shipped[key]
	return ok
}

// ReconcileShipped makes the stored reports match the shipped ones. A
// missing report is created, published, owned and published by the first
// admin; with no admin yet it waits until one appears (shipForAdmin). A
// stored report whose revision is older gets the shipped spec, title and
// description; its owner and whether it is published stay as people left
// them. A spec that does not validate is logged and skipped, so one broken
// report does not stop the others.
func ReconcileShipped(app core.App) error {
	shippedMu.Lock()
	defer shippedMu.Unlock()
	if len(shipped) == 0 {
		return nil
	}
	keys := make([]string, 0, len(shipped))
	for k := range shipped {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var errs []error
	var admin *core.Record
	for _, key := range keys {
		s := shipped[key]
		rec, err := app.FindFirstRecordByFilter(reportsCollection, "shipped_key = {:k}", dbx.Params{"k": key})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			errs = append(errs, fmt.Errorf("shipped report %q: %w", key, err))
			continue
		}
		if rec != nil && rec.GetInt("shipped_revision") >= s.Revision {
			continue
		}
		stored, source, err := shippedSpec(s)
		if err != nil {
			errs = append(errs, fmt.Errorf("shipped report %q: %w", key, err))
			continue
		}
		if rec == nil {
			if admin == nil {
				if admin, err = firstAdmin(app, ""); err != nil {
					return errors.Join(append(errs, err)...)
				}
				if admin == nil {
					return errors.Join(errs...)
				}
			}
			col, err := app.FindCachedCollectionByNameOrId(reportsCollection)
			if err != nil {
				return errors.Join(append(errs, err)...)
			}
			rec = core.NewRecord(col)
			rec.Set("owner", admin.Id)
			rec.Set("published", true)
			rec.Set("published_by", admin.Id)
			rec.Set("shipped_key", key)
		}
		rec.Set("title", s.Title)
		rec.Set("description", s.Description)
		rec.Set("spec", stored)
		rec.Set("source", source)
		rec.Set("shipped_revision", s.Revision)
		rec.Set("broken", nil)
		if err := app.Save(rec); err != nil {
			errs = append(errs, fmt.Errorf("shipped report %q: %w", key, err))
		}
	}
	return errors.Join(errs...)
}

// shippedSpec loads and validates a shipped spec, as a save would.
func shippedSpec(s Shipped) (types.JSONRaw, string, error) {
	spec, _, err := Load(s.Spec)
	if err != nil {
		return nil, "", err
	}
	if err := Validate(spec); err != nil {
		return nil, "", err
	}
	stored, err := json.Marshal(spec)
	if err != nil {
		return nil, "", err
	}
	return stored, spec.Source, nil
}

// shipForAdmin creates the shipped reports when a users record that is an
// active admin is saved: the setup wizard's first owner is the first
// moment a fresh install has someone to own them. Every save of an admin
// runs it; with nothing missing it reads one row per shipped report.
func shipForAdmin(e *core.RecordEvent) error {
	if isActiveAdmin(e.Record) && hasShipped() {
		if err := ReconcileShipped(e.App); err != nil {
			log.Error("shipped reports not reconciled", "err", err)
		}
	}
	return e.Next()
}

func hasShipped() bool {
	shippedMu.Lock()
	defer shippedMu.Unlock()
	return len(shipped) > 0
}

// onReportDelete refuses to delete a shipped report while its package
// ships it: the next start would make it again. Unpublishing hides it from
// everyone else. Once the package is gone, the report can be deleted.
func onReportDelete(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	if key := e.Record.GetString("shipped_key"); key != "" && isShipped(key) {
		const msg = "This report comes with a package. Unpublish it instead of deleting it."
		return e.BadRequestError(msg, fieldError("shipped_key", "shipped", msg))
	}
	return e.Next()
}

// firstAdmin is the oldest active owner, else the oldest active admin,
// leaving out the user except (when set). nil when there is none.
func firstAdmin(app core.App, except string) (*core.Record, error) {
	for _, role := range []string{"owner", "admin"} {
		recs, err := app.FindRecordsByFilter("users",
			"role = {:role} && disabled != true && id != {:except}", "created,id", 1, 0,
			dbx.Params{"role": role, "except": except})
		if err != nil {
			return nil, fmt.Errorf("find the first %s: %w", role, err)
		}
		if len(recs) == 1 {
			return recs[0], nil
		}
	}
	return nil, nil
}
```

- [ ] **Step 4: Reset shipped reports in tests**

In `$WS/tinycld/core/server/reports/registry.go`, replace `ResetForTesting` with:

```go
// ResetForTesting empties the registry and the shipped reports. Tests
// register their own.
func ResetForTesting() {
	mu.Lock()
	sources = map[string]*Source{}
	refused = map[string]string{}
	mu.Unlock()
	shippedMu.Lock()
	shipped = map[string]Shipped{}
	shippedMu.Unlock()
}
```

- [ ] **Step 5: Bind the hooks and reconcile at start**

Replace `$WS/tinycld/core/server/reports/sharing.go` with:

```go
package reports

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
)

// The saved-report collections (core migration 2090000008_create_reports.js).
const (
	reportsCollection = "reports"
	sharesCollection  = "report_shares"
)

// RegisterSharing binds saved reports and their sharing: report_shares as a
// group grant table, the save and publish guards on reports, the share
// guards on report_shares, shipped reports for the first admin who appears,
// and the offboard handler. Call it before groups.Register, which binds the
// expansion of every grant table registered by then.
//
// The collections' own rules (the migration) decide who may read and write
// a row; these hooks guard what a permitted write may change. A superuser
// request passes them unchanged: a superuser can change any row anyway.
func RegisterSharing(app core.App) {
	groups.RegisterGrantTable(groups.GrantTable{Collection: sharesCollection, ResourceField: "report"})
	app.OnRecordCreateRequest(reportsCollection).BindFunc(onReportCreate)
	app.OnRecordUpdateRequest(reportsCollection).BindFunc(onReportUpdate)
	app.OnRecordDeleteRequest(reportsCollection).BindFunc(onReportDelete)
	app.OnRecordCreateRequest(sharesCollection).BindFunc(onShareCreate)
	app.OnRecordUpdateRequest(sharesCollection).BindFunc(onShareUpdate)
	app.OnRecordDeleteRequest(sharesCollection).BindFunc(onShareDelete)
	app.OnRecordAfterCreateSuccess("users").BindFunc(shipForAdmin)
	app.OnRecordAfterUpdateSuccess("users").BindFunc(shipForAdmin)
}
```

In `$WS/tinycld/core/server/reports/endpoint.go`, in the comment on `Register`, replace the five lines Task 5 wrote:

```go
// refuses, so a broken source never reaches a user; then it marks every
// saved report that cannot run (checkSavedAtStart). It serves POST
// /api/reports/run to signed-in users: a draft spec, or a saved report the
// caller can view. Saved reports and their shares are bound by
// RegisterSharing.
```

with:

```go
// refuses, so a broken source never reaches a user; then it ships the
// packages' reports (ReconcileShipped) and marks every saved report that
// cannot run (checkSavedAtStart). It serves POST /api/reports/run to
// signed-in users: a draft spec, or a saved report the caller can view.
// Saved reports and their shares are bound by RegisterSharing.
```

and in `Register`, directly before `checkSavedAtStart(e.App)`, add:

```go
		if err := ReconcileShipped(e.App); err != nil {
			log.Error("shipped reports not reconciled", "err", err)
		}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt or vet output; `ok  tinycld.org/core/reports`.

- [ ] **Step 7: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each.

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/shipped.go core/server/reports/registry.go core/server/reports/sharing.go \
  core/server/reports/endpoint.go core/server/reports/shipped_test.go
git commit -m "feat(core): reports a package ships, published for the first admin"
```

---

### Task 8: Offboarding a report owner

**Files:**
- Create: `$WS/tinycld/core/server/reports/offboard.go`
- Modify: `$WS/tinycld/core/server/reports/sharing.go` (register the handler)
- Test: `$WS/tinycld/core/server/reports/offboard_test.go`

**Interfaces:**
- Consumes: `offboard.Plan`, `offboard.ModeReassign` / `ModeDeleteMyData` / `ModeKeep`, `offboard.ErrInvalidPlan`, `offboard.RegisterHandler`, `offboard.OffboardUser` (tests), `firstAdmin`, `isActiveAdmin`.
- Produces: `offboardReports(txApp core.App, leaver *core.Record, plan offboard.Plan, actorUserID string) error` (an `offboard.Handler`, registered as `"reports"`); `moveReport`, `deleteDirectShares`, `reportHeir`.

- [ ] **Step 1: Write the failing tests**

`$WS/tinycld/core/server/reports/offboard_test.go`:

```go
package reports

import (
	"errors"
	"testing"

	"tinycld.org/core/offboard"
)

func (s *savedZoo) ownerOf(t *testing.T, key string) string {
	t.Helper()
	rec, err := s.app.FindRecordById(reportsCollection, s.id(key))
	if err != nil {
		return ""
	}
	return rec.GetString("owner")
}

func (s *savedZoo) shareExists(key string) bool {
	_, err := s.app.FindRecordById(sharesCollection, s.id(key))
	return err == nil
}

func (s *savedZoo) sharesOf(t *testing.T, report string) int {
	t.Helper()
	rows, err := s.app.FindRecordsByFilter(sharesCollection, "report = {:r}", "", 0, 0, map[string]any{"r": s.id(report)})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// Reassign: every report of the leaver goes to the successor, who loses the
// direct share ownership now covers; other shares stay. The leaver's
// shares on other people's reports go.
func TestOffboardReassignMovesEveryReport(t *testing.T) {
	s := newSavedZoo(t)
	s.share(t, "R2.ann", "R2", "ann", "", "viewer")
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeReassign, SuccessorUserID: s.id("ed")}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
	if s.ownerOf(t, "R") != s.id("ed") || s.ownerOf(t, "P") != s.id("ed") {
		t.Fatalf("owners: R %s, P %s; want ed", s.ownerOf(t, "R"), s.ownerOf(t, "P"))
	}
	if s.shareExists("R.ed") || !s.shareExists("R.vi") || !s.shareExists("R.keepers") || s.shareExists("R2.ann") {
		t.Fatalf("shares: R.ed %v (want gone), R.vi %v, R.keepers %v, R2.ann %v (want gone)",
			s.shareExists("R.ed"), s.shareExists("R.vi"), s.shareExists("R.keepers"), s.shareExists("R2.ann"))
	}
}

// Delete my data: an unpublished report goes with every share row; a
// published one goes to the admin who ran the offboard.
func TestOffboardDeleteKeepsPublishedReports(t *testing.T) {
	s := newSavedZoo(t)
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeDeleteMyData}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
	if s.ownerOf(t, "R") != "" || s.sharesOf(t, "R") != 0 {
		t.Fatalf("R: owner %q, %d shares; want deleted with its shares", s.ownerOf(t, "R"), s.sharesOf(t, "R"))
	}
	if s.ownerOf(t, "P") != s.id("ad") {
		t.Fatalf("P owner %s, want the acting admin", s.ownerOf(t, "P"))
	}
}

// Keep (a self-delete): an unpublished report stays with the anonymized
// account; a published one goes to the first admin.
func TestOffboardKeepMovesOnlyPublishedReports(t *testing.T) {
	s := newSavedZoo(t)
	owner := s.person(t, "own", "owner")
	if _, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeKeep}, s.id("ann")); err != nil {
		t.Fatal(err)
	}
	if s.ownerOf(t, "R") != s.id("ann") || s.sharesOf(t, "R") != 4 {
		t.Fatalf("R: owner %s, %d shares; want ann's, all 4 kept", s.ownerOf(t, "R"), s.sharesOf(t, "R"))
	}
	if s.ownerOf(t, "P") != owner.Id {
		t.Fatalf("P owner %s, want the first admin", s.ownerOf(t, "P"))
	}
}

// With nobody to take a published report, the offboard is refused and
// nothing changes.
func TestOffboardWithNoHeirIsRefused(t *testing.T) {
	s := newSavedZoo(t)
	s.recs["ad"].Set("role", "member")
	if err := s.app.Save(s.recs["ad"]); err != nil {
		t.Fatal(err)
	}
	_, err := offboard.OffboardUser(s.app, s.id("ann"), offboard.Plan{Mode: offboard.ModeDeleteMyData}, s.id("ann"))
	if !errors.Is(err, offboard.ErrInvalidPlan) {
		t.Fatalf("err = %v, want ErrInvalidPlan", err)
	}
	if s.ownerOf(t, "R") != s.id("ann") || s.ownerOf(t, "P") != s.id("ann") {
		t.Fatal("a refused offboard changed reports")
	}
	ann, _ := s.app.FindRecordById("users", s.id("ann"))
	if ann.Email() != "ann@zoo.test" {
		t.Fatal("a refused offboard anonymized the user")
	}
}

// A group member who leaves loses the derived row; the grant stays.
func TestOffboardRemovesAMembersDerivedRow(t *testing.T) {
	s := newSavedZoo(t)
	if _, err := offboard.OffboardUser(s.app, s.id("gm"), offboard.Plan{Mode: offboard.ModeKeep}, s.id("ad")); err != nil {
		t.Fatal(err)
	}
	if !s.shareExists("R.keepers") || s.sharesOf(t, "R") != 3 {
		t.Fatalf("R has %d shares, want 3 (ed, vi, the grant)", s.sharesOf(t, "R"))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run TestOffboard ./reports/`
Expected: FAIL: `TestOffboardReassignMovesEveryReport`: `owners: R <ann's id>, P <ann's id>; want ed`; `TestOffboardDeleteKeepsPublishedReports`: `R: owner "<ann's id>", 4 shares; want deleted with its shares`; `TestOffboardKeepMovesOnlyPublishedReports`: `P owner <ann's id>, want the first admin`; `TestOffboardWithNoHeirIsRefused`: `err = <nil>, want ErrInvalidPlan`. (`TestOffboardRemovesAMembersDerivedRow` passes already: `OffboardUser` removes memberships.)

- [ ] **Step 3: Write `offboard.go`**

`$WS/tinycld/core/server/reports/offboard.go`:

```go
package reports

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/offboard"
)

// offboardReports settles a leaver's saved reports inside the offboard
// transaction. It is a Handler, not a reassignable ref, because a flat FK
// rewrite cannot keep a published report while deleting the rest.
//
//   - ModeReassign: every report the leaver owns goes to the successor.
//   - ModeDeleteMyData: an unpublished report is deleted with its shares; a
//     published one goes to the heir (reportHeir).
//   - ModeKeep: an unpublished report stays with the anonymized account, as
//     other kept content does; a published one goes to the heir.
//
// A report that changes owner loses the new owner's own direct share on it,
// which ownership now covers. In every mode the leaver's direct shares on
// other people's reports are removed: nobody signs in as the anonymized
// account. Their group-derived rows are gone already, because OffboardUser
// removes the leaver's group memberships before the handlers run.
func offboardReports(txApp core.App, leaver *core.Record, plan offboard.Plan, actorUserID string) error {
	if _, err := txApp.FindCollectionByNameOrId(reportsCollection); err != nil {
		return nil // an app without core's report migration owns no reports
	}
	owned, err := txApp.FindRecordsByFilter(reportsCollection, "owner = {:u}", "created,id", 0, 0, dbx.Params{"u": leaver.Id})
	if err != nil {
		return fmt.Errorf("list reports of %s: %w", leaver.Id, err)
	}
	heir := ""
	for _, r := range owned {
		move := plan.Mode == offboard.ModeReassign || r.GetBool("published")
		switch {
		case move:
			if heir == "" {
				if heir, err = reportHeir(txApp, leaver.Id, plan, actorUserID); err != nil {
					return err
				}
			}
			if err := moveReport(txApp, r, heir); err != nil {
				return err
			}
		case plan.Mode == offboard.ModeDeleteMyData:
			if err := txApp.Delete(r); err != nil {
				return fmt.Errorf("delete report %s: %w", r.Id, err)
			}
		}
	}
	return deleteDirectShares(txApp, dbx.Params{"u": leaver.Id}, `user = {:u} && group = ""`)
}

// moveReport gives a report to heir and removes heir's direct share on it.
func moveReport(txApp core.App, report *core.Record, heir string) error {
	report.Set("owner", heir)
	if err := txApp.Save(report); err != nil {
		return fmt.Errorf("reassign report %s: %w", report.Id, err)
	}
	return deleteDirectShares(txApp, dbx.Params{"r": report.Id, "u": heir}, `report = {:r} && user = {:u} && group = ""`)
}

func deleteDirectShares(txApp core.App, params dbx.Params, filter string) error {
	rows, err := txApp.FindRecordsByFilter(sharesCollection, filter, "", 0, 0, params)
	if err != nil {
		return fmt.Errorf("list report shares: %w", err)
	}
	for _, row := range rows {
		if err := txApp.Delete(row); err != nil {
			return fmt.Errorf("delete report share %s: %w", row.Id, err)
		}
	}
	return nil
}

// reportHeir is who takes the leaver's reports: the successor of a
// reassign plan; otherwise the actor, when an admin other than the leaver
// started the offboard; otherwise the first admin. A published report is
// never deleted, so with no heir the offboard is refused.
func reportHeir(txApp core.App, leaverID string, plan offboard.Plan, actorUserID string) (string, error) {
	if plan.Mode == offboard.ModeReassign {
		return plan.SuccessorUserID, nil // OffboardUser checked it
	}
	if actorUserID != "" && actorUserID != leaverID {
		actor, err := txApp.FindRecordById("users", actorUserID)
		if err == nil && isActiveAdmin(actor) {
			return actor.Id, nil
		}
	}
	admin, err := firstAdmin(txApp, leaverID)
	if err != nil {
		return "", err
	}
	if admin == nil {
		return "", fmt.Errorf("%w: this user owns published reports and no other admin can take them; make another user an admin first", offboard.ErrInvalidPlan)
	}
	return admin.Id, nil
}
```

- [ ] **Step 4: Register the handler**

Replace `$WS/tinycld/core/server/reports/sharing.go` with:

```go
package reports

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
	"tinycld.org/core/offboard"
)

// The saved-report collections (core migration 2090000008_create_reports.js).
const (
	reportsCollection = "reports"
	sharesCollection  = "report_shares"
)

// RegisterSharing binds saved reports and their sharing: report_shares as a
// group grant table, the save and publish guards on reports, the share
// guards on report_shares, shipped reports for the first admin who appears,
// and the offboard handler. Call it before groups.Register, which binds the
// expansion of every grant table registered by then.
//
// The collections' own rules (the migration) decide who may read and write
// a row; these hooks guard what a permitted write may change. A superuser
// request passes them unchanged: a superuser can change any row anyway.
func RegisterSharing(app core.App) {
	groups.RegisterGrantTable(groups.GrantTable{Collection: sharesCollection, ResourceField: "report"})
	app.OnRecordCreateRequest(reportsCollection).BindFunc(onReportCreate)
	app.OnRecordUpdateRequest(reportsCollection).BindFunc(onReportUpdate)
	app.OnRecordDeleteRequest(reportsCollection).BindFunc(onReportDelete)
	app.OnRecordCreateRequest(sharesCollection).BindFunc(onShareCreate)
	app.OnRecordUpdateRequest(sharesCollection).BindFunc(onShareUpdate)
	app.OnRecordDeleteRequest(sharesCollection).BindFunc(onShareDelete)
	app.OnRecordAfterCreateSuccess("users").BindFunc(shipForAdmin)
	app.OnRecordAfterUpdateSuccess("users").BindFunc(shipForAdmin)
	offboard.RegisterHandler("reports", offboardReports)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/ ./offboard/`
Expected: no gofmt or vet output; `ok` for both.

- [ ] **Step 6: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each.

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/offboard.go core/server/reports/sharing.go core/server/reports/offboard_test.go
git commit -m "feat(core): offboarding moves or deletes saved reports and never deletes a published one"
```

---

### Task 9: Saved reports in the shared composition, and their audit log

**Files:**
- Modify: `$WS/tinycld/core/server/coreserver/server.go` (`RegisterSharedCore`: `reports.RegisterSharing(app)` before `groups.Register(app)`)
- Modify: `$WS/tinycld/core/server/coreserver/audit.go` (`RegisterAuditHooks`)
- Modify: `$WS/tinycld/core/server/coreserver/composition_parity_test.go` (`hostHookCounts`)

**Interfaces:**
- Consumes: `reports.RegisterSharing(app core.App)`, `audit.RegisterCollection`, `audit.RegisterCollections`, `audit.LabelFromField`.
- Produces: every composition guards saved reports, expands report shares by group, ships package reports and offboards report owners; shares and publishes are audited.

- [ ] **Step 1: Update the golden counts first (the failing test)**

In `$WS/tinycld/core/server/coreserver/composition_parity_test.go`, in `hostHookCounts`, set these eight entries (leave the comments that follow them):

```go
	"OnRecordAfterCreateSuccess":          6,
	"OnRecordAfterUpdateSuccess":          4,
	"OnRecordCreate":                      4,
	"OnRecordCreateRequest":               16,
	"OnRecordDelete":                      5,
	"OnRecordDeleteRequest":               18,
	"OnRecordUpdate":                      8, // incl. app passwords revoked on a tokenKey change
	"OnRecordUpdateRequest":               19,
```

- [ ] **Step 2: Run the tripwire to verify it fails**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./coreserver/ -run TestRegisterBindsTheRecordedHandlerCounts`
Expected: FAIL with eight lines: `OnRecordAfterCreateSuccess: Register binds 5 handler(s), recorded 6`, `OnRecordAfterUpdateSuccess: … binds 3 …, recorded 4`, `OnRecordCreate: … 3 …, recorded 4`, `OnRecordCreateRequest: … 12 …, recorded 16`, `OnRecordDelete: … 4 …, recorded 5`, `OnRecordDeleteRequest: … 14 …, recorded 18`, `OnRecordUpdate: … 7 …, recorded 8`, `OnRecordUpdateRequest: … 15 …, recorded 19`.

- [ ] **Step 3: Register sharing**

In `$WS/tinycld/core/server/coreserver/server.go`, in `RegisterSharedCore`, replace:

```go
	pkgaccess.Register(app)
	groups.Register(app)
```

with:

```go
	pkgaccess.Register(app)
	// Saved reports and their shares. Before groups.Register: report_shares is
	// a group grant table, and groups binds the expansion of every grant
	// table registered by then. Shared: a share reaches its readers wherever
	// the report runs.
	reports.RegisterSharing(app)
	groups.Register(app)
```

(`tinycld.org/core/reports` is imported already, for `reports.Register`.)

- [ ] **Step 4: Audit shares and publishes**

In `$WS/tinycld/core/server/coreserver/audit.go`, directly after `audit.RegisterCollections(app, []string{"group_members"}, nil)`, add:

```go

	// A report share or a publish changes who sees a report's design. The
	// spec and the server's broken mark stay out of the diffs: a spec can
	// hold a megabyte, so the log says that it changed, not what it holds.
	audit.RegisterCollection(app, "reports", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("title"),
		Redact:       []string{"spec", "broken"},
	})
	audit.RegisterCollections(app, []string{"report_shares"}, nil)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l coreserver && go vet -tags no_ui ./coreserver/ && go test -tags no_ui -count=1 ./coreserver/`
Expected: no gofmt or vet output; `ok  tinycld.org/core/coreserver`.

- [ ] **Step 6: Run the quality gate** (the commands of Task 1 Step 7). Expected: exit 0 each; isolation `0 reference(s)` (core's own collections are not package names).

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/coreserver/server.go core/server/coreserver/audit.go \
  core/server/coreserver/composition_parity_test.go
git commit -m "feat(core): saved reports and shares in the shared composition and the audit log"
```

---

### Task 10: `docs/packages.md`: shipped and saved reports, and the final check

**Files:**
- Modify: `$WS/tinycld/docs/packages.md` — the registry table in "### Registries: how core learns what a package provides", and a new subsection "### Shipped and saved reports" directly before "### Test build tag".

**Interfaces:**
- Consumes: the Go API of Tasks 1 to 9.
- Produces: package-author documentation. No in-app help topic (Ruling P19).

- [ ] **Step 1: Add the registry row**

Directly after the table row for `reports.RegisterSource`, add:

```markdown
| the reports it ships, published to every user (see "Shipped and saved reports" below) | `reports.RegisterShipped` |
```

- [ ] **Step 2: Add the subsection**

Insert before `### Test build tag`:

````markdown
### Shipped and saved reports

People save reports in core's `reports` collection and share them through
`report_shares`, with users or groups, as viewer or editor. A share gives the
design only: each reader runs the report with their own rights. An admin can
publish a report to every user except guests.

A package ships its own reports as published reports. Register them from the
package's `Register(app)`:

```go
reports.RegisterShipped(reports.Shipped{
    Key:         "zoo.feed_costs", // "<slug>.<name>", stable
    Title:       "Feed costs",
    Description: "Every feeding's cost",
    Spec:        json.RawMessage(feedCostsSpec),
    Revision:    1,
})
```

- When the server starts, and when an admin is saved, core creates each
  shipped report that is missing. The first admin (the oldest owner, else
  the oldest admin) owns and publishes it. A fresh install gets its shipped
  reports when the setup wizard creates the first owner.
- Raise `Revision` whenever `Spec`, `Title` or `Description` changes. At the
  next start, core replaces the stored report's spec, title and description.
  It keeps the owner, and keeps the report unpublished if an admin
  unpublished it.
- A shipped spec that does not validate against the sources is logged and
  skipped. Pin the spec in the package's tests: call `reports.CheckSources`,
  then `reports.Load` and `reports.Validate`.
- A shipped report cannot be deleted while the package ships it; an admin
  unpublishes it instead.

A source, link or field can disappear when a package is upgraded or removed.
Saved reports then cannot run: a run answers 422 and names the missing part.
At every start, core marks each such report (`reports.broken`); in dev it
logs each one with its owner. To list them with their owners:

```sh
tinycld reports validate
```

It exits non-zero when a report is broken. Keep an alias for every renamed
field or link (see "Report sources" above), so saved reports keep working.

When a user is offboarded, their reports go to the successor, or are deleted
with their shares. A published report is never deleted: it goes to the
successor, the admin who offboards the user, or the first admin.
````

- [ ] **Step 3: Check the result**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && grep -n "reports.RegisterShipped\|### Shipped and saved reports\|tinycld reports validate" docs/packages.md`
Expected: the table row, the heading, the example and the command. Read the section once to confirm the nested code fences close.

- [ ] **Step 4: Run every check once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet -tags no_ui ./... && go test -tags no_ui -count=1 ./... && go test -tags no_ui -count=1 -race ./reports/ ./groups/ ./offboard/`
Expected: exit 0 (`ok` for every package; the `reports` race run took about 8 minutes in the probe, so run it in the background and wait on it).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts`
Expected: Biome, tsc and vitest pass for the `tinycld` member; isolation `0 reference(s)`.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/server && go vet . && go build -o /dev/null .`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add docs/packages.md
git commit -m "docs(core): shipped and saved reports for package authors"
```

---

## Spec coverage

| Spec item | Where |
|---|---|
| `reports`: owner, title, description, spec, source, published, published_by, created, updated | Task 1 (migration); Task 2 (`source` from the spec, `published_by` by the server) |
| `report_shares`: report, user, group, role viewer/editor; direct, grant, derived rows expanded by `core/server/groups` | Task 1 (migration, `groups.RegisterGrantTable`, `BindHooks`); Task 3 (row kinds guarded); `TestAGroupShareFollowsTheGroup` |
| The login requirement on every rule that reads `report_shares.user` | Task 1 (rules; `TestReportRulesGuardGrantRows`; rlstest's core scan) |
| View/list: owner, a share for the reader, or published | Task 1 `TestReportRulesListAndView`, `TestReportRulesGuestShareAndDisabledOwner` |
| Update: owner or editor | Task 1 `TestReportRulesUpdate` |
| Only an admin changes `published` (a server hook) | Task 2 `TestOnlyAnAdminPublishes` |
| Delete: owner, or an admin for a published report | Task 1 `TestReportRulesDelete` |
| Create: any signed-in user, owner = the caller | Task 1 `TestReportRulesCreate` |
| Shares: the owner and editors add and remove; a viewer cannot | Task 1 `TestShareRulesCreateAndDelete`, `TestShareRulesList` |
| Offboard: reassigned or deleted with their shares; a published report always reassigned | Task 8 |
| Published reports as templates ("Save as" keeps a copy) | Task 1 (create with a copied spec is a plain create); the button → p5-report-builder |
| A package's shipped reports: published, owned by the first admin, upgraded when their spec changes | Task 7 (Ruling P12) |
| Spec versions: saved upgraded on its next save; aliases resolved on save | Task 2 `TestASpecIsCheckedAndSavedLoaded` (Ruling P7) |
| `reports validate` (also at startup in dev) lists broken saved reports with the owner | Tasks 5, 6 (Ruling P10, P11) |
| The admin report list shows broken reports | Task 5 (`reports.broken`); the badge → p5-report-builder |
| A report with an unresolved reference opens in an error state naming the missing part | Task 4 `TestASavedReportNamesWhatIsMissing` (422); the view → p5-report-builder |
| Run endpoint `{ reportId, params, options }`; 403 when the caller cannot view it | Task 4 |
| Errors: invalid spec not saved, field-level errors | Task 2 |
| Errors: no access (a share removed while open) | Task 4 (403 after the share goes; the `report_shares` topic triggers the re-run) |
| Security: a share or publish gives the design only | Task 4 `TestASavedReportRunsAsItsReader` (bob sees his own 1150, not ann's 4500) |
| Security: never SQL; values bound | Task 4 (`reportId` is a string looked up by id; strict decode) |
| Testing: Go tests per rule cell through the API | Task 1 |
| Help topic | p5-report-builder (Ruling P19) |

## Concerns for the reviewer

- **Ruling P4 reads the spec literally:** an admin who is not the owner or an editor of a published report cannot unpublish it (only delete it). If the product wants admins to unpublish any published report, the update rule gains `|| (published = true && <admin>)` and `onReportUpdate` must then limit such an admin to the `published` field.
- **Ruling P10 checks saved reports at every start, not only in dev,** so the hub can show broken reports in production. The work is one read per 50 reports and a write only for a report whose mark changed.
- **Ruling P12 replaces "upgraded by the package's migrations" with a `Revision` on the Go registry,** because a fresh install has no admin while its migrations run.
- **The share cap can be passed by concurrent requests** (two shares added at the same moment both pass the count): it bounds growth, it is not exact. A transaction around the count and the insert would serialize every share write of every report.
- **The migration reaches the dev server and e2e only after the next `pnpm install` at the workspace root** (the generator writes one symlink per core migration into `tinycld/server/pb_migrations`). No task here runs the dev server or e2e.
