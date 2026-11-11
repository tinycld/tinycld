import { describe, expect, it } from 'vitest'
import { describeNode } from '../formula-describe'
import { SALES_KEY } from './catalog-fixture'
import {
    CTX,
    IN_PERIOD,
    LARGER,
    LOOKUP,
    SHARE,
    TEAM_WITH_SALES,
    WITH_PERIOD,
    WITH_SECTIONS,
} from './formula-fixture'

describe('describeNode', () => {
    it('says a formula as a person would', () => {
        const net = { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] }
        expect(describeNode(CTX, net)).toBe('Amount − Cost')
        expect(describeNode(CTX, LARGER)).toBe('if Amount > Cost then Amount else Cost')
        expect(describeNode(CTX, LOOKUP)).toBe(
            'look up ID in Stub teams where Name = "North" (smallest)'
        )
        expect(describeNode(CTX, { record: 'byTeam', path: [{ field: 'name' }] })).toBe(
            'Team › Name'
        )
        expect(describeNode(WITH_PERIOD, IN_PERIOD)).toBe('Day is in {Period}')
    })
    it('says an exists inside a lookup by its link and its where', () => {
        expect(describeNode(CTX, TEAM_WITH_SALES)).toBe(
            'look up Name in Stub teams where has related Sales where Amount > 0 (smallest)'
        )
        const none = { ...TEAM_WITH_SALES, where: { op: 'notExists', link: [{ link: 'sales' }] } }
        expect(describeNode(CTX, none)).toBe(
            'look up Name in Stub teams where has no related Sales (smallest)'
        )
    })
    it('names a part it cannot show', () => {
        expect(describeNode(CTX, { op: 'shout', args: [] })).toBe('a part the editor cannot show')
    })
})

describe('describeNode for the v3 parts', () => {
    const sameDay = (pick: 'sum' | 'count', field: string) => ({
        op: 'lookup',
        source: SALES_KEY,
        where: { op: 'eq', args: [{ path: [{ field: 'day' }] }, { outer: [{ field: 'day' }] }] },
        value: { path: [{ field }] },
        pick,
    })

    it("says add up, count and this row's field", () => {
        expect(describeNode(CTX, sameDay('sum', 'amount'))).toBe(
            "add up Amount in Stub sales where Day = this row's Day"
        )
        expect(describeNode(CTX, sameDay('count', 'id'))).toBe(
            "count ID in Stub sales where Day = this row's Day"
        )
    })

    it('says a range start or end by its param', () => {
        const before = {
            op: 'lt',
            args: [{ path: [{ field: 'day' }] }, { param: 'period', bound: 'from' as const }],
        }
        expect(describeNode(WITH_PERIOD, before)).toBe('Day < {Period start}')
        expect(describeNode(WITH_PERIOD, { param: 'period', bound: 'to' })).toBe('{Period end}')
    })
})

describe('a percent of a section', () => {
    it('names the column and the section', () => {
        expect(describeNode(WITH_SECTIONS, SHARE)).toBe('Amount as % of Open sales')
        expect(describeNode(WITH_SECTIONS, { ...SHARE, subtotal: 'gone' })).toBe(
            'Amount as % of gone (missing)'
        )
    })
})
