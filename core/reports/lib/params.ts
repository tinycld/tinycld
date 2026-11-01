import { dayWords, isIsoDay, rangeChipText, rangeWords, shortDay } from './dates'
import type { ParamDef } from './spec'
import type { ResolvedParam } from './types'

// A report's params as the shell edits them: the period chip and the filter chips.
// Presets carry the engine's names (core/server/reports/params.go) and the Books
// shell's labels. The server resolves a preset against the reader's day and the
// source's fiscal year; the client only names it.

export const RANGE_PRESETS = [
    'today',
    'thisMonth',
    'thisMonthToDate',
    'thisQuarter',
    'thisQuarterToDate',
    'thisFiscalYear',
    'thisFiscalYearToDate',
    'lastMonth',
    'lastQuarter',
    'lastFiscalYear',
    'last12Months',
] as const
export const AS_OF_PRESETS = [
    'today',
    'endOfLastMonth',
    'endOfLastQuarter',
    'endOfLastFiscalYear',
] as const
const DAY_PRESETS = ['today'] as const
export type Preset = (typeof RANGE_PRESETS)[number] | (typeof AS_OF_PRESETS)[number]

export const PRESET_LABELS: Record<Preset, string> = {
    today: 'Today',
    thisMonth: 'This month',
    thisMonthToDate: 'This month to date',
    thisQuarter: 'This quarter',
    thisQuarterToDate: 'This quarter to date',
    thisFiscalYear: 'This fiscal year',
    thisFiscalYearToDate: 'This fiscal year to date',
    lastMonth: 'Last month',
    lastQuarter: 'Last quarter',
    lastFiscalYear: 'Last fiscal year',
    last12Months: 'Last 12 months',
    endOfLastMonth: 'End of last month',
    endOfLastQuarter: 'End of last quarter',
    endOfLastFiscalYear: 'End of last fiscal year',
}
export const CUSTOM_LABEL = 'Custom'
export const MAX_TEXT_PARAM = 500

export type ParamValue =
    | { kind: 'preset'; preset: Preset }
    | { kind: 'day'; day: string }
    | { kind: 'range'; from: string; to: string }
    | { kind: 'choice'; values: string[] }
    | { kind: 'text'; text: string }
    | { kind: 'number'; number: number }

/** The values the reader set; a param that is not here takes its default, or is unset. */
export type ParamValues = Readonly<Record<string, ParamValue>>

const isRecord = (value: unknown): value is Record<string, unknown> =>
    typeof value === 'object' && value !== null && !Array.isArray(value)

export function isDateParam(def: Pick<ParamDef, 'type'>): boolean {
    return def.type === 'date' || def.type === 'asOf' || def.type === 'dateRange'
}

export function presetsFor(def: Pick<ParamDef, 'type'>): readonly Preset[] {
    if (def.type === 'dateRange') return RANGE_PRESETS
    if (def.type === 'asOf') return AS_OF_PRESETS
    if (def.type === 'date') return DAY_PRESETS
    return []
}

function presetValue(def: ParamDef, raw: unknown): ParamValue | null {
    if (!isRecord(raw) || Object.keys(raw).length !== 1) return null
    const preset = presetsFor(def).find(p => p === raw.preset)
    return preset ? { kind: 'preset', preset } : null
}

export function rangeValue(from: string, to: string): ParamValue | null {
    return isIsoDay(from) && isIsoDay(to) && from <= to ? { kind: 'range', from, to } : null
}

function choiceValue(def: ParamDef, raw: unknown): ParamValue | null {
    if (!Array.isArray(raw) || raw.length === 0) return null
    const offered = (def.options ?? []).map(option => option.value)
    if (!raw.every(item => typeof item === 'string' && offered.includes(item))) return null
    const values = offered.filter(value => raw.includes(value))
    if (!def.multiple && values.length !== 1) return null
    return { kind: 'choice', values }
}

/** A value from its JSON form (a spec default): null when the JSON does not fit the param. */
export function valueFromJson(def: ParamDef, raw: unknown): ParamValue | null {
    switch (def.type) {
        case 'date':
        case 'asOf':
            if (typeof raw === 'string') return isIsoDay(raw) ? { kind: 'day', day: raw } : null
            return presetValue(def, raw)
        case 'dateRange':
            if (
                isRecord(raw) &&
                typeof raw.from === 'string' &&
                typeof raw.to === 'string' &&
                Object.keys(raw).length === 2
            ) {
                return rangeValue(raw.from, raw.to)
            }
            return presetValue(def, raw)
        case 'choice':
            return choiceValue(def, def.multiple ? raw : [raw])
        case 'text':
            return typeof raw === 'string' && raw.length <= MAX_TEXT_PARAM
                ? { kind: 'text', text: raw }
                : null
        case 'number':
            return typeof raw === 'number' && Number.isFinite(raw)
                ? { kind: 'number', number: raw }
                : null
    }
}

/** The JSON the run endpoint reads for a value (params.go parseParam). */
export function valueToJson(def: ParamDef, value: ParamValue): unknown {
    switch (value.kind) {
        case 'preset':
            return { preset: value.preset }
        case 'day':
            return value.day
        case 'range':
            return { from: value.from, to: value.to }
        case 'choice':
            return def.multiple ? value.values : value.values[0]
        case 'text':
            return value.text
        case 'number':
            return value.number
    }
}

export function defaultValueOf(def: ParamDef): ParamValue | null {
    if (def.default === undefined || def.default === null) return null
    return valueFromJson(def, def.default)
}

