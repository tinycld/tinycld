import { SortableDragHandle, SortableList } from '@tinycld/core/components/SortableList'
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import type { PackageManifest } from '@tinycld/core/lib/packages/types'
import { useAccessiblePackages } from '@tinycld/core/lib/use-accessible-packages'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useUserPreference } from '@tinycld/core/lib/use-user-preference'
import { RotateCcw } from 'lucide-react-native'
import { useCallback, useMemo } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'

function deriveOrder(packages: PackageManifest[], savedOrder: string[]): string[] {
    if (!savedOrder.length) {
        return [...packages]
            .sort((a, b) => (a.nav?.order ?? 99) - (b.nav?.order ?? 99))
            .map(a => a.slug)
    }
    const pkgSlugs = new Set(packages.map(a => a.slug))
    const ordered = savedOrder.filter(slug => pkgSlugs.has(slug))
    const missing = [...packages]
        .filter(a => !savedOrder.includes(a.slug))
        .sort((a, b) => (a.nav?.order ?? 99) - (b.nav?.order ?? 99))
        .map(a => a.slug)
    return [...ordered, ...missing]
}

export function NavigationSection() {
    const foregroundColor = useThemeColor('foreground')
    const surfaceBg = useThemeColor('surface-secondary')
    const packages = useAccessiblePackages()
    const [savedOrder, setSavedOrder] = useUserPreference('core', 'pkg_order', [] as string[])
    const localOrder = useMemo(() => deriveOrder(packages, savedOrder), [packages, savedOrder])

    const pkgMap = new Map(packages.map(a => [a.slug, a]))
    const isCustomized = savedOrder.length > 0

    const handleDragEnd = useCallback(
        (data: string[]) => {
            setSavedOrder(data)
        },
        [setSavedOrder]
    )

    function resetOrder() {
        setSavedOrder([] as string[])
    }

    function renderItem({ item }: { item: string; index: number }) {
        const pkg = pkgMap.get(item)
        if (!pkg) return null
        const Icon = getIcon(pkg.nav?.icon ?? '')

        return (
            <View
                className="flex-row items-center justify-between px-4 py-3.5 border-border"
                style={{
                    borderBottomWidth: StyleSheet.hairlineWidth,
                    backgroundColor: surfaceBg,
                }}
            >
                <View className="flex-row items-center gap-3">
                    <SortableDragHandle />
                    <Icon size={20} color={foregroundColor} />
                    <Text className="text-base text-foreground">{pkg.nav?.label}</Text>
                </View>
            </View>
        )
    }

    const keyExtractor = useCallback((slug: string) => slug, [])

    return (
        <View className="gap-3">
            <Text className="text-xl font-bold text-foreground">Navigation</Text>

            <Text className="text-[13px] text-muted-foreground">
                Drag to reorder your apps. The order is reflected in the sidebar and mobile tab bar.
            </Text>

            <View className="rounded-xl border border-border overflow-hidden">
                <SortableList
                    data={localOrder}
                    keyExtractor={keyExtractor}
                    onReorder={handleDragEnd}
                    renderItem={renderItem}
                />
            </View>

            <ResetButton isVisible={isCustomized} onPress={resetOrder} />
        </View>
    )
}

function ResetButton({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    const mutedColor = useThemeColor('muted-foreground')

    if (!isVisible) return null

    return (
        <Pressable
            onPress={onPress}
            className="flex-row items-center gap-1.5 px-3 py-2 rounded-lg border border-border self-start"
        >
            <RotateCcw size={14} color={mutedColor} />
            <Text className="text-foreground">Reset to Default</Text>
        </Pressable>
    )
}
