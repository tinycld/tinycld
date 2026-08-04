import type { AppOrientation } from '@tinycld/core/lib/safe-area-resolve'
import { create } from '@tinycld/core/lib/store'
import { Platform } from 'react-native'

export interface OrientationState {
    orientation: AppOrientation
    setOrientation: (orientation: AppOrientation) => void
}

// 'unknown' until the first orientation event: resolveInsets passes symmetric
// insets through for it, so the pre-event frames can never put content under
// the sensor housing.
export const useOrientationStore = create<OrientationState>()(set => ({
    orientation: 'unknown',
    setOrientation: orientation => set({ orientation }),
}))

// One subscription for the entire app, mirroring window-size-store. iOS-only:
// the correction this feeds exists because iOS reports symmetric landscape
// insets; Android/web insets are already per-side-correct and need no
// orientation input. The dynamic import also keeps a binary built without the
// expo-screen-orientation native module (stale dev client, OTA mismatch) from
// crashing at startup — the rejection leaves the store at 'unknown', which
// degrades to the old symmetric-inset behavior.
if (Platform.OS === 'ios') {
    import('expo-screen-orientation')
        .then(ScreenOrientation => {
            const { Orientation } = ScreenOrientation
            // Mechanical enum→string mapping only — the physical which-side
            // judgment lives solely in islandSide() in safe-area-resolve.ts.
            const toApp = (orientation: number): AppOrientation => {
                switch (orientation) {
                    case Orientation.PORTRAIT_UP:
                    case Orientation.PORTRAIT_DOWN:
                        return 'portrait'
                    case Orientation.LANDSCAPE_LEFT:
                        return 'landscape-left'
                    case Orientation.LANDSCAPE_RIGHT:
                        return 'landscape-right'
                    default:
                        return 'unknown'
                }
            }
            ScreenOrientation.getOrientationAsync()
                .then(orientation =>
                    useOrientationStore.getState().setOrientation(toApp(orientation))
                )
                .catch(() => {})
            ScreenOrientation.addOrientationChangeListener(event =>
                useOrientationStore
                    .getState()
                    .setOrientation(toApp(event.orientationInfo.orientation))
            )
        })
        .catch(() => {})
}
