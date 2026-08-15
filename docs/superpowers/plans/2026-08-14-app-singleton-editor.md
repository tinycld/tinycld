# One App-Wide Editor Singleton Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One editor instance for the whole app, on both platforms. Core boots it lazily the first time a package says it will need editing, never disposes it, and hands it between surfaces. There is never a second editor: `LazyEditor` renders the live editor when it holds it, and the caller's read view whenever it does not.

**Why:** Three separate wins, only one of which is speed.

1. **Web and native run the same code path.** Today `useWarmEditor` returns `isWarm: false` on web, so `LazyEditor` keeps a second, cold path (`lease.result ?? own`) and every warm-only branch — the generation staleness guard, the `focusedGenerationRef` refocus, draft stash-on-release — is **never exercised by CI**, which runs on web. That blind spot is what let the Task 10 composer bug ship (see `HANDOFF-task10-comments-lazyeditor.md`): a blur destroyed the composer and no e2e could see it.
2. **The editor stops being re-created on section re-entry.** The host currently mounts in `cards/screens/_layout.tsx`, so leaving and re-entering Cards destroys and re-boots the WebView — the full ~1135 ms, paid again. An app-level singleton pays it once per launch.
3. **Web reuse.** Real but small: a Tiptap construction and schema build, single-digit ms. Not the reason for this work.

**Architecture:** The singleton lives in core, above the route tree, and is *declared* by any package that may edit (`useEditorNeeded()` from its layout) rather than *mounted* by it. First declaration boots it; nothing disposes it. `LazyEditor` loses its `own` fallback entirely — `lease.result` is the only editor, and when it is null the surface renders `readView`, whatever the reason (idle, displaced by a steal, or still booting). What that read view shows is the caller's concern, never core's. Handover stays a full reconstruction, never a mutation: native remounts the page's Tiptap on a `generation` bump (`Editor.tsx:172`, `key={init.generation}`), and web does the identical thing by keying its own `useRichEditor` on generation. No editor state can leak between surfaces on either platform.

**Tech Stack:** React Native + Expo Router, TenTap (`@10play/tentap-editor`) as WebView host, Tiptap 3 inside the page, Yjs for collaborative descriptions, Vitest for unit tests, Playwright for e2e, Biome for lint.

**Depends on:** tinycld#193 (`fix/lazy-editor-parent-owned-session`) and cards#29. Task 10's fix — a `startOpen` surface not closing itself on blur — is load-bearing here: the composer must survive losing the instance.

## Global Constraints

- **Never use `any`** to pass type checks. Embrace inference; don't over-specify.
- **Never use `biome-ignore` comments** — fix the underlying issue instead.
- Biome enforces 4-space indent, single quotes, ES5 trailing commas, no superfluous semicolons.
- Components are PascalCase, hooks camelCase `use`-prefixed, utility modules kebab-case.
- **Comments explain "why", not "what".**
- **Light + dark mode:** no raw hex. Semantic Tailwind tokens or `useThemeColor()`.
- **Both platforms must work**, and after this work they must work *the same way* — a native-only branch in shared code is a defect, not an optimization.
- **Errors → `captureException(context, error, extra?)`**. Dev tracing is `if (__DEV__) console.debug(...)`.
- **`editorHtml.ts` is generated** and gitignored — never edit or commit it. After any change under `core/lib/editor/rich/webview/source/`, regenerate with `pnpm run packages:generate` or the change is inert.
- **There is never a second editor.** Any code path that would mount one is a bug, including "just for a moment while the real one boots".
- Run checks from inside the member changed: `pnpm exec tinycld-pkg check`.
- **Component tests use `@testing-library/react`**, with a `// @vitest-environment happy-dom` pragma. `core/lib/editor/__tests__/editor-mount.test.tsx` is the reference.

## Behavior Contract

The whole design reduces to one rule. Every task below serves it.

**`LazyEditor` renders exactly two things: `readView`, or the editor.** There is no third rendering and no loading state in core.

```
holds the live instance  →  the editor
otherwise                →  readView
```

Idle, displaced by a steal, or waiting on a boot that has not finished are all the same case to core: it does not hold a usable editor, so it renders `readView`. Whether that read view is prose, a stashed draft, a placeholder, or a "loading" hint is **the caller's decision** — core never interprets it, exactly as it never interprets content.

**This makes `readView: null` the real defect behind the Task 10 bug.** The composer passes it today, which is why an ended session rendered `<Pressable>{null}</Pressable>` — an invisible box. The fix is not a new state in core; it is that **every caller supplies a real `readView`**. For the composer that is the stashed draft as static text, or the placeholder when empty. Tapping it re-acquires.

