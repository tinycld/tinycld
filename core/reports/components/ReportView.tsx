import type { ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import type { ReactNode } from 'react'
import { View } from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { shownRunKey, type TableLayout, useTableLayout } from '../hooks/useTableLayout'
import { layoutNoticeOf } from '../lib/notices'
import { ReportControls } from './ReportControls'
import { ReportCustomizePanel } from './ReportCustomizePanel'
import { ReportDocumentHeader } from './ReportDocumentHeader'
import { REPORT_TAP_TARGET, ReportHeader } from './ReportHeader'
import { ReportNotices } from './ReportNotices'
import { EmptyBody, LoadingBody, MessageBody, ProblemsBody } from './ReportStates'
import { ReportTable } from './ReportTable'

function toolbarButton(
    key: string,
    label: string,
    testID: string,
    onPress: () => void
): ToolbarItem {
    return {
        type: 'custom',
        key,
        overflow: { label, onPress },
        element: (
            <Button
                testID={testID}
                variant="outline"
                style={{ minHeight: REPORT_TAP_TARGET }}
                onPress={onPress}
            >
                <ButtonText>{label}</ButtonText>
            </Button>
        ),
    }
}

// Every action the reader may take on this report, in the order the toolbar shows them;
// ResponsiveToolbar folds what does not fit into "⋯".
function toolbarItems(model: ReportViewModel): ToolbarItem[] {
    const items: ToolbarItem[] = []
    if (model.foldAllLabel) {
        items.push(toolbarButton('fold', model.foldAllLabel, 'report-fold-all', model.onFoldAll))
    }
    if (!model.isEditing) {
        items.push(
            toolbarButton('customize', 'Customize', 'report-customize-open', model.onOpenCustomize)
        )
    }
    if (model.onEdit && model.access.canEdit && !model.isEditing) {
        items.push(toolbarButton('edit', 'Edit report', 'report-edit-open', model.onEdit))
    }
    if (model.onShare && model.access.canShare) {
        items.push(toolbarButton('share', 'Share', 'report-share-open', model.onShare))
    }
    if (model.onSaveAs && !model.isEditing) {
        items.push(toolbarButton('save-as', 'Save as…', 'report-save-as', model.onSaveAs))
    }
    if (model.onCopyLink) {
        items.push(toolbarButton('copy-link', 'Copy link', 'report-copy-link', model.onCopyLink))
    }
    return items
}

function editAction(model: ReportViewModel) {
    if (!model.onEdit || !model.access.canEdit) return null
    return { label: 'Edit report', testID: 'report-missing-edit', onPress: model.onEdit }
}

// R8: a run refused for the reader's sort (a copied link naming a column the
// report no longer has) offers to clear it, which runs the report in its own
// order; any other refusal offers the editor.
function refusalAction(model: ReportViewModel) {
    if (!model.sortProblem) return editAction(model)
    return { label: 'Clear sort', testID: 'report-clear-sort', onPress: model.onClearSort }
}

interface BodyProps {
    model: ReportViewModel
    table: TableLayout
    /** D9: the top area that scrolls away with the rows (a phone); not given, it stays above. */
    top?: ReactNode
}

// The body of every state but the table's: each shows only in its own state.
function StateBodies({ model }: { model: ReportViewModel }) {
    const { state } = model
    const retry = { label: 'Retry', testID: 'report-retry', onPress: model.run.retry }
    const fix = refusalAction(model)
    return (
        <>
            <LoadingBody isVisible={state === 'loading'} />
            <MessageBody
                isVisible={state === 'error'}
                testID="report-error"
                message="Could not load this report"
                action={retry}
            />
            <MessageBody
                isVisible={state === 'timeout'}
                testID="report-timeout"
                message="This report took too long to run"
                action={retry}
            />
            <MessageBody
                isVisible={state === 'no-access'}
                testID="report-no-access"
                message="You no longer have access to this report"
            />
            <MessageBody
                isVisible={state === 'incomplete'}
                testID="report-incomplete"
                message="Finish the report's design to see it here"
            />
            <ProblemsBody
                isVisible={state === 'missing'}
                testID="report-missing"
                message="This report names something that no longer exists"
                problems={model.problems}
                note={model.missingNote}
                action={fix}
            />
            <ProblemsBody
                isVisible={state === 'invalid'}
                testID="report-invalid"
                message="This report cannot run"
                problems={model.problems}
                note={model.missingNote}
                action={fix}
            />
            <EmptyBody
                isVisible={state === 'empty'}
                message={model.emptyMessage}
                action={model.emptyAction}
                onAction={model.onEmptyAction}
            />
        </>
    )
}

export function ReportBody({ model, table, top }: BodyProps) {
    const states = <StateBodies model={model} />
    // D9 with F11: on a phone the list holds the top area in every state, so the
    // states are drawn inside it, under the top area; elsewhere they stand alone.
    const fixedStates = top === undefined ? states : null
    return (
        <>
            {fixedStates}
            <TableBody model={model} table={table} top={top} states={states} />
        </>
    )
}

function TableBody({ model, table, top, states }: BodyProps & { states: ReactNode }) {
    const isReady = model.state === 'ready'
    if (!isReady && top === undefined) return null
    const body = isReady ? undefined : states
    const runKey = shownRunKey(model)
    return (
        <ReportTable
            columns={model.columns}
            rows={model.rows}
            runKey={runKey}
            format={model.display}
            layout={table.layout}
            isMeasured={table.isMeasured}
            onRowWidth={table.onRowWidth}
            fit={table.fit}
            onToggleGroup={model.onToggleGroup}
            onNearEnd={model.run.loadMore}
            hasMore={model.run.hasMore}
            loadMoreFailed={model.run.loadMoreFailed}
            onRetryMore={model.run.retryLoadMore}
            sort={model.sort}
            onSortColumn={model.onSortColumn}
            onTopRowIndex={model.run.onTopRowIndex}
            top={top}
            body={body}
        />
    )
}

function ViewBody({ model, table, top }: BodyProps) {
    if (!model.isBodyShown) return null
    return <ReportBody model={model} table={table} top={top} />
}

interface ReportTopProps {
    model: ReportViewModel
    isSortMenuShown: boolean
    layoutNotice: string | undefined
    isCompact: boolean
}

// Everything above the rows: the header, the chips, the notices and the document header.
function ReportTop({ model, isSortMenuShown, layoutNotice, isCompact }: ReportTopProps) {
    return (
        <>
            <ReportHeader
                title={model.title}
                onBack={model.onBack}
                actions={toolbarItems(model)}
                isUpdating={model.isUpdating}
                isUpdatingLive={!model.isUpdatingInBuilder}
            />
            <ReportControls model={model} isSortMenuShown={isSortMenuShown} />
            <ReportNotices
                notices={model.notices}
                layoutNotice={layoutNotice}
                isLive={model.isBodyShown}
                isCompact={isCompact}
            />
            <ReportDocumentHeader
                heading={model.heading}
                title={model.title}
                periodWords={model.periodWords}
                isThousands={model.display.divideBy1000}
            />
        </>
    )
}

export function ReportView({ model, side }: { model: ReportViewModel; side?: ReactNode }) {
    const table = useTableLayout(model)
    const isSortMenuShown = table.layout.kind === 'blocks'
    // The table's layout notice joins the run's in the band, once the table is drawn.
    const layoutNotice = model.state === 'ready' ? layoutNoticeOf(table.layout) : undefined
    const isCompact = model.breakpoint === 'mobile'
    const top = (
        <ReportTop
            model={model}
            isSortMenuShown={isSortMenuShown}
            layoutNotice={layoutNotice}
            isCompact={isCompact}
        />
    )
    // D9: on a phone the top area is the list's header and scrolls away with the rows.
    // It stays there in every run state (the list then shows the state under it), so
    // a run that lands with no rows or fails does not mount it again and the focus
    // stays on the control that started it. On a wider screen it stays above the body.
    const isTopInList = isCompact && model.isBodyShown
    const fixedTop = isTopInList ? null : top
    const listTop = isTopInList ? top : undefined
    return (
        <View testID="report-screen" className="flex-1 flex-row bg-background">
            <View className="min-w-0 flex-1">
                {fixedTop}
                <ViewBody model={model} table={table} top={listTop} />
            </View>
            <ReportCustomizePanel model={model} />
            {side}
        </View>
    )
}
