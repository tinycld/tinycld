import type { Href } from 'expo-router'

export interface EventSourceRange {
    start: Date
    end: Date
}

/** One read-only item a source contributes to a host's event grid. */
export interface EventSourceItem {
    /** Unique within the source — typically the backing record's id. */
    id: string
    title: string
    /** ISO datetime. For allDay items: local midnight of the day. */
    start: string
    /** ISO datetime. For allDay items: local end of the day. */
    end: string
    allDay: boolean
    /** Where a press navigates — already org-scoped (built with useOrgHref). */
    href: Href
}

/**
 * The shape an `eventSources` manifest entry's `module` must export.
 *
 * The host mounts one collector component per source and calls this hook
 * unconditionally on every render of that collector, so it must obey the
 * rules of hooks — a live query (useOrgLiveQuery) is the expected
 * implementation. Items are read-only on the host grid: no drag, no edit;
 * a press navigates to `href`. The hook must import only @tinycld/core and
 * its own package (lean shell: the host may be absent).
 */
export interface EventSourceModule {
    useEventSource: (range: EventSourceRange) => {
        items: EventSourceItem[]
        isLoading: boolean
    }
}
