# First-run Setup Wizard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the standalone `/a/setup?token=…` form with a multi-step first-run wizard in core that packages can extend with their own steps.

**Architecture:** The server gets a short setup code (not a 64-char token), a verify endpoint with lockout, and a `setup.wizard` row in `system_settings` that marks a new deployment. The generator gets a `setupSteps` manifest field, emitted into `tinycld.config.ts` like `settings`. A core registry merges the core steps with package steps and sorts them by fractional-indexing key. A shell renders the steps next to a live workspace preview. Entry, resume and "done" are pure functions over the step statuses and the state row.

**Tech Stack:** Go (PocketBase 0.3x hooks/routes), TypeScript, React Native + react-native-web (Expo Router), pbtsdb + TanStack DB, react-hook-form + zod, Zustand, `fractional-indexing`, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-09-25-first-run-setup-wizard-design.md`

## Global Constraints

- Web and native both work. No web-only or native-only code paths except where the existing code already has them (`Platform.OS === 'web'` for `window.location.origin`).
- Core never names a package or a kind of deployment (`pnpm run check:core-isolation` must pass).
- No raw hex colors. Use semantic Tailwind tokens (`bg-primary`, `text-muted-foreground`, `bg-rail-background`, `text-rail-text`, …) or `useThemeColor(...)`.
- No `useEffect`+`useState` to sync data. Server data via `useLiveQuery`; writes via `useMutation` from `@tinycld/core/lib/mutations`; shared UI state via Zustand (`@tinycld/core/lib/store`).
- Keep JSX minimal: no `.map()`, complex ternaries or calculations in the returned JSX. Use `isVisible` props instead of `{cond && <Big/>}`.
- Never `any`, never `biome-ignore`, never `console.*` in runtime code (use `log` from `@tinycld/core/lib/logger`). Server logging via the existing `srvLog` in `coreserver`.
- Setup code: 8 characters from `23456789ABCDEFGHJKMNPQRSTUVWXYZ`, displayed `XXXX-XXXX`, printed link uses `?code=XXXXXXXX` (no dash).
- Lockout: 5 failures from one IP in 10 minutes lock that IP for 10 minutes; 20 failures in total regenerate and reprint the code.
- Wizard state key: `setup.wizard`. Value JSON: `{ startedAt, acknowledged: string[], skipped: string[], dismissedAt?, completedAt? }`.
- Core step order keys: workspace `a0`, apps `a1`, email `a2`, team `a3`. Sort by plain string comparison (`a < b`), ties by registry id. Steps with no key sort last.
- Registry step id: `<slug>:<id>`; core steps use slug `core`. In URLs the `:` becomes `.` (`/a/setup/core.workspace`).
- Core slot for package content in the invite step: target `core`, slot `setup-team`.
- Copy (verbatim):
  - Code error: "That code does not match. Check the server log for the latest code."
  - Lockout: "Too many tries. Wait 10 minutes, or restart the server for a new code."
  - Already set up: "This server is already set up."
  - Code help: "Code not in the log? Restart the server. A new code prints each time it starts until the server is claimed."
  - Apps intro: "These apps come with your server. Clear an app to hide it from everyone. You can show it again, or add more apps, at any time in Settings → Packages."
- Help shortcuts use Mac glyphs only; never hand-write a hostname in help (use `{{server-host}}`).

## File Structure

**Server (Go, `core/server/coreserver/`)**
- `setup_code.go` (new) — code generation, normalization, `setupGuard` (code + lockout + clock), printing.
- `setup_bootstrap.go` (modify) — use `setupGuard`; `/api/setup/verify`; `/api/setup/init` takes `code` + `name`, runs under the guard lock, writes wizard state.
- `setup_wizard_state.go` (new) — `MarkSetupWizardStarted(app)`.
- `owner_command.go` (modify) — call `MarkSetupWizardStarted` when the owner is created.
- `org_name.go` (new) — `POST /api/org-info/name` (owner/admin) writes `Meta.AppName`.
- `pkg_seed.go` (modify) — stop re-enabling disabled rows; extract `loadBundledPackages()`.
- `pkg_enable_hook.go` (new) — on update `disabled → enabled`, choose `bundled` or `installed`.
- `server.go` (modify) — bind the new hook and the name endpoint in `RegisterSharedCore`.

**Generator (`scripts/`)**
- `load-manifest.ts`, `describe-packages.ts`, `gen-config.ts` (modify) — `setupSteps`; `target: 'core'` slot validation.
- `core/lib/packages/types.ts`, `core/lib/packages/config-types.ts` (modify) — types.

**Client core (`core/lib/setup/`, new directory)**
- `core-slots.ts` — `CORE_SLOTS`.
- `types.ts` — step module contract.
- `order.ts` — `compareStepOrder`, `isValidOrderKey`.
- `registry.ts` — `setupStepEntries` (core + generated), `stepIdToParam`, `paramToStepId`.
- `wizard-logic.ts` — `parseWizardState`, `summarizeWizard`, `shouldOpenWizard`.
- `use-setup-wizard-state.ts` — read/write the `setup.wizard` row.
- `use-setup-steps.ts` — load step modules; status chain.
- `use-needs-setup.ts` — `/api/setup/check` query.
- `setup-preview-store.ts` — Zustand store for the draft workspace name.
- `set-package-enabled.ts` — shared enable/disable mutation body for `pkg_registry`.

**Client UI (`core/components/setup/wizard/`, new directory)**
- `SetupWizardShell.tsx`, `ProgressSegments.tsx`, `WorkspacePreview.tsx`, `WorkspacePreviewStrip.tsx`, `ServerLogPreview.tsx`, `use-workspace-preview.ts`
- `ClaimServerStep.tsx`, `CreateOwnerStep.tsx`, `CodeInput.tsx`, `PreAuthSetup.tsx`
- `steps/WorkspaceStep.tsx`, `steps/AppsStep.tsx`, `steps/EmailStep.tsx`, `steps/TeamStep.tsx`, `steps/DoneStep.tsx`
- `SetupStepScreen.tsx` — renders one step by id inside the shell.
- `FinishSetupCard.tsx` — Settings card.
- `core/components/settings/members/use-invite-member.ts` (new) — invite schema + mutation shared with `MembersDrawer`.

**Routes (`app/a/`)**
- delete `setup.tsx`; add `setup/index.tsx`, `setup/recovery.tsx`, `setup/[step].tsx`.
- `app/a/(app)/_layout.tsx` (modify) — redirect owners/admins into the wizard.
- `app/a/(app)/settings/index.tsx` (modify) — `FinishSetupCard`.
- `core/components/workspace/AuthGate.tsx` (modify) — redirect to `/a/setup` when setup is needed.
- `core/components/setup/SetupPage.tsx` (modify) — recovery only.
- delete `core/components/setup/SetupWizard.tsx`.

**Help, tests, CI**
- `core/help/first-run-setup.md` (new), `core/help/installing-packages.md` (modify).
- `tests/standalone/boot-binary.ts` (new), `tests/standalone/standalone-binary.spec.ts` (modify), `tests/standalone/first-run-wizard.spec.ts` (new).
- `tests/install/setup-and-packages.spec.ts`, `tests/install/run-first-boot-admin.sh`, `tests/install/run-todo-install.sh`, `.github/workflows/smoke-test-image.yml` (modify) — code instead of token.

## Commands

- Go tests: `cd core/server && go test ./coreserver -run '<Name>' -v`
- Unit tests: `pnpm exec vitest run <path>` (from `tinycld/`)
- Member check: `pnpm exec tinycld-pkg check` (from `tinycld/`)
- Regenerate: `pnpm run packages:generate`
- Core isolation: `pnpm run check:core-isolation`

---

### Task 1: Hidden packages stay hidden

**Files:**
- Modify: `core/server/coreserver/pkg_seed.go:28-120`
- Create: `core/server/coreserver/pkg_enable_hook.go`
- Modify: `core/server/coreserver/server.go` (`RegisterSharedCore`, near line 361)
- Create: `core/lib/setup/set-package-enabled.ts`
- Modify: `core/components/setup/PackageManager.tsx:498-513`
- Test: `core/server/coreserver/pkg_seed_test.go`, `core/server/coreserver/pkg_enable_hook_test.go`

**Interfaces:**
- Produces: Go `loadBundledPackages() ([]bundledPackage, error)`, `bundledSlugSet() map[string]bool`, `RegisterPkgEnableHook(app core.App)`.
- Produces: TS `enabledStatusFor(isEnabled: boolean): 'installed' | 'disabled'` in `core/lib/setup/set-package-enabled.ts`. The client writes `installed` to enable; the server hook corrects it to `bundled` when the slug is bundled.

- [ ] **Step 1: Write the failing boot-sync test** — append to `pkg_seed_test.go`:

```go
// A bundled package the owner disabled must stay disabled across boots.
// SyncBundledPackages used to flip it back to bundled, so every restart
// undid the owner's choice.
func TestSyncBundledPackagesKeepsOwnerDisabledRow(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	writeBundledJSON(t, dir, []bundledPackage{{Name: "Drive", Slug: "drive", Version: "1.0.0"}})
	withCwd(t, dir)

	SyncBundledPackages(app)
	rec, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'drive'", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec.Set("status", "disabled")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	SyncBundledPackages(app)

	after, _ := app.FindFirstRecordByFilter("pkg_registry", "slug = 'drive'", nil)
	if got := after.GetString("status"); got != "disabled" {
		t.Fatalf("status after re-sync = %q, want disabled", got)
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `cd core/server && go test ./coreserver -run TestSyncBundledPackagesKeepsOwnerDisabledRow -v`
Expected: FAIL — `status after re-sync = "bundled", want disabled`.

- [ ] **Step 3: Fix boot sync and extract the loader** — in `pkg_seed.go`, delete the block

```go
		if existing.GetString("status") == "disabled" {
			// Re-enable if it was disabled but is still bundled
			existing.Set("status", "bundled")
		}
```

and replace the read/parse at the top of `SyncBundledPackages` with a shared loader:

```go
// loadBundledPackages reads bundled-packages.json. A missing file is not an
// error: a dev tree before the first generate has none.
func loadBundledPackages() ([]bundledPackage, error) {
	jsonPath := findBundledPackagesJSON()
	if jsonPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", jsonPath, err)
	}
	var packages []bundledPackage
	if err := json.Unmarshal(data, &packages); err != nil {
		return nil, fmt.Errorf("parse %s: %w", jsonPath, err)
	}
	return packages, nil
}

// bundledSlugSet is the set of slugs compiled into this build.
func bundledSlugSet() map[string]bool {
	packages, err := loadBundledPackages()
	if err != nil {
		srvLog.Error("failed to load bundled packages", "err", err)
	}
	set := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		set[pkg.Slug] = true
	}
	return set
}

func SyncBundledPackages(app core.App) {
	packages, err := loadBundledPackages()
	if err != nil {
		srvLog.Error("failed to load bundled packages", "err", err)
		return
	}
	if packages == nil {
		srvLog.Info("bundled-packages.json not found, skipping sync")
		return
	}
	// ... rest unchanged from "collection, err := app.FindCollectionByNameOrId"
```

Add `"fmt"` to the imports.

- [ ] **Step 4: Run the test and see it pass**

Run: `cd core/server && go test ./coreserver -run 'TestSyncBundledPackages' -v`
Expected: PASS (all `TestSyncBundledPackages*`).

- [ ] **Step 5: Write the failing re-enable test** — create `pkg_enable_hook_test.go`:

```go
package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func saveRegistryRow(t *testing.T, app core.App, slug, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("slug", slug)
	rec.Set("name", slug)
	rec.Set("status", status)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// Re-enabling a disabled row must restore where the package came from. The
// client cannot know: by the time it re-enables, the status says only
// "disabled".
func TestPkgEnableHookRestoresSource(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	writeBundledJSON(t, dir, []bundledPackage{{Name: "Drive", Slug: "drive", Version: "1.0.0"}})
	withCwd(t, dir)
	RegisterPkgEnableHook(app)

	cases := []struct{ slug, want string }{
		{"drive", "bundled"},
		{"todo", "installed"},
	}
	for _, c := range cases {
		rec := saveRegistryRow(t, app, c.slug, "disabled")
		rec.Set("status", "installed")
		if err := app.Save(rec); err != nil {
			t.Fatal(err)
		}
		got, _ := app.FindRecordById("pkg_registry", rec.Id)
		if got.GetString("status") != c.want {
			t.Errorf("%s: status = %q, want %q", c.slug, got.GetString("status"), c.want)
		}
	}
}

// Disabling and unrelated edits pass through untouched.
func TestPkgEnableHookLeavesOtherWritesAlone(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	writeBundledJSON(t, dir, []bundledPackage{{Name: "Drive", Slug: "drive", Version: "1.0.0"}})
	withCwd(t, dir)
	RegisterPkgEnableHook(app)

	rec := saveRegistryRow(t, app, "drive", "bundled")
	rec.Set("status", "disabled")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	got, _ := app.FindRecordById("pkg_registry", rec.Id)
	if got.GetString("status") != "disabled" {
		t.Fatalf("disable was rewritten to %q", got.GetString("status"))
	}
}
```

- [ ] **Step 6: Run it and see it fail**

Run: `cd core/server && go test ./coreserver -run TestPkgEnableHook -v`
Expected: FAIL — `undefined: RegisterPkgEnableHook`.

- [ ] **Step 7: Implement the hook** — create `pkg_enable_hook.go`:

```go
package coreserver

import "github.com/pocketbase/pocketbase/core"

// RegisterPkgEnableHook makes the server, not the client, choose a package's
// status when it is turned back on. "disabled" is shared by "the owner hid
// it" and "it left the build", so the client cannot tell which source to
// restore; bundled-packages.json can.
//
// Model-level hook so it covers app.Save() as well as API writes.
func RegisterPkgEnableHook(app core.App) {
	app.OnRecordUpdate("pkg_registry").BindFunc(func(e *core.RecordEvent) error {
		wasDisabled := e.Record.Original().GetString("status") == "disabled"
		next := e.Record.GetString("status")
		if wasDisabled && next != "disabled" && next != "available" {
			if bundledSlugSet()[e.Record.GetString("slug")] {
				e.Record.Set("status", "bundled")
			} else {
				e.Record.Set("status", "installed")
			}
		}
		return e.Next()
	})
}
```

In `server.go` `RegisterSharedCore`, add next to the other shared registrations:

```go
	RegisterPkgEnableHook(app)
```

- [ ] **Step 8: Run the Go tests and see them pass**

Run: `cd core/server && go test ./coreserver -run 'TestPkgEnableHook|TestSyncBundledPackages|TestUpsertPkgRegistry' -v`
Expected: PASS.

- [ ] **Step 9: Share the client toggle** — create `core/lib/setup/set-package-enabled.ts`:

```ts
/**
 * The status the client writes to turn a package on or off. Enabling always
 * writes `installed`: the server's pkg_registry hook corrects it to `bundled`
 * when the slug is compiled into this build (see pkg_enable_hook.go). The
 * client cannot decide that itself, because a disabled row no longer says
 * where it came from.
 */
export function enabledStatusFor(isEnabled: boolean): 'installed' | 'disabled' {
    return isEnabled ? 'installed' : 'disabled'
}
```

In `PackageManager.tsx` replace the toggle mutation body:

```ts
    const toggle = useMutation({
        mutationFn: mutation(function* () {
            yield pkgRegistryCollection.update(pkg.id, draft => {
                draft.status = enabledStatusFor(!isEnabled)
            })
        }),
        onError: err => captureException('Failed to toggle package status', err),
    })
```

and add `import { enabledStatusFor } from '@tinycld/core/lib/setup/set-package-enabled'`.

- [ ] **Step 10: Unit test the helper** — create `core/lib/setup/__tests__/set-package-enabled.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { enabledStatusFor } from '../set-package-enabled'

describe('enabledStatusFor', () => {
    it('writes installed to enable; the server restores bundled', () => {
        expect(enabledStatusFor(true)).toBe('installed')
    })
    it('writes disabled to disable', () => {
        expect(enabledStatusFor(false)).toBe('disabled')
    })
})
```

Run: `pnpm exec vitest run core/lib/setup/__tests__/set-package-enabled.test.ts`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add core/server/coreserver/pkg_seed.go core/server/coreserver/pkg_seed_test.go \
  core/server/coreserver/pkg_enable_hook.go core/server/coreserver/pkg_enable_hook_test.go \
  core/server/coreserver/server.go core/lib/setup/set-package-enabled.ts \
  core/lib/setup/__tests__/set-package-enabled.test.ts core/components/setup/PackageManager.tsx
git commit -m "fix(packages): keep a hidden package hidden across restarts"
```

---

### Task 2: Setup code and lockout

**Files:**
- Create: `core/server/coreserver/setup_code.go`
- Test: `core/server/coreserver/setup_code_test.go`

**Interfaces:**
- Produces:
  - `const setupCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"`
  - `func generateSetupCode() (string, error)` — 8 chars, no dash.
  - `func normalizeSetupCode(s string) string` — uppercase, drop everything outside the alphabet's character class `[A-Z0-9]`.
  - `func formatSetupCode(code string) string` — `XXXX-XXXX`.
  - `type setupGuard struct` with `newSetupGuard(now func() time.Time, announce func(code string)) *setupGuard`, methods `Issue() error`, `NeedsSetup() bool`, `Check(ip, input string) checkResult`, `Consume(ip, input string, create func() error) (checkResult, error)`.
  - `type checkResult int` with `checkOK`, `checkMismatch`, `checkLocked`, `checkNoSetup`.

- [ ] **Step 1: Write the failing tests** — create `setup_code_test.go`:

```go
package coreserver

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGenerateSetupCodeShape(t *testing.T) {
	for i := 0; i < 200; i++ {
		code, err := generateSetupCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 8 {
			t.Fatalf("len(%q) = %d, want 8", code, len(code))
		}
		for _, r := range code {
			if !strings.ContainsRune(setupCodeAlphabet, r) {
				t.Fatalf("%q contains %q, which is not in the alphabet", code, r)
			}
		}
	}
}

func TestNormalizeAndFormatSetupCode(t *testing.T) {
	if got := normalizeSetupCode(" k7qm-3xpd "); got != "K7QM3XPD" {
		t.Errorf("normalize = %q", got)
	}
	if got := formatSetupCode("K7QM3XPD"); got != "K7QM-3XPD" {
		t.Errorf("format = %q", got)
	}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestGuard(t *testing.T) (*setupGuard, *fakeClock, *[]string) {
	t.Helper()
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	var announced []string
	g := newSetupGuard(clock.now, func(code string) { announced = append(announced, code) })
	if err := g.Issue(); err != nil {
		t.Fatal(err)
	}
	return g, clock, &announced
}

func TestSetupGuardCheck(t *testing.T) {
	g, _, announced := newTestGuard(t)
	code := (*announced)[0]
	if got := g.Check("1.1.1.1", strings.ToLower(formatSetupCode(code))); got != checkOK {
		t.Fatalf("correct code = %v, want checkOK", got)
	}
	if got := g.Check("1.1.1.1", "AAAAAAAA"); got != checkMismatch {
		t.Fatalf("wrong code = %v, want checkMismatch", got)
	}
}

func TestSetupGuardLocksAnIPAfterFiveFailures(t *testing.T) {
	g, clock, announced := newTestGuard(t)
	code := (*announced)[0]
	for i := 0; i < 5; i++ {
		g.Check("1.1.1.1", "AAAAAAAA")
	}
	if got := g.Check("1.1.1.1", code); got != checkLocked {
		t.Fatalf("after 5 failures = %v, want checkLocked", got)
	}
	if got := g.Check("2.2.2.2", code); got != checkOK {
		t.Fatalf("other IP = %v, want checkOK", got)
	}
	clock.t = clock.t.Add(10*time.Minute + time.Second)
	if got := g.Check("1.1.1.1", code); got != checkOK {
		t.Fatalf("after lockout expiry = %v, want checkOK", got)
	}
}

func TestSetupGuardRegeneratesAfterTwentyFailures(t *testing.T) {
	g, _, announced := newTestGuard(t)
	first := (*announced)[0]
	for i := 0; i < 20; i++ {
		// A different IP each time so no single-IP lockout masks the total.
		g.Check(strings.Repeat("9", i+1), "AAAAAAAA")
	}
	if len(*announced) != 2 {
		t.Fatalf("announced %d codes, want 2", len(*announced))
	}
	if got := g.Check("3.3.3.3", first); got != checkMismatch {
		t.Fatalf("old code after regeneration = %v, want checkMismatch", got)
	}
}

// Two concurrent inits with the right code: exactly one creates the owner.
func TestSetupGuardConsumeIsSingleUse(t *testing.T) {
	g, _, announced := newTestGuard(t)
	code := (*announced)[0]
	var mu sync.Mutex
	created := 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = g.Consume("1.1.1.1", code, func() error {
				mu.Lock()
				created++
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if created != 1 {
		t.Fatalf("created %d owners, want 1", created)
	}
	if g.NeedsSetup() {
		t.Fatal("NeedsSetup after a successful consume")
	}
}

// A failed create keeps the code, so the person can fix the input and retry.
func TestSetupGuardConsumeKeepsCodeOnCreateError(t *testing.T) {
	g, _, announced := newTestGuard(t)
	code := (*announced)[0]
	_, err := g.Consume("1.1.1.1", code, func() error { return errors.New("boom") })
	if err == nil {
		t.Fatal("want the create error back")
	}
	if !g.NeedsSetup() {
		t.Fatal("code was cleared by a failed create")
	}
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `cd core/server && go test ./coreserver -run 'TestGenerateSetupCode|TestNormalizeAndFormat|TestSetupGuard' -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement** — create `setup_code.go`:

```go
package coreserver

import (
	"crypto/rand"
	"crypto/subtle"
	"math/big"
	"strings"
	"sync"
	"time"
)

// setupCodeAlphabet leaves out 0/O, 1/I/L so a code read off a terminal and
// typed by hand cannot be misread. 31^8 is about 40 bits; with the lockout
// below that is far past what an online guesser can reach.
const setupCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

const (
	setupCodeLength      = 8
	setupIPFailureLimit  = 5
	setupIPWindow        = 10 * time.Minute
	setupTotalFailLimit  = 20
)

func generateSetupCode() (string, error) {
	max := big.NewInt(int64(len(setupCodeAlphabet)))
	b := make([]byte, setupCodeLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = setupCodeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// normalizeSetupCode accepts what a person types or pastes: any case, with
// the display dash or spaces.
func normalizeSetupCode(s string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func formatSetupCode(code string) string {
	if len(code) != setupCodeLength {
		return code
	}
	return code[:4] + "-" + code[4:]
}

type checkResult int

const (
	checkOK checkResult = iota
	checkMismatch
	checkLocked
	checkNoSetup
)

// setupGuard owns the first-run code and the lockout. One mutex covers the
// code, the counters AND the owner creation in Consume, so a check and the
// clearing of the code can never interleave with a second request.
type setupGuard struct {
	mu            sync.Mutex
	code          string
	failures      map[string][]time.Time
	totalFailures int
	now           func() time.Time
	announce      func(code string)
}

func newSetupGuard(now func() time.Time, announce func(code string)) *setupGuard {
	return &setupGuard{failures: map[string][]time.Time{}, now: now, announce: announce}
}

// Issue makes a new code and announces it. Called at boot while no owner
// exists, and again when the total failure limit is reached.
func (g *setupGuard) Issue() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.issueLocked()
}

func (g *setupGuard) issueLocked() error {
	code, err := generateSetupCode()
	if err != nil {
		return err
	}
	g.code = code
	g.totalFailures = 0
	g.failures = map[string][]time.Time{}
	g.announce(code)
	return nil
}

func (g *setupGuard) NeedsSetup() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.code != ""
}

func (g *setupGuard) Check(ip, input string) checkResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.checkLocked(ip, input)
}

func (g *setupGuard) checkLocked(ip, input string) checkResult {
	if g.code == "" {
		return checkNoSetup
	}
	if g.isLockedLocked(ip) {
		return checkLocked
	}
	if subtle.ConstantTimeCompare([]byte(normalizeSetupCode(input)), []byte(g.code)) == 1 {
		return checkOK
	}
	g.failures[ip] = append(g.recentLocked(ip), g.now())
	g.totalFailures++
	if g.totalFailures >= setupTotalFailLimit {
		if err := g.issueLocked(); err != nil {
			srvLog.Error("setup: failed to regenerate the setup code", "err", err)
		}
	}
	return checkMismatch
}

func (g *setupGuard) recentLocked(ip string) []time.Time {
	cutoff := g.now().Add(-setupIPWindow)
	var kept []time.Time
	for _, at := range g.failures[ip] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	return kept
}

func (g *setupGuard) isLockedLocked(ip string) bool {
	return len(g.recentLocked(ip)) >= setupIPFailureLimit
}

// Consume checks the code and, on a match, runs create while still holding
// the lock. The code is cleared only when create succeeds, so a failed
// create (bad email, weak password) can be retried with the same code.
func (g *setupGuard) Consume(ip, input string, create func() error) (checkResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	result := g.checkLocked(ip, input)
	if result != checkOK {
		return result, nil
	}
	if err := create(); err != nil {
		return checkOK, err
	}
	g.code = ""
	return checkOK, nil
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd core/server && go test ./coreserver -run 'TestGenerateSetupCode|TestNormalizeAndFormat|TestSetupGuard' -race -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/server/coreserver/setup_code.go core/server/coreserver/setup_code_test.go
git commit -m "feat(setup): short setup code with per-IP and total lockout"
```

---

### Task 3: Wizard state row and owner creation

**Files:**
- Create: `core/server/coreserver/setup_wizard_state.go`
- Modify: `core/server/coreserver/owner_command.go:154-189` (`createOperatorIdentities`)
- Test: `core/server/coreserver/setup_wizard_state_test.go`

**Interfaces:**
- Consumes: `upsertSystemSetting(app, key, value, isSecret)` (`vapid_admin.go:62`), `createSystemSettingsCollection` (test helper, `system_config_test.go:16`), `ensureUsersCollection` (`owner_command_test.go:21`).
- Produces: `const setupWizardKey = "setup.wizard"`, `func MarkSetupWizardStarted(app core.App) error` — writes the row only when it does not exist.

- [ ] **Step 1: Write the failing tests** — create `setup_wizard_state_test.go`:

```go
package coreserver

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestMarkSetupWizardStartedWritesOnce(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createSystemSettingsCollection(t, app)

	if err := MarkSetupWizardStarted(app); err != nil {
		t.Fatal(err)
	}
	rec, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey})
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		StartedAt    string   `json:"startedAt"`
		Acknowledged []string `json:"acknowledged"`
		Skipped      []string `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(rec.GetString("value")), &state); err != nil {
		t.Fatal(err)
	}
	if state.StartedAt == "" || state.Acknowledged == nil || state.Skipped == nil {
		t.Fatalf("incomplete state: %+v", state)
	}

	// A second call (create-owner re-run) must not reset progress.
	rec.Set("value", `{"startedAt":"x","acknowledged":["core:apps"],"skipped":[]}`)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	if err := MarkSetupWizardStarted(app); err != nil {
		t.Fatal(err)
	}
	again, _ := app.FindRecordById("system_settings", rec.Id)
	if again.GetString("value") != `{"startedAt":"x","acknowledged":["core:apps"],"skipped":[]}` {
		t.Fatalf("progress was reset: %s", again.GetString("value"))
	}
}

