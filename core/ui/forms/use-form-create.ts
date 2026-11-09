import type { FormDefinition } from '@tinycld/core/lib/forms/definition'
import {
    applyDefaults,
    type EvalContext,
    evaluate,
    type FormValues,
} from '@tinycld/core/lib/forms/logic'
import { completeTableRows } from '@tinycld/core/lib/forms/table-rows'
import { useFormsOfHost } from '@tinycld/core/lib/forms/use-forms-of-host'
import {
    buildSchema,
    emptyTablesOf,
    type HostSchemas,
    prepareValues,
    type SplitValues,
    splitValues,
    type TableRow,
    tableColumnsOf,
} from '@tinycld/core/lib/forms/values'
import type { FormHost } from '@tinycld/core/lib/packages/derive-form-hosts'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import type { ZodTypeAny } from 'zod'
import { EMPTY_DEFINITION, EMPTY_STATE } from './empty-schema'
import { formErrorHandler } from './form-error-handler'
import { useSchemaRefResolver } from './use-schema-ref-resolver'

/** A new record's starting values: no rows in each table, then the defaults, then the caller's. */
function startingValues(
    definition: FormDefinition,
    host: FormHost,
    context: EvalContext,
    given: FormValues
): FormValues {
    return { ...emptyTablesOf(definition, host), ...applyDefaults(definition, context), ...given }
}

export function useFormCreate(
    host: FormHost,
    initialFormId: string | undefined,
    context: EvalContext,
    /** Values the caller already has (a title typed in a composer); they win over the defaults. */
    initialValues: FormValues = {},
    /** The host UI's component schemas (`FormHostUi.schemas`), used by every validation here. */
    schemas: HostSchemas = {},
    /** The host UI's whole-record schema (`FormHostUi.recordSchema`), run after the field schemas. */
    recordSchema?: ZodTypeAny
) {
    const { forms, defaultForm, isReady } = useFormsOfHost(host.key)
    // Which type the dialog shows: local UI state until the record exists.
    const [formId, setFormId] = useState(initialFormId ?? defaultForm?.id ?? '')
    const row = forms.find(f => f.id === (formId || defaultForm?.id))
    const definition: FormDefinition | undefined = row?.definition
    const initialSchema = definition
        ? buildSchema(definition, host, evaluate(definition, {}, context), schemas, recordSchema)
        : buildSchema(EMPTY_DEFINITION, host, EMPTY_STATE, schemas)
    const { schemaRef, resolver } = useSchemaRefResolver(initialSchema)
    // A ref, so an unstable object literal from the caller never re-seeds the form.
    const initial = useRef(initialValues)
    initial.current = initialValues
    const form = useForm<FormValues>({
        resolver,
        defaultValues: definition ? startingValues(definition, host, context, initial.current) : {},
        mode: 'onSubmit',
    })

    // The one effect here: the live query can resolve after first render, when
    // defaultValues were already taken, so seed once per form id as it arrives.
    const seededFormId = useRef<string | null>(null)
    const resolvedId = row?.id
    useEffect(() => {
        if (!definition || !resolvedId || seededFormId.current === resolvedId) return
        seededFormId.current = resolvedId
        form.reset(startingValues(definition, host, context, initial.current))
    }, [definition, resolvedId, form, context, host])

    const switchForm = (nextId: string) => {
        const next = forms.find(f => f.id === nextId)
        if (!next) return
        const current = form.getValues()
        const kept: FormValues = {}
        for (const f of next.definition.fields) {
            const value = current[f.key]
            if (value === undefined || value === '') continue
            const columns = tableColumnsOf(host, f)
            // The new form's columns may differ; a missing cell would take a neighbour's value on a move.
            kept[f.key] =
                columns && Array.isArray(value)
                    ? completeTableRows(columns, value as TableRow[])
                    : value
        }
        form.reset(startingValues(next.definition, host, context, kept))
        seededFormId.current = nextId
        setFormId(nextId)
    }

    const submit = async (): Promise<SplitValues | null> => {
        if (!definition) return null
        schemaRef.current = buildSchema(
            definition,
            host,
            evaluate(definition, prepareValues(definition, host, form.getValues()), context),
            schemas,
            recordSchema
        )
        let out: SplitValues | null = null
        await form.handleSubmit(values => {
            out = splitValues(host, definition, prepareValues(definition, host, values))
        })()
        return out
    }

    return {
        form,
        schemaRef,
        definition,
        formId: row?.id ?? '',
        switchForm,
        submit,
        onError: formErrorHandler(host, form, 'create record'),
        isReady,
    }
}
