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
    // throwIfAnon: false is required, not optional. useAuth() defaults to
    // throwIfAnon: true, which throws AuthRequiredError during RENDER for a
    // signed-out visitor — the effect's `user?.id` guard never gets a chance to
    // run. Mounted in the org layout (which renders before the auth gate), a
    // bare useAuth() crashed the whole layout into the error boundary, so a
    // signed-out deep link showed "Something went wrong" instead of the login
    // form. Registration is a signed-in side effect; absence of a user is a
    // normal state here, not an error.
    const { user } = useAuth({ throwIfAnon: false })

    useEffect(() => {
        if (Platform.OS === 'web' || registered || !user?.id) return
        registered = true
        registerExpoPushToken(user.id)
    }, [user?.id])
}
