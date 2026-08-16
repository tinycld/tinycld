import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useMemo } from 'react'

import type { CatalogAction, CatalogResponse, CatalogTrigger } from './api'

export function useAutomationCatalog(): { catalog: CatalogResponse | undefined; isReady: boolean } {
    const [catalogCollection] = useStore('automation_catalog')
    // Ordered by ref so the menus built from this catalog are stable. Without
    // it row order is whatever the store hands back, which varies run to run:
    // TriggerCard groups by package in encounter order, so a package could
    // land anywhere in the list and, in a viewport-height popover, below the
    // fold — where it renders but can't be clicked.
    const { data: rows, isReady } = useOrgLiveQuery(query =>
        query
            .from({ automation_catalog: catalogCollection })
            .orderBy(({ automation_catalog }) => automation_catalog.ref)
    )
    // definition is a json column: tolerate malformed rows (skip, don't throw) —
    // the engine owns the writes, but a version-skewed client must not crash.
    const catalog = useMemo(() => {
        if (!rows) return undefined
        const triggers: CatalogTrigger[] = []
        const actions: CatalogAction[] = []
        for (const row of rows) {
            const def = row.definition as CatalogTrigger | CatalogAction | null
            if (!def || typeof def !== 'object' || !('ref' in def)) continue
            if (row.kind === 'trigger') triggers.push(def as CatalogTrigger)
            if (row.kind === 'action')
                actions.push({ ...(def as CatalogAction), available: row.available })
        }
        return { triggers, actions }
    }, [rows])
    return { catalog, isReady }
}
