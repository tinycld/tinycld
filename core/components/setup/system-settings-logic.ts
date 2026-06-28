import { z } from '@tinycld/core/ui/form'

// Pure, React/pbtsdb-free logic behind the /admin Settings panels, so the
// field-level rules (upsert decision, write-only secret handling, validation)
// are unit-testable without rendering or mocking the store.

export interface SettingRow {
    id: string
    value: string
    isSecret: boolean
}

/** Map a raw system_settings row list to key → SettingRow. */
export function rowsToMap(
    rows: ReadonlyArray<{ key: string; id: string; value: string; is_secret: boolean }>
): Map<string, SettingRow> {
    return new Map(rows.map(r => [r.key, { id: r.id, value: r.value, isSecret: r.is_secret }]))
}

/**
 * Whether a secret field's submitted value should be written. Write-only fields
 * treat blank as "leave the stored value unchanged" — so we persist only a
 * non-empty (trimmed) value. Non-secret fields always persist (caller decides).
 */
export function shouldPersistSecret(submitted: string): boolean {
    return submitted.trim() !== ''
}

// Field schemas (exported for unit tests + reuse). Blank is allowed where it
// means "disable" (Sentry) or "unchanged" (secret-adjacent), validated as a URL
// otherwise so a typo surfaces before it silently misbehaves.
export const sentryDsnSchema = z.union([
    z.string().url('Enter a valid Sentry DSN URL'),
    z.literal(''),
])

export const vapidSubjectSchema = z.union([
    z.string().url('Use a URL or mailto: URI'),
    z.literal(''),
])
