import { describe, expect, it } from 'vitest'
import {
    chipLabelAsks,
    labelsOfPages,
    optionSections,
    optionText,
    toggleGroup,
} from '../param-options'
import type { ParamDef } from '../spec'

describe('options from a source', () => {
    it('sections the loaded pages by group, in the groups order, the blank group first', () => {
        // The server lists the blank group ("" value, "(none)") first and sends its
        // options with no group (options.go: Group is omitted when empty).
        const pages = [
            {
                options: [
                    { value: 'a', label: 'Bank', group: 'asset' },
                    { value: 'b', label: 'Sales', group: 'income' },
                ],
                groups: [
                    { value: '', label: '(none)' },
                    { value: 'asset', label: 'Asset' },
                    { value: 'income', label: 'Income' },
                ],
                next: 'x',
            },
            {
                options: [
                    { value: 'c', label: 'Cash', group: 'asset' },
                    { value: 'd', label: 'Misc' },
                ],
                groups: [],
            },
        ]
        expect(optionSections(pages, true)).toEqual([
            { group: { value: '', label: '(none)' }, options: [{ value: 'd', label: 'Misc' }] },
            {
                group: { value: 'asset', label: 'Asset' },
                options: [
                    { value: 'a', label: 'Bank', group: 'asset' },
                    { value: 'c', label: 'Cash', group: 'asset' },
                ],
            },
            {
                group: { value: 'income', label: 'Income' },
                options: [{ value: 'b', label: 'Sales', group: 'income' }],
            },
        ])
    })

    it('keeps one head per group when a later page repeats the groups', () => {
        const groups = [{ value: 'asset', label: 'Asset' }]
        const pages = [
            { options: [{ value: 'a', label: 'Bank', group: 'asset' }], groups, next: 'x' },
            { options: [{ value: 'c', label: 'Cash', group: 'asset' }], groups },
        ]
        expect(
            optionSections(pages, true).map(s => [s.group?.value, s.options.map(o => o.value)])
        ).toEqual([['asset', ['a', 'c']]])
    })

    it('files the options of groups past the listed ones under Other, never under a raw value', () => {
        // Past the 200 groups the server lists (groupsCut), a group has no label.
        const pages = [
            {
                options: [
                    { value: 'a', label: 'Bank', group: 'asset' },
                    { value: 'e', label: 'Gear', group: 'r7k2' },
                    { value: 'f', label: 'Loose' },
                ],
                groups: [{ value: 'asset', label: 'Asset' }],
                groupsCut: true,
            },
        ]
        expect(optionSections(pages, true)).toEqual([
            {
                group: { value: 'asset', label: 'Asset' },
                options: [{ value: 'a', label: 'Bank', group: 'asset' }],
            },
            {
                group: null,
                isOther: true,
                options: [
                    { value: 'e', label: 'Gear', group: 'r7k2' },
                    { value: 'f', label: 'Loose' },
                ],
            },
        ])
    })

    it('lists the options of an ungrouped param in one section with no head', () => {
        const pages = [{ options: [{ value: 'a', label: 'Bank' }], groups: [] }]
        expect(optionSections(pages, false)).toEqual([
            { group: null, options: [{ value: 'a', label: 'Bank' }] },
        ])
        expect(optionSections([{ options: [], groups: [] }], false)).toEqual([])
    })

    it('picks a whole group once and unpicks it', () => {
        expect(toggleGroup(['asset'], 'income')).toEqual(['asset', 'income'])
        expect(toggleGroup(['asset', 'income'], 'asset')).toEqual(['income'])
    })
})

describe('option labels', () => {
    it('formats a typed label as a cell of its type, an amount with its currency', () => {
        expect(optionText({ value: 'a', label: 'Bank' })).toBe('Bank')
        expect(optionText({ value: 'a', label: '500000', type: 'money', currency: 'EUR' })).toBe(
            '€5,000.00'
        )
        expect(optionText({ value: 'a', label: '1.5e+06', type: 'money', currency: 'USD' })).toBe(
            '$15,000.00'
        )
        expect(optionText({ value: 'a', label: '1234567', type: 'number' })).toBe('1,234,567')
        expect(optionText({ value: 'a', label: '2019-02-10 00:00:00.000Z', type: 'date' })).toBe(
            'Feb 10, 2019'
        )
        expect(
            optionText({ value: '300 EUR', label: '30000', type: 'money', currency: 'EUR' })
        ).toBe('€300.00')
    })

    it('files option labels by value and group labels under group:<value>', () => {
        const labels = labelsOfPages([
            {
                options: [{ value: 'a', label: '500000', type: 'money', currency: 'USD' }],
                groups: [{ value: 'north', label: 'North' }],
            },
            { options: [{ value: 'b', label: 'Cash' }], groups: [] },
        ])
        expect([...labels]).toEqual([
            ['a', '$5,000.00'],
            ['b', 'Cash'],
            ['group:north', 'North'],
        ])
    })
})

describe('which chips read labels', () => {
    const teams: ParamDef = {
        id: 'teams',
        type: 'choice',
        multiple: true,
        optionsFrom: {
            source: 's',
            value: [{ field: 'id' }],
            label: [{ field: 'name' }],
            group: [{ field: 'region' }],
        },
    }
    const status: ParamDef = {
        id: 'status',
        type: 'choice',
        options: [{ value: 'open', label: 'Open' }],
    }

    it('asks only for choices from a source that the chip names', () => {
        expect(
            chipLabelAsks([teams, status], {
                teams: { kind: 'choice', values: ['a1'], groups: ['north'] },
                status: { kind: 'choice', values: ['open'] },
            })
        ).toEqual([{ param: 'teams', values: ['a1'], groups: ['north'] }])
        // Three choices are counted on the chip, so no label is read.
        expect(
            chipLabelAsks([teams], {
                teams: { kind: 'choice', values: ['a1', 'b2'], groups: ['north'] },
            })
        ).toEqual([])
        expect(chipLabelAsks([teams], {})).toEqual([])
    })
})
