import {
    type OAuthClient,
    useOAuthClients,
    useSetClientDisabled,
} from '@tinycld/core/lib/use-oauth-clients'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { Switch } from '@tinycld/core/ui/switch'
import { useState } from 'react'
import { ActivityIndicator, Text, View } from 'react-native'

// describeAccess is the one-line summary under each client's name: what it can
// reach, and how much is currently connected through it. Phrased around the
// consequence of switching it off, which is the decision the admin is actually
// making.
export function describeAccess(client: OAuthClient): string {
    const scopeCount = client.scopes.trim() ? client.scopes.trim().split(/\s+/).length : 0
    const scopes = `${scopeCount} scope${scopeCount === 1 ? '' : 's'}`

    if (client.active_grants === 0) return `${scopes} · nothing connected`
    const conn = `${client.active_grants} connection${client.active_grants === 1 ? '' : 's'}`
    return `${scopes} · ${conn}`
}

// planToggle decides what a flip of the switch should do. Extracted from the
// component because it is the asymmetry that matters — disabling confirms,
// re-enabling does not — and the unit tests cannot reach it through the DOM:
// react-native's Pressable is stubbed as a plain string tag in the test
// environment (tests/react-native-stub.cjs), so onPress never becomes a click
// handler and no fireEvent can drive it.
export function planToggle(
    client: OAuthClient,
    disabled: boolean
): { action: 'confirm' } | { action: 'mutate'; id: string; disabled: boolean } {
    // Disabling cuts off live integrations with no warning to their users, so
    // it goes through a confirmation. Re-enabling only restores access that
    // was already granted, and making it harder would just discourage undoing
    // a mistaken disable.
    if (disabled) return { action: 'confirm' }
    return { action: 'mutate', id: client.id, disabled: false }
}

// isVisible gates the whole section — including its fetch. The caller knows
// whether the viewer is an admin, and a non-admin must not even issue the
// request: it would 403 and surface as a load error on a screen they are not
// entitled to see.
export function OAuthClientsSection({ isVisible = true }: { isVisible?: boolean }) {
    const { data: clients, isLoading, isError } = useOAuthClients(isVisible)
    const setDisabled = useSetClientDisabled()
    // The client awaiting confirmation before being switched OFF.
    const [pendingDisable, setPendingDisable] = useState<OAuthClient | null>(null)

    const applyDisabled = (client: OAuthClient, disabled: boolean) => {
        const plan = planToggle(client, disabled)
        if (plan.action === 'confirm') {
            setPendingDisable(client)
            return
        }
        setDisabled.mutate({ id: plan.id, disabled: plan.disabled })
    }

    const confirmDisable = () => {
        if (!pendingDisable) return
        setDisabled.mutate({ id: pendingDisable.id, disabled: true })
        setPendingDisable(null)
    }

    if (!isVisible) return null

    // A DISABLED query reports isLoading forever (it has no data and will
    // never fetch), so the isVisible check above must come first or this
    // renders a spinner that never resolves.
    if (isLoading) {
        return (
            <View className="items-center py-6">
                <ActivityIndicator size="small" />
            </View>
        )
    }

    // A failed load must not render as "no clients" — that reads as a healthy
    // empty registry and would hide, for instance, a client that IS registered
    // and compromised.
    if (isError) {
        return (
            <Text className="text-destructive text-sm">
                Couldn't load OAuth clients. Try reloading the page.
            </Text>
        )
    }

    if (!clients || clients.length === 0) {
        return (
            <Text className="text-muted-foreground text-sm">No OAuth clients are registered.</Text>
        )
    }

    return (
        <View className="gap-3">
            <Text className="text-[13px] text-muted-foreground">
                Applications allowed to request access to this organization. Turning one off blocks
                new sign-ins and immediately cuts off the access it already has — without deleting
                anything, so you can turn it back on.
            </Text>

            <View className="gap-1.5">
                {clients.map(client => (
                    <ClientRow
                        key={client.id}
                        client={client}
                        isBusy={setDisabled.isPending && setDisabled.variables?.id === client.id}
                        onChange={disabled => applyDisabled(client, disabled)}
                    />
                ))}
            </View>

            <ConfirmDialog
                isOpen={pendingDisable !== null}
                onClose={() => setPendingDisable(null)}
                onConfirm={confirmDisable}
                title="Turn off this client?"
                message={disableWarning(pendingDisable)}
                confirmLabel="Turn off"
                isDestructive
                isSubmitting={setDisabled.isPending}
            />
        </View>
    )
}

// disableWarning states the blast radius in terms of what the admin will
// actually break — live connections — and that it is reversible, which is the
// fact that makes this safe to use decisively during an incident.
export function disableWarning(client: OAuthClient | null): string {
    if (!client) return ''
    const name = client.name || client.client_id

    if (client.active_grants === 0) {
        return `"${name}" will no longer be able to sign anyone in. You can turn it back on at any time.`
    }
    const conn =
        client.active_grants === 1
            ? '1 active connection'
            : `${client.active_grants} active connections`
    return `"${name}" will immediately lose access, disconnecting ${conn}. You can turn it back on at any time, and nothing is deleted.`
}

function ClientRow({
    client,
    isBusy,
    onChange,
}: {
    client: OAuthClient
    isBusy: boolean
    onChange: (disabled: boolean) => void
}) {
    return (
        <View className="flex-row items-center gap-3 rounded-lg border border-border px-3 py-2.5">
            <View className="flex-1" style={{ minWidth: 0 }}>
                <View className="flex-row items-center gap-1.5" style={{ flexWrap: 'wrap' }}>
                    <Text className="text-foreground text-sm font-medium" numberOfLines={1}>
                        {client.name || client.client_id}
                    </Text>
                    <ClientBadge label={client.type} />
                    <ClientBadge label="first-party" isVisible={client.is_first_party} />
                    <ClientBadge label="off" isVisible={client.disabled} tone="danger" />
                </View>
                <Text className="text-[11px] text-muted-foreground" numberOfLines={1}>
                    {client.client_id} · {describeAccess(client)}
                </Text>
            </View>
            <ClientToggle client={client} isBusy={isBusy} onChange={onChange} />
        </View>
    )
}

function ClientToggle({
    client,
    isBusy,
    onChange,
}: {
    client: OAuthClient
    isBusy: boolean
    onChange: (disabled: boolean) => void
}) {
    if (isBusy) {
        return (
            <View className="p-1.5">
                <ActivityIndicator size="small" />
            </View>
        )
    }

    // The switch reads as "enabled" while the field stores "disabled" — an
    // operator thinks in terms of the client being ON, not the flag being set.
    return (
        <Switch
            value={!client.disabled}
            onValueChange={enabled => onChange(!enabled)}
            accessibilityLabel={`${client.name || client.client_id} enabled`}
        />
    )
}

function ClientBadge({
    label,
    isVisible = true,
    tone = 'neutral',
}: {
    label: string
    isVisible?: boolean
    tone?: 'neutral' | 'danger'
}) {
    if (!isVisible) return null

    const isDanger = tone === 'danger'
    const boxClass = isDanger
        ? 'border-destructive bg-destructive/10'
        : 'border-border bg-surface-secondary'
    const textClass = isDanger ? 'text-destructive' : 'text-muted-foreground'

    return (
        <View className={`rounded-full border px-1.5 ${boxClass}`}>
            <Text className={`text-[10px] font-semibold ${textClass}`}>{label}</Text>
        </View>
    )
}
