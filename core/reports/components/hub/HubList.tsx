import { useAuth } from '@tinycld/core/lib/auth'
import { formatTimeAgo } from '@tinycld/core/lib/format-utils'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useRecord } from '@tinycld/core/lib/use-record'
import type { ReactNode } from 'react'
import { Pressable, Text, View } from 'react-native'
import type { HubRow, ReportsHubModel } from '../../hooks/useReportsHub'
import { parseBroken } from '../../lib/spec'
import { recentKey, useReportRecentStore } from '../../stores/report-recent-store'

const ROW_MIN_HEIGHT = 44

function OwnerName({ userId }: { userId: string }) {
    const [users] = useStore('users')
    const { record } = useRecord(users, userId)
    return (
        <Text numberOfLines={1} className="text-sm text-muted-foreground">
            {record?.name ?? ''}
        </Text>
    )
}

function BrokenBadge({ isVisible, reportId }: { isVisible: boolean; reportId: string }) {
    if (!isVisible) return null
    return (
        <View
            testID={`reports-hub-broken-${reportId}`}
            className="self-start rounded-full bg-danger-soft px-2"
        >
            <Text className="text-xs font-medium text-danger">Broken</Text>
        </View>
    )
}

function useLastRun(reportId: string): string {
    const { user } = useAuth()
    const at = useReportRecentStore(s => s.lastRun[recentKey(user.id, reportId)])
    return at ? formatTimeAgo(at) : 'Never'
}

interface RowProps {
    row: HubRow
    hub: ReportsHubModel
    showOwner: boolean
    menu: ReactNode
}

function TitleBlock({ row }: { row: HubRow }) {
    const { report } = row
    return (
        <View className="min-w-0 flex-1 gap-0.5">
            <Text
                testID="reports-hub-title"
                numberOfLines={1}
                className="text-sm font-semibold text-foreground"
            >
                {report.title}
            </Text>
            <Text numberOfLines={1} className="text-xs text-muted-foreground">
                {report.description}
            </Text>
            <BrokenBadge isVisible={parseBroken(report.broken) !== null} reportId={report.id} />
        </View>
    )
}

function OwnerCell({ isVisible, userId }: { isVisible: boolean; userId: string }) {
    if (!isVisible) return null
    return (
        <View className="w-36">
            <OwnerName userId={userId} />
        </View>
    )
}

function WideRow({ row, hub, showOwner, menu }: RowProps) {
    const lastRun = useLastRun(row.report.id)
    return (
        <Pressable
            testID={`reports-hub-row-${row.report.id}`}
            accessibilityRole="link"
            onPress={() => hub.onOpen(row.report)}
            className="flex-row items-center gap-3 border-b border-border px-4 py-2"
            style={{ minHeight: ROW_MIN_HEIGHT }}
        >
            <TitleBlock row={row} />
            <Text numberOfLines={1} className="w-40 text-sm text-muted-foreground">
                {hub.sourceLabel(row.report.source)}
            </Text>
            <OwnerCell isVisible={showOwner} userId={row.report.owner} />
            <Text numberOfLines={1} className="w-28 text-sm text-muted-foreground">
                {lastRun}
            </Text>
            {menu}
        </Pressable>
    )
}

function PhoneCard({ row, hub, menu }: RowProps) {
    const lastRun = useLastRun(row.report.id)
    return (
        <Pressable
            testID={`reports-hub-row-${row.report.id}`}
            accessibilityRole="link"
            onPress={() => hub.onOpen(row.report)}
            className="flex-row items-start gap-2 rounded-lg border border-border bg-card p-3"
        >
            <View className="min-w-0 flex-1 gap-1">
                <TitleBlock row={row} />
                <View className="flex-row gap-1">
                    <Text className="text-xs text-muted-foreground">
                        {`${hub.sourceLabel(row.report.source)} ·`}
                    </Text>
                    <OwnerName userId={row.report.owner} />
                </View>
                <Text className="text-xs text-muted-foreground">{lastRun}</Text>
            </View>
            {menu}
        </Pressable>
    )
}

function OwnerHeading({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="w-36 text-xs font-semibold uppercase text-muted-foreground">Owner</Text>
}

function HeadingRow({ isVisible, showOwner }: { isVisible: boolean; showOwner: boolean }) {
    if (!isVisible) return null
    return (
        <View
            testID="reports-hub-heading-row"
            className="flex-row gap-3 border-b border-border px-4 py-2"
        >
            <Text className="flex-1 text-xs font-semibold uppercase text-muted-foreground">
                Title
            </Text>
            <Text className="w-40 text-xs font-semibold uppercase text-muted-foreground">
                Source
            </Text>
            <OwnerHeading isVisible={showOwner} />
            <Text className="w-28 text-xs font-semibold uppercase text-muted-foreground">
                Last run
            </Text>
            <View style={{ width: ROW_MIN_HEIGHT }} />
        </View>
    )
}

interface HubListProps {
    hub: ReportsHubModel
    isPhone: boolean
    showOwner: boolean
    menuFor(row: HubRow): ReactNode
}

// Rows with a heading on a wide screen; cards on a phone.
export function HubList({ hub, isPhone, showOwner, menuFor }: HubListProps) {
    if (hub.rows.length === 0) return null
    const Row = isPhone ? PhoneCard : WideRow
    const items = hub.rows.map(row => (
        <Row key={row.report.id} row={row} hub={hub} showOwner={showOwner} menu={menuFor(row)} />
    ))
    const frame = isPhone ? 'gap-2' : 'overflow-hidden rounded-lg border border-border bg-card'
    return (
        <View testID="reports-hub-list" className={frame}>
            <HeadingRow isVisible={!isPhone} showOwner={showOwner} />
            {items}
        </View>
    )
}
