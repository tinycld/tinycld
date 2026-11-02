import { useQuery } from '@tanstack/react-query'
import { errorToString, extractValidationErrors } from '@tinycld/core/lib/errors'
import type { FormValues } from '@tinycld/core/lib/forms/logic'
import type { FormHost } from '@tinycld/core/lib/forms/use-form-hosts'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { postToParent } from '@tinycld/core/lib/web-forms/parent-messages'
import { usePendingFiles } from '@tinycld/core/lib/web-forms/pending-files-store'
import { type PowChallenge, solveChallenge } from '@tinycld/core/lib/web-forms/pow-solver'
import type { PublicForm } from '@tinycld/core/lib/web-forms/use-public-form'
import { formErrorHandler } from '@tinycld/core/ui/forms/form-error-handler'
import { newRecordId } from 'pbtsdb/core'
import { useRef } from 'react'
import type { UseFormReturn } from 'react-hook-form'

// Past the server's minimum fill time, so clock skew never trips it.
const MARGIN_MS = 250
const wait = (ms: number) => new Promise<void>(resolve => setTimeout(resolve, Math.max(0, ms)))

// Started when the page loads, so the hashing runs while the visitor types.
function usePowSolution(token: string, enabled: boolean) {
    return useQuery({
        queryKey: ['public-form-pow', token],
        enabled,
        staleTime: Number.POSITIVE_INFINITY,
        queryFn: async () => {
            const c = await pb.send<PowChallenge>(`/api/forms/public/${token}/challenge`, {
                method: 'GET',
            })
            return solveChallenge(c)
        },
    })
}

function fileKeysOf(form: PublicForm) {
    return form.definition.fields.filter(f => f.kind === 'file').map(f => f.key)
}

function idsOf(raw: unknown): string[] {
    if (Array.isArray(raw)) return raw.map(String)
    return raw ? [String(raw)] : []
}

function buildBody(o: {
    values: FormValues
    fileKeys: string[]
    clientId: string
    stamp: string
    honeypot: string
    pow: string | undefined
}) {
    const files = usePendingFiles.getState().files
    const body = new FormData()
    const plain = Object.fromEntries(
        Object.entries(o.values).filter(([k]) => !o.fileKeys.includes(k))
    )
    body.append('values', JSON.stringify(plain))
    body.append('client_id', o.clientId)
    body.append('stamp', o.stamp)
    body.append('_hp', o.honeypot)
    if (o.pow) body.append('pow', o.pow)
    for (const key of o.fileKeys) {
        for (const id of idsOf(o.values[key])) {
            const picked = files[id]
            if (picked) body.append(`files.${key}`, picked.file, picked.name)
        }
    }
    return body
}

// Field errors land on their fields; anything else (a stale stamp, a failed
// proof of work, a rate limit, no network) is the form's root error, which
// the wizard's error summary shows.
function publicErrorHandler(host: FormHost, rhf: UseFormReturn<FormValues>) {
    const onFieldErrors = formErrorHandler(host, rhf, 'submit web form')
    const prefix = `${host.fieldsColumn}.`
    return (error: unknown) => {
        const found = extractValidationErrors(error)
        const fieldKeys = Object.keys(found ?? {}).filter(k => k.startsWith(prefix))
        if (fieldKeys.length > 0) {
            onFieldErrors(error)
            return
        }
        rhf.setError('root', { type: 'server', message: errorToString(error) })
    }
}

export function usePublicSubmit(o: {
    token: string
    form: PublicForm
    receivedAt: number
    host: FormHost
    rhf: UseFormReturn<FormValues>
}) {
    // One id for the page load: a retry after a lost response is a no-op on the server.
    const clientId = useRef(newRecordId()).current
    const honeypot = useRef('')
    const pow = usePowSolution(o.token, o.form.requirePow)
    const mutation = useMutation({
        mutationFn: async (values: FormValues) => {
            await wait(o.receivedAt + o.form.minDelayMs + MARGIN_MS - Date.now())
            const solution = o.form.requirePow
                ? (pow.data ?? (await pow.refetch()).data)
                : undefined
            const body = buildBody({
                values,
                fileKeys: fileKeysOf(o.form),
                clientId,
                stamp: o.form.stamp,
                honeypot: honeypot.current,
                pow: solution,
            })
            await pb.send(`/api/forms/public/${o.token}/submit`, { method: 'POST', body })
        },
        onSuccess: () => {
            usePendingFiles.getState().clear()
            postToParent({ type: 'tinycld:form-submitted', form: o.form.slug })
        },
        onError: publicErrorHandler(o.host, o.rhf),
    })
    return {
        submit: () => mutation.mutate(o.rhf.getValues()),
        isSubmitting: mutation.isPending,
        isDone: mutation.isSuccess,
        honeypot: {
            onChangeText: (v: string) => {
                honeypot.current = v
            },
        },
    }
}
