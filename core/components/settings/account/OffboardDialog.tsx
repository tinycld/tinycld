import { eq, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { labelForCount, type OffboardPlan } from '@tinycld/core/lib/account'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ActivityIndicator, Pressable, Text, TextInput, View } from 'react-native'

// Shared presentation for the two destructive account flows (self-delete and
// admin member-offboard). Both need the same three things: a modal shell, a
// typed confirmation, and a choice about what happens to authored content.
//
// Salvaged from the former LeaveOrgFlow, minus the org-deletion branch (there
// is no org to delete in single-org) and minus the server preview — that
// endpoint went away with the leave-org routes, so the peer list is a plain
// pbtsdb read, which is genuinely a client concern.

export interface Peer {
    id: string
    name: string
    email: string
    role: string
}

// usePeers lists other non-guest users, for the reassign target. Guests are
// excluded: they exist only to hold a share link and shouldn't inherit
// someone's whole library.
export function usePeers(excludeUserId: string): Peer[] {
    const [usersCollection] = useStore('users')
    const { data } = useLiveQuery(
        query =>
            query
                .from({ users: usersCollection })
                .where(({ users }) => not(eq(users.id, excludeUserId))),
        [excludeUserId]
    )
    return (data ?? [])
        .filter(u => u.role !== 'guest' && !u.disabled)
        .map(u => ({ id: u.id, name: u.name, email: u.email, role: u.role }))
}

export function ModalShell({ children }: { children: React.ReactNode }) {
    const backdropColor = useThemeColor('overlay-backdrop')
    return (
        <View
            className="absolute top-0 left-0 right-0 bottom-0 justify-center items-center"
            style={{ zIndex: 200, backgroundColor: backdropColor }}
        >
            <View
                className="rounded-2xl border border-border p-6 bg-background"
                style={{ width: 480, maxWidth: '95%', maxHeight: '90%' }}
            >
                {children}
            </View>
        </View>
    )
}

export function ErrorMessage({ message }: { message: string | null }) {
    if (!message) return null
    return <Text className="text-error text-sm">{message}</Text>
}

export function DangerButton({
    label,
    onPress,
    disabled,
    isPending,
    testID,
}: {
    label: string
    onPress: () => void
    disabled?: boolean
    isPending?: boolean
    testID?: string
}) {
    return (
        <Pressable
            testID={testID}
            onPress={onPress}
            disabled={disabled}
            className={`rounded-lg px-4 py-3 items-center ${disabled ? 'bg-error/50' : 'bg-error'}`}
        >
            {isPending ? (
                <ActivityIndicator />
            ) : (
                <Text className="text-on-error font-semibold">{label}</Text>
            )}
        </Pressable>
    )
}

export function CancelButton({ onPress, disabled }: { onPress: () => void; disabled?: boolean }) {
    return (
        <Pressable
            onPress={onPress}
            disabled={disabled}
            className="rounded-lg px-4 py-3 items-center border border-border"
        >
            <Text className="text-foreground font-semibold">Cancel</Text>
        </Pressable>
    )
}

// ConfirmInput is the type-this-exact-string gate both destructive flows use.
export function ConfirmInput({
    label,
    placeholder,
    value,
    onChangeText,
    editable,
    testID,
}: {
    label: string
    placeholder: string
    value: string
    onChangeText: (v: string) => void
    editable: boolean
    testID?: string
}) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <View>
            <Text className="text-foreground text-sm font-semibold mb-1.5">{label}</Text>
            <TextInput
                testID={testID}
                className="border border-border rounded-lg p-3 text-base text-foreground bg-surface-secondary"
                value={value}
                onChangeText={onChangeText}
                placeholder={placeholder}
                placeholderTextColor={mutedColor}
                editable={editable}
                autoCapitalize="none"
                autoCorrect={false}
            />
        </View>
    )
}

// ContentPlanPicker: reassign the subject's records to a peer, or delete them.
// Returns the plan the caller submits.
//
// `value` may be undefined — nothing highlighted — for callers that require an
// explicit choice before submitting (self-delete). Callers with an honest
// pre-selected default (admin remove, which submits exactly what it shows)
// pass a definite plan. Either way the highlight must always equal what would
// be sent: highlighting an option the submit doesn't send is the bug this
// shape exists to prevent.
export function ContentPlanPicker({
    peers,
    subjectLabel,
    value,
    onChange,
    disabled,
}: {
    peers: Peer[]
    subjectLabel: string
    value: OffboardPlan | undefined
    onChange: (plan: OffboardPlan) => void
    disabled?: boolean
}) {
    // No peers means there is nobody to reassign to, so the only real choice
    // is delete-or-keep. Hide the picker rather than offer a dead option.
    if (peers.length === 0) return null

    const isReassign = value?.mode === 'reassign'
    const isDelete = value?.mode === 'delete_my_data'
    return (
        <View className="gap-2">
            <Text className="text-foreground text-sm font-semibold">
                What happens to {subjectLabel} files, documents and comments?
            </Text>
            <Pressable
                disabled={disabled}
                onPress={() =>
                    onChange({ mode: 'reassign', successor_user_id: peers[0]?.id ?? '' })
                }
                className={`rounded-lg border p-3 ${isReassign ? 'border-primary' : 'border-border'}`}
            >
                <Text className="text-foreground font-semibold">Reassign to another member</Text>
                <Text className="text-muted-foreground text-[13px]">
                    Ownership transfers; nothing is lost.
                </Text>
            </Pressable>
            {isReassign && (
                <View className="gap-1 pl-3">
                    {peers.map(peer => (
                        <Pressable
                            key={peer.id}
                            disabled={disabled}
                            onPress={() =>
                                onChange({ mode: 'reassign', successor_user_id: peer.id })
                            }
                            className={`rounded-lg border px-3 py-2 ${
                                value?.successor_user_id === peer.id
                                    ? 'border-primary'
                                    : 'border-border'
                            }`}
                        >
                            <Text className="text-foreground text-sm">
                                {peer.name || peer.email}
                            </Text>
                        </Pressable>
                    ))}
                </View>
            )}
            <Pressable
                disabled={disabled}
                onPress={() => onChange({ mode: 'delete_my_data' })}
                className={`rounded-lg border p-3 ${isDelete ? 'border-error' : 'border-border'}`}
            >
                <Text className="text-foreground font-semibold">Delete everything</Text>
                <Text className="text-muted-foreground text-[13px]">
                    Permanently removes the content. This cannot be undone.
                </Text>
            </Pressable>
        </View>
    )
}

export function CountsList({ counts }: { counts: Record<string, number> }) {
    const entries = Object.entries(counts).filter(([, n]) => n > 0)
    if (entries.length === 0) return null
    return (
        <View className="gap-1">
            {entries.map(([key, n]) => (
                <Text key={key} className="text-muted-foreground text-[13px]">
                    {labelForCount(key)}: {n}
                </Text>
            ))}
        </View>
    )
}