// create-owner is how every non-wizard deployment gets its owner, so it must
// start the wizard too.
func TestCreateOperatorIdentitiesStartsWizard(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createSystemSettingsCollection(t, app)

	if _, err := createOperatorIdentities(app, "owner@example.com", "", "OwnerPass1234!", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey}); err != nil {
		t.Fatalf("wizard state row missing: %v", err)
	}
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `cd core/server && go test ./coreserver -run 'TestMarkSetupWizardStarted|TestCreateOperatorIdentitiesStartsWizard' -v`
Expected: FAIL — undefined `MarkSetupWizardStarted`.

- [ ] **Step 3: Implement** — create `setup_wizard_state.go`:

```go
package coreserver

import (
	"encoding/json"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// setupWizardKey holds the first-run wizard's progress. Only the two paths
// that create a deployment's owner write it, so a deployment set up before
// the wizard existed has no row and never sees the wizard.
const setupWizardKey = "setup.wizard"

// MarkSetupWizardStarted writes the initial wizard state unless a row exists.
func MarkSetupWizardStarted(app core.App) error {
	if _, err := app.FindFirstRecordByFilter(
		"system_settings", "key = {:key}", map[string]any{"key": setupWizardKey},
	); err == nil {
		return nil
	}
	value, err := json.Marshal(map[string]any{
		"startedAt":    time.Now().UTC().Format(time.RFC3339),
		"acknowledged": []string{},
		"skipped":      []string{},
	})
	if err != nil {
		return err
	}
	return upsertSystemSetting(app, setupWizardKey, string(value), false)
}
```

