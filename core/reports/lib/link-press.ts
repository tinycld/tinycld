/** The parts of a press event a link reads; a native press has no keys or button. */
export interface PressLike {
    preventDefault?(): void
    stopPropagation?(): void
    metaKey?: boolean
    ctrlKey?: boolean
    shiftKey?: boolean
    altKey?: boolean
    button?: number
}

/** A click the browser opens in a new tab or window: with a key held, or not the main button. */
export function isModifiedClick(event: PressLike): boolean {
    const hasKey = Boolean(event.metaKey || event.ctrlKey || event.shiftKey || event.altKey)
    return hasKey || (event.button !== undefined && event.button !== 0)
}

/**
 * A link's press. On web the Pressable's own click handler replaces Link's, so a
 * plain press prevents the anchor's default and runs the action (a panel, or a push
 * the view chooses); on native Link's onPress runs after this one and skips a
 * default-prevented event. A modified click is left to the browser, which opens the
 * link's href (react-native-web never calls onPress for an ⌥-click). The press stops
 * here, so a row that opens on a press does not open again under its label link.
 */
export function linkPress(action: () => void) {
    return (event: PressLike) => {
        if (isModifiedClick(event)) return
        event.preventDefault?.()
        event.stopPropagation?.()
        action()
    }
}
