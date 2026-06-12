import readline from 'node:readline'
import type { Readable } from 'node:stream'
import { parseCurrentIdFromLogLine } from './identity'

interface WaitOptions {
    // Resolve as soon as a parsed currentId satisfies this.
    predicate: (currentId: string) => boolean
    timeoutMs: number
    // Called for every parsed currentId — used to surface the last-seen value
    // when the wait times out (no silent failures).
    onSeen?: (currentId: string) => void
}

// Tail a server log stream line-by-line, parsing each /api/app/update request's
// q.currentId. Resolves with the first id matching `predicate`. Rejects on
// timeout, embedding the last-seen id so a failure says what the app WAS
// reporting (e.g. still on the embedded bundle → never reloaded).
export function waitForCurrentId(stream: Readable, opts: WaitOptions): Promise<string> {
    const { predicate, timeoutMs, onSeen } = opts
    return new Promise<string>((resolve, reject) => {
        let lastSeen: string | null = null
        let settled = false
        const rl = readline.createInterface({ input: stream })

        function cleanup() {
            clearTimeout(timer)
            rl.close()
        }

        // Single settle path so any one outcome (match, timeout, stream end,
        // stream error) wins exactly once. rl.close() inside cleanup() itself
        // emits 'close', so the guard keeps that from re-rejecting.
        function finish(fn: () => void) {
            if (settled) return
            settled = true
            cleanup()
            fn()
        }

        const timer = setTimeout(() => {
            finish(() =>
                reject(
                    new Error(
                        `waitForCurrentId timed out after ${timeoutMs}ms; ` +
                            `last-seen currentId=${lastSeen ?? '<none>'}`
                    )
                )
            )
        }, timeoutMs)

        rl.on('line', line => {
            const id = parseCurrentIdFromLogLine(line)
            if (id === null) return
            lastSeen = id
            onSeen?.(id)
            if (predicate(id)) finish(() => resolve(id))
        })

        rl.on('close', () => {
            finish(() =>
                reject(
                    new Error(
                        'waitForCurrentId: stream ended before a match; ' +
                            `last-seen currentId=${lastSeen ?? '<none>'}`
                    )
                )
            )
        })

        // readline forwards a source-stream 'error' to the Interface instance,
        // so listen on both: the stream for the original event and rl to absorb
        // the re-emit (an unhandled 'error' on the Interface would crash). The
        // `finish` guard makes the second one a no-op.
        const onError = (err: Error) => finish(() => reject(err))
        stream.on('error', onError)
        rl.on('error', onError)
    })
}
