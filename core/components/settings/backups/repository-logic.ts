import { z } from '@tinycld/core/ui/form'

// Pure helpers behind the Repository card, testable without rendering.

export const pbsSchema = z.object({
    server: z.string().min(1, 'Enter the PBS server'),
    fingerprint: z.string(),
    datastore: z.string().min(1, 'Enter the datastore'),
    namespace: z.string(),
    authId: z.string().regex(/^[^@\s]+@[^!\s]+![^\s]+$/, 'Use an API token: user@realm!name'),
    secret: z.string().min(1, 'Enter the token secret'),
    key: z.string(),
    schedule: z.string().min(1, 'Enter a cron schedule'),
    enabled: z.boolean(),
})

export type PbsForm = z.infer<typeof pbsSchema>

export const DEFAULT_SCHEDULE = '0 3 * * *'

export function parseStoredConfig(
    raw: string | undefined,
    schedule: string | undefined,
    enabled: string | undefined
): PbsForm {
    let stored: Record<string, string> = {}
    try {
        stored = raw ? JSON.parse(raw) : {}
    } catch {
        stored = {}
    }
    return {
        server: stored.server ?? '',
        fingerprint: stored.fingerprint ?? '',
        datastore: stored.datastore ?? '',
        namespace: stored.namespace ?? '',
        authId: stored.auth_id ?? '',
        secret: stored.secret ?? '',
        key: stored.key ?? '',
        schedule: schedule || DEFAULT_SCHEDULE,
        enabled: enabled === 'true',
    }
}

export function toStoredConfig(form: PbsForm): string {
    return JSON.stringify({
        server: form.server.trim(),
        fingerprint: form.fingerprint.trim(),
        datastore: form.datastore.trim(),
        namespace: form.namespace.trim(),
        auth_id: form.authId.trim(),
        secret: form.secret,
        key: form.key,
    })
}

export function formatDedup(bytes: number, uploaded: number): string {
    if (!bytes) return ''
    const saved = Math.round((1 - uploaded / bytes) * 100)
    return `${saved}% deduplicated`
}
