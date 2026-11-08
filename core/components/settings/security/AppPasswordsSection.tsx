import { eq } from '@tanstack/db'
import { formatLastUsed } from '@tinycld/core/components/settings/ConnectedAppsSection'
import { formatSince } from '@tinycld/core/components/settings/security/security-format'
import { errorToString } from '@tinycld/core/lib/errors'
import { createAppPassword } from '@tinycld/core/lib/mfa'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { acceptServerRow } from '@tinycld/core/lib/server-rows'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { PromptDialog } from '@tinycld/core/ui/PromptDialog'
import * as Clipboard from 'expo-clipboard'
import { Trash2 } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

type Created = { name: string; password: string }

function useAppPasswordRows() {
    const [appPasswords] = useStore('app_passwords')
    const { data } = useMyLiveQuery((query, { userId }) =>
        query
            .from({ a: appPasswords })
            .where(({ a }) => eq(a.user, userId))
            .orderBy(({ a }) => a.created)
            .select(({ a }) => ({
                id: a.id,
                name: a.name,
                created: a.created,
                lastUsed: a.last_used,
            }))
    )
    return data ?? []
}

type AppPasswordRow = ReturnType<typeof useAppPasswordRows>[number]

function useAppPasswords() {
    const [appPasswords] = useStore('app_passwords')
    const rows = useAppPasswordRows()
    const [isNaming, setIsNaming] = useState(false)
    const [revoking, setRevoking] = useState<AppPasswordRow | null>(null)

    const create = useMutation({
        mutationFn: async (name: string): Promise<Created> => {
            const result = await createAppPassword(name)
            await acceptServerRow(appPasswords, result.record, 'appPasswords.create')
            return { name: result.record.name, password: result.password }
        },
        onSuccess: () => setIsNaming(false),
        onError: () => {},
    })

    const revoke = useMutation({
        mutationFn: mutation(function* (id: string) {
            yield appPasswords.delete(id)
        }),
        onSuccess: () => {
            setRevoking(null)
            notify.emit({ event: 'account.security_changed', title: 'App password revoked' })
        },
    })

    return {
        rows,
        isNaming,
        startCreate: () => setIsNaming(true),
        cancelCreate: () => {
            create.reset()
            setIsNaming(false)
        },
        submitCreate: (name: string) => create.mutate(name),
        isCreating: create.isPending,
        createError: create.error ? errorToString(create.error) : null,
        created: create.data ?? null,
        dismissCreated: () => create.reset(),
        revoking,
        startRevoke: setRevoking,
        cancelRevoke: () => setRevoking(null),
        confirmRevoke: () => {
            if (revoking) revoke.mutate(revoking.id)
        },
        isRevoking: revoke.isPending,
    }
}

export function AppPasswordsSection() {
    const ap = useAppPasswords()

    return (
        <View
            className="rounded-xl border border-border bg-surface-secondary p-4 gap-3"
            testID="app-passwords-section"
        >
            <Text className="text-base font-semibold text-foreground">App passwords</Text>
            <Text className="text-[13px] text-muted-foreground">
                Mail, calendar, contacts and file apps (such as Apple Mail, Thunderbird or DAVx⁵)
                cannot ask for a second step. Give each one its own app password. When two-step
                verification is on, your account password no longer works in those apps.
            </Text>

            <CreatedPanel created={ap.created} onDone={ap.dismissCreated} />
            <AppPasswordList rows={ap.rows} onRevoke={ap.startRevoke} />
            <Button
                testID="app-password-create"
                variant="outline"
                className="self-start"
                onPress={ap.startCreate}
            >
                <ButtonText>Create an app password</ButtonText>
            </Button>

            <PromptDialog
                isOpen={ap.isNaming}
                onClose={ap.cancelCreate}
                onSubmit={ap.submitCreate}
                title="Create an app password"
                description="Name it after the app or device that will use it."
                placeholder="Thunderbird on my laptop"
                confirmLabel="Create"
                confirmTestID="app-password-name-submit"
                maxLength={100}
                required
                isSubmitting={ap.isCreating}
                errorMessage={ap.createError}
            />
            <ConfirmDialog
                isOpen={!!ap.revoking}
                onClose={ap.cancelRevoke}
                onConfirm={ap.confirmRevoke}
                title="Revoke app password?"
                message={`"${ap.revoking?.name ?? ''}" will stop signing in at once.`}
                confirmLabel="Revoke"
                isDestructive
                isSubmitting={ap.isRevoking}
            />
        </View>
    )
}

function CreatedPanel({ created, onDone }: { created: Created | null; onDone: () => void }) {
    if (!created) return null
    const copy = async () => {
        await Clipboard.setStringAsync(created.password)
        notify.emit({ event: 'account.security_copied', title: 'App password copied' })
    }
    return (
        <View
            className="gap-2 rounded-lg border border-border bg-surface p-3"
            testID="app-password-created"
        >
            <Text className="text-sm text-foreground">
                Your app password for “{created.name}”. Type it into the app in place of your
                password. You will not see it again.
            </Text>
            <Text
                selectable
                testID="app-password-value"
                className="font-mono text-lg text-foreground text-center"
            >
                {created.password}
            </Text>
            <View className="flex-row gap-3">
                <Button variant="outline" onPress={copy}>
                    <ButtonText>Copy</ButtonText>
                </Button>
                <ServerActionButton testID="app-password-done" onPress={onDone}>
                    <ButtonText>Done</ButtonText>
                </ServerActionButton>
            </View>
        </View>
    )
}

function AppPasswordList({
    rows,
    onRevoke,
}: {
    rows: AppPasswordRow[]
    onRevoke: (row: AppPasswordRow) => void
}) {
    if (rows.length === 0) return null
    const items = rows.map(row => (
        <AppPasswordItem key={row.id} row={row} onRevoke={() => onRevoke(row)} />
    ))
    return <View className="gap-1.5">{items}</View>
}

function AppPasswordItem({ row, onRevoke }: { row: AppPasswordRow; onRevoke: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <View className="flex-row items-center gap-3 py-1.5" testID="app-password-row">
            <View className="flex-1">
                <Text className="text-sm font-semibold text-foreground">{row.name}</Text>
                <Text className="text-xs text-muted-foreground">
                    Created {formatSince(row.created)} · {formatLastUsed(row.lastUsed ?? '')}
                </Text>
            </View>
            <Pressable accessibilityLabel={`Revoke ${row.name}`} onPress={onRevoke} hitSlop={8}>
                <Trash2 size={16} color={mutedColor} />
            </Pressable>
        </View>
    )
}
