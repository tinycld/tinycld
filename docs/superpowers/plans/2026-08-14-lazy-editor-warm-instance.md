# LazyEditor and the Warm Editor Instance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the ~1.1 s cold start when opening a card description or comment for editing, by keeping one warm WebView editor alive per cards session and swapping surfaces into it.

**Architecture:** The WebView page already mounts in two stages — it boots, posts `EDITOR_READY`, and renders `null` until an init payload arrives. All 1135 ms is stage one (browser cold start, 0.86 MB bundle, React boot) and is configuration-independent. This work makes the init payload re-sendable so a booted page can be reconfigured for a new surface, keeps one such page alive above the card detail, and adds a core `LazyEditor` component that owns the read/edit swap and the commit semantics. Re-initialization is a full stage-two reconstruction, never a mutation, so no editor state can leak between surfaces.

**Tech Stack:** React Native + Expo Router, TenTap (`@10play/tentap-editor`) as WebView host, Tiptap 3 inside the page, Yjs for collaborative descriptions, Vitest for unit tests, Playwright for e2e, Biome for lint.

**Spec:** `tinycld/docs/superpowers/specs/2026-08-14-lazy-editor-warm-instance-design.md`

## Global Constraints

- **Never use `any`** to pass type checks. Embrace inference; don't over-specify.
- **Never use `biome-ignore` comments** — fix the underlying issue instead.
- Biome enforces 4-space indent, single quotes, ES5 trailing commas, no superfluous semicolons.
- Components are PascalCase (`LazyEditor.tsx`), hooks camelCase `use`-prefixed, utility modules kebab-case (`warm-editor-store.ts`).
- **Comments explain "why", not "what".** Self-explanatory code needs none.
- **Light + dark mode:** no raw hex. Use semantic Tailwind tokens or `useThemeColor()`.
- **Both platforms must work.** Web has no WebView and no cold start; every native-only path needs a web equivalent that behaves identically from the user's point of view.
- **Errors → `captureException(context, error, extra?)`** from `@tinycld/core/lib/errors`. Never `console.log` unguarded; dev tracing is `if (__DEV__) console.debug(...)`.
- **`editorHtml.ts` is generated** by the webview-bundler and is gitignored — never edit or commit it. After any change under `core/lib/editor/rich/webview/source/`, regenerate with `cd ~/code/tinycld/tinycld && pnpm run packages:generate` or the change is inert.
- **Warm is an optimization, never a correctness dependency.** Every path must degrade to a fresh mount rather than to a broken editor.
- Run checks from inside the member changed: `pnpm exec tinycld-pkg check` (biome + tsc + vitest).
- **Component tests use `@testing-library/react`, never `@testing-library/react-native`** — the latter is not a dependency of this workspace. React Native components render under it with a `// @vitest-environment happy-dom` pragma as the first line of the file. `core/lib/editor/__tests__/editor-mount.test.tsx` is the reference example.

## File Structure

**Core — protocol (`core/lib/editor/rich/webview/source/protocol.ts`)**
Adds `generation` to `RichEditorInitPayload` and the `APP_PARK` message type. Shared by host and page, DOM-free, unit-testable.

**Core — page (`core/lib/editor/rich/webview/source/Editor.tsx`)**
`Editor` gains park handling and keys `EditorMounted` on `init.generation`, making stage two reconstructible.

**Core — host (`core/lib/editor/use-webview-editor.tsx`)**
`initSentRef` one-shot latch becomes generation tracking.

**Core — warm instance (new `core/lib/editor/warm/`)**
- `warm-editor-store.ts` — the acquire/release state machine. Plain TS, no React, no WebView: directly unit-testable.
- `draft-store.ts` — the composer draft store. Plain TS, unit-testable.
- `WarmEditorHost.native.tsx` / `WarmEditorHost.web.tsx` — mounts (or, on web, does not mount) the parked WebView.
- `use-warm-editor.native.ts` / `use-warm-editor.web.ts` — consumer-facing acquire hook; web aliases `useRichEditor`.
- `index.ts` — public surface.

**Core — swap component (new `core/components/editor/LazyEditor.tsx`)**
Owns the press target, the read/edit swap, and the commit semantics. Format-neutral: takes `readView` as a prop and never interprets content.

**Cards**
- `screens/_layout.tsx` — mounts `WarmEditorHost`.
- `components/detail/CardDetail.tsx` — description swap replaced by `LazyEditor`.
- `components/detail/DetailActivity.tsx` — inline comment swap replaced by `LazyEditor`.
- `components/detail/CommentComposer.tsx` — draft store wiring.

**Task ordering rationale:** Tasks 1–3 are the protocol and host plumbing, each independently testable. Task 4 is the state machine (pure, no UI). Task 5 is the draft store (pure). Task 6 assembles the warm host + hook. Task 7 is `LazyEditor` with its commit-semantics matrix. Tasks 8–10 migrate cards. Task 11 is device verification.

---

### Task 1: Make the init payload carry a generation

**Files:**
- Modify: `core/lib/editor/rich/webview/source/protocol.ts`
- Test: `core/lib/editor/rich/webview/source/__tests__/protocol-generation.test.ts` (create)

**Interfaces:**
- Consumes: nothing (first task).
- Produces: `RichEditorInitPayload.generation: number` — a monotonically increasing counter, `0` for the first init of a page. `APP_PARK = 'park'`, a host→page message type on the `'app'` namespace with `null` payload.

- [ ] **Step 1: Write the failing test**

Create `core/lib/editor/rich/webview/source/__tests__/protocol-generation.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { makeMessage } from '../../../../message-bus/types'
import { APP_INIT, APP_PARK, type RichEditorInitPayload } from '../protocol'

/**
 * A warm editor is re-initialized rather than remounted, so the page needs a
 * way to tell a NEW configuration from a re-delivery of the one it already
 * applied. The generation is that discriminator: the page keys its Tiptap
 * subtree on it, so a bumped value is a full stage-two reconstruction and a
 * repeated value is a no-op.
 */
describe('init generation', () => {
    it('rides in the init payload so the page can rebuild on a bump', () => {
        const payload: RichEditorInitPayload = {
            generation: 3,
            contentFormat: 'markdown',
            initialContent: 'hello',
            placeholder: '',
            editable: true,
            autofocus: false,
            colors: { bg: '#000', fg: '#fff', placeholder: '#888', primary: '#0f0' },
        }
        const message = makeMessage('app', APP_INIT, payload)

        expect(message.namespace).toBe('app')
        expect((message.payload as RichEditorInitPayload).generation).toBe(3)
    })

    it('names a park message so a released editor can drop to stage one', () => {
        expect(APP_PARK).toBe('park')
        expect(makeMessage('app', APP_PARK, null).type).toBe('park')
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/rich/webview/source/__tests__/protocol-generation.test.ts
```

Expected: FAIL — `APP_PARK` is not exported, and `generation` is not a property of `RichEditorInitPayload`.

- [ ] **Step 3: Add the generation field and park message**

In `core/lib/editor/rich/webview/source/protocol.ts`, beside the existing `APP_INIT` declaration:

```ts
/**
 * host → WebView: drop back to stage one — no Tiptap, no document, no
 * awareness — while keeping the page itself booted.
 *
 * This is what makes a warm editor possible. The expensive part of an editor is
 * the browser cold start and bundle parse, which happen BEFORE init; tearing
 * down only what init built leaves a page that can be reconfigured for the next
 * surface in ~34 ms instead of ~1135 ms.
 */
export const APP_PARK = 'park'
```

In the `RichEditorInitPayload` interface, as the first field:

```ts
    /**
     * Monotonic counter identifying this configuration.
     *
     * Init is no longer one-shot: a warm editor is handed from surface to
     * surface by re-sending this payload. The page keys its editor subtree on
     * this value, so a bump is a full reconstruction — new Tiptap, new Y.Doc,
     * new undo stack. That is deliberate rather than wasteful: a partial reset
     * would risk leaking one surface's undo history into another, and since a
     * blur COMMITS an inline comment edit, leaked state is a data-loss risk
     * rather than a cosmetic one.
     *
     * A repeated value must be ignored, so a re-delivered payload does not
     * discard what the user has typed.
     */
    generation: number
```

- [ ] **Step 4: Run test to verify it passes**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/rich/webview/source/__tests__/protocol-generation.test.ts
```

Expected: PASS (2 tests).

- [ ] **Step 5: Fix the now-broken call site**

`generation` is required, so `use-rich-editor.native.tsx` no longer typechecks. In its `initPayload` memo (around line 182), add as the first field of the returned object:

```ts
            generation: 0,
