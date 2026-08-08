import { Extension } from '@tiptap/core'

export interface SubmitShortcutOptions {
    onSubmit: () => void
}

/**
 * Binds ⌘/Ctrl+Enter to a submit callback.
 *
 * This has to be a ProseMirror keymap rather than a DOM listener on the
 * surrounding view: the editor handles keydown itself and stops propagation, so
 * an `onKeyPress` on a wrapping element never fires. Returning true also keeps
 * the shortcut from inserting a paragraph break on its way out.
 */
export const SubmitShortcut = Extension.create<SubmitShortcutOptions>({
    name: 'submitShortcut',

    addOptions() {
        return { onSubmit: () => {} }
    },

    addKeyboardShortcuts() {
        const submit = () => {
            this.options.onSubmit()
            return true
        }
        return { 'Mod-Enter': submit }
    },
})
