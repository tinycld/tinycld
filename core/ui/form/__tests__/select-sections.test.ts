import { describe, expect, it } from 'vitest'
import type { SelectOption } from '../ButtonGroup'
import { selectSections } from '../select-sections'

const none: SelectOption = { value: '', label: 'None' }
const apple: SelectOption = { value: 'a', label: 'Apple', section: 'Fruit' }
const banana: SelectOption = { value: 'b', label: 'Banana', section: 'Fruit' }
const carrot: SelectOption = { value: 'c', label: 'Carrot', section: 'Roots' }

describe('selectSections', () => {
    it('puts consecutive options of one section in one labelled run, after an unlabelled run', () => {
        expect(selectSections([none, apple, banana, carrot])).toEqual([
            { label: null, options: [none] },
            { label: 'Fruit', options: [apple, banana] },
            { label: 'Roots', options: [carrot] },
        ])
    })

    it('gives options with no section one unlabelled run', () => {
        const plain = [
            { value: 'x', label: 'Ex' },
            { value: 'y', label: 'Why' },
            { value: 'z', label: 'Zed' },
        ]
        expect(selectSections(plain)).toEqual([{ label: null, options: plain }])
    })

    it('keeps the given order: a section that comes back after another forms a second run', () => {
        const date: SelectOption = { value: 'd', label: 'Date', section: 'Fruit' }
        expect(selectSections([apple, carrot, date])).toEqual([
            { label: 'Fruit', options: [apple] },
            { label: 'Roots', options: [carrot] },
            { label: 'Fruit', options: [date] },
        ])
    })

    it('gives no runs for no options', () => {
        expect(selectSections([])).toEqual([])
    })
})
