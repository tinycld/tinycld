import { and, eq, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useQuery } from '@tanstack/react-query'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'

interface SeatsResponse {
    used: number
    limit: number
}

// Seats in use, counted live so an invite or a removal shows at once, against
// the limit the server reports (0 = unlimited). The rule matches the server's
// quota.countsAsSeat: guests and disabled (incl. deleted) accounts are free.
export function useSeatUsage() {
    const [users] = useStore('users')
    const { data: seatRows } = useLiveQuery(query =>
        query
            .from({ u: users })
            .where(({ u }) => and(not(eq(u.role, 'guest')), eq(u.disabled, false)))
            .select(({ u }) => ({ id: u.id }))
    )
    const { data: seats } = useQuery({
        queryKey: ['seats'],
        queryFn: () => pb.send<SeatsResponse>('/api/seats', {}),
    })
    const used = seatRows?.length ?? 0
    const limit = seats?.limit ?? 0
    return { used, limit, isAtLimit: limit > 0 && used >= limit }
}
