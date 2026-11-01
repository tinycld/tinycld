import { asyncStorage, create, persist } from '@tinycld/core/lib/store'

// When this device last ran each saved report, per reader (the hub's "Last run").
// The run endpoint writes nothing, so the record stays on the device (R12).

export const RECENT_CAP = 200

export function recentKey(userId: string, reportId: string): string {
    return `${userId}:${reportId}`
}

/** The map with one run recorded; past `cap` entries the oldest runs are dropped. */
export function withRun(
    lastRun: Readonly<Record<string, string>>,
    key: string,
    at: string,
    cap = RECENT_CAP
): Record<string, string> {
    const next = { ...lastRun, [key]: at }
    const keys = Object.keys(next)
    if (keys.length <= cap) return next
    const newest = new Set(keys.sort((a, b) => next[b].localeCompare(next[a])).slice(0, cap))
    return Object.fromEntries(Object.entries(next).filter(([k]) => newest.has(k)))
}

interface RecentState {
    lastRun: Record<string, string>
    markRun: (userId: string, reportId: string, at: string) => void
}

export const useReportRecentStore = create<RecentState>()(
    persist(
        set => ({
            lastRun: {},
            markRun: (userId, reportId, at) =>
                set(state => ({
                    lastRun: withRun(state.lastRun, recentKey(userId, reportId), at),
                })),
        }),
        {
            name: 'tinycld_report_recent',
            storage: asyncStorage,
            partialize: state => ({ lastRun: state.lastRun }),
        }
    )
)
