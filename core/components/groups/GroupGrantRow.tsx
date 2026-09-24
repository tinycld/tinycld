import type { GroupGrant, GroupRoleOption } from '@tinycld/core/lib/groups/types'
import { useGroupSummary } from '@tinycld/core/lib/groups/use-group-summary'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import { ChevronDown, UsersRound, X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

interface GroupGrantRowProps<Role extends string> {
    grant: GroupGrant<Role>
    roles: readonly GroupRoleOption<Role>[]
    canManage: boolean
    onRoleChange: (grantId: string, role: Role) => void
    onRemove: (grantId: string) => void
}

function memberLabel(count: number) {
    return count === 1 ? '1 member' : `${count} members`
}

export function GroupGrantRow<Role extends string>({
    grant,
    roles,
    canManage,
    onRoleChange,
    onRemove,
}: GroupGrantRowProps<Role>) {
    const summary = useGroupSummary(grant.groupId)
    const fgColor = useThemeColor('foreground')
    const roleLabel = roles.find(r => r.value === grant.role)?.label ?? grant.role

    return (
        <View
            testID={`group-grant-row-${grant.groupId}`}
            className="flex-row items-center gap-3 py-2.5 px-3"
        >
            <UsersRound size={22} color={fgColor} />
            <View className="flex-1 min-w-0">
                <Text className="text-[13.5px] font-medium text-foreground" numberOfLines={1}>
                    {summary.name}
                </Text>
                <Text className="text-[12px] text-muted" numberOfLines={1}>
                    {memberLabel(summary.memberCount)}
                </Text>
            </View>
            <RoleControl
                label={roleLabel}
                name={summary.name}
                roles={roles}
                current={grant.role}
                canManage={canManage}
                onSelect={role => onRoleChange(grant.grantId, role)}
            />
            <RemoveButton
                isVisible={canManage}
                name={summary.name}
                onPress={() => onRemove(grant.grantId)}
            />
        </View>
    )
}

function RoleControl<Role extends string>({
    label,
    name,
    roles,
    current,
    canManage,
    onSelect,
}: {
    label: string
    name: string
    roles: readonly GroupRoleOption<Role>[]
    current: Role
    canManage: boolean
    onSelect: (role: Role) => void
}) {
    const mutedColor = useThemeColor('muted')
    if (!canManage) {
        return (
            <View className="px-2.5 py-1 rounded-md bg-foreground/[0.06]">
                <Text className="text-[12px] font-medium text-foreground">{label}</Text>
            </View>
        )
    }
    return (
        <Menu
            trigger={
                <Pressable
                    accessibilityRole="button"
                    accessibilityLabel={`Change role for group ${name}`}
                    className="flex-row items-center gap-1 px-2.5 py-1 rounded-md border border-border bg-background web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring"
                >
                    <Text className="text-[12px] font-medium text-foreground">{label}</Text>
                    <ChevronDown size={14} color={mutedColor} strokeWidth={2.2} />
                </Pressable>
            }
            placement="bottom-end"
            title="Role"
        >
            {roles.map(option => (
                <Menu.Item
                    key={option.value}
                    label={option.label}
                    isSelected={option.value === current}
                    onSelect={() => onSelect(option.value)}
                />
            ))}
        </Menu>
    )
}

function RemoveButton({
    isVisible,
    name,
    onPress,
}: {
    isVisible: boolean
    name: string
    onPress: () => void
}) {
    const dangerColor = useThemeColor('danger')
    if (!isVisible) return <View style={{ width: 28, height: 28 }} />
    return (
        <Pressable
            accessibilityRole="button"
            accessibilityLabel={`Remove group ${name}`}
            onPress={onPress}
            hitSlop={8}
            className="p-1.5 rounded-md web:outline-none web:focus-visible:ring-2 web:focus-visible:ring-ring"
        >
            <X size={16} color={dangerColor} strokeWidth={2.2} />
        </Pressable>
    )
}
