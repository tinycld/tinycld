import { DEFAULT_DISPLAY, typedLabelText } from './format'
import {
    type ChoiceLabels,
    choiceCount,
    groupLabelKey,
    NAMED_CHOICES,
    type ParamValues,
} from './params'
import type { ParamDef } from './spec'
import type { ParamOption, ParamOptionGroup, ParamOptionsPage } from './types'

// The options of a choice param read from a source (POST /api/reports/param-options),
// as the picker and the chips show them.

export type OptionSection = { group: ParamOptionGroup | null; options: ParamOption[] }

const NUMERIC_LABELS = new Set(['number', 'money', 'percent'])

/**
 * An option's or a group's label as it is shown: a typed label (a number, an amount,
 * a percent, a date) formatted as a cell of its type, an amount with its currency.
 * The server sends the stored value as text ("500000", "2019-02-10 00:00:00.000Z").
 */
export function optionText(option: ParamOption | ParamOptionGroup): string {
    const { type, label } = option
    if (!type) return label
    const value = NUMERIC_LABELS.has(type) ? Number(label) : label
    if (typeof value === 'number' && !Number.isFinite(value)) return label
    return typedLabelText(type, value, option.currency ?? '', DEFAULT_DISPLAY)
}

/** The loaded options in sections: ungrouped first, then each group in the server's order. */
export function optionSections(pages: readonly ParamOptionsPage[]): OptionSection[] {
    const options = pages.flatMap(page => page.options)
    // Groups come with the first page; a group seen twice is one head (F9). An
    // option of a group no page listed (past the 200 the server lists) gets a head
    // of its own, named by its value.
    const groups = new Map<string, ParamOptionGroup>()
    for (const group of pages.flatMap(page => page.groups)) {
        if (!groups.has(group.value)) groups.set(group.value, group)
    }
    for (const option of options) {
        if (option.group && !groups.has(option.group)) {
            groups.set(option.group, { value: option.group, label: option.group })
        }
    }
    const loose = options.filter(o => !o.group)
    const sections: OptionSection[] = loose.length > 0 ? [{ group: null, options: loose }] : []
    for (const group of groups.values()) {
        const inGroup = options.filter(o => o.group === group.value)
        if (inGroup.length > 0) sections.push({ group, options: inGroup })
    }
    return sections
}

/** A group picked whole (or unpicked): the server expands it at run time (R30). */
export function toggleGroup(groups: readonly string[], group: string): string[] {
    return groups.includes(group) ? groups.filter(g => g !== group) : [...groups, group]
}

/**
 * The choices from a source whose chips name them: a chip with more choices counts
 * them, so their labels are not read.
 */
export function chipLabelAsks(
    defs: readonly ParamDef[],
    values: ParamValues
): { param: string; values: string[]; groups: string[] }[] {
    return defs.flatMap(def => {
        const value = values[def.id]
        if (!def.optionsFrom || value?.kind !== 'choice') return []
        if (choiceCount(value) > NAMED_CHOICES) return []
        return [{ param: def.id, values: value.values, groups: value.groups ?? [] }]
    })
}

/** The shown labels of the options and groups in these pages, filed as the chips read them. */
export function labelsOfPages(pages: readonly ParamOptionsPage[]): ChoiceLabels {
    return new Map([
        ...pages.flatMap(page => page.options.map(o => [o.value, optionText(o)] as const)),
        ...pages.flatMap(page =>
            page.groups.map(g => [groupLabelKey(g.value), optionText(g)] as const)
        ),
    ])
}
