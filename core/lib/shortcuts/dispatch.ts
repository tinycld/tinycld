import type { Matcher } from './matcher'

/**
 * Which of the web provider's two window listeners feeds a key to the matcher.
 *
 * react-native-web stops some keys before a bubble-phase window listener can
 * see them: a `TextInput` calls `stopPropagation` on every keydown, and a
 * focused Pressable does the same for Enter (whatever the modifiers) and for
 * Space on a button. So `⌘↵` never reached a form's Post while a button or a
 * field had focus.
 *
 * Moving every key to the capture phase would fix that and break the
 * opposite case: a key the focused element rightly owns. Plain Enter and
 * Space press a focused button, and an open menu answers the arrows, Home,
 * End and typeahead letters itself (core menus push no shortcut scope, so a
 * screen's `j` or `ArrowDown` is still active under one). So the capture
 * phase takes only the keys the old listener could never see and that are
 * the shortcut system's to take: a key typed in a text field (the matcher's
 * `allowInInputs` gate keeps every other shortcut out, so typing is
 * unchanged) and a key held with Ctrl, ⌘ or Alt. Every other key stays in
 * the bubble phase, where the focused element answers it first, as before.
 */
export type KeyPhase = 'capture' | 'bubble'

export interface KeyFacts {
    key: string
    ctrlKey: boolean
    metaKey: boolean
    altKey: boolean
    /** The key was typed in a text field (`isInputKeyEvent`). */
    inInput: boolean
    /** The top open layer dismisses itself on Escape. */
    layerTakesEscape: boolean
    /** The top open layer is not modal: a menu, a popover or a search palette. */
    menuOpen: boolean
}

/**
 * The phase that feeds this key to the matcher, or null when the shortcut
 * system must leave it alone.
 *
 * Null is Escape while a layer that dismisses on Escape is open. The layer
 * stack listens on `document` in the capture phase, and the window capture
 * listener always runs before it (capture runs window, then document), so
 * without this yield a screen's `allowInInputs` Escape would run on the
 * press that was meant only to close the menu or dialog on top.
 *
 * While a menu is open every other key takes the bubble path. A menu pushes
 * no shortcut scope, so the screen's keys stay active under it; read in the
 * capture phase, ⌘↵ typed in the menu's search field, or pressed on its
 * focused trigger, would run the screen's Post under the open menu. The
 * bubble path leaves those keys to the field and the trigger, as before.
 */
export function keyPhase(facts: KeyFacts): KeyPhase | null {
    if (facts.key === 'Escape' && facts.layerTakesEscape) return null
    if (facts.menuOpen) return 'bubble'
    if (facts.inInput) return 'capture'
    if (facts.ctrlKey || facts.metaKey || facts.altKey) return 'capture'
    return 'bubble'
}

/**
 * Feed `atom` to the matcher when this is the key's phase. True when the
 * matcher consumed it (a shortcut ran, or the key continues a sequence); the
 * caller then stops the event. Any other key is left untouched.
 *
 * A key typed in a text field is split: the capture call offers it to scoped
 * shortcuts only, the bubble call to `global` ones only. A global shortcut
 * stays registered by a screen the user has left (`freezeOnBlur`), so reading
 * it first would let that screen take keys from every field in the app (calc's
 * ⌘F opening find on a hidden sheet). In the bubble phase it gets the key only
 * where it did before: a react-native-web field stops it, a contenteditable
 * surface lets it through. One limit: the capture call resets a pending
 * sequence it cannot extend, so a multi-key `global` + `allowInInputs`
 * sequence cannot be typed in a contenteditable (none is registered).
 */
export function handleKey(input: {
    matcher: Matcher
    atom: string
    facts: KeyFacts
    phase: KeyPhase
}): boolean {
    const { matcher, atom, facts, phase } = input
    const target = keyPhase(facts)
    if (target === 'capture' && facts.inInput) {
        const only = phase === 'capture' ? 'scoped' : 'global'
        return matcher.feedAtom(atom, { inInput: true, only })
    }
    if (target !== phase) return false
    return matcher.feedAtom(atom, { inInput: facts.inInput })
}
