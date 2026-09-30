import { describe, expect, it } from 'vitest'
import { formatDedup, parseStoredConfig, pbsSchema, toStoredConfig } from '../repository-logic'

describe('repository config', () => {
    const form = {
        server: 'pbs.example:8007',
        fingerprint: 'aa:bb',
        datastore: 'store',
        namespace: '',
        authId: 'tinycld@pbs!backup',
        secret: 's3cret',
        key: '',
        schedule: '0 3 * * *',
        enabled: true,
    }

    it('round-trips through the stored JSON', () => {
        expect(parseStoredConfig(toStoredConfig(form), '0 3 * * *', 'true')).toEqual(form)
    })

    it('maps camelCase fields to the server names', () => {
        expect(JSON.parse(toStoredConfig(form))).toMatchObject({ auth_id: 'tinycld@pbs!backup' })
    })

    it('rejects an auth id that is not an API token', () => {
        expect(pbsSchema.safeParse({ ...form, authId: 'root@pam' }).success).toBe(false)
    })

    it('returns empty fields for nothing stored', () => {
        expect(parseStoredConfig(undefined, undefined, undefined).server).toBe('')
    })

    const pbsRun = {
        repository: 'pbs',
        status: 'succeeded',
        bytes: 1000,
        uploaded_bytes: 100,
    } as const

    it('describes what deduplication saved', () => {
        expect(formatDedup(pbsRun)).toBe('90% deduplicated')
        expect(formatDedup({ ...pbsRun, uploaded_bytes: 0 })).toBe('100% deduplicated')
        expect(formatDedup({ ...pbsRun, bytes: 0, uploaded_bytes: 0 })).toBe('')
    })

    // uploaded_bytes is 0 on every row that never measured it, which would
    // otherwise read as "100% deduplicated".
    it('says nothing for a row that did not measure deduplication', () => {
        expect(formatDedup({ ...pbsRun, repository: '', uploaded_bytes: 0 })).toBe('')
        expect(formatDedup({ ...pbsRun, repository: 'archive', uploaded_bytes: 0 })).toBe('')
        expect(formatDedup({ ...pbsRun, status: 'failed', uploaded_bytes: 0 })).toBe('')
        expect(formatDedup({ ...pbsRun, status: 'running' })).toBe('')
    })
})
