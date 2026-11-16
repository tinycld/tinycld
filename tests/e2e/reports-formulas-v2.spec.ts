import type { Page } from '@playwright/test'
import { skipWithoutReportsStub } from './helpers'
import { expect, shareMember, test } from './member-fixtures'
import {
    addColumn,
    cellOf,
    chooseOption,
    openReportsHub,
    SALES_SOURCE,
    seedStub,
    startReport,
    TEAMS_SOURCE,
    uniqueName,
} from './reports-helpers'

/**
 * The report formula editor (OQ2): every report operator, edited from menus.
 * The file's shared member owns one sale, for one team, which the three lookup
 * tests read; no test adds a sale to it or saves a report. The other tests have a
 * member each, with their own sales. Teams are shared, so names are unique.
 */

// The shared member's one team. A new worker loads this file again, so it gets a
// new name together with its new member.
const LOOKUP_TEAM = uniqueName('Lookup team')

shareMember('rptlookup', async p => {
    await seedStub(p, {
        teams: [{ name: LOOKUP_TEAM }],
        sales: [{ team: LOOKUP_TEAM, customer: 'Only', amount: 100, cost: 0, day: '2026-01-05' }],
    })
})

/** Picks a field in the open column picker: link keys, then a field key. */
async function pickPath(p: Page, trigger: string, keys: string[]) {
    await p.getByTestId(trigger).click()
    const picker = p.getByTestId('report-builder-picker')
    for (const key of keys.slice(0, -1)) {
        await picker.getByTestId(`report-builder-pick-link-${key}`).click()
    }
    await picker.getByTestId(`report-builder-pick-field-${keys[keys.length - 1]}`).click()
    await expect(picker).toBeHidden()
}

// A formula column that looks up the team named `team` and shows its name.
async function buildTeamLookup(p: Page, team: string) {
    await startReport(p, SALES_SOURCE)
    await p.getByTestId('report-builder-add-formula').click()
    const f = 'report-builder-column-1-formula'
    await chooseOption(p, `${f}-root-part`, 'lookup')
    await chooseOption(p, `${f}-root-source`, TEAMS_SOURCE)
    await chooseOption(p, `${f}-0-part`, 'eq')
    await chooseOption(p, `${f}-0-0-part`, 'path')
    await pickPath(p, `${f}-0-0-path`, ['name'])
    await chooseOption(p, `${f}-0-1-part`, 'text')
    await p.getByTestId(`${f}-0-1-text`).fill(team)
    await chooseOption(p, `${f}-1-part`, 'path')
    await pickPath(p, `${f}-1-path`, ['name'])
    const summary = `= look up Name in Stub teams where Name = "${team}" (smallest)`
    await expect(p.getByTestId(`${f}-summary`)).toHaveText(summary)
    // Picking the same source again keeps its where and value.
    await chooseOption(p, `${f}-root-source`, TEAMS_SOURCE)
    await expect(p.getByTestId(`${f}-summary`)).toHaveText(summary)
}

