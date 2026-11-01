import { appHref } from '@tinycld/core/lib/org-routes'
import { isIsoDay } from './dates'
import { DEFAULT_DISPLAY, type DisplayOptions } from './format'
import {
    defaultValueOf,
    isDateParam,
    type ParamValue,
    presetsFor,
    rangeValue,
    valueFromJson,
} from './params'
import type { ParamDef } from './spec'

// A run's params and Customize options live in the route's search params, so a
// copied link reopens the report as the reader left it. Each value is checked on
// its own: a bad one falls back to its default and no other. Defaults are left
// out, so an untouched report has a clean URL.

export const PARAM_KEY_PREFIX = 'p-'
export const EDIT_KEY = 'edit'
const FLAG_ON = '1'
const FLAG_OFF = '0'
const RANGE_SEPARATOR = '..'
const LIST_SEPARATOR = ','
const COLUMN_ID = /^[A-Za-z][A-Za-z0-9_.]*$/

export type SearchParams = Record<string, string | string[] | undefined>

export interface ViewOptions {
    params: Record<string, ParamValue>
    display: DisplayOptions
}

export function paramKey(id: string): string {
    return `${PARAM_KEY_PREFIX}${id}`
}

function first(value: string | string[] | undefined): string | undefined {
    return Array.isArray(value) ? value[0] : value
}

export function encodeParamValue(def: ParamDef, value: ParamValue): string {
    switch (value.kind) {
        case 'preset':
            return value.preset
        case 'day':
            return value.day
        case 'range':
            return `${value.from}${RANGE_SEPARATOR}${value.to}`
        case 'choice':
            // An option value may hold a comma, so a list is written as JSON.
            return def.multiple ? JSON.stringify(value.values) : (value.values[0] ?? '')
        case 'text':
            return value.text
        case 'number':
            return String(value.number)
    }
}

function jsonList(raw: string): unknown {
    try {
        return JSON.parse(raw)
    } catch {
        return null
    }
}

export function decodeParamValue(def: ParamDef, raw: string): ParamValue | null {
    if (isDateParam(def)) {
        const preset = presetsFor(def).find(p => p === raw)
        if (preset) return { kind: 'preset', preset }
        if (def.type === 'dateRange') {
            const [from, to, extra] = raw.split(RANGE_SEPARATOR)
            return from && to && extra === undefined ? rangeValue(from, to) : null
        }
        return isIsoDay(raw) ? { kind: 'day', day: raw } : null
    }
    if (def.type === 'choice') return valueFromJson(def, def.multiple ? jsonList(raw) : raw)
    if (def.type === 'number') return raw.trim() === '' ? null : valueFromJson(def, Number(raw))
    return valueFromJson(def, raw)
}

function parseFlag(value: string | undefined, fallback: boolean): boolean {
    if (value === FLAG_ON) return true
    if (value === FLAG_OFF) return false
    return fallback
}

function parseHidden(raw: string | undefined): string[] {
    if (!raw) return []
    return [...new Set(raw.split(LIST_SEPARATOR).filter(id => COLUMN_ID.test(id)))]
}

export function parseViewOptions(defs: readonly ParamDef[], search: SearchParams): ViewOptions {
    const params: Record<string, ParamValue> = {}
    for (const def of defs) {
        const raw = first(search[paramKey(def.id)])
        const value = raw === undefined ? null : decodeParamValue(def, raw)
        if (value) params[def.id] = value
    }
    return {
        params,
        display: {
            divideBy1000: parseFlag(first(search.div1000), DEFAULT_DISPLAY.divideBy1000),
            withoutCents: parseFlag(first(search.nocents), DEFAULT_DISPLAY.withoutCents),
            negative: first(search.negative) === 'parens' ? 'parens' : 'minus',
            negativeRed: parseFlag(first(search.red), DEFAULT_DISPLAY.negativeRed),
            hidden: parseHidden(first(search.hide)),
        },
    }
}

function flag(on: boolean, fallback: boolean): string | undefined {
    if (on === fallback) return undefined
    return on ? FLAG_ON : FLAG_OFF
}

// Every key is present; a default is undefined so router.setParams removes it.
export function serializeViewOptions(
    defs: readonly ParamDef[],
    options: ViewOptions
): Record<string, string | undefined> {
    const out: Record<string, string | undefined> = {}
    for (const def of defs) {
        const value = options.params[def.id]
        const fallback = defaultValueOf(def)
        const text = value ? encodeParamValue(def, value) : undefined
        const isDefault = fallback !== null && text === encodeParamValue(def, fallback)
        out[paramKey(def.id)] = isDefault ? undefined : text
    }
    const { display } = options
    out.div1000 = flag(display.divideBy1000, DEFAULT_DISPLAY.divideBy1000)
    out.nocents = flag(display.withoutCents, DEFAULT_DISPLAY.withoutCents)
    out.negative = display.negative === 'parens' ? 'parens' : undefined
    out.red = flag(display.negativeRed, DEFAULT_DISPLAY.negativeRed)
    out.hide = display.hidden.length > 0 ? display.hidden.join(LIST_SEPARATOR) : undefined
    return out
}

/** The full link to a report as the reader sees it; `origin` is the server's address. */
export function reportLink(
    origin: string,
    reportId: string,
    defs: readonly ParamDef[],
    options: ViewOptions
): string {
    const query = new URLSearchParams()
    for (const [key, value] of Object.entries(serializeViewOptions(defs, options))) {
        if (value !== undefined) query.set(key, value)
    }
    const text = query.toString()
    return `${origin}${appHref(`reports/${reportId}`)}${text ? `?${text}` : ''}`
}
