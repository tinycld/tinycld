import { expect, type Page, test } from '@playwright/test'
import { buildAndPublish, visitAnonymously } from './web-forms-helpers'

/**
 * Field widths on a public web form: a select at width "Fit" (auto) takes its
 * content's width beside its neighbours, and a quarter select with "On narrow
 * screens: Full" spans the form below the 640 px breakpoint and stays a quarter
 * above it. Two quarter dates show the existing rule (quarter → half below 640).
 */

const NARROW_BELOW = 640
/** Sub-pixel rounding of the layout. */
const EPSILON = 0.5
const TOLERANCE = 1

const VIEWPORTS = [
    { width: 1280, height: 720, side: 'wide' },
    { width: 911, height: 720, side: 'wide' },
    { width: 640, height: 720, side: 'narrow' },
    { width: 390, height: 844, side: 'narrow' },
] as const

const QUARTER_KEYS = ['start_date', 'end_date', 'team']
const FIT_KEY = 'level'
const NARROW_KEY = 'team'

/**
 * One snapshot of the form: the container, every cell and its bordered box, and
 * the fit cell's max-content width (its style set to max-content for one
 * synchronous read, then restored, so nothing paints in between).
 */
async function snapshot(visitor: Page) {
    return visitor.evaluate(
        ({ keys, fitKey }) => {
            const byId = (id: string) => {
                const el = document.querySelector<HTMLElement>(`[data-testid="${id}"]`)
                if (!el) throw new Error(`no ${id}`)
                return el
            }
            const box = (el: Element) => {
                const r = el.getBoundingClientRect()
                return { left: r.left, right: r.right, top: r.top, width: r.width }
            }
            const container = box(byId('web-form'))
            const cells = Object.fromEntries(
                keys.map(key => [
                    key,
                    {
                        cell: box(byId(`form-field-${key}`)),
                        box: box(byId(`form-field-${key}-box`)),
                    },
                ])
            )
            const fit = byId(`form-field-${fitKey}`)
            const saved = fit.style.cssText
            fit.style.flex = '0 0 auto'
            fit.style.width = 'max-content'
            fit.style.maxWidth = 'none'
            fit.style.minWidth = '0'
            const contentWidth = fit.getBoundingClientRect().width
            fit.style.cssText = saved
            const boxes = [...document.querySelectorAll('[data-testid$="-box"]')]
                .filter(el => el.getAttribute('data-testid')?.startsWith('form-field-'))
                .map(el => el.getBoundingClientRect().right)
            return { container, cells, contentWidth, widestBoxRight: Math.max(...boxes) }
        },
        { keys: [...QUARTER_KEYS, FIT_KEY], fitKey: FIT_KEY }
    )
}

type Snapshot = Awaited<ReturnType<typeof snapshot>>

const near = (a: number, b: number, tolerance = TOLERANCE) => Math.abs(a - b) <= tolerance

/** What must hold at this width, as booleans, so one poll reports every miss. */
function verdict(s: Snapshot) {
    const fit = s.cells[FIT_KEY].cell
    const start = s.cells.start_date.cell
    const narrow = s.cells[NARROW_KEY]
    const isWide = s.container.width >= NARROW_BELOW
    const common = {
        side: isWide ? 'wide' : 'narrow',
        fitIsItsContentWidth: near(fit.width, s.contentWidth),
        nothingPastTheRightEdge: s.widestBoxRight <= s.container.right + EPSILON,
    }
    if (isWide) {
        return {
            ...common,
            fitSharesTheQuartersRow: QUARTER_KEYS.every(k =>
                near(s.cells[k].cell.top, fit.top, EPSILON)
            ),
            narrowFieldIsAQuarter: near(narrow.cell.width, start.width),
            fitIsNotStretched: fit.width < s.container.width / 2,
        }
    }
    return {
        ...common,
        narrowFieldSpansTheForm: near(narrow.box.width, s.container.width),
        datesAreHalf: near(start.width, narrow.cell.width / 2),
        fitIsNotFull: fit.width < s.container.width - TOLERANCE,
    }
}

test.describe('web-forms: field widths', () => {
    test('a fit field keeps its content width and a narrow width applies below 640 px', async ({
        page,
        browser,
    }) => {
        const url = await buildAndPublish(page, `Widths ${Date.now()}`, {
            fields: [
                { kind: 'date', label: 'Start date', width: 'quarter' },
                { kind: 'date', label: 'End date', width: 'quarter' },
                { kind: 'select', label: 'Team', width: 'quarter', narrowWidth: 'full' },
                { kind: 'select', label: 'Level', width: 'auto' },
            ],
        })
        const [first] = VIEWPORTS
        const visitor = await visitAnonymously(browser, url, {
            width: first.width,
            height: first.height,
        })
        try {
            await expect(visitor.getByTestId(`form-field-${FIT_KEY}`)).toBeVisible()
            const fitWidths: number[] = []
            for (const viewport of VIEWPORTS) {
                await visitor.setViewportSize({ width: viewport.width, height: viewport.height })
                const wide = viewport.side === 'wide'
                await expect
                    .poll(async () => verdict(await snapshot(visitor)), {
                        message: `viewport ${viewport.width}`,
                    })
                    .toEqual({
                        side: viewport.side,
                        fitIsItsContentWidth: true,
                        nothingPastTheRightEdge: true,
                        ...(wide
                            ? {
                                  fitSharesTheQuartersRow: true,
                                  narrowFieldIsAQuarter: true,
                                  fitIsNotStretched: true,
                              }
                            : {
                                  narrowFieldSpansTheForm: true,
                                  datesAreHalf: true,
                                  fitIsNotFull: true,
                              }),
                    })
                const s = await snapshot(visitor)
                test.info().annotations.push({
                    type: 'container width',
                    description: `viewport ${viewport.width}: ${s.container.width}`,
                })
                fitWidths.push(s.cells[FIT_KEY].cell.width)
            }
            const [fitAt1280] = fitWidths
            expect(fitWidths.every(w => near(w, fitAt1280))).toBe(true)
        } finally {
            await visitor.context().close()
        }
    })
})
