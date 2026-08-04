// Safe-area insets — side-corrected for landscape — and the one rule for
// combining them with design spacing.
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
// sensor housing itself.
//
// Why not the raw library hook: in landscape iOS reports the sensor housing as
// a SYMMETRIC ~59pt inset on BOTH left and right, whichever side it actually
// occupies. Consuming that raw pair pads the housing-free side for nothing.
// `useDeviceInsets` folds in the interface orientation (which says where the
// housing really is) and zeroes the opposite side, so padding lands only where
// hardware demands it. Consumers should never reach around this to the raw
// library values — the raw pair is the bug this file exists to fix.

import { resolveInsets } from '@tinycld/core/lib/safe-area-resolve'
import { useOrientationStore } from '@tinycld/core/lib/stores/orientation-store'
import { useMemo } from 'react'
import { Platform } from 'react-native'
import type { EdgeInsets } from 'react-native-safe-area-context'
import { useSafeAreaInsets as useRawSafeAreaInsets } from 'react-native-safe-area-context'

export type { EdgeInsets } from 'react-native-safe-area-context'

/** Safe-area insets corrected for which side the sensor housing is on: in iOS
 *  landscape only the housing side keeps its ~59pt, the opposite side reads 0
 *  (full-bleed). Portrait, Android, web, and the moments before the first
 *  orientation event all pass through unchanged. */
export function useDeviceInsets(): EdgeInsets {
    const raw = useRawSafeAreaInsets()
    const orientation = useOrientationStore(s => s.orientation)
    return useMemo(() => resolveInsets(raw, orientation, Platform.OS), [raw, orientation])
}

/** @deprecated Alias of {@link useDeviceInsets}, kept so feature siblings keep
 *  resolving one hook from core rather than adding a
 *  `react-native-safe-area-context` peerDependency each. Note the semantics
 *  are the corrected ones — in landscape the housing-free side reads 0. */
export const useSafeAreaInsets = useDeviceInsets

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
 *  Built on `useDeviceInsets`, so in landscape the horizontal base is compared
 *  against the CORRECTED pair: the housing side gets ~59pt (the inset wins),
 *  the opposite side gets exactly the base (its inset reads 0). Asymmetric
 *  margins in landscape are therefore the intended result, not a bug. */
export function useSafeAreaPadding(base: { horizontal?: number; top?: number; bottom?: number }): {
    paddingLeft: number
    paddingRight: number
    paddingTop: number
    paddingBottom: number
} {
    const insets = useDeviceInsets()
    const horizontal = base.horizontal ?? 0
    return {
        paddingLeft: Math.max(horizontal, insets.left),
        paddingRight: Math.max(horizontal, insets.right),
        paddingTop: Math.max(base.top ?? 0, insets.top),
        paddingBottom: Math.max(base.bottom ?? 0, insets.bottom),
    }
}
