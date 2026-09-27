import { describe, expect, it } from 'vitest'
import { buildIssueBody, buildIssueUrl } from '../report-issue'

const baseArgs = {
    repoUrl: 'https://github.com/tinycld/gizmos',
    pkgName: 'Gizmos',
    pkgSlug: 'gizmos',
    pkgVersion: '0.1.0',
    appVersion: '0.0.2',
    commit: 'abc1234',
    platform: 'web',
}

describe('buildIssueUrl', () => {
    it('targets /issues/new and includes encoded title + body', () => {
        const url = buildIssueUrl(baseArgs)
        expect(url.startsWith('https://github.com/tinycld/gizmos/issues/new?')).toBe(true)
        const qs = new URLSearchParams(url.split('?')[1])
        expect(qs.get('title')).toBe('[gizmos] ')
        expect(qs.get('body')).toContain('Package:** Gizmos')
        expect(qs.get('body')).toContain('@tinycld/gizmos')
        expect(qs.get('template')).toBeNull()
    })

    it('strips trailing slashes from repoUrl', () => {
        const url = buildIssueUrl({ ...baseArgs, repoUrl: 'https://github.com/tinycld/gizmos///' })
        expect(url.startsWith('https://github.com/tinycld/gizmos/issues/new?')).toBe(true)
    })

    it('appends template= when issueTemplate is provided', () => {
        const url = buildIssueUrl({ ...baseArgs, issueTemplate: 'bug.yml' })
        const qs = new URLSearchParams(url.split('?')[1])
        expect(qs.get('template')).toBe('bug.yml')
    })
})

describe('buildIssueBody', () => {
    it('includes package, app version, and platform lines', () => {
        const body = buildIssueBody(baseArgs)
        expect(body).toContain('**Package:** Gizmos (`@tinycld/gizmos` v0.1.0)')
        expect(body).toContain('**App version:** 0.0.2 (abc1234)')
        expect(body).toContain('**Platform:** web')
    })
})
