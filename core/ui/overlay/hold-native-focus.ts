import { Keyboard, TextInput, type ViewInstance } from 'react-native'

/** What `holdFocusOut` reads and does, so the rule runs without a device. */
export interface FocusPort<N> {
    /** The text input that holds the focus, if any. */
    focused: () => N | null | undefined
    /** The input is inside the held content. */
    contains: (input: N) => boolean
    blur: (input: N) => void
    /** Calls the listener each time the keyboard shows; returns the unsubscribe. */
    onKeyboardShow: (listener: () => void) => () => void
}

/**
 * Keeps the focus out of content that a cover holds out of play. An input
 * inside that focused before the cover came up loses the focus now; one that
 * takes it later (it mounts lazily and autofocuses) shows the keyboard, and
 * loses the focus then. Returns the release.
 */
export function holdFocusOut<N>(port: FocusPort<N>): () => void {
    const blurInside = () => {
        const focused = port.focused()
        if (focused != null && port.contains(focused)) port.blur(focused)
    }
    blurInside()
    return port.onKeyboardShow(blurInside)
}

/**
 * Native: `holdFocusOut` for a covered View. pointerEvents and the
 * accessibility props stop new presses and the screen reader, but a focused
 * TextInput keeps taking keys and `onSubmitEditing` without this.
 */
export function holdNativeFocusOut(container: ViewInstance): () => void {
    return holdFocusOut({
        focused: () => TextInput.State.currentlyFocusedInput(),
        contains: input => container.contains(input),
        blur: input => TextInput.State.blurTextInput(input),
        onKeyboardShow: listener => {
            const subscription = Keyboard.addListener('keyboardDidShow', listener)
            return () => subscription.remove()
        },
    })
}
