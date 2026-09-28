import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'
import { useCallback } from 'react'
import { useColorScheme } from 'react-native'

export type ThemePreference = 'system' | 'light' | 'dark'
export type ResolvedTheme = 'light' | 'dark'

const APP = 'core'
const KEY = 'theme'

export function useThemePreference() {
    const systemScheme = useColorScheme()
    const { user, isLoggedIn } = useAuth({ throwIfAnon: false })
    const [userPreferencesCollection] = useStore('user_preferences')

    // Disabled while anon (null query, the useMyLiveQuery idiom). This hook
    // mounts on every screen including the pre-login one, and user_preferences
    // is on-demand — a blank id would send a real `user = ""` request there
    // rather than filtering nothing locally.
    const { data: rows, isReady } = useLiveQuery({
        query: query =>
            user?.id
                ? query
                      .from({ user_preferences: userPreferencesCollection })
                      .where(({ user_preferences }) =>
                          and(
                              eq(user_preferences.app, APP),
                              eq(user_preferences.key, KEY),
                              eq(user_preferences.user, user.id)
                          )
                      )
                : null,
    })

    const existing = rows?.[0]
    const preference: ThemePreference =
        isLoggedIn && existing ? (existing.value as ThemePreference) : 'system'

    const upsert = useMutation({
        mutationFn: mutation(function* (newValue: ThemePreference) {
            if (!user) return
            if (existing) {
                yield userPreferencesCollection.update(existing.id, draft => {
                    draft.value = newValue
                })
            } else {
                yield userPreferencesCollection.insert({
                    id: newRecordId(),
                    app: APP,
                    key: KEY,
                    value: newValue,
                    user: user.id,
                })
            }
        }),
    })

    const setPreference = useCallback(
        (pref: ThemePreference) => {
            // Wait for the row (or its confirmed absence) to load. user_preferences
            // is on-demand, so before this query's fetch lands `existing` is
            // undefined whether or not a row exists — mutating then INSERTs a
            // duplicate, which the unique index on (user, app, key) rejects and
            // TanStack DB rolls back: the theme flips and snaps straight back.
            // Same reasoning as use-user-preference.ts.
            if (!isReady) return
            upsert.mutate(pref)
        },
        [isReady, upsert]
    )

    const resolved: ResolvedTheme =
        preference === 'system' ? (systemScheme === 'dark' ? 'dark' : 'light') : preference

    return { preference, resolved, setPreference }
}
