/**
 * Whether the host listens for ⌘/Ctrl+K. Module state rather than React state
 * because the editor's keymap reads it from a ProseMirror handler, and a window
 * message listener (APP_LINK_SHORTCUT_ENABLED) writes it.
 */
let enabled = false

export function setLinkShortcutEnabled(next: boolean): void {
    enabled = next
}

export function isLinkShortcutEnabled(): boolean {
    return enabled
}
