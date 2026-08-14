# LazyEditor and the warm editor instance

**Date:** 2026-08-14
**Status:** Design, approved for planning
**Context:** `HANDOFF-editor-webview-cost.md`

## The problem

Tapping to edit a comment on iOS takes ~1.2 s before text appears. The cause is
measured, not guessed: creating a WebView editor is a browser cold start, and
the app creates one per edit.

```
[editor.mount] mounted (t0)
[editor.mount] page-ready    1135 ms   96%
[editor.mount] init-sent     1152 ms    1%
[editor.mount] first-height  1186 ms    3%
```

Everything the host controls after the page reports ready totals 51 ms. The
cost is entirely *creation*, which is why reuse is the fix rather than tuning.

## The finding that shapes the design

**The WebView page already mounts in two stages.** `rich/webview/source/Editor.tsx`
boots, posts `EDITOR_READY`, and renders `null` until the init payload arrives.
Only then does it construct Tiptap.

The 1135 ms is spent entirely in stage one — browser cold start, 0.86 MB bundle
parse, React boot — all of it **independent of any editor's configuration**.
Stage two is part of the 51 ms.

So the mechanism is not something to invent. It is already the page's
architecture. Two things block reuse:

1. The host treats init as one-shot (`initSentRef`, `use-webview-editor.tsx:300`).
2. A WebView's lifetime is tied to a consumer component's mount.

The work is therefore: **let a booted page be re-initialized, and let one WebView
outlive its consumers.**

## What ships

Two pieces in core, plus the cards migration.

### 1. A single warm editor instance

Exactly one WebView, owned by core, booted to `EDITOR_READY` and parked. Not a
pool: nowhere in the app are two editors focused at once, so one suffices.

Acquiring it posts a fresh init payload; the page rebuilds stage two for that
configuration. Cost per acquire is Tiptap construction — the 34 ms figure above
— instead of 1135 ms.

**Re-initialization is a full reconstruction, not a mutation.** This is the
central safety property. `EditorMounted` takes its entire configuration from one
prop, so keying it on an init generation gives a new Tiptap instance, a new
`Y.Doc` (`useCollabDoc`'s `useState` initializer reruns), a new undo stack, and a
new extension set. The handoff doc's leaked-undo-history risk — undoing another
comment's text into this one — cannot occur, because nothing survives the swap.

The alternative, making placeholder / characterLimit / collab individually
settable to avoid rebuilding Tiptap, is rejected: it trades ~30 ms for a class
of state-leak bugs in a surface where **a blur commits**, making leaked state a
data-loss risk rather than a cosmetic one.

### 2. `LazyEditor` — the view/edit swap as a core component

Today `CardDetail` and `DetailActivity` each hand-roll the swap. `LazyEditor`
makes it core's job: render the read view, and on press acquire the warm
instance.

This subsumes the handoff doc's "cheap win" (render existing text while the
editor boots) structurally rather than as a bolted-on placeholder — the read
view *is* what shows during the acquire.

**`LazyEditor` does not own rendering.** It takes the read view as a prop:

```tsx
<LazyEditor
    // Consumer's component. Core never interprets the content.
    readView={<MarkdownText body={description} projectId={projectId} />}

    contentFormat="markdown"        // passed to the init payload
    value={description}

    editorOptions={{ placeholder, characterLimit, triggers, collab }}

    canEdit={canEdit}
    onCommit={body => save(body)}
    onCancel={...}                  // absent => no commit semantics
/>
```

#### Format neutrality

`LazyEditor` must never be markdown-only. Three things secure this:

- **Rendering is the consumer's.** `readView` is a prop. Cards' `MarkdownText`
  carries cards-specific concerns — mention-token resolution against
  `useProjectMembers`, protected-file image srcs, a per-surface type scale —
  none of which belong in core. Mail's read view is a different component
  entirely.
- **Content crosses as an opaque string.** `contentFormat` is already
  first-class on `RichEditorInitPayload` and `UseRichEditorOptions`; it selects
  `getMarkdown`/`setMarkdown` versus `getHTML`/`setContent` on the existing
  `EditorResult`. `LazyEditor` forwards it and reads back through whichever
  channel the format names.
- **`editorOptions` is `UseRichEditorOptions`.** Not a curated subset, so an
  option added for mail needs no change here.

#### `LazyEditor` owns the commit semantics

