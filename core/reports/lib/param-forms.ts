import { z } from '@tinycld/core/ui/form'
import { isIsoDay } from './dates'
import { centsFromText } from './money'
import {
    MAX_CHOICE_VALUES,
    MAX_OPTION_GROUPS,
    MAX_TEXT_PARAM,
    type ParamValue,
    presetsFor,
    rangeValue,
    utf8Length,
} from './params'
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

export const TEXT_TOO_LONG_MESSAGE = 'This text is too long.'

export const textValueSchema = z.object({
    // The engine counts UTF-8 bytes, so a letter outside ASCII counts more than one.
    text: z
        .string()
        .trim()
        .min(1, 'Enter some text')
        .refine(text => utf8Length(text) <= MAX_TEXT_PARAM, TEXT_TOO_LONG_MESSAGE),
})
export const numberValueSchema = z.object({
    // The number field holds its text while a typed value is not a number yet.
    number: z.number({ error: 'Enter a number' }).refine(Number.isFinite, 'Enter a number'),
})
export type NumberValue = z.infer<typeof numberValueSchema>
export const choicesSchema = z.object({
    values: z.array(z.string()).min(1, 'Choose at least one'),
})

/** The most characters an option search has (options.go maxOptionSearch, in runes). */
export const MAX_OPTION_SEARCH = 200
export const TOO_MANY_RECORDS_MESSAGE = `Pick at most ${MAX_CHOICE_VALUES} records, or pick a whole group`

// A choice from a source: the typed search, the picked records and the whole groups.
// The records keep the engine's 1000-value cap; a group is one reference the server
// expands under the reader's rules, so it has no such cap (F19).
export const sourceChoiceSchema = z.object({
    search: z
        .string()
        .refine(text => [...text].length <= MAX_OPTION_SEARCH, 'This search is too long.'),
    values: z.array(z.string()).max(MAX_CHOICE_VALUES, TOO_MANY_RECORDS_MESSAGE),
    groups: z.array(z.string()).max(MAX_OPTION_GROUPS, `Pick at most ${MAX_OPTION_GROUPS} groups`),
})
export type SourceChoiceValues = z.infer<typeof sourceChoiceSchema>

export const AMOUNT_MESSAGE = 'Type an amount, like 1500.00'
export const AMOUNT_TOO_LARGE_MESSAGE = 'This amount is too large.'
export const CURRENCY_MESSAGE =
    'Type a three-letter currency code, like USD, or leave it blank for any currency'

// "1,500.00" reads as "1500.00"; commas only where they group thousands.
const GROUPED_AMOUNT = /^-?\d{1,3}(,\d{3})+(\.\d{1,2})?$/

function amountCents(text: string): number | null {
    const trimmed = text.trim()
    return centsFromText(GROUPED_AMOUNT.test(trimmed) ? trimmed.replaceAll(',', '') : trimmed)
}

export const amountValueSchema = z.object({
    amount: z.string().superRefine((text, ctx) => {
        const cents = amountCents(text)
        if (cents === null) ctx.addIssue({ code: 'custom', message: AMOUNT_MESSAGE })
        // The engine holds whole cents up to 2^53-1 (params.go maxAmountCents).
        else if (!Number.isSafeInteger(cents)) {
            ctx.addIssue({ code: 'custom', message: AMOUNT_TOO_LARGE_MESSAGE })
        }
    }),
    currency: z
        .string()
        .trim()
        .regex(/^([A-Za-z]{3})?$/, CURRENCY_MESSAGE),
})
export type AmountValue = z.infer<typeof amountValueSchema>

/** The param value of a submitted amount form; the schema has checked the text. */
export function amountParamValue(form: AmountValue): ParamValue {
    return {
        kind: 'amount',
        cents: amountCents(form.amount) ?? 0,
        currency: form.currency.trim().toUpperCase(),
    }
}
