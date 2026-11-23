import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { log } from '@tinycld/core/lib/logger'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { recoverLiveQueries } from '@tinycld/core/lib/recover-live-queries'
import { newRecordId } from 'pbtsdb/core'
import { useCallback } from 'react'

export interface UserPreferenceOptions {
    /**
     * Called when a save fails, in place of the default error toast of
     * `useMutation`. The optimistic value is rolled back either way.
     */
    onSaveError?: (error: Error) => void
}

export interface UserPreferenceStatus {
    /** The read has loaded (the row, or that there is none). False while signed out. */
    isReady: boolean
    /** The read failed. The setter writes nothing until `retry` recovers it. */
    isError: boolean
    /** Restarts a failed read. */
    retry: () => Promise<void>
}

/**
 * One preference of the signed-in user, kept in `user_preferences` under
 * (`app`, `key`). Returns the value (`defaultValue` until a row loads), a
 * setter, and the read's status, for a screen that shows its own loading and
 * error states. The setter does nothing while signed out, and nothing until
 * the read is ready.
 */
export function useUserPreference<T>(
    app: string,
    key: string,
    defaultValue: T,
    options: UserPreferenceOptions = {}
): readonly [T, (newValue: T) => void, UserPreferenceStatus] {
    // Anon-tolerant: this hook runs in the theme/color path (useColorTheme →
    // useUserPreference), which mounts on every screen INCLUDING the pre-auth
    // login screen. Passing throwIfAnon: false keeps it from throwing before
    // anyone is authed; when anon we skip the user-scoped query and return the
    // caller's default. Mirrors use-theme-preference.ts.
    const { user, isLoggedIn } = useAuth({ throwIfAnon: false })
    const userId = user?.id ?? ''
    const [userPreferencesCollection] = useStore('user_preferences')

    // Disabled while anon (null query, the useMyLiveQuery idiom): under
    // on-demand sync a blank id would send a real `user = ""` request from the
    // login screen instead of filtering nothing locally. setValue's own
    // isLoggedIn guard is what keeps the resulting isReady=true harmless.
    const {
        data: rows,
        isReady,
        isError,
        collection: preferenceQuery,
    } = useLiveQuery({
        query: query =>
            userId
                ? query
                      .from({ user_preferences: userPreferencesCollection })
                      .where(({ user_preferences }) =>
                          and(
                              eq(user_preferences.app, app),
                              eq(user_preferences.key, key),
                              eq(user_preferences.user, userId)
                          )
                      )
                : null,
    })

    const existing = isLoggedIn ? rows?.[0] : undefined
    const value = existing ? (existing.value as T) : defaultValue

    // Re-resolve the row at mutation time rather than capturing the render-time
    // closure. The live query syncs asynchronously: a render where rows is still
    // empty can hand a stale `existing = undefined` to the mutation, which then
    // INSERTs a duplicate. PocketBase's unique index on (user, app, key)
    // rejects the second insert and TanStack DB rolls the optimistic update
    // back, manifesting as the UI flipping to the new value and snapping back.
    //
    // toArray alone is no longer enough to rule that out. user_preferences is
    // on-demand, so the store is empty until THIS query's fetch lands — a
    // pre-load `toArray` is indistinguishable from "no row exists". setValue
    // therefore waits on the query's own isReady before mutating.
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
        // undefined keeps useMutation's default toast and Sentry capture.
        onError: options.onSaveError,
    })

    const setValue = useCallback(
        (newValue: T) => {
            // No-op when anon: there's no user to scope the preference to.
            if (!isLoggedIn) return
            // And no-op until the row (or its confirmed absence) has loaded —
            // see the insert/rollback note above.
            if (!isReady) return
            upsert.mutate(newValue)
        },
        [isLoggedIn, isReady, upsert]
    )

    const retry = useCallback(
        () =>
            recoverLiveQueries({
                queries: [preferenceQuery],
                sources: [userPreferencesCollection],
                onError: err => log.error('core.user-preference', err, { app, key }),
            }),
        [preferenceQuery, userPreferencesCollection, app, key]
    )

    const status: UserPreferenceStatus = {
        isReady: isLoggedIn && isReady,
        isError: isLoggedIn && isError,
        retry,
    }

    return [value, setValue, status] as const
}
