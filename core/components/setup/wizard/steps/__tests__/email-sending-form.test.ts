import { describe, expect, it } from 'vitest'
import {
    type EmailForm,
    emailSchemaFor,
    emailWritesOf,
    storedEmailSettingsOf,
} from '../EmailSendingForm'

const base: EmailForm = {
    deliveryEnabled: true,
    fromAddress: 'hello@example.com',
    provider: 'postmark',
    postmarkAccountToken: '',
    smtpPublicHostname: '',
}

const row = (key: string, value: string) => [key, { id: key, key, value, isSecret: false }] as const

describe('storedEmailSettingsOf', () => {
    it('reads the stored keys, and reports secrets only as set or not', () => {
        const stored = storedEmailSettingsOf(
            new Map([
                row('mail.delivery_enabled', 'false'),
                row('mail.provider', 'smtp'),
                row('mail.postmark_account_token', 'secret'),
                row('mail.smtp_public_hostname', 'mail.example.com'),
            ])
        )
        expect(stored).toEqual({
            deliveryEnabled: false,
            fromAddress: '',
            provider: 'smtp',
            hasAccountToken: true,
            smtpPublicHostname: 'mail.example.com',
        })
    })
    it('defaults to delivering through Postmark on a new server', () => {
        const stored = storedEmailSettingsOf(new Map())
        expect(stored.deliveryEnabled).toBe(true)
        expect(stored.provider).toBe('postmark')
    })
})

describe('emailSchemaFor', () => {
    it('requires a Postmark account token until one is stored', () => {
        const fresh = emailSchemaFor({ hasAccountToken: false })
        expect(fresh.safeParse(base).success).toBe(false)
        expect(fresh.safeParse({ ...base, postmarkAccountToken: 'tok' }).success).toBe(true)
        expect(emailSchemaFor({ hasAccountToken: true }).safeParse(base).success).toBe(true)
    })
    it('requires an SMTP hostname', () => {
        const schema = emailSchemaFor({ hasAccountToken: false })
        expect(schema.safeParse({ ...base, provider: 'smtp' }).success).toBe(false)
        expect(
            schema.safeParse({ ...base, provider: 'smtp', smtpPublicHostname: 'mx.example.com' })
                .success
        ).toBe(true)
    })
    it('checks nothing about the provider while delivery is off', () => {
        const schema = emailSchemaFor({ hasAccountToken: false })
        expect(schema.safeParse({ ...base, deliveryEnabled: false }).success).toBe(true)
    })
})

describe('emailWritesOf', () => {
    it('stores only the switch and the From address while delivery is off', () => {
        expect(emailWritesOf({ ...base, deliveryEnabled: false }).map(w => w.key)).toEqual([
            'mail.delivery_enabled',
            'mail.from_address',
        ])
    })
    it('writes the account token only when one was entered, as a secret', () => {
        const keys = (data: EmailForm) => emailWritesOf(data).map(w => w.key)
        expect(keys(base)).toEqual(['mail.delivery_enabled', 'mail.from_address', 'mail.provider'])
        expect(keys({ ...base, postmarkAccountToken: 'b' })).toEqual([
            'mail.delivery_enabled',
            'mail.from_address',
            'mail.provider',
            'mail.postmark_account_token',
        ])
        const secret = emailWritesOf({ ...base, postmarkAccountToken: 'b' }).at(-1)
        expect(secret?.isSecret).toBe(true)
    })
    it('writes the SMTP hostname and no Postmark keys for SMTP', () => {
        const writes = emailWritesOf({
            ...base,
            provider: 'smtp',
            smtpPublicHostname: 'mx.example.com',
            postmarkAccountToken: 'ignored',
        })
        expect(writes.map(w => w.key)).toEqual([
            'mail.delivery_enabled',
            'mail.from_address',
            'mail.provider',
            'mail.smtp_public_hostname',
        ])
    })
})
