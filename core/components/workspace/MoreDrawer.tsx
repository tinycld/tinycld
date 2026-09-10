import { useAuth } from '@tinycld/core/lib/auth'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useSortedPackages } from '@tinycld/core/lib/use-sorted-packages'
import { Sheet } from '@tinycld/core/ui/sheet'
import { useRouter } from 'expo-router'
import { Bell, LogOut, Settings, User, X } from 'lucide-react-native'
import { useCallback } from 'react'
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { MAX_VISIBLE_TABS } from './MobileTabBar'
import { getIcon } from './package-icon-map'
import { ServersDrawerSection } from './ServersDrawerSection'

export function MoreDrawer() {
    const isMoreOpen = useWorkspaceStore(s => s.isMoreOpen)
    const setMoreOpen = useWorkspaceStore(s => s.setMoreOpen)
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const setNotificationsOpen = useWorkspaceStore(s => s.setNotificationsOpen)
    const railText = useThemeColor('rail-text')
    const railActive = useThemeColor('rail-active-text')
    const borderColor = useThemeColor('border')
    const router = useRouter()
    const orgHref = useOrgHref()
    const { user, logout } = useAuth()
    const sorted = useSortedPackages()
    const textColor = railText
    const activeColor = railActive

    const overflowPkgs = sorted.length > MAX_VISIBLE_TABS ? sorted.slice(MAX_VISIBLE_TABS) : []

    const close = useCallback(() => setMoreOpen(false), [setMoreOpen])

    const handleNav = (action: () => void) => {
        action()
        close()
    }

    return (
        <Sheet isOpen={isMoreOpen} onClose={close} surface="rail-background">
            <View className="flex-row items-center justify-between px-5 pb-3">
                <View className="flex-row items-center gap-3">
                    <View
                        className="w-9 h-9 rounded-full justify-center items-center"
                        style={{ backgroundColor: 'rgba(255,255,255,0.15)' }}
                    >
                        <User size={18} color={activeColor} />
                    </View>
                    <Text className="text-[17px] font-semibold" style={{ color: activeColor }}>
                        {user.name}
                    </Text>
                </View>
                <Pressable onPress={close} hitSlop={12}>
                    <X size={20} color={textColor} />
                </Pressable>
            </View>

            {/* Scrollable because the sheet caps its height at 85% of the
                screen and has no internal scroller — content past the cap is
                silently CLIPPED, and it clips from the bottom, where Sign out and
                the overflow packages live. With up to 10 saved servers the content
                exceeds the cap on a typical phone, which would cost the user their
                Sign out row. Fixed here rather than in Sheet: its other
                callers have their own content strategies. */}
            <ScrollView
                contentContainerStyle={{ paddingHorizontal: 8, paddingBottom: 16 }}
                showsVerticalScrollIndicator={false}
                bounces={false}
            >
                <Pressable
                    className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
                    onPress={() => {
                        close()
                        setNotificationsOpen(true)
                    }}
                >
                    <Bell size={20} color={textColor} />
                    <Text className="text-base font-medium" style={{ color: textColor }}>
                        Notifications
                    </Text>
                </Pressable>

                {/* The settings HUB, not the personal page. MobileTabBar has no
                    settings entry, so this row is the ONLY way into settings on a
                    phone — pointing it at a single section left Rules, Members and
                    every other page unreachable there. */}
                <Pressable
                    className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
                    onPress={() => handleNav(() => router.push(orgHref('settings')))}
                >
                    <Settings size={20} color={textColor} />
                    <Text className="text-base font-medium" style={{ color: textColor }}>
                        Settings
                    </Text>
                </Pressable>

                {/* The saved-server switcher — the phone counterpart to
                    UserMenu's ServersSection. */}
                <ServersDrawerSection onNavigate={handleNav} />

                <View
                    className="my-2 mx-3"
                    style={{
                        height: StyleSheet.hairlineWidth,
                        backgroundColor: borderColor,
                    }}
                />

                <Pressable
                    className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
                    onPress={() => handleNav(logout)}
                >
                    <LogOut size={20} color={textColor} />
                    <Text className="text-base font-medium" style={{ color: textColor }}>
                        Sign out
                    </Text>
                </Pressable>

                {overflowPkgs.length > 0 ? (
                    <>
                        <View
                            className="my-2 mx-3"
                            style={{
                                height: StyleSheet.hairlineWidth,
                                backgroundColor: borderColor,
                            }}
                        />
                        {overflowPkgs.map(pkg => {
                            const Icon = getIcon(pkg.nav?.icon ?? '')
                            const isActive = activePkgSlug === pkg.slug
                            const color = isActive ? activeColor : textColor
                            return (
                                <Pressable
                                    key={pkg.slug}
                                    testID={`nav-${pkg.slug}`}
                                    className="flex-row items-center gap-3.5 px-4 py-3.5 rounded-lg"
                                    onPress={() =>
                                        handleNav(() => router.push(orgHref(pkg.slug as never)))
                                    }
                                >
                                    <Icon size={20} color={color} />
                                    <Text className="text-base font-medium" style={{ color }}>
                                        {pkg.nav?.label}
                                    </Text>
                                </Pressable>
                            )
                        })}
                    </>
                ) : null}
            </ScrollView>
        </Sheet>
    )
}
