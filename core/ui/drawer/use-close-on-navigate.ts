import { usePathname } from 'expo-router'
import { useEffect, useRef } from 'react'

// Close an open drawer when the user navigates to a different route. Drawers
// live at a persistent layout level and hold their open state in a store, so on
// native — where nothing unmounts them — they otherwise linger across
// navigation (the left workspace drawer avoids this only because the org layout
// closes it by hand on every pathname change). The shared Drawer wires this in
// so every right-anchored drawer is fixed at once: help, drive details, calc
// pivot / conditional-format / comments, mailbox and members.
//
// onClose is read through a ref so a caller passing an inline closure (its
// identity changing each render) doesn't re-arm the effect; the effect depends
// only on the pathname. The route at open is the baseline — onClose fires on a
// genuine change away from it, never on the initial mount — and re-baselines
// each time the drawer reopens.
export function useCloseOnNavigate(isOpen: boolean | undefined, onClose?: () => void) {
    const pathname = usePathname()
    const onCloseRef = useRef(onClose)
    onCloseRef.current = onClose
    // The route the drawer was opened on. While closed it tracks the current
    // pathname so the next open re-baselines; while open it stays fixed so the
    // first navigation away registers as a change and fires onClose.
    const openedAtRef = useRef(pathname)

    useEffect(() => {
        if (!isOpen) {
            openedAtRef.current = pathname
            return
        }
        if (pathname !== openedAtRef.current) onCloseRef.current?.()
    }, [isOpen, pathname])
}