In `owner_command.go` `createOperatorIdentities`, after each successful owner creation (both the hash and the plaintext branch), start the wizard. Replace the tail of the function:

```go
	if passwordHash != "" {
		if _, cerr := CreateOwnerAccountWithHash(app, email, name, passwordHash); cerr != nil {
			return created, fmt.Errorf("create owner account: %w", cerr)
		}
	} else if _, cerr := CreateOwnerAccountNamed(app, email, name, password); cerr != nil {
		return created, fmt.Errorf("create owner account: %w", cerr)
	}
	// The wizard is a convenience; a deployment whose owner exists but whose
	// wizard row failed to write must still be provisioned.
	if werr := MarkSetupWizardStarted(app); werr != nil {
		srvLog.Warn("create-owner: could not start the setup wizard", "err", werr)
	}
	return true, nil
}
```

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd core/server && go test ./coreserver -run 'TestMarkSetupWizardStarted|TestCreateOperatorIdentities|TestCreateOwnerCommand' -v`
Expected: PASS. (`TestCreateOwnerCommand*` have no `system_settings` collection, so they exercise the warn path and must still pass.)

- [ ] **Step 5: Commit**

```bash
git add core/server/coreserver/setup_wizard_state.go core/server/coreserver/setup_wizard_state_test.go core/server/coreserver/owner_command.go
git commit -m "feat(setup): start the setup wizard when an owner is created"
```

---

### Task 4: Setup endpoints use the code

**Files:**
- Modify: `core/server/coreserver/setup_bootstrap.go` (whole file)
- Test: `core/server/coreserver/setup_bootstrap_test.go`

**Interfaces:**
- Consumes: `setupGuard`, `formatSetupCode`, `MarkSetupWizardStarted`, `createOwnerOperator`.
- Produces HTTP:
  - `GET /api/setup/check` → `200 {"needsSetup": bool}`
  - `POST /api/setup/verify {code}` → `200 {}` | `403 {"error","reason":"mismatch"|"locked"|"done"}`
  - `POST /api/setup/init {code,name,email,password,appUrl}` → `200 {authToken,email,userId}` | `400 {"error"}` | `403 {"error","reason"}`

- [ ] **Step 1: Write the failing handler test** — append to `setup_bootstrap_test.go`:

```go
func TestSetupInitCreatesOwnerAndStartsWizard(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createSystemSettingsCollection(t, app)

	var announced []string
	guard := newSetupGuard(time.Now, func(c string) { announced = append(announced, c) })
	if err := guard.Issue(); err != nil {
		t.Fatal(err)
	}

	req := setupInitRequest{
		Code: formatSetupCode(announced[0]), Name: "Dana Reyes",
		Email: "dana@example.com", Password: "OwnerPass1234!", AppURL: "https://cloud.example.com",
	}
	res, status := runSetupInit(app, guard, "1.1.1.1", req)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%v)", status, res)
	}
	owner, err := app.FindAuthRecordByEmail("users", "dana@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if owner.GetString("name") != "Dana Reyes" || owner.GetString("role") != "owner" {
		t.Fatalf("owner = name %q role %q", owner.GetString("name"), owner.GetString("role"))
	}
	if _, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey}); err != nil {
		t.Fatal("wizard state row missing")
	}
	if guard.NeedsSetup() {
		t.Fatal("code still active after init")
	}

	_, again := runSetupInit(app, guard, "1.1.1.1", req)
	if again != http.StatusForbidden {
		t.Fatalf("second init status = %d, want 403", again)
	}
}
```

Add imports `net/http` and `time`.

- [ ] **Step 2: Run it and see it fail**

Run: `cd core/server && go test ./coreserver -run TestSetupInitCreatesOwnerAndStartsWizard -v`
Expected: FAIL — `unknown field Code` / undefined `runSetupInit`.

- [ ] **Step 3: Rewrite `setup_bootstrap.go`** — keep the `TINYCLD_PUBLIC_URL` sync and `printBoxed`; replace token state and handlers:

```go
var setupState = newSetupGuard(time.Now, announceSetupCode)

// announceSetupCode prints the code on its own line and inside the link, so
// a person can either click the link or type the code into any client
// (a native app has no link to click).
func announceSetupCode(code string) {
	printBoxed(
		"Finish setup in your browser:",
		setupPublicURL()+approutes.Href("setup")+"?code="+code,
		"Setup code: "+formatSetupCode(code),
	)
}

var setupBaseURL string

// setupPublicURL prefers TINYCLD_PUBLIC_URL so the printed link matches where
// the person browses; PocketBase's baseURL is the bind address, which is
// wrong behind a reverse proxy.
func setupPublicURL() string {
	if publicURL := strings.TrimRight(os.Getenv("TINYCLD_PUBLIC_URL"), "/"); publicURL != "" {
		return publicURL
	}
	return strings.TrimRight(setupBaseURL, "/")
}

func RegisterSetupBootstrap(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// (keep the existing TINYCLD_PUBLIC_URL → Meta.AppURL sync here)

		e.InstallerFunc = func(_ core.App, _ *core.Record, baseURL string) error {
			setupBaseURL = baseURL
			return setupState.Issue()
		}

		e.Router.GET("/api/setup/check", func(re *core.RequestEvent) error {
			return re.JSON(http.StatusOK, map[string]bool{"needsSetup": setupState.NeedsSetup()})
		})
		e.Router.POST("/api/setup/verify", func(re *core.RequestEvent) error {
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
				return re.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
			}
			if result := setupState.Check(re.RealIP(), body.Code); result != checkOK {
				return setupRefusal(re, result)
			}
			return re.JSON(http.StatusOK, map[string]any{})
		})
		e.Router.POST("/api/setup/init", func(re *core.RequestEvent) error {
			var req setupInitRequest
			if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
				return re.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
			}
			body, status := runSetupInit(app, setupState, re.RealIP(), req)
			return re.JSON(status, body)
		})
		return e.Next()
	})
}

type setupInitRequest struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AppURL   string `json:"appUrl"`
}

var setupRefusalBodies = map[checkResult]map[string]string{
	checkMismatch: {"error": "That code does not match. Check the server log for the latest code.", "reason": "mismatch"},
	checkLocked:   {"error": "Too many tries. Wait 10 minutes, or restart the server for a new code.", "reason": "locked"},
	checkNoSetup:  {"error": "This server is already set up.", "reason": "done"},
}

func setupRefusal(re *core.RequestEvent, result checkResult) error {
	return re.JSON(http.StatusForbidden, setupRefusalBodies[result])
}

// runSetupInit is the handler body, split out so tests drive it without HTTP.
func runSetupInit(app core.App, guard *setupGuard, ip string, req setupInitRequest) (any, int) {
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		return map[string]string{"error": "Email and password are required."}, http.StatusBadRequest
	}
	var operator *core.Record
	result, err := guard.Consume(ip, req.Code, func() error {
		created, cerr := createSetupOwner(app, req)
		operator = created
		return cerr
	})
	if result != checkOK {
		return setupRefusalBodies[result], http.StatusForbidden
	}
	if err != nil {
		srvLog.Error("setup: owner creation failed", "err", err)
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}
	authToken, err := operator.NewAuthToken()
	if err != nil {
		return map[string]string{"error": "Owner created but failed to generate auth token."}, http.StatusInternalServerError
	}
	return map[string]string{"authToken": authToken, "email": req.Email, "userId": operator.Id}, http.StatusOK
}

// createSetupOwner mints both identities (see createOperatorIdentities for
// why there are two), saves the app URL and starts the wizard. It runs inside
// the guard's lock.
func createSetupOwner(app core.App, req setupInitRequest) (*core.Record, error) {
	superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return nil, fmt.Errorf("find superusers collection: %w", err)
	}
	superuser := core.NewRecord(superusers)
	superuser.SetEmail(req.Email)
	superuser.SetPassword(req.Password)
	superuser.SetVerified(true)
	if err := app.Save(superuser); err != nil {
		return nil, fmt.Errorf("create superuser: %w", err)
	}
	operator, err := createOwnerOperator(app, req.Email, req.Name, req.Password)
	if err != nil {
		return nil, fmt.Errorf("create owner: %w", err)
	}
	if req.AppURL != "" {
		app.Settings().Meta.AppURL = req.AppURL
		if err := app.Save(app.Settings()); err != nil {
			srvLog.Warn("setup: failed to save app URL", "err", err)
		}
	}
	if err := MarkSetupWizardStarted(app); err != nil {
		srvLog.Warn("setup: could not start the setup wizard", "err", err)
	}
	return operator, nil
}
```

Change `printBoxed(title, url string)` to `printBoxed(title string, lines ...string)` and print a blank line after the title then each line. Delete `setupToken`, `setupTokenMu`, `handleSetupInit`, `generateToken` (check `grep -rn generateToken core/server` first; keep it if anything else uses it). Keep the long comment about the two identities above `createSetupOwner`.

- [ ] **Step 4: Run the tests and see them pass**

Run: `cd core/server && go test ./coreserver -run 'TestSetup|TestCreateOwner' -race -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/server/coreserver/setup_bootstrap.go core/server/coreserver/setup_bootstrap_test.go
git commit -m "feat(setup): claim a new server with the short setup code"
```

---

### Task 5: Workspace name endpoint

**Files:**
- Create: `core/server/coreserver/org_name.go`
- Modify: `core/server/coreserver/server.go` (`RegisterSharedCore`)
- Test: `core/server/coreserver/org_name_test.go`

**Interfaces:**
- Consumes: `isOrgAdmin(record)` (used by `invite.go`).
- Produces: `POST /api/org-info/name {name}` → `200 {"name"}`; `401` anonymous; `403` non-admin; `400` empty or longer than 255. Function `setOrgName(app core.App, name string) error`.

- [ ] **Step 1: Write the failing test** — create `org_name_test.go`:

```go
package coreserver

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestSetOrgName(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	if err := setOrgName(app, "  Harbor Dental  "); err != nil {
		t.Fatal(err)
	}
	if got := app.Settings().Meta.AppName; got != "Harbor Dental" {
		t.Fatalf("AppName = %q", got)
	}
	if err := setOrgName(app, "   "); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := setOrgName(app, strings.Repeat("x", 256)); err == nil {
		t.Fatal("256-char name accepted")
	}
}
```

- [ ] **Step 2: Run it and see it fail**

Run: `cd core/server && go test ./coreserver -run TestSetOrgName -v`
Expected: FAIL — undefined `setOrgName`.

- [ ] **Step 3: Implement** — create `org_name.go`:

```go
package coreserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// setOrgName renames the deployment. The name lives in Meta.AppName because
// /api/org-info already serves it before login; there is no other store.
func setOrgName(app core.App, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Name is required.")
	}
	if len(name) > 255 {
		return errors.New("Name must be 255 characters or fewer.")
	}
	app.Settings().Meta.AppName = name
	return app.Save(app.Settings())
}

// RegisterOrgNameEndpoint lets an owner or admin rename the deployment. PB's
// own /api/settings is superuser-only, and an app owner is not a superuser.
func RegisterOrgNameEndpoint(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.POST("/api/org-info/name", func(re *core.RequestEvent) error {
			if re.Auth == nil {
				return re.UnauthorizedError("Sign in to rename the workspace.", nil)
			}
			if !isOrgAdmin(re.Auth) {
				return re.ForbiddenError("Only an owner or admin can rename the workspace.", nil)
			}
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
				return re.BadRequestError("Invalid request body.", nil)
			}
			if err := setOrgName(re.App, body.Name); err != nil {
				return re.BadRequestError(err.Error(), nil)
			}
			return re.JSON(http.StatusOK, map[string]string{"name": re.App.Settings().Meta.AppName})
		})
		return e.Next()
	})
}
```

In `RegisterSharedCore` add `RegisterOrgNameEndpoint(app)` next to `RegisterOrgInfoEndpoint(app)`.

- [ ] **Step 4: Run the test and see it pass**

Run: `cd core/server && go test ./coreserver -run TestSetOrgName -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/server/coreserver/org_name.go core/server/coreserver/org_name_test.go core/server/coreserver/server.go
git commit -m "feat(org): let an owner or admin rename the workspace"
```

---

### Task 6: `setupSteps` manifest field and core slots in the generator

**Files:**
- Modify: `core/lib/packages/types.ts` (`PackageManifest`), `scripts/load-manifest.ts:5-30`
- Create: `core/lib/setup/core-slots.ts`, `core/lib/setup/order.ts`
- Modify: `scripts/describe-packages.ts` (`manifestToConfigPkg`, `validateSidebarContributions`)
- Modify: `scripts/gen-config.ts` (`ConfigPkg`, `validateConfigPkg`, `buildConfigSource`, `needsLazy`)
- Modify: `core/lib/packages/config-types.ts` (`PackageEntry`, `definePackageEntry`)
- Test: `scripts/__tests__/describe-packages.test.ts`, `scripts/__tests__/gen-config.test.ts`, `core/lib/setup/__tests__/order.test.ts`

**Interfaces:**
- Produces:
  - Manifest: `setupSteps?: { id: string; label: string; module: string; order?: string }[]`
  - `core/lib/setup/core-slots.ts`: `export const CORE_SLOT_TARGET = 'core'`, `export const CORE_SLOTS = ['setup-team'] as const`
  - `core/lib/setup/order.ts`: `isValidOrderKey(key: string): boolean`, `compareStepOrder(a: { id: string; order: string | null }, b: same): number`
  - `ConfigSetupStep { id: string; label: string; module: string; order: string | null }` on `ConfigPkg.setupSteps`
  - Generated entry: `setupSteps: [{ id, label, order, load: () => import('<pkg>/<module>') }]`
  - `PackageSetupStep { id: string; label: string; order: string | null; load: () => Promise<unknown> }` in `config-types.ts`

- [ ] **Step 1: Write the failing order tests** — create `core/lib/setup/__tests__/order.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { compareStepOrder, isValidOrderKey } from '../order'

