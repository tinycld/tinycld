import { useQuery, useQueryClient } from '@tanstack/react-query'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useIsSettingManaged } from '@tinycld/core/lib/use-managed-settings'
import { useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useSystemSettings } from '../../setup/system-settings-store'
import { type PbsForm, parseStoredConfig, pbsSchema, toStoredConfig } from './repository-logic'

const REPOSITORY_PREFIX = 'backup.repository.'
const KIND_KEY = 'backup.repository.kind'
const CONFIG_KEY = 'backup.repository.config'
const SCHEDULE_KEY = 'backup.repository.schedule'
const ENABLED_KEY = 'backup.repository.enabled'

/**
 * Whether a repository is configured and whether it is managed elsewhere,
 * without building the form — `BackupsSection` needs only these two flags to
 * decide what to render.
 */
export function useRepositoryState() {
    const { byKey } = useSystemSettings('backup.repository')
    const isManaged = useIsSettingManaged(REPOSITORY_PREFIX)
    const isConfigured = byKey.get(KIND_KEY)?.value === 'pbs'
    return { isConfigured, isManaged }
}

export function useBackupRepository() {
    const queryClient = useQueryClient()
    const { byKey, upsert } = useSystemSettings('backup.repository')
    const isManaged = useIsSettingManaged(REPOSITORY_PREFIX)
    const stored = parseStoredConfig(
        byKey.get(CONFIG_KEY)?.value,
        byKey.get(SCHEDULE_KEY)?.value,
        byKey.get(ENABLED_KEY)?.value
    )
    const isConfigured = byKey.get(KIND_KEY)?.value === 'pbs'

    const form = useForm({
        mode: 'onChange',
        resolver: zodResolver(pbsSchema),
        // `values` re-syncs when the stored settings load or change.
        values: stored,
    })
    const { setError, getValues } = form

    const save = useMutation({
        mutationFn: async (data: PbsForm) => {
            await upsert.mutateAsync({ key: KIND_KEY, value: 'pbs', isSecret: false })
            await upsert.mutateAsync({
                key: CONFIG_KEY,
                value: toStoredConfig(data),
                isSecret: true,
            })
            await upsert.mutateAsync({ key: SCHEDULE_KEY, value: data.schedule, isSecret: false })
            await upsert.mutateAsync({
                key: ENABLED_KEY,
                value: String(data.enabled),
                isSecret: false,
            })
        },
        onError: handleMutationErrorsWithForm({
            setError,
            getValues,
            operation: 'backup.repository.save',
        }),
    })

    const test = useMutation({
        mutationFn: (data: PbsForm) =>
            pb.send<{ snapshots: number }>('/api/org-backups/repository/test', {
                method: 'POST',
                body: { kind: 'pbs', config: JSON.parse(toStoredConfig(data)) },
            }),
    })

    const generateKey = useMutation({
        mutationFn: () =>
            pb.send<{ key: string }>('/api/org-backups/repository/generate-key', {
                method: 'POST',
                body: { kind: 'pbs' },
            }),
        onSuccess: ({ key }) => form.setValue('key', key, { shouldDirty: true }),
    })

    const backupNow = useMutation({
        mutationFn: () =>
            pb.send<{ id: string }>('/api/org-backups', {
                method: 'POST',
                body: { repository: true },
            }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups'] }),
    })

    return { form, save, test, generateKey, backupNow, isConfigured, isManaged }
}

export type RepositorySnapshot = { ref: string; created: string; bytes: number }

export function useSnapshots(isEnabled: boolean) {
    return useQuery({
        queryKey: ['backup-snapshots'],
        queryFn: () =>
            pb.send<RepositorySnapshot[]>('/api/org-backups/snapshots', { method: 'GET' }),
        enabled: isEnabled,
    })
}

const snapshotRestoreSchema = z.object({
    snapshot: z.string().min(1, 'Choose a snapshot'),
    acknowledged: z.boolean().refine(value => value, 'Confirm that current data will be replaced'),
})

export function useSnapshotRestore() {
    const form = useForm({
        mode: 'onChange',
        resolver: zodResolver(snapshotRestoreSchema),
        defaultValues: { snapshot: '', acknowledged: false },
    })
    const { setError, getValues } = form
    const start = useMutation({
        mutationFn: (data: z.infer<typeof snapshotRestoreSchema>) =>
            pb.send<{ jobId: string }>('/api/org-backups/restore', {
                method: 'POST',
                body: { snapshot: data.snapshot },
            }),
        onError: handleMutationErrorsWithForm({
            setError,
            getValues,
            operation: 'backup.restore.snapshot',
        }),
    })
    return { form, start }
}
