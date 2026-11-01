import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import type { ReactNode } from 'react'
import { View } from 'react-native'
import type { ReportViewModel } from '../hooks/useReportView'
import { ReportDocumentHeader } from './ReportDocumentHeader'
import { REPORT_TAP_TARGET, ReportHeader } from './ReportHeader'
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
    if (model.onEdit && model.access.canEdit && !model.isEditing) {
        items.push(toolbarButton('edit', 'Edit report', 'report-edit-open', model.onEdit))
    }
    return items
}

function editAction(model: ReportViewModel) {
    if (!model.onEdit || !model.access.canEdit) return null
    return { label: 'Edit report', testID: 'report-missing-edit', onPress: model.onEdit }
}

export function ReportBody({ model }: { model: ReportViewModel }) {
    const { state } = model
    const retry = { label: 'Retry', testID: 'report-retry', onPress: model.run.retry }
    const edit = editAction(model)
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
                action={edit}
            />
            <ProblemsBody
                isVisible={state === 'invalid'}
                testID="report-invalid"
                message="This report cannot run"
                problems={model.problems}
                action={edit}
            />
            <EmptyBody
                isVisible={state === 'empty'}
                message={model.emptyMessage}
                action={model.emptyAction}
                onAction={model.onEmptyAction}
            />
            <TableBody model={model} />
        </>
    )
}

function TableBody({ model }: { model: ReportViewModel }) {
    if (model.state !== 'ready') return null
    return (
        <ReportTable
            columns={model.columns}
            rows={model.rows}
            format={model.display}
            breakpoint={model.breakpoint}
            onToggleGroup={model.onToggleGroup}
            onNearEnd={model.run.loadMore}
            isLoadingMore={model.run.isLoadingMore}
        />
    )
}

function ViewBody({ model }: { model: ReportViewModel }) {
    if (!model.isBodyShown) return null
    return <ReportBody model={model} />
}

export function ReportView({ model, side }: { model: ReportViewModel; side?: ReactNode }) {
    const toolbar = <ResponsiveToolbar items={toolbarItems(model)} />
    return (
        <View testID="report-screen" className="flex-1 flex-row bg-background">
            <View className="min-w-0 flex-1">
                <ReportHeader title={model.title} onBack={model.onBack} toolbar={toolbar} />
                <ReportDocumentHeader
                    orgName={model.orgName}
                    title={model.title}
                    periodWords={model.periodWords}
                    isThousands={model.display.divideBy1000}
                />
                <ViewBody model={model} />
            </View>
            {side}
        </View>
    )
}
