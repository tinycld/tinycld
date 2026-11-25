// A short code a person types to name a record (a board key, a customer code), and the
// free variant of a key. deriveCode optimizes for "recognizable at a glance": a name of
// several words gives its initials (Product Launch → PL), one word gives itself
// (Roadmap → ROADMAP). Diacritics fold first, so "Café" gives CAFE. Uniqueness belongs
// to the caller: uniqueKey picks a free variant, and the caller's unique index is the
// backstop.

export const SHORT_CODE_MIN_LENGTH = 2

// '' when the name gives fewer than SHORT_CODE_MIN_LENGTH characters (no ASCII letters or
// digits, or one only); the caller then uses its own fallback or none.
export function deriveCode(name: string, opts: { max: number }): string {
    const folded = name.normalize('NFD').replace(/[̀-ͯ]/g, '')
    const words = folded.split(/[^A-Za-z0-9]+/).filter(Boolean)
    if (words.length === 0) return ''
    const raw = words.length > 1 ? words.map(word => word[0]).join('') : words[0]
    const code = raw.toUpperCase().slice(0, opts.max)
    return code.length >= SHORT_CODE_MIN_LENGTH ? code : ''
}

// base when it is free, else the first free base<sep>2, base<sep>3, …, with base cut so
// the whole key fits in max. The defaults are the forms builder's field keys.
export function uniqueKey(
    base: string,
    taken: ReadonlySet<string>,
    opts: { separator?: string; max?: number } = {}
): string {
    const separator = opts.separator ?? '_'
    const max = opts.max ?? 40
    if (!taken.has(base)) return base
    for (let n = 2; ; n++) {
        const suffix = `${separator}${n}`
        const candidate = `${base.slice(0, max - suffix.length)}${suffix}`
        if (!taken.has(candidate)) return candidate
    }
}
