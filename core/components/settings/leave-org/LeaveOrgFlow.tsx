import { useMutation, useQuery } from '@tanstack/react-query'
import { errorToString } from '@tinycld/core/lib/errors'
import {
    fetchLeaveOrgPreview,
    type LeaveOrgPlan,
    type LeaveOrgPreview,
    type LeaveOrgPreviewPeer,
    type LeaveOrgResult,
    labelForCount,
    postLeaveOrg,
} from '@tinycld/core/lib/leave-org'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useState } from 'react'
import { ActivityIndicator, Pressable, Text, TextInput, View } from 'react-native'

export type LeaveOrgFlowMode = 'self' | 'admin'

export interface LeaveOrgFlowProps {
    isVisible: boolean
    onClose: () => void
    onSuccess: (result: LeaveOrgResult) => void
    userOrgId: string
    // mode controls copy only — auth + behavior is enforced server-side.
    mode: LeaveOrgFlowMode
    // Display name of the user being removed (for admin mode); ignored when
    // mode === 'self'.
    targetDisplayName?: string
}

export function LeaveOrgFlow(props: LeaveOrgFlowProps) {
    if (!props.isVisible) return null
    return <LeaveOrgFlowOpen {...props} />
}

function LeaveOrgFlowOpen({
    onClose,
    onSuccess,
    userOrgId,
    mode,
    targetDisplayName,
}: LeaveOrgFlowProps) {
    const backdropColor = useThemeColor('overlay-backdrop')

    const preview = useQuery({
        queryKey: ['leave-org-preview', userOrgId],
        queryFn: () => fetchLeaveOrgPreview(userOrgId),
        staleTime: 0,
    })

    return (
        <View
            className="absolute top-0 left-0 right-0 bottom-0 justify-center items-center"
            style={{ zIndex: 200, backgroundColor: backdropColor }}
        >
            <View
                className="rounded-2xl border border-border p-6 bg-background"
                style={{
                    width: 480,
                    maxWidth: '95%',
                    maxHeight: '90%',
                    shadowColor: '#000',
                    shadowOffset: { width: 0, height: 8 },
                    shadowOpacity: 0.15,
                    shadowRadius: 24,
                    elevation: 8,
                }}
            >
                <LoadingState isVisible={preview.isPending} />
                <ErrorState
                    isVisible={preview.isError}
                    message={preview.error ? errorToString(preview.error) : ''}
                    onClose={onClose}
                />
                <FlowBody
                    isVisible={!!preview.data}
                    preview={preview.data}
                    onClose={onClose}
                    onSuccess={onSuccess}
                    userOrgId={userOrgId}
                    mode={mode}
                    targetDisplayName={targetDisplayName}
                />
            </View>
        </View>
    )
}

function LoadingState({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="py-12 items-center">
            <ActivityIndicator size="large" />
        </View>
    )
}

function ErrorState({
    isVisible,
    message,
    onClose,
}: {
    isVisible: boolean
    message: string
    onClose: () => void
}) {
    if (!isVisible) return null
    return (
        <View className="gap-4">
            <Text className="text-foreground text-lg font-bold">Couldn't load org info</Text>
            <Text className="text-danger text-sm">{message}</Text>
            <CancelButton label="Close" onPress={onClose} disabled={false} />
        </View>
    )
}

function FlowBody({
    isVisible,
    preview,
    onClose,
    onSuccess,
    userOrgId,
    mode,
    targetDisplayName,
}: {
    isVisible: boolean
    preview: LeaveOrgPreview | undefined
    onClose: () => void
    onSuccess: (result: LeaveOrgResult) => void
    userOrgId: string
    mode: LeaveOrgFlowMode
    targetDisplayName?: string
}) {
    if (!isVisible || !preview) return null
    if (preview.sole_member) {
        return (
            <SoleMemberConfirm
                preview={preview}
                userOrgId={userOrgId}
                mode={mode}
                onClose={onClose}
                onSuccess={onSuccess}
            />
        )
    }
    return (
        <MultiMemberFlow
            preview={preview}
            userOrgId={userOrgId}
            mode={mode}
            targetDisplayName={targetDisplayName}
            onClose={onClose}
            onSuccess={onSuccess}
        />
    )
}

