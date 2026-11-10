import { expect, type Locator, type Page, test } from '@playwright/test'
import { buildAndPublish, visitAnonymously } from './web-forms-helpers'

/**
 * A public web form on a phone: quarter-width fields become half width, so a
 * date field's input and calendar button share a ~150 px slot, as a form band
 * gives them. The button must stay inside the field's border, beside its input,
 * and no field may run past the right edge of the screen.
 */

const PHONE = { width: 390, height: 844 }
const DATE_KEYS = ['start_date', 'end_date']
/** Sub-pixel rounding of the layout. */
const EPSILON = 0.5

type Box = { x: number; y: number; width: number; height: number }

async function boxOf(locator: Locator): Promise<Box> {
    const box = await locator.boundingBox()
    if (!box) throw new Error('element has no box')
    return box
}

const right = (b: Box) => b.x + b.width

/** One snapshot of a date field: its input, its bordered frame and its calendar button. */
async function dateFieldLayout(visitor: Page, key: string) {
    const input = visitor.getByTestId(`form-input-${key}`)
    const [inputBox, frameBox, buttonBox] = await Promise.all([
        boxOf(input),
        boxOf(input.locator('xpath=..')),
        boxOf(visitor.getByTestId(`form-input-${key}-open`)),
    ])
    return {
        buttonInsideFrame:
            buttonBox.x >= frameBox.x - EPSILON && right(buttonBox) <= right(frameBox) + EPSILON,
        buttonInsideViewport: right(buttonBox) <= PHONE.width + EPSILON,
        frameInsideViewport: right(frameBox) <= PHONE.width + EPSILON,
        // Beside its input: no overlap, at most 8 px apart.
        buttonBesideInput:
            buttonBox.x >= right(inputBox) - EPSILON &&
            buttonBox.x - right(inputBox) <= 8 + EPSILON,
    }
}

/** How far a field's widest part (input, stepper, button) runs past the right edge of its own cell. */
async function overflowOfCell(visitor: Page, key: string) {
    return visitor.getByTestId(`form-field-${key}`).evaluate(cell => {
        const edge = cell.getBoundingClientRect().right
        const parts = [...cell.querySelectorAll('*')].map(el => el.getBoundingClientRect().right)
        return Math.round(Math.max(edge, ...parts) - edge)
    })
}

/** The widest right edge of any field control on the page. */
async function widestRightEdge(visitor: Page) {
    return visitor
        .locator('[data-testid^="form-input-"]')
        .evaluateAll(els => Math.max(...els.map(el => el.getBoundingClientRect().right)))
}

test.describe('web-forms: phone layout', () => {
    test('date, number and select fields stay inside a phone screen', async ({ page, browser }) => {
        const url = await buildAndPublish(page, `Phone layout ${Date.now()}`, {
            fields: [
                { kind: 'date', label: 'Start date', width: 'quarter' },
                { kind: 'date', label: 'End date', width: 'quarter' },
                { kind: 'number', label: 'Seats', width: 'quarter' },
                { kind: 'select', label: 'Team', width: 'quarter' },
            ],
        })
        const visitor = await visitAnonymously(browser, url, PHONE)
        try {
            await expect(visitor.getByTestId('form-input-start_date')).toBeVisible()

            for (const key of DATE_KEYS) {
                await expect
                    .poll(() => dateFieldLayout(visitor, key), { message: key })
                    .toMatchObject({
                        buttonInsideFrame: true,
                        buttonInsideViewport: true,
                        frameInsideViewport: true,
                        buttonBesideInput: true,
                    })
            }

            await expect(visitor.getByTestId('form-input-seats')).toBeVisible()
            await expect(visitor.getByTestId('form-input-team')).toBeVisible()
            await expect
                .poll(() => widestRightEdge(visitor))
                .toBeLessThanOrEqual(PHONE.width + EPSILON)
            // A half-width cell's stepper or calendar button must not run into its neighbour.
            for (const key of [...DATE_KEYS, 'seats', 'team']) {
                await expect.poll(() => overflowOfCell(visitor, key), { message: key }).toBe(0)
            }
        } finally {
            await visitor.context().close()
        }
    })
})