These rules were each found on a device and cost real debugging. Centralizing
them means mail and text inherit the fixes instead of rediscovering them.

| Rule | Why it exists |
|---|---|
| A blur before the session ever held focus must not commit | `hasFocusedRef` — an editor that mounted too short blurred immediately and saved a comment nobody had touched |
| Save and the blur-commit must not both write | `settledRef` — an edit session ends at its first commit; the parent unmounts the component |
| An unchanged value is a cancel, not a write | `EditableText`'s convention |
| A dialog taking focus is not the end of the session | The image picker and link prompt both steal focus; closing the editor under the picker unmounts the surface the image is about to land in |

`commitOnBlur` selects the variant. The description passes no `onCancel`: every
keystroke is already shared through Yjs and flushed by the server, so "revert"
and "save" have nowhere to live.

#### Switch policy

When a surface acquires the instance from another, the outgoing surface's text
is **committed if it has commit semantics, and otherwise preserved rather than
lost**. Applied per surface, this preserves existing behavior in every case:

- An inline comment edit already commits on blur today → switching away commits.
  Unchanged.
- The composer has no blur-commit → its draft is stashed, not discarded. See
  below.

Discarding the composer's draft outright would be a behavior regression. Today
`CommentComposer` stays mounted for the life of the open card specifically so a
half-typed draft survives (`CommentComposer.tsx:44` documents this), and under a
singleton that mount no longer holds the text. A composer-scoped draft store
closes exactly that gap.

#### The composer draft store

Release already has the outgoing text in hand — the singleton must be read
before it is handed over. For the composer, that value is kept instead of
dropped:

- On **release**, read the content back through the format's channel
  (`markdownHost.get()`, `use-rich-editor.native.tsx:410`) and stash it.
- On **acquire**, seed a stashed draft into the init payload's `initialContent`
  in place of the persisted value.
- On **commit or explicit clear**, drop the entry.

**Scoped to the composer, and to the life of the open card.** That is the whole
of the lifetime policy, and it costs nothing to enforce: `CardDetail` is already
keyed on the card id at both mount sites (`CardPeek.tsx:172` and
`screens/[cardId].tsx:167`, both keyed because the description editor binds to
one Yjs fragment per mount), so a card switch remounts the subtree and the store
goes with it. The result is today's behavior exactly — a draft that survives
switching to another editor within the card, and dies with the card.

Deliberately **not** carried further:

- **Not the description.** It is Yjs-backed, so a stashed draft could contradict
  the shared document. It has no draft to lose in any case — every keystroke is
  already flushed.
- **Not inline comment edits.** They commit on blur today, so switching away
  commits rather than stashing. Unchanged.
- **Not across card close, logout, or restart.** Surviving a reopen would be new
  behavior nobody asked for, and it is what forces an eviction policy, a
  persisted-vs-draft arbitration on acquire, and an answer for what clears a
  draft whose comment someone else deleted. Scoping to the mount avoids all of
  it.

### Web is a pass-through

Web's `useRichEditor` builds Tiptap in-process — no WebView, no cold start.
`LazyEditor` on web still renders the read view and swaps on press, so both
platforms walk the same code path and the e2e suite exercises the real thing,
but there is no warm instance and no parking. No behavior divergence between
platforms; the native path simply skips a cost web never paid.

## Architecture

### Protocol changes (`rich/webview/source/protocol.ts`)

- `APP_INIT` gains a `generation` field and becomes re-sendable. The page keys
  `EditorMounted` on it, so a new generation is a full stage-two remount.
- New host→page `APP_PARK`: tear down to stage one — render `null`, drop the
  Y.Doc, release awareness. Makes release explicit rather than implicit.

### Host changes

- `use-webview-editor.tsx`: replace the one-shot `initSentRef` latch with
  generation tracking, so a new payload is posted rather than suppressed.
- **The host-side relays need no new teardown — verified, and recorded here so it
  is not "fixed" later.** `YjsWebViewHost` and `AwarenessWebViewHost` are
  memoized on **document identity** (`use-rich-editor.native.tsx:116-143`), and
  in cards that document is the board's: `useBoardPresence.ts:141` returns
  `room?.doc`, one Y.Doc holding a fragment per card. It is the same object
  across every card on a board, so the relay is correctly shared and the only
  thing varying per acquire is the `field` (`card:<id>`) — which rides in the
  init payload and is consumed when the page rebuilds its own doc in
  `useCollabDoc`.

  The stale-relay hazard is real but belongs to a different case: a consumer
  swapping to a **different** Y.Doc. The existing memo already covers it — a new
  doc identity rebuilds both hosts and the `useEffect` cleanups destroy the old
  ones.

