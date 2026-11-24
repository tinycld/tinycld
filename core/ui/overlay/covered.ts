import { createContext, useContext } from 'react'

/**
 * True inside content that a cover holds out of play while it stays mounted
 * (the package terms cover over a closed package's screens). Such content must
 * not come up over the cover or over other content by itself: a Dialog or Sheet
 * it opens stays closed, its shortcuts are not registered, and its text inputs
 * take no keys, until the cover goes.
 *
 * The value belongs to each navigator route, not to the active one: a closed
 * route that stays mounted and frozen while the user is on an open route is
 * still covered there.
 */
const CoveredContext = createContext(false)

export const CoveredProvider = CoveredContext.Provider

/** True for a navigator route whose content is closed (`closed` holds the closed route names). */
export const isRouteCovered = (routeName: string, closed: ReadonlySet<string>) =>
    closed.has(routeName)

/** True inside covered content. */
export function useIsCovered(): boolean {
    return useContext(CoveredContext)
}

/** An overlay's `isOpen`, held closed while its owner is covered. */
export function useOpenUnlessCovered(isOpen: boolean): boolean {
    const isCovered = useIsCovered()
    return isOpen && !isCovered
}

interface InputFocusProps {
    autoFocus?: boolean
    editable?: boolean
}

/**
 * A text input's `autoFocus` and `editable`, held off while it is covered. A
 * hardware keyboard (iOS) sends no keyboard event that could take the focus
 * back, so a covered input must not take keys at all.
 */
export function coveredInputProps(isCovered: boolean, props: InputFocusProps): InputFocusProps {
    if (isCovered) return { autoFocus: false, editable: false }
    return { autoFocus: props.autoFocus, editable: props.editable }
}

/** `coveredInputProps` for the input's own cover state. */
export function useCoveredInputProps(props: InputFocusProps): InputFocusProps {
    return coveredInputProps(useIsCovered(), props)
}
