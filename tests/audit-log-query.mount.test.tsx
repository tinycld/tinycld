// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { auditWindowSize } from '@tinycld/core/lib/audit-log-page'
import { afterEach, expect, test, vi } from 'vitest'

// `audit_logs` is unbounded history and the screen's default view has no filter,
// so the bounding lives entirely in the compiled query. A fixture-based test
// cannot guard that: with fifty rows in the store, dropping `.limit()` still
// renders the same list and still passes. So this mounts the real screen against
// real TanStack DB collections and inspects the query the component built —
// deleting `.limit(auditWindowSize(page))` from audit-log.tsx fails it.
//
// The companion core/tests/unit/audit-log-page.test.ts covers the paging
// arithmetic; it deliberately claims nothing about the query.

afterEach(cleanup)
afterEach(() => {
    seenQueries.length = 0
})

const h = vi.hoisted(() => ({
    logs: Array.from({ length: 4 }, (_, i) => ({
        id: `log${i}`,
        action: 'created',
        resource_type: 'contacts',
        resource_id: `r${i}`,
        resource_label: `Row ${i}`,
        actor: 'u1',
        changes: null,
        snapshot: null,
        created: `2026-09-2${i} 10:00:00`,
        updated: '',
    })),
    users: [{ id: 'u1', name: 'Ada', email: 'ada@example.com', created: '', updated: '' }],
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const mk = (id: string, initialData: { id: string }[]) =>
        createCollection({
            ...localOnlyCollectionOptions({
                id,
                // Typed as string | number: TanStack DB 0.12's BasicIndex constructor is
                // invariant in the key type and is declared for string | number keys.
                getKey: (r: { id: string }): string | number => r.id,
                initialData,
            }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        audit_logs: mk('audit_logs', h.logs),
        users: mk('users', h.users),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

vi.mock('@tinycld/core/lib/use-current-role', () => ({
    useCurrentRole: () => ({ isAdmin: true, isOwner: true, role: 'admin' }),
}))

vi.mock('@tinycld/core/lib/org-routes', () => ({
    useOrgHref: () => (p: string) => `/${p}`,
}))

vi.mock('@tinycld/core/lib/use-navigate-back', () => ({
    useNavigateBack: () => () => {},
}))

// Debounce is a real timer; the query shape is what this test reads, so the term
// must reach it without advancing clocks.
vi.mock('@tinycld/core/lib/use-debounced-value', () => ({
    useDebouncedValue: <T,>(value: T) => value,
}))

// Capture each compiled query so the test asserts on the query itself, not on
// what the fixture happened to render.
const seenQueries: { limit?: unknown; where: string; rawWhere?: unknown }[] = []

const serialize = (value: unknown): string => {
    const seen = new WeakSet()
    return JSON.stringify(value, (key, v) => {
        if (key === 'collection') return undefined
        if (typeof v === 'object' && v !== null) {
            if (seen.has(v)) return undefined
            seen.add(v)
        }
        return v
    })
}

type Built = { query?: { from?: unknown; limit?: unknown; where?: unknown } } | null | undefined
type QueryFn = (q: never) => Built

const recordQuery = (fn: QueryFn) => (q: never) => {
    const built = fn(q)
    if (built?.query && serialize(built.query.from).includes('audit_logs')) {
        seenQueries.push({
            limit: built.query.limit,
            where: serialize(built.query.where),
            rawWhere: built.query.where,
        })
    }
    return built as never
}

vi.mock('@tanstack/react-db', async () => {
    const actual = await vi.importActual<typeof import('@tanstack/react-db')>('@tanstack/react-db')
    return {
        ...actual,
        useLiveQuery: (arg: QueryFn | { query: QueryFn }, deps?: unknown[]) => {
            const config = typeof arg === 'function' ? { query: arg } : arg
            return actual.useLiveQuery(
                { ...config, query: recordQuery(config.query) } as never,
                deps as never
            )
        },
    }
})

import AuditLogSettings from '../app/a/(app)/settings/audit-log'

const mount = () =>
    render(
        <QueryClientProvider client={new QueryClient()}>
            <AuditLogSettings />
        </QueryClientProvider>
    )

// The react-native stub forwards props straight onto a custom element, so RN's
// `testID` lands as a lowercase `testid` attribute rather than `data-testid`.
const byTestId = (container: HTMLElement, id: string) => {
    const el = container.querySelector(`[testid="${id}"]`)
    return el as HTMLElement | null
}

// The stub's TextInput listens for a DOM `input` event, which is how a user
// types into a real field.
const type = (container: HTMLElement, id: string, value: string) => {
    const el = byTestId(container, id)
    if (!el) throw new Error(`no element with testID ${id}`)
    fireEvent.input(el, { target: { value } })
}

// The literal values a compiled expression tree carries — these are what pbtsdb
// hands to escapeValue, so they are the exact strings the filter is built from.
const literalsOf = (node: unknown, out: string[] = []): string[] => {
    if (typeof node === 'string') out.push(node)
    else if (Array.isArray(node)) for (const child of node) literalsOf(child, out)
    else if (node && typeof node === 'object')
        for (const child of Object.values(node)) literalsOf(child, out)
    return out
}

const auditQuery = async () => {
    await waitFor(() => expect(seenQueries.length).toBeGreaterThan(0))
    const last = seenQueries[seenQueries.length - 1]
    if (!last) throw new Error('no audit_logs query was compiled')
    return last
}

test('the audit list query is capped at one page — never an unbounded read', async () => {
    mount()
    const { limit } = await auditQuery()
    expect(limit).toBe(auditWindowSize(1))
})

test('the default view carries no where clause, so the cap is the only bound', async () => {
    mount()
    const { where } = await auditQuery()
    // Nothing is filtered by default, which is exactly why the limit above is
    // load-bearing rather than a nicety.
    expect(where === undefined || where === 'null' || where === '[]').toBe(true)
})

test('a search term compiles to like predicates on the three searchable columns', async () => {
    const { container } = mount()
    await auditQuery()
    type(container, 'audit-search', 'Row 2')

    await waitFor(async () => {
        const { where } = await auditQuery()
        expect(where).toContain('like')
    })
    const { where, limit } = await auditQuery()
    expect(where).toContain('%Row 2%')
    expect(where).toContain('resource_label')
    expect(where).toContain('resource_type')
    // The cap survives a search: filtering is not an excuse to drop the bound.
    expect(limit).toBe(auditWindowSize(1))
})

test('a search term with a backslash cannot reach the filter and break its quoting', async () => {
    const { container } = mount()
    await auditQuery()
    type(container, 'audit-search', 'a\\"b')

    await waitFor(async () => {
        const { where } = await auditQuery()
        expect(where).toContain('like')
    })
    // pbtsdb's escapeValue escapes `"` but not `\`, so a surviving backslash
    // would compile to an unterminated filter literal and PocketBase would 400.
    // Asserted on the literal the expression carries, not on its JSON rendering,
    // because JSON.stringify escapes a quote and would hide the difference.
    const { rawWhere } = await auditQuery()
    // The search terms are the only literals wrapped in `%`; field names are not.
    const terms = literalsOf(rawWhere).filter(v => v.startsWith('%') && v.endsWith('%'))
    expect(terms.length).toBeGreaterThan(0)
    for (const term of terms) {
        expect(term).not.toContain('\\')
        expect(term).toBe('%a"b%')
    }
})

test('Load more is absent when the page came back short of the window', async () => {
    const { container } = mount()
    await auditQuery()
    // Only four rows exist, so the window never filled — the button must not be
    // offered, which is the disable rule asserted against a real mount.
    expect(byTestId(container, 'audit-load-more')).toBeNull()
})
