import { useQuery } from '@tanstack/react-query'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { definitionSchema } from '@tinycld/core/lib/forms/definition'
import { serverFetch } from '@tinycld/core/lib/server-fetch'
import { z } from 'zod'

const publicFormSchema = z.object({
    name: z.string(),
    confirmation: z.string(),
    slug: z.string(),
    definition: definitionSchema,
    requirePow: z.boolean(),
    maxUploadMb: z.number(),
    stamp: z.string(),
    minDelayMs: z.number(),
})

export type PublicForm = z.infer<typeof publicFormSchema>

class FormGoneError extends Error {}

export const publicFormKey = (token: string) => ['public-form', token]

export async function fetchPublicForm(token: string) {
    const res = await serverFetch(`${PB_SERVER_ADDR}/api/forms/public/${encodeURIComponent(token)}`)
    if (res.status === 404) throw new FormGoneError()
    if (!res.ok) throw new Error(`public form ${res.status}`)
    return { form: publicFormSchema.parse(await res.json()), receivedAt: Date.now() }
}

export function usePublicForm(token: string) {
    const q = useQuery({
        queryKey: publicFormKey(token),
        queryFn: () => fetchPublicForm(token),
        retry: (count, err) => !(err instanceof FormGoneError) && count < 2,
        staleTime: Number.POSITIVE_INFINITY,
    })
    if (q.isPending) return { status: 'loading' as const, receivedAt: 0 }
    if (q.error instanceof FormGoneError) return { status: 'gone' as const, receivedAt: 0 }
    if (q.isError) return { status: 'error' as const, receivedAt: 0 }
    return { status: 'ready' as const, form: q.data.form, receivedAt: q.data.receivedAt }
}
