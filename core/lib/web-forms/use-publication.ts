import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { embedCode } from '@tinycld/core/lib/web-forms/embed-code'
import { browserCanSolvePow } from '@tinycld/core/lib/web-forms/pow-capability'
import {
    PUBLICATION_DEFAULTS,
    type PublicationValues,
    publicationSchema,
} from '@tinycld/core/lib/web-forms/publication-schema'
import type { FormPublications } from '@tinycld/core/types/pbSchema'
import { useForm, zodResolver } from '@tinycld/core/ui/form'
import * as Clipboard from 'expo-clipboard'
import { newRecordId } from 'pbtsdb/core'
import { useRef, useState } from 'react'
import { Platform } from 'react-native'

const COPIED_MS = 2000

/** The origin visitors reach this server at. */
function formsOrigin(): string {
    if (Platform.OS === 'web' && typeof window !== 'undefined') return window.location.origin
    return PB_SERVER_ADDR
}

type PublicationColumns = Pick<
    FormPublications,
    | 'enabled'
    | 'allowed_domains'
    | 'require_pow'
    | 'confirmation'
    | 'retention_days'
    | 'max_upload_mb'
>

function toValues(row: PublicationColumns | undefined): PublicationValues {
    if (!row) return PUBLICATION_DEFAULTS
    return {
        enabled: row.enabled,
        allowedDomains: row.allowed_domains,
        requirePow: row.require_pow,
        confirmation: row.confirmation,
        retentionDays: row.retention_days,
        maxUploadMb: row.max_upload_mb,
    }
}

function toColumns(v: PublicationValues): PublicationColumns {
    return {
        enabled: v.enabled,
        allowed_domains: v.allowedDomains.trim(),
        require_pow: v.requirePow,
        confirmation: v.confirmation,
        retention_days: v.retentionDays,
        max_upload_mb: v.maxUploadMb,
    }
}

// A short "Copied" label; local UI state no other component reads.
function useCopied() {
    const [isCopied, setIsCopied] = useState(false)
    const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
    return {
        isCopied,
        mark: () => {
            setIsCopied(true)
            clearTimeout(timer.current)
            timer.current = setTimeout(() => setIsCopied(false), COPIED_MS)
        },
    }
}

export function usePublication(formId: string, formName: string) {
    const [publications] = useStore('form_publications')
    const { data, isReady } = useLiveQuery({
        query: query => query.from({ p: publications }).where(({ p }) => eq(p.form, formId)),
    })
    const row = data?.[0]
    const form = useForm<PublicationValues>({
        resolver: zodResolver(publicationSchema),
        values: toValues(row),
    })
    const save = useMutation({
        mutationFn: function* (v: PublicationValues) {
            if (row) {
                yield publications.update(row.id, d => {
                    Object.assign(d, toColumns(v))
                })
                return
            }
            yield publications.insert({
                id: newRecordId(),
                form: formId,
                token: '',
                ...toColumns(v),
            })
        },
        onError: handleMutationErrorsWithForm({
            setError: form.setError,
            getValues: form.getValues,
        }),
    })
    const regenerate = useMutation({
        mutationFn: function* () {
            if (!row) return
            yield publications.update(row.id, d => {
                d.token = ''
            })
        },
    })
    const copied = useCopied()
    const token = row?.token ?? ''
    const embed = token ? embedCode({ origin: formsOrigin(), token, title: formName }) : null
    return {
        row,
        isReady: Boolean(isReady),
        // Saving before the row loads would create a second publication.
        canSave: Boolean(isReady),
        powAvailable: browserCanSolvePow(),
        form,
        save: () => {
            void form.handleSubmit(v => save.mutate(v))()
        },
        isSaving: save.isPending,
        regenerate: () => regenerate.mutate(),
        embed,
        isCopied: copied.isCopied,
        copy: async () => {
            if (!embed) return
            await Clipboard.setStringAsync(embed)
            copied.mark()
        },
    }
}
