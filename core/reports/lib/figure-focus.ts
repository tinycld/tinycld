import { type ComponentRef, useCallback } from 'react'
import type { View } from 'react-native'
import { AccessibilityInfo, Platform } from 'react-native'
import type { ReportUIScope } from '../stores/report-ui-store'

export type FocusTarget = ComponentRef<typeof View>

/**
 * Moves the focus to a view: the keyboard focus on web, the screen reader's on
 * native, where a view has no keyboard focus to take.
 */
export function focusNode(node: FocusTarget) {
    if (Platform.OS === 'web') node.focus()
    else AccessibilityInfo.sendAccessibilityEvent(node, 'focus')
}

// The figures drawn now, by key. FlashList recycles a row's views for another row,
// so a figure is filed under its key while it shows that key, and only its own
// entry is removed when it leaves.
const figures = new Map<string, FocusTarget>()

// The scope names the view that draws the figure: the main report and the drill
// panel are both mounted and can draw the same column and row ids.
export function figureKey(scope: ReportUIScope, columnId: string, rowId: string): string {
    return `${scope}|${columnId}|${rowId}`
}

/** Files a drawn figure; the returned function removes it, unless a newer one took the key. */
export function registerFigure(key: string, node: FocusTarget): () => void {
    figures.set(key, node)
    return () => {
        if (figures.get(key) === node) figures.delete(key)
    }
}

/** A drilled figure's ref: the list's copy, never a stuck copy, takes the focus back. */
export function useFigureRef(key: string, isCopy: boolean) {
    return useCallback(
        (node: FocusTarget | null) => {
            if (!node || isCopy) return
            return registerFigure(key, node)
        },
        [key, isCopy]
    )
}

/** Sends the focus back to a figure, when it is still drawn. */
export function focusFigure(key: string | null) {
    const node = key === null ? undefined : figures.get(key)
    if (node) focusNode(node)
}
