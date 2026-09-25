import { describe, expect, it } from 'vitest'
import {
    deliveryEnabledValue,
    fromAddressSchema,
    isDeliveryEnabled,
    rowsToMap,
    sentryDsnSchema,
    shouldPersistSecret,
    vapidSubjectSchema,
} from '../system-settings-logic'

describe('rowsToMap', () => {
    it('keys rows by their `key` and carries the secret flag', () => {
        const m = rowsToMap([
            { key: 'sentry.dsn', id: 'a', value: 'https://x/1', is_secret: false },
            { key: 'vapid.private_key', id: 'b', value: 'priv', is_secret: true },
        ])
        expect(m.get('sentry.dsn')).toEqual({ id: 'a', value: 'https://x/1', isSecret: false })
        expect(m.get('vapid.private_key')?.isSecret).toBe(true)
        expect(m.get('missing')).toBeUndefined()
    })
})

describe('shouldPersistSecret', () => {
    it('persists a non-empty value', () => {
        expect(shouldPersistSecret('a-real-token')).toBe(true)
    })
    it('treats blank / whitespace as "leave unchanged" (write-only)', () => {
        expect(shouldPersistSecret('')).toBe(false)
        expect(shouldPersistSecret('   ')).toBe(false)
    })
})

describe('sentryDsnSchema', () => {
    it('accepts a valid URL and blank (blank = disabled)', () => {
        expect(sentryDsnSchema.safeParse('https://k@o1.ingest.sentry.io/1').success).toBe(true)
        expect(sentryDsnSchema.safeParse('').success).toBe(true)
    })
    it('rejects a non-URL', () => {
        expect(sentryDsnSchema.safeParse('not-a-url').success).toBe(false)
    })
})

describe('vapidSubjectSchema', () => {
    it('accepts a mailto: URL and blank', () => {
        expect(vapidSubjectSchema.safeParse('mailto:admin@example.com').success).toBe(true)
        expect(vapidSubjectSchema.safeParse('').success).toBe(true)
    })
    it('rejects a bare string', () => {
        expect(vapidSubjectSchema.safeParse('admin').success).toBe(false)
    })
})

describe('fromAddressSchema', () => {
    it('accepts a valid email and blank (blank = server default)', () => {
        expect(fromAddressSchema.safeParse('noreply@tinycld.org').success).toBe(true)
        expect(fromAddressSchema.safeParse('').success).toBe(true)
    })
    it('rejects a non-email', () => {
        expect(fromAddressSchema.safeParse('not-an-email').success).toBe(false)
    })
})

describe('delivery-enabled mapping', () => {
    it('maps the toggle boolean to a stored string', () => {
        expect(deliveryEnabledValue(true)).toBe('true')
        expect(deliveryEnabledValue(false)).toBe('false')
    })
    it('treats absent / anything-but-"false" as enabled', () => {
        expect(isDeliveryEnabled(undefined)).toBe(true)
        expect(isDeliveryEnabled('true')).toBe(true)
        expect(isDeliveryEnabled('')).toBe(true)
        expect(isDeliveryEnabled('false')).toBe(false)
    })
})
