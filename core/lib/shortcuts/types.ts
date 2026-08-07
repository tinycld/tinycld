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
    run: (e: ShortcutEvent) => void
    /**
     * Which scope instance owns this shortcut. Stamped by `useRegisterShortcut`
     * at registration; never set by hand.
     *
     * A frozen screen keeps its shortcuts registered, so two packages can hold
     * the same key at the same scope (mail's list `j` and a cards board's `j`).
     * The matcher uses this to fire the one belonging to the screen that
     * actually has the keyboard.
     */
    scopeId?: number | null
}
