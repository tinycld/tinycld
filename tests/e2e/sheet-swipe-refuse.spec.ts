import { expect, type Locator, type Page, test } from '@playwright/test'
import { skipWithoutReportsStub } from './helpers'
import { openExportMenu, pressToolbar } from './report-view-helpers'
import { loginAsShippedOwner, openTeamBalance, uniqueName } from './reports-helpers'

/**
 * A swipe down on a core Sheet asks its owner to close; an owner may refuse and
 * keep the sheet open. The sheet must then stay in view, at rest, over its
 * backdrop, so what the owner shows next is seen. On a phone a core Dialog is a
 * Sheet; the report "Email report" dialog refuses to close while its send is
 * on the way. The send is held with `page.route` and answered with a refusal,
 * so no message is sent.
 */

const DESKTOP = { width: 1600, height: 900 }
const PHONE = { width: 390, height: 844 }
const RATE_LIMIT = 'You have sent 20 report emails in the last hour. Try again later.'

test.skip(() => skipWithoutReportsStub(), 'needs the salesstub package')
test.use({ hasTouch: true })

/** Opens the shipped "Team balance" report, then turns the viewport into a phone. */
async function openPhoneReport(page: Page) {
    await loginAsShippedOwner(page)
    await page.setViewportSize(DESKTOP)
    await openTeamBalance(page)
    await expect(page.getByTestId('report-export-open')).toBeEnabled()
    await page.setViewportSize(PHONE)
    await expect(page.getByTestId('toolbar-more-button')).toBeVisible()
}

/** Opens the report's "Email report" sheet on a phone and waits for it at rest. */
async function openEmailSheet(page: Page): Promise<Locator> {
    await openPhoneReport(page)
    await openExportMenu(page)
    const emailRow = page.getByTestId('report-export-email')
    await expect(emailRow).not.toHaveAttribute('aria-disabled', 'true')
    await emailRow.click()
    const sheet = page.getByTestId('report-email-dialog')
    await expect(sheet.getByTestId('sheet-handle')).toBeVisible()
    await expect.poll(() => sheetState(sheet)).toEqual(AT_REST)
    return sheet
}

const AT_REST = { inViewport: true, atRest: true, backdropShown: true }

/**
 * Where the sheet is drawn, read in one snapshot: inside the viewport, not slid
 * away from its resting edge, and its backdrop at full opacity.
 */
async function sheetState(sheet: Locator) {
    const box = await sheet.boundingBox()
    const viewport = sheet.page().viewportSize()
    const inViewport =
        !!box &&
        !!viewport &&
        box.y >= -1 &&
        box.y + box.height <= viewport.height + 1 &&
        box.height > 0
    const { atRest, backdropOpacity } = await sheet.evaluate(el => {
        // At rest the sheet's slide (its translateY) is 0.
        const transform = getComputedStyle(el).transform
        const slide = transform === 'none' ? 0 : new DOMMatrixReadOnly(transform).f
        // The backdrop is the "Close" button beside the sheet, not the one in it.
        let opacity = 0
        for (let node = el.parentElement; node; node = node.parentElement) {
            const backdrop = Array.from(node.querySelectorAll('[aria-label="Close"]')).find(
                b => !el.contains(b)
            )
            if (!backdrop) continue
            opacity = 1
            for (let n: Element | null = backdrop; n; n = n.parentElement) {
                opacity *= Number(getComputedStyle(n).opacity)
            }
            break
        }
        return {
            atRest: Math.abs(slide) <= 1,
            backdropOpacity: opacity,
        }
    })
    return { inViewport, atRest, backdropShown: backdropOpacity > 0.99 }
}

const pause = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

/**
 * A finger drag down from the sheet's handle, sent as raw touch points (see
 * reports-wrap.spec.ts for why not synthesizeScrollGesture). `slow` paces the
 * moves and holds still before the finger lifts, so the drag ends with no flick.
 */
async function swipeDown(sheet: Locator, distance: number, slow = false) {
    await swipeFrom(sheet.page(), await centerOf(sheet.getByTestId('sheet-handle')), distance, slow)
}

type Point = { x: number; y: number }

