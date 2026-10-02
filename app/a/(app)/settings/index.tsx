import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import {
    packageAccountSettings,
    packageSettings,
    packageSystemSettings,
} from '@tinycld/core/lib/packages/derive-components'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { isManagedPrefix, useManagedSettingPrefixes } from '@tinycld/core/lib/use-managed-settings'
import { useRouter } from 'expo-router'
import {
    Bell,
    Bug,
    Cable,
    ChevronRight,
    DatabaseBackup,
    HardDrive,
    History,
    Image,
    Info,
    KeyRound,
    Package,
    Palette,
    ScrollText,
    Send,
    Server,
    ShieldAlert,
    Sliders,
    Tag,
    User,
    Users,
    UsersRound,
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
                    {ACCOUNT_LINKS.map(({ label, route, Icon }) => (
                        <SettingsLink
                            key={route}
                            label={label}
                            onPress={() => router.push(orgHref(route))}
                            icon={<Icon size={20} color={foregroundColor} />}
                        />
                    ))}
                    <PackageAccountLinks />
                </SettingsGroup>

                <SettingsGroup label="This device">
                    <SettingsLink
                        label="Servers"
                        onPress={() => router.push(orgHref('settings/servers'))}
                        icon={<Server size={20} color={foregroundColor} />}
                    />
                </SettingsGroup>

                <AdminSettings isVisible={isAdmin} isOwner={isReady && isOwner} />

                <SettingsGroup label="About">
                    <SettingsLink
                        label="About"
                        onPress={() => router.push(orgHref('settings/about'))}
                        icon={<Info size={20} color={foregroundColor} />}
                    />
                </SettingsGroup>
            </View>
        </ScrollView>
    )
}

// Everything here acts on the signed-in user's own account, so every role
// sees the whole group.
const ACCOUNT_LINKS = [
    { label: 'Profile', route: 'settings/profile', Icon: User },
    { label: 'Appearance', route: 'settings/appearance', Icon: Palette },
    { label: 'Notifications', route: 'settings/notifications', Icon: Bell },
    { label: 'Rules', route: 'settings/rules', Icon: Workflow },
    { label: 'Connected apps', route: 'settings/connected-apps', Icon: Cable },
    { label: 'Account access', route: 'settings/account-access', Icon: ShieldAlert },
] as const

// Per-user panels packages contribute (manifest `accountSettings`), listed
// after core's own account links.
function PackageAccountLinks() {
    const foregroundColor = useThemeColor('foreground')
    const orgHref = useOrgHref()
    const router = useRouter()

    const panels = packageAccountSettings.flatMap(group =>
        group.panels.map(panel => ({ group, panel, Icon: getIcon(group.icon ?? '') }))
    )

    return panels.map(({ group, panel, Icon }) => (
        <SettingsLink
            key={`${group.pkgSlug}:${panel.slug}`}
            label={panel.label}
            onPress={() =>
                router.push(
                    orgHref('settings/account/[...section]', {
                        section: [group.pkgSlug, panel.slug],
                    })
                )
            }
            icon={<Icon size={20} color={foregroundColor} />}
        />
    ))
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
                    label="Groups"
                    onPress={() => router.push(orgHref('settings/groups'))}
                    icon={<UsersRound size={20} color={foregroundColor} />}
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
                <SettingsLink
                    label="Backups"
                    onPress={() => router.push(orgHref('settings/backups'))}
                    icon={<DatabaseBackup size={20} color={foregroundColor} />}
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
// The three core system screens, each tagged with the system_settings namespace
// it edits. The tag is what lets a deployment whose operator owns that namespace
// hide the screen: it could not save there, because those values never reach
// this deployment's database.
const CORE_SYSTEM_LINKS = [
    {
        label: 'Error Reporting',
        route: 'settings/error-reporting',
        keyPrefix: 'sentry.',
        Icon: Bug,
    },
    { label: 'Web Push', route: 'settings/web-push', keyPrefix: 'vapid.', Icon: Bell },
    { label: 'Mail Sending', route: 'settings/mail-sending', keyPrefix: 'mail.', Icon: Send },
] as const

function SystemSettings({ isVisible }: { isVisible: boolean }) {
    const foregroundColor = useThemeColor('foreground')
    const orgHref = useOrgHref()
    const router = useRouter()
    const managed = useManagedSettingPrefixes()

    const coreLinks = CORE_SYSTEM_LINKS.filter(link => !isManagedPrefix(managed, link.keyPrefix))
    // A package panel is hidden by the namespace ITS OWN manifest declares, so
    // core filters it without knowing which package it belongs to. A panel that
    // declares no namespace is never hidden.
    const packagePanels = packageSystemSettings.flatMap(group =>
        group.panels
            .filter(panel => !isManagedPrefix(managed, panel.keyPrefix))
            .map(panel => ({ group, panel }))
    )

    // With every entry managed there is nothing left to administer here, and a
    // bare "System" heading over an empty list is the dead end this group would
    // otherwise become.
    if (!isVisible || coreLinks.length + packagePanels.length === 0) return null

    return (
        <SettingsGroup label="System">
            {coreLinks.map(({ label, route, Icon }) => (
                <SettingsLink
                    key={route}
                    label={label}
                    onPress={() => router.push(orgHref(route))}
                    icon={<Icon size={20} color={foregroundColor} />}
                />
            ))}
            {packagePanels.map(({ group, panel }) => (
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
            ))}
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
