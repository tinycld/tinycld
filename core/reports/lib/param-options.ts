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

export type OptionSection = {
    /** The group whose head the section has; null with no head, or for Other. */
    group: ParamOptionGroup | null
    /** The options of groups the server did not list. */
    isOther?: true
    options: ParamOption[]
}

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

/**
 * The loaded options in sections. An ungrouped param's options are one section with
 * no head. A grouped param's options sit under their groups in the server's order;
 * an option with no group is in the blank group ("" value, which the server lists
 * first). The options of a group the server did not list (past the 200 it lists,
 * `groupsCut`) have no label to head them, so they share one Other section.
 */
export function optionSections(
    pages: readonly ParamOptionsPage[],
    isGrouped: boolean
): OptionSection[] {
    const options = pages.flatMap(page => page.options)
    if (!isGrouped) return options.length > 0 ? [{ group: null, options }] : []
    // Groups come with the first page; a group seen twice is one head (F9).
    const groups = new Map<string, ParamOptionGroup>()
    for (const group of pages.flatMap(page => page.groups)) {
        if (!groups.has(group.value)) groups.set(group.value, group)
    }
    const groupOf = (option: ParamOption) => option.group ?? ''
    const sections: OptionSection[] = []
    for (const group of groups.values()) {
        const inGroup = options.filter(o => groupOf(o) === group.value)
        if (inGroup.length > 0) sections.push({ group, options: inGroup })
    }
    const other = options.filter(o => !groups.has(groupOf(o)))
    if (other.length > 0) sections.push({ group: null, isOther: true, options: other })
    return sections
}

/** Whether a choice from a source has option groups (optionsFrom.group). */
export function hasOptionGroups(def: ParamDef): boolean {
    return (def.optionsFrom?.group?.length ?? 0) > 0
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
