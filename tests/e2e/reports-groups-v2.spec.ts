import { expect, type Page, test } from '@playwright/test'
import { appShell, createInvitedUser, login, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    detailLabels,
    groupWithTeamRegion,
    holdRuns,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    startTeamSales,
    TEAMS_SOURCE,
    uniqueName,
} from './reports-helpers'

/**
 * Group levels in the builder: group by a linked team (its record labels the row),
 * show every team even with no rows, sort by a team field or a formula, hide groups
 * with an asked condition, total only the rows shown, open groups when the report
 * opens, and label opened rows. A fresh invited member owns the sales; teams are
 * shared, so every team name starts with a stem unique to the test and the fill
 * condition keeps only those.
 */

const YEAR = new Date().getFullYear()

async function seedTeams(p: Page, stem: string) {
    await openReportStub(p)
    await addTeam(p, `${stem} A`)
    await addTeam(p, `${stem} B`)
    await addTeam(p, `${stem} C`)
    await addSale(p, {
        team: `${stem} A`,
        customer: 'Acme',
        amount: 1000,
        cost: 0,
        quantity: 5,
        day: '2026-01-05',
    })
    await addSale(p, {
        team: `${stem} B`,
        customer: 'Bolt',
        amount: 300,
        cost: 0,
        quantity: 2,
        day: '2026-01-06',
    })
}

async function pickIn(p: Page, trigger: string, key: string) {
    await p.getByTestId(trigger).click()
    await p.getByTestId('report-builder-picker').getByTestId(key).click()
}

async function groupByTeam(p: Page, stem: string) {
    await startReport(p, SALES_SOURCE)
    await pickIn(p, 'report-builder-add-column', 'report-builder-pick-record-team')
    await addColumn(p, ['amount'])
    await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
    await p.getByTestId('report-builder-section-group').click()
    await p.getByTestId('report-builder-group-add').click()
    await chooseOption(p, 'report-builder-group-0-col', 'team')
    // Only this test's teams: the fill keeps teams whose name starts with the stem.
    await p.getByTestId('report-builder-group-0-fill-on').click()
    await pickIn(p, 'report-builder-group-0-fill-add-condition', 'report-builder-pick-field-name')
    await chooseOption(p, 'report-builder-group-0-fill-0-op', 'startsWith')
    await p.getByTestId('report-builder-group-0-fill-0-value').fill(stem)
    await p.getByTestId('report-builder-section-filter').click()
    await p.getByTestId('report-builder-filter-add-condition').click()
    const picker = p.getByTestId('report-builder-picker')
    await picker.getByTestId('report-builder-pick-link-team').click()
    await picker.getByTestId('report-builder-pick-field-name').click()
    await chooseOption(p, 'report-builder-filter-0-op', 'startsWith')
    await p.getByTestId('report-builder-filter-0-value').fill(stem)
}

const groupRows = (p: Page) =>
    p.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')

// The team's opened rows are shown with their own figures, and only the group row
// shows the region: the opened rows' region cells are there and empty.
async function expectRegionOnGroupOnly(p: Page, team: string) {
    await expect(await cellOf(p, 'amount', 'Ada')).toHaveText('10.00')
    await expect(await cellOf(p, 'amount', 'Abe')).toHaveText('5.00')
    await expect(await cellOf(p, 'formula', team)).toHaveText('South')
    for (const customer of ['Ada', 'Abe']) {
        const region = await cellOf(p, 'formula', customer)
        await expect(region).toBeVisible()
        await expect(region).toHaveText('')
    }
}

