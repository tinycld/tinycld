import { z } from '@tinycld/core/ui/form'
import { isIsoDay } from './dates'
import { MAX_TEXT_PARAM, type ParamValue, presetsFor, rangeValue } from './params'
import type { ParamDef } from './spec'
import type { ResolvedParam } from './types'

// The forms of the param chips. A preset applies at once; Custom days are typed and
// apply once they make a valid period (the report shell's rules).

export const DATE_FORMAT_MESSAGE = 'Enter a date as YYYY-MM-DD.'
export const FROM_AFTER_TO_MESSAGE = 'From must be on or before To.'
export const CUSTOM = 'custom'

export type PeriodValues = { preset: string; from: string | null; to: string | null }

const isDay = (value: string | null): value is string => value !== null && isIsoDay(value)

export function periodSchema(def: ParamDef) {
    return z
        .object({ preset: z.string(), from: z.string().nullable(), to: z.string().nullable() })
        .superRefine(({ preset, from, to }, ctx) => {
            // A preset resolves its own days, so a stale typed date never blocks it.
            if (preset !== CUSTOM) return
            if (!isDay(to))
                ctx.addIssue({ code: 'custom', path: ['to'], message: DATE_FORMAT_MESSAGE })
            if (def.type !== 'dateRange') return
            if (!isDay(from))
                ctx.addIssue({ code: 'custom', path: ['from'], message: DATE_FORMAT_MESSAGE })
            else if (isDay(to) && from > to)
                ctx.addIssue({ code: 'custom', path: ['from'], message: FROM_AFTER_TO_MESSAGE })
        })
}

export function periodDefaults(
    value: ParamValue | null,
    resolved: ResolvedParam | undefined
): PeriodValues {
    if (value?.kind === 'preset') {
        const day = typeof resolved?.value === 'string' ? resolved.value : null
        return { preset: value.preset, from: resolved?.from ?? null, to: resolved?.to ?? day }
    }
    if (value?.kind === 'range') return { preset: CUSTOM, from: value.from, to: value.to }
    if (value?.kind === 'day') return { preset: CUSTOM, from: null, to: value.day }
    return { preset: CUSTOM, from: null, to: null }
}

export function periodValueOf(def: ParamDef, values: PeriodValues): ParamValue | null {
    if (values.preset !== CUSTOM) {
        const preset = presetsFor(def).find(p => p === values.preset)
        return preset ? { kind: 'preset', preset } : null
    }
    if (def.type === 'dateRange')
        return values.from && values.to ? rangeValue(values.from, values.to) : null
    return isDay(values.to) ? { kind: 'day', day: values.to } : null
}

export const textValueSchema = z.object({
    text: z.string().trim().min(1, 'Enter some text').max(MAX_TEXT_PARAM),
})
export const numberValueSchema = z.object({
    // The number field holds its text while a typed value is not a number yet.
    number: z.number({ error: 'Enter a number' }).refine(Number.isFinite, 'Enter a number'),
})
export const choicesSchema = z.object({
    values: z.array(z.string()).min(1, 'Choose at least one'),
})
