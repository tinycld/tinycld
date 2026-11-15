import { describe, expect, it } from 'vitest'
import type { ReportPanelDecl } from '../../core/lib/packages/types'
import { buildReportPanelsSource } from '../gen-report-panels'

const pkg = (name: string, slug: string, reportPanels?: ReportPanelDecl[]) => ({
    name,
    manifest: { slug, reportPanels },
})

const gizmos = pkg('@acme/gizmos', 'gizmos', [
    { path: 'history', component: 'components/history/Panel' },
])
const widgets = pkg('@acme/widgets', 'widgets', [
    { path: 'audit/log', component: 'components/AuditPanel' },
])
const plain = pkg('@acme/plain', 'plain')

describe('buildReportPanelsSource', () => {
    it("lists a package's panel with a lazy import of its component", () => {
        const src = buildReportPanelsSource([gizmos])
        expect(src).toContain(
            "import type { ReportPanelEntry } from '@tinycld/core/reports/lib/record-panel'"
        )
        expect(src).toContain('export const reportPanels: ReportPanelEntry[] = [')
        expect(src).toContain(
            `{ slug: "gizmos", path: "history", load: () => import('@acme/gizmos/components/history/Panel') },`
        )
    })

    it("lists every package's panels in package order", () => {
        const src = buildReportPanelsSource([widgets, plain, gizmos])
        const first = src.indexOf('slug: "widgets"')
        expect(first).toBeGreaterThan(-1)
        expect(src.indexOf('slug: "gizmos"')).toBeGreaterThan(first)
        expect(src).toContain("import('@acme/widgets/components/AuditPanel')")
        expect(src).not.toContain('"plain"')
    })

    it('emits an empty list when no package declares a panel', () => {
        for (const features of [[], [plain]]) {
            const src = buildReportPanelsSource(features)
            expect(src).toContain('export const reportPanels: ReportPanelEntry[] = []')
            expect(src).not.toContain('import(')
        }
    })

    it('refuses a component outside the package', () => {
        for (const component of ['../other/Panel', '/abs/Panel', "x'); evil('"]) {
            const bad = pkg('@acme/gizmos', 'gizmos', [{ path: 'history', component }])
            expect(() => buildReportPanelsSource([bad])).toThrow(/reportPanels\[\]\.component/)
        }
    })

    it('refuses a path that is not slash-joined segments of letters, digits, _ and -', () => {
        for (const path of [
            '',
            '/history',
            'history/',
            'a//b',
            '..',
            'a/../b',
            'his tory',
            'his.tory',
        ]) {
            const bad = pkg('@acme/gizmos', 'gizmos', [{ path, component: 'components/Panel' }])
            expect(() => buildReportPanelsSource([bad])).toThrow(/reportPanels\[\]\.path/)
        }
    })

    it('refuses two panels of one package on one path', () => {
        const twice = pkg('@acme/gizmos', 'gizmos', [
            { path: 'history', component: 'components/A' },
            { path: 'history', component: 'components/B' },
        ])
        expect(() => buildReportPanelsSource([twice])).toThrow(/"history" twice/)
        // Two packages may each have a panel at one path: the slug sets them apart.
        expect(() =>
            buildReportPanelsSource([
                gizmos,
                pkg('@acme/other', 'other', [{ path: 'history', component: 'components/P' }]),
            ])
        ).not.toThrow()
    })
})
