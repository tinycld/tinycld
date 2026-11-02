import { describe, expect, it } from 'vitest'
import { describeNode } from '../formula-describe'
import { CTX, IN_PERIOD, LARGER, LOOKUP, TEAM_WITH_SALES, WITH_PERIOD } from './formula-fixture'

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
