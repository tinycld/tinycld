import { expect, type Locator, type Page, test } from '@playwright/test'
import { createInvitedUser, login, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    chooseParamOption,
    hubRow,
    loginAsShippedOwner,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    SHIPPED_TITLE,
    STUB_HEADING,
    saveNewReport,
    startReport,
    TEAM_TOTALS_TITLE,
    uniqueName,
} from './reports-helpers'

/**
 * Choice params whose options come from a source (E8), the source heading (E9)
 * and an unpublished shipped report reached through a share (E10), against the
 * stub's "Stub team totals" and a draft over sales. A shipped report is owned by
 * the first owner (shipped.go firstAdmin), the seed's admin app user, so the specs
 * that open it sign in as that user; it is unpublished, so no one else sees it
 * until it is shared. Each user reads only their own stub sales, but every
 * signed-in user reads every stub team (salesstub_teams listRule), and both are
 * shared by several tests, so rows are found by team names unique to the test.
 */

// Two teams `${stem} N1` and `${stem} N2` in `region` (North unless told), and
// `${stem} S1` in South, each with one sale.
async function seedRegions(p: Page, stem: string, region: 'north' | 'east' = 'north') {
    await openReportStub(p)
    await addTeam(p, `${stem} N1`, region)
    await addTeam(p, `${stem} N2`, region)
    await addTeam(p, `${stem} S1`, 'south')
    for (const [team, customer] of [
        [`${stem} N1`, 'Nia'],
        [`${stem} N2`, 'Ned'],
        [`${stem} S1`, 'Sol'],
    ] as const) {
        await addSale(p, { team, customer, amount: 100, cost: 0, day: '2026-01-05' })
    }
}

// A draft over sales whose team condition is asked when run as "is any of", its
// teams listed by region (R30).
async function askTeamsByRegion(p: Page) {
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await p.getByTestId('report-builder-section-filter').click()
    await p.getByTestId('report-builder-filter-add-condition').click()
    await p
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-record-team')
        .click()
    await chooseOption(p, 'report-builder-filter-0-op', 'in')
    await p.getByTestId('report-builder-filter-0-ask').click()
    await expect(p.getByTestId(GROUP_ORDER)).toHaveCount(0)
    await regroupTeams(p, 'region', 'Region')
}

const GROUP_ORDER = 'report-builder-filter-0-options-order'

async function regroupTeams(p: Page, field: string, shown: string) {
    const grouping = p.getByTestId('report-builder-filter-0-options-group')
    await grouping.click()
    await p
        .getByTestId('report-builder-picker')
        .getByTestId(`report-builder-pick-field-${field}`)
        .click()
    await expect(grouping).toHaveText(shown)
}

// The team picker's whole-group picks, one per section head and in its order, of
// the teams this test made (searched by its stem).
async function expectGroupHeads(p: Page, stem: string, heads: string[]) {
    await p.getByTestId('report-add-filter').click()
    await p.getByTestId('report-add-filter-team').click()
    const picker = p.getByTestId('report-filter-picker')
    await picker.getByTestId('report-filter-search').fill(stem)
    await expect(picker.locator('[data-testid^="report-filter-group-"]')).toHaveText(heads)
    await p.keyboard.press('Escape')
    await expect(picker).toBeHidden()
}

async function openTeamTotals(p: Page) {
    await openReportsHub(p)
    await p.getByTestId('reports-hub-tab-mine').click()
    await hubRow(p, TEAM_TOTALS_TITLE).click()
    await expect(p.getByTestId('report-title')).toHaveText(TEAM_TOTALS_TITLE)
}

// The Published tab lists the stub's published shipped report and not the
// unpublished one. The published row is waited for first, so the count of the
// unpublished one is read from a loaded list, not from one still loading.
async function expectNotPublished(p: Page) {
    await openReportsHub(p)
    await p.getByTestId('reports-hub-tab-published').click()
    await expect(hubRow(p, SHIPPED_TITLE)).toBeVisible()
    await expect(hubRow(p, TEAM_TOTALS_TITLE)).toHaveCount(0)
}

