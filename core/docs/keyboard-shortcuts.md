# Keyboard Shortcuts

TinyCld has a sequence-aware keyboard shortcut system that works on both web and native (external keyboards on iPad/Catalyst). Press `?` anywhere in the app to see the current set of registered shortcuts.

## Using the app

### Always available

| Keys | Action |
|---|---|
| `?` | Toggle the keyboard shortcut help overlay |
| `Escape` | Close the current dialog |
| `t m` | Jump to Mail |
| `t d` | Jump to Drive |
| `t c` | Jump to Calendar |
| `t o` | Jump to Contacts |

Sequences like `t m` are two presses in quick succession (up to 1 second between keys).

### Mail list

| Keys | Action |
|---|---|
| `j` | Focus next conversation |
| `k` | Focus previous conversation |
| `Enter` / `o` | Open focused conversation |
| `x` | Toggle selection on focused conversation |
| `c` | Compose new email |

### Mail conversation

| Keys | Action |
|---|---|
| `Escape` | Close the conversation and return to the list |

### Mail compose

| Keys | Action |
|---|---|
| `⌘/Ctrl + Enter` | Send email |

### Drive list

| Keys | Action |
|---|---|
| `j` | Focus next item |
| `k` | Focus previous item |
| `Enter` | Open focused item |
| `x` | Toggle selection on focused item |
| `Shift + F` | New folder |

### Contacts list

| Keys | Action |
|---|---|
| `j` | Focus next contact |
| `k` | Focus previous contact |
| `Enter` | Open focused contact |
| `c` | New contact |

### Cards board

| Keys | Action |
|---|---|
| `j` / `↓` | Focus next card |
| `k` / `↑` | Focus previous card |
| `←` / `→` | Focus the adjacent column's card at the same row |
| `Enter` / `o` | Open focused card |
| `Escape` | Clear the focus ring |
| `Shift + ←` / `Shift + →` | Move focused card to the adjacent column |
| `Shift + ↑` / `Shift + ↓` | Move focused card within its column |
| `x` | Archive focused card |
| `n` | Add a card to the focused column |
| `Shift + N` | Add a list |

Moving, archiving and adding need edit rights on the board; a viewer sees only the navigation keys.

