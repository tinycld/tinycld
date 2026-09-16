export type HelpTopicId = `${string}:${string}`

export interface HelpTopic {
    id: HelpTopicId
    pkgSlug: string
    topicId: string
    title: string
    summary: string
    tags: string[]
    /**
     * System-settings namespace this topic documents, when it documents one.
     * Topics whose namespace is administered elsewhere are hidden: the screen
     * they describe is not present on this deployment.
     */
    keyPrefix?: string
    body: string
}

export interface HelpGroup {
    packageName: string
    pkgSlug: string
    topics: HelpTopic[]
}

export function parseHelpTopicId(id: string): { pkgSlug: string; topicId: string } | null {
    const i = id.indexOf(':')
    if (i <= 0 || i === id.length - 1) return null
    return { pkgSlug: id.slice(0, i), topicId: id.slice(i + 1) }
}
