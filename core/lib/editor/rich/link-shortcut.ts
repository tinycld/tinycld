import { Extension } from '@tiptap/core'

export interface LinkShortcutOptions {
    /** Returns true when something handled the key (a toolbar opened its Link dialog). */
    onLink: () => boolean
}

/**
 * Binds ⌘/Ctrl+K to a link callback.
 *
 * A ProseMirror keymap for the same reason as SubmitShortcut: it fires only
 * while this editor has focus, and inside the native WebView page too, where
 * the app's shortcut provider cannot read ⌘ from a hardware keyboard. A
 * callback that returns false leaves the key to the browser.
 */
export const LinkShortcut = Extension.create<LinkShortcutOptions>({
    name: 'linkShortcut',

    addOptions() {
        return { onLink: () => false }
    },

    addKeyboardShortcuts() {
        return { 'Mod-k': () => this.options.onLink() }
    },
})
