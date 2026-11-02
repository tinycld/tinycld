// The rule builder's form: react-hook-form holds the draft, ruleDraftSchemaFor
// validates it. The cards edit it through the callbacks below rather than
// binding inputs one by one, because most edits are structural — picking a
// trigger resets the conditions and actions built against the old one, and a
// condition edit is AST surgery (condition-helpers.ts).
//
// Errors only appear after the first Save (FormErrorSummary is gated on
// isSubmitted), so "Name is required" does not flash while the user types the
// first character. After that they re-validate on every edit.
import { useForm, zodResolver } from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import { useMemo, useRef } from 'react'
import { useFieldArray } from 'react-hook-form'
import type { CatalogResponse } from './api'
import type { RuleDraft } from './draft'
import { ruleDraftSchemaFor } from './rule-draft-schema'

type ActionParams = RuleDraft['actions'][number]['params']

export function useRuleDraft(
    initial: RuleDraft,
    catalog: CatalogResponse,
    extend?: (catalog: CatalogResponse, draft: RuleDraft) => CatalogResponse
) {
    // The resolver reads the newest schema: picking a form adds its fields,
    // and a condition on one must validate against them at once.
    const schemaRef = useRef(ruleDraftSchemaFor(catalog))
    const {
        control,
        watch,
        getValues,
        setValue,
        trigger: validate,
        handleSubmit,
        setFocus,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: (values, context, options) =>
            zodResolver(schemaRef.current)(values, context, options),
        defaultValues: initial,
    })
    const actionFields = useFieldArray({ control, name: 'actions' })
    const draft = watch()
    const extended = extend ? extend(catalog, draft) : catalog
    schemaRef.current = useMemo(() => ruleDraftSchemaFor(extended), [extended])

    // A cross-field edit (a new trigger invalidates conditions, actions and
    // the cron) can change errors anywhere in the draft, so re-run the whole
    // schema rather than only the path that was set.
    const revalidate = () => {
        if (isSubmitted) void validate()
    }

    const selectTrigger = (ref: string) => {
        // Re-picking the trigger already selected is a no-op, not a reset:
        // the menu shows the current choice, so clicking it is the natural way
        // to close the menu — and it used to silently discard every condition
        // and action the user had built.
        if (ref === getValues('trigger')) return
        // Switching triggers invalidates conditions (built against the old
        // trigger's field set) and actions (may target the old trigger's
        // collection) — both reset rather than silently carrying over.
        //
        // Resetting to zero groups is also what re-arms ConditionsCard's ready
        // first row against the NEW trigger's fields (see its SyntheticFirstGroup).
        // The literal deliberately skips ensureUids, which is safe only because
        // the array is empty — anything seeded here would need uids to key on.
        setValue('trigger', ref, { shouldDirty: true })
        setValue('conditions', { match: 'all', groups: [] }, { shouldDirty: true })
        actionFields.replace([])
        revalidate()
    }

    const setConditions = (conditions: RuleDraft['conditions']) => {
        setValue('conditions', conditions, { shouldDirty: true })
        revalidate()
    }

    const addAction = (ref: string) => {
        actionFields.append({ uid: newRecordId(), ref, params: {} })
        revalidate()
    }

    // Out-of-range is a no-op rather than a clamp: the up/down buttons are
    // disabled at the ends, so this only guards against a stale index.
    const moveAction = (from: number, to: number) => {
        if (to < 0 || to >= actionFields.fields.length) return
        actionFields.move(from, to)
        revalidate()
    }

    const removeAction = (index: number) => {
        actionFields.remove(index)
        revalidate()
    }

    const setActionParams = (index: number, params: ActionParams) => {
        setValue(`actions.${index}.params`, params, { shouldDirty: true })
        revalidate()
    }

    // Keyed on useFieldArray's own ids, which move with their action on a
    // reorder — an index key would reconcile an open param Menu or a focused
    // input onto whichever action slides into that slot. Values come from the
    // watched draft: `fields` is a snapshot taken at the last array operation,
    // so it misses param edits made since.
    const actionRows = actionFields.fields.map((field, index) => ({
        key: field.id,
        action: draft.actions[index] ?? field,
    }))

    return {
        control,
        draft,
        errors,
        isSubmitted,
        setFocus,
        handleSubmit,
        actionRows,
        selectTrigger,
        setConditions,
        addAction,
        moveAction,
        removeAction,
        setActionParams,
        catalog: extended,
    }
}
