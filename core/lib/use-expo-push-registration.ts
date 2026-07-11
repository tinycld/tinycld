import { useAuth } from '@tinycld/core/lib/auth'
import { registerExpoPushToken } from '@tinycld/core/lib/expo-push'
import { useEffect } from 'react'
import { Platform } from 'react-native'

// Module-lifetime guard so we register the device's push token at most once
// per app session, even as the hook re-mounts across route changes. It's
// module-scoped (not a component ref) so logout can reset it — otherwise a
// second user signing in on the same running SPA/app would never get their
// own registration. resetExpoPushRegistration() clears it from logout.
let registered = false

export function resetExpoPushRegistration(): void {
    registered = false
}

export function useExpoPushRegistration() {
    const { user } = useAuth()

    useEffect(() => {
        if (Platform.OS === 'web' || registered || !user?.id) return
        registered = true
        registerExpoPushToken(user.id)
    }, [user?.id])
}