async function centerOf(target: Locator): Promise<Point> {
    const box = await target.boundingBox()
    if (!box) throw new Error('the swipe start is not drawn')
    return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

/**
 * The same finger drag, from `start`. A positive `distance` drags down, a
 * negative one up.
 */
async function swipeFrom(page: Page, { x, y }: Point, distance: number, slow = false) {
    const cdp = await page.context().newCDPSession(page)
    const steps = 20
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] })
    for (let i = 1; i <= steps; i++) {
        await cdp.send('Input.dispatchTouchEvent', {
            type: 'touchMove',
            touchPoints: [{ x, y: y + (distance * i) / steps }],
        })
        if (slow) await pause(30)
    }
    if (slow) {
        for (let i = 0; i < 6; i++) {
            await pause(30)
            await cdp.send('Input.dispatchTouchEvent', {
                type: 'touchMove',
                touchPoints: [{ x, y: y + distance }],
            })
        }
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await cdp.detach()
}

/**
 * Starts reading the sheet's slide (its translateY) on every frame, until its
 * node leaves the page or 5 s pass. The result is read once the swipe is done.
 */
async function sampleSlideUntilGone(sheet: Locator) {
    const node = await sheet.elementHandle()
    if (!node) throw new Error('the sheet is not drawn')
    return node.evaluate(
        el =>
            new Promise<{ slides: number[]; isGone: boolean }>(resolve => {
                const slides: number[] = []
                const started = performance.now()
                const read = () => {
                    if (!el.isConnected) return resolve({ slides, isGone: true })
                    const transform = getComputedStyle(el).transform
                    slides.push(transform === 'none' ? 0 : new DOMMatrixReadOnly(transform).f)
                    if (performance.now() - started > 5000)
                        return resolve({ slides, isGone: false })
                    requestAnimationFrame(read)
                }
                read()
            })
    )
}

/**
 * The largest step back toward rest in a run of slides, measured away from the
 * sheet's edge (`dir` 1 for a bottom sheet, -1 for a top sheet). A sheet that
 * leaves without a bounce only ever moves away from its edge.
 */
function largestStepBack(slides: number[], dir: 1 | -1) {
    let farthest = 0
    let stepBack = 0
    for (const slide of slides) {
        const away = dir * slide
        farthest = Math.max(farthest, away)
        stepBack = Math.max(stepBack, farthest - away)
    }
    return { farthest, stepBack }
}

/**
 * Swipes past the threshold, away from the sheet's edge, on an owner that
 * closes, and checks the sheet slides on out from where the finger left it and
 * is gone, never sliding back toward rest first.
 */
async function acceptedSwipeLeaves(sheet: Locator, start: Point, dir: 1 | -1) {
    const samples = sampleSlideUntilGone(sheet)
    // 200 px clears the 100 px threshold and keeps a swipe up on the screen.
    await swipeFrom(sheet.page(), start, dir * 200)
    const { slides, isGone } = await samples
    const { farthest, stepBack } = largestStepBack(slides, dir)
    expect(farthest, 'the drag moves the sheet away from its edge').toBeGreaterThan(100)
    expect(stepBack, 'the closed sheet never slides back toward rest').toBeLessThanOrEqual(2)
    expect(isGone, 'the closed sheet leaves the page').toBe(true)
    await expect(sheet).toHaveCount(0)
}

/**
 * Holds a send, swipes the sheet down while it is on the way, and checks the
 * sheet stays in view; then answers the send with a refusal, which shows in
 * view, and a swipe down closes the sheet.
 */
