// Side-corrects iOS landscape safe-area insets so only the sensor-housing
// (notch / Dynamic Island) side is inset and the opposite side is full-bleed.
//
// iOS reports SYMMETRIC left/right insets in landscape — ~59pt on BOTH sides
// of a Dynamic Island phone (44pt on notch phones) — regardless of which side
// the housing physically sits on. The insets alone therefore cannot say which
// side needs clearing; that takes the interface orientation, which tells us
// which way the device was rotated and so where the housing ended up.
//
// This module is deliberately free of React/react-native/Expo imports so unit
// tests exercise it with no mocks. The orientation feed lives in
// `stores/orientation-store.ts`; the React binding in `use-safe-area.ts`.

export interface Insets {
    top: number
    right: number
    bottom: number
    left: number
}

export type AppOrientation = 'portrait' | 'landscape-left' | 'landscape-right' | 'unknown'

/** Which interface side the sensor housing occupies, or null when not in a
 *  known landscape orientation.
 *
 *  This is the ONE place the notorious interface-vs-device left/right naming
 *  flip is decided. The names really are crossed: interface LANDSCAPE_LEFT
 *  puts the housing on the RIGHT, and vice versa. Verified empirically on an
 *  iPhone 17 simulator (iOS 26.5) by locking each landscape orientation and
 *  observing which side of the interface the Dynamic Island overlapped. Do NOT
 *  edit these values from memory or Apple-docs reasoning — re-verify on a
 *  simulator (the locked cases in safe-area-resolve.test.ts must be updated in
 *  the same change). */
export function islandSide(orientation: AppOrientation): 'left' | 'right' | null {
    if (orientation === 'landscape-left') return 'right'
    if (orientation === 'landscape-right') return 'left'
    return null
}

/** iOS-landscape-only correction: zero the inset on the side opposite the
 *  sensor housing. Everything else passes through untouched:
 *  - portrait / 'unknown' (including before the first orientation event):
 *    symmetric passthrough — conservative, never puts content under the island
 *  - non-iOS: Android display-cutout insets are genuinely per-side and web
 *    reports zeros, so both are already correct
 *  - the `left > 0 && right > 0` guard skips iPad (0/0), one-sided Android
 *    cutouts, and Split View / Stage Manager, which never report the
 *    symmetric-pair shape this correction exists for */
export function resolveInsets(
    raw: Insets,
    orientation: AppOrientation,
    platformOS: string
): Insets {
    if (platformOS !== 'ios') return raw
    const side = islandSide(orientation)
    if (side === null) return raw
    if (!(raw.left > 0 && raw.right > 0)) return raw
    return side === 'left' ? { ...raw, right: 0 } : { ...raw, left: 0 }
}
