import { eq } from '@tanstack/db'
import { captureException } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { Trash2 } from 'lucide-react-native'
import { useState } from 'react'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'

// The projection GrantRow actually renders — see the .select() comment below
// for why this is narrower than the full OauthGrants row.
interface ConnectedAppGrant {
    id: string
    device_label: string
    last_used_at: string
    status: string
}

// formatLastUsed turns an ISO timestamp into the coarse relative string the
// list shows. Coarse on purpose: the exact minute is noise, and "never used"
// is the signal that matters when auditing what to revoke.
export function formatLastUsed(iso: string): string {
    if (!iso) return 'Never used'
    const then = new Date(iso).getTime()
    if (Number.isNaN(then)) return 'Never used'

    const minutes = Math.floor((Date.now() - then) / 60000)
    if (minutes < 60) return 'Used in the last hour'
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return `Used ${hours} hour${hours === 1 ? '' : 's'} ago`
    const days = Math.floor(hours / 24)
    return `Used ${days} day${days === 1 ? '' : 's'} ago`
}

// Revocation goes through POST /oauth/grants/{id}/revoke, a session-authenticated
// endpoint — never /oauth/revoke (RFC 7009), which authenticates by presenting a
// TOKEN and has no shape for "a grant row id from a browser session". The write
// itself happens in Go (RevokeGrant), so this mutation is a plain async call
// rather than a pbtsdb Transaction; the list still refreshes live because
// oauth_grants is realtime-subscribed and PocketBase broadcasts the server-side
// update to every subscriber, regardless of who wrote it.
function useRevokeGrant() {
    return useMutation({
        mutationFn: async (grantId: string) => {
            await pb.send(`/oauth/grants/${grantId}/revoke`, { method: 'POST' })
        },
        onError: err => captureException('oauth.connectedApps.revoke', err),
    })
}

export function ConnectedAppsSection() {
    const [grantsCollection] = useStore('oauth_grants')
    const revoke = useRevokeGrant()
    // The grant awaiting confirmation, not yet revoked. Revoke is one mis-tap
    // away from killing a live CLI or integration session with no undo, so
    // the trash icon opens a confirmation instead of revoking immediately.
    const [pendingRevoke, setPendingRevoke] = useState<ConnectedAppGrant | null>(null)

    // .select() narrows what the client store (and the realtime subscription
    // feeding it) actually holds. oauth_grants rows carry credential material
    // (refresh_token_hash, device_code, auth_code_hash) that PocketBase's
    // row-scoped list/view rule does not redact — only field selection does —
    // and this screen has no business holding any of it in memory just to
    // render a label and a timestamp.
    const { data: grants } = useOrgLiveQuery((query, { userId }) =>
        query
            .from({ grant: grantsCollection })
            .where(({ grant }) => eq(grant.user, userId))
            .select(({ grant }) => ({
                id: grant.id,
                device_label: grant.device_label,
                last_used_at: grant.last_used_at,
                status: grant.status,
            }))
    )

    const active = (grants ?? []).filter(grant => grant.status === 'active')

    const confirmRevoke = () => {
        if (!pendingRevoke) return
        revoke.mutate(pendingRevoke.id)
        setPendingRevoke(null)
    }

    if (active.length === 0) return null

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">Connected apps</Text>
            <View className="rounded-xl border border-border bg-surface-secondary p-4 gap-2">
                <Text className="text-[13px] text-muted-foreground">
                    Devices and integrations with access to your account. Revoke anything you don't
                    recognize.
                </Text>
                <View className="gap-1.5 mt-1">
                    {active.map(grant => (
                        <GrantRow
                            key={grant.id}
                            grant={grant}
                            isRevoking={revoke.isPending && revoke.variables === grant.id}
                            onRevoke={() => setPendingRevoke(grant)}
                        />
                    ))}
                </View>
            </View>
            <ConfirmDialog
                isOpen={pendingRevoke !== null}
                onClose={() => setPendingRevoke(null)}
                onConfirm={confirmRevoke}
                title="Revoke access?"
                message={`"${pendingRevoke?.device_label || 'This device'}" will immediately lose access to your account. This can't be undone.`}
                confirmLabel="Revoke"
                isDestructive
                isSubmitting={revoke.isPending}
            />
        </View>
    )
}

function GrantRow({
    grant,
    isRevoking,
    onRevoke,
}: {
    grant: ConnectedAppGrant
    isRevoking: boolean
    onRevoke: () => void
}) {
    const danger = useThemeColor('danger')

    return (
        <View className="flex-row items-center gap-2 rounded-lg border border-border px-3 py-2.5">
            <View className="flex-1">
                <Text className="text-foreground text-sm font-medium">
                    {grant.device_label || 'Unnamed device'}
                </Text>
                <Text className="text-[11px] text-muted-foreground">
                    {formatLastUsed(grant.last_used_at)}
                </Text>
            </View>
            <RevokeButton isRevoking={isRevoking} color={danger} onPress={onRevoke} />
        </View>
    )
}

function RevokeButton({
    isRevoking,
    color,
    onPress,
}: {
    isRevoking: boolean
    color: string
    onPress: () => void
}) {
    if (isRevoking) {
        return (
            <View className="p-1.5">
                <ActivityIndicator size="small" />
            </View>
        )
    }

    return (
        <Pressable
            onPress={onPress}
            accessibilityRole="button"
            accessibilityLabel="Revoke access"
            className="p-1.5"
        >
            <Trash2 size={15} color={color} />
        </Pressable>
    )
}
