import { createContext, useContext } from 'react'

/**
 * True inside content that a cover holds out of play while it stays mounted
 * (the package terms cover over the package screens). Such content must not
 * come up over the cover by itself: a Dialog or Sheet it opens stays closed,
 * and its shortcuts are not registered, until the cover goes.
 */
const CoveredContext = createContext(false)

export const CoveredProvider = CoveredContext.Provider

/** True inside covered content. */
export function useIsCovered(): boolean {
    return useContext(CoveredContext)
}

/** An overlay's `isOpen`, held closed while its owner is covered. */
export function useOpenUnlessCovered(isOpen: boolean): boolean {
    const isCovered = useIsCovered()
    return isOpen && !isCovered
}
