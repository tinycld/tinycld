import type { FormDefinition } from '@tinycld/core/lib/forms/definition'
import type { FormHostDef } from '@tinycld/core/lib/forms/host'
import { type EvalContext, evaluate, type FormValues } from '@tinycld/core/lib/forms/logic'
import {
    completeTableRows,
    planTableWrites,
    type TableWritePlan,
} from '@tinycld/core/lib/forms/table-rows'
import { useFormsOfHost } from '@tinycld/core/lib/forms/use-forms-of-host'
import {
    buildSchema,
    type ChecklistItem,
    computedKeysOf,
    type HostSchemas,
    mergeValues,
    prepareValues,
    type SplitValues,
    splitValues,
    type TableRow,
    tableColumnsOf,
} from '@tinycld/core/lib/forms/values'
import type { FormHost } from '@tinycld/core/lib/packages/derive-form-hosts'
import { useForm } from 'react-hook-form'
import type { ZodTypeAny } from 'zod'
import { EMPTY_DEFINITION, EMPTY_STATE } from './empty-schema'
import { formErrorHandler } from './form-error-handler'
import { useSchemaRefResolver } from './use-schema-ref-resolver'

/**
 * Edit mode: values come from the record; each `patch()` returns the keys
 * that changed, split for the host's update mutation. When a custom key
 * changed, `fields` is the record's whole JSON column with the changes laid
 * over it, so the host can write it as is without dropping other keys.
 *
 * The record is a live row, so the `values` prop updates when another client
 * edits it, and react-hook-form resets to it while keeping what the user is
 * typing. `patch()` validates only the dirty keys and returns null when one is
 * invalid. It does not mark anything saved: the host writes the result, then
 * calls `markSaved(written)` with the result's `written` map in the mutation's
 * `onSuccess`. A failed write therefore leaves the keys dirty and a retry sends
 * them again. `written` holds only the dirty keys and the values that were
 * sent, so an edit made while the write was in flight stays dirty. For a host
 * table, `tables` holds the rows the host loaded, `patch()` returns the changed
 * rows in `tables` (computed cells filled) and the write plan in `tableWrites`,
 * and the host writes both before `markSaved`.
 */
export type PatchResult = SplitValues & {
    written: FormValues
    /** For each host table in this patch: the rows to insert, update and remove, against the `tables` passed in. */
    tableWrites: Record<string, TableWritePlan>
}

/** Plain objects with their keys sorted, so the comparison below ignores key order. */
function withSortedKeys(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(withSortedKeys)
    if (value === null || typeof value !== 'object') return value
    const proto = Object.getPrototypeOf(value)
    if (proto !== Object.prototype && proto !== null) return value
    const record = value as Record<string, unknown>
    return Object.fromEntries(
        Object.keys(record)
            .sort()
            .map(key => [key, withSortedKeys(record[key])])
    )
}

// Core builds a table row id first (`newTableRow`) and a host's live row id last
// (`tableRowsOf`), so the same row reaches markSaved with its keys in another order.
const sameValue = (a: unknown, b: unknown) =>
    JSON.stringify(withSortedKeys(a)) === JSON.stringify(withSortedKeys(b))

function tableWritesOf(
    def: FormDefinition,
    host: FormHostDef,
    before: Record<string, TableRow[]>,
    after: Record<string, TableRow[]>
): Record<string, TableWritePlan> {
    const out: Record<string, TableWritePlan> = {}
    for (const f of def.fields) {
        const rows = after[f.key]
        const columns = tableColumnsOf(host, f)
        if (!rows || !columns) continue
        out[f.key] = planTableWrites(completeTableRows(columns, before[f.key] ?? []), rows, columns)
    }
    return out
}

export function useFormEdit(
    host: FormHost,
    record: Record<string, unknown> & { id: string },
    context: EvalContext,
    rows: Record<string, ChecklistItem[]> = {},
    /**
     * The rows of each host table, from the host's own live query
     * (`tableRowsOf`). Pass a table only once its rows have loaded: while it is
     * absent the table shows "Loading rows…" and totals over it keep their
     * stored values.
     */
    tables: Record<string, TableRow[]> = {},
    /** The host UI's component schemas (`FormHostUi.schemas`), used by every validation here. */
    schemas: HostSchemas = {},
    /** The host UI's whole-record schema (`FormHostUi.recordSchema`), run after the field schemas. */
    recordSchema?: ZodTypeAny
) {
    const { forms, defaultForm, isReady } = useFormsOfHost(host.key)
    const named = (record[host.formColumn] as string) || ''
    const found = forms.find(f => f.id === named)
    const chosen = found ?? defaultForm
    const definition: FormDefinition | undefined = chosen?.definition
    // The form the record names is deleted or unreadable: the record edits on the
    // default, and the host can log the id it lost. Not while the forms still load.
    const fallback = named && isReady && !found ? named : undefined
    const values = definition ? mergeValues(host, definition, record, rows, tables) : {}
    const initialSchema = definition
        ? buildSchema(
              definition,
              host,
              evaluate(definition, values, context),
              schemas,
              recordSchema
          )
        : buildSchema(EMPTY_DEFINITION, host, EMPTY_STATE, schemas)
    const { schemaRef, resolver } = useSchemaRefResolver(initialSchema)
    const form = useForm<FormValues>({
        resolver,
        values,
        resetOptions: { keepDirtyValues: true },
        mode: 'onBlur',
    })

    const markSaved = (written: FormValues) => {
        for (const [key, value] of Object.entries(written)) {
            // The user kept typing during the write: this key is still dirty.
            if (!sameValue(form.getValues(key), value)) continue
            // resetField acts only on a registered field. A field on a page the user never
            // opened has no Controller yet, so it is registered here first.
            form.register(key)
            form.resetField(key, { defaultValue: form.getValues(key) })
        }
    }

    const patch = async (): Promise<PatchResult | null> => {
        if (!definition) return null
        const current = prepareValues(definition, host, form.getValues())
        schemaRef.current = buildSchema(
            definition,
            host,
            evaluate(definition, current, context),
            schemas,
            recordSchema
        )
        const dirtyKeys = definition.fields
            .map(f => f.key)
            .filter(key => form.getFieldState(key).isDirty)
        // Only the dirty keys are validated: a field an admin tightened later must not
        // block an unrelated edit.
        const ok = await form.trigger(dirtyKeys)
        if (!ok) return null
        const changed: FormValues = {}
        const written: FormValues = {}
        for (const key of dirtyKeys) {
            changed[key] = current[key]
            written[key] = form.getValues(key)
        }
        // A total moves when the rows under it do; it is never dirty itself. One that
        // reads rows not passed in kept its stored value, so it is not written.
        for (const key of computedKeysOf(definition, host))
            if (!sameValue(current[key], values[key])) changed[key] = current[key]
        const split = splitValues(host, definition, changed)
        const stored = (record[host.fieldsColumn] ?? {}) as Record<string, unknown>
        const hasCustom = Object.keys(split.fields).length > 0
        return {
            ...split,
            fields: hasCustom ? { ...stored, ...split.fields } : {},
            written,
            tableWrites: tableWritesOf(definition, host, tables, split.tables),
        }
    }

    return {
        form,
        schemaRef,
        definition,
        fallback,
        patch,
        markSaved,
        onError: formErrorHandler(host, form, 'update record'),
        isReady,
    }
}