describe('isValidOrderKey', () => {
    it('accepts fractional-indexing keys', () => {
        for (const key of ['a0', 'a1', 'a0V', 'a0k', 'Zz']) expect(isValidOrderKey(key)).toBe(true)
    })
    it('rejects malformed keys', () => {
        for (const key of ['', 'a', 'a00', '10']) expect(isValidOrderKey(key)).toBe(false)
    })
})

describe('compareStepOrder', () => {
    const sort = (steps: { id: string; order: string | null }[]) =>
        [...steps].sort(compareStepOrder).map(s => s.id)

    it('sorts by plain string comparison, not locale', () => {
        expect(
            sort([
                { id: 'core:apps', order: 'a1' },
                { id: 'widgets:plan', order: 'Zz' },
                { id: 'widgets:domain', order: 'a0k' },
                { id: 'core:workspace', order: 'a0' },
                { id: 'widgets:web', order: 'a0V' },
            ])
        ).toEqual(['widgets:plan', 'core:workspace', 'widgets:web', 'widgets:domain', 'core:apps'])
    })
    it('breaks ties by id and puts unkeyed steps last', () => {
        expect(
            sort([
                { id: 'b:x', order: null },
                { id: 'z:y', order: 'a0' },
                { id: 'a:y', order: 'a0' },
                { id: 'a:x', order: null },
            ])
        ).toEqual(['a:y', 'z:y', 'a:x', 'b:x'])
    })
})
```

- [ ] **Step 2: Run and see it fail**

Run: `pnpm exec vitest run core/lib/setup/__tests__/order.test.ts`
Expected: FAIL — cannot resolve `../order`.

- [ ] **Step 3: Implement `order.ts` and `core-slots.ts`**

`core/lib/setup/order.ts`:

```ts
import { generateKeyBetween } from 'fractional-indexing'

/**
 * Setup steps use a fractional-indexing rank scheme, so a package can place a step between
 * two others without renumbering anything. A key is valid when the library
 * accepts it as a lower bound.
 */
export function isValidOrderKey(key: string): boolean {
    try {
        generateKeyBetween(key, null)
        return true
    } catch {
        return false
    }
}

interface Ordered {
    id: string
    order: string | null
}

// Plain < on purpose: fractional-indexing keys are ordered by code unit, and
// localeCompare would put 'Zz' after 'a0'.
function compareText(a: string, b: string): number {
    if (a < b) return -1
    if (a > b) return 1
    return 0
}

export function compareStepOrder(a: Ordered, b: Ordered): number {
    if (a.order !== b.order) {
        if (a.order === null) return 1
        if (b.order === null) return -1
        return compareText(a.order, b.order)
    }
    return compareText(a.id, b.id)
}
```

`core/lib/setup/core-slots.ts`:

```ts
/**
 * Slots core renders for packages. Packages target them from
 * `sidebarContributions` with `target: 'core'`. The generator reads this list
 * to validate those contributions, so a typo fails the build instead of
 * rendering nothing.
 */
export const CORE_SLOT_TARGET = 'core'
export const CORE_SLOTS = ['setup-team'] as const
```

Add `fractional-indexing` to core's peer dependencies if `core/package.json` does not list it: check `grep fractional core/package.json core/package-versions.json`. If absent, add the range `">=3.2.0 <4"` to `core/package.json` `peerDependencies`; it is already installed in the workspace.

Run: `pnpm exec vitest run core/lib/setup/__tests__/order.test.ts`
Expected: PASS.

- [ ] **Step 4: Write failing generator tests** — append to `scripts/__tests__/describe-packages.test.ts`:

```ts
describe('setupSteps', () => {
    const base = { name: 'Acme', slug: 'acme', version: '1.0.0', description: 'x' }

    it('carries setup steps with their order', () => {
        const pkg = manifestToConfigPkg('@acme/acme', {
            ...base,
            setupSteps: [{ id: 'plan', label: 'Plan', module: 'setup/plan', order: 'Zz' }],
        })
        expect(pkg.setupSteps).toEqual([{ id: 'plan', label: 'Plan', module: 'setup/plan', order: 'Zz' }])
    })

    it('rejects an invalid order key', () => {
        expect(() =>
            manifestToConfigPkg('@acme/acme', {
                ...base,
                setupSteps: [{ id: 'plan', label: 'Plan', module: 'setup/plan', order: '10' }],
            })
        ).toThrow(/order '10'/)
    })

    it('rejects a step id outside [a-z0-9-]', () => {
        expect(() =>
            manifestToConfigPkg('@acme/acme', {
                ...base,
                setupSteps: [{ id: 'Plan.x', label: 'Plan', module: 'setup/plan' }],
            })
        ).toThrow(/id 'Plan.x'/)
    })
})

describe('core slot contributions', () => {
    const pkg = (slot: string) =>
        manifestToConfigPkg('@acme/acme', {
            name: 'Acme',
            slug: 'acme',
            version: '1.0.0',
            description: 'x',
            sidebarContributions: [{ target: 'core', slot, component: 'setup/seats' }],
        })

    it('accepts a slot core declares', () => {
        expect(() => validateSidebarContributions([pkg('setup-team')])).not.toThrow()
    })
    it('rejects an unknown core slot', () => {
        expect(() => validateSidebarContributions([pkg('nope')])).toThrow(/unknown slot 'core:nope'/)
    })
})
```

Append to `scripts/__tests__/gen-config.test.ts` (use the file's existing `ConfigPkg` fixture builder; if it has none, build one inline with every required field set to its empty value):

```ts
it('emits setup steps as load thunks', () => {
    const src = buildConfigSource([
        {
            ...emptyPkg('@acme/acme', 'acme'),
            setupSteps: [{ id: 'plan', label: 'Plan', module: 'setup/plan', order: 'Zz' }],
        },
    ])
    expect(src).toContain(
        `setupSteps: [\n            { id: "plan", label: "Plan", order: "Zz", load: () => import('@acme/acme/setup/plan') },`
    )
})
```

- [ ] **Step 5: Run and see them fail**

Run: `pnpm exec vitest run scripts/__tests__/describe-packages.test.ts scripts/__tests__/gen-config.test.ts`
Expected: FAIL — `setupSteps` missing.

- [ ] **Step 6: Implement the generator changes**

`scripts/load-manifest.ts` and `core/lib/packages/types.ts` — add to `PackageManifest`:

```ts
    /**
     * Steps this package adds to the first-run setup wizard. `module` is a
     * package-exports subpath whose module default-exports the step component
     * and may export `useIsStepDone` / `useIsStepVisible` (see
     * core/lib/setup/types.ts). `order` is a fractional-indexing key; core's
     * steps are a0 (workspace), a1 (apps), a2 (email), a3 (team).
     */
    setupSteps?: { id: string; label: string; module: string; order?: string }[]
```

`scripts/gen-config.ts` — add:

```ts
export interface ConfigSetupStep {
    id: string
    label: string
    module: string // package-exports subpath e.g. 'setup/plan'
    order: string | null
}
```

add `setupSteps: ConfigSetupStep[]` to `ConfigPkg`; in `validateConfigPkg` add `for (const s of p.setupSteps) assertSafeImportField('setupSteps[].module', s.module)`; add a line helper and emit it in `buildConfigSource` after `pushEventSourceLines(lines, p)`:

```ts
// Bare load thunks, not lazy(): the module exports hooks alongside the
// component, and the wizard needs them before it renders any step.
function pushSetupStepLines(lines: string[], p: ConfigPkg): void {
    if (p.setupSteps.length === 0) return
    lines.push('        setupSteps: [')
    for (const s of p.setupSteps) {
        lines.push(
            `            { id: ${jsonLiteral(s.id)}, label: ${jsonLiteral(s.label)}, order: ${jsonLiteral(s.order)}, load: () => import('${p.packageName}/${s.module}') },`
        )
    }
    lines.push('        ],')
}
```

`scripts/describe-packages.ts` — in `manifestToConfigPkg` build and validate:

```ts
import { CORE_SLOT_TARGET, CORE_SLOTS } from '../core/lib/setup/core-slots'
import { isValidOrderKey } from '../core/lib/setup/order'

function toSetupSteps(manifest: PackageManifest): ConfigSetupStep[] {
    return (manifest.setupSteps ?? []).map(s => {
        if (!/^[a-z0-9-]+$/.test(s.id)) {
            throw new Error(
                `[generate] ${manifest.slug}: setupStep id '${s.id}' is invalid — ids appear in URLs and must match [a-z0-9-]+`
            )
        }
        if (s.order !== undefined && !isValidOrderKey(s.order)) {
            throw new Error(
                `[generate] ${manifest.slug}: setupStep '${s.id}' has order '${s.order}', which is not a fractional-indexing key (core uses a0–a3)`
            )
        }
        return { id: s.id, label: s.label, module: s.module, order: s.order ?? null }
    })
}
```

return `setupSteps: toSetupSteps(manifest)` from `manifestToConfigPkg`. In `validateSidebarContributions`, seed core's slots before the package loop:

```ts
    slotsByTarget.set(CORE_SLOT_TARGET, new Set(CORE_SLOTS))
```

`core/lib/packages/config-types.ts` — add:

```ts
// A first-run wizard step. `load` is a bare thunk (not React.lazy) because
// the module exports hooks as well as the component — see
// core/lib/setup/types.ts for the contract.
export interface PackageSetupStep {
    id: string
    label: string
    order: string | null
    load: () => Promise<unknown>
}
```

add `setupSteps?: PackageSetupStep[]` to `PackageEntry` and `setupSteps?: PackageEntry<S, R>['setupSteps']` to the `definePackageEntry` parameter.

- [ ] **Step 7: Run tests, regenerate, typecheck**

Run: `pnpm exec vitest run scripts/__tests__ core/lib/setup && pnpm run packages:generate && pnpm exec tinycld-pkg typecheck`
Expected: PASS; generation succeeds with no setupSteps anywhere yet.

- [ ] **Step 8: Commit**

```bash
git add core/lib/packages/types.ts core/lib/packages/config-types.ts core/lib/setup/order.ts \
  core/lib/setup/core-slots.ts core/lib/setup/__tests__/order.test.ts scripts/load-manifest.ts \
  scripts/describe-packages.ts scripts/gen-config.ts scripts/__tests__/describe-packages.test.ts \
  scripts/__tests__/gen-config.test.ts core/package.json
git commit -m "feat(packages): setupSteps manifest field and core slot targets"
```

---

### Task 7: Wizard logic, state and step registry

**Files:**
- Create: `core/lib/setup/types.ts`, `core/lib/setup/wizard-logic.ts`, `core/lib/setup/registry.ts`, `core/lib/setup/use-setup-wizard-state.ts`, `core/lib/setup/use-setup-steps.ts`, `core/lib/setup/use-needs-setup.ts`
- Test: `core/lib/setup/__tests__/wizard-logic.test.ts`, `core/lib/setup/__tests__/registry.test.ts`

**Interfaces:**
- Consumes: `compareStepOrder` (Task 6), `PackageSetupStep` (Task 6), `useSystemSettings` (`core/components/setup/system-settings-store.ts`), `tinycldConfig`.
- Produces (`types.ts`):

```ts
import type { ComponentType } from 'react'

export interface SetupStepProps {
    /** Call after the step's own save succeeds; the shell moves on. */
    next: () => void
}

/** What a step module exports. Hooks are optional; the registry fills defaults. */
export interface SetupStepModule {
    default: ComponentType<SetupStepProps>
    /** Derived from real data. undefined while loading. Absent: done once acknowledged. */
    useIsStepDone?: () => boolean | undefined
    useIsStepVisible?: () => boolean
}

export interface SetupStepEntry {
    id: string // `<slug>:<id>`
    label: string
    order: string | null
    load: () => Promise<SetupStepModule>
}

export interface LoadedSetupStep {
    id: string
    label: string
    Component: ComponentType<SetupStepProps>
    /** null: the step has no derived state; `acknowledged` decides. */
    useIsStepDone: () => boolean | undefined | null
    useIsStepVisible: () => boolean
}

export interface StepStatus {
    id: string
    label: string
    isVisible: boolean
    /** true / false; undefined while its data loads; null when the step has no derived state. */
    isDone: boolean | undefined | null
}

export interface WizardState {
    startedAt: string
    acknowledged: string[]
    skipped: string[]
    dismissedAt?: string
    completedAt?: string
}
```

- Produces (`wizard-logic.ts`): `parseWizardState(raw: string | undefined): WizardState | null`, `stepIsDone(status, state): boolean | undefined`, `summarizeWizard(statuses: StepStatus[], state: WizardState): WizardSummary`, `shouldOpenWizard(input: { role: string | null; state: WizardState | null; isSettled: boolean }): boolean`, where

```ts
export interface WizardSummary {
    steps: { id: string; label: string; phase: 'done' | 'skipped' | 'todo' }[]
    /** First visible step that is not done and not skipped; null: open Done. */
    nextStepId: string | null
    doneCount: number
    total: number
    /** false while any visible step's status is still loading. */
    isSettled: boolean
}
```

- Produces (`registry.ts`): `setupStepEntries: SetupStepEntry[]` (sorted), `stepIdToParam(id)`, `paramToStepId(param)`, `buildSetupStepEntries(core: SetupStepEntry[], pkgs: readonly { manifest: { slug: string }; setupSteps?: PackageSetupStep[] }[])`.
- Produces hooks: `useSetupWizardState(): { state: WizardState | null; isReady: boolean; update: (patch: (s: WizardState) => WizardState) => Promise<void> }`, `useSetupSteps(): { steps: LoadedSetupStep[] | undefined }`, `useNeedsSetup(): boolean | undefined`.

- [ ] **Step 1: Write the failing logic tests** — create `core/lib/setup/__tests__/wizard-logic.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { StepStatus, WizardState } from '../types'
import { parseWizardState, shouldOpenWizard, summarizeWizard } from '../wizard-logic'

