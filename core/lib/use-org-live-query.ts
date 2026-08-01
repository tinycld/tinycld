import type { Context, InitialQueryBuilder, QueryBuilder } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { useMemo } from 'react'

// Single-org deployment: the process IS one org, so there is no org to scope by.
// The only value queries still scope on is the current user's id — used to filter
// "my own rows" (records whose owner/user/author FK now points straight at the
// users collection). The query is disabled until the user id is known.
export interface OrgScope {
    userId: string
}

export function useOrgLiveQuery<TContext extends Context>(
    queryFn: (q: InitialQueryBuilder, org: OrgScope) => QueryBuilder<TContext> | undefined | null,
    deps: unknown[] = []
) {
    const { user } = useAuth()
    const scope = useMemo(() => ({ userId: user?.id ?? '' }), [user?.id])

    return useLiveQuery(
        q => {
            if (!scope.userId) return null
            return queryFn(q, scope)
        },
        [scope.userId, ...deps]
    )
}
