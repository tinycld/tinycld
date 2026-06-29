import { afterEach, describe, expect, it, vi } from 'vitest'

const update = vi.fn()
const authWithPassword = vi.fn()
const captureException = vi.fn()

const authStore = { record: { id: 'user_1' } as { id: string } | null }

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    pb: {
        authStore,
        collection: () => ({ update, authWithPassword }),
    },
}))

vi.mock('@tinycld/core/lib/errors', () => ({
    captureException: (...args: unknown[]) => captureException(...args),
}))

async function importChangePassword() {
    return (await import('@tinycld/core/lib/account-password')).changeMyPassword
}

const args = {
    email: 'a@b.com',
    oldPassword: 'old-secret',
    newPassword: 'new-secret-1',
    passwordConfirm: 'new-secret-1',
}

describe('changeMyPassword', () => {
    afterEach(() => {
        update.mockReset()
        authWithPassword.mockReset()
        captureException.mockReset()
        authStore.record = { id: 'user_1' }
    })

    it('updates the auth record then re-authenticates with the new password', async () => {
        const changeMyPassword = await importChangePassword()
        await changeMyPassword(args)

        expect(update).toHaveBeenCalledWith('user_1', {
            oldPassword: 'old-secret',
            password: 'new-secret-1',
            passwordConfirm: 'new-secret-1',
        })
        expect(authWithPassword).toHaveBeenCalledWith('a@b.com', 'new-secret-1')
    })

    it('throws when there is no authenticated user', async () => {
        authStore.record = null
        const changeMyPassword = await importChangePassword()

        await expect(changeMyPassword(args)).rejects.toThrow('Not authenticated')
        expect(update).not.toHaveBeenCalled()
    })

    it('does not catch update failures (surfaced inline to the form)', async () => {
        const updateError = new Error('old password is invalid')
        update.mockRejectedValueOnce(updateError)
        const changeMyPassword = await importChangePassword()

        await expect(changeMyPassword(args)).rejects.toThrow('old password is invalid')
        expect(authWithPassword).not.toHaveBeenCalled()
        expect(captureException).not.toHaveBeenCalled()
    })

    it('captures and rethrows when re-authentication fails', async () => {
        const reauthError = new Error('reauth failed')
        authWithPassword.mockRejectedValueOnce(reauthError)
        const changeMyPassword = await importChangePassword()

        await expect(changeMyPassword(args)).rejects.toThrow('reauth failed')
        expect(captureException).toHaveBeenCalledWith('account.changePassword.reauth', reauthError)
    })
})
