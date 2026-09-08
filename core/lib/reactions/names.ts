// How a chip's tooltip names the people behind it.

/** Named in full before the tooltip switches to a count. */
const MAX_NAMED = 3

export interface ReactorNamesOptions {
    /** Index of the caller in `names`, if they are among them. */
    selfIndex?: number
}

/**
 * "You, Nathan and 2 others reacted 👍".
 *
 * The caller is named first and as "You": their own reaction is the one they
 * are most likely to be checking, and reading your own name back is odd.
 *
 * Truncates rather than listing everyone, because this renders in an overlay
 * anchored to a chip and an unbounded list would need its own scroll region.
 */
export function formatReactors(
    names: readonly string[],
    emoji: string,
    { selfIndex }: ReactorNamesOptions = {}
): string {
    if (names.length === 0) return ''

    const ordered =
        selfIndex === undefined || selfIndex < 0
            ? [...names]
            : ['You', ...names.filter((_, index) => index !== selfIndex)]

    const named = ordered.slice(0, MAX_NAMED)
    const remaining = ordered.length - named.length

    return `${joinNames(named, remaining)} reacted ${emoji}`
}

function joinNames(named: readonly string[], remaining: number): string {
    if (remaining > 0) {
        return `${named.join(', ')} and ${remaining} ${remaining === 1 ? 'other' : 'others'}`
    }
    if (named.length === 1) return named[0]
    return `${named.slice(0, -1).join(', ')} and ${named[named.length - 1]}`
}
