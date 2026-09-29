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

    it('describes what deduplication saved', () => {
        expect(formatDedup(1000, 100)).toBe('90% deduplicated')
        expect(formatDedup(0, 0)).toBe('')
    })
})
