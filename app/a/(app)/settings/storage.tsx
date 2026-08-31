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
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Storage usage + per-user limit for this deployment. Org branding (name /
// slug / logo) is owned by the deployment (the hosting router) and is not
// editable in-app, so storage is what this screen manages — the route, title,
// and nav label all say so rather than promising organization management.

const storageLimitSchema = z.object({
    limitGb: z.number().min(0, 'Must be 0 or greater'),
})

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

function StorageSection() {
    const queryClient = useQueryClient()
    const [settingsCollection] = useStore('settings')
    const [showBreakdown, setShowBreakdown] = useState(false)

    const dangerColor = useThemeColor('danger')
    const warningColor = useThemeColor('warning')
    const successColor = useThemeColor('success')

    const { data: storageInfo, isLoading } = useQuery({
        queryKey: ['storage-usage'],
        queryFn: () =>
            pb.send('/api/drive/storage-usage', {
                query: { breakdown: 'users' },
            }),
    })

    const { data: settings } = useOrgLiveQuery(query =>
        query
            .from({ settings: settingsCollection })
            .where(({ settings }) =>
                and(eq(settings.app, 'core'), eq(settings.key, 'storage_limit_bytes'))
            )
    )

    const existingSetting = settings?.[0]

    const currentLimitGb = storageInfo?.has_limit
        ? storageInfo.limit_bytes / (1024 * 1024 * 1024)
        : 0

    const {
        control: limitControl,
        handleSubmit: handleLimitSubmit,
        setError: setLimitError,
        getValues: getLimitValues,
        formState: { errors: limitErrors, isSubmitted: isLimitSubmitted, isDirty: isLimitDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(storageLimitSchema),
        values: { limitGb: currentLimitGb },
    })

    const saveLimit = useMutation({
        mutationFn: mutation(function* (data: z.infer<typeof storageLimitSchema>) {
            const valueBytes = Math.round(data.limitGb * 1024 * 1024 * 1024)
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
            setError: setLimitError,
            getValues: getLimitValues,
        }),
    })

    const onSaveLimit = handleLimitSubmit(data => saveLimit.mutate(data))
    const canSaveLimit = !saveLimit.isPending && isLimitDirty

    if (isLoading) {
        return (
            <View className="gap-3">
                <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                    Loading...
                </Text>
            </View>
        )
    }

    const userUsed = storageInfo?.user_used_bytes ?? 0
    const limitBytes = storageInfo?.limit_bytes ?? 0
    const hasLimit = storageInfo?.has_limit ?? false
    const usagePercent =
        hasLimit && limitBytes > 0 ? Math.min((userUsed / limitBytes) * 100, 100) : 0
    const orgDriveBytes = storageInfo?.org_drive_bytes ?? 0
    const orgMailBytes = storageInfo?.org_mail_bytes ?? 0
    const users = storageInfo?.users as
        | { user_name: string; user_email: string; drive_used: number }[]
        | undefined

    const barColor =
        usagePercent > 90 ? dangerColor : usagePercent > 70 ? warningColor : successColor

    return (
        <View className="gap-4">
            <View className="gap-2">
                <Text className="text-primary" style={{ fontSize: 13 }}>
                    Your Usage
                </Text>
                <View className="flex-row justify-between items-center">
                    <Text className="text-foreground" style={{ fontSize: 15 }}>
                        {formatStorageBytes(userUsed)}
                        {hasLimit ? ` of ${formatStorageBytes(limitBytes)}` : ''}
                    </Text>
                    {hasLimit && (
                        <Text
                            className={usagePercent > 90 ? 'text-danger' : 'text-muted-foreground'}
                            style={{ fontSize: 13 }}
                        >
                            {usagePercent.toFixed(1)}%
                        </Text>
                    )}
                </View>
                {hasLimit && (
                    <View className="h-2 rounded overflow-hidden bg-surface-secondary">
                        <View
                            className="h-full rounded"
                            style={{
                                width: `${usagePercent}%`,
                                backgroundColor: barColor,
                            }}
                        />
                    </View>
                )}
            </View>

            <View className="gap-2">
                <Text className="text-primary" style={{ fontSize: 13 }}>
                    Organization Total
                </Text>
                <View className="flex-row gap-4">
                    <View>
                        <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                            Drive
                        </Text>
                        <Text className="text-foreground" style={{ fontSize: 15 }}>
                            {formatStorageBytes(orgDriveBytes)}
                        </Text>
                    </View>
                    <View>
                        <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                            Mail
                        </Text>
                        <Text className="text-foreground" style={{ fontSize: 15 }}>
                            {formatStorageBytes(orgMailBytes)}
                        </Text>
                    </View>
                </View>
            </View>

            <Divider />

            <View className="gap-3">
                <Text className="text-foreground" style={{ fontSize: 15, fontWeight: '600' }}>
                    Per-User Storage Limit
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                    Set to 0 for unlimited storage. Applies to drive uploads only.
                </Text>

                <FormErrorSummary errors={limitErrors} isEnabled={isLimitSubmitted} />

                <View className="flex-row gap-3 items-end">
                    <View className="flex-1">
                        <NumberInput control={limitControl} name="limitGb" label="Limit (GB)" />
                    </View>
                    <Pressable
                        onPress={onSaveLimit}
                        disabled={!canSaveLimit}
                        className={`px-4 py-2 rounded-lg self-start bg-primary ${canSaveLimit ? 'opacity-100' : 'opacity-50'}`}
                    >
                        <Text className="text-primary-foreground" style={{ fontWeight: '600' }}>
                            {saveLimit.isPending ? 'Saving...' : 'Save Limit'}
                        </Text>
                    </Pressable>
                </View>
            </View>

            {users && users.length > 0 && (
                <>
                    <Divider />
                    <View className="gap-3">
                        <Pressable onPress={() => setShowBreakdown(v => !v)}>
                            <Text
                                className="text-foreground"
                                style={{ fontSize: 15, fontWeight: '600' }}
                            >
                                Per-User Breakdown {showBreakdown ? '▾' : '▸'}
                            </Text>
                        </Pressable>
                        <UserBreakdownTable
                            users={users}
                            limitBytes={limitBytes}
                            isVisible={showBreakdown}
                        />
                    </View>
                </>
            )}
        </View>
    )
}

function UserBreakdownTable({
    users,
    limitBytes,
    isVisible,
}: {
    users: { user_name: string; user_email: string; drive_used: number }[]
    limitBytes: number
    isVisible: boolean
}) {
    if (!isVisible) return null

    return (
        <View className="gap-2">
            {users.map(user => {
                const percent =
                    limitBytes > 0 ? Math.min((user.drive_used / limitBytes) * 100, 100) : 0
                return (
                    <View
                        key={user.user_email}
                        className="flex-row justify-between items-center py-1"
                    >
                        <View className="flex-1">
                            <Text className="text-foreground" style={{ fontSize: 13 }}>
                                {user.user_name || user.user_email}
                            </Text>
                            {user.user_name && (
                                <Text className="text-muted-foreground" style={{ fontSize: 12 }}>
                                    {user.user_email}
                                </Text>
                            )}
                        </View>
                        <Text
                            className={percent > 90 ? 'text-danger' : 'text-muted-foreground'}
                            style={{ fontSize: 13 }}
                        >
                            {formatStorageBytes(user.drive_used)}
                        </Text>
                    </View>
                )
            })}
        </View>
    )
}
