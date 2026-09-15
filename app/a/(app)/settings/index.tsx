import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import {
    packageSettings,
    packageSystemSettings,
} from '@tinycld/core/lib/packages/derive-components'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useSavedServers } from '@tinycld/core/lib/use-saved-servers'
import { useRouter } from 'expo-router'
import {
    Bell,
    Bug,
    ChevronRight,
    HardDrive,
    History,
    Image,
    KeyRound,
    Package,
    ScrollText,
    Send,
    Server,
    Sliders,
    Tag,
    User,
    Users,
    Workflow,
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
                    <SettingsLink
                        label="Rules"
                        onPress={() => router.push(orgHref('settings/rules'))}
                        icon={<Workflow size={20} color={foregroundColor} />}
                    />
                </SettingsGroup>

                <DeviceSettings />

                <AdminSettings isVisible={isAdmin} isOwner={isReady && isOwner} />
            </View>
        </ScrollView>
    )
}

// Saved servers are device/connection scope — not a personal preference and not
// org administration, so neither existing group fits.
//
// Gated on having something to switch BETWEEN, the same rule the user menu's
// switcher uses. The list always contains at least the current origin (on web
// it is seeded from it), so a lone entry is just a label for where you already
// are — and it left this link opening a titled screen with a blank body, since
// ServersSection renders nothing for an empty list.
function DeviceSettings() {
    const foregroundColor = useThemeColor('foreground')
    const orgHref = useOrgHref()
    const router = useRouter()
    const { servers } = useSavedServers()

    if (servers.length < 2) return null

    return (
        <SettingsGroup label="This device">
            <SettingsLink
                label="Servers"
                onPress={() => router.push(orgHref('settings/servers'))}
                icon={<Server size={20} color={foregroundColor} />}
            />
        </SettingsGroup>
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
                    label="Branding"
                    onPress={() => router.push(orgHref('settings/organization'))}
                    icon={<Image size={20} color={foregroundColor} />}
                />
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
                    label="OAuth Clients"
                    onPress={() => router.push(orgHref('settings/oauth-clients'))}
                    icon={<KeyRound size={20} color={foregroundColor} />}
                />
                <SettingsLink
                    label="Audit Log"
                    onPress={() => router.push(orgHref('settings/audit-log'))}
                    icon={<ScrollText size={20} color={foregroundColor} />}
                />
                <OwnerLinks isVisible={isOwner} />
            </SettingsGroup>

            <SystemSettings isVisible={isOwner} />

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

// Deployment-wide configuration: these values configure the whole server, not
// one organization, which is why they are a group of their own rather than more
// rows under Organization. Owner-only, matching Packages and Build History.
//
// Package-contributed panels (manifest `systemSettings`) route through
// settings/system/<pkgSlug>/<panelSlug> — a separate tree from the org-scoped
// packageSettings above, because a package may declare the same slug in both
// (mail declares `provider` twice) and one route could not address both.
function SystemSettings({ isVisible }: { isVisible: boolean }) {
    const foregroundColor = useThemeColor('foreground')
    const orgHref = useOrgHref()
    const router = useRouter()

    if (!isVisible) return null

    return (
        <SettingsGroup label="System">
            <SettingsLink
                label="Error Reporting"
                onPress={() => router.push(orgHref('settings/error-reporting'))}
                icon={<Bug size={20} color={foregroundColor} />}
            />
            <SettingsLink
                label="Web Push"
                onPress={() => router.push(orgHref('settings/web-push'))}
                icon={<Bell size={20} color={foregroundColor} />}
            />
            <SettingsLink
                label="Mail Sending"
                onPress={() => router.push(orgHref('settings/mail-sending'))}
                icon={<Send size={20} color={foregroundColor} />}
            />
            {packageSystemSettings.map(group =>
                group.panels.map(panel => (
                    <SettingsLink
                        key={`${group.pkgSlug}:${panel.slug}`}
                        label={`${group.packageName} — ${panel.label}`}
                        onPress={() =>
                            router.push(
                                orgHref('settings/system/[...section]', {
                                    section: [group.pkgSlug, panel.slug],
                                })
                            )
                        }
                        icon={<Sliders size={20} color={foregroundColor} />}
                    />
                ))
            )}
        </SettingsGroup>
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
