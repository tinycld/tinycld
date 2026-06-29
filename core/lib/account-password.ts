import { captureException } from '@tinycld/core/lib/errors'
import { pb } from '@tinycld/core/lib/pocketbase'

interface ChangePasswordArgs {
    email: string
    oldPassword: string
    newPassword: string
    passwordConfirm: string
}

// Changing a password is a PocketBase auth-record operation: oldPassword /
// password / passwordConfirm are write-only auth params (not schema fields) and
// PB rotates the session token on success. The pbtsdb store can't carry this —
// it would optimistically write the cleartext password into the local record —
// so this is the documented per-site exception to the raw-PB-access rule. The
// server-side users field guard (users_guard.go) explicitly allows a self
// `password` change; PB's own form validation enforces oldPassword correctness.
export async function changeMyPassword(args: ChangePasswordArgs): Promise<void> {
    const userId = pb.authStore.record?.id
    if (!userId) throw new Error('Not authenticated')

    // biome-ignore lint/plugin/pbtsdb-no-raw-pb-access: auth-record password change; write-only fields + token rotation pbtsdb can't model
    await pb.collection('users').update(userId, {
        oldPassword: args.oldPassword,
        password: args.newPassword,
        passwordConfirm: args.passwordConfirm,
    })

    // PB invalidates the current token on a password change, so re-auth right
    // after to keep the session alive instead of forcing the user to sign in.
    try {
        await pb.collection('users').authWithPassword(args.email, args.newPassword)
    } catch (err) {
        captureException('account.changePassword.reauth', err)
        throw err
    }
}