test.describe('report formulas', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an if over a comparison picks the larger of amount and cost', async ({ freshMember }) => {
        const p = freshMember.page
        const team = uniqueName('Team')
        await seedStub(p, {
            teams: [{ name: team }],
            sales: [
                { team, customer: 'Cheap', amount: 300, cost: 900, day: '2026-01-05' },
                { team, customer: 'Dear', amount: 1500, cost: 400, day: '2026-01-06' },
            ],
        })
        await openReportsHub(p)
        await startReport(p, SALES_SOURCE)
        await addColumn(p, ['amount'])
        await addColumn(p, ['cost'])
        await p.getByTestId('report-builder-add-formula').click()
        const f = 'report-builder-column-3-formula'
        await chooseOption(p, `${f}-root-part`, 'if')
        await expect(p.getByTestId(f)).toContainText('When')
        await chooseOption(p, `${f}-0-part`, 'gt')
        await chooseOption(p, `${f}-0-0-part`, 'field')
        await chooseOption(p, `${f}-0-0-field`, 'amount')
        await chooseOption(p, `${f}-0-1-part`, 'field')
        await chooseOption(p, `${f}-0-1-field`, 'cost')
        await chooseOption(p, `${f}-1-part`, 'field')
        await chooseOption(p, `${f}-1-field`, 'amount')
        await chooseOption(p, `${f}-2-part`, 'field')
        await chooseOption(p, `${f}-2-field`, 'cost')
        await expect(p.getByTestId(`${f}-summary`)).toHaveText(
            '= if Amount > Cost then Amount else Cost'
        )
        await expect(await cellOf(p, 'formula', 'Cheap')).toHaveText('9.00')
        await expect(await cellOf(p, 'formula', 'Dear')).toHaveText('15.00')
    })

    test('a first match gains and loses its Otherwise; a field path keeps only rows with a team', async ({
        freshMember,
    }) => {
        const p = freshMember.page
        const team = uniqueName('Case team')
        await seedStub(p, {
            teams: [{ name: team }],
            sales: [
                { team, customer: 'Cheap', amount: 300, cost: 900, day: '2026-01-05' },
                { team, customer: 'Dear', amount: 1500, cost: 400, day: '2026-01-06' },
            ],
        })
        await openReportsHub(p)
        await startReport(p, SALES_SOURCE)
        await addColumn(p, ['amount'])
        await addColumn(p, ['cost'])
        await p.getByTestId('report-builder-add-formula').click()
        const f = 'report-builder-column-3-formula'
        await chooseOption(p, `${f}-root-part`, 'case')
        await chooseOption(p, `${f}-0-part`, 'gt')
        await chooseOption(p, `${f}-0-0-part`, 'field')
        await chooseOption(p, `${f}-0-0-field`, 'amount')
        await chooseOption(p, `${f}-0-1-part`, 'field')
        await chooseOption(p, `${f}-0-1-field`, 'cost')
        await chooseOption(p, `${f}-1-part`, 'path')
        await pickPath(p, `${f}-1-path`, ['team', 'name'])
        await chooseOption(p, `${f}-1-join-0`, 'required')
        await expect(p.getByTestId(`${f}-1-join-0`)).toHaveText('Only rows with Team')

        await p.getByTestId(`${f}-root-otherwise-add`).click()
        await chooseOption(p, `${f}-2-part`, 'text')
        await p.getByTestId(`${f}-2-text`).fill('Small')
        await expect(p.getByTestId(`${f}-summary`)).toHaveText(
            '= first match of (Amount > Cost, Team › Name, "Small")'
        )
        await expect(await cellOf(p, 'formula', 'Dear')).toHaveText(team)
        await expect(await cellOf(p, 'formula', 'Cheap')).toHaveText('Small')

        await p.getByTestId(`${f}-root-otherwise-remove`).click()
        await expect(p.getByTestId(`${f}-summary`)).toHaveText(
            '= first match of (Amount > Cost, Team › Name)'
        )
        await expect(await cellOf(p, 'formula', 'Cheap')).toBeEmpty()
    })

    // The list is typed a key at a time, so "Cheap," passes on its way to the next item.
    test('an if tests a value against a typed list', async ({ freshMember }) => {
        const p = freshMember.page
        const team = uniqueName('List team')
        await seedStub(p, {
            teams: [{ name: team }],
            sales: [
                { team, customer: 'Cheap', amount: 300, cost: 0, day: '2026-01-05' },
                { team, customer: 'Dear', amount: 1500, cost: 0, day: '2026-01-06' },
            ],
        })
        await openReportsHub(p)
        await startReport(p, SALES_SOURCE)
        await p.getByTestId('report-builder-add-formula').click()
        const f = 'report-builder-column-1-formula'
        await chooseOption(p, `${f}-root-part`, 'if')
        await chooseOption(p, `${f}-0-part`, 'in')
        await chooseOption(p, `${f}-0-0-part`, 'path')
        await pickPath(p, `${f}-0-0-path`, ['customer'])
        await p.getByTestId(`${f}-0-1-list`).pressSequentially('Cheap, Other')
        await expect(p.getByTestId(`${f}-0-1-list`)).toHaveValue('Cheap, Other')
        await chooseOption(p, `${f}-1-part`, 'text')
        await p.getByTestId(`${f}-1-text`).fill('Listed')
        await chooseOption(p, `${f}-2-part`, 'text')
        await p.getByTestId(`${f}-2-text`).fill('Not listed')
        await expect(p.getByTestId(`${f}-summary`)).toHaveText(
            '= if Customer is any of ["Cheap", "Other"] then "Listed" else "Not listed"'
        )
        await expect(await cellOf(p, 'formula', 'Cheap')).toHaveText('Listed')
        await expect(await cellOf(p, 'formula', 'Dear')).toHaveText('Not listed')
    })

    test('a lookup reads a team by name from another source', async ({ p }) => {
        await openReportsHub(p)
        await buildTeamLookup(p, LOOKUP_TEAM)
        await expect(await cellOf(p, 'formula', 'Only')).toHaveText(LOOKUP_TEAM)
    })

    test('on a phone a lookup is built from menus in the builder sheet', async ({ p }) => {
        await openReportsHub(p)
        await p.setViewportSize({ width: 390, height: 844 })
        await buildTeamLookup(p, LOOKUP_TEAM)
        await p.getByTestId('report-builder-mode-preview').click()
        await expect(await cellOf(p, 'formula', 'Only')).toHaveText(LOOKUP_TEAM)
    })

    // The reader sees only their own sales (the stub's rules), so the one team that
    // has a sale over 0 for this member is the team they sold for.
    test('a lookup keeps the teams that have related sales', async ({ p }) => {
        await openReportsHub(p)
        await startReport(p, SALES_SOURCE)
        await p.getByTestId('report-builder-add-formula').click()
        const f = 'report-builder-column-1-formula'
        await chooseOption(p, `${f}-root-part`, 'lookup')
        await chooseOption(p, `${f}-root-source`, TEAMS_SOURCE)
        await chooseOption(p, `${f}-0-part`, 'exists')
        await chooseOption(p, `${f}-0-link`, 'sales')
        await chooseOption(p, `${f}-0-0-part`, 'gt')
        await chooseOption(p, `${f}-0-0-0-part`, 'path')
        await pickPath(p, `${f}-0-0-0-path`, ['amount'])
        await chooseOption(p, `${f}-1-part`, 'path')
        await pickPath(p, `${f}-1-path`, ['name'])
        await expect(p.getByTestId(`${f}-summary`)).toHaveText(
            '= look up Name in Stub teams where has related Sales where Amount > 0 (smallest)'
        )
        await expect(await cellOf(p, 'formula', 'Only')).toHaveText(LOOKUP_TEAM)
    })
})
