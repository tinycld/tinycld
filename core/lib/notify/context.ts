/**
 * Who a notification is for. Single-org: the user id is the whole answer, and
 * carrying an orgId here is what made bell notifications dead app-wide — the
 * component that publishes this gated on an org id that no longer exists.
 */
export type NotifyContext = {
    userId: string
}

let current: NotifyContext | null = null

export function setNotifyContext(ctx: NotifyContext): void {
    current = ctx
}

export function clearNotifyContext(): void {
    current = null
}

export function getNotifyContext(): NotifyContext | null {
    return current
}
