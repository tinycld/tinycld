import { useEffect } from 'react'
import { AccessibilityInfo, Platform } from 'react-native'

// VoiceOver on iOS does not read live regions (aria-live is web's,
// accessibilityLiveRegion Android's), so a status that turns on is announced there
// explicitly. Announcing reaches an external system, which is what an effect is for.
export function useIOSAnnouncement(message: string, isOn: boolean) {
    useEffect(() => {
        if (Platform.OS === 'ios' && isOn) AccessibilityInfo.announceForAccessibility(message)
    }, [message, isOn])
}
