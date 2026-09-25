import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useQueryClient } from '@tanstack/react-query'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { formatTimeAgo } from '@tinycld/core/lib/format-utils'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useState } from 'react'

const STALE_MS = 7 * 86400 * 1000

export function useBackupRows() {
    const [backupsCollection, usersCollection] = useStore('backups', 'users')
    return useLiveQuery(
        query =>
            query
                .from({ backup: backupsCollection })
                // A left join, so a row the server wrote with no initiator (a
                // scheduled run) still lists.
                .join({ initiator: usersCollection }, ({ backup, initiator }) =>
                    eq(backup.initiated_by, initiator.id)
                )
                .orderBy(({ backup }) => backup.started, 'desc')
                .select(({ backup, initiator }) => ({ ...backup, initiatorName: initiator?.name })),
        [backupsCollection, usersCollection]
    )
}

export type BackupRow = NonNullable<ReturnType<typeof useBackupRows>['data']>[number]

type LedgerEntry = Pick<BackupRow, 'status' | 'finished' | 'kind'>

/**
 * The headline line for the panel. Pure, so the ledger's own summary can be
 * asserted without rendering: a restore is not a backup, so it never counts as
 * one, and never having a backup is itself the stale case.
 */
export function lastBackedUp(rows: readonly LedgerEntry[] | undefined) {
    const last = rows?.find(row => row.status === 'succeeded' && row.kind !== 'restore')
    if (!last?.finished) return { label: 'Never backed up', isStale: true }
    const age = Date.now() - new Date(last.finished.replace(' ', 'T')).getTime()
    return { label: `Last backed up ${formatTimeAgo(last.finished)}`, isStale: age > STALE_MS }
}

const urlField = z.string().url('Enter a full https:// URL')
const passphraseField = z.string().min(12, 'At least 12 characters')

const backupSchema = z
    .object({ target: urlField, passphrase: passphraseField, confirm: z.string() })
    .refine(values => values.passphrase === values.confirm, {
        path: ['confirm'],
        message: 'Passphrases do not match',
    })

export function useBackupNow() {
    const queryClient = useQueryClient()
    const form = useForm({
        mode: 'onChange',
        resolver: zodResolver(backupSchema),
        defaultValues: { target: '', passphrase: '', confirm: '' },
    })
    const { setError, getValues, reset } = form
    const start = useMutation({
        mutationFn: (data: z.infer<typeof backupSchema>) =>
            pb.send<{ id: string }>('/api/org-backups', {
                method: 'POST',
                body: { target: data.target, passphrase: data.passphrase },
            }),
        onSuccess: () => {
            // Clear the passphrase as soon as the job is accepted — it has no
            // further use in the client and nothing should hold it.
            reset()
            queryClient.invalidateQueries({ queryKey: ['backups'] })
        },
        onError: handleMutationErrorsWithForm({ setError, getValues, operation: 'backup.create' }),
    })
    return { form, start }
}

const restoreSchema = z.object({
    source: urlField,
    passphrase: passphraseField,
    // A plain boolean with a refine, not z.literal(true): the toggle starts
    // off, and a literal type would force the default value to be cast.
    acknowledged: z.boolean().refine(value => value, 'Confirm that current data will be replaced'),
})

export function useRestore() {
    const form = useForm({
        mode: 'onChange',
        resolver: zodResolver(restoreSchema),
        defaultValues: { source: '', passphrase: '', acknowledged: false },
    })
    const { setError, getValues } = form
    const start = useMutation({
        mutationFn: (data: z.infer<typeof restoreSchema>) =>
            pb.send<{ jobId: string }>('/api/org-backups/restore', {
                method: 'POST',
                body: { source: data.source, passphrase: data.passphrase },
            }),
        onError: handleMutationErrorsWithForm({ setError, getValues, operation: 'backup.restore' }),
    })
    return { form, start }
}

const swapSchema = z.object({ source: urlField })

export function useSwapSource(jobId: string) {
    const queryClient = useQueryClient()
    const form = useForm({
        mode: 'onChange',
        resolver: zodResolver(swapSchema),
        defaultValues: { source: '' },
    })
    const { setError, getValues, reset } = form
    const swap = useMutation({
        mutationFn: (data: z.infer<typeof swapSchema>) =>
            pb.send(`/api/org-backups/restore/${jobId}`, {
                method: 'PATCH',
                body: { source: data.source },
            }),
        onSuccess: () => {
            reset()
            queryClient.invalidateQueries({ queryKey: ['backups'] })
        },
        onError: handleMutationErrorsWithForm({ setError, getValues, operation: 'backup.swap' }),
    })
    return { form, swap }
}

/** Read one flag off a row's metadata column, which the server writes as JSON. */
function metadataOf(row: Pick<BackupRow, 'metadata'>): Record<string, unknown> {
    const { metadata } = row
    if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) return {}
    return metadata as Record<string, unknown>
}

/**
 * A restore the server staged but could not restart for (a dev-mode server has
 * no supervisor to relaunch it) is still owed a restart. The row stays running,
 * so without this flag the panel says "in progress" about something that will
 * never move on its own.
 */
export function isAwaitingRestart(row: Pick<BackupRow, 'kind' | 'status' | 'metadata'>) {
    if (row.kind !== 'restore' || row.status !== 'running') return false
    return metadataOf(row).awaiting_restart === true
}

/**
 * The pre-restore identity is written into the row's metadata by the server, so
 * there is nothing to acknowledge server-side — the dismissal only hides the
 * reminder for this session.
 */
export function useDismissPreRestoreKey(row: Pick<BackupRow, 'metadata'>) {
    const [isDismissed, setIsDismissed] = useState(false)
    const identity = metadataOf(row).pre_restore_identity
    return {
        identity: typeof identity === 'string' && identity ? identity : undefined,
        isDismissed,
        dismiss: () => setIsDismissed(true),
    }
}