// SoleMemberConfirm — user is the only member of this org. Leaving deletes
// the entire org and all its data. Requires typed confirmation of the org
// name to prevent fat-finger destruction.
function SoleMemberConfirm({
    preview,
    userOrgId,
    mode,
    onClose,
    onSuccess,
}: {
    preview: LeaveOrgPreview
    userOrgId: string
    mode: LeaveOrgFlowMode
    onClose: () => void
    onSuccess: (result: LeaveOrgResult) => void
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const [typed, setTyped] = useState('')
    const [error, setError] = useState<string | null>(null)
    const expected = preview.org_name.trim()
    const matches = typed.trim() === expected
    const totalCount = Object.values(preview.counts).reduce((a, b) => a + b, 0)
    const totalRecords = totalCount

    const mutation = useMutation({
        mutationFn: () => postLeaveOrg(userOrgId, { mode: 'delete_org' }),
        onSuccess,
        onError: e => setError(errorToString(e)),
    })

    const canSubmit = matches && !mutation.isPending

    const action =
        mode === 'self'
            ? `You're the only member of ${preview.org_name}.`
            : `This user is the only member of ${preview.org_name}.`

    return (
        <View className="gap-4">
            <Text className="text-foreground text-lg font-bold">Delete this organization?</Text>
            <Text className="text-muted-foreground text-sm">{action}</Text>
            <Text className="text-muted-foreground text-sm">
                Leaving will permanently delete the org and{' '}
                <Text className="text-foreground font-semibold">all {totalRecords} records</Text> in
                it across every app. This cannot be undone.
            </Text>

            <CountsList counts={preview.counts} />

            <ErrorMessage message={error} />

            <View>
                <Text className="text-foreground text-sm font-semibold mb-1.5">
                    Type the org name to confirm
                </Text>
                <TextInput
                    className="border border-border rounded-lg p-3 text-base text-foreground bg-surface-secondary"
                    value={typed}
                    onChangeText={setTyped}
                    placeholder={preview.org_name}
                    placeholderTextColor={mutedColor}
                    editable={!mutation.isPending}
                />
            </View>

            <DangerButton
                label={`Delete ${preview.org_name}`}
                onPress={() => mutation.mutate()}
                disabled={!canSubmit}
                isPending={mutation.isPending}
            />
            <CancelButton onPress={onClose} disabled={mutation.isPending} />
        </View>
    )
}

// MultiMemberFlow — leaver has peers in the org. They can either reassign
// their records to a chosen peer or delete the records outright. If they are
// the sole owner, the chosen peer is implicitly promoted to owner.
function MultiMemberFlow({
    preview,
    userOrgId,
    mode,
    targetDisplayName,
    onClose,
    onSuccess,
}: {
    preview: LeaveOrgPreview
    userOrgId: string
    mode: LeaveOrgFlowMode
    targetDisplayName?: string
    onClose: () => void
    onSuccess: (result: LeaveOrgResult) => void
}) {
    // Track only the user's explicit override. The effective successor is
    // derived during render from `userPickedSuccessor || defaultSuccessor(preview)`,
    // so when the preview's peer list refetches we naturally update without
    // a useState+useEffect dance. The default-picked id is recomputed each
    // render, which is cheap — preview is memoized by react-query.
    const [userPickedSuccessor, setUserPickedSuccessor] = useState<string>('')
    const [planMode, setPlanMode] = useState<'reassign' | 'delete_my_data'>('reassign')
    const [error, setError] = useState<string | null>(null)

    const effectiveSuccessor = userPickedSuccessor || defaultSuccessor(preview)

    const mutation = useMutation({
        mutationFn: () => {
            const plan: LeaveOrgPlan =
                planMode === 'reassign'
                    ? { mode: 'reassign', successor_user_org_id: effectiveSuccessor }
                    : { mode: 'delete_my_data' }
            return postLeaveOrg(userOrgId, plan)
        },
        onSuccess,
        onError: e => setError(errorToString(e)),
    })

    const totalCount = Object.values(preview.counts).reduce((a, b) => a + b, 0)
    const subjectLabel = mode === 'admin' ? (targetDisplayName ?? 'this user') : 'your'

    return (
        <View className="gap-4">
            <Text className="text-foreground text-lg font-bold">
                {mode === 'admin'
                    ? `Remove ${targetDisplayName ?? 'this user'} from ${preview.org_name}`
                    : `Leave ${preview.org_name}`}
            </Text>

            <SoleOwnerWarning isVisible={preview.sole_owner} />

            <Text className="text-muted-foreground text-sm">
                {totalCount > 0
                    ? `Choose what happens to ${mode === 'admin' ? `${subjectLabel}'s` : 'your'} ${totalCount} records in this org.`
                    : `${mode === 'admin' ? subjectLabel : 'You'} have no records in this org. Removal is straightforward.`}
            </Text>

            <CountsList counts={preview.counts} />

            <ModePicker isVisible={totalCount > 0} mode={planMode} onSelect={setPlanMode} />

            <SuccessorPicker
                isVisible={planMode === 'reassign'}
                peers={preview.peers}
                chosen={effectiveSuccessor}
                onSelect={setUserPickedSuccessor}
                soleOwner={preview.sole_owner}
            />

            <ErrorMessage message={error} />

            <DangerButton
                label={
                    planMode === 'delete_my_data'
                        ? `Delete records and ${mode === 'admin' ? 'remove' : 'leave'}`
                        : mode === 'admin'
                          ? 'Reassign and remove'
                          : 'Reassign and leave'
                }
                onPress={() => mutation.mutate()}
                disabled={
                    mutation.isPending ||
                    (planMode === 'reassign' && totalCount > 0 && !effectiveSuccessor)
                }
                isPending={mutation.isPending}
            />
            <CancelButton onPress={onClose} disabled={mutation.isPending} />
        </View>
    )
}

function defaultSuccessor(preview: LeaveOrgPreview): string {
    // Match the server's auto-pick: oldest owner first, oldest peer otherwise.
    // Peers come back in +created order from the server.
    const oldestOwner = preview.peers.find(p => p.role === 'owner')
    if (oldestOwner) return oldestOwner.user_org_id
    return preview.peers[0]?.user_org_id ?? ''
}

function SoleOwnerWarning({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="rounded-lg p-3 bg-warning-soft border border-warning">
            <Text className="text-warning text-sm font-semibold">
                You are the only owner of this org.
            </Text>
            <Text className="text-warning text-xs">
                The person you choose will be promoted to owner so the org isn't left ownerless.
            </Text>
        </View>
    )
}

function CountsList({ counts }: { counts: Record<string, number> }) {
    const entries = Object.entries(counts).filter(([, n]) => n > 0)
    if (entries.length === 0) return null
    return (
        <View className="rounded-lg p-3 bg-surface-secondary border border-border gap-1">
            {entries.map(([key, n]) => (
                <View key={key} className="flex-row justify-between">
                    <Text className="text-foreground text-sm">{labelForCount(key)}</Text>
                    <Text className="text-foreground text-sm font-semibold">{n}</Text>
                </View>
            ))}
        </View>
    )
}

function ModePicker({
    isVisible,
    mode,
    onSelect,
}: {
    isVisible: boolean
    mode: 'reassign' | 'delete_my_data'
    onSelect: (v: 'reassign' | 'delete_my_data') => void
}) {
    if (!isVisible) return null
    return (
        <View className="gap-2">
            <ModeOption
                value="reassign"
                label="Reassign to another member"
                description="Records stay; ownership transfers."
                selected={mode === 'reassign'}
                onSelect={onSelect}
            />
            <ModeOption
                value="delete_my_data"
                label="Delete the records"
                description="Permanently remove them. Cannot be undone."
                selected={mode === 'delete_my_data'}
                onSelect={onSelect}
            />
        </View>
    )
}

function ModeOption({
    value,
    label,
    description,
    selected,
    onSelect,
}: {
    value: 'reassign' | 'delete_my_data'
    label: string
    description: string
    selected: boolean
    onSelect: (v: 'reassign' | 'delete_my_data') => void
}) {
    return (
        <Pressable
            onPress={() => onSelect(value)}
            className={`rounded-lg p-3 border ${selected ? 'border-primary bg-primary-soft' : 'border-border'}`}
        >
            <Text className="text-foreground text-sm font-semibold">{label}</Text>
            <Text className="text-muted-foreground text-xs">{description}</Text>
        </Pressable>
    )
}

function SuccessorPicker({
    isVisible,
    peers,
    chosen,
    onSelect,
    soleOwner,
}: {
    isVisible: boolean
    peers: LeaveOrgPreviewPeer[]
    chosen: string
    onSelect: (id: string) => void
    soleOwner: boolean
}) {
    if (!isVisible) return null
    return (
        <View className="gap-2">
            <Text className="text-foreground text-sm font-semibold">
                {soleOwner ? 'Promote to owner' : 'Reassign to'}
            </Text>
            {peers.map(p => (
                <PeerRow
                    key={p.user_org_id}
                    peer={p}
                    selected={chosen === p.user_org_id}
                    onSelect={() => onSelect(p.user_org_id)}
                />
            ))}
        </View>
    )
}

function PeerRow({
    peer,
    selected,
    onSelect,
}: {
    peer: LeaveOrgPreviewPeer
    selected: boolean
    onSelect: () => void
}) {
    return (
        <Pressable
            onPress={onSelect}
            className={`rounded-lg p-3 border flex-row items-center justify-between ${selected ? 'border-primary bg-primary-soft' : 'border-border'}`}
        >
            <View>
                <Text className="text-foreground text-sm font-semibold">
                    {peer.name || peer.email}
                </Text>
                <Text className="text-muted-foreground text-xs">{peer.role}</Text>
            </View>
        </Pressable>
    )
}

function ErrorMessage({ message }: { message: string | null }) {
    if (!message) return null
    return (
        <View className="rounded-lg p-3 bg-danger-soft">
            <Text className="text-danger text-sm">{message}</Text>
        </View>
    )
}

function DangerButton({
    label,
    onPress,
    disabled,
    isPending,
}: {
    label: string
    onPress: () => void
    disabled: boolean
    isPending: boolean
}) {
    const dangerFg = useThemeColor('danger-foreground')
    return (
        <Pressable
            className={`rounded-lg items-center p-3.5 bg-danger ${disabled ? 'opacity-50' : 'opacity-100'}`}
            onPress={onPress}
            disabled={disabled}
        >
            {isPending ? (
                <ActivityIndicator color={dangerFg} size="small" />
            ) : (
                <Text className="text-base font-semibold text-danger-foreground">{label}</Text>
            )}
        </Pressable>
    )
}

function CancelButton({
    onPress,
    disabled,
    label = 'Cancel',
}: {
    onPress: () => void
    disabled: boolean
    label?: string
}) {
    return (
        <Pressable
            className={`rounded-lg items-center p-3.5 border border-border ${disabled ? 'opacity-50' : 'opacity-100'}`}
            onPress={onPress}
            disabled={disabled}
        >
            <Text className="text-base font-semibold text-foreground">{label}</Text>
        </Pressable>
    )
}