```

and leave the dependency array unchanged — a literal has no identity to track. Task 6 replaces this with the real counter.

- [ ] **Step 6: Typecheck**

```sh
cd ~/code/tinycld/tinycld && pnpm exec tinycld-pkg typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add core/lib/editor/rich/webview/source/protocol.ts \
        core/lib/editor/rich/webview/source/__tests__/protocol-generation.test.ts \
        core/lib/editor/rich/use-rich-editor.native.tsx
git commit -m "feat(editor): carry an init generation and name the park message"
```

---

### Task 2: Rebuild the page's editor on a generation bump

**Files:**
- Modify: `core/lib/editor/rich/webview/source/Editor.tsx:111-144`
- Test: `core/lib/editor/rich/webview/source/__tests__/init-reducer.test.ts` (create)

**Interfaces:**
- Consumes: `RichEditorInitPayload.generation`, `APP_PARK` (Task 1).
- Produces: `reduceInit(current: RichEditorInitPayload | null, incoming: RichEditorInitPayload | null): RichEditorInitPayload | null` — exported from `Editor.tsx` for testing. `null` incoming means park.

The page's message handling is inside a `useEffect` and needs a DOM to test. Extracting the decision into a pure reducer makes the interesting part testable without one.

- [ ] **Step 1: Write the failing test**

Create `core/lib/editor/rich/webview/source/__tests__/init-reducer.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { reduceInit } from '../Editor'
import type { RichEditorInitPayload } from '../protocol'

function payload(generation: number, content = ''): RichEditorInitPayload {
    return {
        generation,
        contentFormat: 'markdown',
        initialContent: content,
        placeholder: '',
        editable: true,
        autofocus: false,
        colors: { bg: '#000', fg: '#fff', placeholder: '#888', primary: '#0f0' },
    }
}

