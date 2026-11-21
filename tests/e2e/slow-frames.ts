import type { Page } from '@playwright/test'

/**
 * Delays every `requestAnimationFrame` callback of the page by `delayMs`, as a
 * busy machine does. A spec uses it to make a gap certain that CI load opens
 * only at times: work deferred to the next frame has not run yet when the
 * next step of the test acts. Call before the first navigation.
 */
export async function slowAnimationFrames(page: Page, delayMs = 500) {
    await page.addInitScript(delay => {
        const nextFrame = window.requestAnimationFrame.bind(window)
        const cancelFrame = window.cancelAnimationFrame.bind(window)
        const frames = new Map<number, number>()
        window.requestAnimationFrame = callback => {
            const timer = window.setTimeout(() => {
                frames.set(
                    timer,
                    nextFrame(time => {
                        frames.delete(timer)
                        callback(time)
                    })
                )
            }, delay)
            return timer
        }
        window.cancelAnimationFrame = handle => {
            window.clearTimeout(handle)
            const frame = frames.get(handle)
            if (frame !== undefined) cancelFrame(frame)
            frames.delete(handle)
        }
    }, delayMs)
}