const ENTRANCE_FRAMES = 10
// The exit watch stops counting here, so a sheet that never leaves fails the
// test instead of hanging it (the animated close keeps it ~300 ms, ~18 frames).
const EXIT_FRAME_CAP = 30

type SheetEntrance = { offsets: number[]; opacities: string[]; backdropFound: boolean[] }
type SheetWatch = { entrance?: Promise<SheetEntrance>; exit?: Promise<number> }

// Waits for the sheet surface with this test id to enter the DOM, then reads it
// once after each of the next frames is painted (a rAF, then a task). Each read
// is the surface's offset from its resting place: its drawn top minus its laid
// out top (offsetTop ignores a transform), so a slide shows as a non-zero
// offset whatever the sheet's height. The backdrop is the layer before the
// surface's (core/ui/sheet draws the backdrop, then the surface), found by the
// Close press it holds, and read for its opacity. Awaited before the press that
// opens the sheet, so the watch is in place first; readSheetEntrance collects it.
function watchSheetEntrance(p: Page, testId: string) {
    return p.evaluate(
        ({ testId, frames }) => {
            const watch = window as unknown as SheetWatch
            watch.entrance = new Promise(resolve => {
                const seen: SheetEntrance = { offsets: [], opacities: [], backdropFound: [] }
                const read = (surface: HTMLElement) => {
                    const layer = surface.offsetParent as HTMLElement
                    const restTop =
                        layer.getBoundingClientRect().top + layer.clientTop + surface.offsetTop
                    seen.offsets.push(Math.round(surface.getBoundingClientRect().top - restTop))
                    const backdrop = layer.previousElementSibling as HTMLElement
                    seen.backdropFound.push(
                        backdrop.querySelector('[role="button"][aria-label="Close"]') !== null
                    )
                    seen.opacities.push(getComputedStyle(backdrop).opacity)
                }
                const next = (surface: HTMLElement) =>
                    requestAnimationFrame(() =>
                        setTimeout(() => {
                            read(surface)
                            if (seen.offsets.length < frames) next(surface)
                            else resolve(seen)
                        })
                    )
                const selector = `[data-testid="${testId}"]`
                const observer = new MutationObserver(() => {
                    const surface = document.querySelector<HTMLElement>(selector)
                    if (!surface) return
                    observer.disconnect()
                    next(surface)
                })
                observer.observe(document.body, { childList: true, subtree: true })
            })
        },
        { testId, frames: ENTRANCE_FRAMES }
    )
}

// Counts the frames painted after the next Escape keydown while the sheet's
// surface or its backdrop is still in the DOM (0: both gone by the first frame).
function watchSheetExit(sheet: Locator) {
    return sheet.evaluate((surface, cap) => {
        const backdrop = (surface as HTMLElement).offsetParent?.previousElementSibling
        const watch = window as unknown as SheetWatch
        watch.exit = new Promise(resolve => {
            let frames = 0
            const tick = () => {
                if (!surface.isConnected && !backdrop?.isConnected) return resolve(frames)
                frames += 1
                if (frames >= cap) return resolve(frames)
                requestAnimationFrame(tick)
            }
            // On window in the capture phase: the overlay's Escape handler sits
            // on the document and stops the key there.
            const onKey = (event: KeyboardEvent) => {
                if (event.key !== 'Escape') return
                window.removeEventListener('keydown', onKey, true)
                requestAnimationFrame(tick)
            }
            window.addEventListener('keydown', onKey, true)
        })
    }, EXIT_FRAME_CAP)
}

function readSheetEntrance(p: Page) {
    return p.evaluate(() => (window as unknown as SheetWatch).entrance)
}

function readSheetExit(p: Page) {
    return p.evaluate(() => (window as unknown as SheetWatch).exit)
}

// Opens a sheet with a press, watching its entrance: it must stand at rest, its
// backdrop fully shown, on every frame from the first.
async function expectSheetOpensAtRest(p: Page, testId: string, open: () => Promise<void>) {
    await watchSheetEntrance(p, testId)
    await open()
    const entrance = await readSheetEntrance(p)
    expect(entrance?.offsets).toHaveLength(ENTRANCE_FRAMES)
    expect(entrance?.backdropFound).toEqual(Array(ENTRANCE_FRAMES).fill(true))
    expect(entrance?.offsets).toEqual(Array(ENTRANCE_FRAMES).fill(0))
    expect(entrance?.opacities).toEqual(Array(ENTRANCE_FRAMES).fill('1'))
}

