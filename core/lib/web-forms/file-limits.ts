import type { FormDefinition } from '@tinycld/core/lib/forms/definition'

const MB = 1024 * 1024
const DEFAULT_MAX_FILE_MB = 10

export interface PendingFileInfo {
    name: string
    type: string
    size: number
}

export interface FileLimitIssue {
    path: [string]
    message: string
}

function idsOf(raw: unknown): string[] {
    if (Array.isArray(raw)) return raw.map(String)
    return raw ? [String(raw)] : []
}

/** Reads accept entries as the HTML accept attribute does, like the server. */
export function acceptMatches(accept: string[], file: PendingFileInfo): boolean {
    if (accept.length === 0) return true
    const type = file.type.toLowerCase()
    const dot = file.name.lastIndexOf('.')
    const ext = dot === -1 ? '' : file.name.slice(dot).toLowerCase()
    return accept.some(raw => {
        const a = raw.trim().toLowerCase()
        if (a.startsWith('.')) return a === ext
        if (a.endsWith('/*')) return type.startsWith(a.slice(0, -1))
        return a === type
    })
}

/**
 * The checks the server makes on uploads, made before upload so a visitor
 * hears about a file that is too large or of the wrong type at once. The
 * submission cap is flagged on the last file field that holds files.
 */
export function fileLimitIssues(
    def: FormDefinition,
    values: Record<string, unknown>,
    files: Record<string, PendingFileInfo>,
    maxUploadMb: number
): FileLimitIssue[] {
    const issues: FileLimitIssue[] = []
    let total = 0
    let lastWithFiles: string | null = null
    for (const field of def.fields) {
        if (field.kind !== 'file') continue
        const picked = idsOf(values[field.key])
            .map(id => files[id])
            .filter(f => f !== undefined)
        if (picked.length > 0) lastWithFiles = field.key
        const limit = field.maxSizeMb ?? DEFAULT_MAX_FILE_MB
        for (const f of picked) {
            total += f.size
            if (!acceptMatches(field.accept, f)) {
                issues.push({ path: [field.key], message: `${f.name} is not an accepted type` })
                break
            }
            if (f.size > limit * MB) {
                issues.push({ path: [field.key], message: `${f.name} is larger than ${limit} MB` })
                break
            }
        }
    }
    if (issues.length === 0 && lastWithFiles && total > maxUploadMb * MB)
        issues.push({
            path: [lastWithFiles],
            message: `The files together are larger than ${maxUploadMb} MB`,
        })
    return issues
}
