// A report open on screen runs again when a collection it reads changes. Change
// signals come in bursts (a posted entry writes several rows), so the scheduler
// waits for quiet; a signal that arrives while a run is in flight queues one more
// run after it, so the figures shown are never older than the last change.

export const RERUN_DELAY_MS = 250

export interface RerunScheduler {
    signal(): void
    dispose(): void
}

export function createRerunScheduler(input: {
    delayMs: number
    run: () => Promise<void>
    onError: (error: unknown) => void
}): RerunScheduler {
    let timer: ReturnType<typeof setTimeout> | null = null
    let running = false
    let queued = false
    let disposed = false

    const arm = () => {
        if (timer) clearTimeout(timer)
        timer = setTimeout(start, input.delayMs)
    }

    function start() {
        timer = null
        running = true
        input
            .run()
            .catch(input.onError)
            .finally(() => {
                running = false
                if (queued && !disposed) {
                    queued = false
                    arm()
                }
            })
    }

    return {
        signal() {
            if (disposed) return
            if (running) {
                queued = true
                return
            }
            arm()
        },
        dispose() {
            disposed = true
            if (timer) clearTimeout(timer)
            timer = null
        },
    }
}
