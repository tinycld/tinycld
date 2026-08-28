import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { RulesPanel } from '@tinycld/core/components/rules/RulesPanel'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft, Workflow } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'

type Segment = 'personal' | 'org'

export default function RulesSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const foregroundColor = useThemeColor('foreground')
    const { isAdmin } = useCurrentRole()
    const [segment, setSegment] = useState<Segment>('personal')

    return (
        // RulesPanel's SortableList (drag-to-reorder rows) needs a
        // GestureHandlerRootView ancestor — same requirement as
        // personal.tsx's NavigationSection.
        <GestureHandlerRootView className="flex-1">
            <DocumentTitle pkg="Settings" title="Rules" />
            <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
                <View className="p-5 max-w-[600px] gap-5 w-full">
                    <View className="flex-row gap-3 items-center">
                        <Pressable onPress={navigateBack}>
                            <ArrowLeft size={24} color={foregroundColor} />
                        </Pressable>
                        <Workflow size={24} color={foregroundColor} />
                        <Text className="text-foreground text-[22px] font-bold flex-1">Rules</Text>
                        <HelpIcon topic="core:rules" />
                    </View>

                    <SegmentPicker segment={segment} onSelect={setSegment} />

                    <RulesBody segment={segment} isAdmin={isAdmin} />
                </View>
            </ScrollView>
        </GestureHandlerRootView>
    )
}

function SegmentPicker({
    segment,
    onSelect,
}: {
    segment: Segment
    onSelect: (segment: Segment) => void
}) {
    return (
        <View className="flex-row gap-1.5">
            <SegmentTab
                label="My rules"
                isActive={segment === 'personal'}
                onPress={() => onSelect('personal')}
            />
            <SegmentTab
                label="Organization"
                isActive={segment === 'org'}
                onPress={() => onSelect('org')}
            />
        </View>
    )
}

function SegmentTab({
    label,
    isActive,
    onPress,
}: {
    label: string
    isActive: boolean
    onPress: () => void
}) {
    return (
        <Pressable
            onPress={onPress}
            className={`px-3 py-1.5 rounded-md ${isActive ? 'bg-primary' : 'border border-border'}`}
        >
            <Text className={`text-sm ${isActive ? 'text-primary-foreground' : 'text-primary'}`}>
                {label}
            </Text>
        </Pressable>
    )
}

// Non-admins get the read-only org list (canEdit={isAdmin}) rather than a
// hidden segment — RLS already permits every member to read org rules, and
// seeing what an admin has automated is useful even without edit rights.
function RulesBody({ segment, isAdmin }: { segment: Segment; isAdmin: boolean }) {
    if (segment === 'personal') {
        return <RulesPanel scope="personal" canEdit />
    }
    return <RulesPanel scope="org" canEdit={isAdmin} />
}
