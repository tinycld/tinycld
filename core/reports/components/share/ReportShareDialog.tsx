import { GroupShareSection } from '@tinycld/core/components/groups/GroupShareSection'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useRecord } from '@tinycld/core/lib/use-record'
import { Dialog } from '@tinycld/core/ui/dialog'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { type ReportShares, useReportShares } from '../../hooks/useReportShares'
import { SHARE_ROLES, type ShareRole } from '../../lib/access'
import { useReportUIStore } from '../../stores/report-ui-store'
import { REPORT_TAP_TARGET } from '../ReportHeader'

type DirectShare = ReportShares['direct'][number]
type Candidate = ReportShares['candidates'][number]

function roleLabel(role: ShareRole): string {
    return SHARE_ROLES.find(r => r.value === role)?.label ?? role
}

function roleItems(current: ShareRole, onPick: (role: ShareRole) => void): PickItem[] {
    return SHARE_ROLES.map(r => ({
        id: r.value,
        label: r.label,
        isSelected: r.value === current,
        onSelect: () => onPick(r.value),
    }))
}

function UserName({ userId, className }: { userId: string; className: string }) {
    const [users] = useStore('users')
    const { record } = useRecord(users, userId)
    return (
        <Text numberOfLines={1} className={className}>
            {record?.name ?? ''}
        </Text>
    )
}

function SearchRow({ share }: { share: ReportShares }) {
    const muted = useThemeColor('muted-foreground')
    return (
        <View className="flex-row items-center gap-2 px-5 pb-3">
            <PlainInput
                value={share.search}
                onChangeText={share.setSearch}
                placeholder="Add people by name"
                placeholderTextColor={muted}
                accessibilityLabel="Add people by name"
                testID="report-share-search"
                className="flex-1 rounded-md border border-border px-3 py-2 text-sm text-foreground"
            />
            <PickMenu
                label={roleLabel(share.role)}
                purpose="Role for people you add"
                items={roleItems(share.role, share.setRole)}
                testID="report-share-role"
            />
        </View>
    )
}

function CandidateRow({ candidate, onPick }: { candidate: Candidate; onPick: () => void }) {
    return (
        <Pressable
            testID={`report-share-candidate-${candidate.id}`}
            accessibilityRole="button"
            onPress={onPick}
            className="justify-center rounded-md px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className="text-sm text-foreground">{candidate.name}</Text>
        </Pressable>
    )
}

function Candidates({ share }: { share: ReportShares }) {
    if (share.candidates.length === 0) return null
    const rows = share.candidates.map(candidate => (
        <CandidateRow
            key={candidate.id}
            candidate={candidate}
            onPick={() => share.add(candidate.id)}
        />
    ))
    return <View className="gap-1 pb-3">{rows}</View>
}

function DirectRow({ share, row }: { share: ReportShares; row: DirectShare }) {
    const color = useThemeColor('muted-foreground')
    return (
        <View
            testID={`report-share-row-${row.id}`}
            className="flex-row items-center gap-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <UserName userId={row.user} className="min-w-0 flex-1 text-sm text-foreground" />
            <PickMenu
                label={roleLabel(row.role)}
                purpose="Role"
                items={roleItems(row.role, next => share.changeRole(row.id, next))}
                testID={`report-share-row-role-${row.id}`}
            />
            <Pressable
                testID={`report-share-row-remove-${row.id}`}
                accessibilityRole="button"
                accessibilityLabel="Remove access"
                onPress={() => share.remove(row.id)}
                className="items-center justify-center"
                style={{ width: REPORT_TAP_TARGET, height: REPORT_TAP_TARGET }}
            >
                <Trash2 size={16} color={color} />
            </Pressable>
        </View>
    )
}

function People({ share }: { share: ReportShares }) {
    const rows = share.direct.map(row => <DirectRow key={row.id} share={share} row={row} />)
    return (
        <View className="gap-1">
            <Text className="text-xs font-semibold uppercase text-muted-foreground">
                People with access
            </Text>
            <View className="flex-row items-center gap-2" style={{ minHeight: REPORT_TAP_TARGET }}>
                <UserName userId={share.owner} className="min-w-0 flex-1 text-sm text-foreground" />
                <Text className="text-sm text-muted-foreground">Owner</Text>
            </View>
            {rows}
        </View>
    )
}

function PublishRow({ share }: { share: ReportShares }) {
    if (!share.access.canPublish) return null
    return (
        <View className="flex-row items-center gap-3 border-t border-border pt-3">
            <View className="min-w-0 flex-1">
                <Text className="text-sm font-medium text-foreground">Publish to everyone</Text>
                <Text className="text-xs text-muted-foreground">
                    Every member sees it under Published. Guests do not.
                </Text>
            </View>
            <Switch
                value={share.published}
                onValueChange={share.setPublished}
                accessibilityLabel="Publish to everyone"
                testID="report-share-publish"
            />
        </View>
    )
}

function OpenShareDialog({ reportId, onClose }: { reportId: string; onClose: () => void }) {
    const share = useReportShares(reportId)
    return (
        <Dialog
            isOpen
            onClose={onClose}
            title={`Share “${share.title}”`}
            size="lg"
            testID="report-share-dialog"
        >
            <SearchRow share={share} />
            <Dialog.Body contentClassName="px-5 gap-4">
                <Candidates share={share} />
                <People share={share} />
                <GroupShareSection
                    grants={share.groups.grants}
                    isReady={share.groups.isReady}
                    roles={SHARE_ROLES}
                    canManage={share.access.canShare}
                    isPending={share.groups.isPending}
                    onAdd={share.groups.onAdd}
                    onRoleChange={share.groups.onRoleChange}
                    onRemove={share.groups.onRemove}
                />
                <PublishRow share={share} />
                <Text testID="report-share-note" className="text-sm text-muted-foreground">
                    Each person sees only the data they can access.
                </Text>
            </Dialog.Body>
            <Dialog.Footer>
                <Dialog.ActionButton
                    label="Done"
                    onPress={onClose}
                    requiresServer={false}
                    testID="report-share-done"
                />
            </Dialog.Footer>
        </Dialog>
    )
}

// Mounted once per screen; opened from the hub or a report through the store.
export function ReportShareDialog() {
    const reportId = useReportUIStore(s => s.shareReportId)
    const closeShare = useReportUIStore(s => s.closeShare)
    if (!reportId) return null
    return <OpenShareDialog reportId={reportId} onClose={closeShare} />
}
