import { afterEach, describe, expect, it, vi } from 'vitest'
import { refusalBodyOf } from '../ClaimServerStep'
import { claimSummary } from '../claim-summary'
import { formatSetupCode, postSetup, SetupRequestError } from '../setup-api'

vi.mock('@tinycld/core/lib/config', () => ({ PB_SERVER_ADDR: 'http://server' }))

async function failureOf(request: Promise<unknown>): Promise<SetupRequestError> {
    const error = await request.catch(e => e)
    if (!(error instanceof SetupRequestError)) throw new Error('expected a SetupRequestError')
    return error
}

afterEach(() => {
    vi.unstubAllGlobals()
})

describe('postSetup', () => {
    it('posts JSON and returns the body', async () => {
        const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ a: 1 }) })
        vi.stubGlobal('fetch', fetchMock)
        await expect(postSetup('verify', { code: 'K7QM3XPD' })).resolves.toEqual({ a: 1 })
        expect(fetchMock).toHaveBeenCalledWith(
            'http://server/api/setup/verify',
            expect.objectContaining({ method: 'POST', body: '{"code":"K7QM3XPD"}' })
        )
    })

    it('carries the refusal body of a 403', async () => {
        const body = { error: 'This server is already set up.', reason: 'done' }
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue({ ok: false, status: 403, json: async () => body })
        )
        const error = await failureOf(postSetup('init', {}))
        expect(error.status).toBe(403)
        expect(refusalBodyOf(error)).toEqual(body)
    })

    it('reads a failed request as no answer', async () => {
        vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
        const error = await failureOf(postSetup('verify', {}))
        expect(error.status).toBeNull()
        expect(refusalBodyOf(error)).toBeNull()
    })

    it('keeps a refusal without a readable body distinct from no answer', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue({
                ok: false,
                status: 500,
                json: async () => {
                    throw new SyntaxError('bad json')
                },
            })
        )
        const error = await failureOf(postSetup('verify', {}))
        expect(refusalBodyOf(error)).toEqual({})
    })
})

describe('formatSetupCode', () => {
    it('adds the dash to a link code', () => {
        expect(formatSetupCode('k7qm3xpd')).toBe('K7QM-3XPD')
    })
    it('shows nothing for a partial code', () => {
        expect(formatSetupCode('K7Q')).toBe('')
    })
})

describe('claimSummary', () => {
    it('marks the code step done once verified', () => {
        expect(claimSummary(false).steps.map(s => s.phase)).toEqual(['todo', 'todo'])
        const done = claimSummary(true)
        expect(done.steps.map(s => s.phase)).toEqual(['done', 'todo'])
        expect(done.doneCount).toBe(1)
    })
})
