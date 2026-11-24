import { describe, expect, it } from 'vitest'
import { searchHelpTopics } from '../search'
import type { HelpGroup } from '../types'
import { visibleGroups as filterGroups } from '../use-help-topics'

// The real filter, tested directly rather than through the hooks, which need a
// React tree and a fetched org-info. No package is blocked unless a case says so.
function visibleGroups(
    all: readonly HelpGroup[],
    managed: readonly string[],
    blocked: ReadonlySet<string> = new Set()
) {
    return filterGroups(all, managed, blocked)
}

const topic = (pkgSlug: string, topicId: string, keyPrefix?: string) => ({
    id: `${pkgSlug}:${topicId}` as const,
    pkgSlug,
    topicId,
    title: topicId,
    summary: '',
    tags: [],
    keyPrefix,
    body: '',
})

const groups: HelpGroup[] = [
    {
        packageName: 'Gizmos',
        pkgSlug: 'gizmos',
        topics: [topic('gizmos', 'provider-setup', 'mail.'), topic('gizmos', 'custom-domains')],
    },
    {
        packageName: 'Push',
        pkgSlug: 'push',
        topics: [topic('push', 'web-push-setup', 'vapid.')],
    },
]

describe('help topic filtering', () => {
    it('hides a topic that documents a setting administered elsewhere', () => {
        const visible = visibleGroups(groups, ['mail.', 'vapid.'])
        const gizmos = visible.find(g => g.pkgSlug === 'gizmos')
        expect(gizmos?.topics.map(t => t.topicId)).toEqual(['custom-domains'])
    })

    // A package left with no topics contributes no section, rather than an
    // empty heading — the same dead end the settings group avoids.
    it('drops a package whose every topic is hidden', () => {
        const visible = visibleGroups(groups, ['vapid.'])
        expect(visible.find(g => g.pkgSlug === 'push')).toBeUndefined()
    })

    // The standalone guarantee: a deployment that administers everything
    // documents everything.
    it('keeps every topic when nothing is managed', () => {
        const visible = visibleGroups(groups, [])
        expect(visible.flatMap(g => g.topics)).toHaveLength(3)
    })

    it('drops a group of a blocked package and keeps the others', () => {
        const visible = visibleGroups(groups, [], new Set(['push']))
        expect(visible.map(g => g.pkgSlug)).toEqual(['gizmos'])
    })

    it('returns the same array when nothing is managed or blocked', () => {
        expect(visibleGroups(groups, [], new Set())).toBe(groups)
    })

    // A deep link resolves against the visible set, so a hidden topic is
    // not-found rather than stale instructions for a screen that is not here.
    it('does not resolve a hidden topic by id', () => {
        const visible = visibleGroups(groups, ['mail.'])
        const byId = new Map(visible.flatMap(g => g.topics).map(t => [t.id, t]))
        expect(byId.get('gizmos:provider-setup')).toBeUndefined()
        expect(byId.get('gizmos:custom-domains')).toBeDefined()
    })
})

describe('help search and images', () => {
    const withImage = {
        ...topic('gizmos', 'img'),
        title: 'Images',
        body: 'Step one ![Create invoice screen](images/zebra-1.png)',
    }

    it('matches image alt text', () => {
        expect(searchHelpTopics([withImage], 'invoice screen')).toHaveLength(1)
    })

    it('does not match the image file path', () => {
        expect(searchHelpTopics([withImage], 'zebra')).toHaveLength(0)
    })
})
