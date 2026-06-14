import type { DispatchInput, NotifyChannel } from '@tinycld/core/lib/notify/channels/types'
import { getNotifyContext } from '@tinycld/core/lib/notify/context'
import { captureExceptionToSentry as captureException } from '@tinycld/core/lib/sentry'
import { newRecordId } from 'pbtsdb/core'

// pocketbase is imported lazily inside dispatch() to break a require cycle:
// pocketbase → errors → notify/dispatcher → bell → pocketbase. dispatch() is
// already async, and the bell channel only writes a record at call time, so
// deferring the import keeps pocketbase out of the notify graph's load order.

export const bellChannel: NotifyChannel = {
    name: 'bell',
    async dispatch(input: DispatchInput) {
        const ctx = getNotifyContext()
        if (!ctx) {
            captureException(
                'notify.bell.no_context',
                new Error(`skipped "${input.event}" — no org/user context`)
            )
            return
        }

        const { notificationsCollection } = await import('@tinycld/core/lib/pocketbase')
        const tx = notificationsCollection.insert({
            id: newRecordId(),
            user: ctx.userId,
            org: ctx.orgId,
            type: input.event,
            package: 'core',
            title: input.title,
            body: input.body ?? '',
            url: input.url ?? '',
            metadata: input.data ?? {},
            read: false,
            dismissed: false,
        })
        await tx.isPersisted.promise
    },
}
