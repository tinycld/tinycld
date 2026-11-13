import { describe, expect, it } from 'vitest'
import { mailListenerEnv } from '../e2e-mail-ports'

describe('mailListenerEnv', () => {
    it('keeps the default ports when the environment names none', () => {
        expect(mailListenerEnv({}, 'primary')).toEqual({ IMAP_ADDR: ':1193' })
        expect(mailListenerEnv({}, 'second')).toEqual({
            IMAP_ADDR: ':1293',
            IMAPS_ADDR: ':2093',
            SMTP_ADDR: ':1687',
            SMTPS_ADDR: ':1466',
        })
    })

    it('takes the primary ports from the environment', () => {
        const env = {
            E2E_IMAP_PORT: '2193',
            E2E_IMAPS_PORT: '2194',
            E2E_SMTP_PORT: '2587',
            E2E_SMTPS_PORT: '2465',
        }
        expect(mailListenerEnv(env, 'primary')).toEqual({
            IMAP_ADDR: ':2193',
            IMAPS_ADDR: ':2194',
            SMTP_ADDR: ':2587',
            SMTPS_ADDR: ':2465',
        })
    })

    it('takes the second instance ports from the _2 names only', () => {
        const env = { E2E_IMAP_PORT: '2193', E2E_SMTP_PORT_2: '3587' }
        expect(mailListenerEnv(env, 'second')).toEqual({
            IMAP_ADDR: ':1293',
            IMAPS_ADDR: ':2093',
            SMTP_ADDR: ':3587',
            SMTPS_ADDR: ':1466',
        })
    })

    it('refuses a value that is not a port', () => {
        expect(() => mailListenerEnv({ E2E_IMAP_PORT: 'imap' }, 'primary')).toThrow(
            /E2E_IMAP_PORT must be a port number/
        )
    })
})