`n` resolves its target column from the focus ring (focused column → focused card's column → first column) and expands a collapsed column first, since a collapsed column mounts no composer. Both composers open through the cards UI store, because each lives inside a memoized `BoardColumn` the shortcut hook holds no reference to.

### Cards detail

| Keys | Action |
|---|---|
| `j` / `k` | Next / previous card, in board order |
| `e` | Edit the card title |
| `Escape` | Close the card |

`e` is registered by the peek (`modal`) and the page (`thread`), not by the shared `CardDetail` body they both render: `useRegisterShortcut` stamps the scope instance current when its effect runs, and child effects run before parent ones, so registering inside `CardDetail` picks up the wrong instance. `CardDetail` takes a ref and the containers own the binding — the general rule being that `useShortcutScope` and the registration belong in the same component.

### Calendar schedule view

| Keys | Action |
|---|---|
| `j` | Focus next event |
| `k` | Focus previous event |
| `Enter` | Open focused event |
| `Shift + C` | New event |

## How it works

Shortcuts live under `lib/shortcuts/`. The system is intentionally small and avoids coupling to any UI framework beyond React.

- `lib/shortcuts/registry.ts` — a Zustand store holding `Map<id, Shortcut>`.
- `lib/shortcuts/scopes.ts` — a stack of active scopes (`global`, `list`, `thread`, `compose`, `modal`) pushed by `useShortcutScope`.
- `lib/shortcuts/matcher.ts` — the sequence state machine. A 1-second inter-key timeout matches the tinykeys default. Both providers feed raw atoms into the same matcher so web and native share behaviour.
- `lib/shortcuts/provider.web.tsx` — wires single atoms to `window` via [tinykeys](https://github.com/jamiebuilds/tinykeys), twice: once in the capture phase and once in the bubble phase. Tinykeys handles `$mod` (⌘ on macOS, Ctrl elsewhere) and key normalization. `lib/shortcuts/dispatch.ts` (`keyPhase`, `handleKey`) decides which phase feeds each shortcut a key, so no shortcut is offered one key twice:
  - **capture** — a key typed in a text field, and a key held with Ctrl, ⌘ or Alt. react-native-web stops every key in a `TextInput`, and Enter (with any modifiers) and Space on a focused Pressable, before a bubble listener sees it, so these must be read first. A matched key is consumed with `preventDefault` + `stopPropagation`, so the focused element never sees it; any other key reaches it unchanged.
  - **bubble** — every other key (no modifier, or Shift only, outside a text field). The focused element answers it first, as it should: Enter and Space press a focused button, an open menu keeps its arrows and typeahead letters, `g i` sequences work.
  - **while a menu, popover or search palette is open** (any non-modal layer on top) — every key goes to bubble. A menu pushes no shortcut scope, so the screen's keys stay active under it; the bubble path lets the menu's search field and its focused trigger keep their keys, as before.
  - **`global` shortcuts and a key typed in a text field** — capture feeds such a key only to scoped shortcuts; `global` ones get it in bubble, as before. A global shortcut stays registered by a screen the user has left (`freezeOnBlur`), so reading it first would let that screen take keys from every field in the app.
  - **Escape while a layer is open** — neither. The layer stack (`ui/overlay/layer-stack.ts`) closes the top menu, dialog or sheet from its own capture listener on `document`, which runs after the window capture listener; the provider yields Escape to it so a screen's Escape never fires on the same press.
- `lib/shortcuts/dispatch.ts` — the pure phase decision (`keyPhase`, `handleKey`), unit-tested.
- `lib/shortcuts/provider.native.tsx` — wraps the app in `KeyboardExtendedView` from [`react-native-external-keyboard`](https://github.com/dorian-marchal/react-native-external-keyboard) and translates native `KeyPress` events to the same atom format.
- `lib/shortcuts/help.tsx` + `lib/shortcuts/help-list.ts` + `ui/Kbd.tsx` — the `?` overlay, its store and list, and the key badge component. The overlay lists only shortcuts whose scope is active (`isScopeActive`), not everything registered: a screen shadowed by an inner scope, or one left mounted by `freezeOnBlur`, still has its shortcuts in the registry but none of them can fire. Activity is judged against the stack below the overlay's own `modal` entry: opening the overlay records `nextScopeId()` as `scopeFloor`, and `helpGroups` reads `topScopeEntry(scopeFloor)`, so the list shows the keys of the screen (or dialog) it was opened over. It also leaves out a `global` shortcut when an active scoped one binds the same keys, because the matcher fires the scoped one (`pickFiring` in `matcher.ts` holds that rule for both).

The matcher dispatches a shortcut only when:

1. Its `scope` is `global` **or** equals the top of the scope stack, and
2. `allowInInputs` is true, **or** focus is not currently in a text input / contenteditable / ProseMirror surface, and
3. Its optional `when()` guard (if any) returns true.

Opening a dialog, a sheet or a search palette pushes the `modal` scope (its `LayerEscape`), which silences list/thread/compose shortcuts until it closes. A menu or popover pushes no scope. `Escape` is registered in the `modal` scope with `allowInInputs: true` for the help list; on web the layer stack closes the layer, even from a focused input.

## Registering shortcuts

### From a screen or component

Use `useRegisterShortcut` for a single shortcut, or `useRegisterShortcuts` for an array. Wrap the shortcut object in `useMemo` (or declare it at module level) so re-renders don't thrash the registry.

```tsx
import { useMemo } from 'react'
import { type Shortcut, useRegisterShortcut, useShortcutScope } from '~/lib/shortcuts'

export function MyList({ items }: { items: Item[] }) {
    useShortcutScope('list')

    const shortcut = useMemo<Shortcut>(
        () => ({
            id: 'mything.refresh',
            keys: 'r',
            scope: 'list',
            group: 'My Thing',
            description: 'Refresh the list',
            run: () => refresh(),
        }),
        []
    )
    useRegisterShortcut(shortcut)

    // ...
}
```

Fields:

- **`id`** — stable string, namespaced by package (`mail.list.next`). Re-registering with the same id overwrites the previous entry.
- **`keys`** — tinykeys syntax: `"j"`, `"Shift+F"`, `"$mod+Enter"`, `"t i"`.
- **`scope`** — `global` (always active) or one of `list`, `thread`, `compose`, `modal` (active only when at the top of the scope stack).
- **`description`** / **`group`** — displayed in the `?` help overlay, which renders one row per shortcut sorted by description. Two shortcuts that do the same thing under different keys therefore need distinct wording, or they read as the action listed twice; suffix the alias `(alt)` (`'Open card'` / `'Open card (alt)'`).
- **`allowInInputs`** — defaults to `false`. Set `true` for shortcuts that should fire while an input is focused (`⌘+Enter` to send, `Escape` to close).
- **`when`** — optional guard, e.g. `when: () => !selection.isEmpty`.

### Package-wide jump shortcut

A package declares its jump letter in its manifest:

```ts
// packages/mail/manifest.ts
const manifest = {
    name: 'Mail',
    slug: 'mail',
    nav: { label: 'Mail', icon: 'mail', order: 5, shortcut: 'm' },
    // ...
}
```

`validateNavShortcuts` (`scripts/describe-packages.ts`, run from `scripts/generate.ts`) fails generation if two manifests claim the same letter — otherwise both register and `t <letter>` fires whichever the matcher reaches first, leaving one package unreachable. At runtime, `components/CoreShortcuts.tsx` iterates `packageRegistry` and registers `t <letter>` jumps for every package that declares one.

Letters in use: `c` calendar, `d` drive, `b` boards, `m` mail, `o` contacts, `s` calc, `t` text.

## Testing

Unit tests for the matcher live at `tests/unit/shortcuts.test.ts` and cover single combos, `$mod` combos, sequences, the 1 s timeout, scope gating, the input-field gate, `when()` guards, the web phase decision (`keyPhase`, `handleKey`), and the help list's scope floor. Run with `pnpm run test:unit`.

End-to-end coverage on a real browser lives at `tests/e2e/keyboard-shortcuts.spec.ts` and exercises `?`, `t o`/`t m` jumps, and `j`/`k`/`Enter` on the mail list.

## Platform notes

- **Web** — tinykeys binds to `window`, in the capture and the bubble phase (see "How it works"). `$mod` resolves to `Meta` (⌘) on macOS and `Control` elsewhere. An `allowInInputs` shortcut fires from a react-native-web text field too, because the capture listener reads the key before the field stops it.
- **iPad / Catalyst** — the route-level `KeyboardExtendedView` receives hardware key events. Per-row focusable rows using the library's `Pressable` are a future enhancement; today the route-level wrapper covers the declared shortcut set.
- **Android** — the same native provider works in principle but has seen less testing. If route-level key capture proves unreliable on any platform, the fallback is to register shortcuts only on web; the public API is unchanged.
