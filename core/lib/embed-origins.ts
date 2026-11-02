import { z } from 'zod'

export const MAX_EMBED_ORIGINS = 10

/**
 * Why one origin cannot frame a page, or null when it can. Mirrors
 * core/server/embedpolicy/origins.go so a person sees the problem while
 * typing; the server still checks.
 */
export function embedOriginProblem(raw: string): string | null {
    const schemeEnd = raw.indexOf('://')
    if (schemeEnd === -1) return `"${raw}" must include a scheme, e.g. https://${raw}`
    if (/[;,'"]/.test(raw)) return `"${raw}" contains an unsupported character`
    const scheme = raw.slice(0, schemeEnd).toLowerCase()
    if (scheme !== 'http' && scheme !== 'https') return `"${raw}" must use http or https`
    const host = raw.slice(schemeEnd + 3).replace(/\/$/, '')
    if (host === '') return `"${raw}" is missing a host`
    if (/[/?#@]/.test(host)) return `"${raw}" must be an origin only, with no path or credentials`
    if (host.includes('*')) return `"${raw}" may not use a wildcard`
    return null
}

/** A space or newline separated list of origins; empty means any website. */
export const embedOriginsSchema = z
    .string()
    .max(2000, 'At most 2000 characters')
    .superRefine((raw, ctx) => {
        const origins = raw.split(/\s+/).filter(Boolean)
        if (origins.length > MAX_EMBED_ORIGINS) {
            ctx.addIssue({ code: 'custom', message: `At most ${MAX_EMBED_ORIGINS} websites` })
            return
        }
        for (const origin of origins) {
            const problem = embedOriginProblem(origin)
            if (problem) {
                ctx.addIssue({ code: 'custom', message: problem })
                return
            }
        }
    })
