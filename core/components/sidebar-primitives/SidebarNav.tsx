import { useSafeAreaPadding } from '@tinycld/core/lib/use-safe-area'
import type { ReactNode } from 'react'
import { ScrollView, View } from 'react-native'

export function SidebarNav({ children }: { children: ReactNode }) {
    // The home-indicator inset belongs on the scroll CONTENT, not on the
    // sidebar box: as outer padding it walled off a dead strip the list could
    // never scroll into, so the last item stopped short of the bottom on every
    // gesture-nav device. Here the list scrolls the full height and the inset
    // is only reserved past the final item. Base 8 matches the horizontal pad
    // and is a minimum, not an addend — see useSafeAreaPadding.
    const { paddingBottom } = useSafeAreaPadding({ bottom: 8 })

    return (
        <View className="flex-1 bg-sidebar-background">
            <ScrollView
                className="flex-1"
                contentContainerStyle={{
                    paddingTop: 8,
                    paddingHorizontal: 8,
                    paddingBottom,
                    gap: 2,
                }}
                showsVerticalScrollIndicator={false}
            >
                {children}
            </ScrollView>
        </View>
    )
}
