// tinycld/core/lib/forms/use-form-admin.ts
import { extractValidationErrors } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { newRecordId } from 'pbtsdb/core'
import { uniqueKey } from '../short-code'
import { keyFromLabel } from './builder/key'
import type { FormDefinition } from './definition'
import { defaultDefinitionFor } from './host'
import { useFormHost } from './use-form-hosts'
import { useFormsOfHost } from './use-forms-of-host'

export function slugFor(name: string, taken: Set<string>): string {
    return uniqueKey(keyFromLabel(name), taken)
}

/** Server validation errors keyed `definition.<path>` become `<path>`, which is the editor form's own path. */
export function mapDefinitionErrors(error: unknown): Record<string, string> {
    const out: Record<string, string> = {}
    for (const [key, message] of Object.entries(extractValidationErrors(error) ?? {})) {
        out[key.startsWith('definition.') ? key.slice('definition.'.length) : key] = message
    }
    return out
}

export function useFormAdmin(hostKey: string | undefined) {
    const host = useFormHost(hostKey)
    const [formsCollection] = useStore('forms')
    const { forms, defaultForm, isReady } = useFormsOfHost(hostKey)
    const slugs = () => new Set(forms.map(f => f.slug))
    const nextOrder = () => forms.reduce((max, f) => Math.max(max, f.order ?? 0), -1) + 1

    const create = useMutation<string, Error, string>({
        mutationFn: mutation(function* (name) {
            if (!host || !hostKey) throw new Error('no form host')
            const id = newRecordId()
            yield formsCollection.insert({
                id,
                host: hostKey,
                name: name.trim(),
                slug: slugFor(name, slugs()),
                definition: defaultDefinitionFor(host),
                is_default: false,
                archived: false,
                order: nextOrder(),
            })
            return id
        }),
    })
    const duplicate = useMutation<string, Error, string>({
        mutationFn: mutation(function* (sourceId) {
            const source = forms.find(f => f.id === sourceId)
            if (!source || !hostKey) throw new Error('no such form')
            const id = newRecordId()
            const name = `${source.name} copy`
            yield formsCollection.insert({
                id,
                host: hostKey,
                name,
                slug: slugFor(name, slugs()),
                definition: source.definition,
                is_default: false,
                archived: false,
                order: nextOrder(),
            })
            return id
        }),
    })
    const rename = useMutation<void, Error, { id: string; name: string }>({
        mutationFn: mutation(function* ({ id, name }) {
            yield formsCollection.update(id, d => {
                d.name = name.trim()
            })
        }),
    })
    const setDefault = useMutation<void, Error, string>({
        mutationFn: mutation(function* (id) {
            yield formsCollection.update(id, d => {
                d.is_default = true
            })
        }),
    })
    const archive = useMutation<void, Error, { id: string; archived: boolean }>({
        mutationFn: mutation(function* ({ id, archived }) {
            yield formsCollection.update(id, d => {
                d.archived = archived
            })
        }),
    })
    const reorder = useMutation<void, Error, string[]>({
        mutationFn: mutation(function* (ids) {
            for (const [index, id] of ids.entries())
                yield formsCollection.update(id, d => {
                    d.order = index
                })
        }),
    })
    const saveDefinition = useMutation<void, Error, { id: string; definition: FormDefinition }>({
        mutationFn: mutation(function* ({ id, definition }) {
            yield formsCollection.update(id, d => {
                d.definition = definition
            })
        }),
        // The editor owns the error path: a refused field shows on that field,
        // and an error no field explains is reported and shown there once.
        // The default toast and report would say it a second time.
        onError: () => {},
    })
    const remove = useMutation<void, Error, string>({
        mutationFn: mutation(function* (id) {
            yield formsCollection.delete(id)
        }),
    })

    return {
        host,
        forms,
        defaultForm,
        isReady,
        create,
        duplicate,
        rename,
        setDefault,
        archive,
        reorder,
        saveDefinition,
        remove,
    }
}
