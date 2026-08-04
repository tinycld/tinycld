// Safe-area insets, and the one rule for combining them with design spacing.
//
// A sibling cannot import `react-native-safe-area-context` directly: it is not
// in any feature's peerDependencies, and adding it to each one would mean a
// coordinated change across every sibling repo for what is one hook. Core
// already depends on it, and siblings already resolve `@tinycld/core/lib/*` by
// package name, so this is the sanctioned route.
//
// Why a feature needs this at all: most screens render inside the workspace
// chrome, which insets its content for them. The exception is a surface that
// escapes that chrome — a full-screen overlay or an `absolute` window pinned to
// the viewport — which bypasses every ancestor's padding and has to clear the
// sensor housing itself. In landscape iOS reports that housing as a ~59pt
// left/right inset, which is where a flush-mounted close button ends up.

import { useSafeAreaInsets } from 'react-native-safe-area-context'

export type { EdgeInsets } from 'react-native-safe-area-context'
export { useSafeAreaInsets } from 'react-native-safe-area-context'

/** Padding that clears the device's safe area while never falling below a base
 *  spacing. The single place this rule is expressed — hand-rolling it per
 *  screen is how three different conventions (bare inset, base+inset, max)
 *  ended up in the tree at once.
 *
 *  The base is a MINIMUM, not an addend. Adding the two double-counts: on a
 *  device reporting a modest ~21pt home-indicator inset, `28 + 21` pushed
 *  content 49pt off an edge that only needed 28 for looks and 21 for hardware.
 *  Taking the larger satisfies both constraints at once — content is never
 *  closer to the edge than the design wants, and never under the sensor
 *  housing.
 *
 *  This does NOT flatten the left/right asymmetry it might appear to: when an
 *  inset genuinely exceeds the base (the ~59pt notch side in landscape) it wins
 *  and that side is visibly wider. Equal margins mean both insets were smaller
 *  than the base, which is the correct result, not a lost signal. */
export function useSafeAreaPadding(base: { horizontal?: number; top?: number; bottom?: number }): {
    paddingLeft: number
    paddingRight: number
    paddingTop: number
    paddingBottom: number
} {
    const insets = useSafeAreaInsets()
    const horizontal = base.horizontal ?? 0
    return {
        paddingLeft: Math.max(horizontal, insets.left),
        paddingRight: Math.max(horizontal, insets.right),
        paddingTop: Math.max(base.top ?? 0, insets.top),
        paddingBottom: Math.max(base.bottom ?? 0, insets.bottom),
    }
}
