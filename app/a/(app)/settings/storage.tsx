import { and, eq } from '@tanstack/db'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { formatBytes } from '@tinycld/core/lib/format-utils'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { Divider } from '@tinycld/core/ui/divider'
import { FormErrorSummary, NumberInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { ArrowLeft } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Who is using the disk, and the per-user cap. This deployment's operator owns
// the hardware, so the only storage question the app can answer for them is
// which user is filling it — there is no plan to report against and no bytes
// total to bill for. Anything shaped like a ceiling sold to an organization
// belongs to whoever sells the hosting, not here.
//
// Usage comes from core's /api/storage-usage, which sums every collection the
// installed packages registered as a quota source. It names no package, so the
// numbers stay right as packages are added or removed.

const BYTES_PER_GB = 1024 * 1024 * 1024

const storageLimitSchema = z.object({
    limitGb: z.number().min(0, 'Must be 0 or greater'),
})

interface UserBytes {
    userId: string
    name: string
    email: string
    bytes: number
}

interface StorageUsage {
    users: UserBytes[]
    limitPerUser: number
}

function formatStorageBytes(bytes: number): string {
    return bytes === 0 ? '0 B' : formatBytes(bytes)
}

export default function StorageSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const { isAdmin } = useCurrentRole()
    const fgColor = useThemeColor('foreground')

    if (!isAdmin) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title="Storage" />
                <Text className="text-muted-foreground" style={{ fontSize: 16 }}>
                    Only admins can manage storage settings.
                </Text>
            </View>
        )
    }

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} className="bg-background">
            <DocumentTitle pkg="Settings" title="Storage" />
            <View className="flex-1 p-5 max-w-[600px]">
                <View className="flex-row gap-3 items-center mb-5">
                    <Pressable onPress={navigateBack}>
                        <ArrowLeft size={24} color={fgColor} />
                    </Pressable>
                    <Text className="text-foreground" style={{ fontSize: 22, fontWeight: 'bold' }}>
                        Storage
                    </Text>
                </View>

                <StorageSection />
            </View>
        </ScrollView>
    )
}

function usePerUserLimit() {
    const queryClient = useQueryClient()
    const [settingsCollection] = useStore('settings')

    const { data: usage, isLoading } = useQuery<StorageUsage>({
        queryKey: ['storage-usage'],
        queryFn: () => pb.send('/api/storage-usage', {}),
    })

    const { data: settings } = useOrgLiveQuery(query =>
        query
            .from({ settings: settingsCollection })
            .where(({ settings }) =>
                and(eq(settings.app, 'core'), eq(settings.key, 'storage_limit_bytes'))
            )
    )
    const existingSetting = settings?.[0]

    const form = useForm({
        mode: 'onChange',
        resolver: zodResolver(storageLimitSchema),
        values: { limitGb: (usage?.limitPerUser ?? 0) / BYTES_PER_GB },
    })

    const saveLimit = useMutation({
        mutationFn: mutation(function* (data: z.infer<typeof storageLimitSchema>) {
            const valueBytes = Math.round(data.limitGb * BYTES_PER_GB)
            if (existingSetting) {
                yield settingsCollection.update(existingSetting.id, draft => {
                    draft.value = valueBytes
                })
            } else {
                yield settingsCollection.insert({
                    id: newRecordId(),
                    app: 'core',
                    key: 'storage_limit_bytes',
                    value: valueBytes,
                })
            }
        }),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['storage-usage'] })
        },
        onError: handleMutationErrorsWithForm({
            setError: form.setError,
            getValues: form.getValues,
        }),
    })

    return { usage, isLoading, form, saveLimit }
}

function StorageSection() {
    const { usage, isLoading, form, saveLimit } = usePerUserLimit()

    if (isLoading) {
        return (
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Loading...
            </Text>
        )
    }

    const limitBytes = usage?.limitPerUser ?? 0
    const onSaveLimit = form.handleSubmit(data => saveLimit.mutate(data))
    const canSave = !saveLimit.isPending && form.formState.isDirty

    return (
        <View className="gap-4">
            <View className="gap-3">
                <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                    Per-User Storage Limit
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                    Set to 0 for unlimited storage. Applies to everything a user owns.
                </Text>
                <FormErrorSummary
                    errors={form.formState.errors}
                    isEnabled={form.formState.isSubmitted}
                />
                <View className="flex-row gap-3 items-end">
                    <View className="flex-1">
                        <NumberInput control={form.control} name="limitGb" label="Limit (GB)" />
                    </View>
                    <Pressable
                        onPress={onSaveLimit}
                        disabled={!canSave}
                        className={`px-4 py-2 rounded-lg self-start bg-primary ${canSave ? 'opacity-100' : 'opacity-50'}`}
                    >
                        <Text className="text-primary-foreground" style={{ fontWeight: '600' }}>
                            {saveLimit.isPending ? 'Saving...' : 'Save Limit'}
                        </Text>
                    </Pressable>
                </View>
            </View>

            <Divider />

            <UsageByUser users={usage?.users ?? []} limitBytes={limitBytes} />
        </View>
    )
}

// Largest consumer first, as the server sorts them.
function UsageByUser({ users, limitBytes }: { users: UserBytes[]; limitBytes: number }) {
    return (
        <View className="gap-3">
            <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                Usage by User
            </Text>
            <EmptyUsers isVisible={users.length === 0} />
            {users.map(user => (
                <UserUsageRow key={user.userId} user={user} limitBytes={limitBytes} />
            ))}
        </View>
    )
}

function EmptyUsers({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null

    return (
        <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
            No stored data yet.
        </Text>
    )
}

function UserUsageRow({ user, limitBytes }: { user: UserBytes; limitBytes: number }) {
    const isOverLimit = limitBytes > 0 && user.bytes > limitBytes

    return (
        <View className="flex-row justify-between items-center py-1">
            <View className="flex-1">
                <Text className="text-foreground" style={{ fontSize: 13 }}>
                    {user.name || user.email}
                </Text>
                <UserSubtitle name={user.name} email={user.email} />
            </View>
            <Text
                className={isOverLimit ? 'text-danger' : 'text-muted-foreground'}
                style={{ fontSize: 13 }}
            >
                {formatStorageBytes(user.bytes)}
            </Text>
        </View>
    )
}

function UserSubtitle({ name, email }: { name: string; email: string }) {
    if (!name) return null

    return (
        <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
            {email}
        </Text>
    )
}
