import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { Menu } from '@tinycld/core/ui/menu'
import { EllipsisVertical } from 'lucide-react-native'
import { type ReactNode, useState } from 'react'
import { type GestureResponderEvent, Pressable } from 'react-native'
import type { HubActions } from '../../hooks/useHubActions'
import type { HubRow } from '../../hooks/useReportsHub'
import { canCopyReport } from '../../lib/spec'
import { useReportUIStore } from '../../stores/report-ui-store'
import { REPORT_TAP_TARGET } from '../ReportHeader'

function swallow(event: GestureResponderEvent) {
    event.stopPropagation()
}

// The menu's content is portaled, but React still bubbles its presses to the row's
// Pressable, which would open the report behind the action just chosen.
function PressBoundary({ children }: { children: ReactNode }) {
    return (
        <Pressable accessible={false} focusable={false} onPress={swallow}>
            {children}
        </Pressable>
    )
}

interface MenuItemSpec {
    id: string
    label: string
    isVisible: boolean
    /** A write: held while another write is pending, so a second press adds no copy. */
    isWrite?: boolean
    isDestructive?: boolean
    onSelect: () => void
}

interface HubRowMenuProps {
    row: HubRow
    actions: HubActions
    onOpen: () => void
}

// Each action shows only when the reader may take it (R13); a shipped report offers
// Unpublish instead of Delete.
export function HubRowMenu({ row, actions, onOpen }: HubRowMenuProps) {
    const color = useThemeColor('muted-foreground')
    const openShare = useReportUIStore(s => s.openShare)
    // The delete confirm is a modal toggle: local UI state.
    const [isConfirmingDelete, setConfirmingDelete] = useState(false)
    const { report, access } = row
    const specs: MenuItemSpec[] = [
        { id: 'open', label: 'Open', isVisible: true, onSelect: onOpen },
        {
            id: 'duplicate',
            label: 'Duplicate',
            isVisible: canCopyReport(report),
            isWrite: true,
            onSelect: () => actions.duplicate(report),
        },
        {
            id: 'share',
            label: 'Share',
            isVisible: access.canShare,
            onSelect: () => openShare(report.id),
        },
        {
            id: 'publish',
            label: 'Publish to everyone',
            isVisible: access.canPublish && !report.published,
            isWrite: true,
            onSelect: () => actions.setPublished(report, true),
        },
        {
            id: 'unpublish',
            label: 'Unpublish',
            isVisible: access.canPublish && report.published,
            isWrite: true,
            onSelect: () => actions.setPublished(report, false),
        },
        {
            id: 'delete',
            label: 'Delete',
            isVisible: access.canDelete,
            isWrite: true,
            isDestructive: true,
            onSelect: () => setConfirmingDelete(true),
        },
    ]
    const items = specs
        .filter(spec => spec.isVisible)
        .map(spec => (
            <Menu.Item
                key={spec.id}
                testID={`reports-hub-action-${spec.id}`}
                label={spec.label}
                isDestructive={spec.isDestructive}
                isDisabled={spec.isWrite === true && actions.isPending}
                onSelect={spec.onSelect}
            />
        ))
    const trigger = (
        <Pressable
            testID={`reports-hub-row-menu-${report.id}`}
            accessibilityRole="button"
            accessibilityLabel={`Actions for ${report.title}`}
            onPress={swallow}
            className="items-center justify-center"
            style={{ width: REPORT_TAP_TARGET, minHeight: REPORT_TAP_TARGET }}
        >
            <EllipsisVertical size={18} color={color} />
        </Pressable>
    )
    const onConfirmDelete = () => {
        setConfirmingDelete(false)
        actions.remove(report)
    }
    return (
        <PressBoundary>
            <Menu
                title={report.title}
                placement="bottom-end"
                sheetSide="bottom"
                trigger={trigger}
                testID="reports-hub-row-actions"
            >
                {items}
            </Menu>
            <ConfirmDialog
                isOpen={isConfirmingDelete}
                onClose={() => setConfirmingDelete(false)}
                onConfirm={onConfirmDelete}
                title={`Delete “${report.title}”?`}
                message="Everyone it is shared with loses it. This cannot be undone."
                confirmLabel="Delete"
                confirmTestID="reports-hub-delete-confirm"
                isSubmitting={actions.isPending}
                isDestructive
            />
        </PressBoundary>
    )
}