**A steal is a blur.** `acquire` is an unconditional transfer, so any surface may take the instance from any other. The displaced surface's editor blurs, which for a `commitOnBlur` surface (an inline comment edit) commits, and for the composer stashes the draft without posting. Per tinycld#193 the composer stays *open* rather than closing — but since a non-holder renders `readView` either way, that affects only whether it must re-acquire on tap, not what the user sees.

**`startOpen` still exists**, but only for the initial mount (the composer opens already-editing instead of waiting for a tap) and for the #193 no-self-close rule. It no longer implies a distinct rendering.

**Only one surface can be editing at a time**, which is why one instance suffices: an idle `LazyEditor` renders `readView` and holds nothing.

## File Structure

**Core — the singleton (new `core/lib/editor/warm/editor-singleton.tsx`)**
Module-level holder + React context. Owns the one `useRichEditor` result, the boot latch, and readiness. Platform-neutral: the *same* file for web and native, which is the point of the exercise. Replaces `WarmEditorHost.native.tsx` / `.web.tsx`.

**Core — declaration hook (new `core/lib/editor/warm/use-editor-needed.ts`)**
`useEditorNeeded()` — a package layout calls this to say "this section may edit". First call boots the singleton (in an effect, never during render). Idempotent; never disposes.

**Core — mount point (`app/_layout.tsx`)**
The singleton's provider wraps the route tree so the instance outlives any package's section. Renders nothing until booted, so an app whose user never opens an editing package pays nothing.

**Core — store (`core/lib/editor/warm/warm-editor-store.ts`)**
Unchanged in behavior; gains a `ready` flag so a holder can distinguish "booting" from "held".

**Core — hook (`core/lib/editor/warm/use-warm-editor.ts`)**
Collapses `use-warm-editor.native.ts` + `.web.ts` into ONE file. `isWarm` is now always true when a host exists on either platform.

**Core — web reuse (`core/lib/editor/rich/use-rich-editor.web.tsx`)**
Honors `generation`, which it currently ignores entirely (native reads it; web does not). Keying the editor on it makes a web handover the same full reconstruction native already performs.

**Core — swap component (`core/components/editor/LazyEditor.tsx`)**
Drops the `own` cold fallback. Renders `readView` or the editor, and nothing else.

**Cards**
- `screens/_layout.tsx` — `WarmEditorHost` wrapper becomes a `useEditorNeeded()` call.
- `components/detail/CommentEditor.tsx` — composer's displaced-state rendering.

**Task ordering rationale:** Task 1 makes web honor `generation` (independently testable, no behavior change on its own). Task 2 is the singleton + declaration hook (pure-ish, unit-testable). Task 3 unifies the lease hook. Task 4 removes `LazyEditor`'s fallback and collapses it to two renderings — the risky one, and it lands only after the machinery under it is proven. Task 5 migrates cards. Task 6 is the e2e that could not exist before. Task 7 is device verification.

---

### Task 1: Make the web editor honor `generation`

**Files:**
- Modify: `core/lib/editor/rich/use-rich-editor.web.tsx`
- Modify: `core/lib/editor/rich/options.ts` (doc comment only — the option is no longer native-only)
- Create: `core/lib/editor/rich/__tests__/web-generation.test.tsx`

**Interfaces:**
- Consumes: `UseRichEditorOptions.generation` (already declared, currently ignored on web).
- Produces: a web editor that fully reconstructs when `generation` changes.

The native page does this at `webview/source/Editor.tsx:172` with `<EditorMounted key={init.generation} …>` — "new Tiptap, new Y.Doc, new undo stack, new extension set. Nothing survives the swap." Web must match, or a handover would leak the previous surface's undo history and selection into the next.

`useEditor`'s dep array is currently `[extensions, editable]`. Adding `generation` to it is the whole mechanism — tiptap tears down and rebuilds on a dep change.

- [x] **Step 1: Add `generation` to the editor's deps**

Destructure `generation = 0` from options and append it to the `useEditor` dep array. Note in a comment WHY (undo/selection/extension leak across surfaces), referencing the native page's equivalent.

- [x] **Step 2: Prove reconstruction**

Write `web-generation.test.tsx`: render with content A at generation 0, bump to 1 with content B, assert the editor reports B and that undo cannot reach A.

- [x] **Step 3: Update the option's doc comment**

`options.ts:86-92` says "for a reused (warm) editor … re-initializes the underlying WebView page". Make it platform-neutral: it now also rebuilds the in-process editor on web.

- [x] **Step 4: Check**

```sh
cd ~/code/tinycld/tinycld && pnpm exec tinycld-pkg check
```

Expected: PASS. No behavior change yet — nothing passes a changing `generation` on web until Task 3.

---