const state = (patch: Partial<WizardState> = {}): WizardState => ({
    startedAt: '2026-09-25T00:00:00Z',
    acknowledged: [],
    skipped: [],
    ...patch,
})
const step = (id: string, patch: Partial<StepStatus> = {}): StepStatus => ({
    id,
    label: id,
    isVisible: true,
    isDone: false,
    ...patch,
})

describe('parseWizardState', () => {
    it('returns null for a missing or corrupt row', () => {
        expect(parseWizardState(undefined)).toBeNull()
        expect(parseWizardState('{nope')).toBeNull()
        expect(parseWizardState('{"acknowledged":[]}')).toBeNull()
    })
    it('parses a valid row', () => {
        expect(parseWizardState(JSON.stringify(state({ skipped: ['core:email'] })))?.skipped).toEqual([
            'core:email',
        ])
    })
})

describe('summarizeWizard', () => {
    it('resumes at the first visible step that is neither done nor skipped', () => {
        const summary = summarizeWizard(
            [
                step('core:workspace', { isDone: true }),
                step('core:apps', { isDone: null }),
                step('core:email', { isVisible: false }),
                step('core:team'),
            ],
            state({ skipped: ['core:apps'] })
        )
        expect(summary.nextStepId).toBe('core:team')
        expect(summary.steps.map(s => s.phase)).toEqual(['done', 'skipped', 'todo'])
        expect(summary.total).toBe(3)
        expect(summary.doneCount).toBe(1)
    })
    it('treats an acknowledged step without derived state as done', () => {
        const summary = summarizeWizard([step('core:apps', { isDone: null })], state({ acknowledged: ['core:apps'] }))
        expect(summary.nextStepId).toBeNull()
    })
    it('is not settled while a visible step is loading', () => {
        expect(summarizeWizard([step('a:b', { isDone: undefined })], state()).isSettled).toBe(false)
    })
})

describe('shouldOpenWizard', () => {
    const open = (role: string | null, s: WizardState | null) =>
        shouldOpenWizard({ role, state: s, isSettled: true })

    it('opens for owners and admins with an active row', () => {
        expect(open('owner', state())).toBe(true)
        expect(open('admin', state())).toBe(true)
    })
    it('never opens for members, guests, or without a row', () => {
        expect(open('member', state())).toBe(false)
        expect(open('guest', state())).toBe(false)
        expect(open('owner', null)).toBe(false)
    })
    it('stays closed once dismissed or completed, or before the role settles', () => {
        expect(open('owner', state({ dismissedAt: 'x' }))).toBe(false)
        expect(open('owner', state({ completedAt: 'x' }))).toBe(false)
        expect(shouldOpenWizard({ role: 'owner', state: state(), isSettled: false })).toBe(false)
    })
})
```

`isDone: null` models a step with no derived state (see `StepStatus` below).

- [ ] **Step 2: Run and see it fail**

Run: `pnpm exec vitest run core/lib/setup/__tests__/wizard-logic.test.ts`
Expected: FAIL — cannot resolve `../wizard-logic`.

- [ ] **Step 3: Implement `types.ts` (exactly as in Interfaces above) and `wizard-logic.ts`**

```ts
import { z } from 'zod'
import type { StepStatus, WizardState } from './types'

const wizardStateSchema = z.object({
    startedAt: z.string(),
    acknowledged: z.array(z.string()),
    skipped: z.array(z.string()),
    dismissedAt: z.string().optional(),
    completedAt: z.string().optional(),
})

export function parseWizardState(raw: string | undefined): WizardState | null {
    if (!raw) return null
    try {
        const parsed = wizardStateSchema.safeParse(JSON.parse(raw))
        return parsed.success ? parsed.data : null
    } catch {
        return null
    }
}

/** undefined: still loading. */
export function stepIsDone(status: StepStatus, state: WizardState): boolean | undefined {
    if (status.isDone === null) return state.acknowledged.includes(status.id)
    return status.isDone
}

export interface WizardSummary {
    steps: { id: string; label: string; phase: 'done' | 'skipped' | 'todo' }[]
    nextStepId: string | null
    doneCount: number
    total: number
    isSettled: boolean
}

function phaseOf(status: StepStatus, state: WizardState): 'done' | 'skipped' | 'todo' {
    if (stepIsDone(status, state)) return 'done'
    if (state.skipped.includes(status.id)) return 'skipped'
    return 'todo'
}

export function summarizeWizard(statuses: StepStatus[], state: WizardState): WizardSummary {
    const visible = statuses.filter(s => s.isVisible)
    const steps = visible.map(s => ({ id: s.id, label: s.label, phase: phaseOf(s, state) }))
    return {
        steps,
        nextStepId: steps.find(s => s.phase === 'todo')?.id ?? null,
        doneCount: steps.filter(s => s.phase === 'done').length,
        total: steps.length,
        isSettled: visible.every(s => stepIsDone(s, state) !== undefined),
    }
}

export function shouldOpenWizard(input: {
    role: string | null
    state: WizardState | null
    isSettled: boolean
}): boolean {
    if (!input.isSettled || !input.state) return false
    if (input.role !== 'owner' && input.role !== 'admin') return false
    return !input.state.dismissedAt && !input.state.completedAt
}
```

Run: `pnpm exec vitest run core/lib/setup/__tests__/wizard-logic.test.ts`
Expected: PASS.

- [ ] **Step 4: Write the failing registry test** — create `core/lib/setup/__tests__/registry.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { buildSetupStepEntries, paramToStepId, stepIdToParam } from '../registry'

const load = () => Promise.resolve({ default: () => null })

describe('buildSetupStepEntries', () => {
    it('merges core and package steps in order with slug-prefixed ids', () => {
        const entries = buildSetupStepEntries(
            [
                { id: 'core:workspace', label: 'Workspace', order: 'a0', load },
                { id: 'core:apps', label: 'Apps', order: 'a1', load },
            ],
            [
                { manifest: { slug: 'acme' }, setupSteps: [{ id: 'plan', label: 'Plan', order: 'Zz', load }] },
                { manifest: { slug: 'mail' }, setupSteps: [{ id: 'address', label: 'Address', order: 'a0s', load }] },
                { manifest: { slug: 'none' } },
            ]
        )
        expect(entries.map(e => e.id)).toEqual(['acme:plan', 'core:workspace', 'mail:address', 'core:apps'])
    })
})

describe('step URL params', () => {
    it('round-trips ids through a URL-safe param', () => {
        expect(stepIdToParam('widgets:web-address')).toBe('widgets.web-address')
        expect(paramToStepId('widgets.web-address')).toBe('widgets:web-address')
    })
})
```

- [ ] **Step 5: Implement `registry.ts`** (core step modules are added in Task 10; reference them now by their paths):

```ts
import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import type { PackageSetupStep } from '../packages/config-types'
import { compareStepOrder } from './order'
import type { SetupStepEntry, SetupStepModule } from './types'

type Loader = () => Promise<SetupStepModule>

// Core has no manifest, so its steps are listed here. Keys leave room on both
// sides for package steps (see docs/packages.md "setupSteps").
const CORE_STEPS: SetupStepEntry[] = [
    { id: 'core:workspace', label: 'Workspace', order: 'a0', load: () => import('../../components/setup/wizard/steps/WorkspaceStep') },
    { id: 'core:apps', label: 'Apps', order: 'a1', load: () => import('../../components/setup/wizard/steps/AppsStep') },
    { id: 'core:email', label: 'Email sending', order: 'a2', load: () => import('../../components/setup/wizard/steps/EmailStep') },
    { id: 'core:team', label: 'Your team', order: 'a3', load: () => import('../../components/setup/wizard/steps/TeamStep') },
]

export function buildSetupStepEntries(
    core: SetupStepEntry[],
    pkgs: readonly { manifest: { slug: string }; setupSteps?: PackageSetupStep[] }[]
): SetupStepEntry[] {
    const fromPackages = pkgs.flatMap(p =>
        (p.setupSteps ?? []).map(s => ({
            id: `${p.manifest.slug}:${s.id}`,
            label: s.label,
            order: s.order,
            // The generator only emits modules that follow the step contract;
            // the registry normalizes the shape when it loads them.
            load: s.load as Loader,
        }))
    )
    return [...core, ...fromPackages].sort(compareStepOrder)
}

export const setupStepEntries = buildSetupStepEntries(CORE_STEPS, tinycldConfig)

// `:` is legal in a path but reads as a scheme separator to some routers and
// link parsers; slugs and step ids never contain `.`.
export function stepIdToParam(id: string): string {
    return id.replace(':', '.')
}

export function paramToStepId(param: string): string {
    return param.replace('.', ':')
}
```

Run: `pnpm exec vitest run core/lib/setup/__tests__/registry.test.ts`
Expected: PASS. (If the vitest config does not alias `@tinycld/app-generated/tinycld-config`, mock it the way `core/lib/packages/__tests__/derive-components.test.ts` does.)

- [ ] **Step 6: Implement the hooks**

`core/lib/setup/use-setup-wizard-state.ts`:

```ts
import { useSystemSettings } from '../../components/setup/system-settings-store'
import type { WizardState } from './types'
import { parseWizardState } from './wizard-logic'

export const SETUP_WIZARD_KEY = 'setup.wizard'

/**
 * The deployment's wizard progress. system_settings is owner/admin-only, so
 * for anyone else `state` is null — which is also what "no wizard" means.
 */
export function useSetupWizardState() {
    const { byKey, upsert, isReady } = useSystemSettings()
    const state = parseWizardState(byKey.get(SETUP_WIZARD_KEY)?.value)

    const update = async (patch: (s: WizardState) => WizardState) => {
        if (!state) return
        await upsert.mutateAsync({
            key: SETUP_WIZARD_KEY,
            value: JSON.stringify(patch(state)),
            isSecret: false,
        })
    }

    return { state, isReady, update }
}
```

In `system-settings-store.ts` return `isReady` from the live query: `const { data: rows = [], isReady } = useLiveQuery(...)` and `return { byKey, upsert, isReady }`.

`core/lib/setup/use-needs-setup.ts`:

```ts
import { useQuery } from '@tanstack/react-query'
import { getResolvedAddress } from '../server-address'

export const NEEDS_SETUP_QUERY_KEY = ['setup-check'] as const

/** undefined until the server answers. A failed check reads as "set up". */
export function useNeedsSetup(): boolean | undefined {
    const { data } = useQuery({
        queryKey: NEEDS_SETUP_QUERY_KEY,
        queryFn: async () => {
            const addr = getResolvedAddress()
            if (!addr) return false
            const res = await fetch(`${addr}/api/setup/check`, { cache: 'no-store' })
            if (!res.ok) return false
            const body = (await res.json()) as { needsSetup?: boolean }
            return body.needsSetup === true
        },
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })
    return data
}
```

`core/lib/setup/use-setup-steps.ts`:

```ts
import { useQuery } from '@tanstack/react-query'
import { setupStepEntries } from './registry'
import type { LoadedSetupStep, SetupStepModule } from './types'

const useNoDerivedState = () => null
const useAlwaysVisible = () => true

function normalize(entry: (typeof setupStepEntries)[number], mod: SetupStepModule): LoadedSetupStep {
    return {
        id: entry.id,
        label: entry.label,
        Component: mod.default,
        useIsStepDone: mod.useIsStepDone ?? useNoDerivedState,
        useIsStepVisible: mod.useIsStepVisible ?? useAlwaysVisible,
    }
}

/**
 * Loads every step module once. All of them are needed before the shell can
 * render: progress and resume depend on each step's hooks.
 */
export function useSetupSteps() {
    const { data } = useQuery({
        queryKey: ['setup-step-modules'],
        queryFn: () => Promise.all(setupStepEntries.map(async e => normalize(e, await e.load()))),
        staleTime: Number.POSITIVE_INFINITY,
        gcTime: Number.POSITIVE_INFINITY,
    })
    return { steps: data }
}
```

- [ ] **Step 7: Typecheck and run tests**

Run: `pnpm exec vitest run core/lib/setup && pnpm exec tinycld-pkg typecheck`
Expected: typecheck FAILS only on the four missing step modules in `registry.ts` (they arrive in Task 10). To keep this task green, create each as a placeholder that satisfies the contract now:

```ts
// core/components/setup/wizard/steps/WorkspaceStep.tsx (same for AppsStep, EmailStep, TeamStep)
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'

export default function WorkspaceStep(_props: SetupStepProps) {
    return null
}
```

Re-run: PASS.

- [ ] **Step 8: Commit**

```bash
git add core/lib/setup core/components/setup/system-settings-store.ts core/components/setup/wizard/steps
git commit -m "feat(setup): wizard state, resume logic and step registry"
```

---

### Task 8: Wizard shell and live workspace preview

**Files:**
- Create: `core/lib/setup/setup-preview-store.ts`
- Create: `core/components/setup/wizard/use-workspace-preview.ts`, `WorkspacePreview.tsx`, `WorkspacePreviewStrip.tsx`, `ServerLogPreview.tsx`, `ProgressSegments.tsx`, `SetupWizardShell.tsx`
- Test: `core/components/setup/wizard/__tests__/use-workspace-preview.test.ts`

**Interfaces:**
- Consumes: `useOrgInfo`, `usePackages` (`core/lib/packages/use-packages`), `getIcon` (`core/components/workspace/package-icon-map.ts`), `useStore('pkg_registry')`, `useStore('users')`, `useSystemSettings`, `isDeliveryEnabled` (`core/components/setup/system-settings-logic.ts`).
- Produces:
  - `useSetupPreviewStore` with `{ draftName: string | null; setDraftName: (name: string | null) => void }`.
  - `buildPreviewModel(input: { orgName: string; draftName: string | null; logoUrl: string; apps: { slug: string; icon: string }[]; memberInitials: string[]; isMailOn: boolean }): PreviewModel` where `PreviewModel = { name: string; initial: string; logoUrl: string; apps: { slug: string; icon: string }[]; memberInitials: string[]; isMailOn: boolean; isEmpty: boolean }`.
  - `useWorkspacePreview(): PreviewModel`.
  - `<SetupWizardShell summary={WizardSummary | null} currentStepId={string | null} onFinishLater={(() => void) | null} onSkip={(() => void) | null} preview={'workspace' | 'server-log' | 'ghost'} code={string} children />`.

- [ ] **Step 1: Write the failing model test** — create `core/components/setup/wizard/__tests__/use-workspace-preview.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { buildPreviewModel } from '../use-workspace-preview'

