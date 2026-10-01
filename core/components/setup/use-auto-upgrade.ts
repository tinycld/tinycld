import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { errorToString } from '@tinycld/core/lib/errors'
import { log } from '@tinycld/core/lib/logger'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { useStore } from '@tinycld/core/lib/pocketbase'
import type PocketBase from 'pocketbase'
import {
    type AutoUpgradeStatusResponse,
    DEFAULT_WINDOW,
    isOn,
    KEY_ENABLED,
    KEY_WINDOW,
} from './auto-upgrade-logic'
import { useSystemSettings } from './system-settings-store'

async function fetchStatus(pb: PocketBase): Promise<AutoUpgradeStatusResponse> {
    const res = await fetch(`${PB_SERVER_ADDR}/api/admin/packages/auto-upgrade/status`, {
        headers: { Authorization: pb.authStore.token },
    })
    if (!res.ok) throw new Error(`auto-upgrade status failed: ${res.status}`)
    return res.json() as Promise<AutoUpgradeStatusResponse>
}

const STATUS_KEY = ['auto-upgrade-status']

// Both the Packages page and the setup wizard write through this hook, so the
// failure is reported here once rather than by each screen.
function reportSaveError(err: Error) {
    log.error('setup.autoupgrade', err)
    notify.emit({
        event: 'mutation.error',
        title: 'Could not change automatic updates',
        body: errorToString(err),
        data: { operation: 'setup.autoupgrade', error: errorToString(err) },
    })
}

/** Narrow a row's JSON `target` column (server-written, a slug→version map) to a plain string map. */
function targetOf(target: unknown): Record<string, string> {
    if (!target || typeof target !== 'object' || Array.isArray(target)) return {}
    return target as Record<string, string>
}

// One place for the automatic-updates controls on the Packages page and in the
// setup wizard: the stored flag and window, the pause and blocked rows, and the
// computed status from the server.
export function useAutoUpgrade(pb: PocketBase, enabled = true) {
    const queryClient = useQueryClient()
    const { byKey, upsert, isReady } = useSystemSettings('autoupgrade', {
        onError: reportSaveError,
    })
    const [stateCollection] = useStore('autoupgrade_state')

    const { data: pauses = [] } = useLiveQuery(query =>
        query.from({ s: stateCollection }).where(({ s }) => eq(s.kind, 'pause'))
    )
    const { data: blockedRows = [] } = useLiveQuery(query =>
        query
            .from({ s: stateCollection })
            .where(({ s }) => and(eq(s.kind, 'blocked'), eq(s.cleared, false)))
            .select(({ s }) => ({ id: s.id, reason: s.reason, target: s.target }))
    )

    const statusQuery = useQuery({
        queryKey: STATUS_KEY,
        queryFn: () => fetchStatus(pb),
        enabled,
    })

    const clear = useMutation({
        mutationFn: mutation(function* (id: string) {
            yield stateCollection.update(id, draft => {
                draft.cleared = true
            })
        }),
    })

    const pause = pauses[0]
    return {
        isReady,
        isOn: isOn(byKey.get(KEY_ENABLED)?.value),
        window: byKey.get(KEY_WINDOW)?.value ?? DEFAULT_WINDOW,
        windowManaged: statusQuery.data?.windowManaged ?? false,
        status: statusQuery.data?.status,
        pause: pause ? { reason: pause.reason, target: targetOf(pause.target) } : undefined,
        blocked: blockedRows.map(r => ({ ...r, target: targetOf(r.target) })),
        // The server's status depends on the switch, so it is fetched again.
        setOn: (next: boolean) =>
            upsert.mutate(
                { key: KEY_ENABLED, value: String(next), isSecret: false },
                { onSuccess: () => queryClient.invalidateQueries({ queryKey: STATUS_KEY }) }
            ),
        saveWindow: upsert,
        clearBlocked: (id: string) => clear.mutate(id),
    }
}
