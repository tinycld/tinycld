import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'
import { useCallback } from 'react'

export function useUserPreference<T>(
    app: string,
    key: string,
    defaultValue: T
): readonly [T, (newValue: T) => void] {
    // Anon-tolerant: this hook runs in the theme/color path (useColorTheme →
    // useUserPreference), which mounts on every screen INCLUDING the pre-auth
    // login screen. Passing throwIfAnon: false keeps it from throwing before
    // anyone is authed; when anon we skip the user-scoped query and return the
    // caller's default. Mirrors use-theme-preference.ts.
    const { user, isLoggedIn } = useAuth({ throwIfAnon: false })
    const userId = user?.id ?? ''
    const [userPreferencesCollection] = useStore('user_preferences')

    const { data: rows } = useLiveQuery(
        query =>
            query
                .from({ user_preferences: userPreferencesCollection })
                .where(({ user_preferences }) =>
                    and(
                        eq(user_preferences.app, app),
                        eq(user_preferences.key, key),
                        eq(user_preferences.user, userId)
                    )
                ),
        [app, key, userId]
    )

    const existing = isLoggedIn ? rows?.[0] : undefined
    const value = existing ? (existing.value as T) : defaultValue

    // Re-resolve the row at mutation time rather than capturing the render-time
    // closure. The live query syncs asynchronously: a render where rows is still
    // empty can hand a stale `existing = undefined` to the mutation, which then
    // INSERTs a duplicate. PocketBase's unique index on (user, app, key)
    // rejects the second insert and TanStack DB rolls the optimistic update
    // back, manifesting as the UI flipping to the new value and snapping back.
    const upsert = useMutation({
        mutationFn: mutation(function* (newValue: T) {
            if (!user) return
            const current = userPreferencesCollection.toArray.find(
                r => r.app === app && r.key === key && r.user === user.id
            )
            if (current) {
                yield userPreferencesCollection.update(current.id, draft => {
                    draft.value = newValue
                })
            } else {
                yield userPreferencesCollection.insert({
                    id: newRecordId(),
                    app,
                    key,
                    value: newValue,
                    user: user.id,
                })
            }
        }),
    })

    const setValue = useCallback(
        (newValue: T) => {
            // No-op when anon: there's no user to scope the preference to.
            if (!isLoggedIn) return
            upsert.mutate(newValue)
        },
        [isLoggedIn, upsert]
    )

    return [value, setValue] as const
}
