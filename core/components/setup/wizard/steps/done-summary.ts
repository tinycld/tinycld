function countOf(n: number, one: string, many: string): string {
    return `${n} ${n === 1 ? one : many}`
}

/** "3 apps on · 2 people · email sending on", leaving out each part that is zero. */
export function doneSummaryOf(input: {
    appCount: number
    memberCount: number
    isMailOn: boolean
}): string {
    const parts: string[] = []
    if (input.appCount > 0) parts.push(`${countOf(input.appCount, 'app', 'apps')} on`)
    if (input.memberCount > 0) parts.push(countOf(input.memberCount, 'person', 'people'))
    if (input.isMailOn) parts.push('email sending on')
    return parts.join(' · ')
}