async function refusedSwipeKeepsSheet(page: Page) {
    const sheet = await openEmailSheet(page)
    await page
        .getByTestId('report-email-to')
        .fill(`${uniqueName('swipe refuse').toLowerCase().replace(/\s+/g, '-')}@example.test`)

    let release = () => {}
    const held = new Promise<void>(resolve => {
        release = resolve
    })
    await page.route('**/api/reports/email', async route => {
        await held
        await route.fulfill({
            status: 429,
            contentType: 'application/json',
            body: JSON.stringify({ status: 429, message: RATE_LIMIT, data: {} }),
        })
    })
    const send = page.getByTestId('report-email-send')
    await send.click()
    await expect(send).toHaveText(/^Sending…/)

    // Past the threshold: the sheet asks to close, and the owner refuses.
    await swipeDown(sheet, 300)
    await expect
        .poll(() => sheetState(sheet), {
            message: 'a refused swipe leaves the sheet at rest, in view, over its backdrop',
        })
        .toEqual(AT_REST)
    await expect(sheet).toHaveCount(1)
    await expect(page.getByTestId('report-email-to')).toBeInViewport()

    // The send ends with a refusal, which the reader sees in the sheet.
    release()
    await expect(page.getByTestId('report-email-refusal')).toContainText(RATE_LIMIT)
    await expect(page.getByTestId('report-email-refusal')).toBeInViewport()
    await page.unrouteAll({ behavior: 'wait' })

    // Nothing is on the way now: a swipe down closes the sheet.
    await swipeDown(sheet, 300)
    await expect(sheet).toHaveCount(0)
}

test.describe('sheet: a refused swipe-down close keeps the sheet in view', () => {
    test('a swipe while the send is on the way leaves the sheet at rest, and a swipe after it closes the sheet', async ({
        page,
    }) => {
        await refusedSwipeKeepsSheet(page)
    })
})

test.describe('sheet: a refused swipe-down close with motion', () => {
    test.use({ reducedMotion: 'no-preference' })

    test('the sheet springs back to rest after a refused swipe, and a swipe after the send closes it', async ({
        page,
    }) => {
        await refusedSwipeKeepsSheet(page)
    })
})

test.describe('sheet: swipe-down to close', () => {
    test('a short swipe snaps the sheet back to rest', async ({ page }) => {
        const sheet = await openEmailSheet(page)
        await swipeDown(sheet, 40, true)
        await expect.poll(() => sheetState(sheet)).toEqual(AT_REST)
        await expect(sheet).toHaveCount(1)
    })

    test('a swipe past the threshold closes the sheet', async ({ page }) => {
        const sheet = await openEmailSheet(page)
        await swipeDown(sheet, 300)
        await expect(sheet).toHaveCount(0)
    })
})

test.describe('sheet: an accepted swipe close with motion', () => {
    test.use({ reducedMotion: 'no-preference' })

    // A core Menu on a phone is a top sheet when its trigger is near the top, and
    // it stays mounted when it closes, so the sheet's own exit slide runs.
    test('a swipe up on a top sheet slides it out without a bounce back', async ({ page }) => {
        await openPhoneReport(page)
        await openExportMenu(page)
        const emailRow = page.getByTestId('report-export-email')
        await expect(emailRow).toBeVisible()
        const handle = page.getByTestId('sheet-handle')
        const sheet = handle.locator('xpath=..')
        await expect(sheet.getByTestId('report-export-email')).toBeVisible()
        // A top sheet draws its handle at its far (bottom) end, under the rows.
        await expect
            .poll(async () => {
                const row = await emailRow.boundingBox()
                const pill = await handle.boundingBox()
                return !!row && !!pill && pill.y > row.y + row.height
            })
            .toBe(true)
        await expect.poll(() => sheetState(sheet)).toEqual(AT_REST)
        await acceptedSwipeLeaves(sheet, await centerOf(handle), -1)
    })

    // An edit closes by a URL param change, which lands in a later commit than
    // the one right after the close request.
    test('a swipe down on the report builder, which closes a commit later, slides it out without a bounce back', async ({
        page,
    }) => {
        await openPhoneReport(page)
        await pressToolbar(page, 'report-edit-open', 'Edit report')
        const sheet = page.getByTestId('report-builder')
        await expect(sheet).toBeVisible()
        // Only the slide is waited for: the builder is as tall as the window, so
        // its box is not inside the phone's content region.
        await expect.poll(async () => (await sheetState(sheet)).atRest).toBe(true)
        // The builder has no handle; the finger starts on the Design pill (the
        // mode already shown), above the scrolled sections, with room below.
        const design = sheet.getByTestId('report-builder-mode-design')
        await acceptedSwipeLeaves(sheet, await centerOf(design), 1)
    })
})