### Task 2: The app-level singleton and its declaration hook

**Files:**
- Create: `core/lib/editor/warm/editor-singleton.tsx`
- Create: `core/lib/editor/warm/use-editor-needed.ts`
- Create: `core/lib/editor/warm/__tests__/editor-singleton.test.tsx`
- Modify: `core/lib/editor/warm/warm-editor-store.ts` (add `ready`)
- Modify: `core/lib/editor/warm/index.ts`
- Delete: `WarmEditorHost.native.tsx`, `WarmEditorHost.web.tsx`, `WarmEditorHost.d.ts`

**Interfaces:**
- Produces: `EditorSingletonProvider`, `useEditorNeeded()`, `useEditorSingleton()`.
- Consumes: `useRichEditor`, `createWarmEditorStore`, `createDraftStore`.

The provider holds the one editor and boots it only after a package declares need. Before that it renders `children` and nothing else — an app whose user never opens cards or mail must not construct an editor.

Boot is a state flip in an effect (`useEditorNeeded`), never during render, since it changes shared state other components subscribe to.

**Never disposed:** there is no unmount path that tears the editor down. It survives package navigation by construction, because it lives above the route tree.

**Parking:** the native viewport rules still apply — a parked WebView must stay laid out and off-viewport (not `display:none`, not zero-size), or iOS may never paint it and it never finishes booting. Carry `WarmEditorViewport` over verbatim. On web the parked editor is likewise rendered off-viewport rather than unmounted, so both platforms park identically.

- [x] **Step 1: Add `ready` to the store**

`LazyEditor` renders the editor only when it holds a *usable* one; a holder whose editor is still booting renders `readView` like any other non-holder. Add `ready: boolean` to `WarmSnapshot`, set when the editor reports usable, and have the lease surface `result: null` until then — so the "is there an editor to render" question stays a single null check rather than a second condition at every call site.

- [x] **Step 2: Write the singleton provider**

One `useRichEditor` (autofocus false — a parked editor must never open the keyboard), the store, the draft store, and the parked viewport. Context value carries `{ store, drafts, result, ready, setOptions }`.

- [x] **Step 3: Write `useEditorNeeded()`**

Idempotent; first call flips the provider's booted flag in an effect. Document that it is a DECLARATION, not a mount — the caller gets nothing back and renders nothing.

- [x] **Step 4: Unit-test the singleton**

Cover: nothing constructs before a declaration; one declaration boots exactly one editor; a second declaration from another package does not boot a second; the editor survives a subtree unmount (the "never disposed" guarantee).

- [x] **Step 5: Delete the old host trio and re-export**

Remove `WarmEditorHost.*`; update `index.ts`. `useDraftStore` now reads the singleton context and is no longer null on web.

- [x] **Step 6: Check**

```sh
cd ~/code/tinycld/tinycld && pnpm exec tinycld-pkg check
```

---

### Task 3: One lease hook for both platforms

**Files:**
- Create: `core/lib/editor/warm/use-warm-editor.ts`
- Delete: `use-warm-editor.native.ts`, `use-warm-editor.web.ts`, `use-warm-editor.d.ts`
- Modify: `core/lib/editor/warm/types.ts`

**Interfaces:**
- Produces: `useWarmEditor(surfaceId, options)` returning `{ isWarm, ready, acquire, release, result, generation }`.

The web stub currently returns `isWarm: false, result: null, generation: 0` and a no-op acquire — the source of the second code path. One file replaces both; the platform split disappears entirely at this layer.

`generation` is no longer "always 0 on web" — update that doc comment in `types.ts`, which explicitly says so today.

- [x] **Step 1: Write the unified hook**

Port `use-warm-editor.native.ts` verbatim, reading the singleton context instead of the per-section host. Keep the release-on-unmount effect: a surface can be unmounted mid-edit (the card closes) and the store must not keep a holder that no longer exists.

- [x] **Step 2: Delete the platform variants and fix the doc comments**

- [x] **Step 3: Check**

```sh
cd ~/code/tinycld/tinycld && pnpm exec tinycld-pkg check
```

Expected: PASS. `LazyEditor` still has its `own` fallback at this point, so web now has BOTH a working lease and a fallback — harmless, and removed next.

---

### Task 4: `LazyEditor` — one editor, two renderings

**Files:**
- Modify: `core/components/editor/LazyEditor.tsx`
- Modify: `core/components/editor/__tests__/lazy-editor-handover.test.tsx`
- Create: `core/components/editor/__tests__/lazy-editor-states.test.tsx`

**Interfaces:**
- Consumes: the unified `useWarmEditor`.
- Produces: `readView` or the editor, with no second editor anywhere.

