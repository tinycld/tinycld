// Minimal ambient declaration for fb-watchman (no published @types package).
// Covers only the surface used by dev.ts: capabilityCheck, command, end, on('error').
declare module 'fb-watchman' {
    type Cmd = unknown[]
    type Callback<T = unknown> = (error: Error | null, resp: T) => void

    export class Client {
        capabilityCheck(opts: { optional?: string[]; required?: string[] }, cb: Callback): void
        command<T = unknown>(args: Cmd, cb: Callback<T>): void
        end(): void
        on(event: 'subscription', listener: (resp: SubscriptionEvent) => void): this
        on(event: 'error', listener: (err: Error) => void): this
        on(event: string, listener: (...args: unknown[]) => void): this
    }

    export interface SubscriptionEvent {
        root: string
        subscription: string
        files?: Array<{ name: string; exists: boolean; type?: string }>
        is_fresh_instance?: boolean
    }

    const _default: { Client: typeof Client }
    export default _default
}
