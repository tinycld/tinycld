import { log } from '@tinycld/core/lib/logger'
import { pb } from '@tinycld/core/lib/pocketbase'
import { RealtimeSubscribeAbortedError, subscribeTopic } from 'pbtsdb'
import { useEffect, useEffectEvent } from 'react'
import { createRerunScheduler, RERUN_DELAY_MS } from '../lib/rerun-scheduler'

// While a report is on screen it listens to the change-signal topic of every
// collection its result read, and runs again after each burst of changes. A
// subscription is an external system, which is what an effect is for; the topics
// joined into one string are its only dependency.
export function useLiveTopics(topics: readonly string[], rerun: () => Promise<void>) {
    const key = topics.join('\n')
    const onRerun = useEffectEvent(rerun)
    useEffect(() => {
        if (!key) return
        const controller = new AbortController()
        const scheduler = createRerunScheduler({
            delayMs: RERUN_DELAY_MS,
            run: () => onRerun(),
            onError: error => log.error('reports.live', error),
        })
        const subscriptions = key.split('\n').map(topic =>
            subscribeTopic(pb, topic, () => scheduler.signal(), {
                signal: controller.signal,
            }).catch((error: unknown) => {
                if (!(error instanceof RealtimeSubscribeAbortedError)) {
                    log.error('reports.live', error, { topic })
                }
                return null
            })
        )
        return () => {
            controller.abort()
            scheduler.dispose()
            for (const pending of subscriptions) {
                void pending
                    .then(unsubscribe => unsubscribe?.())
                    .catch((error: unknown) =>
                        log.warn('reports.live', 'unsubscribe failed', { error })
                    )
            }
        }
    }, [key])
}
