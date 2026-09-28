import { describe, expect, it, vi } from 'vitest'

// Every core collection must be on-demand + realtime:'query' (the `onDemand`
// const in pocketbase.ts). This is not introspectable on a built collection —
// pbtsdb keeps `syncMode`/`realtime` internal — so the factory is wrapped and
// the options each `newCollection(...)` call actually passed are recorded.
// Wrapping the REAL factory (not a stub) keeps the assertion honest: the module
// still builds working collections, so a call that stops compiling or throws
// fails here rather than passing vacuously.
const h = vi.hoisted(() => ({
    calls: [] as { name: string; options: Record<string, unknown> }[],
}))

vi.mock('pbtsdb', async () => {
    const actual = await vi.importActual<typeof import('pbtsdb')>('pbtsdb')
    return {
        ...actual,
        createCollection: ((...factoryArgs: Parameters<typeof actual.createCollection>) => {
            const build = actual.createCollection(...factoryArgs)
            return (name: string, options: Record<string, unknown>) => {
                h.calls.push({ name, options })
                return (build as unknown as (n: string, o: unknown) => unknown)(name, options)
            }
        }) as unknown as typeof actual.createCollection,
    }
})

// The collections core registers itself, in pocketbase.ts. Listed explicitly
// rather than derived from the recorded calls so that dropping a registration
// fails this test instead of silently shrinking the expectation.
const CORE_COLLECTIONS = [
    'users',
    'groups',
    'group_members',
    'settings',
    'user_preferences',
    'labels',
    'label_assignments',
    'org_pkg_access',
    'oauth_grants',
    'pkg_registry',
    'pkg_build',
    'system_settings',
    'org_branding',
    'audit_logs',
    'backups',
    'pkg_install_log',
    'notifications',
    'rules',
    'rule_runs',
    'automation_catalog',
    'comment_mentions',
] as const

describe('core collection sync modes', () => {
    it('registers every core collection as on-demand with per-query realtime', async () => {
        await import('@tinycld/core/lib/pocketbase')

        const byName = new Map(h.calls.map(c => [c.name, c.options]))
        expect([...byName.keys()].sort()).toEqual([...CORE_COLLECTIONS].sort())

        for (const name of CORE_COLLECTIONS) {
            const options = byName.get(name)
            expect(options, `${name} was never registered`).toBeDefined()
            expect({ name, ...options }).toMatchObject({
                name,
                syncMode: 'on-demand',
                realtime: 'query',
            })
        }
    })
})
