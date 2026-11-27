export type Scope = 'global' | 'list' | 'thread' | 'compose' | 'modal'

export interface ShortcutEvent {
    keys: string
}

export interface Shortcut {
    id: string
    keys: string
    scope: Scope
    description: string
    group?: string
    allowInInputs?: boolean
    when?: () => boolean
    /**
     * Fires and takes part in which registration wins its keys, but the ⇧?
     * help list never shows it: a guard that only holds a key (a ⌘S that
     * stops the browser's Save page while the screen offers no save).
     */
    isUnlisted?: boolean
    run: (e: ShortcutEvent) => void
    /**
     * Which scope instance owns this shortcut. Stamped by `useRegisterShortcut`
     * at registration; never set by hand.
     *
     * A frozen screen keeps its shortcuts registered, so two packages can hold
     * the same key at the same scope (mail's list `j` and a Boards board's `j`).
     * The matcher uses this to fire the one belonging to the screen that
     * actually has the keyboard.
     */
    scopeId?: number | null
}
