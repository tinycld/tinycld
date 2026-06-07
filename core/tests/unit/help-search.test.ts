import { searchHelpTopics } from '@tinycld/core/lib/help/search'
import type { HelpTopic, HelpTopicId } from '@tinycld/core/lib/help/types'
import { describe, expect, it } from 'vitest'

const topics: HelpTopic[] = [
    {
        id: 'core:themes',
        pkgSlug: 'core',
        topicId: 'themes',
        title: 'Light and dark themes',
        summary: 'Changing the appearance of the app',
        tags: ['theme', 'appearance', 'dark mode'],
        body: 'The app supports a light theme and a dark theme.',
    },
    {
        id: 'core:sharing',
        pkgSlug: 'core',
        topicId: 'sharing',
        title: 'Sharing content',
        summary: 'Generating share links',
        tags: ['share', 'links'],
        body: 'Share tokens live outside the org-scoped tree and are revocable.',
    },
    {
        id: 'mail:compose',
        pkgSlug: 'mail',
        topicId: 'compose',
        title: 'Composing a message',
        summary: 'Write, draft, and send mail',
        tags: ['compose', 'draft'],
        body: 'Open the compose window with the new-message button.',
    },
]

// `id` is a plain test fixture id (e.g. 'zen'); HelpTopic.id is the branded
// `${pkg}:${topic}` template type. These fixtures don't exercise the id format,
// so accept a plain string and brand it for the field.
function makeTopic(id: string, title: string, summary: string, tags: string[] = []): HelpTopic {
    return {
        id: id as HelpTopicId,
        pkgSlug: 'test',
        topicId: id,
        title,
        summary,
        tags,
        body: '',
    }
}

describe('searchHelpTopics', () => {
    it('returns every topic for an empty query', () => {
        expect(searchHelpTopics(topics, '')).toHaveLength(3)
        expect(searchHelpTopics(topics, '   ')).toHaveLength(3)
    })

    it('matches the title, case-insensitively', () => {
        const results = searchHelpTopics(topics, 'THEMES')
        expect(results.map(r => r.topic.id)).toEqual(['core:themes'])
    })

    it('matches a tag', () => {
        const results = searchHelpTopics(topics, 'draft')
        expect(results.map(r => r.topic.id)).toEqual(['mail:compose'])
    })

    it('matches the summary', () => {
        const results = searchHelpTopics(topics, 'generating share')
        expect(results.map(r => r.topic.id)).toEqual(['core:sharing'])
    })

    it('matches the body', () => {
        const results = searchHelpTopics(topics, 'revocable')
        expect(results.map(r => r.topic.id)).toEqual(['core:sharing'])
    })

    it('requires every term to match', () => {
        const results = searchHelpTopics(topics, 'themes draft')
        expect(results).toHaveLength(0)
    })

    it('ranks title hits ahead of body hits', () => {
        const results = searchHelpTopics(topics, 'share')
        expect(results[0]?.topic.id).toBe('core:sharing')
    })

    it('returns an empty array when nothing matches', () => {
        expect(searchHelpTopics(topics, 'xyznomatch')).toEqual([])
    })

    it('ranks word-start matches above mid-word matches in titles', () => {
        const ts: HelpTopic[] = [
            makeTopic('a', 'Italic formatting', 'turn text italic'),
            makeTopic('b', 'Alignment', 'left right center justify'),
        ]
        const results = searchHelpTopics(ts, 'ali')
        expect(results[0]?.topic.id).toBe('b')
    })

    it('ranks tag word-start matches above tag mid-word matches', () => {
        // Mid-word case has "italic" embedded inside a single token
        // (unitalicized); start case has it as the first chars of its
        // own kebab-case token. Hyphens count as word boundaries.
        const ts: HelpTopic[] = [
            makeTopic('mid', 'Unrelated A', 'no match in summary', ['unitalicized']),
            makeTopic('start', 'Unrelated B', 'no match in summary', ['italic-emphasis']),
        ]
        const results = searchHelpTopics(ts, 'italic')
        expect(results[0]?.topic.id).toBe('start')
        expect(results.map(r => r.topic.id)).toContain('mid')
    })

    it('breaks ties by title using case-insensitive locale compare', () => {
        const ts: HelpTopic[] = [
            makeTopic('zen', 'Zen mode', 'focus mode'),
            makeTopic('zebra', 'Zebra striping', 'alternating rows'),
        ]
        const results = searchHelpTopics(ts, 'ze')
        expect(results.map(r => r.topic.id)).toEqual(['zebra', 'zen'])
    })
})