const base = { orgName: '', draftName: null, logoUrl: '', apps: [], memberInitials: [], isMailOn: false }

describe('buildPreviewModel', () => {
    it('prefers the name being typed over the saved one', () => {
        const m = buildPreviewModel({ ...base, orgName: 'tinycld', draftName: 'Harbor Dental' })
        expect(m.name).toBe('Harbor Dental')
        expect(m.initial).toBe('H')
    })
    it('is empty with no name, apps or members', () => {
        expect(buildPreviewModel(base).isEmpty).toBe(true)
        expect(buildPreviewModel({ ...base, memberInitials: ['DR'] }).isEmpty).toBe(false)
    })
})
```

- [ ] **Step 2: Run and see it fail**

Run: `pnpm exec vitest run core/components/setup/wizard/__tests__/use-workspace-preview.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the store and the preview model**

`core/lib/setup/setup-preview-store.ts`:

```ts
import { create } from '@tinycld/core/lib/store'

interface SetupPreviewState {
    /** The workspace name while it is typed, before it is saved. */
    draftName: string | null
    setDraftName: (name: string | null) => void
}

export const useSetupPreviewStore = create<SetupPreviewState>()(set => ({
    draftName: null,
    setDraftName: draftName => set({ draftName }),
}))
```

`core/components/setup/wizard/use-workspace-preview.ts`:

```ts
import { inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useSetupPreviewStore } from '@tinycld/core/lib/setup/setup-preview-store'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { isDeliveryEnabled } from '../../setup/system-settings-logic'
import { useSystemSettings } from '../../setup/system-settings-store'

export interface PreviewModel {
    name: string
    initial: string
    logoUrl: string
    apps: { slug: string; icon: string }[]
    memberInitials: string[]
    isMailOn: boolean
    isEmpty: boolean
}

function initialsOf(name: string): string {
    return name
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map(part => part[0]?.toUpperCase() ?? '')
        .join('')
}

export function buildPreviewModel(input: {
    orgName: string
    draftName: string | null
    logoUrl: string
    apps: { slug: string; icon: string }[]
    memberInitials: string[]
    isMailOn: boolean
}): PreviewModel {
    const name = (input.draftName ?? input.orgName).trim()
    return {
        name,
        initial: name.charAt(0).toUpperCase(),
        logoUrl: input.logoUrl,
        apps: input.apps,
        memberInitials: input.memberInitials,
        isMailOn: input.isMailOn,
        isEmpty: !name && input.apps.length === 0 && input.memberInitials.length === 0,
    }
}

const MAX_AVATARS = 4

export function useWorkspacePreview(): PreviewModel {
    const { org } = useOrgInfo()
    const draftName = useSetupPreviewStore(s => s.draftName)
    const [pkgRegistry, users] = useStore('pkg_registry', 'users')
    const packages = usePackages()
    const { byKey } = useSystemSettings()

    const { data: enabled = [] } = useLiveQuery(query =>
        query
            .from({ p: pkgRegistry })
            .where(({ p }) => inArray(p.status, ['bundled', 'installed']))
            .select(({ p }) => ({ slug: p.slug }))
    )
    const { data: people = [] } = useLiveQuery(query =>
        query.from({ u: users }).select(({ u }) => ({ name: u.name }))
    )

    const enabledSlugs = new Set(enabled.map(e => e.slug))
    const apps = packages
        .filter(p => p.nav && enabledSlugs.has(p.slug))
        .map(p => ({ slug: p.slug, icon: p.nav?.icon ?? '' }))

    return buildPreviewModel({
        orgName: org?.name ?? '',
        draftName,
        logoUrl: org?.logoUrl ?? '',
        apps,
        memberInitials: people.slice(0, MAX_AVATARS).map(p => initialsOf(p.name)),
        isMailOn: isDeliveryEnabled(byKey.get('mail.delivery_enabled')?.value),
    })
}
```

Run the test: PASS.

- [ ] **Step 4: Build the preview components**

Mockup for reference: `.superpowers/brainstorm/28983-1790348859/content/steps-v2.html`. Use these tokens: rail `bg-rail-background`, rail icon `text-rail-text`, new/active icon `text-primary` on `bg-primary/20`, card `bg-background border-border`, pane `bg-surface-secondary`.

`WorkspacePreview.tsx`:

```tsx
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Image, Text, View } from 'react-native'
import type { PreviewModel } from './use-workspace-preview'

function RailApp({ icon }: { icon: string }) {
    const color = useThemeColor('primary')
    const Icon = getIcon(icon)
    return (
        <View className="size-6 items-center justify-center rounded-md bg-primary/20">
            <Icon size={13} color={color} />
        </View>
    )
}

function OrgMark({ model }: { model: PreviewModel }) {
    if (model.logoUrl) {
        return <Image source={{ uri: model.logoUrl }} className="size-7 rounded-lg" />
    }
    return (
        <View className="size-7 items-center justify-center rounded-lg bg-primary">
            <Text className="text-xs font-extrabold text-primary-foreground">{model.initial || '?'}</Text>
        </View>
    )
}

function Avatar({ initials }: { initials: string }) {
    return (
        <View className="-ml-1.5 size-5 items-center justify-center rounded-full border-2 border-background bg-muted/30">
            <Text className="text-[8px] font-bold text-foreground">{initials}</Text>
        </View>
    )
}

/** A miniature of the workspace that fills in as each step is completed. */
export function WorkspacePreview({ model, isGhost }: { model: PreviewModel; isGhost: boolean }) {
    const rail = model.apps.map(a => <RailApp key={a.slug} icon={a.icon} />)
    const avatars = model.memberInitials.map((i, n) => <Avatar key={`${i}-${n}`} initials={i} />)
    return (
        <View
            className="w-full max-w-[340px] h-[230px] flex-row overflow-hidden rounded-xl bg-background shadow-lg"
            style={{ opacity: isGhost ? 0.7 : 1 }}
            accessibilityLabel="Preview of your workspace"
        >
            <View className="w-12 items-center gap-2 bg-rail-background py-2">
                <OrgMark model={model} />
                {rail}
            </View>
            <View className="w-24 border-r border-border bg-surface-secondary p-2.5">
                <Text className="mb-2 text-[11px] font-bold text-foreground" numberOfLines={1}>
                    {model.name}
                </Text>
                <View className="mb-2 h-1.5 rounded bg-border" />
                <View className="h-1.5 w-2/3 rounded bg-border" />
            </View>
            <View className="flex-1 p-3">
                <View className="flex-row justify-end pl-1.5">{avatars}</View>
            </View>
        </View>
    )
}
```

`WorkspacePreviewStrip.tsx` (phones): a `flex-row items-center gap-2 bg-rail-background px-3 py-2.5` bar with `OrgMark`, the name in `text-rail-active-text font-bold text-xs`, a spacer, then the `RailApp` list. Export `OrgMark` and `RailApp` from `WorkspacePreview.tsx` so the strip reuses them.

`ServerLogPreview.tsx` — the code step's preview: a `bg-rail-background rounded-xl p-3.5` block in `font-mono text-[11px] text-rail-text`, showing the boxed log lines from Task 4 with the code as `bg-primary text-primary-foreground font-bold px-1 rounded` and a caption "Look for this box in the server log." below it. Takes `{ code: string }` (`'XXXX-XXXX'` when empty).

`ProgressSegments.tsx`:

```tsx
import { View } from 'react-native'
import type { WizardSummary } from '@tinycld/core/lib/setup/wizard-logic'

const PHASE_CLASS = { done: 'bg-primary', skipped: 'bg-muted/40', todo: 'bg-border' } as const

export function ProgressSegments({ summary, currentStepId }: { summary: WizardSummary; currentStepId: string | null }) {
    const segments = summary.steps.map(s => (
        <View
            key={s.id}
            accessibilityLabel={`${s.label}: ${s.phase}`}
            className={`h-1 flex-1 rounded-sm ${s.id === currentStepId ? 'bg-primary/60' : PHASE_CLASS[s.phase]}`}
        />
    ))
    return <View className="flex-1 flex-row items-center gap-1">{segments}</View>
}
```

`SetupWizardShell.tsx` — layout: top bar (progress + "Step N of M" + "Finish later"), below it a row with the form column (`w-full md:w-[52%] p-6`) and the preview pane (`hidden md:flex flex-1 items-center justify-center bg-surface-secondary border-l border-border p-5`). Below `md`, render `WorkspacePreviewStrip` above the top bar instead. Buttons use `Button`/`ButtonText` from `@tinycld/core/ui/button` (`variant="link"` for Skip and Finish later). Derive "Step N of M" and the preview element above the JSX:

```tsx
import { useSetupWizardLayout } from './use-setup-wizard-layout' // see below

export function SetupWizardShell(props: SetupWizardShellProps) {
    const layout = useSetupWizardLayout(props)
    return (
        <View className="flex-1 bg-background">
            <WorkspacePreviewStrip model={layout.model} isVisible={layout.showStrip} />
            <View className="flex-row items-center gap-3 border-b border-border px-4 py-2.5">
                <ProgressSegments summary={layout.summary} currentStepId={props.currentStepId} />
                <Text className="text-[11px] text-muted-foreground">{layout.stepLabel}</Text>
                <FinishLaterButton onPress={props.onFinishLater} />
            </View>
            <View className="flex-1 flex-row">
                <ScrollView className="w-full md:w-[52%]" contentContainerClassName="p-6 gap-4">
                    {props.children}
                    <SkipButton onPress={props.onSkip} />
                </ScrollView>
                <PreviewPane layout={layout} code={props.code} />
            </View>
        </View>
    )
}
```

`useSetupWizardLayout` lives in the same file (above the component): it calls `useWorkspacePreview()`, reads the breakpoint with `useWindowDimensions().width < 768`, builds `stepLabel` (`"<label> · N of M"`, or `"Claim this server"` when `summary` is null), and returns `{ model, summary: summary ?? EMPTY_SUMMARY, stepLabel, showStrip, preview }`. `FinishLaterButton` / `SkipButton` return `null` when `onPress` is null. `PreviewPane` switches on `preview`: `'server-log'` → `ServerLogPreview`, `'ghost'` → `WorkspacePreview isGhost`, `'workspace'` → `WorkspacePreview`.

- [ ] **Step 5: Typecheck, lint, test**

Run: `pnpm exec tinycld-pkg check`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add core/lib/setup/setup-preview-store.ts core/components/setup/wizard
git commit -m "feat(setup): wizard shell with a live workspace preview"
```

---

### Task 9: Claim screens, routes and the entry redirect

**Files:**
- Create: `core/components/setup/wizard/CodeInput.tsx`, `ClaimServerStep.tsx`, `CreateOwnerStep.tsx`, `PreAuthSetup.tsx`, `SetupStepScreen.tsx`, `use-setup-wizard.ts`
- Delete: `app/a/setup.tsx`, `core/components/setup/SetupWizard.tsx`
- Create: `app/a/setup/index.tsx`, `app/a/setup/recovery.tsx`, `app/a/setup/[step].tsx`
- Modify: `core/components/setup/SetupPage.tsx` (recovery only), `core/components/workspace/AuthGate.tsx`, `app/a/(app)/_layout.tsx`
- Test: `core/components/setup/wizard/__tests__/claim-errors.test.ts`

**Interfaces:**
- Consumes: `useNeedsSetup`, `useSetupWizardState`, `useSetupSteps`, `summarizeWizard`, `shouldOpenWizard`, `stepIdToParam`, `paramToStepId`, `SetupWizardShell`, `useCurrentRole`, `appHref`, `pb` (`core/lib/pocketbase`), `PB_SERVER_ADDR`.
- Produces:
  - `claimErrorMessage(body: { error?: string; reason?: string } | null): string`
  - `useSetupWizard(): { steps: LoadedSetupStep[] | undefined; state: WizardState | null; statuses; summary: WizardSummary | null; skip(id), acknowledge(id), finishLater(), complete() }`
  - `useShouldOpenSetupWizard(): boolean`
  - Routes: `/a/setup` (claim), `/a/setup/recovery`, `/a/setup/next`, `/a/setup/done`, `/a/setup/<slug>.<id>`.

- [ ] **Step 1: Write the failing error-copy test** — `core/components/setup/wizard/__tests__/claim-errors.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { claimErrorMessage } from '../ClaimServerStep'

describe('claimErrorMessage', () => {
    it('uses the server message', () => {
        expect(claimErrorMessage({ error: 'Too many tries. Wait 10 minutes, or restart the server for a new code.', reason: 'locked' })).toMatch(/Too many tries/)
    })
    it('explains a network failure without apologizing', () => {
        expect(claimErrorMessage(null)).toBe('The server did not answer. Check that it is running, then try again.')
    })
})
```

Run: `pnpm exec vitest run core/components/setup/wizard/__tests__/claim-errors.test.ts` → FAIL.

- [ ] **Step 2: Build `CodeInput`**

One `TextInput` so paste and autofill work on both platforms; the eight cells are drawn behind it. The input is transparent text over the cells.

```tsx
import { useRef } from 'react'
import { Pressable, TextInput, Text, View } from 'react-native'

const CELLS = 8

function cellsOf(value: string): string[] {
    const chars = value.replace(/[^A-Z0-9]/gi, '').toUpperCase().slice(0, CELLS).split('')
    return Array.from({ length: CELLS }, (_, i) => chars[i] ?? '')
}

function Cell({ char, isFilled }: { char: string; isFilled: boolean }) {
    return (
        <View className={`h-11 w-9 items-center justify-center rounded-lg border-[1.5px] ${isFilled ? 'border-primary bg-primary/10' : 'border-border'}`}>
            <Text className="font-mono text-xl font-bold text-foreground">{char}</Text>
        </View>
    )
}