test.describe('params from a source, headings and unpublished shipped reports', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an unpublished shipped report prints its heading and reaches only who it is shared with', async ({
        page,
    }) => {
        // A fresh reader each run, with a name no one else has, so a re-run on the
        // same database finds them as a candidate (F24). The shared test user
        // invites; the shipped report's owner then signs in on the same page.
        const readerName = uniqueName('Reader')
        const { inviteePage: c, close } = await createInvitedUser(page, 'rptshr', {
            name: readerName,
        })
        try {
            await loginAsShippedOwner(page)
            await expectNotPublished(page)
            await openTeamTotals(page)
            await expect(page.getByTestId('report-business-name')).toHaveText(STUB_HEADING)

            await expectNotPublished(c)

            await pressToolbar(page, 'report-share-open', 'Share')
            await page.getByTestId('report-share-search').fill(readerName)
            await page
                .getByTestId('report-share-dialog')
                .locator('[data-testid^="report-share-candidate-"]')
                .filter({ hasText: readerName })
                .click()
            await page.getByTestId('report-share-done').click()

            await c.getByTestId('reports-hub-tab-shared').click()
            await hubRow(c, TEAM_TOTALS_TITLE).click()
            await expect(c.getByTestId('report-title')).toHaveText(TEAM_TOTALS_TITLE)
            await expect(c.getByTestId('report-business-name')).toHaveText(STUB_HEADING)
        } finally {
            await close()
        }
    })

    test('a choice from a source searches, picks one record, then a whole group', async ({
        page,
    }) => {
        // The whole group shows every team of the region that holds a sale of the
        // shipped report's owner, and other specs (openPagedTeams) give that owner
        // many North teams. The table draws only the rows near its view, so this
        // test's teams sit in East, which no other spec gives the owner a sale in.
        await loginAsShippedOwner(page)
        const stem = uniqueName('Src')
        await seedRegions(page, stem, 'east')
        await openTeamTotals(page)
        await chooseParamOption(page, 'teams', {
            from: 'add-filter',
            search: stem,
            labels: [`${stem} N1`],
        })
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${stem} N1`)
        const rows = page.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')
        await expect(rows).toHaveText([`${stem} N1`])
        // The report names its group pick (optionsFrom.groupLabel "All {group} teams").
        await page.getByTestId('report-param-teams').click()
        const picker = page.getByTestId('report-filter-picker')
        await picker.getByTestId('report-filter-search').fill(stem)
        await expect(picker.getByTestId('report-filter-group-east')).toHaveText('All East teams')
        await page.keyboard.press('Escape')
        await expect(picker).toBeHidden()
        await chooseParamOption(page, 'teams', { from: 'chip', search: stem, group: 'east' })
        await expect(page.getByTestId('report-param-teams')).toHaveText(
            `Teams: ${stem} N1, All East teams`
        )
        await expect(rowOf(page, `${stem} N2`)).toBeVisible()
        await expect(rows.filter({ hasText: `${stem} S1` })).toHaveCount(0)
    })

    test('on a phone the source picker is a sheet and the page does not scroll sideways', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const stem = uniqueName('SrcPh')
        await seedRegions(page, stem)
        await openTeamTotals(page)
        await page.setViewportSize({ width: 390, height: 844 })
        await page.getByTestId('report-add-filter').click()
        await page.getByTestId('report-add-filter-teams').click()
        const picker = page.getByTestId('report-filter-picker')
        await expect(picker.getByTestId('sheet-handle')).toBeVisible()
        await expect.poll(async () => (await picker.boundingBox())?.width).toBe(390)
        await page.keyboard.press('Escape')
        await expect(picker).toBeHidden()
        await chooseParamOption(page, 'teams', {
            from: 'add-filter',
            search: stem,
            labels: [`${stem} S1`],
        })
        await expect(rowOf(page, `${stem} S1`)).toBeVisible()
        await expect
            .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
            .toBeLessThanOrEqual(390)
    })

    // Reduced motion (the e2e config's `reducedMotion: 'reduce'`, the OS setting
    // for a person): a sheet is drawn at rest from its first frame, with its
    // backdrop fully shown, so Playwright never waits for it to stop moving.
    // Closed, it leaves at once and a press beneath it lands.
    test('on a phone with reduced motion the sheet does not slide, and closed it lets a press through', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        await openTeamTotals(page)
        await page.setViewportSize({ width: 390, height: 844 })
        expect(
            await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)
        ).toBe(true)

        // The "Add filter" menu is a bottom sheet on a phone.
        const addFilter = page.getByTestId('report-add-filter')
        const menu = page.getByTestId('report-add-filter-menu')
        await expectSheetOpensAtRest(page, 'report-add-filter-menu', () => addFilter.click())
        await expect(menu.getByTestId('report-add-filter-teams')).toBeVisible()

        // Closed, the menu stays mounted (only its open state changes), so the
        // sheet itself decides when it leaves: at once, backdrop and all.
        await watchSheetExit(menu)
        await page.keyboard.press('Escape')
        expect(await readSheetExit(page)).toBeLessThanOrEqual(1)
        await expect(menu).toHaveCount(0)
        // A raw press, with no actionability wait or retry: it reaches the
        // chip only if nothing of the sheet still covers it.
        const chip = await addFilter.boundingBox()
        expect(chip).not.toBeNull()
        await page.mouse.click(
            (chip?.x ?? 0) + (chip?.width ?? 0) / 2,
            (chip?.y ?? 0) + (chip?.height ?? 0) / 2
        )
        await expect(menu.getByTestId('report-add-filter-teams')).toBeVisible()

        // The source picker, opened from the menu, is a bottom sheet too.
        const picker = page.getByTestId('report-filter-picker')
        await expectSheetOpensAtRest(page, 'report-filter-picker', () =>
            menu.getByTestId('report-add-filter-teams').click()
        )
        await expect(picker.getByTestId('sheet-handle')).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(picker).toHaveCount(0)
    })

    test('scrolling to the end of the options loads the next page once, with no repeats', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const stem = uniqueName('Page')
        const names = Array.from(
            { length: 12 },
            (_, i) => `${stem} T${String(i + 1).padStart(2, '0')}`
        )
        await openReportStub(page)
        for (const name of names) await addTeam(page, name, 'north')
        await openTeamTotals(page)

        // The server pages by 50; the searched list asks for pages of 10 instead
        // (the request's `limit`, which the server takes from 1 to 1000), so 12
        // teams make two real pages joined by the server's `after` cursor. The
        // next page is held until its spinner has been seen.
        let release = () => {}
        const held = new Promise<void>(resolve => {
            release = resolve
        })
        const afters: string[] = []
        await page.route('**/api/reports/param-options', async route => {
            const body = route.request().postDataJSON()
            if (body.search !== stem || body.values || body.limit) return route.continue()
            if (body.after) {
                afters.push(body.after)
                await held
            }
            await route.continue({ postData: JSON.stringify({ ...body, limit: 10 }) })
        })
        await page.getByTestId('report-add-filter').click()
        await page.getByTestId('report-add-filter-teams').click()
        const picker = page.getByTestId('report-filter-picker')
        await picker.getByTestId('report-filter-search').fill(stem)
        const options = picker.locator('[data-testid^="report-filter-option-"]')
        await expect(options).toHaveText(names.slice(0, 10))
        expect(afters).toEqual([])

        await picker.getByTestId('report-filter-options').hover()
        await page.mouse.wheel(0, 2000)
        await expect(picker.getByTestId('report-filter-more')).toBeVisible()
        release()
        await expect(options).toHaveText(names)
        await expect(picker.getByTestId('report-filter-more')).toHaveCount(0)
        expect(afters).toHaveLength(1)

        // A team from the second page is picked like any other.
        const last = names[names.length - 1] ?? ''
        await picker.getByRole('checkbox', { name: last, exact: true }).click()
        await picker.getByTestId('report-filter-apply').click()
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${last}`)
    })

    test('a condition on a linked team picks a record, and Ask when run lists the teams', async ({
        page,
    }) => {
        await login(page)
        const stem = uniqueName('Rel')
        await seedRegions(page, stem)
        await openReportsHub(page)
        await startReport(page, SALES_SOURCE)
        await page.getByTestId('report-builder-section-filter').click()
        await page.getByTestId('report-builder-filter-add-condition').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-record-team')
            .click()
        await page.getByTestId('report-builder-filter-0-value').click()
        await page.getByTestId('report-filter-search').fill(`${stem} S1`)
        await page
            .getByTestId('report-filter-picker')
            .getByRole('radio', { name: `${stem} S1`, exact: true })
            .click()
        await expect(page.getByTestId('report-builder-filter-0-value')).toHaveText(`${stem} S1`)
        // Every seed of this user (other tests, other runs on this database)
        // adds a Nia, a Ned and a Sol, so before the filter applies the preview
        // shows several of them. Filtered to this stem's S1 team, the table
        // shows exactly one of the three names: one Sol. One retrying
        // assertion of that whole state, so neither a loading table nor the
        // unfiltered one can pass it.
        const table = page.getByTestId('report-table')
        await expect(table.getByText(/^(Sol|Nia|Ned)$/)).toHaveText(['Sol'])

        // "Is any of" shows each picked record as a chip that removes it.
        await chooseOption(page, 'report-builder-filter-0-op', 'in')
        await page.getByTestId('report-builder-filter-0-value').click()
        await page.getByTestId('report-filter-search').fill(stem)
        const picker = page.getByTestId('report-filter-picker')
        await picker.getByRole('checkbox', { name: `${stem} N1`, exact: true }).click()
        await picker.getByRole('checkbox', { name: `${stem} N2`, exact: true }).click()
        await picker.getByTestId('report-filter-apply').click()
        const chips = page.locator('[data-testid^="report-builder-filter-0-value-"]')
        await expect(chips).toHaveText([`${stem} N1`, `${stem} N2`])
        await expect(table.getByText('Ned', { exact: true })).toBeVisible()
        await chips.filter({ hasText: `${stem} N2` }).click()
        await expect(chips).toHaveText([`${stem} N1`])
        await expect(table.getByText('Ned', { exact: true })).toHaveCount(0)
        await expect(table.getByText('Nia', { exact: true })).toBeVisible()

        await page.getByTestId('report-builder-filter-0-ask').click()
        await chooseParamOption(page, 'team', {
            from: 'add-filter',
            search: stem,
            labels: [`${stem} N1`, `${stem} N2`],
        })
        await expect(table.getByText('Nia', { exact: true })).toBeVisible()
        await expect(table.getByText('Ned', { exact: true })).toBeVisible()
        await expect(table.getByText('Sol', { exact: true })).toHaveCount(0)
    })

    // R30: a builder-made ask lists its records in groups when "Group the list by"
    // names a field, and the reader picks a whole group with "All <group>". A
    // fresh member owns only this test's sales, so each customer shows once.
    test('Ask when run can list the teams by region, and a whole region is picked at once', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptaskgrp')
        try {
            const stem = uniqueName('Grp')
            await seedRegions(p, stem)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-record-team')
                .click()
            await chooseOption(p, 'report-builder-filter-0-op', 'in')
            await p.getByTestId('report-builder-filter-0-ask').click()
            const grouping = p.getByTestId('report-builder-filter-0-options-group')
            await expect(grouping).toHaveText('No groups')
            await grouping.click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-region')
                .click()
            await expect(grouping).toHaveText('Region')

            await chooseParamOption(p, 'team', { from: 'add-filter', search: stem, group: 'north' })
            await expect(p.getByTestId('report-param-team')).toHaveText('Team: All North')
            const table = p.getByTestId('report-table')
            await expect(table.getByText('Nia', { exact: true })).toBeVisible()
            await expect(table.getByText('Ned', { exact: true })).toBeVisible()
            await expect(table.getByText('Sol', { exact: true })).toHaveCount(0)

            // The group pick can have its own text, with {group} for the group's name.
            const groupText = p.getByTestId('report-builder-filter-0-group-label')
            await groupText.fill('Every team')
            await expect(p.getByText('Write {group} where the group’s name goes')).toBeVisible()
            await groupText.fill('Every {group} team')
            await expect(p.getByText('Write {group} where the group’s name goes')).toHaveCount(0)
            await expect(p.getByTestId('report-param-team')).toHaveText('Team: Every North team')

            // Without groups a whole-group pick no longer fits the param (view-url.ts
            // reads each value against its param), so the filter is unset again.
            await p.getByTestId('report-builder-filter-0-options-group-clear').click()
            await expect(grouping).toHaveText('No groups')
            await expect(groupText).toHaveCount(0)
            await expect(table.getByText('Sol', { exact: true })).toBeVisible()
        } finally {
            await close()
        }
    })

    // E1: "Order the groups by" orders the regions a team list is grouped in: by
    // name (East, North, South), or in the region field's own order (North, South,
    // East, as the stub declares them). The preview's picker follows each choice,
    // and the saved report's picker keeps it.
    test('Order the groups by lists the regions in the field’s own order, in the preview and the saved report', async ({
        page,
    }) => {
        await login(page)
        const stem = uniqueName('Ord')
        await openReportStub(page)
        await addTeam(page, `${stem} E1`, 'east')
        await addTeam(page, `${stem} N1`, 'north')
        await addTeam(page, `${stem} S1`, 'south')
        await askTeamsByRegion(page)
        const byName = ['All East', 'All North', 'All South']
        const declared = ['All North', 'All South', 'All East']
        await expectGroupHeads(page, stem, byName)
        await chooseOption(page, GROUP_ORDER, 'declared')
        await expectGroupHeads(page, stem, declared)
        await chooseOption(page, GROUP_ORDER, 'label')
        await expectGroupHeads(page, stem, byName)
        await chooseOption(page, GROUP_ORDER, 'declared')
        await saveNewReport(page, uniqueName('Regions in order'))
        await expectGroupHeads(page, stem, declared)
    })

    // E1: "A field" orders the regions by the smallest value of a field among each
    // region's teams; a new one starts at the options' label field, the name
    // (ConditionValues.tsx orderFor). A region's smallest value is read over every
    // team it holds, not only the searched ones (options.go rankedOptionsSQL), and
    // every signed-in user reads every stub team (salesstub_teams listRule). So these
    // names start with '!', which sorts before the letter every other spec's team
    // name starts with (uniqueName): each region's smallest name is then this
    // test's, or the same test's from an earlier run, whose names hold the same
    // order. A to Z by name puts South (A) first, then East (B), then North (C),
    // which neither the regions' names nor their declared order give.
    test('Order the groups by a field lists the regions by their smallest name, in the preview and the saved report', async ({
        page,
    }) => {
        await login(page)
        const stem = uniqueName('PathOrd')
        await openReportStub(page)
        await addTeam(page, `!${stem} A`, 'south')
        await addTeam(page, `!${stem} B`, 'east')
        await addTeam(page, `!${stem} C`, 'north')
        await askTeamsByRegion(page)
        const byName = ['All East', 'All North', 'All South']
        const bySmallestName = ['All South', 'All East', 'All North']
        await expectGroupHeads(page, stem, byName)

        await chooseOption(page, GROUP_ORDER, 'path')
        const orderField = page.getByTestId(`${GROUP_ORDER}-field`)
        await expect(orderField).toHaveText('Name')
        await expectGroupHeads(page, stem, bySmallestName)

        // By the smallest region, each region comes by its own value, A to Z.
        await orderField.click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-region')
            .click()
        await expect(orderField).toHaveText('Region')
        await expectGroupHeads(page, stem, ['All East', 'All North', 'All South'])

        await orderField.click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-name')
            .click()
        await expect(orderField).toHaveText('Name')
        await expectGroupHeads(page, stem, bySmallestName)

        await saveNewReport(page, uniqueName('Regions by name'))
        await expectGroupHeads(page, stem, bySmallestName)
    })

    // A field that is not a select has no order of its own: grouped by the team's
    // name, the field's own order is not offered and a chosen one is cleared.
    test('grouping the list by a field that is not a select clears the field’s own order', async ({
        page,
    }) => {
        await login(page)
        await askTeamsByRegion(page)
        const order = page.getByTestId(GROUP_ORDER)
        const byName = (await order.textContent()) ?? ''
        expect(byName).not.toBe('')
        await chooseOption(page, GROUP_ORDER, 'declared')
        await expect(order).not.toHaveText(byName)
        await regroupTeams(page, 'name', 'Name')
        await expect(order).toHaveText(byName)
        await order.click()
        await expect(page.getByTestId(`${GROUP_ORDER}-label`)).toBeVisible()
        await expect(page.getByTestId(`${GROUP_ORDER}-declared`)).toHaveCount(0)
        await page.keyboard.press('Escape')
        await expect(page.getByTestId(`${GROUP_ORDER}-label`)).toHaveCount(0)
    })

    test('a having on a money total asks an amount, kept to its currency', async ({ page }) => {
        await login(page)
        const stem = uniqueName('Amt')
        await seedRegions(page, stem)
        // N1 adds up to 4.00 USD, N2 to 1.00 USD.
        await addSale(page, {
            team: `${stem} N1`,
            customer: 'Nia',
            amount: 300,
            cost: 0,
            day: '2026-01-06',
        })
        await openReportsHub(page)
        await startReport(page, SALES_SOURCE)
        await page.getByTestId('report-builder-add-column').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-record-team')
            .click()
        // A new draft starts with the customer column, so team is column 1.
        await addColumn(page, ['amount'])
        await chooseOption(page, 'report-builder-column-2-aggregate', 'sum')
        await page.getByTestId('report-builder-section-filter').click()
        await page.getByTestId('report-builder-filter-add-condition').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-link-team')
            .click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-name')
            .click()
        await chooseOption(page, 'report-builder-filter-0-op', 'startsWith')
        await page.getByTestId('report-builder-filter-0-value').fill(stem)
        await page.getByTestId('report-builder-section-group').click()
        await page.getByTestId('report-builder-group-add').click()
        await chooseOption(page, 'report-builder-group-0-col', 'team')
        await page.getByTestId('report-builder-group-0-having-add-total').click()
        await page
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-amount')
            .click()
        await chooseOption(page, 'report-builder-group-0-having-0-aggregate', 'sum')
        await chooseOption(page, 'report-builder-group-0-having-0-op', 'gte')
        await page.getByTestId('report-builder-group-0-having-0-ask').click()

        const table = page.getByTestId('report-table')
        const chip = page.getByTestId('report-param-amount')
        await page.getByTestId('report-add-filter').click()
        await page.getByTestId('report-add-filter-amount').click()
        const picker = page.getByTestId('report-filter-picker')
        await picker.getByTestId('report-filter-value').fill('2.00')
        await picker.getByTestId('report-filter-currency').fill('usd')
        await picker.getByTestId('report-filter-apply').click()
        await expect(chip).toHaveText('Sum of Amount: USD 2.00')
        await expect(rowOf(page, `${stem} N1`)).toBeVisible()
        await expect(table.getByText(`${stem} N2`, { exact: true })).toHaveCount(0)

        // A typo is refused in the picker and applies nothing.
        await chip.click()
        await picker.getByTestId('report-filter-value').fill('2.0.0')
        await expect(picker.getByText('Type an amount, like 1500.00')).toBeVisible()
        await expect(picker.getByTestId('report-filter-apply')).toBeDisabled()

        // The sales are in USD: a EUR amount compares with none of them (R29).
        await picker.getByTestId('report-filter-value').fill('2.00')
        await picker.getByTestId('report-filter-currency').fill('EUR')
        await picker.getByTestId('report-filter-apply').click()
        await expect(chip).toHaveText('Sum of Amount: EUR 2.00')
        await expect(table.getByText(`${stem} N1`, { exact: true })).toHaveCount(0)
    })
})
