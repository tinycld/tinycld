import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'

function currentMonth() {
    return new Date().toISOString().slice(0, 7)
}

export function useClassifierUsage() {
    const [usage] = useStore('classifier_usage')
    const month = currentMonth()
    const { data } = useLiveQuery(query =>
        query
            .from({ u: usage })
            .where(({ u }) => eq(u.month, month))
            .findOne()
    )
    return { costUsd: data?.cost_usd ?? 0, calls: data?.calls ?? 0 }
}