This is the load-bearing task. `const own = useRichEditor(...)` and `const active = lease.result ?? own` both go away. `active` becomes `lease.result`, which is nullable, so every use of it needs a null path — including `readContent` inside `submit`, `endSession`'s stash read, and the slots object.

**No new prop, and no loading component.** Holding a usable editor renders it; anything else renders `readView`. `readView` therefore stops being optional in practice: a caller passing null gets an invisible box, which is the Task 10 bug.

- [x] **Step 1: Remove the fallback and collapse to two renderings**

Guard every `active.editor` use. A submit with no editor must not silently write an empty string over the user's content — return without committing and `captureException`.

- [x] **Step 2: Harden the stash**

With the fallback gone, `onRelease`'s async read is the ONLY copy of an uncommitted draft — today the surface's own editor keeps the text alive regardless. A failed read currently logs and swallows. Keep the session ending, but make the failure visible in the surface rather than silent.

- [x] **Step 3: Unit-test the renderings**

`lazy-editor-states.test.tsx`: holding a ready editor renders it; not holding renders `readView`; a holder whose editor is still booting renders `readView`; a steal moves the instance and leaves the loser rendering `readView` (not closed, not blank); a submit while holding nothing writes nothing.

- [x] **Step 4: Check**

```sh
cd ~/code/tinycld/tinycld && pnpm exec tinycld-pkg check
```

---

### Task 5: Migrate cards

**Files:**
- Modify: `cards/tinycld/cards/screens/_layout.tsx`
- Modify: `cards/tinycld/cards/components/detail/CommentEditor.tsx`

- [x] **Step 1: Layout declares need instead of mounting a host**

```tsx
export default function CardsLayout() {
    useEditorNeeded()
    return <Stack screenOptions={{ headerShown: false }} />
}
```

The `options={{ contentFormat: 'markdown', minHeight: 72 }}` prop goes away — it was already vestigial (`acquire()` pushes the surface's own options before every handover, and the page rebuilds from the init payload), and an app-level singleton cannot take a per-package content format. Rewrite the layout's comment, which currently explains per-section warming.

- [x] **Step 2: Composer supplies a real `readView`**

Replace `readView: null` — the root of the Task 10 bug — with the stashed draft as static text in the composer's frame, placeholder when empty. Tapping it re-acquires. This is also what the composer shows while the singleton is still booting, so no separate loading affordance is needed.

- [x] **Step 3: Cards checks**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg check
```

---

### Task 6: The e2e that could not exist before

**Files:**
- Create: `cards/tests/e2e/editor-handover.spec.ts`

These assertions were impossible while web had no warm path — every one of them exercises code CI has never run.

- [x] **Step 1: Handover between comment surfaces**

Edit comment A, then click comment B's prose. Assert exactly ONE ProseMirror exists on the page at all times (the core invariant), B is editing, and A committed its edit (`commitOnBlur`).

- [x] **Step 2: The composer steal**

Type a draft in the composer, start editing a comment, assert the composer shows the draft as static text and posts nothing. Return to the composer, assert the draft is intact and editable — the stash round-trip, which previously only "worked" because web kept a second editor.

- [x] **Step 3: Description ↔ comment handover**

The collab/non-collab crossing. Assert no text bleeds between them and undo in one cannot reach the other's content.

- [x] **Step 4: Run**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg test:e2e -- editor-handover.spec.ts comment-editing.spec.ts card-description.spec.ts
```

---

### Task 7: Device verification

- [ ] **Step 1: iOS simulator**

Enter Cards: singleton boots once (`page-ready` fires on section entry, not on first tap). First description edit shows only `init-sent` + `first-height` (~34 ms), not a fresh `page-ready`.

- [ ] **Step 2: The re-entry win**

Leave Cards, return, edit again. Assert NO second `page-ready` — the old per-section host re-booted here; the singleton must not.

- [ ] **Step 3: Loading state**

Tap to edit immediately on section entry, beating the boot. Confirm the surface keeps showing its read view and then swaps to a working editor once ready — never a dead or empty box.

- [ ] **Step 4: Retire the superseded handoffs**

Delete `HANDOFF-editor-webview-cost.md` and `HANDOFF-task10-comments-lazyeditor.md`.

## Open Questions

- **Mail** stays on `useMailEditor` (own editor, HTML). The singleton is markdown/HTML-neutral (`contentFormat` comes from the acquiring surface), so mail can migrate later without rework — but until it does, "one editor app-wide" is aspirational: mail's compose is still a second editor by construction. Scope decision, deliberate.
- **`minHeight` on a shared instance.** Cards' host passed 72; surfaces pass their own (composer 60, inline 48). Confirm the acquiring surface's value always wins now that no host-level default exists.