describe('init reducer', () => {
    it('accepts the first init', () => {
        expect(reduceInit(null, payload(0))?.generation).toBe(0)
    })

    it('accepts a bumped generation, so a handover reconfigures the page', () => {
        expect(reduceInit(payload(0), payload(1, 'next surface'))?.initialContent).toBe(
            'next surface'
        )
    })

    /**
     * The host posts init from an effect, and a re-delivery of the SAME
     * configuration must not rebuild the editor — that would discard whatever
     * the user has typed since it was applied.
     */
    it('ignores a repeated generation, so typing is not discarded', () => {
        const current = payload(2, 'typed by the user')
        expect(reduceInit(current, payload(2, 'the original seed'))).toBe(current)
    })

    /** Out-of-order delivery must not roll the page back to an older surface. */
    it('ignores an older generation', () => {
        const current = payload(5)
        expect(reduceInit(current, payload(4))).toBe(current)
    })

    it('parks on a null, dropping to stage one', () => {
        expect(reduceInit(payload(3), null)).toBeNull()
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/rich/webview/source/__tests__/init-reducer.test.ts
```

Expected: FAIL — `reduceInit` is not exported from `Editor.tsx`.

- [ ] **Step 3: Add the reducer and wire it into the page**

In `core/lib/editor/rich/webview/source/Editor.tsx`, add above `export function Editor()`:

```ts
/**
 * Decide what an incoming init means for the page's current configuration.
 *
 * Pure so it can be tested without a DOM — the message plumbing around it needs
 * a WebView, but the interesting decisions are here.
 *
 * `null` incoming is a park request: drop to stage one, keeping the booted page.
 */
export function reduceInit(
    current: RichEditorInitPayload | null,
    incoming: RichEditorInitPayload | null
): RichEditorInitPayload | null {
    if (incoming === null) return null
    // A repeat or a late-arriving older payload must not rebuild the editor —
    // that discards whatever has been typed since the current one was applied.
    if (current !== null && incoming.generation <= current.generation) return current
    return incoming
}
```

Replace the body of the `onMessage` handler's init branch (currently `setInit(parsed.payload as RichEditorInitPayload)`) with:

```ts
            if (parsed.namespace === 'app' && parsed.type === 'init') {
                const incoming = parsed.payload as RichEditorInitPayload
                setInit(current => reduceInit(current, incoming))
                return
            }
            if (parsed.namespace === 'app' && parsed.type === APP_PARK) {
                setInit(current => reduceInit(current, null))
            }
```

Add `APP_PARK` to the existing import from `./protocol`.

- [ ] **Step 4: Key the editor subtree on the generation**

Change the render at the end of `Editor()` from `return <EditorMounted init={init} />` to:

```tsx
    // Keyed on the generation so a handover is a full reconstruction: new
    // Tiptap, new Y.Doc (useCollabDoc's useState initializer reruns), new undo
    // stack, new extension set. Nothing survives the swap, which is what makes
    // it safe to reuse one page across surfaces.
    return <EditorMounted key={init.generation} init={init} />
```

- [ ] **Step 5: Run test to verify it passes**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/rich/webview/source/__tests__/init-reducer.test.ts
```

Expected: PASS (5 tests).

- [ ] **Step 6: Regenerate the WebView bundle**

The page is bundled into a generated `editorHtml.ts`; the change is inert until this runs.

```sh
cd ~/code/tinycld/tinycld && pnpm run packages:generate
```

Expected: completes without error. Do **not** stage `editorHtml.ts` — it is gitignored.

- [ ] **Step 7: Commit**

```bash
git add core/lib/editor/rich/webview/source/Editor.tsx \
        core/lib/editor/rich/webview/source/__tests__/init-reducer.test.ts
git commit -m "feat(editor): rebuild the webview editor on a generation bump"
```

---

### Task 3: Let the host post init more than once

**Files:**
- Modify: `core/lib/editor/use-webview-editor.tsx:300-317`
- Test: `core/lib/editor/__tests__/init-dispatch.test.ts` (create)

**Interfaces:**
- Consumes: `RichEditorInitPayload.generation` (Task 1).
- Produces: `shouldPostInit(lastPostedGeneration: number | null, incomingGeneration: number, isReady: boolean): boolean` — exported from `use-webview-editor.tsx`.

- [ ] **Step 1: Write the failing test**

Create `core/lib/editor/__tests__/init-dispatch.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { shouldPostInit } from '../use-webview-editor'

/**
 * Init used to be latched one-shot per mount. A warm editor is handed between
 * surfaces by re-initializing it, so the latch becomes generation tracking —
 * post each generation exactly once, and never before the page is ready.
 */
describe('init dispatch', () => {
    it('posts the first init once the page is ready', () => {
        expect(shouldPostInit(null, 0, true)).toBe(true)
    })

    it('waits for the page, since nothing would receive it', () => {
        expect(shouldPostInit(null, 0, false)).toBe(false)
    })

    it('does not repost the generation it already sent', () => {
        expect(shouldPostInit(0, 0, true)).toBe(false)
    })

    it('posts a bumped generation, which is how a handover happens', () => {
        expect(shouldPostInit(0, 1, true)).toBe(true)
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/__tests__/init-dispatch.test.ts
```

Expected: FAIL — `shouldPostInit` is not exported.

- [ ] **Step 3: Add the predicate**

In `core/lib/editor/use-webview-editor.tsx`, above `export function useWebViewEditor`:

```ts
/**
 * Whether the host should post this init payload.
 *
 * Replaces the previous one-shot latch. A warm editor is reconfigured by
 * re-sending init, so the rule is "each generation exactly once, never before
 * the page reports ready" rather than "only ever once".
 */
export function shouldPostInit(
    lastPostedGeneration: number | null,
    incomingGeneration: number,
    isReady: boolean
): boolean {
    if (!isReady) return false
    return lastPostedGeneration === null || incomingGeneration > lastPostedGeneration
}
```

- [ ] **Step 4: Replace the latch in the effect**

Replace the `initSentRef` declaration with:

```ts
    // Which generation has been posted, rather than whether ANY init was — the
    // warm editor reconfigures itself by posting a new one.
    const lastInitGenerationRef = useRef<number | null>(null)
```

and replace the effect body with:

```ts
    useEffect(() => {
        const generation = (initPayload as { generation?: unknown })?.generation
        // Consumers that predate the warm path (mail, text) send no generation;
        // treat those as a single one-shot init, exactly as before.
        const incoming = typeof generation === 'number' ? generation : 0
        if (!shouldPostInit(lastInitGenerationRef.current, incoming, webviewReady)) return
        const webview = bridge.webviewRef?.current
        if (!webview) return
        const message = makeMessage('app', 'init', initPayload)
        try {
            if (__DEV__) {
                console.log('[editor.mount] init-sent', Date.now() - mountAtRef.current, 'ms')
            }
            webview.postMessage(JSON.stringify(message))
            lastInitGenerationRef.current = incoming
        } catch (err) {
            // postMessage can fail mid-handshake; the next render's
            // webviewReady or bridge identity change will retry. The generation
            // is deliberately NOT recorded here, so the retry still fires.
            captureException('editor.postInit', err, { generation: incoming })
        }
    }, [bridge, webviewReady, initPayload])
```

Add the import at the top of the file:

```ts
import { captureException } from '../errors'
```

- [ ] **Step 5: Run test to verify it passes**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/__tests__/init-dispatch.test.ts
```

Expected: PASS (4 tests).

- [ ] **Step 6: Run the full core editor suite**

Nothing should regress — mail and text send no generation and keep one-shot behavior.

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add core/lib/editor/use-webview-editor.tsx core/lib/editor/__tests__/init-dispatch.test.ts
git commit -m "feat(editor): post init per generation instead of once per mount"
```

---

### Task 4: The warm editor state machine

**Files:**
- Create: `core/lib/editor/warm/warm-editor-store.ts`
- Test: `core/lib/editor/warm/__tests__/warm-editor-store.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type SurfaceId = string` — `composer:<cardId>`, `comment:<id>`, `description:<cardId>`.
  - `createWarmEditorStore(): WarmEditorStore`
  - `WarmEditorStore` = `{ acquire(surfaceId: SurfaceId): number; release(surfaceId: SurfaceId): boolean; holder(): SurfaceId | null; generation(): number; subscribe(listener: () => void): () => void; getSnapshot(): WarmSnapshot }`
  - `interface WarmSnapshot { holder: SurfaceId | null; generation: number }`
  - `acquire` returns the generation the caller must send. `release` returns `true` if the caller actually held it (a stale release from a surface that already lost it returns `false` and changes nothing).

- [ ] **Step 1: Write the failing test**

Create `core/lib/editor/warm/__tests__/warm-editor-store.test.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'
import { createWarmEditorStore } from '../warm-editor-store'

/**
 * One WebView is shared by every editing surface in a package. The store is who
 * holds it and which configuration generation is live — deliberately plain TS
 * with no React and no WebView, because the handover rules are the part worth
 * testing and neither of those is needed to test them.
 */
describe('warm editor store', () => {
    it('starts unheld', () => {
        const store = createWarmEditorStore()
        expect(store.holder()).toBeNull()
    })

    it('hands the instance to an acquiring surface', () => {
        const store = createWarmEditorStore()
        store.acquire('comment:a')
        expect(store.holder()).toBe('comment:a')
    })

    it('bumps the generation on every acquire, so the page rebuilds', () => {
        const store = createWarmEditorStore()
        const first = store.acquire('comment:a')
        const second = store.acquire('comment:b')
        expect(second).toBeGreaterThan(first)
    })

    /**
     * The handover case. Taking the instance must transfer it outright — an
     * acquire while another surface holds it is how tapping a second comment
     * behaves, and leaving the old holder recorded would let its release later
     * evict the new one.
     */
    it('transfers the instance when another surface acquires it', () => {
        const store = createWarmEditorStore()
        store.acquire('composer:card1')
        store.acquire('comment:a')
        expect(store.holder()).toBe('comment:a')
    })

    it('reports a release by the current holder', () => {
        const store = createWarmEditorStore()
        store.acquire('comment:a')
        expect(store.release('comment:a')).toBe(true)
        expect(store.holder()).toBeNull()
    })

    /**
     * A surface that was displaced still unmounts and still calls release. That
     * late call must not evict whoever took over, or tapping from one comment to
     * another would blank the editor that just opened.
     */
    it('ignores a release from a surface that no longer holds it', () => {
        const store = createWarmEditorStore()
        store.acquire('composer:card1')
        store.acquire('comment:a')

        expect(store.release('composer:card1')).toBe(false)
        expect(store.holder()).toBe('comment:a')
    })

    it('notifies subscribers on acquire and release', () => {
        const store = createWarmEditorStore()
        const listener = vi.fn()
        store.subscribe(listener)

        store.acquire('comment:a')
        store.release('comment:a')

        expect(listener).toHaveBeenCalledTimes(2)
    })

    it('stops notifying after unsubscribe', () => {
        const store = createWarmEditorStore()
        const listener = vi.fn()
        store.subscribe(listener)()

        store.acquire('comment:a')

        expect(listener).not.toHaveBeenCalled()
    })

    /** useSyncExternalStore requires a stable snapshot or it loops forever. */
    it('returns a stable snapshot while nothing changes', () => {
        const store = createWarmEditorStore()
        store.acquire('comment:a')
        expect(store.getSnapshot()).toBe(store.getSnapshot())
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/warm/__tests__/warm-editor-store.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement the store**

Create `core/lib/editor/warm/warm-editor-store.ts`:

```ts
/** Names an editing surface: `composer:<cardId>`, `comment:<id>`, `description:<cardId>`. */
export type SurfaceId = string

export interface WarmSnapshot {
    holder: SurfaceId | null
    generation: number
}

export interface WarmEditorStore {
    /** Take the instance. Returns the generation the caller must post. */
    acquire(surfaceId: SurfaceId): number
    /** Give it back. Returns false if the caller had already been displaced. */
    release(surfaceId: SurfaceId): boolean
    holder(): SurfaceId | null
    generation(): number
    subscribe(listener: () => void): () => void
    getSnapshot(): WarmSnapshot
}

/**
 * Who holds the single warm editor, and which configuration is live.
 *
 * A store rather than component state because the holder changes from event
 * handlers in unrelated subtrees (a comment row, the composer, the description),
 * and useSyncExternalStore lets each of them re-render without a context that
 * re-renders the whole card.
 */
export function createWarmEditorStore(): WarmEditorStore {
    let holder: SurfaceId | null = null
    let generation = 0
    // Rebuilt only on change: useSyncExternalStore compares snapshots by
    // identity and loops forever if a fresh object is returned each call.
    let snapshot: WarmSnapshot = { holder, generation }
    const listeners = new Set<() => void>()

    function commit() {
        snapshot = { holder, generation }
        for (const listener of listeners) listener()
    }

    return {
        acquire(surfaceId) {
            // Unconditional transfer: acquiring while another surface holds the
            // instance is the ordinary handover (tapping a second comment), and
            // the previous holder is dropped outright so its later release
            // cannot evict whoever took over.
            holder = surfaceId
            generation += 1
            commit()
            return generation
        },
        release(surfaceId) {
            // A displaced surface still unmounts and still releases. Honouring
            // that would blank the editor that just took over.
            if (holder !== surfaceId) return false
            holder = null
            commit()
            return true
        },
        holder: () => holder,
        generation: () => generation,
        subscribe(listener) {
            listeners.add(listener)
            return () => listeners.delete(listener)
        },
        getSnapshot: () => snapshot,
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/warm/__tests__/warm-editor-store.test.ts
```

Expected: PASS (9 tests).

- [ ] **Step 5: Commit**

```bash
git add core/lib/editor/warm/warm-editor-store.ts core/lib/editor/warm/__tests__/warm-editor-store.test.ts
git commit -m "feat(editor): add the warm editor acquire/release store"
```

---

### Task 5: The composer draft store

**Files:**
- Create: `core/lib/editor/warm/draft-store.ts`
- Test: `core/lib/editor/warm/__tests__/draft-store.test.ts`

**Interfaces:**
- Consumes: `SurfaceId` (Task 4).
- Produces: `createDraftStore(): DraftStore`, where `DraftStore` = `{ stash(surfaceId: SurfaceId, content: string): void; take(surfaceId: SurfaceId): string | null; clear(surfaceId: SurfaceId): void; clearAll(): void }`.
- `stash` of empty/whitespace-only content clears rather than stores. `take` reads without removing (re-acquiring twice must yield the same draft).

- [ ] **Step 1: Write the failing test**

Create `core/lib/editor/warm/__tests__/draft-store.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { createDraftStore } from '../draft-store'

/**
 * With one shared editor, a half-typed comment no longer survives in a mounted
 * composer — the instance moves to whatever the user tapped. The draft store is
 * where that text lives instead: stashed on release, re-seeded on acquire.
 *
 * Scoped to the composer and to the life of the open card. CardDetail is
 * already keyed on the card id at both mount sites, so a card switch drops the
 * store with the subtree and there is no eviction policy to get wrong.
 */
describe('draft store', () => {
    it('has nothing for an untouched surface', () => {
        expect(createDraftStore().take('composer:card1')).toBeNull()
    })

    it('returns what was stashed', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'half a thought')
        expect(drafts.take('composer:card1')).toBe('half a thought')
    })

    /** Re-acquiring twice without typing must not lose the draft. */
    it('does not consume the draft on read', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'still here')
        drafts.take('composer:card1')
        expect(drafts.take('composer:card1')).toBe('still here')
    })

    it('keeps drafts separate per surface', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'one')
        drafts.stash('composer:card2', 'two')
        expect(drafts.take('composer:card1')).toBe('one')
    })

    it('replaces an earlier draft for the same surface', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'first')
        drafts.stash('composer:card1', 'second')
        expect(drafts.take('composer:card1')).toBe('second')
    })

    /**
     * An empty editor is not a draft. Stashing one would re-seed an empty
     * string over the placeholder and make the composer look broken.
     */
    it('treats empty content as no draft', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'typed')
        drafts.stash('composer:card1', '   \n  ')
        expect(drafts.take('composer:card1')).toBeNull()
    })

    it('clears on commit', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'sent now')
        drafts.clear('composer:card1')
        expect(drafts.take('composer:card1')).toBeNull()
    })

    it('clears everything when the card closes', () => {
        const drafts = createDraftStore()
        drafts.stash('composer:card1', 'one')
        drafts.stash('composer:card2', 'two')
        drafts.clearAll()
        expect(drafts.take('composer:card1')).toBeNull()
        expect(drafts.take('composer:card2')).toBeNull()
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/warm/__tests__/draft-store.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement the draft store**

Create `core/lib/editor/warm/draft-store.ts`:

```ts
import type { SurfaceId } from './warm-editor-store'

export interface DraftStore {
    /** Keep uncommitted text. Empty content clears rather than stores. */
    stash(surfaceId: SurfaceId, content: string): void
    /** Read without consuming — re-acquiring twice must yield the same draft. */
    take(surfaceId: SurfaceId): string | null
    clear(surfaceId: SurfaceId): void
    clearAll(): void
}

/**
 * Uncommitted text belonging to a surface that does not hold the warm editor.
 *
 * Only surfaces with no commit semantics need this. An inline comment edit
 * commits on blur, so handing the editor away writes it; the composer has no
 * such commit, and before the warm editor its draft survived only because the
 * composer stayed mounted for the life of the open card.
 */
export function createDraftStore(): DraftStore {
    const drafts = new Map<SurfaceId, string>()
    return {
        stash(surfaceId, content) {
            // An empty editor is not a draft: re-seeding "" would paint over
            // the placeholder and read as a broken composer.
            if (content.trim() === '') drafts.delete(surfaceId)
            else drafts.set(surfaceId, content)
        },
        take: surfaceId => drafts.get(surfaceId) ?? null,
        clear: surfaceId => void drafts.delete(surfaceId),
        clearAll: () => drafts.clear(),
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/warm/__tests__/draft-store.test.ts
```

Expected: PASS (8 tests).

- [ ] **Step 5: Commit**

```bash
git add core/lib/editor/warm/draft-store.ts core/lib/editor/warm/__tests__/draft-store.test.ts
git commit -m "feat(editor): add the composer draft store"
```

---

### Task 6: The warm host and acquire hook

**Files:**
- Create: `core/lib/editor/warm/WarmEditorHost.native.tsx`
- Create: `core/lib/editor/warm/WarmEditorHost.web.tsx`
- Create: `core/lib/editor/warm/use-warm-editor.native.ts`
- Create: `core/lib/editor/warm/use-warm-editor.web.ts`
- Create: `core/lib/editor/warm/use-warm-editor.d.ts`
- Create: `core/lib/editor/warm/index.ts`
- Modify: `core/lib/editor/rich/use-rich-editor.native.tsx` (accept a caller-supplied generation)
- Modify: `core/package.json` (exports entry)
- Test: `core/lib/editor/warm/__tests__/warm-context.test.tsx`

**Interfaces:**
- Consumes: `createWarmEditorStore`, `SurfaceId` (Task 4); `createDraftStore` (Task 5); `shouldPostInit` (Task 3).
- Produces:
  - `<WarmEditorHost options={UseRichEditorOptions}>{children}</WarmEditorHost>` — mounts one offscreen WebView and provides the warm context.
  - `useWarmEditor(surfaceId: SurfaceId, options: UseRichEditorOptions): WarmEditorLease`
  - `interface WarmEditorLease { isWarm: boolean; acquire(): void; release(): void; result: EditorResult | null }` — `result` is non-null only while this surface holds the instance. `isWarm` is false on web and whenever no host is mounted, telling the consumer to mount its own editor.
  - `useDraftStore(): DraftStore`

**Web behavior:** `WarmEditorHost.web.tsx` renders `children` and provides a context whose `isWarm` is always false. Web builds Tiptap in-process with no cold start, so there is nothing to warm; consumers fall through to their own `useRichEditor`.

- [ ] **Step 1: Write the failing test**

Create `core/lib/editor/warm/__tests__/warm-context.test.tsx`:

```tsx
// @vitest-environment happy-dom
import { render } from '@testing-library/react'
import { Text } from 'react-native'
import { describe, expect, it } from 'vitest'
import { WarmEditorHost } from '../WarmEditorHost.web'
import { useWarmEditor } from '../use-warm-editor.web'

/**
 * Web has no WebView and no cold start, so there is nothing to warm. The host
 * still renders its children and the hook still answers — reporting isWarm
 * false so the consumer mounts its own editor. That keeps LazyEditor's call
 * sites identical on both platforms.
 */
function Probe() {
    const lease = useWarmEditor('composer:card1', {})
    return <Text>{lease.isWarm ? 'warm' : 'cold'}</Text>
}

describe('warm editor on web', () => {
    it('renders children rather than swallowing the tree', () => {
        const { getByText } = render(
            <WarmEditorHost options={{}}>
                <Text>content</Text>
            </WarmEditorHost>
        )
        expect(getByText('content')).toBeTruthy()
    })

    it('reports cold, so consumers mount their own editor', () => {
        const { getByText } = render(
            <WarmEditorHost options={{}}>
                <Probe />
            </WarmEditorHost>
        )
        expect(getByText('cold')).toBeTruthy()
    })

    it('reports cold with no host mounted at all', () => {
        const { getByText } = render(<Probe />)
        expect(getByText('cold')).toBeTruthy()
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/warm/__tests__/warm-context.test.tsx
```

Expected: FAIL — modules not found.

- [ ] **Step 3: Write the shared types and web implementations**

Create `core/lib/editor/warm/use-warm-editor.d.ts`:

```ts
import type { UseRichEditorOptions } from '../rich/options'
import type { EditorResult } from '../types'
import type { SurfaceId } from './warm-editor-store'

export interface WarmEditorLease {
    /** False on web, and whenever no warm host is mounted. */
    isWarm: boolean
    acquire(): void
    release(): void
    /** Non-null only while this surface holds the instance. */
    result: EditorResult | null
}

export declare function useWarmEditor(
    surfaceId: SurfaceId,
    options: UseRichEditorOptions
): WarmEditorLease
```

Create `core/lib/editor/warm/use-warm-editor.web.ts`:

```ts
import type { UseRichEditorOptions } from '../rich/options'
import type { SurfaceId } from './warm-editor-store'
import type { WarmEditorLease } from './use-warm-editor.d'

/**
 * Web builds Tiptap in-process, so there is no browser cold start to hide and
 * nothing to keep warm. The lease exists only so LazyEditor's call sites are
 * identical on both platforms; reporting cold sends the consumer down its own
 * useRichEditor path.
 */
export function useWarmEditor(_surfaceId: SurfaceId, _options: UseRichEditorOptions): WarmEditorLease {
    return {
        isWarm: false,
        acquire: () => {},
        release: () => {},
        result: null,
    }
}
```

Create `core/lib/editor/warm/WarmEditorHost.web.tsx`:

```tsx
import type { ReactNode } from 'react'
import type { UseRichEditorOptions } from '../rich/options'

/**
 * A pass-through on web. Kept so a package mounts the same component on both
 * platforms and the e2e suite walks the same tree.
 */
export function WarmEditorHost({
    children,
}: {
    options: UseRichEditorOptions
    children: ReactNode
}) {
    return <>{children}</>
}
```

Create `core/lib/editor/warm/index.ts`:

```ts
export { createDraftStore, type DraftStore } from './draft-store'
export { useWarmEditor, type WarmEditorLease } from './use-warm-editor'
export { WarmEditorHost } from './WarmEditorHost'
export { createWarmEditorStore, type SurfaceId, type WarmEditorStore } from './warm-editor-store'
```

- [ ] **Step 4: Run test to verify it passes**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/lib/editor/warm/__tests__/warm-context.test.tsx
```

Expected: PASS (3 tests).

- [ ] **Step 5: Let `useRichEditor` take a caller-supplied generation**

The warm hook drives reconfiguration by bumping the generation, so it must be an input rather than the hard-coded `0` from Task 1.

In `core/lib/editor/rich/options.ts`, add to `UseRichEditorOptions`:

```ts
    /**
     * Configuration generation, for a reused (warm) editor.
     *
     * Bumping this re-initializes the underlying WebView page in place — a full
     * stage-two reconstruction — instead of mounting a new one. Omitted by
     * ordinary consumers, which mount and destroy their own editor.
     */
    generation?: number
```

In `core/lib/editor/rich/use-rich-editor.native.tsx`, destructure `generation = 0` from `options` alongside the other fields, replace the literal `generation: 0` in the `initPayload` memo with `generation`, and add `generation` to that memo's dependency array.

- [ ] **Step 6: Write the native host and hook**

Create `core/lib/editor/warm/WarmEditorHost.native.tsx`:

```tsx
import { createContext, type ReactNode, useContext, useMemo, useRef, useSyncExternalStore } from 'react'
import { View } from 'react-native'
import { useRichEditor } from '../rich'
import type { UseRichEditorOptions } from '../rich/options'
import type { EditorResult } from '../types'
import { createDraftStore, type DraftStore } from './draft-store'
import { createWarmEditorStore, type SurfaceId, type WarmEditorStore } from './warm-editor-store'

interface WarmContextValue {
    store: WarmEditorStore
    drafts: DraftStore
    result: EditorResult
    setOptions: (options: UseRichEditorOptions) => void
}

const WarmContext = createContext<WarmContextValue | null>(null)

export function useWarmContext(): WarmContextValue | null {
    return useContext(WarmContext)
}

/**
 * Keeps ONE WebView editor booted and parked, ready to be handed to whichever
 * surface the user starts editing.
 *
 * The cost this removes is measured: creating a WebView editor is a browser cold
 * start plus a 0.86 MB bundle parse — 1135 ms of the 1186 ms an edit used to
 * take. That work is configuration-independent and finishes before the init
 * payload is even sent, so a page booted in advance can be reconfigured for a
 * new surface in the remaining ~34 ms.
 *
 * Mount this where the package's editing surfaces live — for cards, the route
 * layout, so it warms on entering the section and stays warm across boards and
 * cards. NOT a manifest `provider`: PackageProviderWrapper builds its chain at
 * module load and wraps the whole app, which would boot a WebView at launch for
 * anyone who has the package installed but never opens it.
 */
export function WarmEditorHost({
    options,
    children,
}: {
    options: UseRichEditorOptions
    children: ReactNode
}) {
    const storeRef = useRef<WarmEditorStore | null>(null)
    if (storeRef.current === null) storeRef.current = createWarmEditorStore()
    const store = storeRef.current

    const draftsRef = useRef<DraftStore | null>(null)
    if (draftsRef.current === null) draftsRef.current = createDraftStore()
    const drafts = draftsRef.current

    // The live configuration, held in a ref and read through the store's
    // generation: putting it in state would re-render this provider (and so the
    // whole section) on every handover.
    const optionsRef = useRef<UseRichEditorOptions>(options)
    const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot)

    const result = useRichEditor({
        ...optionsRef.current,
        generation: snapshot.generation,
        // Never on acquire: the surface decides when to take the caret, and
        // focusing a parked editor would open the keyboard over a card nobody
        // is editing.
        autofocus: false,
    })

    const value = useMemo<WarmContextValue>(
        () => ({
            store,
            drafts,
            result,
            setOptions: next => {
                optionsRef.current = next
            },
        }),
        [store, drafts, result]
    )

    return (
        <WarmContext.Provider value={value}>
            {children}
            <WarmEditorViewport isParked={snapshot.holder === null}>
                <result.EditorComponent />
            </WarmEditorViewport>
        </WarmContext.Provider>
    )
}

/**
 * Where the WebView lives while nobody is editing.
 *
 * Deliberately NOT `display: none` or a zero-size box: an unlaid-out WebView may
 * never paint on iOS, and a page that never paints never finishes booting —
 * which would defeat the entire point of warming it. It is therefore kept at a
 * real size and pushed off-viewport instead.
 */
function WarmEditorViewport({ isParked, children }: { isParked: boolean; children: ReactNode }) {
    if (!isParked) return null
    return (
        <View
            style={{ position: 'absolute', left: -10000, top: 0, width: 320, height: 200 }}
            pointerEvents="none"
            accessibilityElementsHidden
            importantForAccessibility="no-hide-descendants"
        >
            {children}
        </View>
    )
}
```

Create `core/lib/editor/warm/use-warm-editor.native.ts`:

```ts
import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react'
import type { UseRichEditorOptions } from '../rich/options'
import type { SurfaceId } from './warm-editor-store'
import type { WarmEditorLease } from './use-warm-editor.d'
import { useWarmContext } from './WarmEditorHost.native'

/**
 * Lease the package's single warm editor for one surface.
 *
 * `isWarm` false means no host is mounted; the consumer must mount its own
 * editor and pay the cold start. Warm is an optimization, never a correctness
 * dependency — a fault here degrades to the previous behavior, not a broken
 * editor.
 */
export function useWarmEditor(
    surfaceId: SurfaceId,
    options: UseRichEditorOptions
): WarmEditorLease {
    const warm = useWarmContext()
    const optionsRef = useRef(options)
    optionsRef.current = options

    const snapshot = useSyncExternalStore(
        warm?.store.subscribe ?? noopSubscribe,
        warm?.store.getSnapshot ?? emptySnapshot
    )

    const acquire = useCallback(() => {
        if (!warm) return
        warm.setOptions(optionsRef.current)
        warm.store.acquire(surfaceId)
    }, [warm, surfaceId])

    const release = useCallback(() => {
        warm?.store.release(surfaceId)
    }, [warm, surfaceId])

    // A surface can be unmounted while still holding the instance (the card
    // closes mid-edit). Without this the store would keep a holder that no
    // longer exists and the editor would never park.
    useEffect(() => release, [release])

    const isHolder = warm != null && snapshot.holder === surfaceId
    return {
        isWarm: warm != null,
        acquire,
        release,
        result: isHolder ? warm.result : null,
    }
}

const EMPTY = { holder: null, generation: 0 }
const noopSubscribe = () => () => {}
const emptySnapshot = () => EMPTY
```

- [ ] **Step 7: Re-seed the markdown fallback on acquire**

`MarkdownWebViewHost.lastKnown` is what `getMarkdown` returns when the WebView
does not answer in time (`markdown-webview-host.ts:48`), and that value is
persisted. Across a handover it would otherwise still hold the *previous*
surface's text, so a timeout during a save would write one comment's words over
another's.

In `use-rich-editor.native.tsx`, the markdown host is created once per mount
with a one-time seed. Add an effect that re-seeds whenever the generation
changes:

```ts
    // A warm editor is reused across surfaces, so the timeout fallback has to
    // follow the current one. Without this a slow getMarkdown during a handover
    // resolves with the PREVIOUS surface's text — and that value gets saved.
    useEffect(() => {
        if (contentFormat !== 'markdown') return
        markdownHost.seed(initialContent ?? '')
    }, [markdownHost, contentFormat, initialContent, generation])
```

- [ ] **Step 8: Add the package export**

In `core/package.json`, alongside the other `./lib/editor/*` entries, confirm the existing wildcard covers `./lib/editor/warm`. If the exports map lists editor subpaths explicitly, add:

```json
        "./lib/editor/warm": "./lib/editor/warm/index.ts",
```

- [ ] **Step 9: Typecheck and run the editor suite**

```sh
cd ~/code/tinycld/tinycld && pnpm exec tinycld-pkg typecheck && pnpm exec vitest run core/lib/editor
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add core/lib/editor/warm core/lib/editor/rich/options.ts \
        core/lib/editor/rich/use-rich-editor.native.tsx core/package.json
git commit -m "feat(editor): add the warm editor host and acquire hook"
```

---

### Task 7: LazyEditor

**Files:**
- Create: `core/components/editor/LazyEditor.tsx`
- Create: `core/components/editor/commit-policy.ts`
- Test: `core/components/editor/__tests__/commit-policy.test.ts`

**Interfaces:**
- Consumes: `useWarmEditor`, `WarmEditorLease` (Task 6).
- Produces:
  - `shouldCommitOnBlur(state: CommitState): boolean`
  - `interface CommitState { commitOnBlur: boolean; hasFocused: boolean; isSettled: boolean; isDialogOpen: boolean }`
  - `isNoOpEdit(next: string, baseline: string): boolean`
  - `<LazyEditor ... />` with props: `readView: ReactNode`, `value: string`, `contentFormat: 'markdown' | 'html'`, `editorOptions: UseRichEditorOptions`, `surfaceId: SurfaceId`, `canEdit: boolean`, `commitOnBlur?: boolean`, `onCommit: (content: string) => void`, `onCancel?: () => void`, `renderEditor: (slots: LazyEditorSlots) => ReactNode`, `testID?: string`.
  - `interface LazyEditorSlots { EditorComponent: ComponentType; commands: EditorCommands; toolbarState: EditorToolbarState; submit: () => void; cancel: () => void; setDialogOpen: (open: boolean) => void }`

`renderEditor` keeps chrome (toolbars, Save/Cancel, dialogs) with the consumer, exactly as `readView` keeps rendering with the consumer. `LazyEditor` owns only the swap and the commit rules.

- [ ] **Step 1: Write the failing test**

Create `core/components/editor/__tests__/commit-policy.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { isNoOpEdit, shouldCommitOnBlur } from '../commit-policy'

/**
 * Each of these rules was found on a device, and each protects a write. They
 * live in core so a second consumer inherits the fixes rather than
 * rediscovering them.
 */
describe('commit on blur', () => {
    const base = { commitOnBlur: true, hasFocused: true, isSettled: false, isDialogOpen: false }

    it('commits when a focused session loses focus', () => {
        expect(shouldCommitOnBlur(base)).toBe(true)
    })

    /**
     * The composer has no blur-commit: leaving it must not post a comment.
     */
    it('never commits a surface that does not commit on blur', () => {
        expect(shouldCommitOnBlur({ ...base, commitOnBlur: false })).toBe(false)
    })

    /**
     * The editor opens with autofocus at a placeholder height, and until the
     * page reports its real content height the caret can land outside the
     * visible box and blur immediately. Since a blur COMMITS, that saved a
     * comment nobody had touched.
     */
    it('never commits before the session has held focus', () => {
        expect(shouldCommitOnBlur({ ...base, hasFocused: false })).toBe(false)
    })

    /**
     * Save and the blur-commit race each other: pressing Save blurs the editor.
     * Both writing would submit twice.
     */
    it('does not commit again once the session has settled', () => {
        expect(shouldCommitOnBlur({ ...base, isSettled: true })).toBe(false)
    })

    /**
     * The image picker and link prompt both steal focus. Treating that as the
     * end of the session unmounts the surface the picked image is about to be
     * inserted into.
     */
    it('does not commit while a dialog holds the focus', () => {
        expect(shouldCommitOnBlur({ ...base, isDialogOpen: true })).toBe(false)
    })
})

describe('no-op edits', () => {
    it('treats an unchanged value as nothing to write', () => {
        expect(isNoOpEdit('same text', 'same text')).toBe(true)
    })

    it('ignores surrounding whitespace, which the editor adds on its own', () => {
        expect(isNoOpEdit('  same text\n', 'same text')).toBe(true)
    })

    it('treats a real change as a write', () => {
        expect(isNoOpEdit('new text', 'old text')).toBe(false)
    })

    /** An emptied editor is a deletion the caller must decide about, not a no-op. */
    it('does not call an emptied editor unchanged', () => {
        expect(isNoOpEdit('', 'had content')).toBe(false)
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/components/editor/__tests__/commit-policy.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 3: Implement the policy**

Create `core/components/editor/commit-policy.ts`:

```ts
export interface CommitState {
    /** Does this surface write on focus loss? An edit does; a composer does not. */
    commitOnBlur: boolean
    /** Has the session ever actually held focus? */
    hasFocused: boolean
    /** Has it already committed or cancelled? */
    isSettled: boolean
    /** Is a dialog (image picker, link prompt) holding the focus? */
    isDialogOpen: boolean
}

/**
 * Whether a blur should write.
 *
 * Every clause here exists because of a bug found on a device, and the stakes
 * are asymmetric: a missed commit loses an edit the user can redo, while a
 * spurious one writes text nobody typed.
 */
export function shouldCommitOnBlur(state: CommitState): boolean {
    if (!state.commitOnBlur) return false
    // The mount racing itself, not a person finishing an edit.
    if (!state.hasFocused) return false
    if (state.isSettled) return false
    // A detour inside the session, not the end of it.
    if (state.isDialogOpen) return false
    return true
}

/**
 * Whether a submitted value differs from what the session opened with.
 *
 * An unchanged edit is a cancel rather than a write — EditableText's rule. The
 * baseline is snapshotted at acquire, so a realtime update arriving mid-edit
 * cannot become the comparison target.
 */
export function isNoOpEdit(next: string, baseline: string): boolean {
    return next.trim() === baseline.trim()
}
```

- [ ] **Step 4: Run test to verify it passes**

```sh
cd ~/code/tinycld/tinycld && pnpm exec vitest run core/components/editor/__tests__/commit-policy.test.ts
```

Expected: PASS (9 tests).

- [ ] **Step 5: Implement LazyEditor**

Create `core/components/editor/LazyEditor.tsx`:

```tsx
import { type ComponentType, type ReactNode, useCallback, useRef, useState } from 'react'
import { Pressable } from 'react-native'
import { captureException } from '../../lib/errors'
import { useRichEditor } from '../../lib/editor/rich'
import type { UseRichEditorOptions } from '../../lib/editor/rich/options'
import type { EditorCommands, EditorHandle, EditorToolbarState } from '../../lib/editor/types'
import { useWarmEditor } from '../../lib/editor/warm'
import type { SurfaceId } from '../../lib/editor/warm/warm-editor-store'
import { isNoOpEdit, shouldCommitOnBlur } from './commit-policy'

export interface LazyEditorSlots {
    EditorComponent: ComponentType
    commands: EditorCommands
    toolbarState: EditorToolbarState
    submit: () => void
    cancel: () => void
    /** Tell the swap a dialog holds the focus, so a blur is not the session ending. */
    setDialogOpen: (open: boolean) => void
}

export interface LazyEditorProps {
    /** Shown while idle. The consumer's component — core never interprets content. */
    readView: ReactNode
    /** Current persisted content, in `contentFormat`. */
    value: string
    contentFormat: 'markdown' | 'html'
    editorOptions: UseRichEditorOptions
    surfaceId: SurfaceId
    canEdit: boolean
    /** True for an edit of existing content; false for a composer. */
    commitOnBlur?: boolean
    onCommit: (content: string) => void
    /** Absent when there is nothing to revert (a collaborative description). */
    onCancel?: () => void
    /** The consumer's chrome around the editing surface. */
    renderEditor: (slots: LazyEditorSlots) => ReactNode
    testID?: string
    accessibilityLabel?: string
}

/**
 * Renders content, and swaps in a real editor when someone starts editing.
 *
 * Two jobs, both previously hand-rolled per consumer:
 *
 *  - **The swap.** The read view IS the boot placeholder, so an edit never
 *    shows an empty box while the editor initializes. On native the editor is
 *    the package's warm instance when one is available, which turns a ~1135 ms
 *    cold start into a ~34 ms reconfiguration.
 *  - **The commit rules.** See commit-policy.ts — each clause protects a write,
 *    and a blur COMMITS, so getting them wrong loses or invents user text.
 *
 * Deliberately NOT format-aware. `readView` is the consumer's, content crosses
 * as an opaque string, and `contentFormat` only selects which channel to read
 * back through — so mail's HTML surfaces use this exactly as cards' markdown
 * ones do.
 */
export function LazyEditor({
    readView,
    value,
    contentFormat,
    editorOptions,
    surfaceId,
    canEdit,
    commitOnBlur = false,
    onCommit,
    onCancel,
    renderEditor,
    testID,
    accessibilityLabel = 'Edit',
}: LazyEditorProps) {
    const [isEditing, setIsEditing] = useState(false)
    const [isDialogOpen, setIsDialogOpen] = useState(false)
    // The revert/no-op baseline, snapshotted when the session opens so a
    // realtime update mid-edit cannot become the comparison target.
    const baselineRef = useRef(value)
    const hasFocusedRef = useRef(false)
    const settledRef = useRef(false)

    const lease = useWarmEditor(surfaceId, {
        ...editorOptions,
        contentFormat,
        initialContent: value,
    })

    const startEditing = useCallback(() => {
        baselineRef.current = value
        hasFocusedRef.current = false
        settledRef.current = false
        lease.acquire()
        setIsEditing(true)
    }, [lease, value])

    const endSession = useCallback(() => {
        settledRef.current = true
        lease.release()
        setIsEditing(false)
    }, [lease])

    if (!isEditing) {
        if (!canEdit) return <>{readView}</>
        return (
            <Pressable
                testID={testID}
                onPress={startEditing}
                accessibilityRole="button"
                accessibilityLabel={accessibilityLabel}
            >
                {readView}
            </Pressable>
        )
    }

    return (
        <LazyEditorSession
            lease={lease}
            value={value}
            contentFormat={contentFormat}
            editorOptions={editorOptions}
            commitOnBlur={commitOnBlur}
            isDialogOpen={isDialogOpen}
            setDialogOpen={setIsDialogOpen}
            baselineRef={baselineRef}
            hasFocusedRef={hasFocusedRef}
            settledRef={settledRef}
            onCommit={onCommit}
            onCancel={onCancel}
            endSession={endSession}
            renderEditor={renderEditor}
        />
    )
}

/**
 * The live editing session, split out so the fallback `useRichEditor` below is
 * only ever called while editing — a hook cannot sit behind the swap's branch.
 */
function LazyEditorSession({
    lease,
    value,
    contentFormat,
    editorOptions,
    commitOnBlur,
    isDialogOpen,
    setDialogOpen,
    baselineRef,
    hasFocusedRef,
    settledRef,
    onCommit,
    onCancel,
    endSession,
    renderEditor,
}: {
    lease: ReturnType<typeof useWarmEditor>
    value: string
    contentFormat: 'markdown' | 'html'
    editorOptions: UseRichEditorOptions
    commitOnBlur: boolean
    isDialogOpen: boolean
    setDialogOpen: (open: boolean) => void
    baselineRef: { current: string }
    hasFocusedRef: { current: boolean }
    settledRef: { current: boolean }
    onCommit: (content: string) => void
    onCancel?: () => void
    endSession: () => void
    renderEditor: (slots: LazyEditorSlots) => ReactNode
}) {
    const readContent = useCallback(
        async (editor: EditorHandle): Promise<string> =>
            contentFormat === 'markdown' ? ((await editor.getMarkdown?.()) ?? '') : editor.getHTML(),
        [contentFormat]
    )

    const submitRef = useRef<() => void>(() => {})
    const blurRef = useRef<() => void>(() => {})

    // The warm instance when one is available; otherwise this surface mounts
    // its own and pays the cold start. Warm is an optimization, never a
    // correctness dependency.
    const own = useRichEditor({
        ...editorOptions,
        contentFormat,
        initialContent: value,
        autofocus: true,
        onFocus: () => {
            hasFocusedRef.current = true
        },
        onBlur: () => blurRef.current(),
        onSubmitShortcut: () => submitRef.current(),
    })
    const active = lease.result ?? own

    const submit = useCallback(() => {
        if (settledRef.current) return
        void (async () => {
            let content: string
            try {
                content = (await readContent(active.editor)).trim()
            } catch (err) {
                captureException('editor.lazy.readContent', err)
                return
            }
            if (onCancel && isNoOpEdit(content, baselineRef.current)) {
                settledRef.current = true
                endSession()
                onCancel()
                return
            }
            settledRef.current = true
            onCommit(content)
            endSession()
        })()
    }, [active.editor, readContent, onCommit, onCancel, endSession, baselineRef, settledRef])

    submitRef.current = submit
    blurRef.current = () => {
        if (
            shouldCommitOnBlur({
                commitOnBlur,
                hasFocused: hasFocusedRef.current,
                isSettled: settledRef.current,
                isDialogOpen,
            })
        ) {
            submit()
            return
        }
        // A surface with no blur-commit still ends its session on blur — the
        // caller decides whether that text was worth keeping.
        if (!commitOnBlur && hasFocusedRef.current && !isDialogOpen) endSession()
    }

    const cancel = useCallback(() => {
        settledRef.current = true
        endSession()
        onCancel?.()
    }, [endSession, onCancel, settledRef])

    return (
        <>
            {renderEditor({
                EditorComponent: active.EditorComponent,
                commands: active.commands,
                toolbarState: active.toolbarState,
                submit,
                cancel,
                setDialogOpen,
            })}
        </>
    )
}
```

- [ ] **Step 6: Typecheck**

```sh
cd ~/code/tinycld/tinycld && pnpm exec tinycld-pkg typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add core/components/editor
git commit -m "feat(editor): add LazyEditor with core-owned commit semantics"
```

---

### Task 8: Mount the warm host in cards

**Files:**
- Modify: `cards/tinycld/cards/screens/_layout.tsx`

**Interfaces:**
- Consumes: `WarmEditorHost` (Task 6).
- Produces: a warm instance available to every cards screen.

- [ ] **Step 1: Mount the host**

Replace `cards/tinycld/cards/screens/_layout.tsx` with:

```tsx
import { WarmEditorHost } from '@tinycld/core/lib/editor/warm'
import { Stack } from 'expo-router'

/**
 * Boots one editor WebView on entering Cards and keeps it warm for the section.
 *
 * Creating an editor is a browser cold start plus a 0.86 MB bundle parse — 1135
 * of the 1186 ms an edit used to take, all of it before any configuration is
 * applied. Warming it here means the first description or comment edit pays only
 * the reconfiguration.
 *
 * Here rather than in the manifest's `provider`: that chain wraps the whole app
 * and is built at module load, so it would boot a WebView at launch for anyone
 * who has cards installed and never opens it.
 */
export default function CardsLayout() {
    return (
        <WarmEditorHost options={{ contentFormat: 'markdown', minHeight: 72 }}>
            <Stack screenOptions={{ headerShown: false }} />
        </WarmEditorHost>
    )
}
```

- [ ] **Step 2: Typecheck cards**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg typecheck
```

Expected: PASS.

- [ ] **Step 3: Verify the app still boots and cards renders**

```sh
cd ~/code/tinycld/tinycld && pnpm run dev
```

Open Cards in the browser. Expected: the board renders as before (web's host is a pass-through). Stop the dev server.

- [ ] **Step 4: Commit**

```bash
cd ~/code/tinycld/cards
git add tinycld/cards/screens/_layout.tsx
git commit -m "feat(cards): keep one editor warm for the cards section"
```

---

### Task 9: Move the description onto LazyEditor

**Files:**
- Modify: `cards/tinycld/cards/components/detail/CardDetail.tsx:302-360`
- Modify: `cards/tinycld/cards/components/detail/DescriptionEditor.tsx`

**Interfaces:**
- Consumes: `LazyEditor`, `LazyEditorSlots` (Task 7).
- Produces: description editing through the warm instance.

The description passes **no `onCancel`**: every keystroke is already shared through Yjs and flushed by the server, so revert and save have nowhere to live. Its surface id is `description:<cardId>`.

- [ ] **Step 1: Replace the hand-rolled swap**

In `CardDetail.tsx`, the `isEditingDescription` state, the `isEditingCollab` branch, and `DescriptionReadView`'s press wiring are replaced by a `LazyEditor`. Keep `DescriptionReadView` itself — it becomes the `readView` prop, minus its `Pressable` (LazyEditor supplies the press target). Keep `useDescriptionEditor` for the chrome it builds, driving it from `LazyEditorSlots` rather than owning the editor.

Pass:

```tsx
<LazyEditor
    surfaceId={`description:${card.id}`}
    readView={<DescriptionReadView description={card.description} projectId={projectId} />}
    value={card.description ?? ''}
    contentFormat="markdown"
    canEdit={canEdit && canEditDoc}
    editorOptions={{
        placeholder: 'Add a description — what does done look like?',
        characterLimit: DESCRIPTION_LIMIT,
        minHeight: 72,
        containerClassName: 'min-h-[72px]',
        triggers: mention.triggers,
        overlayKey: mention.overlayKey,
        collab: doc
            ? { document: doc, field: `card:${card.id}`, awareness: awareness ?? undefined, user: identity ?? undefined }
            : undefined,
    }}
    onCommit={value => updateCard.mutate({ cardId: card.id, description: value })}
    renderEditor={slots => descriptionChrome(slots)}
    testID="cards-description-read"
    accessibilityLabel="Edit description"
/>
```

- [ ] **Step 2: Run the cards unit suite**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg test
```

Expected: PASS. Fix any failure at its source.

- [ ] **Step 3: Run the description e2e specs**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg test:e2e -- card-description.spec.ts card-description-collab.spec.ts card-description-toolbar.spec.ts card-description-images.spec.ts
```

Expected: PASS. These are the regression net for the swap's layout neutrality — a failure means the swap moved something, and the fix is in the layout, never in the assertion.

- [ ] **Step 4: Commit**

```bash
cd ~/code/tinycld/cards
git add tinycld/cards/components/detail/CardDetail.tsx tinycld/cards/components/detail/DescriptionEditor.tsx
git commit -m "feat(cards): edit descriptions through LazyEditor"
```

---

### Task 10: Move comments onto LazyEditor, with the composer draft store

**Files:**
- Modify: `cards/tinycld/cards/components/detail/DetailActivity.tsx:150-160`
- Modify: `cards/tinycld/cards/components/detail/CommentEditor.tsx`
- Modify: `cards/tinycld/cards/components/detail/CommentComposer.tsx`

**Interfaces:**
- Consumes: `LazyEditor` (Task 7), `useDraftStore` via the warm context (Task 6).
- Produces: comment editing and composing through the warm instance.

Surface ids: `comment:<commentId>` for an inline edit, `composer:<cardId>` for the composer.

Inline edits pass `commitOnBlur` — that is today's behavior and switching away must keep committing. The composer does not; its text is stashed instead.

- [ ] **Step 1: Move the inline editor onto LazyEditor**

In `DetailActivity.tsx`, replace the `isEditing ? <InlineCommentEditor .../> : <author line + body>` swap with a `LazyEditor` whose `readView` is the existing author line + `MarkdownText` body, `commitOnBlur` is true, `onCommit` calls `onSaveEdit`, and `onCancel` calls `onCancelEdit`. `CommentEditor.tsx`'s `useCommentEditorCore` loses its blur/settled/hasFocused refs — those now live in `commit-policy.ts` — and keeps only the chrome (toolbar, Save/Cancel, dialogs) as a `renderEditor` callback.

- [ ] **Step 2: Wire the composer draft store**

In `CommentComposer.tsx`, the composer becomes a `LazyEditor` with `surfaceId={`composer:${cardId}`}` and no `commitOnBlur`. On release, stash the editor's content; on acquire, seed it. Read the draft store from the warm context:

```tsx
const drafts = useDraftStore()
const draft = drafts.take(`composer:${cardId}`)
```

Seed `value={draft ?? ''}`, and clear on a successful send:

```tsx
onCommit={body => {
    drafts.clear(`composer:${cardId}`)
    onSubmit(body)
}}
```

- [ ] **Step 3: Run the cards unit suite**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg test
```

Expected: PASS.

- [ ] **Step 4: Run the comment e2e specs**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg test:e2e -- comment-editing.spec.ts comment-markdown.spec.ts card-mentions.spec.ts
```

Expected: PASS. `comment-editing.spec` carries the ±2px layout anchor; a failure means the swap moved the prose and the fix is the layout.

- [ ] **Step 5: Run the full cards check**

```sh
cd ~/code/tinycld/cards && pnpm exec tinycld-pkg check
```

Expected: PASS (biome + tsc + vitest).

- [ ] **Step 6: Commit**

```bash
cd ~/code/tinycld/cards
git add tinycld/cards/components/detail/
git commit -m "feat(cards): edit comments through LazyEditor with a composer draft store"
```

---

### Task 11: Verify on device and document

**Files:**
- Modify: `cards/help/<topic>.md` (only if user-visible behavior changed)
- Delete: `HANDOFF-editor-webview-cost.md` (superseded by the spec and this plan)

**Interfaces:**
- Consumes: everything above.

The measurement that motivated this work must be re-taken. The four `__DEV__` marks in `use-webview-editor.tsx` are the instrument and stay in place.

- [ ] **Step 1: Run the app on the iOS simulator**

```sh
cd ~/code/tinycld/tinycld && pnpm run dev
```

Open the iOS simulator and navigate into Cards.

- [ ] **Step 2: Confirm the warm boot happens on section entry**

Expected in the console on entering Cards, with no card open:

```
[editor.mount] mounted (t0)
[editor.mount] page-ready    ~1100 ms
```

`page-ready` firing here rather than on first edit is the whole point — the cost moved off the interaction.

- [ ] **Step 3: Confirm a warm acquire is fast**

Open a card and tap the description. Expected: `init-sent` and `first-height` marks only, **no new `page-ready`**, and the total from tap to caret under ~100 ms. If `page-ready` appears, the surface fell back to its own editor — check that `WarmEditorHost` is mounted above the screen.

- [ ] **Step 4: Verify the handover cases by hand**

- [ ] Tap description → type → tap a comment's Edit. The description's text persists (Yjs flushed it) and the comment editor opens warm.
- [ ] Open the composer → type a partial draft → tap a comment's Edit → cancel that edit → return to the composer. **The draft is still there.**
- [ ] Edit a comment → tap elsewhere without pressing Save. The edit **commits**, as it does today.
- [ ] Open a card, tap description, close the card mid-edit. No crash, and the editor parks (the next card's first edit is still fast).

- [ ] **Step 5: Check the help topic**

Read `cards/help/` for any topic describing description or comment editing. If the visible behavior is unchanged — it should be, apart from speed — no edit is needed. If a topic mentions waiting or slowness, update it.

- [ ] **Step 6: Remove the superseded handoff**

```bash
cd ~/code/tinycld && git rm HANDOFF-editor-webview-cost.md
```

- [ ] **Step 7: Run the full ecosystem checks**

```sh
cd ~/code/tinycld/tinycld && pnpm run checks && pnpm run pkg:check
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
cd ~/code/tinycld
git add -A
git commit -m "docs: retire the editor webview cost handoff"
```

---

## Self-Review

**Spec coverage:**

| Spec requirement | Task |
|---|---|
| Re-sendable init with a generation | 1 |
| Page rebuilds stage two on a bump; `APP_PARK` | 1, 2 |
| Host posts per generation, not once per mount | 3 |
| Single instance, not a pool | 4 |
| Handover transfers; stale release ignored | 4 |
| Host-side relays: no teardown needed | Verified — see the note below |
| `MarkdownWebViewHost` re-seeded per acquire | 6 (Step 7) |
| `MarkdownWebViewHost` re-seeded per acquire | 6 (Step 7) |
| `WarmEditorHost` offscreen at real size, not `display:none` | 6 |
| `useWarmEditor`; web pass-through | 6 |
| Fallback to a fresh mount | 6, 7 |
| `LazyEditor` owns the swap; `readView` is a prop | 7 |
| Format neutrality via `contentFormat` + opaque content | 7 |
| Commit semantics matrix in core | 7 |
| Composer draft store, composer-scoped | 5, 10 |
| Inline edits commit on switch | 7, 10 |
| Description gets no `onCancel` | 9 |
| Mounted in cards' route layout, not a manifest provider | 8 |
| Device re-measurement | 11 |
| Existing e2e must pass unchanged | 9, 10 |

**Gap examined and resolved:** the spec calls for the host-side Yjs and awareness relays to be rebuilt per acquire. Checking the code, no separate task is needed, and the reason is worth recording so nobody "fixes" it later:

`yjsHost` and `awarenessHost` are memoized on **document identity** (`use-rich-editor.native.tsx:116-143`). In cards that document is the board's — `useBoardPresence.ts:141` returns `room?.doc`, one Y.Doc holding a fragment per card — so it is the *same object* across every card on a board. The relay is therefore correctly shared, and the only thing that varies per acquire is the `field` (`card:<id>`), which rides in the init payload and is consumed when the page rebuilds its own doc in `useCollabDoc`.

The spec's stale-relay concern is real but applies to a different case: a consumer that swaps to a **different** Y.Doc. The existing memo already handles that — a new doc identity rebuilds both hosts and the `useEffect` cleanups destroy the old ones. Nothing to add.

**Placeholder scan:** none — every code step carries real code. Tasks 9 and 10 describe edits to large existing components in prose plus the concrete props; the implementer has the exact file, line range, prop list, and surface-id format.

**Type consistency:** `SurfaceId`, `WarmSnapshot`, `WarmEditorStore`, `DraftStore`, `WarmEditorLease`, `LazyEditorSlots`, `CommitState`, `shouldPostInit`, `reduceInit`, `shouldCommitOnBlur`, `isNoOpEdit` are each defined once and used consistently. `generation` is a `number` throughout.
