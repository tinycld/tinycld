import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { packageSettings } from '@tinycld/core/lib/packages/derive-components'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useRouter } from 'expo-router'
import {
    ChevronRight,
    HardDrive,
    History,
    Package,
    ScrollText,
    Tag,
    User,
    Users,
} from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

export default function SettingsIndex() {
    const foregroundColor = useThemeColor('foreground')
    const { isAdmin, isOwner, isReady } = useCurrentRole()
    const orgHref = useOrgHref()
    const router = useRouter()

    return (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
            <View className="p-5 max-w-[600px] w-full">
                <Text className="mb-4 text-foreground text-[28px] font-bold">Settings</Text>

                <SettingsGroup label="Account">
                    <SettingsLink
                        label="Personal"
                        onPress={() => router.push(orgHref('settings/personal'))}
                        icon={<User size={20} color={foregroundColor} />}
                    />
                </SettingsGroup>

                <AdminSettings isVisible={isAdmin} isOwner={isReady && isOwner} />
            </View>
        </ScrollView>
    )
}

function AdminSettings({ isVisible, isOwner }: { isVisible: boolean; isOwner: boolean }) {
    const foregroundColor = useThemeColor('foreground')
    const orgHref = useOrgHref()
    const router = useRouter()

    if (!isVisible) return null

    return (
        <>
            <SettingsGroup label="Organization">
                <SettingsLink
                    label="Storage"
                    onPress={() => router.push(orgHref('settings/storage'))}
                    icon={<HardDrive size={20} color={foregroundColor} />}
                />
                <SettingsLink
                    label="Members"
                    onPress={() => router.push(orgHref('settings/members'))}
                    icon={<Users size={20} color={foregroundColor} />}
                />
                <SettingsLink
                    label="Labels"
                    onPress={() => router.push(orgHref('settings/labels'))}
                    icon={<Tag size={20} color={foregroundColor} />}
                />
                <SettingsLink
                    label="Audit Log"
                    onPress={() => router.push(orgHref('settings/audit-log'))}
                    icon={<ScrollText size={20} color={foregroundColor} />}
                />
                <OwnerLinks isVisible={isOwner} />
            </SettingsGroup>

            {packageSettings.map(group => {
                const Icon = getIcon(group.icon ?? '')
                return (
                    <SettingsGroup key={group.pkgSlug} label={group.packageName}>
                        {group.panels.map(panel => (
                            <SettingsLink
                                key={panel.slug}
                                label={panel.label}
                                onPress={() =>
                                    router.push(
                                        orgHref('settings/[...section]', {
                                            section: [group.pkgSlug, panel.slug],
                                        })
                                    )
                                }
                                icon={<Icon size={20} color={foregroundColor} />}
                            />
                        ))}
                    </SettingsGroup>
                )
            })}
        </>
    )
}

// Owner-only entries. Packages installs, removes, and re-versions the artifact
// the whole deployment runs — and enabling/disabling one is the same
// pkg_registry write — so it sits alongside Build History, which reports on
// those rebuilds and offers revert, on the owner side of the line.
function OwnerLinks({ isVisible }: { isVisible: boolean }) {
    const foregroundColor = useThemeColor('foreground')
    const orgHref = useOrgHref()
    const router = useRouter()

    if (!isVisible) return null

    return (
        <>
            <SettingsLink
                label="Packages"
                onPress={() => router.push(orgHref('settings/packages'))}
                icon={<Package size={20} color={foregroundColor} />}
            />
            <SettingsLink
                label="Build History"
                onPress={() => router.push(orgHref('settings/builds'))}
                icon={<History size={20} color={foregroundColor} />}
            />
        </>
    )
}

function SettingsGroup({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <View className="mb-5">
            <Text
                className="mb-2 text-primary text-[13px] font-semibold uppercase"
                style={{ letterSpacing: 0.5 }}
            >
                {label}
            </Text>
            <View className="rounded-xl border overflow-hidden bg-surface-secondary border-border">
                {children}
            </View>
        </View>
    )
}

function SettingsLink({
    label,
    onPress,
    icon,
}: {
    label: string
    onPress: () => void
    icon: React.ReactNode
}) {
    const mutedColor = useThemeColor('muted-foreground')

    return (
        <Pressable
            onPress={onPress}
            className="flex-row items-center justify-between px-4 py-3.5"
            style={({ pressed }) => [pressed && { opacity: 0.7 }]}
        >
            <View className="flex-row items-center gap-3">
                {icon}
                <Text className="text-foreground text-base">{label}</Text>
            </View>
            <ChevronRight size={18} color={mutedColor} />
        </Pressable>
    )
}