- **`MarkdownWebViewHost` must re-seed on acquire.** Its `lastKnown` fallback is
  what `getMarkdown` returns when the round-trip times out
  (`markdown-webview-host.ts:48`), and that value sits on the save path. Left
  unseeded, a timeout during a handover would return the *previous surface's*
  text and persist it over the current one.
- New `lib/editor/warm/`:
  - `WarmEditorHost` — renders the parked WebView. Positioned absolutely
    off-viewport **at real size**, not `display:none` and not zero-height: a
    zero-size WebView may not lay out or paint on iOS, which would defeat the
    warming. `pointerEvents="none"` and `accessibilityElementsHidden`.
  - `useWarmEditor(options)` — acquire/release around the singleton, returning
    the same `EditorResult` contract as `useRichEditor` so call sites change
    minimally. On web, an alias for `useRichEditor`.
- `LazyEditor` — the swap component described above.

### Fallback

If the warm instance is unavailable for any reason — not yet booted, or held by
another surface mid-release — the consumer mounts its own editor exactly as
today, paying the cold start. **Warm is an optimization, never a correctness
dependency**: a bug in the warm path degrades to current behavior rather than to
a broken editor.

### Cards changes

- `screens/_layout.tsx` mounts `WarmEditorHost`. This is the per-package mount
  point that matches "warm as soon as cards loads" — a manifest `provider`
  would not, because `PackageProviderWrapper` builds its chain at module load
  and wraps the whole app, booting a WebView at app launch for anyone with cards
  installed whether or not they ever open it.
- `CardDetail`'s `DescriptionReadView` swap and `DetailActivity`'s inline-edit
  swap are both replaced by `LazyEditor`.
- The board's Y.Doc is per-board, not per-card (`useBoardPresence.ts:138` — one
  doc holding a fragment per card), so the warm instance binds to the board doc
  and only the field (`card:<id>`) varies per acquire. This is what makes the
  description work through the same instance as comments.

## Testing

- **Unit, core:** re-init generation handling; that a new generation produces a
  fresh Y.Doc and empty undo stack; park/unpark; the commit-semantics matrix
  above (each rule from the table, driven through `LazyEditor`'s props);
  `contentFormat` routing to the correct read-back channel.
- **Unit, protocol:** re-sendable init round-trips, alongside the existing
  `trigger-serialization.test.ts` pinning.
- **E2E:** the existing comment-editing and description specs must pass
  unchanged — they are the regression net for the swap's layout neutrality
  (the ±2px anchor `comment-editing.spec` asserts).

  **E2E cannot cover the switch policy.** Playwright runs on web, where there is
  no warm singleton and each surface still mounts its own editor — so the
  handover never triggers there. No existing spec asserts composer-draft
  survival across an inline edit (checked), so nothing goes red; but nothing
  guards it either.

  The switch policy is therefore covered by unit tests over the acquire/release
  state machine — which surface holds the instance, what release does to its
  content — rather than end to end. The draft store is plain host code with no
  WebView in it, so it is directly unit-testable: stash on release, seed on
  acquire, drop on commit, and that a description or inline-edit release stashes
  nothing. One manual device check confirms the composer draft survives a
  round-trip through an inline edit.
- **Device:** the measurement that motivated this work, re-taken. The four
  `__DEV__` marks in `use-webview-editor.tsx` stay; a warm acquire should show
  no `page-ready` mark at all, only `init-sent` and `first-height`.

## Explicitly out of scope

- **Android verification.** Everything measured to date is iOS simulator. The
  mount cost should be comparable but is unmeasured, and the native mention
  picker has not been exercised there. Called out in the handoff doc as owed
  regardless of this work.
- **Backfilling old `@someone` mentions.** A migration, no decision made.
- **Draft persistence beyond the open card.** The composer draft store is scoped
  to `CardDetail`'s mount (see above). Surviving card close, logout, or restart
  is deliberately excluded — it is new behavior, and it is what would force an
  eviction policy and a persisted-vs-draft arbitration.
