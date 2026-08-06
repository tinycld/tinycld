import * as Haptics from 'expo-haptics'
import { Platform } from 'react-native'

const isTouchDevice = Platform.OS === 'ios' || Platform.OS === 'android'

/**
 * Haptics are garnish: a simulator, a device with no vibrator, or a platform
 * quirk rejecting the call must never surface an error into the gesture
 * callback that asked for a tick — swallow instead of reporting.
 */
function fire(perform: () => Promise<void>) {
    if (!isTouchDevice) return
    perform().catch(() => {})
}

/** Light physical tick for grabbing or lifting something (drag activation). */
export function hapticImpactLight() {
    fire(() => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light))
}

/** Selection change — crossing into a new container or slot mid-gesture. */
export function hapticSelection() {
    fire(() => Haptics.selectionAsync())
}

/** A completed operation the user should feel land (a successful drop). */
export function hapticSuccess() {
    fire(() => Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success))
}
