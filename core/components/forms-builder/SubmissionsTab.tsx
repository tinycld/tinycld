import {
    ActionResultsList,
    parseResults,
    RunError,
} from '@tinycld/core/components/rules/RunHistory'
import { downloadFile } from '@tinycld/core/file-viewer/file-url'
import { formatTimeAgo } from '@tinycld/core/lib/format-utils'
import type { FormDefinition } from '@tinycld/core/lib/forms/definition'
import {
    deliveryStatus,
    newestRun,
    type RunLike,
    runSucceeded,
} from '@tinycld/core/lib/web-forms/delivery-status'
import { answersOf, previewOf } from '@tinycld/core/lib/web-forms/submission-answers'
import { useDestinations } from '@tinycld/core/lib/web-forms/use-destinations'
import {
    type SubmissionRow,
    useDeleteSubmission,
    useRetry,
    useRunsBySubmission,
    useSubmissionFiles,
    useSubmissionRuns,
    useSubmissions,
} from '@tinycld/core/lib/web-forms/use-submissions'
import type { Rules } from '@tinycld/core/types/pbSchema'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ConfirmDialog } from '@tinycld/core/ui/ConfirmDialog'
import { Dialog } from '@tinycld/core/ui/dialog'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'

interface SubmissionsTabProps {
    formId: string
    definition: FormDefinition
    isVisible: boolean
}

// Mounted only while the tab shows, so the submissions query runs only then.
export function SubmissionsTab({ isVisible, ...rest }: SubmissionsTabProps) {
    if (!isVisible) return null
    return <SubmissionsPanel {...rest} />
}

// A page of the list; "Show more" adds another.
const PAGE_SIZE = 50

function useSubmissionsPanel(formId: string) {
    // How many pages are shown, and which submission's detail is open: local
    // to this panel.
    const [pages, setPages] = useState(1)
    const { submissions, hasMore, isReady } = useSubmissions(formId, pages * PAGE_SIZE)
    const runsBySubmission = useRunsBySubmission(submissions.map(s => s.id))
    const { rules } = useDestinations(formId)
    const [openId, setOpenId] = useState<string | null>(null)
    const open = submissions.find(s => s.id === openId)
    return {
        submissions,
        hasMore,
        showMore: () => setPages(n => n + 1),
        runsBySubmission,
        isReady,
        destinations: rules,
        destinationIds: rules.map(r => r.id),
        open,
        setOpenId,
        close: () => setOpenId(null),
    }
}

function SubmissionsPanel({ formId, definition }: Omit<SubmissionsTabProps, 'isVisible'>) {
    const p = useSubmissionsPanel(formId)
    return (
        <View className="gap-3 max-w-2xl" testID="web-form-submissions">
            <Text className="text-foreground text-lg font-semibold">Submissions</Text>
            <SubmissionsEmpty isVisible={p.isReady && p.submissions.length === 0} />
            <SubmissionList
                submissions={p.submissions}
                runsBySubmission={p.runsBySubmission}
                destinationIds={p.destinationIds}
                definition={definition}
                onOpen={p.setOpenId}
            />
            <ShowMore isVisible={p.hasMore} onPress={p.showMore} />
            <SubmissionDetail
                submission={p.open}
                definition={definition}
                destinations={p.destinations}
                onClose={p.close}
            />
        </View>
    )
}

