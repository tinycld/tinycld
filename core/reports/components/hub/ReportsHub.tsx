import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { LoadingState } from '@tinycld/core/components/LoadingState'
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { type ReactNode, useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { useHubActions } from '../../hooks/useHubActions'
import {
    HUB_TABS,
    type HubRow,
    type HubTab,
    type ReportsHubModel,
    useReportsHub,
} from '../../hooks/useReportsHub'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { MessageBody } from '../ReportStates'
import { ReportShareDialog } from '../share/ReportShareDialog'
import { HubList } from './HubList'
import { HubRowMenu } from './HubRowMenu'
import { NewReportDialog } from './NewReportDialog'

const EMPTY: Record<HubTab, string> = {
    mine: 'You have no reports yet. Use New report to make one.',
    shared: 'No one has shared a report with you.',
    published: 'No reports are published.',
}

function TabPill({
    item,
    isSelected,
    onChange,
}: {
    item: (typeof HUB_TABS)[number]
    isSelected: boolean
    onChange: (tab: HubTab) => void
}) {
    const pillClass = isSelected ? 'border-primary bg-primary' : 'border-border'
    const textClass = isSelected ? 'text-primary-foreground' : 'text-foreground'
    return (
        <Pressable
            testID={`reports-hub-tab-${item.id}`}
            accessibilityRole="tab"
            aria-selected={isSelected}
            onPress={() => onChange(item.id)}
            className={`justify-center rounded-full border px-3 ${pillClass}`}
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className={`text-sm font-medium ${textClass}`}>{item.label}</Text>
        </Pressable>
    )
}

function Tabs({ tab, onChange }: { tab: HubTab; onChange: (tab: HubTab) => void }) {
    const pills = HUB_TABS.map(item => (
        <TabPill key={item.id} item={item} isSelected={item.id === tab} onChange={onChange} />
    ))
    return (
        <ScrollView
            horizontal
            showsHorizontalScrollIndicator={false}
            contentContainerClassName="gap-2"
            accessibilityRole="tablist"
        >
            {pills}
        </ScrollView>
    )
}

function Loading({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="reports-hub-loading">
            <LoadingState />
        </View>
    )
}

interface ReportsHubViewProps {
    hub: ReportsHubModel
    menuFor(row: HubRow): ReactNode
    banner: ReactNode
}

export function ReportsHubView({ hub, menuFor, banner }: ReportsHubViewProps) {
    const breakpoint = useBreakpoint()
    // The New report dialog is a modal toggle: local UI state.
    const [isChoosing, setChoosing] = useState(false)
    const isEmpty = hub.state === 'ready' && hub.rows.length === 0
    const retry = { label: 'Retry', testID: 'reports-hub-retry', onPress: hub.retry }
    const onPick = (key: string) => {
        setChoosing(false)
        hub.onNew(key)
    }
    return (
        <ScrollView
            testID="reports-hub"
            className="flex-1 bg-background"
            contentContainerClassName="w-full gap-4 p-4"
        >
            <View className="flex-row items-center justify-between gap-2">
                <View className="flex-row items-center gap-2">
                    <Text
                        testID="reports-hub-heading"
                        className="text-2xl font-semibold text-foreground"
                    >
                        Reports
                    </Text>
                    <HelpIcon topic="core:reports" />
                </View>
                <Button
                    testID="reports-hub-new"
                    onPress={() => setChoosing(true)}
                    isDisabled={!hub.isCatalogReady}
                >
                    <ButtonText>New report</ButtonText>
                </Button>
            </View>
            {banner}
            <Tabs tab={hub.tab} onChange={hub.setTab} />
            <Loading isVisible={hub.state === 'loading'} />
            <MessageBody
                isVisible={hub.state === 'error'}
                testID="reports-hub-error"
                message="Could not load reports"
                action={retry}
            />
            <MessageBody isVisible={isEmpty} testID="reports-hub-empty" message={EMPTY[hub.tab]} />
            <HubList
                hub={hub}
                isPhone={breakpoint === 'mobile'}
                showOwner={breakpoint === 'desktop'}
                menuFor={menuFor}
            />
            <NewReportDialog
                isOpen={isChoosing}
                sources={hub.sources}
                onClose={() => setChoosing(false)}
                onPick={onPick}
            />
        </ScrollView>
    )
}

function PackageBanner({ hub }: { hub: ReportsHubModel }) {
    if (!hub.packageSlug) return null
    return (
        <View
            testID="reports-hub-filter"
            className="flex-row flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-surface-secondary px-3 py-2"
        >
            <Text className="text-sm text-foreground">{`Showing reports from ${hub.packageLabel}`}</Text>
            <Button
                testID="reports-hub-show-all"
                variant="outline"
                size="sm"
                onPress={hub.onShowAll}
            >
                <ButtonText>Show all reports</ButtonText>
            </Button>
        </View>
    )
}

export function ReportsHub() {
    const hub = useReportsHub()
    const actions = useHubActions()
    const menuFor = (row: HubRow) => (
        <HubRowMenu row={row} actions={actions} onOpen={() => hub.onOpen(row.report)} />
    )
    return (
        <>
            <ReportsHubView hub={hub} menuFor={menuFor} banner={<PackageBanner hub={hub} />} />
            <ReportShareDialog />
        </>
    )
}
