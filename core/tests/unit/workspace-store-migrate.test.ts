import { migrateWorkspaceStore as migrate } from '@tinycld/core/lib/stores/workspace-store'
import { describe, expect, it } from 'vitest'

describe('workspace-store v0 → v1 lastPackageHref migration', () => {
    it('prefixes stored v0 hrefs, preserving the query string', () => {
        const out = migrate({ lastPackageHref: { gizmos: '/gizmos?folder=sent' } }, 0)
        expect(out.lastPackageHref).toEqual({ gizmos: '/a/gizmos?folder=sent' })
    })

    it('prefixes deep hrefs', () => {
        const out = migrate({ lastPackageHref: { cogs: '/cogs/folder/f1' } }, 0)
        expect(out.lastPackageHref).toEqual({ cogs: '/a/cogs/folder/f1' })
    })

    it('is idempotent — an already-prefixed value is left alone', () => {
        const out = migrate({ lastPackageHref: { gizmos: '/a/gizmos' } }, 0)
        expect(out.lastPackageHref).toEqual({ gizmos: '/a/gizmos' })
    })

    it('drops values that are not app paths', () => {
        const out = migrate(
            { lastPackageHref: { gizmos: 'https://example.com/gizmos', trinkets: '/trinkets' } },
            0
        )
        expect(out.lastPackageHref).toEqual({ trinkets: '/a/trinkets' })
    })

    it('leaves v1 state untouched', () => {
        const state = { lastPackageHref: { gizmos: '/gizmos' } }
        expect(migrate(state, 1)).toBe(state)
    })

    it('preserves other persisted fields', () => {
        const out = migrate({ isSidebarOpen: true, lastPackageHref: { gizmos: '/gizmos' } }, 0)
        expect(out).toMatchObject({ isSidebarOpen: true })
    })
})
