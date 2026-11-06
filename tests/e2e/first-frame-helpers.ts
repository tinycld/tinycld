import type { Page } from '@playwright/test'

/** What a surface paints on the first frame after it enters the DOM. */
export interface FirstFrame {
    /** The page's own reading of the OS setting, the precondition of a reduced-motion assert. */
    reducedMotion: boolean
    /** The computed animation-name: set inline, it runs only if its @keyframes are defined. */
    animationName: string
    /** The animations running on the surface: a CSS animation by its name. */
    animations: string[]
    opacity: string
    transform: string
    /** The transform moves nothing ('none', or a matrix that is the identity). */
    isAtRest: boolean
}

type FirstFrameWatch = { firstFrame?: Promise<FirstFrame> }

/**
 * Waits for the first element that matches the selector to enter the DOM, then reads it
 * in the next animation frame, before that frame paints. A 120 ms open animation still
 * runs then, so an animation that ran and ended cannot pass as none. Awaited before the
 * action that opens the surface, so the watch is in place first; readFirstFrame collects it.
 */
export function watchFirstFrame(page: Page, selector: string) {
    return page.evaluate(selector => {
        const watch = window as unknown as FirstFrameWatch
        watch.firstFrame = new Promise(resolve => {
            const read = (el: HTMLElement): FirstFrame => {
                const style = getComputedStyle(el)
                return {
                    reducedMotion: matchMedia('(prefers-reduced-motion: reduce)').matches,
                    animationName: style.animationName,
                    animations: el
                        .getAnimations()
                        .map(a =>
                            a instanceof CSSAnimation ? a.animationName : a.constructor.name
                        ),
                    opacity: style.opacity,
                    transform: style.transform,
                    isAtRest:
                        style.transform === 'none' || new DOMMatrix(style.transform).isIdentity,
                }
            }
            const observer = new MutationObserver(() => {
                const el = document.querySelector<HTMLElement>(selector)
                if (!el) return
                observer.disconnect()
                requestAnimationFrame(() => resolve(read(el)))
            })
            observer.observe(document.body, { childList: true, subtree: true })
        })
    }, selector)
}

export async function readFirstFrame(page: Page): Promise<FirstFrame> {
    const frame = await page.evaluate(() => (window as unknown as FirstFrameWatch).firstFrame)
    if (!frame) throw new Error('watchFirstFrame was not awaited before the surface opened')
    return frame
}
