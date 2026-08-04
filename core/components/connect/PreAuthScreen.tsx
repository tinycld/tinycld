import { useSafeAreaPadding } from '@tinycld/core/lib/use-safe-area'
import { ScrollView, View } from 'react-native'

// The shared scaffold for the full-bleed screens shown before a workspace
// exists: /connect (pick a server) and /pick-org (pick an organization).
//
// It exists for the safe area. Both screens previously used
// `<SafeAreaView edges={['top', 'bottom']}>`, which handles the notch in
// PORTRAIT and does nothing in landscape — where the sensor housing sits on
// one side. Their content ran under the notch, and which side broke depended
// on which way the device was turned. useSafeAreaPadding (via useDeviceInsets)
// pads only the side the housing is actually on; the other side falls back to
// the design gutter.
//
// The rule this encodes is the same one WorkspaceLayout follows: the BACKGROUND
// spans the screen edge to edge, and only CONTENT is inset. So the background
// lives on the outer View (no padding), and the insets are applied to the
// scroll content — never as padding on a coloured container, which is what
// produced the light gutter beside the workspace rail.
export function PreAuthScreen({
    children,
    gutter = 28,
    testID,
}: {
    children: React.ReactNode
    /** Minimum spacing from the edge of the app. The safe-area inset wins when
     *  it is larger — see useSafeAreaPadding. */
    gutter?: number
    testID?: string
}) {
    const padding = useSafeAreaPadding({ horizontal: gutter, top: 16, bottom: 32 })

    return (
        <View className="flex-1 bg-background" testID={testID}>
            <ScrollView
                contentContainerStyle={{ flexGrow: 1, ...padding }}
                showsVerticalScrollIndicator={false}
            >
                {children}
            </ScrollView>
        </View>
    )
}