test.describe('group levels', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a group by a linked team shows every team, labelled and sorted by a team field', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptrec')
        try {
            const stem = uniqueName('Rec')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            await chooseOption(p, 'report-builder-sort-add-menu', 'record.level.name')
            await p
                .getByTestId('report-builder-sort-0')
                .getByRole('radio', { name: 'Z to A' })
                .click()
            await expect(groupRows(p)).toHaveText([`${stem} C`, `${stem} B`, `${stem} A`])
            await expect(await cellOf(p, 'amount', `${stem} C`)).toHaveText('0.00')
            // A team with no sales has nothing to open.
            await expect(rowOf(p, `${stem} C`)).not.toHaveAttribute('aria-expanded')
            await expect(rowOf(p, `${stem} A`)).toHaveAttribute('aria-expanded', 'false')
        } finally {
            await close()
        }
    })

    test('a group opened while its run is on its way turns its arrow at once and shows a spinner until the rows land', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptpend')
        try {
            const stem = uniqueName('Pend')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            const team = rowOf(p, `${stem} A`)
            await expect(team).toHaveAttribute('aria-expanded', 'false')
            await expect(detailLabels(p)).toHaveCount(0)
            const id = ((await team.getAttribute('data-testid')) ?? '').replace('report-row-', '')
            const label = team.getByText(`${stem} A`, { exact: true })
            const labelX = async () => (await label.boundingBox())?.x ?? null
            // Read the label's place once the first run has landed and the row is still.
            await expect.poll(labelX).not.toBeNull()
            const before = await labelX()
            const release = await holdRuns(p)
            await team.click()
            await expect(team).toHaveAttribute('aria-expanded', 'true')
            await expect(team).toHaveAttribute('aria-busy', 'true')
            await expect(p.getByTestId(`report-pending-${id}`)).toBeVisible()
            await expect(detailLabels(p)).toHaveCount(0)
            // The spinner sits in the caret's slot: the label does not move.
            await expect.poll(labelX).toBe(before)
            release()
            await p.unrouteAll({ behavior: 'wait' })
            await expect(detailLabels(p)).toHaveCount(1)
            await expect(p.getByTestId(`report-pending-${id}`)).toHaveCount(0)
            await expect(team).not.toHaveAttribute('aria-busy', 'true')
        } finally {
            await close()
        }
    })

    test('an asked group condition hides teams and the total adds only the rows shown', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rpthaving')
        try {
            const stem = uniqueName('Hav')
            await seedTeams(p, stem)
            await openReportsHub(p)
            // groupByTeam leaves Columns, Group and sort, and Filter open.
            await groupByTeam(p, stem)
            await chooseOption(p, 'report-builder-column-2-total', 'rows')
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            await addColumn(p, ['quantity'])
            await chooseOption(p, 'report-builder-column-4-aggregate', 'sum')
            await chooseOption(p, 'report-builder-group-0-having-add-column', 'quantity')
            // A condition with no value says so by the column's label, so Save is not
            // blocked without a word.
            await expect(p.getByTestId('report-builder-group-0-having-error')).toHaveText(
                'Quantity: enter a value'
            )
            await chooseOption(p, 'report-builder-group-0-having-0-op', 'gte')
            await p.getByTestId('report-builder-group-0-having-0-ask').click()
            await expect(p.getByTestId('report-builder-group-0-having-error')).toHaveCount(0)
            const title = uniqueName('Having')
            await saveNewReport(p, title)
            await p.getByTestId('report-add-filter').click()
            await p.getByTestId('report-add-filter-quantity').click()
            await p.getByTestId('report-filter-value').fill('3')
            await p.getByTestId('report-filter-apply').click()
            await expect(groupRows(p)).toHaveText([`${stem} A`])
            const totalRow = p
                .getByTestId('report-table')
                .locator('[data-testid^="report-row-total"]')
            const totalId = (await totalRow.getAttribute('data-testid'))?.replace('report-row-', '')
            await expect(p.getByTestId(`report-cell-amount-${totalId}`)).toHaveText('$10.00')
            await expect(p.getByTestId(`report-cell-amount2-${totalId}`)).toHaveText('$13.00')
        } finally {
            await close()
        }
    })

    test('an average group condition keeps its places, and an average of amounts whole cents', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptavg')
        try {
            const stem = uniqueName('Avg')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            const h = 'report-builder-group-0-having'
            await pickIn(p, `${h}-add-total`, 'report-builder-pick-field-quantity')
            await chooseOption(p, `${h}-0-aggregate`, 'avg')
            await chooseOption(p, `${h}-0-places`, '1')
            await expect(p.getByTestId(`${h}-0-places`)).toHaveText('1 place')
            await chooseOption(p, `${h}-0-op`, 'gt')
            await p.getByTestId(`${h}-0-value`).fill('3')
            await expect(groupRows(p)).toHaveText([`${stem} A`])

            await pickIn(p, `${h}-add-total`, 'report-builder-pick-field-amount')
            await chooseOption(p, `${h}-1-aggregate`, 'avg')
            await expect(p.getByTestId(`${h}-1-cents`)).toBeVisible()
            await expect(p.getByTestId(`${h}-1-places`)).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('groups open when the report opens, Collapse all keeps them closed through a live re-run, and opened rows carry their label', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptopen')
        try {
            const stem = uniqueName('Open')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            await p.getByTestId('report-builder-group-0-open').click()
            await p.getByTestId('report-builder-detail-label-on').click()
            const f = 'report-builder-detail-label'
            await chooseOption(p, `${f}-root-part`, 'concat')
            await chooseOption(p, `${f}-0-part`, 'path')
            await pickIn(p, `${f}-0-path`, 'report-builder-pick-field-customer')
            await p.getByTestId(`${f}-root-add`).click()
            await chooseOption(p, `${f}-1-part`, 'text')
            await p.getByTestId(`${f}-1-text`).fill(' at ')
            await p.getByTestId(`${f}-root-add`).click()
            await chooseOption(p, `${f}-2-part`, 'path')
            await pickIn(p, `${f}-2-path`, 'report-builder-pick-field-day')
            const title = uniqueName('Open groups')
            await saveNewReport(p, title)
            await expect(rowOf(p, 'Acme at 2026-01-05')).toBeVisible()
            await expect(rowOf(p, 'Bolt at 2026-01-06')).toBeVisible()
            await pressToolbar(p, 'report-fold-all', 'Collapse all')
            await expect(rowOf(p, 'Acme at 2026-01-05')).toBeHidden()
            const other = await p.context().newPage()
            try {
                // A new page loads the app first (the initial load login also makes).
                await other.goto('/')
                await appShell(other).waitFor({ state: 'visible' })
                await openReportStub(other)
                await addSale(other, {
                    team: `${stem} C`,
                    customer: 'Cole',
                    amount: 50,
                    cost: 0,
                    day: '2026-01-07',
                })
            } finally {
                await other.close()
            }
            await expect(await cellOf(p, 'amount', `${stem} C`)).toHaveText('0.50')
            await expect(rowOf(p, 'Cole at 2026-01-07')).toBeHidden()
        } finally {
            await close()
        }
    })

    test('a group label formula reads the team record, and a formula sort orders the teams', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptlblf')
        try {
            const stem = uniqueName('Lblf')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            await chooseOption(p, 'report-builder-group-0-label', ':formula')
            const f = 'report-builder-group-0-label-formula'
            await p.getByTestId(`${f}-root-add`).click()
            await chooseOption(p, `${f}-1-part`, 'text')
            await p.getByTestId(`${f}-1-text`).fill(' in ')
            await p.getByTestId(`${f}-root-add`).click()
            await chooseOption(p, `${f}-2-part`, 'record')
            await expect(p.getByTestId(`${f}-2-record-group`)).toHaveText('Team')
            await pickIn(p, `${f}-2-record-field`, 'report-builder-pick-field-region')
            await expect(rowOf(p, `${stem} A in North`)).toBeVisible()

            // Sorted by a formula: the sum of each team's quantity, smallest first.
            await chooseOption(p, 'report-builder-sort-add-menu', ':formula')
            const s = 'report-builder-sort-0-formula'
            await chooseOption(p, `${s}-root-part`, 'sum')
            await chooseOption(p, `${s}-0-part`, 'path')
            await pickIn(p, `${s}-0-path`, 'report-builder-pick-field-quantity')
            await expect(p.getByTestId('report-builder-sort-0-col')).toHaveText('Formula')
            const shown = groupRows(p).filter({ hasText: new RegExp(`^${stem} [AB] in North$`) })
            await expect(shown).toHaveText([`${stem} B in North`, `${stem} A in North`])
        } finally {
            await close()
        }
    })

    test('a formula asks whether a day is in an asked period', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptperiod')
        try {
            const team = uniqueName('Period team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Now', amount: 100, cost: 0, day: `${YEAR}-01-05` })
            await addSale(p, {
                team,
                customer: 'Before',
                amount: 100,
                cost: 0,
                day: `${YEAR - 1}-06-05`,
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await pickIn(p, 'report-builder-filter-add-condition', 'report-builder-pick-field-day')
            await chooseOption(p, 'report-builder-filter-0-op', 'between')
            await p.getByTestId('report-builder-filter-0-ask').click()
            await chooseOption(p, 'report-builder-filter-0-default', 'thisFiscalYear')
            await p.getByTestId('report-builder-add-formula').click()
            const f = 'report-builder-column-1-formula'
            await chooseOption(p, `${f}-root-part`, 'if')
            await chooseOption(p, `${f}-0-part`, 'inPeriod')
            await chooseOption(p, `${f}-0-0-part`, 'path')
            await pickIn(p, `${f}-0-0-path`, 'report-builder-pick-field-day')
            await expect(p.getByTestId(`${f}-0-1-param`)).toHaveText('Day')
            await chooseOption(p, `${f}-1-part`, 'text')
            await p.getByTestId(`${f}-1-text`).fill('In')
            await chooseOption(p, `${f}-2-part`, 'text')
            await p.getByTestId(`${f}-2-text`).fill('Out')
            // The formula still reads the asked period once the filter is gone.
            await p.getByTestId('report-builder-filter-0-remove').click()
            await expect(await cellOf(p, 'formula', 'Now')).toHaveText('In')
            await expect(await cellOf(p, 'formula', 'Before')).toHaveText('Out')
        } finally {
            await close()
        }
    })

    test('a link column shows its team by name, then by the field chosen under Show as', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptlbl')
        try {
            const stem = uniqueName('Lbl')
            await openReportStub(p)
            await addTeam(p, `${stem} T`, 'south')
            await addSale(p, {
                team: `${stem} T`,
                customer: `${stem} Cy`,
                amount: 100,
                cost: 0,
                day: '2026-01-05',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await pickIn(p, 'report-builder-add-column', 'report-builder-pick-record-team')
            // The row's label cell is the customer; the team is the link column's cell.
            await expect(await cellOf(p, 'team', `${stem} Cy`)).toHaveText(`${stem} T`)
            await chooseOption(p, 'report-builder-column-1-label-field', 'region')
            await expect(await cellOf(p, 'team', `${stem} Cy`)).toHaveText('South')
        } finally {
            await close()
        }
    })

    test("a column's link keeps or drops rows without it, and a to-many link turns into child rows", async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rpthops')
        try {
            const stem = uniqueName('Hop')
            await openReportStub(p)
            await addTeam(p, `${stem} A`)
            await addTeam(p, `${stem} B`)
            await addSale(p, {
                team: `${stem} A`,
                customer: 'Acme',
                amount: 100,
                cost: 0,
                quantity: 5,
                day: '2026-01-05',
            })
            await openReportsHub(p)
            await startReport(p, TEAMS_SOURCE)
            await addColumn(p, ['sales', 'quantity'], 'total')
            await p.getByTestId('report-builder-section-filter').click()
            await pickIn(p, 'report-builder-filter-add-condition', 'report-builder-pick-field-name')
            await chooseOption(p, 'report-builder-filter-0-op', 'startsWith')
            await p.getByTestId('report-builder-filter-0-value').fill(stem)
            await expect(rowOf(p, `${stem} B`)).toBeVisible()
            await expect(await cellOf(p, 'sales_quantity', `${stem} A`)).toHaveText('5')

            const c = 'report-builder-column-1'
            await expect(p.getByTestId(`${c}-join-0`)).toHaveText('Keep rows without Sales')
            await chooseOption(p, `${c}-join-0`, 'required')
            await expect(p.getByTestId(`${c}-join-0`)).toHaveText('Only rows with Sales')
            await expect(rowOf(p, `${stem} B`)).toBeHidden()
            await expect(rowOf(p, `${stem} A`)).toBeVisible()

            // Child rows show the sales there are: no join to choose.
            await chooseOption(p, `${c}-mode-0`, 'nest')
            await expect(p.getByTestId(c)).toContainText('Sales › Quantity · child rows')
            await expect(p.getByTestId(`${c}-join-0`)).toHaveCount(0)
            await expect(rowOf(p, `${stem} B`)).toBeVisible()
        } finally {
            await close()
        }
    })

    test('a deeper group shows every team of its region, even with no rows, under each region', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptfill2')
        try {
            const stem = uniqueName('Deep')
            await openReportStub(p)
            await addTeam(p, `${stem} N1`, 'north')
            await addTeam(p, `${stem} N2`, 'north')
            await addTeam(p, `${stem} S1`, 'south')
            await addSale(p, {
                team: `${stem} N1`,
                customer: 'Nia',
                amount: 100,
                cost: 0,
                day: '2026-01-05',
            })
            await addSale(p, {
                team: `${stem} S1`,
                customer: 'Sol',
                amount: 100,
                cost: 0,
                day: '2026-01-05',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['team', 'region'])
            await pickIn(p, 'report-builder-add-column', 'report-builder-pick-record-team')
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'team_region')
            await p.getByTestId('report-builder-group-0-open').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-1-col', 'team')
            await p.getByTestId('report-builder-group-1-fill-on').click()
            const fill = 'report-builder-group-1-fill'
            await pickIn(p, `${fill}-add-condition`, 'report-builder-pick-field-name')
            await chooseOption(p, `${fill}-0-op`, 'startsWith')
            await p.getByTestId(`${fill}-0-value`).fill(stem)
            await pickIn(p, `${fill}-add-condition`, 'report-builder-pick-field-region')
            await chooseOption(p, `${fill}-1-above`, 'level')
            await expect(p.getByTestId(`${fill}-1-value`)).toHaveCount(0)
            await chooseOption(p, 'report-builder-sort-add-menu', 'record.level2.name')
            await expect(groupRows(p).filter({ hasText: stem })).toHaveText([
                `${stem} N1`,
                `${stem} N2`,
                `${stem} S1`,
            ])
            await expect(rowOf(p, `${stem} N2`)).not.toHaveAttribute('aria-expanded')
        } finally {
            await close()
        }
    })

    // E2: a column that only names its group (the team's region) can be left blank on
    // opened rows; its group row keeps the value. The builder offers it only on a
    // grouped report and never on the first column, which names opened rows.
    test('a column left blank on opened rows is empty there while its group row shows it', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptblank')
        try {
            const team = uniqueName('Blank')
            await openReportStub(p)
            await addTeam(p, team, 'south')
            await addSale(p, { team, customer: 'Ada', amount: 1000, cost: 0, day: '2026-01-05' })
            await addSale(p, { team, customer: 'Abe', amount: 500, cost: 0, day: '2026-01-06' })
            await openReportsHub(p)
            await startTeamSales(p)
            await expect(p.getByTestId('report-builder-column-2')).toBeVisible()
            await expect(p.getByTestId('report-builder-column-2-detail')).toHaveCount(0)
            await groupWithTeamRegion(p)
            await expect(p.getByTestId('report-builder-column-0')).toBeVisible()
            await expect(p.getByTestId('report-builder-column-0-detail')).toHaveCount(0)
            await expect(p.getByTestId('report-builder-column-2-detail')).toBeVisible()

            await rowOf(p, team).click()
            await expect(rowOf(p, team)).toHaveAttribute('aria-expanded', 'true')
            await expect(await cellOf(p, 'formula', 'Ada')).toHaveText('South')
            await chooseOption(p, 'report-builder-column-3-detail', 'blank')
            await expectRegionOnGroupOnly(p, team)

            await saveNewReport(p, uniqueName('Blank region'))
            await rowOf(p, team).click()
            await expect(rowOf(p, team)).toHaveAttribute('aria-expanded', 'true')
            await expectRegionOnGroupOnly(p, team)

            // Shown on opened rows again.
            await pressToolbar(p, 'report-edit-open', 'Edit report')
            await chooseOption(p, 'report-builder-column-3-detail', 'show')
            await expect(await cellOf(p, 'formula', 'Ada')).toHaveText('South')
            await expect(await cellOf(p, 'formula', 'Abe')).toHaveText('South')
        } finally {
            await close()
        }
    })

    // The builder never offers what the engine refuses: a group row's value it does
    // not have, a total of a running value, a sort by one, a having on a layout's
    // first group, or a row label on a pivot.
    test('the builder offers no choice the engine refuses', async ({ page }) => {
        await login(page)
        await openReportsHub(page)
        await startReport(page, SALES_SOURCE)
        await addColumn(page, ['status'])
        await addColumn(page, ['amount'])
        await chooseOption(page, 'report-builder-column-2-aggregate', 'sum')
        await page.getByTestId('report-builder-add-formula').click()
        await chooseOption(page, 'report-builder-column-3-formula-root-part', 'runningSum')
        await expect(page.getByTestId('report-builder-column-2-total')).toBeVisible()
        await expect(page.getByTestId('report-builder-column-3-total')).toHaveCount(0)

        await page.getByTestId('report-builder-section-group').click()
        await page.getByTestId('report-builder-group-add').click()
        await chooseOption(page, 'report-builder-group-0-col', 'status')
        const having = 'report-builder-group-0-having-add-column'
        await page.getByTestId(having).click()
        await expect(page.getByTestId(`${having}-amount`)).toBeVisible()
        await expect(page.getByTestId(`${having}-status`)).toBeVisible()
        // A row's customer, and a running value, have no one value on a group row.
        await expect(page.getByTestId(`${having}-customer`)).toHaveCount(0)
        await expect(page.getByTestId(`${having}-formula`)).toHaveCount(0)
        await page.keyboard.press('Escape')
        await page.getByTestId('report-builder-sort-add-menu').click()
        await expect(page.getByTestId('report-builder-sort-add-menu-amount')).toBeVisible()
        await expect(page.getByTestId('report-builder-sort-add-menu-formula')).toHaveCount(0)
        await page.keyboard.press('Escape')

        await page.getByTestId('report-builder-section-layout').click()
        await page.getByTestId('report-builder-layout-add-section').click()
        await expect(page.getByTestId('report-builder-group-0-having-add-total')).toHaveCount(0)

        await expect(page.getByTestId('report-builder-detail-label-on')).toBeVisible()
        await page.getByTestId('report-builder-section-pivot').click()
        await page.getByTestId('report-builder-pivot-on').click()
        await expect(page.getByTestId('report-builder-detail-label-on')).toHaveCount(0)
    })

    test('on a phone a group by a linked team is set up in the builder sheet', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptrecph')
        try {
            const stem = uniqueName('RecPh')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await p.setViewportSize({ width: 390, height: 844 })
            await groupByTeam(p, stem)
            await p.getByTestId('report-builder-mode-preview').click()
            await expect(groupRows(p)).toHaveCount(3)
            await expect(rowOf(p, `${stem} C`)).toBeVisible()
        } finally {
            await close()
        }
    })
})
