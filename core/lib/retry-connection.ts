// The connection options' Retry action, with its effects injected so it tests
// without a network. use-connection-options.ts wires the real ones.

export interface RetryConnectionDeps {
    /** The server the app is connected to; null before one is chosen. */
    address: string | null
    /** The health check: resolves when the server answers. */
    probe: (address: string) => Promise<void>
    /** Reload every live collection, which also wakes their waiting retries. */
    reload: () => Promise<void>
    setServerReachable: (reachable: boolean) => void
    onReloadError: (error: unknown) => void
}

/**
 * Checks the server and reloads every live collection. Resolves once the
 * health check answers and rejects when it does not.
 *
 * The reload is started, not awaited: while the server is down a reload waits
 * out its loads' retries, and the Retry button must not spin that long.
 */
export async function retryConnection(deps: RetryConnectionDeps): Promise<void> {
    deps.reload().catch(deps.onReloadError)
    if (!deps.address) throw new Error('No server is chosen.')
    await deps.probe(deps.address)
    deps.setServerReachable(true)
}
