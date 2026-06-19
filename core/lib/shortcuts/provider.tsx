// Base module for TypeScript resolution. At bundle time Metro picks
// provider.web.tsx on web, provider.android.tsx on Android, and
// provider.native.tsx on every other native platform (iOS today).
import type { ReactNode } from 'react'

export interface ShortcutsProviderProps {
    children: ReactNode
}

export function ShortcutsProvider(_props: ShortcutsProviderProps): ReactNode {
    throw new Error(
        'ShortcutsProvider base module should never run — Metro should resolve a .web or .native override'
    )
}