export function CodeInput({ value, onChangeText }: { value: string; onChangeText: (v: string) => void }) {
    const ref = useRef<TextInput>(null)
    const cells = cellsOf(value).map((c, i) => <Cell key={i} char={c} isFilled={c !== ''} />)
    const first = cells.slice(0, 4)
    const second = cells.slice(4)
    return (
        <Pressable onPress={() => ref.current?.focus()} className="relative flex-row items-center gap-1.5">
            {first}
            <Text className="text-lg text-muted-foreground">–</Text>
            {second}
            <TextInput
                ref={ref}
                testID="setup-code"
                accessibilityLabel="Setup code"
                value={value}
                onChangeText={onChangeText}
                autoCapitalize="characters"
                autoCorrect={false}
                autoComplete="one-time-code"
                textContentType="oneTimeCode"
                maxLength={9}
                className="absolute inset-0 opacity-0"
            />
        </Pressable>
    )
}
```

- [ ] **Step 3: Build `ClaimServerStep` and `CreateOwnerStep`**

`ClaimServerStep.tsx` — form with `useForm` + zod (`code: z.string().regex(/^[A-Z0-9]{4}-?[A-Z0-9]{4}$/i, 'Enter the 8-character code')`), a `Controller` around `CodeInput`, and a `useMutation` that POSTs to `/api/setup/verify`. On success call `onVerified(code)`. On failure set a form error with `claimErrorMessage(body)`. Export:

```ts
export function claimErrorMessage(body: { error?: string; reason?: string } | null): string {
    if (!body) return 'The server did not answer. Check that it is running, then try again.'
    return body.error ?? 'That code does not match. Check the server log for the latest code.'
}
```

Copy: heading "Claim this server"; intro "Enter the setup code from the server log. Only someone with access to the server can see it."; button "Continue"; help line from Global Constraints.

`CreateOwnerStep.tsx` — fields `name` (required), `email`, `password` (min 10), `confirmPassword`, and `appUrl` inside a collapsed "Advanced" disclosure (a local `useState` toggle is fine — genuinely local UI state). Default `appUrl` is the same expression the old `SetupWizard.tsx` used (`Platform.OS === 'web' ? window.location.origin : (getResolvedAddress() ?? '')`) — keep its comment. The mutation POSTs `{ code, name, email, password, appUrl }` to `/api/setup/init`; on success it saves the auth exactly as `SetupWizard.tsx:84-89` did, invalidates `NEEDS_SETUP_QUERY_KEY`, and calls `router.replace(appHref('setup/next'))`. Errors use `handleMutationErrorsWithForm`; a 403 body shows `claimErrorMessage(body)` and sends the person back to the code screen (`onCodeRejected()`). Heading "Create your owner account"; intro "You manage this server and everyone on it."; button "Create account". The field `onChangeText` for `name` also calls the `onNameChange(initials)` prop, so `PreAuthSetup` can show the owner's avatar on the ghost preview. Add `ghostInitials?: string` to `SetupWizardShellProps`; when `preview === 'ghost'`, `PreviewPane` passes `memberInitials: ghostInitials ? [ghostInitials] : []` into `buildPreviewModel` instead of the live users.

`PreAuthSetup.tsx`:

```tsx
import { useState } from 'react'
import { SetupWizardShell } from './SetupWizardShell'
import { ClaimServerStep } from './ClaimServerStep'
import { CreateOwnerStep } from './CreateOwnerStep'

/** The two screens before an owner exists: prove access, then create the owner. */
export function PreAuthSetup({ initialCode }: { initialCode: string | undefined }) {
    const [verifiedCode, setVerifiedCode] = useState<string | null>(null)
    const [ghostInitials, setGhostInitials] = useState('')
    if (verifiedCode === null) {
        return (
            <SetupWizardShell summary={null} currentStepId={null} onFinishLater={null} onSkip={null} preview="server-log" code={initialCode ?? ''}>
                <ClaimServerStep initialCode={initialCode} onVerified={setVerifiedCode} />
            </SetupWizardShell>
        )
    }
    return (
        <SetupWizardShell summary={null} currentStepId={null} onFinishLater={null} onSkip={null} preview="ghost" code="" ghostInitials={ghostInitials}>
            <CreateOwnerStep code={verifiedCode} onNameChange={setGhostInitials} onCodeRejected={() => setVerifiedCode(null)} />
        </SetupWizardShell>
    )
}
```

`ClaimServerStep` with an `initialCode` verifies it once on mount through the mutation's `mutate` called from a `useRef`-guarded first render (not `useEffect`+state): `const autoTried = useRef(false); if (initialCode && !autoTried.current) { autoTried.current = true; verify.mutate(initialCode) }`.

- [ ] **Step 4: Build `useSetupWizard` and `SetupStepScreen`**

`use-setup-wizard.ts` resolves statuses with a status chain so every step's hooks run unconditionally at the top level of their own component:

```tsx
import type { ReactNode } from 'react'
import type { LoadedSetupStep, StepStatus } from '@tinycld/core/lib/setup/types'

/**
 * Each step's hooks live in its own module, so they cannot be called in a
 * loop in one component. The chain renders one tiny component per step; each
 * calls its step's hooks at top level and passes the growing status list on.
 * No effects, no state: statuses are recomputed on every render.
 */
export function StepStatusChain({ steps, index = 0, acc = [], children }: {
    steps: LoadedSetupStep[]
    index?: number
    acc?: StepStatus[]
    children: (statuses: StepStatus[]) => ReactNode
}) {
    if (index === steps.length) return <>{children(acc)}</>
    return <StepStatusLink step={steps[index]} steps={steps} index={index} acc={acc}>{children}</StepStatusLink>
}

function StepStatusLink({ step, steps, index, acc, children }: {
    step: LoadedSetupStep
    steps: LoadedSetupStep[]
    index: number
    acc: StepStatus[]
    children: (statuses: StepStatus[]) => ReactNode
}) {
    const isDone = step.useIsStepDone()
    const isVisible = step.useIsStepVisible()
    const next = [...acc, { id: step.id, label: step.label, isDone, isVisible }]
    return <StepStatusChain steps={steps} index={index + 1} acc={next}>{children}</StepStatusChain>
}
```

(Name the file `use-setup-wizard.tsx` since it contains JSX.) Also export the actions hook:

```ts
export function useWizardActions() {
    const { state, update } = useSetupWizardState()
    const now = () => new Date().toISOString()
    const addTo = (list: string[], id: string) => (list.includes(id) ? list : [...list, id])
    return {
        state,
        skip: (id: string) => update(s => ({ ...s, skipped: addTo(s.skipped, id) })),
        acknowledge: (id: string) =>
            update(s => ({ ...s, acknowledged: addTo(s.acknowledged, id), skipped: s.skipped.filter(x => x !== id) })),
        finishLater: () => update(s => ({ ...s, dismissedAt: now() })),
        complete: () => update(s => ({ ...s, completedAt: now() })),
    }
}
```

`SetupStepScreen.tsx` — props `{ param: string }`. Inside `StepStatusChain`, compute `summary = summarizeWizard(statuses, state)`. Resolve the target:
- `param === 'next'` → `<Redirect href={appHref(summary.nextStepId ? `setup/${stepIdToParam(summary.nextStepId)}` : 'setup/done')} />` once `summary.isSettled`.
- `param === 'done'` → render `DoneStep` (Task 10) in the shell with `preview="workspace"`.
- otherwise render the step's `Component` with `next = async () => { await acknowledge(id); router.replace(appHref('setup/next')) }`, `onSkip = async () => { await skip(id); router.replace(appHref('setup/next')) }`, `onFinishLater = async () => { await finishLater(); router.replace(appHref('')) }`.
- An unknown id, or a step that is not visible → redirect to `setup/next`.
- Before steps load or `state` is ready → render nothing (the shell skeleton is not needed; this is sub-second).
Put all of this in a `useStepScreen(param, statuses)` helper above the JSX so the component body only picks between `<Redirect>`, the shell, and `null`.

- [ ] **Step 5: Routes**

Delete `app/a/setup.tsx`. Create:

`app/a/setup/index.tsx`:

```tsx
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { PreAuthSetup } from '@tinycld/core/components/setup/wizard/PreAuthSetup'
import { appHref } from '@tinycld/core/lib/org-routes'
import { useNeedsSetup } from '@tinycld/core/lib/setup/use-needs-setup'
import { Redirect, useLocalSearchParams } from 'expo-router'

// First-run entry. Pre-auth by design (outside app/(app)/): nobody can sign in
// until the owner this screen creates exists. Once the server is claimed, the
// signed-in wizard lives at /a/setup/<step> and recovery at /a/setup/recovery.
export default function SetupIndex() {
    const { code } = useLocalSearchParams<{ code?: string }>()
    const needsSetup = useNeedsSetup()
    if (needsSetup === undefined) return null
    if (!needsSetup) return <Redirect href={appHref('setup/next')} />
    return (
        <>
            <DocumentTitle title="Set up" includeOrg={false} />
            <PreAuthSetup initialCode={code} />
        </>
    )
}
```

`app/a/setup/recovery.tsx` — renders `SetupPage` (recovery only) with `DocumentTitle title="Recovery"`.

`app/a/setup/[step].tsx` — requires a session: `useAuth({ throwIfAnon: false })`; while initializing return `null`; not logged in → `<AuthGate />`; role settled and not owner/admin → `<Redirect href={appHref('')} />`; else `<SetupStepScreen param={step} />`.

`SetupPage.tsx` — remove the `token` prop, the `needsSetup`/`SetupWizard` branches and the "Setup Required" text; if `useNeedsSetup()` is true, redirect to `appHref('setup')`. Keep the admin → settings redirect and the superuser recovery console.

`AuthGate.tsx` — before rendering `LoginModal`:

```tsx
    const needsSetup = useNeedsSetup()
    if (needsSetup) return <Redirect href={appHref('setup')} />
```

(Keep the existing `setPendingRoute` effect above it; hooks stay unconditional.)

`app/a/(app)/_layout.tsx` — after `isReady`, redirect owners/admins with an active wizard:

```tsx
    const shouldOpenWizard = useShouldOpenSetupWizard()
    ...
    if (shouldOpenWizard) return <Redirect href={appHref('setup/next')} />
```

with, in `use-setup-wizard.tsx`:

```ts
export function useShouldOpenSetupWizard(): boolean {
    const { role, isReady: roleReady } = useCurrentRole()
    const { state, isReady } = useSetupWizardState()
    return shouldOpenWizard({ role, state, isSettled: roleReady && isReady })
}
```

Update every reference to `?token=` / `SetupWizard`: `grep -rn "setup?token\|SetupWizard\b\|token={token}" app core tests docs --include='*.ts' --include='*.tsx' --include='*.md'`.

- [ ] **Step 6: Check**

Run: `pnpm exec vitest run core/components/setup && pnpm exec tinycld-pkg check && pnpm run check:core-isolation`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add -A app/a/setup app/a/setup.tsx "app/a/(app)/_layout.tsx" core/components/setup core/components/workspace/AuthGate.tsx
git commit -m "feat(setup): claim screens, wizard routes and entry redirect"
```

---

### Task 10: Core steps, the invite slot and Done

**Files:**
- Modify (replace placeholders): `core/components/setup/wizard/steps/WorkspaceStep.tsx`, `AppsStep.tsx`, `EmailStep.tsx`, `TeamStep.tsx`
- Create: `core/components/setup/wizard/steps/DoneStep.tsx`
- Create: `core/components/settings/members/use-invite-member.ts`; modify `MembersDrawer.tsx:449-500` to use it
- Test: `core/components/setup/wizard/steps/__tests__/step-done.test.ts`

**Interfaces:**
- Consumes: `SetupStepProps`, `useSetupPreviewStore`, `OrgBrandingSection`, `MailSendingPanel`, `packageSystemSettings` (`core/lib/packages/derive-components.ts`), `useIsSettingManaged`, `enabledStatusFor`, `SidebarSlot`, `useCurrentRole`.
- Produces per step module: `default`, and where noted `useIsStepDone` / `useIsStepVisible`. Pure helpers for tests: `workspaceIsDone(name: string): boolean`, `teamIsDone(userCount: number): boolean`, `emailIsDone(value: string | undefined): boolean`.
- Produces `useInviteMember({ setError, getValues, onInvited })` returning the mutation, plus `inviteSchema`.

- [ ] **Step 1: Write the failing "done" tests** — `core/components/setup/wizard/steps/__tests__/step-done.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { emailIsDone } from '../EmailStep'
import { teamIsDone } from '../TeamStep'
import { workspaceIsDone } from '../WorkspaceStep'

describe('derived step state', () => {
    it('workspace is done once it has a name', () => {
        expect(workspaceIsDone('')).toBe(false)
        expect(workspaceIsDone('  ')).toBe(false)
        expect(workspaceIsDone('Harbor Dental')).toBe(true)
    })
    it('team is done once anyone besides the owner exists', () => {
        expect(teamIsDone(1)).toBe(false)
        expect(teamIsDone(2)).toBe(true)
    })
    it('email is done once delivery is on', () => {
        expect(emailIsDone(undefined)).toBe(false)
        expect(emailIsDone('true')).toBe(true)
    })
})
```

Run: `pnpm exec vitest run core/components/setup/wizard/steps/__tests__/step-done.test.ts` → FAIL.

- [ ] **Step 2: WorkspaceStep**

- Form: `name` (`z.string().trim().min(1, 'Enter a name').max(255)`), default `org?.name ?? ''`. `onChangeText` also calls `useSetupPreviewStore.getState().setDraftName(value)` so the preview updates while typing (wrap the `TextInput` in a `Controller` render that calls both).
- Save: `useMutation` → `pb.send('/api/org-info/name', { method: 'POST', body: { name } })`, then invalidate `ORG_INFO_QUERY_KEY`, clear the draft name, call `next()`.
- Below the form render `<OrgBrandingSection />` for the logo.
- Heading "Your workspace"; intro "People see this name and logo when they sign in and in invite emails."; button "Continue".
- Exports:

```ts
export function workspaceIsDone(name: string): boolean {
    return name.trim() !== ''
}
export function useIsStepDone() {
    const { org, isPending } = useOrgInfo()
    return isPending ? undefined : workspaceIsDone(org?.name ?? '')
}
```

(If `useOrgInfo` does not return `isPending`, return it from the hook: it already reads `isPending` from `useQuery`.)

- [ ] **Step 3: AppsStep**