/** A param with no default is a filter the reader adds; unset, it filters nothing. */
export function isOptional(def: ParamDef): boolean {
    return defaultValueOf(def) === null
}

export function effectiveValue(def: ParamDef, values: ParamValues): ParamValue | null {
    return values[def.id] ?? defaultValueOf(def)
}

/** Only the values the reader set: an absent param takes its default on the server. */
export function postedParams(
    defs: readonly ParamDef[],
    values: ParamValues
): Record<string, unknown> {
    const out: Record<string, unknown> = {}
    for (const def of defs) {
        const value = values[def.id]
        if (value) out[def.id] = valueToJson(def, value)
    }
    return out
}

export function firstDateParam(defs: readonly ParamDef[]): ParamDef | undefined {
    return defs.find(isDateParam)
}

export type ControlItem =
    | { kind: 'param'; def: ParamDef; value: ParamValue | null; isRemovable: boolean }
    | { kind: 'add-filter'; defs: ParamDef[] }

// The control row: the first date param first, then the other params in spec order.
// A param with a default is always a chip; an optional one is a chip only once set,
// and is offered under Add filter until then.
export function controlItems(defs: readonly ParamDef[], values: ParamValues): ControlItem[] {
    const period = firstDateParam(defs)
    const ordered = period ? [period, ...defs.filter(def => def !== period)] : [...defs]
    const items: ControlItem[] = []
    for (const def of ordered) {
        const optional = isOptional(def)
        const set = values[def.id]
        if (!optional)
            items.push({
                kind: 'param',
                def,
                value: effectiveValue(def, values),
                isRemovable: false,
            })
        else if (set) items.push({ kind: 'param', def, value: set, isRemovable: true })
    }
    const addable = ordered.filter(def => isOptional(def) && !values[def.id])
    if (addable.length > 0) items.push({ kind: 'add-filter', defs: addable })
    return items
}

function resolvedDates(resolved: ResolvedParam | undefined): string | null {
    if (resolved?.from && resolved.to) return rangeChipText(resolved.from, resolved.to)
    if (typeof resolved?.value === 'string' && isIsoDay(resolved.value))
        return shortDay(resolved.value)
    return null
}

function choiceLabels(def: ParamDef, values: readonly string[]): string {
    const labels = values.map(
        value => def.options?.find(option => option.value === value)?.label ?? value
    )
    return labels.length <= 2 ? labels.join(', ') : `${labels.length} selected`
}

export function valueText(
    def: ParamDef,
    value: ParamValue | null,
    resolved: ResolvedParam | undefined
): string {
    if (!value) return 'Any'
    switch (value.kind) {
        case 'preset': {
            const dates = resolvedDates(resolved)
            return dates ? `${PRESET_LABELS[value.preset]} · ${dates}` : PRESET_LABELS[value.preset]
        }
        case 'day':
            return shortDay(value.day)
        case 'range':
            return rangeChipText(value.from, value.to)
        case 'choice':
            return choiceLabels(def, value.values)
        case 'text':
            return `“${value.text}”`
        case 'number':
            return String(value.number)
    }
}

export function chipText(
    def: ParamDef,
    value: ParamValue | null,
    resolved: ResolvedParam | undefined
): string {
    const text = valueText(def, value, resolved)
    return def.label ? `${def.label}: ${text}` : text
}

/** The period in words for the document header: the days the server ran. */
export function periodWords(
    def: ParamDef | undefined,
    resolved: ResolvedParam | undefined
): string | null {
    if (!def || !resolved || resolved.unset) return null
    if (def.type === 'dateRange' && resolved.from && resolved.to)
        return rangeWords(resolved.from, resolved.to)
    if (typeof resolved.value !== 'string' || !isIsoDay(resolved.value)) return null
    return def.type === 'asOf' ? `As of ${dayWords(resolved.value)}` : dayWords(resolved.value)
}

export function emptyMessage(
    def: ParamDef | undefined,
    resolved: ResolvedParam | undefined
): string {
    const words = periodWords(def, resolved)
    if (!def || !words) return 'No rows'
    if (def.type === 'dateRange') return `No rows for ${words}`
    if (def.type === 'asOf') return `No rows as of ${words.replace(/^As of /, '')}`
    return `No rows on ${words}`
}

export type EmptyAction =
    | { kind: 'clear-filters'; label: string }
    | { kind: 'preset'; paramId: string; preset: Preset; label: string }

export function emptyAction(defs: readonly ParamDef[], values: ParamValues): EmptyAction | null {
    if (defs.some(def => isOptional(def) && values[def.id]))
        return { kind: 'clear-filters', label: 'Clear filters' }
    const period = firstDateParam(defs)
    if (!period) return null
    const target =
        period.type === 'dateRange'
            ? 'thisFiscalYear'
            : period.type === 'asOf'
              ? 'endOfLastFiscalYear'
              : null
    if (!target) return null
    const current = effectiveValue(period, values)
    if (current?.kind === 'preset' && current.preset === target) return null
    const label =
        target === 'thisFiscalYear' ? 'Show this fiscal year' : 'Show end of last fiscal year'
    return { kind: 'preset', paramId: period.id, preset: target, label }
}

/** The values with every optional param removed: Clear filters. */
export function withoutOptional(
    defs: readonly ParamDef[],
    values: ParamValues
): Record<string, ParamValue> {
    const out: Record<string, ParamValue> = {}
    for (const def of defs) {
        const value = values[def.id]
        if (value && !isOptional(def)) out[def.id] = value
    }
    return out
}
