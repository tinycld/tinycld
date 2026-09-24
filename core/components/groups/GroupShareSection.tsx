import type { GroupGrant, GroupRoleOption } from '@tinycld/core/lib/groups/types'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { UsersRound } from 'lucide-react-native'
import { useState } from 'react'
import { Text, View } from 'react-native'
import { GroupGrantRow } from './GroupGrantRow'
import { GroupPicker } from './GroupPicker'

interface GroupShareSectionProps<Role extends string> {
    grants: GroupGrant<Role>[]
    isReady: boolean
    roles: readonly GroupRoleOption<Role>[]
    canManage: boolean
    isPending: boolean
    onAdd: (groupId: string, role: Role) => void
    onRoleChange: (grantId: string, role: Role) => void
    onRemove: (grantId: string) => void
}

/**
 * The group half of a package's share dialog. Drop it under the user list;
 * feed it the spread of useGroupGrants plus the package's role options and
 * whether the caller may manage sharing.
 */
export function GroupShareSection<Role extends string>({
    grants,
    isReady,
    roles,
    canManage,
    isPending,
    onAdd,
    onRoleChange,
    onRemove,
}: GroupShareSectionProps<Role>) {
    const [isPicking, setIsPicking] = useState(false)
    const fgColor = useThemeColor('foreground')
    const isEmpty = isReady && grants.length === 0

    if (!canManage && isEmpty) return null

    return (
        <View testID="group-share-section" className="mt-4 gap-2">
            <View className="flex-row items-center justify-between">
                <Text className="text-[12px] font-semibold text-muted uppercase tracking-wide">
                    Groups
                </Text>
                <AddGroupButton
                    isVisible={canManage}
                    color={fgColor}
                    isPending={isPending}
                    onPress={() => setIsPicking(true)}
                />
            </View>
            <EmptyNote isVisible={isEmpty} />
            <GrantList
                grants={grants}
                roles={roles}
                canManage={canManage}
                onRoleChange={onRoleChange}
                onRemove={onRemove}
            />
            <GroupPicker
                isVisible={isPicking}
                excludeIds={new Set(grants.map(g => g.groupId))}
                roles={roles}
                onPick={onAdd}
                onClose={() => setIsPicking(false)}
            />
        </View>
    )
}

function AddGroupButton({
    isVisible,
    color,
    isPending,
    onPress,
}: {
    isVisible: boolean
    color: string
    isPending: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <Button size="sm" variant="outline" onPress={onPress} isDisabled={isPending}>
            <UsersRound size={14} color={color} strokeWidth={2.2} />
            <ButtonText>Add group</ButtonText>
        </Button>
    )
}

function EmptyNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text className="text-[12px] text-muted px-1">
            No groups yet. Everyone in a group you add gets this role.
        </Text>
    )
}

function GrantList<Role extends string>({
    grants,
    roles,
    canManage,
    onRoleChange,
    onRemove,
}: {
    grants: GroupGrant<Role>[]
    roles: readonly GroupRoleOption<Role>[]
    canManage: boolean
    onRoleChange: (grantId: string, role: Role) => void
    onRemove: (grantId: string) => void
}) {
    if (grants.length === 0) return null
    return (
        <View className="rounded-xl border border-border overflow-hidden">
            {grants.map((grant, index) => (
                <View key={grant.grantId} className={index > 0 ? 'border-t border-border' : ''}>
                    <GroupGrantRow
                        grant={grant}
                        roles={roles}
                        canManage={canManage}
                        onRoleChange={onRoleChange}
                        onRemove={onRemove}
                    />
                </View>
            ))}
        </View>
    )
}
