import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { useStore } from '@tinycld/core/lib/pocketbase'

export function useUserPreferences(app: string) {
    // Anon-tolerant to match useUserPreference: user-scoped preference hooks may
    // mount before login (the theme/color path renders on the pre-auth screen),
    // so we must not throw when anon — skip the query and return no rows.
    const { user, isLoggedIn } = useAuth({ throwIfAnon: false })
    const userId = user?.id ?? ''
    const [userPreferencesCollection] = useStore('user_preferences')

    const { data: preferences } = useLiveQuery(
        query =>
            query
                .from({ user_preferences: userPreferencesCollection })
                .where(({ user_preferences }) =>
                    and(eq(user_preferences.app, app), eq(user_preferences.user, userId))
                ),
        [app, userId]
    )

    return isLoggedIn ? (preferences ?? []) : []
}
