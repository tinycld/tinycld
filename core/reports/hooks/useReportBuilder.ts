import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@tinycld/core/lib/auth'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'
import { useForm, useWatch, zodResolver } from '@tinycld/core/ui/form'
import { router } from 'expo-router'
import { newRecordId } from 'pbtsdb'
import { useMemo, useState } from 'react'
import type { Catalog } from '../lib/builder/columns'
import {
    type BuilderDraft,
    builderSchema,
    draftFromSpec,
    emptyDraft,
    specFromDraft,
} from '../lib/builder/draft'
import { problemsFromError } from '../lib/builder/errors'
import { forgetSavedRuns } from '../lib/run-client'
import { parseSpec, type ReportSpec } from '../lib/spec'
import type { CatalogSource } from '../lib/types'
import { EDIT_KEY } from '../lib/view-url'
import type { SavedReport } from './useReportRecord'

const PREVIEW_DELAY_MS = 400
const MAX_TITLE = 200

export function copyTitle(title: string): string {
    return `Copy of ${title}`.slice(0, MAX_TITLE)
}

export interface BuilderInput {
    mode: 'new' | 'edit'
    catalog: Catalog
    base: CatalogSource
    record: SavedReport | null
    spec: ReportSpec | null
}

type SaveInput = { draft: BuilderDraft; asCopy: boolean; title: string }
const LIST_CHANGE = { shouldDirty: true, shouldValidate: true } as const

// The builder's form and its save. The preview runs the draft as it would be
// saved, 400 ms after the last valid change; a title is not needed to preview.
// Only a draft the schema accepts reaches specFromDraft, which reads it as valid.
export function useReportBuilder(input: BuilderInput) {
    const { mode, catalog, base, record, spec } = input
    const { user } = useAuth()
    const orgHref = useOrgHref()
    const queryClient = useQueryClient()
    const [reportsCollection] = useStore('reports')
    const schema = useMemo(() => builderSchema(catalog), [catalog])
    const defaults =
        record && spec
            ? draftFromSpec(spec, { title: record.title, description: record.description }, catalog)
            : emptyDraft(base)
    const form = useForm<BuilderDraft>({
        resolver: zodResolver(schema),
        mode: 'onChange',
        defaultValues: defaults,
    })
    const values = useWatch({ control: form.control })
    const parsed = schema.safeParse({ ...values, title: 'Preview' })
    const specText = parsed.success ? JSON.stringify(specFromDraft(parsed.data, catalog)) : ''
    const debounced = useDebouncedValue(specText, PREVIEW_DELAY_MS)
    const previewSpec = useMemo(
        () => (debounced ? parseSpec(JSON.parse(debounced)) : null),
        [debounced]
    )
    // The Discard confirm of an edit: local UI state.
    const [isConfirmingDiscard, setConfirmingDiscard] = useState(false)

    const exit = () => {
        if (mode === 'edit') router.setParams({ [EDIT_KEY]: undefined })
        else if (router.canGoBack()) router.back()
        else router.replace(orgHref('reports'))
    }

    const save = useMutation({
        mutationFn: function* ({ draft, asCopy, title }: SaveInput) {
            const nextSpec = specFromDraft(draft, catalog)
            if (mode === 'edit' && record && !asCopy) {
                yield reportsCollection.update(record.id, row => {
                    row.title = draft.title
                    row.description = draft.description
                    row.spec = nextSpec
                })
                return record.id
            }
            const id = newRecordId()
            yield reportsCollection.insert({
                id,
                owner: user.id,
                title,
                description: draft.description,
                spec: nextSpec,
                published: false,
            })
            return id
        },
        onSuccess: (id, { asCopy }) => {
            form.reset(form.getValues())
            if (mode === 'edit' && !asCopy) {
                // The view comes back on the same run key; its cached run is the old spec's.
                forgetSavedRuns(queryClient, id)
                router.setParams({ [EDIT_KEY]: undefined })
            } else router.replace(orgHref('reports/[id]', { id }))
        },
        onError: (error: unknown) => {
            const problems = problemsFromError(error)
            for (const problem of problems) {
                if (problem.path === 'title' || problem.path === 'description') {
                    form.setError(problem.path, { type: 'server', message: problem.message })
                }
            }
            // Spec problems show in their sections (saveProblems); anything else is a toast.
            if (problems.length === 0) {
                handleMutationErrorsWithForm({
                    setError: form.setError,
                    getValues: form.getValues,
                    operation: 'reports.save',
                })(error)
            }
        },
    })

    return {
        mode,
        form,
        catalog,
        base,
        previewSpec,
        title: values.title ?? '',
        isDirty: form.formState.isDirty,
        isSaving: save.isPending,
        isSaved: save.isSuccess,
        saveProblems: save.error ? problemsFromError(save.error) : [],
        onSave: () => {
            void form.handleSubmit(draft =>
                save.mutate({ draft, asCopy: false, title: draft.title })
            )()
        },
        onSaveAs: (title: string) => {
            void form.handleSubmit(draft => save.mutate({ draft, asCopy: true, title }))()
        },
        // An edit closes by a param change, which no leave guard sees, so it asks here.
        // A new report closes by leaving its route, where the panel's leave guard asks.
        onClose: () => {
            if (mode === 'edit' && form.formState.isDirty) setConfirmingDiscard(true)
            else exit()
        },
        isConfirmingDiscard,
        onKeepEditing: () => setConfirmingDiscard(false),
        onDiscard: () => {
            setConfirmingDiscard(false)
            form.reset(defaults)
            exit()
        },
        // Each replaces a whole list as one change, so a move or a removal is one edit.
        setColumns: (items: BuilderDraft['columns']) =>
            form.setValue('columns', items, LIST_CHANGE),
        setGroups: (items: BuilderDraft['groups']) => form.setValue('groups', items, LIST_CHANGE),
        setSort: (items: BuilderDraft['sort']) => form.setValue('sort', items, LIST_CHANGE),
        setLayout: (items: BuilderDraft['layout']) => form.setValue('layout', items, LIST_CHANGE),
        // A filter change carries the params it asks for: one edit, validated once.
        setFilter: (filter: BuilderDraft['filter'], params: BuilderDraft['params']) => {
            form.setValue('params', params, { shouldDirty: true })
            form.setValue('filter', filter, LIST_CHANGE)
        },
        setParams: (params: BuilderDraft['params']) => form.setValue('params', params, LIST_CHANGE),
        setPivotPath: (path: BuilderDraft['pivot']['path']) =>
            form.setValue('pivot.path', path, LIST_CHANGE),
    }
}

export type ReportBuilder = ReturnType<typeof useReportBuilder>
