import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'
import { rowsToMap, type SettingRow } from './system-settings-logic'

export type { SettingRow }

/**
 * Read/write access to the system_settings collection for the /admin Settings
 * console. Returns a key→row map of everything currently stored and an `upsert`
 * mutation (update existing row by id, or insert a new one). System-scoped, so a
 * plain useLiveQuery (not useOrgLiveQuery). The console runs as a super-admin app
 * user, so the collection rules authorize these writes.
 */
export function useSystemSettings() {
    const [systemSettings] = useStore('system_settings')

    const { data: rows = [] } = useLiveQuery(query => query.from({ s: systemSettings }))

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
    })

    return { byKey, upsert }
}
