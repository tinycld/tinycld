import { ClientResponseError } from 'pocketbase'
import { describe, expect, it, vi } from 'vitest'
import { inviteErrorsOnForm } from '../use-invite-member'

function makeForm() {
    return {
        setError: vi.fn(),
        getValues: () => ({ username: 'alice', email: '', role: 'member' as const }),
    }
}

function responseError(status: number, data: Record<string, unknown>) {
    return new ClientResponseError({ url: '/api/invite-member', status, response: data })
}

describe('inviteErrorsOnForm', () => {
    it('puts a refusal with no field errors on the form as a whole', () => {
        const form = makeForm()
        inviteErrorsOnForm(form)(responseError(403, { message: 'Your plan has no free seats.' }))
        expect(form.setError).toHaveBeenCalledWith('root', {
            type: 'server',
            message: 'Your plan has no free seats.',
        })
    })
    it('puts a field error on its field', () => {
        const form = makeForm()
        inviteErrorsOnForm(form)(
            responseError(400, {
                message: 'Failed.',
                data: { username: { code: 'taken', message: 'That username is taken.' } },
            })
        )
        expect(form.setError).toHaveBeenCalledWith('username', {
            type: 'manual',
            message: 'That username is taken.',
        })
        expect(form.setError).not.toHaveBeenCalledWith('root', expect.anything())
    })
})
