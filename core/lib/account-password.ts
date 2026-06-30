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

/**
 * Request a password-reset email for the given address. Uses PocketBase's
 * built-in flow (PB mints the token and owns its lifecycle); the outbound email
 * is overridden server-side to send via the core mailer with a link to the
 * app's own /reset-password/<token> screen.
 *
 * Failures are reported but NOT rethrown: the login UI always shows the same
 * "if an account exists, we've sent a link" confirmation so the response can't
 * be used to discover whether an email is registered.
 */
export async function requestPasswordReset(email: string): Promise<void> {
    try {
        await pb.collection('users').requestPasswordReset(email)
    } catch (err) {
        captureException('account.requestPasswordReset', err)
    }
}

/**
 * Complete a password reset using the token from the emailed link. Errors here
 * (invalid/expired token, weak password) ARE surfaced so the confirm screen can
 * show them to the user.
 */
export async function confirmPasswordReset(
    token: string,
    password: string,
    passwordConfirm: string
): Promise<void> {
    await pb.collection('users').confirmPasswordReset(token, password, passwordConfirm)
}