function SubmissionsEmpty({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-sm">No submissions yet.</Text>
}

function ShowMore({ isVisible, onPress }: { isVisible: boolean; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Button
            variant="outline"
            size="sm"
            className="self-start"
            onPress={onPress}
            testID="web-form-submissions-more"
        >
            <ButtonText>Show more</ButtonText>
        </Button>
    )
}

type RunsBySubmission = ReturnType<typeof useRunsBySubmission>

function SubmissionList({
    submissions,
    runsBySubmission,
    destinationIds,
    definition,
    onOpen,
}: {
    submissions: SubmissionRow[]
    runsBySubmission: RunsBySubmission
    destinationIds: string[]
    definition: FormDefinition
    onOpen: (id: string) => void
}) {
    const rows = submissions.map(submission => (
        <SubmissionRowView
            key={submission.id}
            submission={submission}
            runs={runsBySubmission.get(submission.id) ?? []}
            destinationIds={destinationIds}
            definition={definition}
            onOpen={onOpen}
        />
    ))
    return <View className="gap-2">{rows}</View>
}

function SubmissionRowView({
    submission,
    runs,
    destinationIds,
    definition,
    onOpen,
}: {
    submission: SubmissionRow
    runs: RunLike[]
    destinationIds: string[]
    definition: FormDefinition
    onOpen: (id: string) => void
}) {
    const preview = previewOf(definition, submission.values)
    return (
        <Pressable
            onPress={() => onOpen(submission.id)}
            className="rounded-lg border border-border bg-surface-secondary p-3 gap-1"
            testID={`web-form-submission-${submission.id}`}
        >
            <View className="flex-row items-center justify-between gap-2">
                <Text className="text-xs text-muted-foreground">
                    {formatTimeAgo(submission.created)}
                </Text>
                <DeliveryBadge
                    submissionId={submission.id}
                    runs={runs}
                    destinationIds={destinationIds}
                />
            </View>
            <Text className="text-sm text-foreground" numberOfLines={1}>
                {preview}
            </Text>
        </Pressable>
    )
}

function DeliveryBadge({
    submissionId,
    runs,
    destinationIds,
}: {
    submissionId: string
    runs: RunLike[]
    destinationIds: string[]
}) {
    const { delivered, total } = deliveryStatus(destinationIds, runs)
    if (total === 0) return null
    const tone = delivered < total ? 'text-danger' : 'text-muted-foreground'
    return (
        <Text className={`text-xs ${tone}`} testID={`web-form-delivery-${submissionId}`}>
            {`${delivered} of ${total} delivered`}
        </Text>
    )
}

function SubmissionDetail({
    submission,
    definition,
    destinations,
    onClose,
}: {
    submission: SubmissionRow | undefined
    definition: FormDefinition
    destinations: Rules[]
    onClose: () => void
}) {
    return (
        <Dialog
            isOpen={submission !== undefined}
            onClose={onClose}
            title="Submission"
            size="lg"
            testID="web-form-submission-detail"
        >
            <SubmissionDetailBody
                submission={submission}
                definition={definition}
                destinations={destinations}
                onClose={onClose}
            />
        </Dialog>
    )
}

function SubmissionDetailBody({
    submission,
    definition,
    destinations,
    onClose,
}: {
    submission: SubmissionRow | undefined
    definition: FormDefinition
    destinations: Rules[]
    onClose: () => void
}) {
    if (!submission) return null
    return (
        <>
            <Dialog.Body contentClassName="px-5 pb-3 gap-4">
                <Text className="text-xs text-muted-foreground">
                    {formatTimeAgo(submission.created)}
                </Text>
                <AnswerList definition={definition} values={submission.values} />
                <FileList submissionId={submission.id} />
                <DestinationRunList submissionId={submission.id} destinations={destinations} />
            </Dialog.Body>
            <Dialog.Footer>
                <DeleteSubmission submissionId={submission.id} onDeleted={onClose} />
            </Dialog.Footer>
        </>
    )
}

function AnswerList({ definition, values }: { definition: FormDefinition; values: unknown }) {
    const rows = answersOf(definition, values).map(answer => (
        <View key={answer.key} className="gap-0.5">
            <Text className="text-xs text-muted-foreground">{answer.label}</Text>
            <Text className="text-sm text-foreground">{answer.text || '—'}</Text>
        </View>
    ))
    return <View className="gap-2">{rows}</View>
}

function FileList({ submissionId }: { submissionId: string }) {
    const files = useSubmissionFiles(submissionId)
    if (files.length === 0) return null
    const rows = files.map(file => (
        <View key={file.id} className="flex-row items-center justify-between gap-2">
            <Text className="text-sm text-foreground flex-1" numberOfLines={1}>
                {file.name}
            </Text>
            <Button
                variant="outline"
                size="sm"
                onPress={() =>
                    downloadFile({
                        collectionId: 'form_submission_files',
                        recordId: file.id,
                        fileName: file.file,
                        displayName: file.name,
                        mimeType: '',
                        size: file.size,
                    })
                }
            >
                <ButtonText>Download</ButtonText>
            </Button>
        </View>
    ))
    return (
        <View className="gap-2">
            <Text className="text-sm font-semibold text-foreground">Files</Text>
            {rows}
        </View>
    )
}

function DestinationRunList({
    submissionId,
    destinations,
}: {
    submissionId: string
    destinations: Rules[]
}) {
    const runs = useSubmissionRuns(submissionId)
    if (destinations.length === 0) return null
    const rows = destinations.map(rule => (
        <DestinationRunRow
            key={rule.id}
            rule={rule}
            submissionId={submissionId}
            newest={newestRun(rule.id, runs)}
        />
    ))
    return (
        <View className="gap-2">
            <Text className="text-sm font-semibold text-foreground">Destinations</Text>
            {rows}
        </View>
    )
}

type RunRow = ReturnType<typeof useSubmissionRuns>[number]

function useRetryRow(ruleId: string, submissionId: string, newest: RunRow | undefined) {
    const retry = useRetry()
    // The newest run when Retry was pressed: the button reads "Queued" until a
    // newer run arrives through the live query. Derived, so no effect resets
    // it, and no clock is compared, so a skewed client clock cannot hold it.
    const [retriedAfter, setRetriedAfter] = useState<string | undefined>(undefined)
    const isQueued = retriedAfter !== undefined && (newest?.fired_at ?? '') === retriedAfter
    const run = () => {
        setRetriedAfter(newest?.fired_at ?? '')
        retry.mutate({ ruleId, submissionId })
    }
    const isVisible = isQueued || !newest || !runSucceeded(newest)
    return { run, isQueued, isVisible, isPending: retry.isPending }
}

function DestinationRunRow({
    rule,
    submissionId,
    newest,
}: {
    rule: Rules
    submissionId: string
    newest: RunRow | undefined
}) {
    const r = useRetryRow(rule.id, submissionId, newest)
    const label = r.isQueued ? 'Queued' : 'Retry'
    return (
        <View className="rounded-lg border border-border bg-surface-secondary p-3 gap-2">
            <View className="flex-row items-center justify-between gap-2">
                <Text className="text-sm text-foreground flex-1" numberOfLines={1}>
                    {rule.name}
                </Text>
                <RetryButton
                    label={label}
                    onPress={r.run}
                    isDisabled={r.isPending || r.isQueued}
                    isVisible={r.isVisible}
                    testID={`web-form-retry-${rule.id}`}
                />
            </View>
            <RunSummary run={newest} />
        </View>
    )
}

function RetryButton({
    label,
    onPress,
    isDisabled,
    isVisible,
    testID,
}: {
    label: string
    onPress: () => void
    isDisabled: boolean
    isVisible: boolean
    testID: string
}) {
    if (!isVisible) return null
    return (
        <Button
            variant="outline"
            size="sm"
            onPress={onPress}
            isDisabled={isDisabled}
            testID={testID}
        >
            <ButtonText>{label}</ButtonText>
        </Button>
    )
}

function RunSummary({ run }: { run: RunRow | undefined }) {
    if (!run)
        return <Text className="text-xs text-muted-foreground">No run for this submission.</Text>
    return (
        <View className="gap-1">
            <Text className="text-xs text-muted-foreground">{formatTimeAgo(run.fired_at)}</Text>
            <ActionResultsList results={parseResults(run.results)} />
            <RunError error={run.error} />
        </View>
    )
}

function DeleteSubmission({
    submissionId,
    onDeleted,
}: {
    submissionId: string
    onDeleted: () => void
}) {
    const remove = useDeleteSubmission()
    const [isConfirming, setIsConfirming] = useState(false)
    const confirm = () => remove.mutate(submissionId, { onSuccess: onDeleted })
    return (
        <>
            <Button
                variant="outline"
                size="sm"
                onPress={() => setIsConfirming(true)}
                testID="web-form-delete-submission"
            >
                <ButtonText>Delete</ButtonText>
            </Button>
            <ConfirmDialog
                isOpen={isConfirming}
                title="Delete this submission?"
                message="Its files are deleted too. Cards and rows already made stay."
                confirmLabel="Delete"
                isDestructive
                isSubmitting={remove.isPending}
                confirmTestID="web-form-delete-confirm"
                onConfirm={confirm}
                onClose={() => setIsConfirming(false)}
            />
        </>
    )
}