- Live query `pkg_registry` where `status` in `['bundled', 'disabled']` and slug ≠ `core`, joined in `.select()` with the static manifest name/icon from `usePackages()`. Only rows that are bundled in this build appear: filter to slugs present in `usePackages()` (the static registry is exactly the bundled set).
- Each row is a `Pressable` card with `testID={`setup-app-${slug}`}` and a checkbox look (`border-primary bg-primary/10` when on). Pressing calls a `useMutation` that updates `status` to `enabledStatusFor(!isOn)`. Changes apply immediately; the preview rail updates from the same live data.
- Copy: heading "Choose your apps"; intro = Apps intro from Global Constraints; button "Continue" → `next()`.
- Exports: `useIsStepVisible = () => useCurrentRole().isOwner` (package management is owner-only; `pkg_registry` writes are owner-only). No `useIsStepDone` (acknowledged).

- [ ] **Step 4: EmailStep**

- Render `<MailSendingPanel />`, then every system settings panel whose `keyPrefix` starts with `mail.` (from `packageSystemSettings`, flattened in a helper above the JSX, rendered as `<Panel.Component />` inside `Suspense`). This lets a mail package's provider panel appear without core naming it.
- Button "Continue" → `next()` (the panels save themselves).
- Exports:

```ts
export function emailIsDone(value: string | undefined): boolean {
    return isDeliveryEnabled(value)
}
export function useIsStepDone() {
    const { byKey, isReady } = useSystemSettings()
    return isReady ? emailIsDone(byKey.get('mail.delivery_enabled')?.value) : undefined
}
export function useIsStepVisible() {
    return !useIsSettingManaged('mail.')
}
```

- [ ] **Step 5: Extract the invite mutation and build TeamStep**

Move `inviteSchema`, `InviteFormValues` and the `invite` `useMutation` out of `MembersDrawer.tsx` into `use-invite-member.ts`:

```ts
export function useInviteMember(opts: {
    setError: UseFormSetError<InviteFormValues>
    getValues: UseFormGetValues<InviteFormValues>
    onInvited: (result: { userId: string; inviteUrl: string }) => void
}) {
    return useMutation({
        mutationFn: async (data: InviteFormValues) =>
            pb.send<{ userId: string; inviteUrl: string }>('/api/invite-member', {
                method: 'POST',
                body: JSON.stringify({
                    username: data.username.trim().toLowerCase(),
                    email: data.email?.trim() ?? '',
                    role: data.role,
                }),
                headers: { 'Content-Type': 'application/json' },
            }),
        onSuccess: opts.onInvited,
        onError: handleMutationErrorsWithForm({ setError: opts.setError, getValues: opts.getValues }),
    })
}
```

Keep the existing "Single-org: …userId" comment on it. `MembersDrawer` calls `useInviteMember({ setError, getValues, onInvited: setResult })`.

TeamStep:
- `<SidebarSlot target={CORE_SLOT_TARGET} slot="setup-team" />` at the top (a package such as a seat meter renders here).
- An invite form (username, email, role select limited to `member`/`admin`), button "Send invite". On success, reset the form and show the invite link with the existing `InviteLinkSuccessView` pattern inline (reuse `InviteLinkPanel`).
- A list of people already in the workspace (live query on `users`, name + role).
- A refused invite (e.g. a seat limit enforced by the server) shows on the form through `handleMutationErrorsWithForm`; nothing else is blocked.
- Button "Continue" → `next()`.
- Exports:

```ts
export function teamIsDone(userCount: number): boolean {
    return userCount > 1
}
export function useIsStepDone() {
    const [users] = useStore('users')
    const { data, isReady } = useLiveQuery(query => query.from({ u: users }).select(({ u }) => ({ id: u.id })))
    return isReady ? teamIsDone(data?.length ?? 0) : undefined
}
```

- [ ] **Step 6: DoneStep**

Renders in the shell with the preview filling the pane (`WorkspacePreview` at `max-w-[420px]`), heading "<name> is ready", a summary line built above the JSX from the preview model ("3 apps on · 2 people · email sending on", omitting zero parts), and a button "Open <name>" that calls `complete()` then `router.replace(appHref(''))`.

- [ ] **Step 7: Check**

Run: `pnpm exec vitest run core/components/setup core/components/settings && pnpm exec tinycld-pkg check && pnpm run check:core-isolation`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add core/components/setup/wizard/steps core/components/settings/members
git commit -m "feat(setup): workspace, apps, email and team steps"
```

---

### Task 11: Finish-setup card and help

**Files:**
- Create: `core/components/setup/wizard/FinishSetupCard.tsx`
- Modify: `app/a/(app)/settings/index.tsx`
- Create: `core/help/first-run-setup.md`; modify `core/help/installing-packages.md`

**Interfaces:**
- Consumes: `useSetupSteps`, `StepStatusChain`, `useSetupWizardState`, `summarizeWizard`, `useCurrentRole`.

- [ ] **Step 1: Build the card**

`FinishSetupCard` returns `null` unless the current user is owner/admin and `state` exists with no `completedAt`. Otherwise it shows "Finish setup" / "<doneCount> of <total> done" and a "Continue" button → `router.push(appHref('setup/next'))`. Continue also clears `dismissedAt` via `update(s => ({ ...s, dismissedAt: undefined }))` so the redirect resumes until the person finishes or dismisses again. Render it at the top of `app/a/(app)/settings/index.tsx`.

- [ ] **Step 2: Help topics**

`core/help/first-run-setup.md`:

```markdown
---
title: Setting up a new server
summary: Claim a new server with its setup code, then set up your workspace, apps, email and team.
tags: [setup, "first run", owner]
order: 1
---

To claim a new server, open it in your browser. The server prints a setup code in its log each time it starts, until someone claims it. Enter that code, or open the link printed next to it, which fills in the code for you.

If the code is not in the log, restart the server. A new code prints at each start. After too many wrong codes, the server prints a new code.

Next, create the owner account. The owner manages the server and everyone on it.

The setup steps then help you:

- **Name your workspace** and add a logo.
- **Choose your apps.** Clear an app to hide it from everyone. You can show it again at any time in Settings → Packages.
- **Set up email sending,** so invites and password resets reach people.
- **Invite your team.**

You can skip any step. To stop and come back later, select **Finish later**. A **Finish setup** card stays at the top of Settings until you complete the last step.
```

`core/help/installing-packages.md`: add a paragraph "To hide an app from everyone without removing it, clear its switch in Settings → Packages. A hidden app stays hidden after the server restarts. Turn the switch on to show it again."

Run: `pnpm run packages:generate`.

- [ ] **Step 3: Check and commit**

Run: `pnpm exec tinycld-pkg check`
Expected: PASS.

```bash
git add core/components/setup/wizard/FinishSetupCard.tsx "app/a/(app)/settings/index.tsx" core/help
git commit -m "feat(setup): finish-setup card and help"
```

---

### Task 12: End-to-end: the shipped binary, from first boot to Done

**Files:**
- Create: `tests/standalone/boot-binary.ts`
- Modify: `tests/standalone/standalone-binary.spec.ts` (use the helper)
- Create: `tests/standalone/first-run-wizard.spec.ts`
- Modify: `tests/install/setup-and-packages.spec.ts`, `tests/install/run-first-boot-admin.sh:108-131`, `tests/install/run-todo-install.sh:220-231`, `.github/workflows/smoke-test-image.yml:100-186`

**Interfaces:**
- Produces: `bootBinary(opts?: { dataDir?: string }): Promise<{ baseURL: string; dataDir: string; logs: () => string; stop: () => Promise<void> }>` and `buildBinary(): void` in `boot-binary.ts`; `setupCodeFromLog(log: string): string | null`.

Why here and not `tests/e2e/`: the e2e server is created with a superuser already present, so it never enters first-run, and the wizard's state is deployment-wide — writing it there would redirect every parallel spec that signs in as the owner. The standalone suite boots its own binary with an empty data dir and can restart it.

- [ ] **Step 1: Extract the boot helper** — move `freePort`, the `go build` call, the spawn, the log capture and the health poll from `standalone-binary.spec.ts` into `boot-binary.ts`; `stop()` sends SIGTERM and waits for `exit`. Add:

```ts
export function setupCodeFromLog(log: string): string | null {
    const matches = [...log.matchAll(/code=([A-Z0-9]{8})/g)]
    return matches.at(-1)?.[1] ?? null
}
```

`standalone-binary.spec.ts` uses `buildBinary()` in `beforeAll` and `bootBinary()`; its assertions do not change. Run it to confirm the refactor (see Step 3 for the command).

- [ ] **Step 2: Write the wizard spec** — `tests/standalone/first-run-wizard.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { bootBinary, buildBinary, setupCodeFromLog } from './boot-binary'

// Drives the real first-run path on a fresh binary. The code is read from the
// server log, the same way a person reads it.
test.describe.configure({ mode: 'serial' })

const OWNER = { name: 'Dana Reyes', email: 'dana@example.com', password: 'OwnerPass1234!' }

type Server = Awaited<ReturnType<typeof bootBinary>>

async function readCode(server: Server): Promise<string> {
    await expect.poll(() => setupCodeFromLog(server.logs()), { timeout: 30_000 }).not.toBeNull()
    return setupCodeFromLog(server.logs()) as string
}

async function claimServer(page: Page, server: Server) {
    const code = await readCode(server)
    await page.goto(server.baseURL)
    await expect(page.getByText('Claim this server')).toBeVisible()
    await page.getByTestId('setup-code').fill(`${code.slice(0, 4)}-${code.slice(4)}`)
    await page.getByRole('button', { name: 'Continue' }).click()

    await page.getByRole('textbox', { name: 'Name', exact: true }).fill(OWNER.name)
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(OWNER.email)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(OWNER.password)
    await page.getByRole('textbox', { name: 'Confirm password', exact: true }).fill(OWNER.password)
    await page.getByRole('button', { name: 'Create account' }).click()
    await expect(page.getByText('Your workspace')).toBeVisible()
}

async function signIn(page: Page, baseURL: string) {
    await page.goto(baseURL)
    await page.getByTestId('identifier').fill(OWNER.email)
    await page.getByPlaceholder('Password').fill(OWNER.password)
    await page.getByRole('button', { name: 'Sign in' }).click()
}

test.beforeAll(() => buildBinary())

test('a wrong code is refused with a specific message', async ({ page }) => {
    const server = await bootBinary()
    try {
        await readCode(server)
        await page.goto(server.baseURL)
        await page.getByTestId('setup-code').fill('AAAA-AAAA')
        await page.getByRole('button', { name: 'Continue' }).click()
        await expect(
            page.getByText('That code does not match. Check the server log for the latest code.')
        ).toBeVisible()
    } finally {
        await server.stop()
    }
})

test('a new server is claimed, set up, paused and resumed', async ({ page }) => {
    const server = await bootBinary()
    try {
        await claimServer(page, server)

        await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Harbor Dental')
        await page.getByRole('button', { name: 'Continue' }).click()

        await expect(page.getByText('Choose your apps')).toBeVisible()
        await page.getByRole('button', { name: 'Finish later' }).click()

        // Finish later lands in the app; Settings offers to resume.
        await page.getByTestId('nav-settings').click()
        await expect(page.getByText('Finish setup')).toBeVisible()
        await page.getByRole('button', { name: 'Continue' }).click()
        await expect(page.getByText('Choose your apps')).toBeVisible()
    } finally {
        await server.stop()
    }
})

test('a hidden app stays hidden after a restart', async ({ page }) => {
    // End to end, not only in a Go test, because the bug was the boot path
    // itself re-enabling the row.
    const first = await bootBinary()
    const dataDir = first.dataDir
    let hiddenSlug = ''
    try {
        await claimServer(page, first)
        await page.getByRole('button', { name: 'Skip' }).click()
        await expect(page.getByText('Choose your apps')).toBeVisible()
        const card = page.getByTestId(/^setup-app-/).first()
        hiddenSlug = ((await card.getAttribute('data-testid')) ?? '').replace('setup-app-', '')
        await card.click()
    } finally {
        await first.stop()
    }

    const second = await bootBinary({ dataDir })
    try {
        await signIn(page, second.baseURL)
        await page.getByRole('button', { name: 'Finish later' }).click()
        await expect(page.getByTestId('nav-settings')).toBeVisible()
        await expect(page.getByTestId(`nav-${hiddenSlug}`)).toHaveCount(0)
    } finally {
        await second.stop()
    }
})
```

Each app card in `AppsStep` must carry `testID={\`setup-app-${slug}\`}` (add it in Task 10 Step 3). `bootBinary({ dataDir })` reuses the directory, so the second boot sees the claimed database and prints no code.

- [ ] **Step 3: Run the standalone suite**

Run: `pnpm exec tsx scripts/export-web.ts && pnpm run stage:embed && pnpm exec playwright test -c tests/standalone/playwright.config.ts`
Expected: all standalone specs PASS.

- [ ] **Step 4: Update the install smoke test and its scrapers**

- `run-first-boot-admin.sh` and `run-todo-install.sh`: `grep -oE 'code=[A-Z0-9]{8}' | head -1 | cut -d= -f2`; rename `TOKEN`/`PW_SETUP_TOKEN` to `CODE`/`PW_SETUP_CODE`; update messages ("no setup code printed").
- `.github/workflows/smoke-test-image.yml`: same regex; output `code`; env `PW_SETUP_CODE: ${{ steps.code.outputs.code }}`.
- `setup-and-packages.spec.ts` test 1: `page.goto('/a/setup?code=' + SETUP_CODE)`, then the owner form ("Create your owner account", labels Name/Email/Password/Confirm password, button "Create account"), then assert "Your workspace" is visible. Update the header comment (token → code, wizard instead of dashboard). Tests 2 and 3 do not change.

- [ ] **Step 5: Full check and commit**

Run: `pnpm exec tinycld-pkg check && pnpm run checks`
Expected: PASS.

```bash
git add tests/standalone tests/install .github/workflows/smoke-test-image.yml
git commit -m "test(setup): first-run wizard end to end on the shipped binary"
```

---

## Out of scope for this plan

- Steps and `core:setup-team` slot contributions from packages. Each belongs to its own repo's plan and uses the `setupSteps` field and `core:setup-team` slot this plan adds.
- A deployment that re-reads its name from `.runtime/app.json` at boot (see `org_info.go`) loses a rename made through `/api/org-info/name` when it restarts. A service provider that writes that file must decide how a rename reaches it.
