import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { solveChallenge } from '../pow-solver'

const digest = async (s: string) => createHash('sha256').update(s).digest('hex')

describe('solveChallenge', () => {
    it('finds the number and encodes the solution', async () => {
        const salt = 'abc?pub=p&expires=1'
        const challenge = await digest(`${salt}1234`)
        const encoded = await solveChallenge(
            { algorithm: 'SHA-256', challenge, maxnumber: 5000, salt, signature: 'sig' },
            { batch: 100, digest }
        )
        expect(JSON.parse(atob(encoded))).toEqual({
            algorithm: 'SHA-256',
            challenge,
            number: 1234,
            salt,
            signature: 'sig',
        })
    })

    it('rejects when no number matches', async () => {
        await expect(
            solveChallenge(
                {
                    algorithm: 'SHA-256',
                    challenge: 'x',
                    maxnumber: 10,
                    salt: 's',
                    signature: 'sig',
                },
                { batch: 4, digest }
            )
        ).rejects.toThrow(/no solution/)
    })

    it('hashes with Web Crypto by default', async () => {
        const salt = 's?pub=p&expires=1'
        const challenge = await digest(`${salt}7`)
        const encoded = await solveChallenge({
            algorithm: 'SHA-256',
            challenge,
            maxnumber: 20,
            salt,
            signature: 'sig',
        })
        expect(JSON.parse(atob(encoded)).number).toBe(7)
    })
})
