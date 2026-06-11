import { useQuery } from '@tanstack/react-query'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { captureException } from '@tinycld/core/lib/errors'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonIcon, ButtonText } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Plus, ShieldCheck, Trash2, X } from 'lucide-react-native'
import type PocketBase from 'pocketbase'
import { useState } from 'react'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'
import { PageHeader, RowIcon, SectionLabel } from './console-ui'

interface SuperAdminRow {
    id: string
    userId: string
    name: string
    email: string
}

const grantSchema = z.object({
    email: z.email(),
})

async function adminFetch(pb: PocketBase, path: string, init?: RequestInit) {
    const response = await fetch(`${PB_SERVER_ADDR}/api/admin/super-admins${path}`, {
        ...init,
        headers: {
            'Content-Type': 'application/json',
            Authorization: pb.authStore.token,
            ...init?.headers,
        },
    })
    const data = await response.json().catch(() => ({}))
    if (!response.ok) {
        throw new Error(data.error ?? 'Request failed')
    }
    return data
}

export function SuperAdminsTab({ isVisible, pb }: { isVisible: boolean; pb: PocketBase }) {
    const [showGrantForm, setShowGrantForm] = useState(false)

    // The roster comes from a custom /api/admin endpoint (the collection's RLS
    // only exposes the caller's own row), so it's a useQuery read, not a live
    // collection query. enabled:isVisible skips the fetch while the tab is hidden.
    const {
        data: admins = [],
        isLoading,
        error,
        refetch,
    } = useQuery({
        queryKey: ['admin', 'super-admins'],
        queryFn: async (): Promise<SuperAdminRow[]> => {
            const data = await adminFetch(pb, '')
            return data.superAdmins ?? []
        },
        enabled: isVisible,
    })

    if (!isVisible) return null

    return (
        <View className="gap-6">
            <PageHeader
                title="Super Admins"
                subtitle="Users granted the cross-org admin console. A super admin can manage packages, organizations, versions, and grant other super admins."
                actions={
                    <Button
                        onPress={() => setShowGrantForm(v => !v)}
                        size="sm"
                        variant={showGrantForm ? 'outline' : 'default'}
                    >
                        <ButtonIcon as={showGrantForm ? X : Plus} />
                        <ButtonText>{showGrantForm ? 'Cancel' : 'Grant access'}</ButtonText>
                    </Button>
                }
            />

            <GrantSection
                isVisible={showGrantForm}
                pb={pb}
                onGranted={() => {
                    setShowGrantForm(false)
                    refetch()
                }}
            />

            <SuperAdminList
                admins={admins}
                isLoading={isLoading}
                isError={error !== null}
                pb={pb}
                onRevoked={refetch}
            />
        </View>
    )
}

function GrantSection({
    isVisible,
    pb,
    onGranted,
}: {
    isVisible: boolean
    pb: PocketBase
    onGranted: () => void
}) {
    const {
        control,
        handleSubmit,
        reset,
        setError,
        formState: { errors, isSubmitting, isSubmitted },
    } = useForm({
        resolver: zodResolver(grantSchema),
        defaultValues: { email: '' },
        mode: 'onChange',
    })

    const onSubmit = handleSubmit(async values => {
        try {
            await adminFetch(pb, '', {
                method: 'POST',
                body: JSON.stringify({ email: values.email }),
            })
            reset()
            onGranted()
        } catch (err) {
            setError('email', {
                message: err instanceof Error ? err.message : 'Failed to grant access',
            })
        }
    })

    if (!isVisible) return null

    return (
        <View className="gap-4 p-5 rounded-2xl bg-surface-secondary border border-border">
            <SectionLabel>Grant by email</SectionLabel>
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <TextInput
                control={control}
                name="email"
                label="User email"
                placeholder="person@example.com"
                autoCapitalize="none"
                keyboardType="email-address"
            />
            <View className="flex-row justify-end">
                <Button onPress={onSubmit} size="sm" isDisabled={isSubmitting}>
                    <ButtonIcon as={ShieldCheck} />
                    <ButtonText>Grant super admin</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function SuperAdminList({
    admins,
    isLoading,
    isError,
    pb,
    onRevoked,
}: {
    admins: SuperAdminRow[]
    isLoading: boolean
    isError: boolean
    pb: PocketBase
    onRevoked: () => void
}) {
    if (isLoading) {
        return (
            <View className="py-12 items-center">
                <ActivityIndicator />
            </View>
        )
    }

    if (isError) {
        return (
            <View className="py-12 items-center">
                <Text className="text-danger" style={{ fontSize: 14 }}>
                    Couldn't load super admins. Check your connection and try again.
                </Text>
            </View>
        )
    }

    if (admins.length === 0) {
        return (
            <View className="py-12 items-center">
                <Text className="text-muted-foreground" style={{ fontSize: 14 }}>
                    No super admins yet. Grant access by email above.
                </Text>
            </View>
        )
    }

    return (
        <View className="gap-2">
            {admins.map(admin => (
                <SuperAdminRowItem key={admin.id} admin={admin} pb={pb} onRevoked={onRevoked} />
            ))}
        </View>
    )
}

function SuperAdminRowItem({
    admin,
    pb,
    onRevoked,
}: {
    admin: SuperAdminRow
    pb: PocketBase
    onRevoked: () => void
}) {
    const [isRevoking, setIsRevoking] = useState(false)
    const mutedColor = useThemeColor('muted-foreground')

    const revoke = async () => {
        setIsRevoking(true)
        try {
            await adminFetch(pb, `/${admin.userId}`, { method: 'DELETE' })
            onRevoked()
        } catch (err) {
            captureException('superAdmins.revoke', err, { userId: admin.userId })
            setIsRevoking(false)
        }
    }

    return (
        <View className="flex-row items-center gap-3 p-3 rounded-xl bg-surface-secondary border border-border">
            <RowIcon Icon={ShieldCheck} accent />
            <View className="flex-1">
                <Text className="text-foreground" style={{ fontSize: 14, fontWeight: '600' }}>
                    {admin.name || admin.email || admin.userId}
                </Text>
                {admin.email ? (
                    <Text className="text-muted-foreground" style={{ fontSize: 12.5 }}>
                        {admin.email}
                    </Text>
                ) : null}
            </View>
            <Pressable
                onPress={revoke}
                disabled={isRevoking}
                accessibilityLabel="Revoke super admin"
                className="w-9 h-9 rounded-lg items-center justify-center"
            >
                {isRevoking ? (
                    <ActivityIndicator color={mutedColor} />
                ) : (
                    <Trash2 size={17} color={mutedColor} />
                )}
            </Pressable>
        </View>
    )
}
