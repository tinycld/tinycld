import type { SelectOption } from './ButtonGroup'

export type SelectSection = { label: string | null; options: SelectOption[] }

/**
 * Splits a select's options into runs of one section each, in the given
 * order. A run starts whenever an option's section differs from the option
 * before it, so nothing is regrouped; options with no section form runs with
 * a `null` label.
 */
export function selectSections(options: readonly SelectOption[]): SelectSection[] {
    const runs: SelectSection[] = []
    for (const option of options) {
        const label = option.section || null
        const last = runs.at(-1)
        if (last && last.label === label) {
            last.options.push(option)
        } else {
            runs.push({ label, options: [option] })
        }
    }
    return runs
}
