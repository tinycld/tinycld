import { migrateWorkspaceStore as migrate } from '@tinycld/core/lib/stores/workspace-store'
import { describe, expect, it } from 'vitest'

describe('workspace-store v0 → v1 lastPackageHref migration', () => {
    it('prefixes stored v0 hrefs, preserving the query string', () => {
        const out = migrate({ lastPackageHref: { mail: '/mail?folder=sent' } }, 0)
        expect(out.lastPackageHref).toEqual({ mail: '/a/mail?folder=sent' })
    })

    it('prefixes deep hrefs', () => {
        const out = migrate({ lastPackageHref: { drive: '/drive/folder/f1' } }, 0)
        expect(out.lastPackageHref).toEqual({ drive: '/a/drive/folder/f1' })
    })

    it('is idempotent — an already-prefixed value is left alone', () => {
        const out = migrate({ lastPackageHref: { mail: '/a/mail' } }, 0)
        expect(out.lastPackageHref).toEqual({ mail: '/a/mail' })
    })

    it('drops values that are not app paths', () => {
        const out = migrate(
            { lastPackageHref: { mail: 'https://example.com/mail', calc: '/calc' } },
            0
        )
        expect(out.lastPackageHref).toEqual({ calc: '/a/calc' })
    })

    it('leaves v1 state untouched', () => {
        const state = { lastPackageHref: { mail: '/mail' } }
        expect(migrate(state, 1)).toBe(state)
    })

    it('preserves other persisted fields', () => {
        const out = migrate({ isSidebarOpen: true, lastPackageHref: { mail: '/mail' } }, 0)
        expect(out).toMatchObject({ isSidebarOpen: true })
    })
})
