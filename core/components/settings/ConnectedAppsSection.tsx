import { eq } from '@tanstack/db'
import { captureException } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import type { OauthGrants } from '@tinycld/core/types/pbSchema'
import { Trash2 } from 'lucide-react-native'
import { ActivityIndicator, Pressable, Text, View } from 'react-native'

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

    const { data: grants } = useOrgLiveQuery((query, { userId }) =>
        query.from({ grant: grantsCollection }).where(({ grant }) => eq(grant.user, userId))
    )

    const active = (grants ?? []).filter(grant => grant.status === 'active')

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
                            onRevoke={() => revoke.mutate(grant.id)}
                        />
                    ))}
                </View>
            </View>
        </View>
    )
}

function GrantRow({
    grant,
    isRevoking,
    onRevoke,
}: {
    grant: OauthGrants
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
