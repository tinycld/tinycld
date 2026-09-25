function countOf(n: number, one: string, many: string): string {
    return `${n} ${n === 1 ? one : many}`
}

/**
 * "3 apps on · 2 people", leaving out each part that is zero. Mail is not
 * listed: unset delivery reads as on, so it would claim something nobody set up.
 */
export function doneSummaryOf(input: { appCount: number; memberCount: number }): string {
    const parts: string[] = []
    if (input.appCount > 0) parts.push(`${countOf(input.appCount, 'app', 'apps')} on`)
    if (input.memberCount > 0) parts.push(countOf(input.memberCount, 'person', 'people'))
    return parts.join(' · ')
}

/** A skipped workspace step leaves no chosen name, so the heading stays neutral. */
export function doneHeadingOf(name: string): { heading: string; buttonLabel: string } {
    if (!name) return { heading: 'Your workspace is ready', buttonLabel: 'Open your workspace' }
    return { heading: `${name} is ready`, buttonLabel: `Open ${name}` }
}
