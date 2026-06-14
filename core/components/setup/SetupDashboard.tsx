import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Building2, History, type LucideIcon, Package, ShieldCheck } from 'lucide-react-native'
import type PocketBase from 'pocketbase'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { DraxProvider } from 'react-native-drax'
import { BuildHistoryTab } from './BuildHistoryTab'
import { OrganizationsTab } from './OrganizationsTab'
import { PackageManager } from './PackageManager'
import { SuperAdminsTab } from './SuperAdminsTab'

type SetupTab = 'organizations' | 'packages' | 'builds' | 'super-admins'

interface NavEntry {
    tab: SetupTab
    label: string
    crumb: string
    Icon: LucideIcon
}

// Order here is the rail order. `crumb` is the topbar breadcrumb leaf shown after
// the `admin /` root (the breadcrumb is the only place a route literal appears).
const NAV: NavEntry[] = [
    { tab: 'organizations', label: 'Organizations', crumb: 'organizations', Icon: Building2 },
    { tab: 'packages', label: 'Packages', crumb: 'packages', Icon: Package },
    { tab: 'builds', label: 'Build History', crumb: 'build history', Icon: History },
    { tab: 'super-admins', label: 'Super Admins', crumb: 'super admins', Icon: ShieldCheck },
]

interface SetupDashboardProps {
    pb: PocketBase
    defaultTab?: SetupTab
}

export function SetupDashboard({ pb, defaultTab = 'packages' }: SetupDashboardProps) {
    const [activeTab, setActiveTab] = useState<SetupTab>(defaultTab)
    const crumb = NAV.find(n => n.tab === activeTab)?.crumb ?? activeTab

    return (
        // DraxProvider here (not just at the app root) because the setup
        // dashboard mounts under its own GestureHandlerRootView in the
        // pre-org/superuser flows, outside the app-root Providers tree — the
        // package reorder list needs Drax context regardless of how setup was
        // reached.
        <DraxProvider style={{ flex: 1 }}>
            <View className="flex-1 flex-row">
                <SetupRail activeTab={activeTab} onSelect={setActiveTab} />

                <View className="flex-1">
                    <SetupTopBar crumb={crumb} />
                    <ScrollView className="flex-1">
                        <View className="w-full self-center p-8 gap-6" style={{ maxWidth: 1040 }}>
                            <OrganizationsTab isVisible={activeTab === 'organizations'} pb={pb} />
                            <PackagesTab isVisible={activeTab === 'packages'} pb={pb} />
                            <BuildHistoryTab isVisible={activeTab === 'builds'} pb={pb} />
                            <SuperAdminsTab isVisible={activeTab === 'super-admins'} pb={pb} />
                        </View>
                    </ScrollView>
                </View>
            </View>
        </DraxProvider>
    )
}

function SetupRail({
    activeTab,
    onSelect,
}: {
    activeTab: SetupTab
    onSelect: (tab: SetupTab) => void
}) {
    const railBg = useThemeColor('rail-background')
    const railText = useThemeColor('rail-text')

    return (
        <View className="w-60 py-6 px-3 gap-1" style={{ backgroundColor: railBg }}>
            <View className="flex-row items-center gap-3 px-3 pb-5">
                <View className="w-9 h-9 rounded-xl items-center justify-center bg-primary">
                    <Text
                        className="text-primary-foreground"
                        style={{ fontFamily: 'monospace', fontWeight: '700', fontSize: 15 }}
                    >
                        t/
                    </Text>
                </View>
                <View>
                    <Text style={{ color: railText, fontSize: 15, fontWeight: '600' }}>
                        tinycld
                    </Text>
                    <Text
                        style={{
                            color: railText,
                            opacity: 0.55,
                            fontSize: 10,
                            letterSpacing: 1,
                            textTransform: 'uppercase',
                        }}
                    >
                        setup console
                    </Text>
                </View>
            </View>

            <Text
                className="px-3 pb-2"
                style={{
                    color: railText,
                    opacity: 0.5,
                    fontSize: 10,
                    letterSpacing: 1.2,
                    textTransform: 'uppercase',
                }}
            >
                Workspace
            </Text>

            {NAV.map(entry => (
                <SetupRailItem
                    key={entry.tab}
                    label={entry.label}
                    Icon={entry.Icon}
                    isActive={activeTab === entry.tab}
                    onPress={() => onSelect(entry.tab)}
                />
            ))}
        </View>
    )
}

function SetupRailItem({
    label,
    Icon,
    isActive,
    onPress,
}: {
    label: string
    Icon: LucideIcon
    isActive: boolean
    onPress: () => void
}) {
    const railText = useThemeColor('rail-text')
    const railActive = useThemeColor('rail-active-text')
    const indicatorColor = useThemeColor('active-indicator')
    const textColor = isActive ? railActive : railText

    return (
        <View className="relative">
            {isActive && (
                <View
                    className="absolute w-1 h-5 rounded-sm top-2.5"
                    style={{ backgroundColor: indicatorColor, left: -4 }}
                />
            )}
            <Pressable
                onPress={onPress}
                accessibilityRole="tab"
                accessibilityState={{ selected: isActive }}
                className="flex-row items-center gap-3 px-3 py-2.5 rounded-xl"
                style={isActive ? { backgroundColor: `${indicatorColor}22` } : undefined}
            >
                <Icon size={18} color={textColor} />
                <Text
                    style={{ color: textColor, fontSize: 14, fontWeight: isActive ? '600' : '500' }}
                >
                    {label}
                </Text>
            </Pressable>
        </View>
    )
}

function SetupTopBar({ crumb }: { crumb: string }) {
    const mutedColor = useThemeColor('muted-foreground')
    const fgColor = useThemeColor('foreground')
    const successColor = useThemeColor('success')

    return (
        <View className="flex-row items-center px-8 py-4 border-b border-border">
            <Text style={{ color: mutedColor, fontSize: 13 }}>
                admin <Text style={{ color: mutedColor, opacity: 0.5 }}>/</Text>{' '}
                <Text style={{ color: fgColor }}>{crumb}</Text>
            </Text>
            <View className="flex-1" />
            <View className="flex-row items-center gap-2 px-3 py-1.5 rounded-full border border-border">
                <View
                    className="w-1.5 h-1.5 rounded-full"
                    style={{ backgroundColor: successColor }}
                />
                <Text style={{ color: mutedColor, fontSize: 12 }}>connected</Text>
            </View>
        </View>
    )
}

function PackagesTab({ isVisible, pb }: { isVisible: boolean; pb: PocketBase }) {
    if (!isVisible) return null
    return <PackageManager pb={pb} isVisible={isVisible} />
}
