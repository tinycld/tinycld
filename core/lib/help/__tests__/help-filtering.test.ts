import { describe, expect, it } from 'vitest'
import { isManagedPrefix } from '../../use-managed-settings'
import type { HelpGroup } from '../types'

// The filter use-help-topics applies. Tested against the same rule rather than
// through the hooks, which need a React tree and a fetched org-info.
function visibleGroups(all: HelpGroup[], managed: readonly string[]): HelpGroup[] {
    if (managed.length === 0) return all
    const out: HelpGroup[] = []
    for (const group of all) {
        const topics = group.topics.filter(t => !isManagedPrefix(managed, t.keyPrefix))
        if (topics.length > 0) out.push({ ...group, topics })
    }
    return out
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
        packageName: 'Mail',
        pkgSlug: 'mail',
        topics: [topic('mail', 'provider-setup', 'mail.'), topic('mail', 'custom-domains')],
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
        const mail = visible.find(g => g.pkgSlug === 'mail')
        expect(mail?.topics.map(t => t.topicId)).toEqual(['custom-domains'])
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

    // A deep link resolves against the visible set, so a hidden topic is
    // not-found rather than stale instructions for a screen that is not here.
    it('does not resolve a hidden topic by id', () => {
        const visible = visibleGroups(groups, ['mail.'])
        const byId = new Map(visible.flatMap(g => g.topics).map(t => [t.id, t]))
        expect(byId.get('mail:provider-setup')).toBeUndefined()
        expect(byId.get('mail:custom-domains')).toBeDefined()
    })
})
