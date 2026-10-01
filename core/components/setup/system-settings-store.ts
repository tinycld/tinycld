import { like } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'
import { rowsToMap, type SettingRow } from './system-settings-logic'

export type { SettingRow }

/**
 * Read/write access to one namespace of the system_settings collection, for the
 * /admin Settings console. Returns a key→row map of that namespace and an
 * `upsert` mutation (update existing row by id, or insert a new one).
 * System-scoped, so a plain useLiveQuery (not useMyLiveQuery). The console runs
 * as a super-admin app user, so the collection rules authorize these writes.
 *
 * `prefix` is mandatory and scopes the read to `<prefix>.*`. system_settings is
 * admin-readable *including secret values*, so an unfiltered read would pull
 * every package's secrets into a client that is editing one panel. It also
 * decouples the panels from each other: adding a namespace no longer changes
 * what an unrelated panel holds.
 *
 * `upsert` still works because `byKey` covers the whole namespace a panel
 * writes — a panel must only write keys under its own prefix.
 *
 * `onError` replaces useMutation's generic failure toast for a panel that
 * can say what failed.
 */
export function useSystemSettings(
    prefix: string,
    options: { onError?: (err: Error) => void } = {}
) {
    const [systemSettings] = useStore('system_settings')

    const { data: rows = [], isReady } = useLiveQuery(query =>
        query.from({ s: systemSettings }).where(({ s }) => like(s.key, `${prefix}.%`))
    )

    const byKey = rowsToMap(rows)

    const upsert = useMutation({
        mutationFn: mutation(function* (input: { key: string; value: string; isSecret: boolean }) {
            const existing = byKey.get(input.key)
            if (existing) {
                yield systemSettings.update(existing.id, draft => {
                    draft.value = input.value
                })
            } else {
                yield systemSettings.insert({
                    id: newRecordId(),
                    key: input.key,
                    value: input.value,
                    is_secret: input.isSecret,
                } as never)
            }
        }),
        onError: options.onError,
    })

    return { byKey, upsert, isReady }
}
