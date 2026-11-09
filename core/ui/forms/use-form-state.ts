import { applyComputed } from '@tinycld/core/lib/forms/compute'
import type { FormDefinition } from '@tinycld/core/lib/forms/definition'
import { type EvalContext, evaluate, type FormValues } from '@tinycld/core/lib/forms/logic'
import { buildSchema, type HostSchemas } from '@tinycld/core/lib/forms/values'
import type { FormHost } from '@tinycld/core/lib/packages/derive-form-hosts'
import { useEffect, useMemo, useRef } from 'react'
import { type Control, type UseFormSetValue, useWatch } from 'react-hook-form'
import type { ZodTypeAny } from 'zod'

/** A stable empty default, so the schema memo does not rebuild on every render. */
const NO_SCHEMAS: HostSchemas = {}

/**
 * The effective state for the current values, recomputed on every change.
 * `ctx` must be referentially stable (memoize it), or the state recomputes every render.
 *
 * set_value actions fire once, when their rule becomes active after mount; the ref holds
 * which rules were active on the last commit so a user's later edit is kept, and it is
 * seeded from the first evaluation so a rule already active on mount never overwrites
 * loaded values. Pass `setValue` only where the form is editable.
 */
export function useFormState(
    def: FormDefinition,
    host: FormHost,
    control: Control<FormValues>,
    ctx: EvalContext,
    setValue?: UseFormSetValue<FormValues>,
    /** The host UI's component schemas (`FormHostUi.schemas`); keep it referentially stable. */
    schemas: HostSchemas = NO_SCHEMAS,
    /** The host UI's whole-record schema (`FormHostUi.recordSchema`); keep it referentially stable. */
    recordSchema?: ZodTypeAny
) {
    const values = useWatch({ control }) as FormValues
    // Rules and the display read the computed values, so a rule on a total reacts as rows change.
    const computed = useMemo(() => applyComputed(def, host, values), [def, host, values])
    const state = useMemo(() => evaluate(def, computed, ctx), [def, computed, ctx])
    const schema = useMemo(
        () => buildSchema(def, host, state, schemas, recordSchema),
        [def, host, state, schemas, recordSchema]
    )
    const previouslyActive = useRef<Set<string> | null>(null)
    if (previouslyActive.current === null) previouslyActive.current = new Set(state.activeRules)
    const seen = previouslyActive.current
    const newlyActiveKey = state.activeRules.filter(k => !seen.has(k)).join(',')

    // The one justified effect: react-hook-form's setValue publishes synchronously, so
    // calling it during render would update other components while this one renders.
    useEffect(() => {
        if (!setValue || !newlyActiveKey) return
        const newly = newlyActiveKey.split(',')
        for (const r of def.rules) {
            if (!newly.includes(r.key)) continue
            for (const a of r.actions) {
                if (a.action === 'set_value' && a.field in state.setValues) {
                    setValue(a.field, state.setValues[a.field])
                }
            }
        }
    }, [newlyActiveKey, def.rules, state.setValues, setValue])

    // Declared after the apply effect so it runs second on every commit.
    useEffect(() => {
        previouslyActive.current = new Set(state.activeRules)
    }, [state.activeRules])

    return { state, schema, computed }
}

export type UseFormStateResult = ReturnType<typeof useFormState>
